package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	"github.com/tobib-dev/frnkstn/api/internal/auth"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *SessionService) RefreshSession(
	ctx context.Context, req *sessionsV1.RefreshSessionRequest,
) (*sessionsV1.RefreshSessionResponse, error) {
	sessionID, err := parseID(req.GetSessionId(), "session ID")
	if err != nil {
		return nil, err
	}
	value := req.GetInternalRefreshToken()
	if strings.TrimSpace(value) == "" {
		return nil, status.Error(codes.Unauthenticated, "refresh token is required")
	}
	token, err := s.tokens.GetInternalRefreshToken(ctx, value)
	if errors.Is(err, gocql.ErrNotFound) {
		return nil, status.Error(codes.Unauthenticated, "invalid refresh token")
	}
	if err != nil {
		// Do not log database errors here: query errors may contain the token binding.
		s.cfg.logger.Error("failed to load refresh token")
		return nil, status.Error(codes.Internal, "could not renew session")
	}
	if token.UserID == (gocql.UUID{}) || !token.RevokedAt.IsZero() ||
		!token.Expiration.After(time.Now()) {
		return nil, status.Error(codes.Unauthenticated, "refresh token expired or revoked")
	}
	session, err := s.store.GetSession(ctx, db.GetSessionParams{ID: sessionID})
	if errors.Is(err, gocql.ErrNotFound) {
		return nil, status.Error(codes.Unauthenticated, "session no longer exists")
	}
	if err != nil {
		s.cfg.logger.Error("failed to load session for renewal", "error", err)
		return nil, status.Error(codes.Internal, "could not renew session")
	}
	if session.UserID != token.UserID || !session.SignedOutAt.IsZero() {
		return nil, status.Error(codes.Unauthenticated, "session is not active")
	}
	if _, err := s.users.GetUserByID(ctx, token.UserID); err != nil {
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, status.Error(codes.Unauthenticated, "user no longer exists")
		}
		s.cfg.logger.Error("failed to load user for renewal", "error", err)
		return nil, status.Error(codes.Internal, "could not renew session")
	}
	now := time.Now()
	access, err := auth.MakeJWT(token.UserID.String(), auth.DefaultUserPermissions(),
		s.cfg.signingKey, internalAccessTokenLifetime)
	if err != nil {
		s.cfg.logger.Error("failed to issue renewed access token", "error", err)
		return nil, status.Error(codes.Internal, "could not renew session")
	}
	return &sessionsV1.RefreshSessionResponse{
		InternalAccessToken:          access,
		InternalAccessTokenExpiresAt: now.Add(internalAccessTokenLifetime).Format(time.RFC3339Nano),
	}, nil
}
