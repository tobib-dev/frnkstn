package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/gocql/gocql"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type SessionService struct {
	cfg   *Config
	store db.SessionStore
	sessionsV1.UnimplementedSessionServiceServer
}

func NewSessionService(cfg *Config) *SessionService {
	return &SessionService{cfg: cfg, store: &cfg.db.session}
}

func (serv *SessionService) CreateSession(ctx context.Context, sessionInfo *sessionsV1.CreateSessionRequest) (*sessionsV1.CreateSessionResponse, error) {
	userID, err := gocql.ParseUUID(sessionInfo.UserId)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "valid user ID is required")
	}
	if sessionInfo.AccessToken == "" {
		return nil, status.Error(codes.InvalidArgument, "access token is required")
	}
	now := time.Now()
	expiresAt, err := tokenExpiry(now, sessionInfo.AccessTokenExpiresAt)
	if err != nil {
		return nil, err
	}
	refreshExpiresAt, err := tokenExpiry(now, sessionInfo.RefreshTokenExpiresAt)
	if err != nil {
		return nil, err
	}
	sess, err := serv.store.CreateSession(ctx, db.CreateSessionParams{
		UserID: userID, AccessToken: sessionInfo.AccessToken, ExpiresIn: expiresAt,
		RefreshToken: sessionInfo.RefreshToken, RefreshTokenExpiresIn: refreshExpiresAt,
	})
	if err != nil {
		return nil, status.Error(codes.Internal, "could not create session")
	}
	return &sessionsV1.CreateSessionResponse{SessionId: sess.ID.String(), UserId: sess.UserID.String()}, nil
}

func (serv *SessionService) GetSession(ctx context.Context, req *sessionsV1.GetSessionRequest) (*sessionsV1.GetSessionResponse, error) {
	sess, err := serv.store.GetSession(ctx, db.GetSessionParams{
		RefreshToken: req.RefreshToken,
	})
	if err != nil {
		if errors.Is(err, gocql.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "session not found")
		}
		return nil, status.Error(codes.Internal, "could not get session")
	}

	resp := &sessionsV1.GetSessionResponse{
		SessionId:    sess.SessionID.String(),
		UserId:       sess.UserID.String(),
		ExpiresAt:    sess.ExpiresIn.String(),
		RefreshToken: sess.RefreshToken,
	}
	return resp, nil
}

// The current wire contract sends token lifetimes as decimal seconds.
func tokenExpiry(now time.Time, seconds string) (time.Time, error) {
	n, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil || n < 0 || n > int64((1<<63-1)/time.Second) {
		return time.Time{}, status.Error(codes.InvalidArgument, "invalid token lifetime")
	}
	if n == 0 {
		return time.Time{}, nil
	}
	return now.Add(time.Duration(n) * time.Second), nil
}
