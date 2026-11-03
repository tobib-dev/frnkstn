package main

import (
	"context"

	servV1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/db"
)

type serverService struct {
	cfg   *Config
	store db.ServerStore
	servV1.UnimplementedServerServiceServer
}

func NewServerService(cfg *Config) *serverService {
	service := &serverService{cfg: cfg, store: &cfg.db.session}
	return service
}

func (s *serverService) CreateServer(
	ctx context.Context,
	req *servV1.CreateServerRequest,
) (*servV1.CreateServerResponse, error) {
	//
	return &servV1.CreateServerResponse{}, nil
}
