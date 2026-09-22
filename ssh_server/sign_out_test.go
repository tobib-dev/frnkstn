package main

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type signOutTestServer struct {
	sessionsV1.UnimplementedSessionServiceServer
	requests chan *sessionsV1.UpdateSessionRequest
	fail     bool
}

func (s *signOutTestServer) UpdateSession(_ context.Context, req *sessionsV1.UpdateSessionRequest) (*sessionsV1.UpdateSessionResponse, error) {
	s.requests <- req
	if s.fail {
		return nil, status.Error(codes.Unavailable, "unavailable")
	}
	return &sessionsV1.UpdateSessionResponse{SessionId: req.SessionId, UserId: req.UserId, SignedOutAt: time.Now().Format(time.RFC3339Nano)}, nil
}

func TestHomeSignOut(t *testing.T) {
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
			service := &signOutTestServer{requests: make(chan *sessionsV1.UpdateSessionRequest, 1), fail: fail}
			server := grpc.NewServer()
			sessionsV1.RegisterSessionServiceServer(server, service)
			t.Cleanup(server.Stop)
			go server.Serve(listener)
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			m := mainModel{width: 80, height: 24, signIn: newSignInModel(80, 24, "client", port), home: newHomeModel(80, 24)}
			m.signIn.session = sessionInfo{sessionID: "session", userID: "user"}
			m.signIn.token = tokenInfo{AccessToken: "access", RefreshToken: "refresh"}
			m.signIn.user = userInfo{userID: "user"}
			updated, _ := m.Update(SwitchToHomeMsg{})
			m = updated.(mainModel)
			m.home.list.Select(3)
			updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			pending := updated.(mainModel)
			if cmd == nil || pending.home.state != homeSigningOut || !strings.Contains(pending.View().Content, "Signing out") {
				t.Fatal("sign-out did not start")
			}
			_, duplicate := pending.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if duplicate != nil {
				t.Fatal("duplicate sign-out request")
			}
			result := cmd()
			req := <-service.requests
			if req.SessionId != "session" || req.UserId != "user" {
				t.Fatalf("incorrect IDs: %v", req)
			}
			updated, _ = pending.Update(result)
			done := updated.(mainModel)
			if fail {
				if done.state != homeView || done.home.state != homeReady || done.signIn.token.AccessToken != "access" || !strings.Contains(done.View().Content, "Sign out failed") {
					t.Fatal("failed sign-out did not retain login and show error")
				}
				_, retry := done.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if retry == nil {
					t.Fatal("sign-out cannot be retried")
				}
			} else {
				if done.state != signInView || done.signIn.token != (tokenInfo{}) || done.signIn.session != (sessionInfo{}) || done.signIn.user != (userInfo{}) || done.home.session != (sessionInfo{}) {
					t.Fatal("successful sign-out did not clear credentials and return to sign-in")
				}
				if done.signIn.grpcPort != port || done.signIn.clientID != "client" {
					t.Fatal("lost sign-in configuration")
				}
			}
		})
	}
}
