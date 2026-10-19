package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gocql/gocql"
	dmsv1 "github.com/tobib-dev/frnkstn-proto/dms/v1"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	notificationsv1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type messagesTestServer struct {
	dmsv1.UnimplementedDMServiceServer
	friendsv1.UnimplementedFriendServiceServer
	requestedUser string
	requestedDM   string
	subscribedDM  string
	subscribedAt  time.Time
	incomingID    string
	sent          *dmsv1.DirectMessageItem
}

func (s *messagesTestServer) GetMessagesByUser(_ context.Context, req *dmsv1.GetMessagesByUserRequest) (*dmsv1.GetMessagesByUserResponse, error) {
	s.requestedUser = req.GetUserId()
	return &dmsv1.GetMessagesByUserResponse{Items: []*dmsv1.DirectMessageMetadata{{
		DmId: "existing-friendship", LastMessageTime: timestamppb.New(time.Unix(1, 0)),
	}}}, nil
}

func (s *messagesTestServer) GetFriends(_ context.Context, req *friendsv1.GetFriendsRequest) (*friendsv1.GetFriendsResponse, error) {
	return &friendsv1.GetFriendsResponse{Items: []*friendsv1.GetFriendsItem{
		{UserId: req.GetUserId(), FriendshipId: "new-friendship", FriendId: "friend", FriendUsername: "bob", Status: "accepted"},
		{UserId: req.GetUserId(), FriendshipId: "existing-friendship", FriendId: "existing-friend", FriendUsername: "alice", Status: "accepted"},
	}}, nil
}

func (s *messagesTestServer) SendMessages(_ context.Context, req *dmsv1.SendMessagesRequest) (*dmsv1.SendMessagesResponse, error) {
	s.sent = req.GetItem()
	s.sent.MessageId = gocql.TimeUUID().String()
	return &dmsv1.SendMessagesResponse{Item: req.GetItem()}, nil
}

func (s *messagesTestServer) GetMessages(_ context.Context, req *dmsv1.GetMessagesRequest) (*dmsv1.GetMessagesResponse, error) {
	s.requestedDM = req.GetItem().GetDmId()
	return &dmsv1.GetMessagesResponse{Items: []*dmsv1.DirectMessageItem{
		{DmId: s.requestedDM, MessageId: gocql.TimeUUID().String(), AuthorId: "friend", Content: "hello from them"},
		{DmId: s.requestedDM, MessageId: gocql.UUIDFromTime(time.Unix(2, 0)).String(), AuthorId: "user", Content: "hello from me"},
	}}, nil
}

func (s *messagesTestServer) SubscribeMessages(req *dmsv1.SubscribeMessagesRequest, stream grpc.ServerStreamingServer[dmsv1.DirectMessageItem]) error {
	s.subscribedDM = req.GetDmId()
	s.subscribedAt = req.GetLastMsgTime().AsTime()
	s.incomingID = gocql.UUIDFromTime(s.subscribedAt.Add(time.Second)).String()
	return stream.Send(&dmsv1.DirectMessageItem{DmId: req.GetDmId(), MessageId: s.incomingID, AuthorId: "friend", Content: "live message"})
}

func TestMessagesOptionLoadsMetadataAndStartsMessage(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &messagesTestServer{}
	server := grpc.NewServer()
	dmsv1.RegisterDMServiceServer(server, service)
	friendsv1.RegisterFriendServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())

	home := newHomeModel(80, 24)
	home.user = userInfo{userID: "user"}
	home.grpcPort = port
	home.list.Select(0)
	home, cmd := home.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || home.state != homeViewingMessages {
		t.Fatal("Messages did not start loading")
	}
	home, _ = home.Update(cmd())
	view := home.View().Content
	if service.requestedUser != "user" || !strings.Contains(view, "New Message") || !strings.Contains(view, "existing-friendship") {
		t.Fatalf("messages were not loaded: user=%q view=%q", service.requestedUser, view)
	}

	home, cmd = home.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || home.messages.state != messagesChoosingFriend {
		t.Fatal("New Message did not load friends")
	}
	home, _ = home.Update(cmd())
	if !strings.Contains(home.View().Content, "bob") {
		t.Fatal("accepted friend was not listed")
	}
	home, _ = home.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	home.messages.input.SetValue("hello")
	home, cmd = home.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || home.messages.state != messagesSending {
		t.Fatal("message was not submitted")
	}
	home, cmd = home.Update(cmd())
	if service.sent == nil || service.sent.GetDmId() != "new-friendship" || service.sent.GetAuthorId() != "user" || service.sent.GetReceiverId() != "friend" || service.sent.GetContent() != "hello" {
		t.Fatalf("unexpected sent message: %+v", service.sent)
	}
	home, _ = home.Update(cmd())
	home.messages.list.Select(0)
	home, cmd = home.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	home, _ = home.Update(cmd())
	home.messages.friends.Select(1)
	home, _ = home.Update(notificationsPolledMsg{items: []notificationItem{{
		kind: messageNotification, message: &notificationsv1.MsgN10NItem{Id: "notification", AuthorId: "existing-friend"},
	}}})
	if got := home.messageCounter.Load(); got != 1 {
		t.Fatalf("incoming conversation was not counted: %d", got)
	}
	home, cmd = home.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || home.messages.state != messagesViewingConversation || home.messages.selectedDM.GetDmId() != "existing-friendship" {
		t.Fatal("selecting an existing friend did not open the previous conversation")
	}
	if got := home.messageCounter.Load(); got != 0 {
		t.Fatalf("opening a conversation did not clear its message count: %d", got)
	}
}

func TestOpeningConversationDisplaysHistoryAndIncomingMessage(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &messagesTestServer{}
	server := grpc.NewServer()
	dmsv1.RegisterDMServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())

	model := newMessagesModel(80, 24, "user", port)
	model, _ = model.Update(messageFriendsLoadedMsg{items: []*friendsv1.GetFriendsItem{{
		UserId: "user", FriendshipId: "friendship", FriendId: "friend", FriendUsername: "bob", Status: "accepted",
	}}})
	model, _ = model.Update(messagesLoadedMsg{items: []*dmsv1.DirectMessageMetadata{{
		DmId: "friendship", LastMessageTime: timestamppb.New(time.Unix(1, 0)),
	}}})
	model.list.Select(1)
	model, cmd := model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || model.state != messagesViewingConversation {
		t.Fatal("conversation did not start loading")
	}

	model, cmd = model.Update(cmd())
	if cmd == nil || service.requestedDM != "friendship" {
		t.Fatal("conversation history was not loaded")
	}
	model, cmd = model.Update(cmd())
	if cmd == nil || model.subscription == nil {
		t.Fatal("message subscription was not started")
	}
	model, cmd = model.Update(cmd())
	if cmd == nil || service.subscribedDM != "friendship" {
		t.Fatal("next stream receive was not scheduled")
	}
	if !model.subscription.firstResponseLogged {
		t.Fatal("message subscription did not record its first response")
	}
	if !service.subscribedAt.After(time.Unix(1, 0)) {
		t.Fatalf("subscription used stale cursor: %v", service.subscribedAt)
	}
	incomingTime, _ := messageTime(&dmsv1.DirectMessageItem{MessageId: service.incomingID})
	if !model.selectedDM.GetLastMessageTime().AsTime().Equal(incomingTime) {
		t.Fatalf("cursor was not advanced to incoming message: got %v want %v", model.selectedDM.GetLastMessageTime().AsTime(), incomingTime)
	}
	if len(model.seenMessageIDs) != 3 {
		t.Fatalf("displayed messages were not recorded as seen: %v", model.seenMessageIDs)
	}

	view := model.View()
	for _, text := range []string{"Conversation with bob", "You: hello from me •", "bob: hello from them •", "bob: live message •"} {
		if !strings.Contains(view, text) {
			t.Fatalf("conversation view missing %q: %q", text, view)
		}
	}
	if !strings.Contains(view, "Message: ") {
		t.Fatalf("conversation is missing the reply input: %q", view)
	}

	model.input.SetValue("reply from conversation")
	model, cmd = model.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !model.sending || model.state != messagesViewingConversation {
		t.Fatal("conversation reply was not submitted")
	}
	model, _ = model.Update(cmd())
	if model.sending || model.state != messagesViewingConversation || model.input.Value() != "" || service.sent.GetDmId() != "friendship" {
		t.Fatal("conversation reply did not complete in the open thread")
	}
	if !strings.Contains(model.View(), "You: reply from conversation") {
		t.Fatal("sent conversation reply was not displayed")
	}
	model, _ = model.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if model.state != messagesBrowsing || model.subscription != nil {
		t.Fatal("leaving conversation did not close the subscription")
	}
}
