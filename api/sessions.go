package main

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	"github.com/tobib-dev/frnkstn/api/db"
)

type SessionService struct {
	db *db.DB
	sessionsV1.UnimplementedSessionServiceServer
}

func NewSessionService(db *db.DB) *SessionService {
	return &SessionService{db: db}
}

func (cfg *SessionService) CreateSession(ctx context.Context, sessionInfo *sessionsV1.CreateSessionRequest) (*sessionsV1.CreateSessionResponse, error) {
	var sessions []string
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

	log.Printf("Successfully created session: %s\n", sessionId.String())
	return session, nil
}

func (cfg *SessionService) GetSessions(ctx context.Context, req *sessionsV1.GetSessionRequest) (*sessionsV1.GetSessionResponse, error) {
	return &sessionsV1.GetSessionResponse{}, nil
}
