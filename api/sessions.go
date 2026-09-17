package main

import (
	"context"
	//"fmt"

	"github.com/google/uuid"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
)

type SessionService struct {
	cfg *Config
	sessionsV1.UnimplementedSessionServiceServer
}

func NewSessionService(cfg *Config) *SessionService {
	return &SessionService{cfg: cfg}
}

func (serv *SessionService) CreateSession(ctx context.Context, sessionInfo *sessionsV1.CreateSessionRequest) (*sessionsV1.CreateSessionResponse, error) {
	sessId := uuid.New()
	serv.cfg.logger.Info("creating session", "sessionID", sessId.String(), "token", sessionInfo.RefreshToken)
	return &sessionsV1.CreateSessionResponse{
		SessionId: sessId.String(),
	}, nil
}

func (serv *SessionService) GetSessions(ctx context.Context, req *sessionsV1.GetSessionRequest) (*sessionsV1.GetSessionResponse, error) {
	return &sessionsV1.GetSessionResponse{}, nil
}
