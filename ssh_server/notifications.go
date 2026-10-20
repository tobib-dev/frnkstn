package main

import (
	"context"
	"fmt"
	"io"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/log/v2"
	notificationsv1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
)

type notificationKind int

const (
	messageNotification notificationKind = iota
	friendNotification
)

type notificationItem struct {
	kind    notificationKind
	message *notificationsv1.MsgN10NItem
	friend  *notificationsv1.FriendN10NItem
}

func (n notificationItem) label() string {
	if n.kind == messageNotification {
		return fmt.Sprintf("Message from %s: %s", n.message.GetAuthorName(), n.message.GetContent())
	}
	return fmt.Sprintf("Friend request from %s (%s)", n.friend.GetFriendName(), n.friend.GetStatus())
}

func (n notificationItem) id() string {
	if n.kind == messageNotification {
		return n.message.GetId()
	}
	return n.friend.GetId()
}

type notificationsPolledMsg struct{ items []notificationItem }
type notificationsPollFailedMsg struct{ err error }
type notificationsPollTickMsg struct{}
type notificationUpdatedMsg struct {
	index             int
	replacement       *notificationItem
	enterConversation bool
	authorID          string
}
type notificationUpdateFailedMsg struct{ err error }
type enterNotificationConversationMsg struct{ authorID string }

type notificationAction int

const (
	markNotificationSeen notificationAction = iota
	ignoreNotification
	enterNotificationConversation
)

type notificationsModel struct {
	userID, grpcPort string
	items            []notificationItem
	cursor           int
	selected         int
	actionCursor     int
	loading          bool
	updating         bool
	errorText        string
}

func newNotificationsModel(userID, grpcPort string) notificationsModel {
	return notificationsModel{userID: userID, grpcPort: grpcPort, selected: -1, loading: true}
}

func (m notificationsModel) Update(msg tea.Msg) (notificationsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case notificationUpdatedMsg:
		m.updating = false
		if msg.replacement == nil {
			m.items = append(m.items[:msg.index], m.items[msg.index+1:]...)
			if m.cursor >= len(m.items) && m.cursor > 0 {
				m.cursor--
			}
		} else {
			m.items[msg.index] = *msg.replacement
		}
		m.selected = -1
		m.errorText = ""
		if msg.enterConversation {
			return m, func() tea.Msg { return enterNotificationConversationMsg{authorID: msg.authorID} }
		}
		return m, nil
	case notificationUpdateFailedMsg:
		m.updating = false
		m.errorText = "Could not mark notification as seen."
		return m, nil
	}
	if m.loading || m.updating {
		return m, nil
	}
	if m.selected >= 0 {
		return m.updateSelected(msg)
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.items) == 0 {
				return m, nil
			}
			m.selected, m.actionCursor, m.errorText = m.cursor, 0, ""
		}
	}
	return m, nil
}

func (m notificationsModel) updateSelected(msg tea.Msg) (notificationsModel, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	actions := m.actions()
	switch key.String() {
	case "up":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "down":
		if m.actionCursor < len(actions)-1 {
			m.actionCursor++
		}
	case "esc":
		m.selected = -1
		m.errorText = ""
	case "enter":
		action := actions[m.actionCursor]
		if action == ignoreNotification {
			m.selected = -1
			return m, nil
		}
		m.updating, m.errorText = true, ""
		return m, updateNotification(m.userID, m.grpcPort, m.selected, m.items[m.selected], action == enterNotificationConversation)
	}
	return m, nil
}

func (m notificationsModel) actions() []notificationAction {
	actions := []notificationAction{markNotificationSeen, ignoreNotification}
	if m.items[m.selected].kind == messageNotification {
		actions = append(actions, enterNotificationConversation)
	}
	return actions
}

func (a notificationAction) label() string {
	switch a {
	case markNotificationSeen:
		return "Mark as seen"
	case ignoreNotification:
		return "Ignore"
	default:
		return "Enter conversation"
	}
}

func (m notificationsModel) View() string {
	if m.selected >= 0 {
		view := m.items[m.selected].label() + "\n\n"
		for i, action := range m.actions() {
			cursor := "  "
			if i == m.actionCursor {
				cursor = "> "
			}
			view += cursor + action.label() + "\n"
		}
		if m.updating {
			view += "\nMarking notification as seen…"
		} else if m.errorText != "" {
			view += "\n" + m.errorText
		}
		return view + "\n\n↑/↓ to select • Enter to confirm • Esc to go back • Ctrl+C to quit"
	}
	view := "Notifications\n"
	switch {
	case m.loading:
		view += "\nLoading…"
	case len(m.items) == 0:
		view += "\nNo new notifications."
	default:
		for i, item := range m.items {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}
			view += "\n" + cursor + item.label()
		}
	}
	if m.updating {
		view += "\n\nMarking notification as seen…"
	} else if m.errorText != "" {
		view += "\n\n" + m.errorText
	}
	return view + "\n\n↑/↓ to select • Enter for options • Esc for home • Ctrl+C to quit"
}

const notificationPollInterval = 5 * time.Second

func pollNotifications(userID, grpcPort string) tea.Cmd {
	return func() tea.Msg {
		conn, err := newGRPCClient("localhost:" + grpcPort)
		if err != nil {
			return notificationsPollFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		client := notificationsv1.NewN10NServiceClient(conn)
		startedAt := time.Now()
		stream, err := client.Subscribe(ctx, &notificationsv1.SubscribeRequest{UserId: userID})
		if err != nil {
			return notificationsPollFailedMsg{err: err}
		}
		items := make([]notificationItem, 0, 2)
		firstResponseLogged := false
		for {
			response, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				return notificationsPollFailedMsg{err: err}
			}
			if !firstResponseLogged {
				log.Info("subscription first response received", "method", notificationsv1.N10NService_Subscribe_FullMethodName, "user_id", userID, "time_to_first_response_ms", time.Since(startedAt).Seconds()*1000)
				firstResponseLogged = true
			}
			if message := response.GetMessage(); message != nil {
				items = append(items, notificationItem{kind: messageNotification, message: message})
			} else if friend := response.GetFriend(); friend != nil {
				items = append(items, notificationItem{kind: friendNotification, friend: friend})
			}
		}
		return notificationsPolledMsg{items: items}
	}
}

func scheduleNotificationPoll() tea.Cmd {
	return tea.Tick(notificationPollInterval, func(time.Time) tea.Msg { return notificationsPollTickMsg{} })
}

func updateNotification(userID, grpcPort string, index int, item notificationItem, enterConversation bool) tea.Cmd {
	return func() tea.Msg {
		conn, err := newGRPCClient("localhost:" + grpcPort)
		if err != nil {
			return notificationUpdateFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		client := notificationsv1.NewN10NServiceClient(conn)
		if item.kind == messageNotification {
			_, err = client.UpdateMsgN10N(ctx, &notificationsv1.UpdateMsgN10NRequest{Item: &notificationsv1.MsgN10NItem{Id: item.message.GetId(), UserId: userID}})
			if err != nil {
				return notificationUpdateFailedMsg{err: err}
			}
		} else {
			_, err = client.UpdateFriendN10N(ctx, &notificationsv1.UpdateFriendN10NRequest{Item: &notificationsv1.FriendN10NItem{Id: item.friend.GetId(), UserId: userID}})
			if err != nil {
				return notificationUpdateFailedMsg{err: err}
			}
		}
		return notificationUpdatedMsg{index: index, enterConversation: enterConversation, authorID: item.message.GetAuthorId()}
	}
}
