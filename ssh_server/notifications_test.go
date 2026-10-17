package main

import (
	"context"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	notificationsv1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type notificationTestServer struct {
	notificationsv1.UnimplementedN10NServiceServer
	messages      []*notificationsv1.MsgN10NItem
	friends       []*notificationsv1.FriendN10NItem
	updated       []string
	subscriptions int
}

func (s *notificationTestServer) GetMsgN10N(context.Context, *notificationsv1.GetMsgN10NRequest) (*notificationsv1.GetMsgN10NResponse, error) {
	if len(s.messages) == 0 {
		return nil, status.Error(codes.NotFound, "none")
	}
	return &notificationsv1.GetMsgN10NResponse{Item: s.messages[0]}, nil
}

func (s *notificationTestServer) GetFriendN10N(context.Context, *notificationsv1.GetFriendN10NRequest) (*notificationsv1.GetFriendN10NResponse, error) {
	if len(s.friends) == 0 {
		return nil, status.Error(codes.NotFound, "none")
	}
	return &notificationsv1.GetFriendN10NResponse{Item: s.friends[0]}, nil
}

func (s *notificationTestServer) UpdateMsgN10N(_ context.Context, req *notificationsv1.UpdateMsgN10NRequest) (*emptypb.Empty, error) {
	s.updated = append(s.updated, req.GetItem().GetId())
	s.messages = s.messages[1:]
	return &emptypb.Empty{}, nil
}

func (s *notificationTestServer) UpdateFriendN10N(_ context.Context, req *notificationsv1.UpdateFriendN10NRequest) (*emptypb.Empty, error) {
	s.updated = append(s.updated, req.GetItem().GetId())
	s.friends = s.friends[1:]
	return &emptypb.Empty{}, nil
}

func (s *notificationTestServer) Subscribe(_ *notificationsv1.SubscribeRequest, stream grpc.ServerStreamingServer[notificationsv1.SubscribeResponse]) error {
	s.subscriptions++
	for _, message := range s.messages {
		if err := stream.Send(&notificationsv1.SubscribeResponse{Item: &notificationsv1.SubscribeResponse_Message{Message: message}}); err != nil {
			return err
		}
	}
	for _, friend := range s.friends {
		if err := stream.Send(&notificationsv1.SubscribeResponse{Item: &notificationsv1.SubscribeResponse_Friend{Friend: friend}}); err != nil {
			return err
		}
	}
	return nil
}

func TestHomeNotificationsGetAndMarkSeen(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &notificationTestServer{
		messages: []*notificationsv1.MsgN10NItem{
			{Id: "message-1", AuthorName: "Alice", Content: "hello"},
			{Id: "message-2", AuthorName: "Alice", Content: "again"},
		},
		friends: []*notificationsv1.FriendN10NItem{{Id: "friend-1", FriendName: "Bob", Status: "pending"}},
	}
	server := grpc.NewServer()
	notificationsv1.RegisterN10NServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())

	m := newHomeModel(80, 24)
	m.user = userInfo{userID: "user"}
	m.grpcPort = port
	m.list.Select(3)
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || m.state != homeViewingNotifications {
		t.Fatal("notifications screen did not open")
	}
	m, _ = m.Update(cmd())
	if view := m.View().Content; !strings.Contains(view, "Message from Alice: hello") || !strings.Contains(view, "Friend request from Bob") {
		t.Fatalf("notifications were not displayed: %q", view)
	}
	m, cmd = m.Update(notificationsPollTickMsg{})
	m, _ = m.Update(cmd())
	if service.subscriptions != 2 {
		t.Fatalf("notification polling did not subscribe again: %d", service.subscriptions)
	}

	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.notifications.updating {
		t.Fatal("message notification was not marked as seen")
	}
	m, _ = m.Update(cmd())
	if len(service.updated) != 1 || service.updated[0] != "message-1" || !strings.Contains(m.View().Content, "again") {
		t.Fatalf("message notification was not replaced: updates=%v view=%q", service.updated, m.View().Content)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(cmd())
	if len(service.updated) != 2 || service.updated[1] != "friend-1" || strings.Contains(m.View().Content, "Friend request from Bob") {
		t.Fatalf("friend notification was not marked seen: updates=%v view=%q", service.updated, m.View().Content)
	}
}

func TestHomeNotificationsSkipsMessagesAlreadyDisplayed(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &notificationTestServer{messages: []*notificationsv1.MsgN10NItem{
		{Id: "notification-seen", MessageId: "message-seen", AuthorName: "Alice", Content: "already read"},
		{Id: "notification-unread", MessageId: "message-unread", AuthorName: "Alice", Content: "new message"},
	}}
	server := grpc.NewServer()
	notificationsv1.RegisterN10NServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())

	m := newHomeModel(80, 24)
	m.user = userInfo{userID: "user"}
	m.grpcPort = port
	m.messages.seenMessageIDs = map[string]struct{}{"message-seen": {}}
	m.list.Select(3)
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(cmd())

	view := m.View().Content
	if len(service.updated) != 1 || service.updated[0] != "notification-seen" || strings.Contains(view, "already read") || !strings.Contains(view, "new message") {
		t.Fatalf("seen notification was not deduplicated: updates=%v view=%q", service.updated, view)
	}
}
