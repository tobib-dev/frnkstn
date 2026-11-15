package main

import (
	"context"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	notificationsv1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type inviteNotificationTestServer struct {
	notificationsv1.UnimplementedN10NServiceServer
	updated string
}

func (*inviteNotificationTestServer) Subscribe(
	_ *notificationsv1.SubscribeRequest,
	stream grpc.ServerStreamingServer[notificationsv1.SubscribeResponse],
) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	if values := md.Get("authorization"); len(values) != 1 || values[0] != "Bearer token" {
		return status.Error(codes.Unauthenticated, "token")
	}
	return stream.Send(&notificationsv1.SubscribeResponse{Item: &notificationsv1.SubscribeResponse_Invite{
		Invite: &notificationsv1.InviteN10NItem{Id: "invite", ServerName: "Shared", ShareCode: "code"},
	}})
}

func (s *inviteNotificationTestServer) UpdateInviteN10N(
	ctx context.Context, req *notificationsv1.UpdateInviteN10NRequest,
) (*notificationsv1.UpdateInviteN10NResponse, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if values := md.Get("authorization"); len(values) != 1 || values[0] != "Bearer token" {
		return nil, status.Error(codes.Unauthenticated, "token")
	}
	s.updated = req.GetId()
	return &notificationsv1.UpdateInviteN10NResponse{}, nil
}

func TestInvitationNotificationFlow(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	service := &inviteNotificationTestServer{}
	notificationsv1.RegisterN10NServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	polled, ok := pollNotifications(bearerToken{value: "token"}, port)().(notificationsPolledMsg)
	if !ok || len(polled.items) != 1 || polled.items[0].kind != inviteNotification {
		t.Fatal("invite not polled")
	}
	m := newNotificationsModel("user", bearerToken{value: "token"}, port)
	m.loading = false
	m.items = append([]notificationItem{{kind: messageNotification}}, polled.items...)
	m, _ = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	if m.cursor != 1 || !strings.Contains(m.View(), "Shared") || strings.Contains(m.View(), "Message from") {
		t.Fatal("invites filter did not select the invitation")
	}
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	m, _ = m.Update(enter)
	m.actionCursor = 1
	m, cmd := m.Update(enter)
	msg, ok := cmd().(enterNotificationInviteMsg)
	if !ok || msg.shareCode != "code" {
		t.Fatal("invitation did not open")
	}
	home := newHomeModel(80, 24)
	home.user.userID, home.session.internalAccessToken, home.grpcPort = "user", bearerToken{value: "token"}, port
	home, cmd = home.Update(msg)
	if cmd == nil || home.state != homeViewingServers || home.servers.input.Value() != "code" {
		t.Fatal("invitation did not reach server preview")
	}
	m, _ = m.Update(enter)
	m, cmd = m.Update(enter)
	result := cmd()
	if _, ok := result.(notificationUpdatedMsg); !ok {
		t.Fatalf("update failed: %T", result)
	}
	m, _ = m.Update(result)
	if service.updated != "invite" || len(m.items) != 1 {
		t.Fatal("invite not dismissed")
	}
	if cmd := func() tea.Cmd { _, cmd := m.Update(enter); return cmd }(); cmd != nil {
		t.Fatal("empty filtered list should not select unrelated notification")
	}
}

func TestJoiningClearsInvitationOnlyAfterSuccess(t *testing.T) {
	for _, success := range []bool{false, true} {
		home := newHomeModel(80, 24)
		home.state = homeViewingServers
		home.servers = newServersModel("user", bearerToken{}, "")
		home.servers.page = serverJoinConfirmation
		home.servers.input.SetValue(" code ")
		home.notificationItems = []notificationItem{
			{kind: inviteNotification, invite: &notificationsv1.InviteN10NItem{Id: "accepted", ShareCode: "code"}},
			{kind: inviteNotification, invite: &notificationsv1.InviteN10NItem{Id: "other", ShareCode: "other"}},
		}
		result := serverResultMsg{operation: joinServer}
		if success {
			result.item = &serversv1.ServerItem{ServerId: "server"}
		} else {
			result.err = status.Error(codes.PermissionDenied, "denied")
		}
		home, _ = home.Update(result)
		want := 2
		if success {
			want = 1
		}
		if len(home.notificationItems) != want {
			t.Fatal("incorrect invitation cleanup")
		}
		if success && (home.notificationItems[0].id() != "other" || home.notificationCounter.Load() != 1) {
			t.Fatal("wrong invitation or counter removed")
		}
	}
}

func TestInvitationEscapeLeavesItPending(t *testing.T) {
	m := newNotificationsModel("user", bearerToken{}, "")
	m.loading = false
	m.items = []notificationItem{{kind: inviteNotification,
		invite: &notificationsv1.InviteN10NItem{Id: "invite", ShareCode: "code"},
	}}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if strings.Contains(m.View(), "Ignore") {
		t.Fatal("explicit ignore action remains")
	}
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil || len(m.items) != 1 || m.selected != -1 {
		t.Fatal("escape changed invitation")
	}
	home := newHomeModel(80, 24)
	home.notificationItems = m.items
	home, _ = home.Update(enterNotificationInviteMsg{shareCode: "code"})
	home.servers.busy = false
	home.servers.page = serverJoinConfirmation
	home, _ = home.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if len(home.notificationItems) != 1 || home.servers.page != serverJoin {
		t.Fatal("cancelling join changed invitation")
	}
}
