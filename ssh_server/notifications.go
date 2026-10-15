package main

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
	notificationsv1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type notificationKind int

const (
	messageNotification notificationKind = iota
	friendNotification
)

type notificationItem struct {
	kind    notificationKind
	message *notificationsv1.MsgN10NItem
	friend  *notificationsv1.FriendN10NItem
}

func (n notificationItem) label() string {
	if n.kind == messageNotification {
		return fmt.Sprintf("Message from %s: %s", n.message.GetAuthorName(), n.message.GetContent())
	}
	return fmt.Sprintf("Friend request from %s (%s)", n.friend.GetFriendName(), n.friend.GetStatus())
}

type notificationsLoadedMsg struct{ items []notificationItem }
type notificationsLoadFailedMsg struct{ err error }
type notificationUpdatedMsg struct {
	index       int
	replacement *notificationItem
}
type notificationUpdateFailedMsg struct{ err error }

type notificationsModel struct {
	userID, grpcPort string
	items            []notificationItem
	cursor           int
	loading          bool
	updating         bool
	errorText        string
}

func newNotificationsModel(userID, grpcPort string) notificationsModel {
	return notificationsModel{userID: userID, grpcPort: grpcPort, loading: true}
}

func (m notificationsModel) Update(msg tea.Msg) (notificationsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case notificationsLoadedMsg:
		m.loading = false
		m.items = msg.items
		m.errorText = ""
		return m, nil
	case notificationsLoadFailedMsg:
		m.loading = false
		m.errorText = "Could not load notifications."
		return m, nil
	case notificationUpdatedMsg:
		m.updating = false
		if msg.replacement == nil {
			m.items = append(m.items[:msg.index], m.items[msg.index+1:]...)
			if m.cursor >= len(m.items) && m.cursor > 0 {
				m.cursor--
			}
		} else {
			m.items[msg.index] = *msg.replacement
		}
		m.errorText = ""
		return m, nil
	case notificationUpdateFailedMsg:
		m.updating = false
		m.errorText = "Could not mark notification as seen."
		return m, nil
	}
	if m.loading || m.updating {
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}
		case "enter":
			if len(m.items) == 0 {
				return m, nil
			}
			m.updating, m.errorText = true, ""
			return m, updateNotification(m.userID, m.grpcPort, m.cursor, m.items[m.cursor])
		}
	}
	return m, nil
}

func (m notificationsModel) View() string {
	view := "Notifications\n"
	switch {
	case m.loading:
		view += "\nLoading…"
	case len(m.items) == 0:
		view += "\nNo new notifications."
	default:
		for i, item := range m.items {
			cursor := "  "
			if i == m.cursor {
				cursor = "> "
			}
			view += "\n" + cursor + item.label()
		}
	}
	if m.updating {
		view += "\n\nMarking notification as seen…"
	} else if m.errorText != "" {
		view += "\n\n" + m.errorText
	}
	return view + "\n\n↑/↓ to select • Enter to mark seen • Esc for home • Ctrl+C to quit"
}

func fetchNotifications(userID, grpcPort string) tea.Cmd {
	return func() tea.Msg {
		conn, err := newGRPCClient("localhost:" + grpcPort)
		if err != nil {
			return notificationsLoadFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		client := notificationsv1.NewN10NServiceClient(conn)
		items := make([]notificationItem, 0, 2)
		message, err := client.GetMsgN10N(ctx, &notificationsv1.GetMsgN10NRequest{Item: &notificationsv1.MsgN10NItem{UserId: userID}})
		if err != nil && status.Code(err) != codes.NotFound {
			return notificationsLoadFailedMsg{err: err}
		}
		if err == nil && message.GetItem() != nil {
			items = append(items, notificationItem{kind: messageNotification, message: message.GetItem()})
		}
		friend, err := client.GetFriendN10N(ctx, &notificationsv1.GetFriendN10NRequest{Item: &notificationsv1.FriendN10NItem{UserId: userID}})
		if err != nil && status.Code(err) != codes.NotFound {
			return notificationsLoadFailedMsg{err: err}
		}
		if err == nil && friend.GetItem() != nil {
			items = append(items, notificationItem{kind: friendNotification, friend: friend.GetItem()})
		}
		return notificationsLoadedMsg{items: items}
	}
}

func updateNotification(userID, grpcPort string, index int, item notificationItem) tea.Cmd {
	return func() tea.Msg {
		conn, err := newGRPCClient("localhost:" + grpcPort)
		if err != nil {
			return notificationUpdateFailedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		client := notificationsv1.NewN10NServiceClient(conn)
		var replacement *notificationItem
		if item.kind == messageNotification {
			_, err = client.UpdateMsgN10N(ctx, &notificationsv1.UpdateMsgN10NRequest{Item: &notificationsv1.MsgN10NItem{Id: item.message.GetId(), UserId: userID}})
			if err != nil {
				return notificationUpdateFailedMsg{err: err}
			}
			var response *notificationsv1.GetMsgN10NResponse
			response, err = client.GetMsgN10N(ctx, &notificationsv1.GetMsgN10NRequest{Item: &notificationsv1.MsgN10NItem{UserId: userID}})
			if err == nil && response.GetItem() != nil {
				next := notificationItem{kind: messageNotification, message: response.GetItem()}
				replacement = &next
			}
		} else {
			_, err = client.UpdateFriendN10N(ctx, &notificationsv1.UpdateFriendN10NRequest{Item: &notificationsv1.FriendN10NItem{Id: item.friend.GetId(), UserId: userID}})
			if err != nil {
				return notificationUpdateFailedMsg{err: err}
			}
			var response *notificationsv1.GetFriendN10NResponse
			response, err = client.GetFriendN10N(ctx, &notificationsv1.GetFriendN10NRequest{Item: &notificationsv1.FriendN10NItem{UserId: userID}})
			if err == nil && response.GetItem() != nil {
				next := notificationItem{kind: friendNotification, friend: response.GetItem()}
				replacement = &next
			}
		}
		if err != nil && status.Code(err) != codes.NotFound {
			return notificationUpdateFailedMsg{err: err}
		}
		return notificationUpdatedMsg{index: index, replacement: replacement}
	}
}
