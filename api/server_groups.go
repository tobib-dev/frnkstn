package main

import (
	"context"
	"errors"
	"time"

	"github.com/gocql/gocql"
	servV1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type serverService struct {
	cfg   *Config
	store db.ServerStore
	users db.UserStore
	servV1.UnimplementedServerServiceServer
}

func NewServerService(cfg *Config) *serverService {
	service := &serverService{
		cfg:   cfg,
		store: &cfg.db.session,
		users: &cfg.db.session,
	}
	return service
}

func (s *serverService) CreateServer(
	ctx context.Context,
	req *servV1.CreateServerRequest,
) (*servV1.CreateServerResponse, error) {
	logger := s.cfg.logger
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	user, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		logger.Error("failed to get server creator", "user_id", userID, "error", err)
		return nil, userLookupError(err)
	}

	serverID := gocql.UUIDFromTime(time.Now())
	server, err := s.store.CreateServer(
		ctx,
		db.Server{
			ServerID:   serverID,
			ServerName: req.GetServerName(),
			ServerBio:  req.GetServerBio(),
			Owner:      userID,
			OwnerName:  user.Username,
		},
	)
	if err != nil {
		logger.Error("failed to create server", "error", err)
		return nil, status.Error(codes.Internal, "could not create server")
	}
	return &servV1.CreateServerResponse{
		Item: serverItem(server),
	}, nil
}

func (s *serverService) GetServer(
	ctx context.Context,
	req *servV1.GetServerRequest,
) (*servV1.GetServerResponse, error) {
	logger := s.cfg.logger
	if _, err := authenticatedUserID(ctx, s.cfg, ""); err != nil {
		return nil, err
	}
	serverID, err := parseID(req.GetServerId(), "server ID")
	if err != nil {
		return nil, err
	}

	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		logger.Error("failed to get server", "server_id", serverID, "error", err)
		return nil, serverLookupError(err)
	}
	return &servV1.GetServerResponse{
		Item: serverItem(server),
	}, nil
}

func (s *serverService) GetUserServers(
	ctx context.Context,
	req *servV1.GetUserServersRequest,
) (*servV1.GetUserServersResponse, error) {
	logger := s.cfg.logger
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}

	servers, err := s.store.GetServersByUser(ctx, userID)
	if err != nil {
		logger.Error("failed to list user servers", "user_id", userID, "error", err)
		return nil, status.Error(codes.Internal, "could not get user servers")
	}
	serverItems := make([]*servV1.UserServerItem, len(servers))

	for i, serv := range servers {
		serverItems[i] = userServerItem(serv)
	}
	return &servV1.GetUserServersResponse{
		Items: serverItems,
	}, nil
}

/*
 * Edit Server must clean up records with a background task
 * to reduce user facing latency. Must also verify if owner
 * is being changed. If so, owner can only be changed by
 * current owner
 */
func (s *serverService) EditServer(
	ctx context.Context,
	req *servV1.EditServerRequest,
) (*servV1.EditServerResponse, error) {
	item := req.GetItem()
	if item == nil {
		return nil, status.Error(codes.InvalidArgument, "server item is required")
	}
	logger := s.cfg.logger
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}

	serverID, err := parseID(item.ServerId, "server ID")
	if err != nil {
		return nil, err
	}
	ownerID, err := parseID(item.Owner, "server owner ID")
	if err != nil {
		return nil, err
	}

	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		logger.Error("failed to get server for edit", "server_id", serverID, "error", err)
		return nil, serverLookupError(err)
	}
	if userID != server.Owner {
		return nil, status.Error(codes.PermissionDenied, "only the owner can edit the server")
	}
	newServerInfo, err := s.store.EditServer(
		ctx, serverID,
		db.EditServerParams{
			ServerName: item.ServerName,
			ServerBio:  item.ServerBio,
			Owner:      ownerID,
		},
	)
	if err != nil {
		logger.Error("failed to edit server", "server_id", serverID, "error", err)
		return nil, status.Error(codes.Internal, "could not edit server")
	}

	// handle cleaning up server by users and return server item
	if server.ServerName != newServerInfo.ServerName {
		s.handleServerByUserCleanup(ctx, serverID, "update server name")
	}
	return &servV1.EditServerResponse{
		Item: serverItem(newServerInfo),
	}, nil
}

/*
 * Delete Server similar to Edit must clean up records of
 * variant tables in a background task.
 */
func (s *serverService) DeleteServer(
	ctx context.Context,
	req *servV1.DeleteServerRequest,
) (*servV1.DeleteServerResponse, error) {
	logger := s.cfg.logger
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}

	serverID, err := parseID(req.GetServerId(), "server ID")
	if err != nil {
		return nil, err
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		logger.Error("failed to get server for deletion", "server_id", serverID, "error", err)
		return nil, serverLookupError(err)
	}
	if server.Owner != userID {
		return nil, status.Error(codes.PermissionDenied, "only the owner can delete the server")
	}
	delTime := time.Now()
	err = s.store.DeleteServer(ctx, serverID, &delTime)
	if err != nil {
		logger.Error("failed to delete server", "server_id", serverID, "error", err)
		return nil, status.Error(codes.Internal, "could not delete server")
	}
	s.handleServerByUserCleanup(ctx, serverID, "delete server")
	return &servV1.DeleteServerResponse{
		Status: "successfully deleted",
	}, nil
}

func serverLookupError(err error) error {
	if errors.Is(err, gocql.ErrNotFound) {
		return status.Error(codes.NotFound, "server not found")
	}
	return status.Error(codes.Internal, "could not get server")
}

func serverItem(serv db.Server) *servV1.ServerItem {
	return &servV1.ServerItem{
		ServerId:   serv.ServerID.String(),
		ServerName: serv.ServerName,
		ServerBio:  serv.ServerBio,
		Owner:      serv.Owner.String(),
	}
}

func userServerItem(serv db.ServerByUser) *servV1.UserServerItem {
	return &servV1.UserServerItem{
		ServerId:   serv.ServerID.String(),
		ServerName: serv.ServerName,
		Cursor:     timestamppb.New(serv.Cursor),
	}
}
