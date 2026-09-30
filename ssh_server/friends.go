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

type addFriendModel struct {
	input            textinput.Model
	userID, grpcPort string
	adding           bool
	errorText        string
}

func newAddFriendModel(userID, grpcPort string) addFriendModel {
	input := textinput.New()
	input.Prompt = "Friend username: "
	input.SetVirtualCursor(true)
	return addFriendModel{input: input, userID: userID, grpcPort: grpcPort}
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
			if err := addFriend(m.userID, username, m.grpcPort); err != nil {
				return friendAddFailedMsg{err: err}
			}
			return friendAddedMsg{username: username}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
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
