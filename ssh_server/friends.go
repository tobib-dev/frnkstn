package main

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	usersv1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type friendAddedMsg struct{ username string }
type friendAddFailedMsg struct{ err error }
type pendingFriendRequestsLoadedMsg struct{ items []*friendsv1.GetFriendsItem }
type pendingFriendRequestsFailedMsg struct{ err error }
type friendRequestResolvedMsg struct{ message string }
type friendRequestFailedMsg struct{ err error }

const pendingFriendStatus = "pending"

type friendRequestAction int

const (
	cancelFriendRequest friendRequestAction = iota
	acceptFriendRequest
	declineFriendRequest
)

type addFriendModel struct {
	input                         textinput.Model
	userID, accessToken, grpcPort string
	adding                        bool
	errorText                     string
}

func newAddFriendModel(userID, accessToken, grpcPort string) addFriendModel {
	input := textinput.New()
	input.Prompt = "Friend username: "
	input.SetVirtualCursor(true)
	return addFriendModel{input: input, userID: userID, accessToken: accessToken, grpcPort: grpcPort}
}

func (m addFriendModel) Update(msg tea.Msg) (addFriendModel, tea.Cmd) {
	if failure, ok := msg.(friendAddFailedMsg); ok {
		m.adding = false
		m.errorText = "Could not add friend. Please try again."
		if status.Code(failure.err) == codes.NotFound {
			m.errorText = "No user found with that username."
		}
		return m, m.input.Focus()
	}
	if m.adding {
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		username := strings.TrimSpace(m.input.Value())
		if username == "" {
			m.errorText = "Username is required."
			return m, nil
		}
		m.adding, m.errorText = true, ""
		m.input.Blur()
		return m, func() tea.Msg {
			if err := addFriend(m.accessToken, username, m.grpcPort); err != nil {
				return friendAddFailedMsg{err: err}
			}
			return friendAddedMsg{username: username}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

type friendRequestOption struct {
	label   string
	action  friendRequestAction
	message string
}

func (m addFriendModel) View() string {
	view := "Add friend\n\n" + m.input.View()
	if m.adding {
		return view + "\n\nSending friend request…"
	}
	if m.errorText != "" {
		view += "\n\n" + m.errorText
	}
	return view + "\n\nEnter to send request • Esc to cancel • Ctrl+C to quit"
}

func fetchPendingFriendRequests(accessToken, grpcPort string) tea.Cmd {
	return func() tea.Msg {
		conn, err := newGRPCClient("localhost:" + grpcPort)
		if err != nil {
			return pendingFriendRequestsFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		ctx = bearerContext(ctx, accessToken)
		response, err := friendsv1.NewFriendServiceClient(conn).GetFriends(ctx, &friendsv1.GetFriendsRequest{})
		if err != nil {
			return pendingFriendRequestsFailedMsg{err: err}
		}
		return pendingFriendRequestsLoadedMsg{items: response.Items}
	}
}

func addFriend(accessToken, username, grpcPort string) error {
	conn, err := newGRPCClient("localhost:" + grpcPort)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	ctx = bearerContext(ctx, accessToken)
	user, err := usersv1.NewUserServiceClient(conn).GetUserByUsername(ctx, &usersv1.GetUserByUsernameRequest{Username: username})
	if err != nil {
		return fmt.Errorf("get friend: %w", err)
	}
	if user.UserId == "" {
		return fmt.Errorf("get friend returned no user ID")
	}
	_, err = friendsv1.NewFriendServiceClient(conn).AddFriend(ctx, &friendsv1.AddFriendRequest{FriendId: user.UserId})
	if err != nil {
		return fmt.Errorf("add friend: %w", err)
	}
	return nil
}

func resolveFriendRequest(userID, accessToken, grpcPort string, request *friendsv1.GetFriendsItem, action friendRequestAction) error {
	conn, err := newGRPCClient("localhost:" + grpcPort)
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	ctx = bearerContext(ctx, accessToken)
	client := friendsv1.NewFriendServiceClient(conn)
	friendID := request.FriendId
	if request.UserId != userID {
		friendID = request.UserId
	}
	switch action {
	case cancelFriendRequest:
		_, err = client.RemoveFriend(ctx, &friendsv1.RemoveFriendRequest{FriendshipId: request.FriendshipId, FriendId: friendID})
	case acceptFriendRequest:
		_, err = client.AcceptFriend(ctx, &friendsv1.AcceptFriendRequest{FriendshipId: request.FriendshipId, FriendId: friendID})
	case declineFriendRequest:
		_, err = client.RejectFriend(ctx, &friendsv1.RejectFriendRequest{FriendshipId: request.FriendshipId, FriendId: friendID})
	}
	return err
}
