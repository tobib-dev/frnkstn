package main

import (
	"context"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	notificationsV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type renewalTestServer struct {
	sessionsV1.UnimplementedSessionServiceServer
	notificationsV1.UnimplementedN10NServiceServer
	refreshes atomic.Int64
	failure   atomic.Int32
}

func (s *renewalTestServer) RefreshSession(
	ctx context.Context, req *sessionsV1.RefreshSessionRequest,
) (*sessionsV1.RefreshSessionResponse, error) {
	s.refreshes.Add(1)
	md, _ := metadata.FromIncomingContext(ctx)
	if len(md.Get("authorization")) != 0 || req.SessionId != "session" || req.InternalRefreshToken != "refresh" {
		return nil, status.Error(codes.InvalidArgument, "wrong refresh credentials")
	}
	if failure := codes.Code(s.failure.Load()); failure != codes.OK {
		return nil, status.Error(failure, "refresh failed")
	}
	return &sessionsV1.RefreshSessionResponse{
		InternalAccessToken:          "new-token",
		InternalAccessTokenExpiresAt: time.Now().Add(15 * time.Minute).Format(time.RFC3339Nano),
	}, nil
}

func validRenewedHeader(ctx context.Context) error {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get("authorization")
	if len(values) != 1 || values[0] != "Bearer new-token" {
		return status.Error(codes.Unauthenticated, "stale or duplicate token")
	}
	return nil
}

func (*renewalTestServer) GetSession(
	ctx context.Context, _ *sessionsV1.GetSessionRequest,
) (*sessionsV1.GetSessionResponse, error) {
	return &sessionsV1.GetSessionResponse{}, validRenewedHeader(ctx)
}

func (*renewalTestServer) Subscribe(
	_ *notificationsV1.SubscribeRequest, stream grpc.ServerStreamingServer[notificationsV1.SubscribeResponse],
) error {
	return validRenewedHeader(stream.Context())
}

func TestSharedTokenRenewal(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	service := &renewalTestServer{}
	server := grpc.NewServer()
	sessionsV1.RegisterSessionServiceServer(server, service)
	notificationsV1.RegisterN10NServiceServer(server, service)
	go server.Serve(listener)
	t.Cleanup(server.Stop)
	conn, err := newGRPCClient(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	credentials := &tokenRenewal{
		access: "old-token", refresh: "refresh", sessionID: "session", expiresAt: time.Now().Add(-time.Hour),
	}
	token := bearerToken{value: "old-token", renewal: credentials}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, err := sessionsV1.NewSessionServiceClient(conn).GetSession(bearerContext(ctx, token),
				&sessionsV1.GetSessionRequest{})
			if err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if service.refreshes.Load() != 1 {
		t.Fatal("concurrent requests did not share renewal")
	}
	// A new stream must also renew using the same credentials.
	credentials.expiresAt = time.Now().Add(30 * time.Second)
	stream, err := notificationsV1.NewN10NServiceClient(conn).Subscribe(bearerContext(ctx, token),
		&notificationsV1.SubscribeRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Recv(); err != io.EOF {
		t.Fatalf("stream authentication: %v", err)
	}
	if service.refreshes.Load() != 2 {
		t.Fatal("stream did not renew proactively")
	}

	credentials.expiresAt = time.Now().Add(-time.Second)
	service.failure.Store(int32(codes.Unavailable))
	_, err = sessionsV1.NewSessionServiceClient(conn).GetSession(bearerContext(ctx, token),
		&sessionsV1.GetSessionRequest{})
	if status.Code(err) != codes.Unavailable || credentials.closed {
		t.Fatal("transient failure must not sign out")
	}
	service.failure.Store(int32(codes.OK))
	_, err = sessionsV1.NewSessionServiceClient(conn).GetSession(bearerContext(ctx, token),
		&sessionsV1.GetSessionRequest{})
	if err != nil {
		t.Fatal("renewal did not recover", err)
	}

	credentials.expiresAt = time.Now().Add(-time.Second)
	service.failure.Store(int32(codes.Unauthenticated))
	_, err = sessionsV1.NewSessionServiceClient(conn).GetSession(bearerContext(ctx, token),
		&sessionsV1.GetSessionRequest{})
	if status.Code(err) != codes.Unauthenticated || !credentials.closed {
		t.Fatal("revoked refresh token must end session")
	}
	model := mainModel{
		state: homeView,
		home: newHomeModel(
			80,
			24,
		),
		signIn: newSignInModel(
			80,
			24,
			"client",
			"port",
		),
	}
	model.home.session.internalAccessToken = token
	updated, _ := model.Update(sessionCheckMsg{renewal: credentials})
	signedOut := updated.(mainModel)
	if signedOut.state != signInView || signedOut.home.session != (sessionInfo{}) {
		t.Fatal("expired credentials must return to sign-in")
	}
	// A late check from an old session must not sign out a newer session.
	other := &tokenRenewal{}
	model.home.session.internalAccessToken.renewal = other
	updated, cmd := model.Update(sessionCheckMsg{renewal: credentials})
	if updated.(mainModel).state != homeView || cmd != nil {
		t.Fatal("stale session event was applied")
	}
}
