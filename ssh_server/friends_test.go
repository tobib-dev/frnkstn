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
	action  string
	fail    bool
}

func (s *addFriendTestServer) GetFriends(_ context.Context, req *friendsv1.GetFriendsRequest) (*friendsv1.GetFriendsResponse, error) {
	return &friendsv1.GetFriendsResponse{Items: []*friendsv1.GetFriendsItem{
		{UserId: req.UserId, FriendshipId: "pending-friendship", FriendId: "pending-friend", FriendUsername: "bob", Status: "pending"},
		{UserId: req.UserId, FriendshipId: "accepted-friendship", FriendId: "accepted-friend", FriendUsername: "carol", Status: "accepted"},
	}}, nil
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

func (s *addFriendTestServer) RemoveFriend(_ context.Context, req *friendsv1.RemoveFriendRequest) (*friendsv1.RemoveFriendResponse, error) {
	s.action = "cancel:" + req.FriendshipId
	return &friendsv1.RemoveFriendResponse{Status: "removed"}, nil
}

func (s *addFriendTestServer) AcceptFriend(_ context.Context, req *friendsv1.AcceptFriendRequest) (*friendsv1.AcceptFriendResponse, error) {
	s.action = "accept:" + req.FriendshipId
	return &friendsv1.AcceptFriendResponse{Status: "accepted"}, nil
}

func (s *addFriendTestServer) RejectFriend(_ context.Context, req *friendsv1.RejectFriendRequest) (*friendsv1.RejectFriendResponse, error) {
	s.action = "decline:" + req.FriendshipId
	return &friendsv1.RejectFriendResponse{Status: "rejected"}, nil
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

func TestAddFriendDisplaysUnrespondedRequests(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &addFriendTestServer{}
	server := grpc.NewServer()
	friendsv1.RegisterFriendServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())

	model := newAddFriendModel("user", port)
	msg := fetchPendingFriendRequests("user", port)()
	model, _ = model.Update(msg)
	view := model.View()
	if model.loadingRequests || !strings.Contains(view, "Unresponded requests") || !strings.Contains(view, "bob") {
		t.Fatalf("pending request was not displayed: %q", view)
	}
	if strings.Contains(view, "carol") {
		t.Fatalf("accepted friend was displayed as an unresponded request: %q", view)
	}
}

func TestSelectAndResolveUnrespondedRequest(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &addFriendTestServer{}
	server := grpc.NewServer()
	friendsv1.RegisterFriendServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())

	tests := []struct {
		name, requester, wantAction string
		actionCursor                int
		wantOptions                 []string
	}{
		{name: "requester cancels", requester: "user", wantAction: "cancel:friendship", wantOptions: []string{"Cancel", "Home"}},
		{name: "recipient accepts", requester: "other", wantAction: "accept:friendship", wantOptions: []string{"Accept", "Decline", "Home"}},
		{name: "recipient declines", requester: "other", actionCursor: 1, wantAction: "decline:friendship", wantOptions: []string{"Accept", "Decline", "Home"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service.action = ""
			m := newHomeModel(80, 24)
			m.state = homeAddingFriend
			m.addFriend = newAddFriendModel("user", port)
			m.addFriend.loadingRequests = false
			friendID := "user"
			if test.requester == "user" {
				friendID = "other"
			}
			m.addFriend.pendingRequests = []*friendsv1.GetFriendsItem{{UserId: test.requester, FriendshipId: "friendship", FriendId: friendID, FriendUsername: "bob", Status: "pending"}}

			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			view := m.View().Content
			for _, option := range test.wantOptions {
				if !strings.Contains(view, option) {
					t.Fatalf("request view missing %q: %q", option, view)
				}
			}
			for range test.actionCursor {
				m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			}
			m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			if cmd == nil || !m.addFriend.acting {
				t.Fatal("friend request action did not start")
			}
			m, _ = m.Update(cmd())
			if m.state != homeReady || service.action != test.wantAction {
				t.Fatalf("action did not return home: state=%v action=%q", m.state, service.action)
			}
		})
	}

	m := newHomeModel(80, 24)
	m.state = homeAddingFriend
	m.addFriend = newAddFriendModel("user", port)
	m.addFriend.selectedRequest = &friendsv1.GetFriendsItem{UserId: "user"}
	m.addFriend.actionCursor = 1
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m, _ = m.Update(cmd())
	if m.state != homeReady {
		t.Fatal("Home option did not return home")
	}
}
