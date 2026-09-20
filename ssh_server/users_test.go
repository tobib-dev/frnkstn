package main

import (
	"context"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
)

type githubTransport func(*http.Request) (*http.Response, error)

func (f githubTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func mockGitHubUser(t *testing.T) {
	t.Helper()
	old := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = old })
	http.DefaultClient = &http.Client{Transport: githubTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != "https://api.github.com/user" || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("incorrect GitHub request")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"id":123,"name":"Alice","login":"alice-gh"}`)), Header: make(http.Header)}, nil
	})}
}

type failingUserServer struct {
	usersV1.UnimplementedUserServiceServer
	err error
}

func (s failingUserServer) GetUserByGHID(context.Context, *usersV1.GetUserByGHIDRequest) (*usersV1.GetUserByGHIDResponse, error) {
	return &usersV1.GetUserByGHIDResponse{}, s.err
}

func TestLookupFailureDoesNotStartAccountOrSessionCreation(t *testing.T) {
	mockGitHubUser(t)
	for _, tt := range []struct {
		name string
		err  error
	}{
		{"unavailable", status.Error(codes.Unavailable, "service unavailable")},
		{"internal", status.Error(codes.Internal, "database failure")},
		{"empty response", nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			server := grpc.NewServer()
			usersV1.RegisterUserServiceServer(server, failingUserServer{err: tt.err})
			t.Cleanup(server.Stop)
			go server.Serve(listener)
			_, port, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			m := newSignInModel(80, 24, "client", port)
			m, cmd := m.Update(signInSuccessMsg{token: tokenInfo{AccessToken: "test-token"}})
			msg := cmd()
			if _, ok := msg.(signInFailureMsg); !ok {
				t.Fatalf("lookup failure was treated as a missing account: %T", msg)
			}
			m, _ = m.Update(msg)
			if m.state != signInFailed || m.session.sessionID != "" {
				t.Fatal("lookup failure opened account setup or created a session")
			}
		})
	}
}
