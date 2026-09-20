package main

import (
	"context"
	"errors"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	github "github.com/tobib-dev/frnkstn/internal/github"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"net"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"google.golang.org/grpc"
)

type usernameTestServer struct {
	usersV1.UnimplementedUserServiceServer
	sessionsV1.UnimplementedSessionServiceServer
	calls    []string
	username string
}

func (s *usernameTestServer) CreateUser(_ context.Context, req *usersV1.CreateUserRequest) (*usersV1.CreateUserResponse, error) {
	s.calls = append(s.calls, "create user")
	s.username = req.Username
	if req.GithubUserId != "123" {
		return nil, status.Error(codes.Unauthenticated, "missing token")
	}
	return &usersV1.CreateUserResponse{UserId: "new-user"}, nil
}

func (s *usernameTestServer) GetUserByGHID(_ context.Context, req *usersV1.GetUserByGHIDRequest) (*usersV1.GetUserByGHIDResponse, error) {
	s.calls = append(s.calls, "get user")
	if req.GithubUserId != "123" {
		return nil, status.Error(codes.Unauthenticated, "missing token")
	}
	return nil, status.Error(codes.NotFound, "user not found")
}
func (s *usernameTestServer) CreateSession(_ context.Context, req *sessionsV1.CreateSessionRequest) (*sessionsV1.CreateSessionResponse, error) {
	s.calls = append(s.calls, "create session")
	if req.UserId != "new-user" || req.AccessToken != "test-token" {
		return nil, status.Error(codes.InvalidArgument, "wrong user or token")
	}
	return &sessionsV1.CreateSessionResponse{SessionId: "session", UserId: req.UserId}, nil
}

func TestNewUserCreatesAccountBeforeHome(t *testing.T) {
	mockGitHubUser(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	users := &usernameTestServer{}
	usersV1.RegisterUserServiceServer(server, users)
	sessionsV1.RegisterSessionServiceServer(server, users)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	m := newSignInModel(80, 24, "client", port)
	m, cmd := m.Update(signInSuccessMsg{token: tokenInfo{AccessToken: "test-token"}})
	m, _ = m.Update(cmd())
	if !reflect.DeepEqual(users.calls, []string{"get user"}) {
		t.Fatalf("session created before account: %v", users.calls)
	}
	if m.state != signInUsername || !m.username.input.Focused() || !strings.Contains(m.View().Content, "enter username") {
		t.Fatal("new user did not get a focused username prompt")
	}
	m.username.input.SetValue("  alice  ")
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.username.submitting {
		t.Fatal("expected account creation command")
	}
	_, duplicate := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if duplicate != nil {
		t.Fatal("submitted account twice")
	}
	m, cmd = m.Update(cmd())
	if m.user.userID != "new-user" || users.username != "alice" {
		t.Fatal("account creation did not retain user or submit trimmed username")
	}
	if cmd == nil {
		t.Fatal("expected home navigation")
	}
	m, cmd = m.Update(cmd())
	if m.session.userID != "new-user" || !reflect.DeepEqual(users.calls, []string{"get user", "create user", "create session"}) {
		t.Fatalf("wrong session or call order: %v", users.calls)
	}
	if _, ok := cmd().(SwitchToHomeMsg); !ok {
		t.Fatal("account creation did not navigate home")
	}
}

func TestUsernameValidationAndRetry(t *testing.T) {
	m := newUsernameModel("7789", github.User{ID: 123})
	m.input.Focus()
	m.input.SetValue("   ")
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !strings.Contains(m.View(), "Username is required") {
		t.Fatal("blank username was accepted")
	}
	m.input.SetValue("alice")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(usernameFailedMsg{err: errors.New("private backend detail")})
	if m.submitting || !m.input.Focused() || m.input.Value() != "alice" {
		t.Fatal("failed submission cannot be edited and retried")
	}
	if !strings.Contains(m.View(), "Please try again") || strings.Contains(m.View(), "private backend detail") {
		t.Fatal("unexpected error message")
	}
}
