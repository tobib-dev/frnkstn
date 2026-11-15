package main

import (
	"context"
	"net"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type modUITestService struct {
	memberUITestService
	promoted bool
	denied   bool
}

func (s *modUITestService) AddMod(
	_ context.Context, req *serversv1.AddModRequest,
) (*serversv1.AddModResponse, error) {
	if s.denied {
		return nil, status.Error(codes.PermissionDenied, "denied")
	}
	if req.ServerId != "server" || req.ModUserId != "member" {
		return nil, status.Error(codes.InvalidArgument, "wrong promotion target")
	}
	s.promoted = true
	return &serversv1.AddModResponse{Item: &serversv1.ModItem{
		ServerId: "server", ModId: "member", ModName: "Member",
	}}, nil
}

func (s *modUITestService) GetMods(
	_ context.Context, req *serversv1.GetModsRequest,
) (*serversv1.GetModsResponse, error) {
	if req.ServerId != "server" {
		return nil, status.Error(codes.NotFound, "server")
	}
	mods := []*serversv1.ModItem{{ServerId: "server", ModId: "owner", ModName: "Owner"}}
	if s.promoted {
		mods = append(mods, &serversv1.ModItem{
			ServerId: "server", ModId: "member", ModName: "Member",
		})
	}
	return &serversv1.GetModsResponse{Items: mods}, nil
}

func (s *modUITestService) GetMod(
	ctx context.Context, req *serversv1.GetModRequest,
) (*serversv1.GetModResponse, error) {
	mods, err := s.GetMods(ctx, &serversv1.GetModsRequest{ServerId: req.ServerId})
	if err != nil {
		return nil, err
	}
	for _, mod := range mods.Items {
		if mod.ModId == req.ModId {
			return &serversv1.GetModResponse{Item: mod}, nil
		}
	}
	return nil, status.Error(codes.NotFound, "moderator")
}

func (s *modUITestService) DeleteMod(
	_ context.Context, req *serversv1.DeleteModRequest,
) (*serversv1.DeleteModResponse, error) {
	if req.ServerId != "server" || req.ModId != "member" || req.ModUserId != "member" {
		return nil, status.Error(codes.InvalidArgument, "wrong demotion target")
	}
	s.promoted = false
	return &serversv1.DeleteModResponse{}, nil
}

func TestServerModeratorFlow(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &modUITestService{}
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
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	m := newServersModel("owner", bearerToken{value: "token"}, port)
	m.busy, m.page = false, serverDetails
	m.item = &serversv1.ServerItem{ServerId: "server", ServerName: "Shared", Owner: "owner"}
	press := func(code rune) tea.Cmd {
		t.Helper()
		text := string(code)
		if code == tea.KeyEnter || code == tea.KeyEscape || code == tea.KeyDown {
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
	run(press('o'))
	if m.page != serverMods || !strings.Contains(m.View(), "Owner (owner)") {
		t.Fatal("moderator list missing")
	}
	run(press(tea.KeyEnter))
	press('d')
	if m.page != serverModDetails {
		t.Fatal("owner demotion offered")
	}
	press(tea.KeyEscape)
	press(tea.KeyEscape)
	run(press('m'))
	press('p')
	if m.page != serverPromoteConfirmation || service.promoted {
		t.Fatal("promotion needs confirmation")
	}
	press('n')
	if m.page != serverMembers {
		t.Fatal("promotion cancel failed")
	}
	press('p')
	service.denied = true
	cmd := press('y')
	m, _ = m.Update(cmd())
	if m.busy || m.page != serverPromoteConfirmation || m.errorText == "" || service.promoted {
		t.Fatal("denied promotion did not preserve retry/cancel state")
	}
	service.denied = false
	run(press('y'))
	if !service.promoted || m.page != serverMods || len(m.mods) != 2 {
		t.Fatal("promotion did not refresh moderators")
	}
	press(tea.KeyDown)
	run(press(tea.KeyEnter))
	if m.mod.GetModId() != "member" || !strings.Contains(m.View(), "Member") {
		t.Fatal("wrong moderator details")
	}
	m.userID = "member"
	press('d')
	if m.page != serverModDetails {
		t.Fatal("non-owner offered demotion")
	}
	m.userID = "owner"
	press('d')
	if m.page != serverDemoteConfirmation || !service.promoted {
		t.Fatal("demotion needs confirmation")
	}
	press(tea.KeyEscape)
	if m.page != serverModDetails {
		t.Fatal("demotion cancel failed")
	}
	press('d')
	run(press('y'))
	if service.promoted || len(m.mods) != 1 || m.modSelected != 0 {
		t.Fatal("demotion failed")
	}
	press(tea.KeyEscape)
	run(press('m'))
	if len(m.members) != 1 {
		t.Fatal("demotion removed membership")
	}
}

func TestEmptyModeratorAndMemberLists(t *testing.T) {
	m := newServersModel("owner", bearerToken{}, "")
	m.busy, m.page = false, serverMods
	m.item = &serversv1.ServerItem{Owner: "owner"}
	if !strings.Contains(m.View(), "No moderators") {
		t.Fatal("missing empty state")
	}
	for _, key := range []tea.KeyPressMsg{{Code: tea.KeyEnter}, {Code: 'd', Text: "d"}} {
		var cmd tea.Cmd
		m, cmd = m.Update(key)
		if cmd != nil || m.page != serverMods {
			t.Fatal("empty list allowed moderator action")
		}
	}
	m.page = serverMembers
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'p', Text: "p"})
	if cmd != nil || m.page != serverMembers {
		t.Fatal("empty list allowed promotion")
	}
}
