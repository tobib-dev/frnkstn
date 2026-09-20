package main

import (
	"context"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"google.golang.org/grpc"
)

type signInTestSessionServer struct {
	sessionsV1.UnimplementedSessionServiceServer
	usersV1.UnimplementedUserServiceServer
}

func (signInTestSessionServer) GetUserByGHID(context.Context, *usersV1.GetUserByGHIDRequest) (*usersV1.GetUserByGHIDResponse, error) {
	return &usersV1.GetUserByGHIDResponse{UserId: "test-user", Username: "alice"}, nil
}

func (signInTestSessionServer) GetSession(context.Context, *sessionsV1.GetSessionRequest) (*sessionsV1.GetSessionResponse, error) {
	return &sessionsV1.GetSessionResponse{SessionId: "test-session", UserId: "test-user"}, nil
}

func (signInTestSessionServer) CreateSession(context.Context, *sessionsV1.CreateSessionRequest) (*sessionsV1.CreateSessionResponse, error) {
	return &sessionsV1.CreateSessionResponse{SessionId: "test-session", UserId: "test-user"}, nil
}

func TestSuccessfulSignInOpensHome(t *testing.T) {
	mockGitHubUser(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	sessionsV1.RegisterSessionServiceServer(server, signInTestSessionServer{})
	usersV1.RegisterUserServiceServer(server, signInTestSessionServer{})
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}

	m := mainModel{
		state:  signInView,
		signIn: newSignInModel(80, 24, "test-client", port),
		home:   newHomeModel(80, 24),
	}
	updated, cmd := m.Update(signInSuccessMsg{token: tokenInfo{AccessToken: "test-token"}})
	if cmd == nil {
		t.Fatal("expected home navigation command")
	}
	updated, cmd = updated.Update(cmd())
	updated, cmd = updated.Update(cmd())
	updated, _ = updated.Update(cmd())
	home := updated.(mainModel)
	if home.state != homeView || home.signIn.token.AccessToken != "test-token" || home.signIn.session.sessionID != "test-session" {
		t.Fatal("successful sign-in did not open home with the token retained")
	}
	view := home.View().Content
	for _, label := range []string{"Sign in successful", "Messages", "Groups", "Profile", "Exit"} {
		if !strings.Contains(view, label) {
			t.Errorf("home view is missing %q", label)
		}
	}
	if len(home.home.list.Items()) != 4 {
		t.Fatal("expected exactly four home options")
	}
}

func TestHomeExitQuits(t *testing.T) {
	m := newHomeModel(80, 24)
	m.list.Select(3)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected exit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("exit did not quit")
	}
}
