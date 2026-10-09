package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"
	"github.com/gocql/gocql"
	dmsv1 "github.com/tobib-dev/frnkstn-proto/dms/v1"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type messagesState int

const (
	messagesBrowsing messagesState = iota
	messagesChoosingFriend
	messagesComposing
	messagesSending
	messagesViewingConversation
)

type messagesLoadedMsg struct {
	items []*dmsv1.DirectMessageMetadata
}
type messagesLoadFailedMsg struct{ err error }
type messageFriendsLoadedMsg struct{ items []*friendsv1.GetFriendsItem }
type messageFriendsLoadFailedMsg struct{ err error }
type messageSentMsg struct{}
type messageSendFailedMsg struct{ err error }
type conversationLoadedMsg struct {
	dmID   string
	items  []*dmsv1.DirectMessageItem
	cursor time.Time
}
type conversationLoadFailedMsg struct {
	dmID string
	err  error
}
type messageStreamStartedMsg struct {
	dmID         string
	subscription *messageSubscription
}
type messageStreamReceivedMsg struct {
	dmID         string
	item         *dmsv1.DirectMessageItem
	subscription *messageSubscription
}
type messageStreamClosedMsg struct {
	dmID         string
	subscription *messageSubscription
}
type messageStreamFailedMsg struct {
	dmID         string
	err          error
	subscription *messageSubscription
}

type messageSubscription struct {
	conn   *grpc.ClientConn
	stream grpc.ServerStreamingClient[dmsv1.DirectMessageItem]
	cancel context.CancelFunc
}

func (s *messageSubscription) close() {
	if s == nil {
		return
	}
	s.cancel()
	s.conn.Close()
}

type newMessageItem struct{}

func (newMessageItem) FilterValue() string { return "New Message" }
func (newMessageItem) Title() string       { return "New Message" }
func (newMessageItem) Description() string { return "Start a conversation with a friend" }

type directMessageListItem struct {
	metadata *dmsv1.DirectMessageMetadata
	name     string
}

func (i directMessageListItem) FilterValue() string { return i.Title() + " " + i.metadata.GetDmId() }
func (i directMessageListItem) Title() string {
	if i.name != "" {
		return i.name
	}
	return i.metadata.GetDmId()
}
func (i directMessageListItem) Description() string {
	if i.metadata.GetLastMessageTime() == nil {
		return ""
	}
	return "Last message " + i.metadata.GetLastMessageTime().AsTime().Local().Format("Jan 2, 2006 3:04 PM")
}

type messageFriendItem struct {
	friend *friendsv1.GetFriendsItem
	userID string
}

func (i messageFriendItem) FilterValue() string { return i.Title() }
func (i messageFriendItem) Title() string {
	if username := i.friend.GetFriendUsername(); username != "" {
		return username
	}
	return i.receiverID()
}
func (i messageFriendItem) Description() string { return "" }
func (i messageFriendItem) receiverID() string {
	if i.friend.GetUserId() == i.userID {
		return i.friend.GetFriendId()
	}
	return i.friend.GetUserId()
}

type messagesModel struct {
	list, friends    list.Model
	input            textinput.Model
	state            messagesState
	userID, grpcPort string
	loading          bool
	loadingFriends   bool
	selectedFriend   *friendsv1.GetFriendsItem
	selectedDM       *dmsv1.DirectMessageMetadata
	conversation     []*dmsv1.DirectMessageItem
	subscription     *messageSubscription
	metadata         []*dmsv1.DirectMessageMetadata
	friendNames      map[string]string
	errorText        string
}

func newMessagesModel(width, height int, userID, grpcPort string) messagesModel {
	delegate := list.NewDefaultDelegate()
	messages := list.New([]list.Item{newMessageItem{}}, delegate, width, height)
	messages.Title = "Messages"
	friends := list.New(nil, delegate, width, height)
	friends.Title = "New Message"
	input := textinput.New()
	input.Prompt = "Message: "
	input.SetVirtualCursor(true)
	return messagesModel{
		list: messages, friends: friends, input: input, userID: userID, grpcPort: grpcPort,
		loading: true, friendNames: make(map[string]string),
	}
}

func (m *messagesModel) setSize(width, height int) {
	m.list.SetSize(width, height)
	m.friends.SetSize(width, height)
}

func (m messagesModel) Update(msg tea.Msg) (messagesModel, tea.Cmd) {
	switch msg := msg.(type) {
	case messagesLoadedMsg:
		m.loading = false
		m.errorText = ""
		m.metadata = msg.items
		return m, m.setMessageItems()
	case messagesLoadFailedMsg:
		m.loading = false
		m.errorText = "Could not load messages."
		return m, nil
	case messageFriendsLoadedMsg:
		m.loadingFriends = false
		m.friendNames = make(map[string]string)
		items := make([]list.Item, 0, len(msg.items))
		for _, item := range msg.items {
			if item.GetStatus() == "accepted" {
				friend := messageFriendItem{friend: item, userID: m.userID}
				items = append(items, friend)
				m.friendNames[item.GetFriendshipId()] = friend.Title()
			}
		}
		m.friends.SetItems(items)
		return m, m.setMessageItems()
	case messageFriendsLoadFailedMsg:
		m.loadingFriends = false
		m.errorText = "Could not load friends."
		return m, nil
	case messageSentMsg:
		m.state = messagesBrowsing
		m.selectedFriend = nil
		m.input.Reset()
		m.loading = true
		m.errorText = "Message sent."
		return m, fetchMessagesByUser(m.userID, m.grpcPort)
	case messageSendFailedMsg:
		m.state = messagesComposing
		m.errorText = "Could not send message. Please try again."
		return m, m.input.Focus()
	case conversationLoadedMsg:
		if !m.isViewing(msg.dmID) {
			return m, nil
		}
		m.loading = false
		m.conversation = mergeMessages(nil, msg.items)
		m.setLastMessageTime(msg.dmID, msg.cursor)
		return m, startMessageStream(m.grpcPort, m.selectedDM)
	case conversationLoadFailedMsg:
		if m.isViewing(msg.dmID) {
			m.loading = false
			m.errorText = "Could not load this conversation."
		}
		return m, nil
	case messageStreamStartedMsg:
		if !m.isViewing(msg.dmID) {
			msg.subscription.close()
			return m, nil
		}
		m.stopSubscription()
		m.subscription = msg.subscription
		return m, receiveMessage(msg.dmID, msg.subscription)
	case messageStreamReceivedMsg:
		if !m.isViewing(msg.dmID) || m.subscription != msg.subscription {
			msg.subscription.close()
			return m, nil
		}
		m.conversation = appendMessage(m.conversation, msg.item)
		if cursor, ok := messageTime(msg.item); ok {
			m.setLastMessageTime(msg.dmID, cursor)
		}
		return m, receiveMessage(msg.dmID, msg.subscription)
	case messageStreamClosedMsg:
		if m.isViewing(msg.dmID) && m.subscription == msg.subscription {
			m.stopSubscription()
		}
		return m, nil
	case messageStreamFailedMsg:
		if m.isViewing(msg.dmID) && (msg.subscription == nil || m.subscription == msg.subscription) {
			log.Error("message subscription failed", "dm_id", msg.dmID, "error", msg.err)
			m.stopSubscription()
			m.errorText = "Live message updates stopped."
		}
		return m, nil
	case tea.WindowSizeMsg:
		m.setSize(msg.Width, msg.Height)
	}

	switch m.state {
	case messagesBrowsing:
		if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" && !m.loading {
			if _, ok := m.list.SelectedItem().(newMessageItem); ok {
				m.state = messagesChoosingFriend
				m.loadingFriends = true
				m.errorText = ""
				return m, fetchMessageFriends(m.userID, m.grpcPort)
			}
			if item, ok := m.list.SelectedItem().(directMessageListItem); ok {
				m.state = messagesViewingConversation
				m.selectedDM = item.metadata
				m.conversation = nil
				m.loading = true
				m.errorText = ""
				return m, fetchConversation(item.metadata.GetDmId(), m.grpcPort)
			}
		}
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	case messagesChoosingFriend:
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				m.state = messagesBrowsing
				m.errorText = ""
				return m, nil
			case "enter":
				if item, ok := m.friends.SelectedItem().(messageFriendItem); ok && !m.loadingFriends {
					m.selectedFriend = item.friend
					m.state = messagesComposing
					m.errorText = ""
					return m, m.input.Focus()
				}
			}
		}
		var cmd tea.Cmd
		m.friends, cmd = m.friends.Update(msg)
		return m, cmd
	case messagesComposing:
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				m.input.Blur()
				m.state = messagesChoosingFriend
				m.errorText = ""
				return m, nil
			case "enter":
				content := strings.TrimSpace(m.input.Value())
				if content == "" {
					m.errorText = "Message is required."
					return m, nil
				}
				friend := m.selectedFriend
				m.state = messagesSending
				m.errorText = ""
				m.input.Blur()
				return m, sendDirectMessage(m.userID, m.grpcPort, friend, content)
			}
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	case messagesViewingConversation:
		if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "esc" {
			m.stopSubscription()
			m.state = messagesBrowsing
			m.selectedDM = nil
			m.conversation = nil
			m.loading = false
			m.errorText = ""
		}
		return m, nil
	}
	return m, nil
}

func (m messagesModel) View() string {
	switch m.state {
	case messagesChoosingFriend:
		view := m.friends.View()
		if m.loadingFriends {
			view += "\nLoading friends…"
		} else if len(m.friends.Items()) == 0 {
			view += "\nNo accepted friends."
		}
		if m.errorText != "" {
			view += "\n" + m.errorText
		}
		return view + "\n\nEnter to choose • Esc to go back"
	case messagesComposing, messagesSending:
		name := "friend"
		if m.selectedFriend != nil {
			name = messageFriendItem{friend: m.selectedFriend, userID: m.userID}.Title()
		}
		view := "New message to " + name + "\n\n" + m.input.View()
		if m.state == messagesSending {
			view += "\n\nSending…"
		} else if m.errorText != "" {
			view += "\n\n" + m.errorText
		}
		return view + "\n\nEnter to send • Esc to go back"
	case messagesViewingConversation:
		friendName := m.friendNames[m.selectedDM.GetDmId()]
		if friendName == "" {
			friendName = "Them"
		}
		view := "Conversation with " + friendName + "\n\n"
		if m.loading {
			view += "Loading messages…"
		} else if len(m.conversation) == 0 {
			view += "No messages yet."
		} else {
			for _, item := range m.conversation {
				author := friendName
				if item.GetAuthorId() == m.userID {
					author = "You"
				}
				view += author + ": " + item.GetContent()
				if sentAt, ok := messageTime(item); ok {
					view += " • " + sentAt.Local().Format("Jan 2, 2006 3:04 PM")
				}
				view += "\n"
			}
		}
		if m.errorText != "" {
			view += "\n" + m.errorText
		}
		return view + "\n\nEsc to return to messages"
	default:
		view := m.list.View()
		if m.loading {
			view += "\nLoading messages…"
		}
		if m.errorText != "" {
			view += "\n" + m.errorText
		}
		return view + "\n\nEsc to return home"
	}
}

func (m *messagesModel) setMessageItems() tea.Cmd {
	items := []list.Item{newMessageItem{}}
	for _, item := range m.metadata {
		items = append(items, directMessageListItem{metadata: item, name: m.friendNames[item.GetDmId()]})
	}
	return m.list.SetItems(items)
}

func (m messagesModel) isViewing(dmID string) bool {
	return m.state == messagesViewingConversation && m.selectedDM != nil && m.selectedDM.GetDmId() == dmID
}

func (m *messagesModel) stopSubscription() {
	m.subscription.close()
	m.subscription = nil
}

func (m *messagesModel) setLastMessageTime(dmID string, cursor time.Time) {
	if m.selectedDM != nil && m.selectedDM.GetDmId() == dmID {
		if current := m.selectedDM.GetLastMessageTime(); current != nil && !cursor.After(current.AsTime()) {
			return
		}
		m.selectedDM.LastMessageTime = timestamppb.New(cursor)
	}
}

func fetchMessagesByUser(userID, grpcPort string) tea.Cmd {
	return func() tea.Msg {
		conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return messagesLoadFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		response, err := dmsv1.NewDMServiceClient(conn).GetMessagesByUser(ctx, &dmsv1.GetMessagesByUserRequest{UserId: userID})
		if err != nil {
			return messagesLoadFailedMsg{err: err}
		}
		return messagesLoadedMsg{items: response.GetItems()}
	}
}

func fetchMessageFriends(userID, grpcPort string) tea.Cmd {
	return func() tea.Msg {
		conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return messageFriendsLoadFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		response, err := friendsv1.NewFriendServiceClient(conn).GetFriends(ctx, &friendsv1.GetFriendsRequest{UserId: userID})
		if err != nil {
			return messageFriendsLoadFailedMsg{err: err}
		}
		return messageFriendsLoadedMsg{items: response.GetItems()}
	}
}

func sendDirectMessage(userID, grpcPort string, friend *friendsv1.GetFriendsItem, content string) tea.Cmd {
	return func() tea.Msg {
		if friend == nil {
			return messageSendFailedMsg{err: fmt.Errorf("friend is required")}
		}
		receiverID := friend.GetFriendId()
		if friend.GetUserId() != userID {
			receiverID = friend.GetUserId()
		}
		conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return messageSendFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		_, err = dmsv1.NewDMServiceClient(conn).SendMessages(ctx, &dmsv1.SendMessagesRequest{Item: &dmsv1.DirectMessageItem{
			DmId: friend.GetFriendshipId(), AuthorId: userID, ReceiverId: receiverID, Content: content,
		}})
		if err != nil {
			return messageSendFailedMsg{err: err}
		}
		return messageSentMsg{}
	}
}

func fetchConversation(dmID, grpcPort string) tea.Cmd {
	return func() tea.Msg {
		cursor := time.Now()
		conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return conversationLoadFailedMsg{dmID: dmID, err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		response, err := dmsv1.NewDMServiceClient(conn).GetMessages(ctx, &dmsv1.GetMessagesRequest{
			Item: &dmsv1.DirectMessageItem{DmId: dmID},
		})
		if err != nil {
			return conversationLoadFailedMsg{dmID: dmID, err: err}
		}
		for _, item := range response.GetItems() {
			if sentAt, ok := messageTime(item); ok && sentAt.After(cursor) {
				cursor = sentAt
			}
		}
		return conversationLoadedMsg{dmID: dmID, items: response.GetItems(), cursor: cursor}
	}
}

func startMessageStream(grpcPort string, metadata *dmsv1.DirectMessageMetadata) tea.Cmd {
	return func() tea.Msg {
		if metadata == nil {
			return messageStreamFailedMsg{err: fmt.Errorf("message metadata is required")}
		}
		dmID := metadata.GetDmId()
		if metadata.GetLastMessageTime() == nil {
			return messageStreamFailedMsg{dmID: dmID, err: fmt.Errorf("last message time is required")}
		}
		msgTime := metadata.GetLastMessageTime()
		if err := msgTime.CheckValid(); err != nil {
			return messageStreamFailedMsg{dmID: dmID, err: fmt.Errorf("invalid last message time: %w", err)}
		}
		conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return messageStreamFailedMsg{dmID: dmID, err: err}
		}
		ctx, cancel := context.WithCancel(context.Background())
		stream, err := dmsv1.NewDMServiceClient(conn).SubscribeMessages(ctx, &dmsv1.SubscribeMessagesRequest{
			DmId: dmID, LastMsgTime: msgTime,
		})
		if err != nil {
			cancel()
			conn.Close()
			return messageStreamFailedMsg{dmID: dmID, err: err}
		}
		return messageStreamStartedMsg{dmID: dmID, subscription: &messageSubscription{conn: conn, stream: stream, cancel: cancel}}
	}
}

func messageTime(item *dmsv1.DirectMessageItem) (time.Time, bool) {
	if item == nil {
		return time.Time{}, false
	}
	id, err := gocql.ParseUUID(item.GetMessageId())
	if err != nil || id.Version() != 1 {
		return time.Time{}, false
	}
	return id.Time(), true
}

func receiveMessage(dmID string, subscription *messageSubscription) tea.Cmd {
	return func() tea.Msg {
		item, err := subscription.stream.Recv()
		if err == io.EOF {
			return messageStreamClosedMsg{dmID: dmID, subscription: subscription}
		}
		if err != nil {
			return messageStreamFailedMsg{dmID: dmID, err: err, subscription: subscription}
		}
		return messageStreamReceivedMsg{dmID: dmID, item: item, subscription: subscription}
	}
}

func mergeMessages(current, loaded []*dmsv1.DirectMessageItem) []*dmsv1.DirectMessageItem {
	messages := make([]*dmsv1.DirectMessageItem, 0, len(current)+len(loaded))
	for i := len(loaded) - 1; i >= 0; i-- {
		messages = appendMessage(messages, loaded[i])
	}
	for _, item := range current {
		messages = appendMessage(messages, item)
	}
	return messages
}

func appendMessage(messages []*dmsv1.DirectMessageItem, item *dmsv1.DirectMessageItem) []*dmsv1.DirectMessageItem {
	if item == nil {
		return messages
	}
	for _, existing := range messages {
		if existing.GetMessageId() == item.GetMessageId() {
			return messages
		}
	}
	return append(messages, item)
}
