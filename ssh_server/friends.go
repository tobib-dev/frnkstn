package main

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	usersv1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
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
	input            textinput.Model
	userID, grpcPort string
	adding           bool
	loadingRequests  bool
	pendingRequests  []*friendsv1.GetFriendsItem
	requestCursor    int
	selectedRequest  *friendsv1.GetFriendsItem
	actionCursor     int
	acting           bool
	requestsError    string
	errorText        string
}

func newAddFriendModel(userID, grpcPort string) addFriendModel {
	input := textinput.New()
	input.Prompt = "Friend username: "
	input.SetVirtualCursor(true)
	return addFriendModel{input: input, userID: userID, grpcPort: grpcPort, loadingRequests: true, requestCursor: -1}
}

func (m addFriendModel) Update(msg tea.Msg) (addFriendModel, tea.Cmd) {
	switch msg := msg.(type) {
	case pendingFriendRequestsLoadedMsg:
		m.loadingRequests = false
		m.pendingRequests = nil
		for _, item := range msg.items {
			if item.GetStatus() == pendingFriendStatus {
				m.pendingRequests = append(m.pendingRequests, item)
			}
		}
		m.requestsError = ""
		return m, nil
	case pendingFriendRequestsFailedMsg:
		m.loadingRequests = false
		m.requestsError = "Could not load friend requests."
		return m, nil
	case friendRequestFailedMsg:
		m.acting = false
		m.errorText = "Could not update friend request. Please try again."
		return m, nil
	}
	if m.selectedRequest != nil {
		return m.updateSelectedRequest(msg)
	}
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
	if m.requestCursor >= 0 {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "up":
				if m.requestCursor == 0 {
					m.requestCursor = -1
					return m, m.input.Focus()
				}
				m.requestCursor--
			case "down":
				if m.requestCursor < len(m.pendingRequests)-1 {
					m.requestCursor++
				}
			case "tab":
				m.requestCursor = -1
				return m, m.input.Focus()
			case "enter":
				m.selectedRequest = m.pendingRequests[m.requestCursor]
				m.actionCursor, m.errorText = 0, ""
			}
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "tab", "down":
			if len(m.pendingRequests) > 0 && (key.String() == "tab" || strings.TrimSpace(m.input.Value()) == "") {
				m.requestCursor = 0
				m.input.Blur()
				return m, nil
			}
		case "enter":
			username := strings.TrimSpace(m.input.Value())
			if username == "" {
				m.errorText = "Username is required."
				return m, nil
			}
			m.adding, m.errorText = true, ""
			m.input.Blur()
			return m, func() tea.Msg {
				if err := addFriend(m.userID, username, m.grpcPort); err != nil {
					return friendAddFailedMsg{err: err}
				}
				return friendAddedMsg{username: username}
			}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m addFriendModel) updateSelectedRequest(msg tea.Msg) (addFriendModel, tea.Cmd) {
	if m.acting {
		return m, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	actions := m.requestActions()
	switch key.String() {
	case "up":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "down":
		if m.actionCursor < len(actions)-1 {
			m.actionCursor++
		}
	case "esc":
		m.selectedRequest = nil
	case "enter":
		action := actions[m.actionCursor]
		if action.label == "Home" {
			return m, func() tea.Msg { return friendRequestResolvedMsg{} }
		}
		m.acting, m.errorText = true, ""
		request := m.selectedRequest
		return m, func() tea.Msg {
			if err := resolveFriendRequest(m.userID, m.grpcPort, request, action.action); err != nil {
				return friendRequestFailedMsg{err: err}
			}
			return friendRequestResolvedMsg{message: action.message}
		}
	}
	return m, nil
}

type friendRequestOption struct {
	label   string
	action  friendRequestAction
	message string
}

func (m addFriendModel) requestActions() []friendRequestOption {
	if m.selectedRequest.GetUserId() == m.userID {
		return []friendRequestOption{{label: "Cancel", action: cancelFriendRequest, message: "Friend request canceled"}, {label: "Home"}}
	}
	return []friendRequestOption{
		{label: "Accept", action: acceptFriendRequest, message: "Friend request accepted"},
		{label: "Decline", action: declineFriendRequest, message: "Friend request declined"},
		{label: "Home"},
	}
}

func (m addFriendModel) View() string {
	if m.selectedRequest != nil {
		username := m.selectedRequest.GetFriendUsername()
		if username == "" {
			username = m.selectedRequest.GetFriendId()
		}
		view := "Friend request: " + username + "\n\n"
		for i, action := range m.requestActions() {
			cursor := "  "
			if i == m.actionCursor {
				cursor = "> "
			}
			view += cursor + action.label + "\n"
		}
		if m.acting {
			view += "\nUpdating friend request…"
		} else if m.errorText != "" {
			view += "\n" + m.errorText
		}
		return view + "\n\n↑/↓ to select • Enter to confirm • Esc to go back • Ctrl+C to quit"
	}
	view := "Add friend\n\n" + m.input.View()
	view += "\n\nUnresponded requests\n"
	switch {
	case m.loadingRequests:
		view += "Loading…"
	case m.requestsError != "":
		view += m.requestsError
	case len(m.pendingRequests) == 0:
		view += "No unresponded requests."
	default:
		for i, request := range m.pendingRequests {
			username := request.FriendUsername
			if username == "" {
				username = request.FriendId
			}
			cursor := "  "
			if i == m.requestCursor {
				cursor = "> "
			}
			view += "\n" + cursor + username
		}
	}
	if m.adding {
		return view + "\n\nSending friend request…"
	}
	if m.errorText != "" {
		view += "\n\n" + m.errorText
	}
	return view + "\n\nTab or ↓ to select requests • Enter to continue • Esc to cancel • Ctrl+C to quit"
}

func fetchPendingFriendRequests(userID, grpcPort string) tea.Cmd {
	return func() tea.Msg {
		conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			return pendingFriendRequestsFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		response, err := friendsv1.NewFriendServiceClient(conn).GetFriends(ctx, &friendsv1.GetFriendsRequest{UserId: userID})
		if err != nil {
			return pendingFriendRequestsFailedMsg{err: err}
		}
		return pendingFriendRequestsLoadedMsg{items: response.Items}
	}
}

func addFriend(userID, username, grpcPort string) error {
	conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	user, err := usersv1.NewUserServiceClient(conn).GetUserByUsername(ctx, &usersv1.GetUserByUsernameRequest{Username: username})
	if err != nil {
		return fmt.Errorf("get friend: %w", err)
	}
	if user.UserId == "" {
		return fmt.Errorf("get friend returned no user ID")
	}
	_, err = friendsv1.NewFriendServiceClient(conn).AddFriend(ctx, &friendsv1.AddFriendRequest{UserId: userID, FriendId: user.UserId})
	if err != nil {
		return fmt.Errorf("add friend: %w", err)
	}
	return nil
}

func resolveFriendRequest(userID, grpcPort string, request *friendsv1.GetFriendsItem, action friendRequestAction) error {
	conn, err := grpc.NewClient("localhost:"+grpcPort, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
	defer cancel()
	client := friendsv1.NewFriendServiceClient(conn)
	friendID := request.FriendId
	if request.UserId != userID {
		friendID = request.UserId
	}
	switch action {
	case cancelFriendRequest:
		_, err = client.RemoveFriend(ctx, &friendsv1.RemoveFriendRequest{FriendshipId: request.FriendshipId, UserId: userID, FriendId: friendID})
	case acceptFriendRequest:
		_, err = client.AcceptFriend(ctx, &friendsv1.AcceptFriendRequest{FriendshipId: request.FriendshipId, UserId: userID, FriendId: friendID})
	case declineFriendRequest:
		_, err = client.RejectFriend(ctx, &friendsv1.RejectFriendRequest{FriendshipId: request.FriendshipId, UserId: userID, FriendId: friendID})
	}
	return err
}
