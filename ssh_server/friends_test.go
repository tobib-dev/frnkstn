package main

import (
	"context"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	usersv1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type addFriendTestServer struct {
	friendsv1.UnimplementedFriendServiceServer
	usersv1.UnimplementedUserServiceServer
	request *friendsv1.AddFriendRequest
	fail    bool
}

func (s *addFriendTestServer) GetUserByUsername(_ context.Context, req *usersv1.GetUserByUsernameRequest) (*usersv1.GetUserByUsernameResponse, error) {
	if s.fail {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	if req.Username != "bob" {
		return nil, status.Error(codes.InvalidArgument, "unexpected username")
	}
	return &usersv1.GetUserByUsernameResponse{UserId: "friend"}, nil
}

func (s *addFriendTestServer) AddFriend(_ context.Context, req *friendsv1.AddFriendRequest) (*friendsv1.AddFriendResponse, error) {
	s.request = req
	return &friendsv1.AddFriendResponse{FriendshipId: "friendship", FriendId: req.FriendId, Status: "pending"}, nil
}

func TestHomeAddFriend(t *testing.T) {
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "missing user"
		}
		t.Run(name, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			service := &addFriendTestServer{fail: fail}
			server := grpc.NewServer()
			friendsv1.RegisterFriendServiceServer(server, service)
			usersv1.RegisterUserServiceServer(server, service)
			t.Cleanup(server.Stop)
			go server.Serve(listener)
			_, port, _ := net.SplitHostPort(listener.Addr().String())

			m := newHomeModel(80, 24)
			m.user = userInfo{userID: "user"}
			m.grpcPort = port
			m.list.Select(2)
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if m.state != homeAddingFriend || !m.addFriend.input.Focused() || !strings.Contains(m.View().Content, "Add friend") {
				t.Fatal("add friend form did not open")
			}
			m.addFriend.input.SetValue(" bob ")
			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil || !m.addFriend.adding {
				t.Fatal("friend request did not start")
			}
			m, _ = m.Update(cmd())
			if fail {
				if m.state != homeAddingFriend || m.addFriend.adding || !strings.Contains(m.View().Content, "No user found") {
					t.Fatal("missing friend did not keep form open with an error")
				}
				return
			}
			if service.request == nil || service.request.UserId != "user" || service.request.FriendId != "friend" {
				t.Fatalf("incorrect friend request: %+v", service.request)
			}
			if m.state != homeReady {
				t.Fatal("successful request did not return home")
			}
		})
	}
}
