package main

import (
	"context"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	dmsv1 "github.com/tobib-dev/frnkstn-proto/dms/v1"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"google.golang.org/grpc"
)

type signInTestSessionServer struct {
	sessionsV1.UnimplementedSessionServiceServer
	usersV1.UnimplementedUserServiceServer
	dmsv1.UnimplementedDMServiceServer
	friendsv1.UnimplementedFriendServiceServer
	requestedMessagesFor string
}

func (*signInTestSessionServer) GetUserByGHID(context.Context, *usersV1.GetUserByGHIDRequest) (*usersV1.GetUserByGHIDResponse, error) {
	return &usersV1.GetUserByGHIDResponse{UserId: "test-user", Username: "alice"}, nil
}

func (*signInTestSessionServer) GetSession(context.Context, *sessionsV1.GetSessionRequest) (*sessionsV1.GetSessionResponse, error) {
	return &sessionsV1.GetSessionResponse{SessionId: "test-session", UserId: "test-user"}, nil
}

func (*signInTestSessionServer) CreateSession(context.Context, *sessionsV1.CreateSessionRequest) (*sessionsV1.CreateSessionResponse, error) {
	return &sessionsV1.CreateSessionResponse{SessionId: "test-session", UserId: "test-user"}, nil
}

func (s *signInTestSessionServer) GetMessagesByUser(_ context.Context, req *dmsv1.GetMessagesByUserRequest) (*dmsv1.GetMessagesByUserResponse, error) {
	s.requestedMessagesFor = req.GetUserId()
	return &dmsv1.GetMessagesByUserResponse{Items: []*dmsv1.DirectMessageMetadata{{DmId: "friendship"}}}, nil
}

func (*signInTestSessionServer) GetFriends(context.Context, *friendsv1.GetFriendsRequest) (*friendsv1.GetFriendsResponse, error) {
	return &friendsv1.GetFriendsResponse{Items: []*friendsv1.GetFriendsItem{{
		FriendshipId: "friendship", FriendUsername: "bob", Status: "accepted",
	}}}, nil
}

func TestSuccessfulSignInOpensHome(t *testing.T) {
	mockGitHubUser(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	service := &signInTestSessionServer{}
	sessionsV1.RegisterSessionServiceServer(server, service)
	usersV1.RegisterUserServiceServer(server, service)
	dmsv1.RegisterDMServiceServer(server, service)
	friendsv1.RegisterFriendServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	m := mainModel{
		state: signInView, width: 80, height: 24,
		signIn: newSignInModel(80, 24, "test-client", port),
		home:   newHomeModel(80, 24),
	}
	updated, cmd := m.Update(signInSuccessMsg{token: tokenInfo{AccessToken: "test-token"}})
	if cmd == nil {
		t.Fatal("expected home navigation command")
	}
	updated, cmd = updated.Update(cmd())
	updated, cmd = updated.Update(cmd())
	updated, cmd = updated.Update(cmd())
	batch, ok := cmd().(tea.BatchMsg)
	if !ok || len(batch) != 3 {
		t.Fatal("sign-in did not schedule message metadata fetch")
	}
	updated, _ = updated.Update(batch[1]())
	updated, _ = updated.Update(batch[2]())
	home := updated.(mainModel)
	if home.state != homeView || home.signIn.token.AccessToken != "test-token" || home.signIn.session.sessionID != "test-session" {
		t.Fatal("successful sign-in did not open home with the token retained")
	}
	view := home.View().Content
	for _, label := range []string{"Sign in successful", "Messages", "Groups", "Add friend", "Profile", "Sign out", "Exit"} {
		if !strings.Contains(view, label) {
			t.Errorf("home view is missing %q", label)
		}
	}
	if len(home.home.list.Items()) != 6 {
		t.Fatal("expected exactly six home options")
	}
	if service.requestedMessagesFor != "test-user" || home.home.messages.loading {
		t.Fatal("messages were not fetched during sign-in")
	}
	messagesView := home.home.messages.View()
	if !strings.Contains(messagesView, "bob") || strings.Contains(messagesView, "friendship") {
		t.Fatalf("message list did not use the friend's name: %q", messagesView)
	}
}

func TestHomeExitQuits(t *testing.T) {
	m := newHomeModel(80, 24)
	m.list.Select(5)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected exit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("exit did not quit")
	}
}
