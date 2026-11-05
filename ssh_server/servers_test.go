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

type tuiServerTestService struct {
	serversv1.UnimplementedServerServiceServer
}

func (*tuiServerTestService) GetUserServers(context.Context, *serversv1.GetUserServersRequest) (*serversv1.GetUserServersResponse, error) {
	return &serversv1.GetUserServersResponse{Items: []*serversv1.UserServerItem{{ServerId: "server", ServerName: "Original"}}}, nil
}

func (*tuiServerTestService) GetServer(_ context.Context, req *serversv1.GetServerRequest) (*serversv1.GetServerResponse, error) {
	if req.ServerId != "server" {
		return nil, status.Error(codes.NotFound, "missing")
	}
	return &serversv1.GetServerResponse{Item: &serversv1.ServerItem{ServerId: "server", ServerName: "Original", ServerBio: "Bio", Owner: "owner"}}, nil
}

func (*tuiServerTestService) CreateServer(_ context.Context, req *serversv1.CreateServerRequest) (*serversv1.CreateServerResponse, error) {
	return &serversv1.CreateServerResponse{Item: &serversv1.ServerItem{ServerId: "created", ServerName: req.ServerName, ServerBio: req.ServerBio, Owner: "owner"}}, nil
}

func (*tuiServerTestService) EditServer(_ context.Context, req *serversv1.EditServerRequest) (*serversv1.EditServerResponse, error) {
	if req.Item.ServerId != "server" || req.Item.Owner != "owner" {
		return nil, status.Error(codes.InvalidArgument, "identity changed")
	}
	return &serversv1.EditServerResponse{Item: req.Item}, nil
}

func (*tuiServerTestService) DeleteServer(_ context.Context, req *serversv1.DeleteServerRequest) (*serversv1.DeleteServerResponse, error) {
	if req.ServerId != "server" {
		return nil, status.Error(codes.NotFound, "missing")
	}
	return &serversv1.DeleteServerResponse{}, nil
}

func TestHomeServerFlow(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, _ := metadata.FromIncomingContext(ctx)
		if values := md.Get("authorization"); len(values) != 1 || values[0] != "Bearer internal-token" {
			return nil, status.Error(codes.Unauthenticated, "missing bearer token")
		}
		return handler(ctx, req)
	}))
	serversv1.RegisterServerServiceServer(server, &tuiServerTestService{})
	t.Cleanup(server.Stop)
	go server.Serve(listener)
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	m := newHomeModel(80, 24)
	m.user.userID, m.session.internalAccessToken, m.grpcPort = "owner", "internal-token", port
	m.list.Select(1)
	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	esc := tea.KeyPressMsg{Code: tea.KeyEscape}
	m, cmd := m.Update(enter)
	if m.state != homeViewingServers || cmd == nil {
		t.Fatal("servers did not open")
	}
	m, _ = m.Update(cmd())
	if len(m.servers.items) != 1 {
		t.Fatal("servers not loaded")
	}
	m, cmd = m.Update(enter)
	m, _ = m.Update(cmd())
	if m.servers.page != serverDetails || m.servers.item.ServerBio != "Bio" {
		t.Fatal("details not loaded")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'e', Text: "e"})
	m.servers.name.SetValue("Renamed")
	m, cmd = m.Update(enter)
	m, _ = m.Update(cmd())
	if m.servers.item.ServerName != "Renamed" || m.servers.items[0].ServerName != "Renamed" {
		t.Fatal("edit not reflected")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	if m.servers.page != serverDeleteConfirmation {
		t.Fatal("delete requires confirmation")
	}
	m, cmd = m.Update(tea.KeyPressMsg{Code: 'y', Text: "y"})
	m, _ = m.Update(cmd())
	if len(m.servers.items) != 0 || m.servers.page != serversList {
		t.Fatal("deleted server not removed")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'n', Text: "n"})
	m.servers.name.SetValue(" New server ")
	m.servers.bio.SetValue("New bio")
	m, cmd = m.Update(enter)
	m, _ = m.Update(cmd())
	if m.servers.page != serverDetails || m.servers.item.ServerName != "New server" || m.servers.item.ServerBio != "New bio" {
		t.Fatal("server not created")
	}
	m, _ = m.Update(esc)
	m, _ = m.Update(esc)
	if m.state != homeReady {
		t.Fatal("escape did not return home")
	}
}

func TestServerActionsAndErrors(t *testing.T) {
	m := newServersModel("member", "token", "port")
	m.busy, m.page = false, serverDetails
	m.item = &serversv1.ServerItem{Owner: "owner", ServerName: "Server"}
	for _, key := range []rune{'e', 'd'} {
		updated, cmd := m.Update(tea.KeyPressMsg{Code: key, Text: string(key)})
		if updated.page != serverDetails || cmd != nil {
			t.Fatal("non-owner could start mutation")
		}
	}
	if strings.Contains(m.View(), "e edit") {
		t.Fatal("owner actions shown to non-owner")
	}
	m.page = serverCreate
	m, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || m.busy || m.errorText == "" {
		t.Fatal("blank name accepted")
	}
	m.busy = true
	m, _ = m.Update(serverResultMsg{operation: createServer, err: status.Error(codes.Unavailable, "offline")})
	if m.busy || m.page != serverCreate || m.errorText == "" {
		t.Fatal("failure did not allow retry")
	}
}
