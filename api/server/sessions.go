package main

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	sessionsV1 "github.com/tobib-dev/frnkstn/api/proto/sessions/v1"
)

type SessionConfig struct {
	sessionsV1.UnimplementedSessionServiceServer
	sessions []*sessionsV1.GetSessionResponse
}

func (cfg *SessionConfig) CreateSession(ctx context.Context, sessionInfo *sessionsV1.CreateSessionRequest) (*sessionsV1.CreateSessionResponse, error) {
	sessions := cfg.sessions
	for _, ses := range sessions {
		if sessionInfo.RefreshToken == ses.RefreshToken {
			return &sessionsV1.CreateSessionResponse{}, fmt.Errorf("Session already exist!!!\n")
		}
	}

	sessionId := uuid.New()
	session := &sessionsV1.CreateSessionResponse{
		SessionId:    sessionId.String(),
		RefreshToken: sessionInfo.RefreshToken,
		ExpiresAt:    sessionInfo.RefreshTokenExpiresAt,
	}

	ss := &sessionsV1.GetSessionResponse{
		SessionId:    sessionId.String(),
		RefreshToken: sessionInfo.RefreshToken,
		ExpiresAt:    sessionInfo.RefreshTokenExpiresAt,
	}
	sessions = append(sessions, ss)
	return session, nil
}
