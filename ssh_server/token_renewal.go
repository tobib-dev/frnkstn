package main

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Copies held by screens share renewal state for this SSH session only.
type bearerToken struct {
	value   string
	renewal *tokenRenewal
}
type bearerTokenKey struct{}

type tokenRenewal struct {
	mu                         sync.Mutex
	access, refresh, sessionID string
	expiresAt                  time.Time
	closed                     bool
}

const tokenRenewalMargin = time.Minute

func (r *tokenRenewal) accessToken(ctx context.Context, conn *grpc.ClientConn) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return "", status.Error(codes.Unauthenticated, "session expired")
	}
	if time.Until(r.expiresAt) > tokenRenewalMargin {
		return r.access, nil
	}
	// Holding the lock coalesces simultaneous polls and requests into one renewal.
	response, err := sessionsV1.NewSessionServiceClient(conn).RefreshSession(ctx,
		&sessionsV1.RefreshSessionRequest{SessionId: r.sessionID, InternalRefreshToken: r.refresh})
	if err != nil {
		if status.Code(err) == codes.Unauthenticated {
			r.closed = true
		}
		return "", err
	}
	expires, err := time.Parse(time.RFC3339Nano, response.GetInternalAccessTokenExpiresAt())
	if err != nil || response.GetInternalAccessToken() == "" || !expires.After(time.Now()) {
		return "", status.Error(codes.Internal, "invalid token renewal response")
	}
	r.access, r.expiresAt = response.InternalAccessToken, expires
	return r.access, nil
}

func (r *tokenRenewal) stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
}

func renewedContext(ctx context.Context, conn *grpc.ClientConn) (context.Context, error) {
	token, ok := ctx.Value(bearerTokenKey{}).(bearerToken)
	if !ok || token.renewal == nil {
		return ctx, nil
	}
	// The refresh call must bypass this interceptor's token resolution.
	refreshCtx := context.WithValue(ctx, bearerTokenKey{}, bearerToken{})
	refreshCtx = metadata.NewOutgoingContext(refreshCtx, metadata.MD{})
	refreshCtx, cancel := context.WithTimeout(refreshCtx, authRequestTimeout)
	defer cancel()
	access, err := token.renewal.accessToken(refreshCtx, conn)
	if err != nil {
		return nil, err
	}
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set("authorization", "Bearer "+access)
	return metadata.NewOutgoingContext(ctx, md), nil
}

func renewUnaryToken(
	ctx context.Context, method string, req, reply any, conn *grpc.ClientConn,
	invoke grpc.UnaryInvoker, opts ...grpc.CallOption,
) error {
	ctx, err := renewedContext(ctx, conn)
	if err != nil {
		return err
	}
	return invoke(ctx, method, req, reply, conn, opts...)
}

func renewStreamToken(
	ctx context.Context, desc *grpc.StreamDesc, conn *grpc.ClientConn,
	method string, streamer grpc.Streamer, opts ...grpc.CallOption,
) (grpc.ClientStream, error) {
	ctx, err := renewedContext(ctx, conn)
	if err != nil {
		return nil, err
	}
	return streamer(ctx, desc, conn, method, opts...)
}

type sessionCheckMsg struct{ renewal *tokenRenewal }

func checkSession(r *tokenRenewal) tea.Cmd {
	if r == nil {
		return nil
	}
	return tea.Tick(5*time.Second, func(time.Time) tea.Msg { return sessionCheckMsg{renewal: r} })
}
