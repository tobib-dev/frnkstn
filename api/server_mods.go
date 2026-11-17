package main

import (
	"context"
	"errors"

	"github.com/gocql/gocql"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Any moderator can add a mod
func (s *serverService) AddMod(
	ctx context.Context,
	req *serversv1.AddModRequest,
) (response *serversv1.AddModResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "AddMod", "server_id", req.GetServerId(),
		"mod_id", req.GetModUserId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	server, err := s.authorizeMods(
		ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	modID, err := parseID(
		req.GetModUserId(), "moderator user ID")
	if err != nil {
		return nil, err
	}
	// Server creation already grants the owner moderator permissions.
	if modID == server.Owner {
		return nil, status.Error(
			codes.AlreadyExists,
			"server owner is already a moderator")
	}
	mod, err := s.mods.AddMod(ctx, server.ServerID, modID)
	if err != nil {
		return nil, s.memberError("add moderator", err)
	}
	return &serversv1.AddModResponse{Item: modItem(mod)}, nil
}

func (s *serverService) GetMod(
	ctx context.Context,
	req *serversv1.GetModRequest,
) (response *serversv1.GetModResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "GetMod", "server_id", req.GetServerId(),
		"mod_id", req.GetModId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	server, err := s.authorizeMods(
		ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	modID, err := parseID(
		req.GetModId(), "moderator ID")
	if err != nil {
		return nil, err
	}
	mod, err := s.mods.GetMod(
		ctx, server.ServerID, modID)
	if err != nil {
		return nil,
			s.memberError("get moderator", err)
	}
	return &serversv1.GetModResponse{
		Item: modItem(mod)}, nil
}

func (s *serverService) GetMods(
	ctx context.Context,
	req *serversv1.GetModsRequest,
) (response *serversv1.GetModsResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "GetMods", "server_id", req.GetServerId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	server, err := s.authorizeMods(
		ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	mods, err := s.mods.GetMods(
		ctx, server.ServerID)
	if err != nil {
		return nil,
			s.memberError("get moderators", err)
	}
	items := make([]*serversv1.ModItem, len(mods))
	for i, mod := range mods {
		items[i] = modItem(mod)
	}
	return &serversv1.GetModsResponse{
		Items: items}, nil
}

func (s *serverService) DeleteMod(
	ctx context.Context,
	req *serversv1.DeleteModRequest,
) (response *serversv1.DeleteModResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "DeleteMod", "server_id", req.GetServerId(),
		"mod_id", req.GetModId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	server, err := s.authorizeMods(
		ctx, req.GetServerId(), true)
	if err != nil {
		return nil, err
	}
	modID, err := parseID(
		req.GetModId(), "moderator ID")
	if err != nil {
		return nil, err
	}
	// Both fields identify the same user in the current schema.
	if req.GetModUserId() != "" {
		userID, err := parseID(
			req.GetModUserId(), "moderator user ID")
		if err != nil {
			return nil, err
		}
		if userID != modID {
			return nil,
				status.Error(
					codes.InvalidArgument, "moderator IDs must match")
		}
	}
	if modID == server.Owner {
		return nil,
			status.Error(
				codes.PermissionDenied, "cannot demote server owner")
	}
	if err := s.mods.DeleteMod(
		ctx, server.ServerID, modID); err != nil {
		return nil,
			s.memberError("delete moderator", err)
	}
	return &serversv1.DeleteModResponse{}, nil
}

func (s *serverService) authorizeMods(
	ctx context.Context, rawServerID string, ownerOnly bool,
) (db.Server, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return db.Server{}, err
	}
	serverID, err := parseID(rawServerID, "server ID")
	if err != nil {
		return db.Server{}, err
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return db.Server{}, s.memberError("get server", err)
	}
	if userID == server.Owner {
		return server, nil
	}
	if ownerOnly {
		return db.Server{}, status.Error(
			codes.PermissionDenied,
			"server owner permission required")
	}
	moderator, err := s.members.IsServerModerator(
		ctx, serverID, userID)
	if err != nil {
		return db.Server{},
			s.memberError("check moderator", err)
	}
	if !moderator {
		_, err := s.members.GetMember(
			ctx, serverID, userID)
		if errors.Is(err, gocql.ErrNotFound) {
			return db.Server{},
				status.Error(
					codes.PermissionDenied,
					"server membership required")
		}
		if err != nil {
			return db.Server{},
				s.memberError("check membership", err)
		}
	}
	return server, nil
}

func modItem(mod db.Mod) *serversv1.ModItem {
	return &serversv1.ModItem{
		ServerId: mod.ServerID.String(),
		ModId:    mod.ModID.String(),
		ModName:  mod.ModName,
	}
}
