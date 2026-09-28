package main

import (
	"context"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type profileTestServer struct {
	usersV1.UnimplementedUserServiceServer
	requests   chan *usersV1.UpdateUserRequest
	deletes    chan *usersV1.DeleteUserRequest
	fail       bool
	deleteFail bool
}

func (s *profileTestServer) DeleteUser(_ context.Context, req *usersV1.DeleteUserRequest) (*usersV1.DeleteUserResponse, error) {
	s.deletes <- req
	if s.deleteFail {
		return nil, status.Error(codes.Unavailable, "unavailable")
	}
	return &usersV1.DeleteUserResponse{Status: "deleted"}, nil
}

func (s *profileTestServer) UpdateUser(_ context.Context, req *usersV1.UpdateUserRequest) (*usersV1.UpdateUserResponse, error) {
	s.requests <- req
	if s.fail {
		return nil, status.Error(codes.AlreadyExists, "taken")
	}
	return &usersV1.UpdateUserResponse{UserId: req.UserId, Name: req.Name, Username: req.Username}, nil
}

func TestProfileSave(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			service := &profileTestServer{requests: make(chan *usersV1.UpdateUserRequest, 1), deletes: make(chan *usersV1.DeleteUserRequest, 1), fail: fail}
			server := grpc.NewServer()
			usersV1.RegisterUserServiceServer(server, service)
			t.Cleanup(server.Stop)
			go server.Serve(listener)
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			original := userInfo{userID: "user", name: "Alice", username: "alice"}
			m := mainModel{home: newHomeModel(80, 24), signIn: newSignInModel(80, 24, "client", port)}
			m.signIn.user = original
			m.signIn.session = sessionInfo{sessionID: "session", userID: "user"}
			updated, _ := m.Update(SwitchToHomeMsg{})
			m = updated.(mainModel)
			m.home.list.Select(2)
			updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = updated.(mainModel)
			if m.home.state != homeEditingProfile || m.home.profile.name.Value() != "Alice" || m.home.profile.username.Value() != "alice" {
				t.Fatal("profile was not prefilled")
			}
			if !m.home.profile.name.Focused() || m.home.profile.username.Focused() {
				t.Fatal("name field was not focused when opening profile")
			}
			updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			m = updated.(mainModel)
			if !m.home.profile.username.Focused() || m.home.profile.name.Focused() {
				t.Fatal("Tab did not focus the username field")
			}
			updated, _ = m.Update(tea.KeyPressMsg{Code: '9', Text: "9"})
			m = updated.(mainModel)
			if m.home.profile.username.Value() != "alice9" {
				t.Fatal("username field did not receive keyboard input")
			}
			m.home.profile.name.SetValue("  Alice Example  ")
			m.home.profile.username.SetValue("  alice19  ")
			updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = updated.(mainModel)
			if cmd == nil || !m.home.profile.saving {
				t.Fatal("save did not start")
			}
			_, duplicate := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if duplicate != nil {
				t.Fatal("duplicate submission")
			}
			result := cmd()
			req := <-service.requests
			if req.UserId != "user" || req.Name != "Alice Example" || req.Username != "alice19" {
				t.Fatalf("incorrect update: %v", req)
			}
			updated, _ = m.Update(result)
			m = updated.(mainModel)
			if fail {
				if m.home.state != homeEditingProfile || m.home.profile.saving || m.signIn.user != original || m.home.user != original || !strings.Contains(m.View().Content, "Username already exists") {
					t.Fatal("failed save did not preserve profile and show error")
				}
				if m.home.profile.name.Value() != "  Alice Example  " {
					t.Fatal("edits were lost")
				}
				_, retry := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if retry == nil {
					t.Fatal("cannot retry")
				}
			} else {
				want := userInfo{userID: "user", name: "Alice Example", username: "alice19"}
				if m.home.state != homeReady || m.home.user != want || m.signIn.user != want {
					t.Fatal("updated profile was not retained")
				}
				updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if updated.(mainModel).home.profile.name.Value() != want.name {
					t.Fatal("reopened form has stale data")
				}
			}
		})
	}
}

func TestProfileDeleteAccount(t *testing.T) {
	for _, deleteFail := range []bool{false, true} {
		name := "success"
		if deleteFail {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			service := &profileTestServer{requests: make(chan *usersV1.UpdateUserRequest, 1), deletes: make(chan *usersV1.DeleteUserRequest, 1), deleteFail: deleteFail}
			server := grpc.NewServer()
			usersV1.RegisterUserServiceServer(server, service)
			t.Cleanup(server.Stop)
			go server.Serve(listener)
			_, port, _ := net.SplitHostPort(listener.Addr().String())

			m := mainModel{width: 80, height: 24, home: newHomeModel(80, 24), signIn: newSignInModel(80, 24, "client", port)}
			m.signIn.user = userInfo{userID: "user", name: "Alice", username: "alice"}
			m.signIn.session = sessionInfo{sessionID: "session", userID: "user"}
			updated, _ := m.Update(SwitchToHomeMsg{})
			m = updated.(mainModel)
			m.home.list.Select(2)
			updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			m = updated.(mainModel)
			updated, cmd := m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
			m = updated.(mainModel)
			if cmd != nil || !m.home.profile.confirmDelete || !strings.Contains(m.View().Content, "Delete this account") {
				t.Fatal("delete confirmation was not displayed")
			}
			updated, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
			m = updated.(mainModel)
			if cmd == nil || !m.home.profile.deleting {
				t.Fatal("account deletion did not start")
			}
			result := cmd()
			request := <-service.deletes
			if request.UserId != "user" {
				t.Fatalf("incorrect delete request: %v", request)
			}
			updated, cmd = m.Update(result)
			m = updated.(mainModel)
			if deleteFail {
				if m.home.state != homeEditingProfile || m.home.profile.deleting || !strings.Contains(m.View().Content, "Could not delete account") {
					t.Fatal("delete failure did not preserve the profile form")
				}
				return
			}
			if cmd == nil || m.signIn.user != (userInfo{}) || m.signIn.session != (sessionInfo{}) {
				t.Fatal("account deletion did not clear local account state")
			}
			updated, _ = m.Update(cmd())
			m = updated.(mainModel)
			if m.state != signInView {
				t.Fatal("account deletion did not return to sign-in")
			}
		})
	}
}

func TestProfileValidationAndCancel(t *testing.T) {
	m := newHomeModel(80, 24)
	m.user = userInfo{userID: "user", name: "Alice", username: "alice"}
	m.list.Select(2)
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.profile.name.SetValue("   ")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.profile.saving || !strings.Contains(m.View().Content, "Name is required") {
		t.Fatal("blank name accepted")
	}
	m.profile.name.SetValue("Edited")
	m.profile.username.SetValue("   ")
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.profile.saving || !strings.Contains(m.View().Content, "Username is required") {
		t.Fatal("blank username accepted")
	}
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || m.state != homeReady || m.user.name != "Alice" {
		t.Fatal("cancel modified profile")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.profile.name.Value() != "Alice" {
		t.Fatal("cancelled changes persisted")
	}
}
