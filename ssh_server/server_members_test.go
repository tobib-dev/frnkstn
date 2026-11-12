package main

import (
	"context"
	"net"
	"testing"

	tea "charm.land/bubbletea/v2"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type memberUITestService struct {
	serversv1.UnimplementedServerServiceServer
	friendsv1.UnimplementedFriendServiceServer
	invited, removed, left string
}

func (*memberUITestService) GetServerByCode(
	_ context.Context, req *serversv1.GetServerByCodeRequest,
) (*serversv1.GetServerByCodeResponse, error) {
	if req.ShareCode != "invite-code" {
		return nil, status.Error(codes.NotFound, "code")
	}
	return &serversv1.GetServerByCodeResponse{Item: &serversv1.ServerItem{
		ServerId: "server", ServerName: "Shared", Owner: "owner",
	}}, nil
}

func (s *memberUITestService) JoinServer(
	ctx context.Context, req *serversv1.JoinServerRequest,
) (*serversv1.JoinServerResponse, error) {
	response, err := s.GetServerByCode(ctx, &serversv1.GetServerByCodeRequest{
		ShareCode: req.ShareCode,
	})
	return &serversv1.JoinServerResponse{Item: response.GetItem()}, err
}

func (*memberUITestService) GetFriends(
	context.Context, *friendsv1.GetFriendsRequest,
) (*friendsv1.GetFriendsResponse, error) {
	return &friendsv1.GetFriendsResponse{Items: []*friendsv1.GetFriendsItem{{
		UserId: "mod", FriendId: "invitee", FriendUsername: "Invitee", Status: "accepted",
	}}}, nil
}

func (s *memberUITestService) InviteMember(
	_ context.Context, req *serversv1.InviteMemberRequest,
) (*serversv1.InviteMemberResponse, error) {
	if req.ServerId != "server" {
		return nil, status.Error(codes.NotFound, "server")
	}
	s.invited = req.MemberId
	return &serversv1.InviteMemberResponse{}, nil
}

func (s *memberUITestService) GetMembers(
	_ context.Context, req *serversv1.GetMembersRequest,
) (*serversv1.GetMembersResponse, error) {
	if req.ServerId != "server" {
		return nil, status.Error(codes.NotFound, "server")
	}
	response := &serversv1.GetMembersResponse{}
	if s.removed == "" {
		response.Items = []*serversv1.MemberItem{{MemberId: "member", MemberName: "Member"}}
	}
	return response, nil
}

func (s *memberUITestService) DeleteMember(
	_ context.Context, req *serversv1.DeleteMemberRequest,
) (*serversv1.DeleteMemberResponse, error) {
	if req.ServerId != "server" {
		return nil, status.Error(codes.NotFound, "server")
	}
	s.removed = req.MemberId
	return &serversv1.DeleteMemberResponse{}, nil
}

func (s *memberUITestService) LeaveServer(
	_ context.Context, req *serversv1.LeaveServerRequest,
) (*serversv1.LeaveServerResponse, error) {
	s.left = req.ServerId
	return &serversv1.LeaveServerResponse{}, nil
}

func TestServerMembershipFlow(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &memberUITestService{}
	server := grpc.NewServer(grpc.UnaryInterceptor(func(
		ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
	) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		tokens := md.Get("authorization")
		if len(tokens) != 1 || tokens[0] != "Bearer token" {
			return nil, status.Error(codes.Unauthenticated, "token")
		}
		return handler(ctx, req)
	}))
	serversv1.RegisterServerServiceServer(server, service)
	friendsv1.RegisterFriendServiceServer(server, service)
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	m := newServersModel("mod", bearerToken{value: "token"}, port)
	m.busy = false
	press := func(code rune) tea.Cmd {
		t.Helper()
		text := string(code)
		if code == tea.KeyEnter {
			text = ""
		}
		var cmd tea.Cmd
		m, cmd = m.Update(tea.KeyPressMsg{Code: code, Text: text})
		return cmd
	}
	run := func(cmd tea.Cmd) {
		t.Helper()
		if cmd == nil {
			t.Fatal("expected request")
		}
		for cmd != nil {
			m, cmd = m.Update(cmd())
		}
		if m.errorText != "" {
			t.Fatal(m.errorText)
		}
	}
	press('c')
	if cmd := press(tea.KeyEnter); cmd != nil || m.errorText == "" {
		t.Fatal("blank code accepted")
	}
	m.input.SetValue(" invite-code ")
	run(press(tea.KeyEnter))
	if m.page != serverJoinConfirmation || len(m.items) != 0 {
		t.Fatal("preview must not add membership")
	}
	run(press('y'))
	if m.page != serverDetails || len(m.items) != 1 {
		t.Fatal("join failed")
	}
	run(press('i'))
	run(press(tea.KeyEnter))
	if service.invited != "invitee" || m.page != serverDetails {
		t.Fatal("invite failed")
	}
	run(press('m'))
	if m.page != serverMembers || len(m.members) != 1 {
		t.Fatal("members missing")
	}
	if cmd := press('d'); cmd != nil || service.removed != "" {
		t.Fatal("removal did not require confirmation")
	}
	press('n')
	if m.page != serverMembers {
		t.Fatal("cancel failed")
	}
	press('d')
	run(press('y'))
	if service.removed != "member" || len(m.members) != 0 {
		t.Fatal("removal failed")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	press('l')
	if m.page != serverLeaveConfirmation || service.left != "" {
		t.Fatal("leave did not require confirmation")
	}
	run(press('y'))
	if service.left != "server" || m.page != serversList || len(m.items) != 0 {
		t.Fatal("leave failed")
	}
}

func TestMemberRequestFailureAllowsRetry(t *testing.T) {
	m := newServersModel("member", bearerToken{value: "token"}, "port")
	m.page, m.busy = serverRemoveConfirmation, true
	m, cmd := m.Update(serverResultMsg{
		operation: removeMember, err: status.Error(codes.PermissionDenied, "denied"),
	})
	if cmd != nil || m.busy || m.page != serverRemoveConfirmation || m.errorText == "" {
		t.Fatal("failed removal must preserve the page and allow cancellation")
	}
}
