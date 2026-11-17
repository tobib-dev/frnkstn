package main

import (
	"context"
	"errors"
	"strings"

	"github.com/gocql/gocql"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *serverService) AddChannel(
	ctx context.Context,
	req *serversv1.AddChannelRequest,
) (res *serversv1.AddChannelResponse, rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "AddChannel",
		"server_id", req.GetServerId())
	defer func() { logMemberRPCError(logger, rpcErr) }()
	serverID, err := s.authorizeChannels(
		ctx, req.GetServerId(), true)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.GetChannelName())
	if name == "" {
		return nil, status.Error(
			codes.InvalidArgument, "channel name is required")
	}
	channel, err := s.channels.AddChannel(
		ctx, db.Channel{
			ServerID:    serverID,
			ChannelID:   gocql.TimeUUID(),
			ChannelName: name,
		})
	if err != nil {
		return nil,
			s.channelError("add channel", err)
	}
	return &serversv1.AddChannelResponse{
		Item: channelItem(channel)}, nil
}

func (s *serverService) GetChannel(
	ctx context.Context,
	req *serversv1.GetChannelRequest,
) (res *serversv1.GetChannelResponse,
	rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "GetChannel", "server_id",
		req.GetServerId(), "channel_id", req.GetChannelId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	serverID, err := s.authorizeChannels(
		ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(
		req.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	channel, err := s.channels.GetChannel(
		ctx, serverID, channelID)
	if err != nil {
		return nil, s.channelError(
			"get channel", err)
	}
	return &serversv1.GetChannelResponse{
		Item: channelItem(channel)}, nil
}

func (s *serverService) GetChannels(
	ctx context.Context,
	req *serversv1.GetChannelsRequest,
) (res *serversv1.GetChannelsResponse,
	rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "GetChannels",
		"server_id", req.GetServerId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	serverID, err := s.authorizeChannels(
		ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	channels, err := s.channels.GetChannels(ctx, serverID)
	if err != nil {
		return nil, s.channelError("get channels", err)
	}
	items := make([]*serversv1.ChannelItem, len(channels))
	for i, channel := range channels {
		items[i] = channelItem(channel)
	}
	return &serversv1.GetChannelsResponse{
		Items: items}, nil
}

func (s *serverService) EditChannel(
	ctx context.Context,
	req *serversv1.EditChannelRequest,
) (res *serversv1.EditChannelResponse,
	rpcErr error) {
	item := req.GetItem()
	logger := s.cfg.logger.With(
		"operation", "EditChannel", "server_id",
		item.GetServerId(), "channel_id", item.GetChannelId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	serverID, err := s.authorizeChannels(
		ctx, item.GetServerId(), true)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(item.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(item.GetChannelName())
	if name == "" {
		return nil,
			status.Error(
				codes.InvalidArgument,
				"channel name is required")
	}
	channel, err := s.channels.EditChannel(
		ctx, db.EditChannelParams{
			ServerID:    serverID,
			ChannelID:   channelID,
			ChannelName: name,
		})
	if err != nil {
		return nil,
			s.channelError("edit channel", err)
	}
	return &serversv1.EditChannelResponse{
		Item: channelItem(channel)}, nil
}

func (s *serverService) DeleteChannel(
	ctx context.Context,
	req *serversv1.DeleteChannelRequest,
) (res *serversv1.DeleteChannelResponse,
	rpcErr error) {
	logger := s.cfg.logger.With(
		"operation", "DeleteChannel", "server_id",
		req.GetServerId(), "channel_id", req.GetChannelId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	serverID, err := s.authorizeChannels(
		ctx, req.GetServerId(), true)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(req.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	if err := s.channels.DeleteChannel(
		ctx, serverID, channelID); err != nil {
		return nil, s.channelError("delete channel", err)
	}
	return &serversv1.DeleteChannelResponse{}, nil
}

func (s *serverService) authorizeChannels(
	ctx context.Context, rawServerID string, moderatorOnly bool,
) (gocql.UUID, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return gocql.UUID{}, err
	}
	serverID, err := parseID(rawServerID, "server ID")
	if err != nil {
		return gocql.UUID{}, err
	}
	server, err := s.store.GetServer(ctx, serverID)
	if err != nil {
		return gocql.UUID{}, s.memberError("get server", err)
	}
	if server.Owner == userID {
		return serverID, nil
	}
	moderator, err := s.members.IsServerModerator(
		ctx, serverID, userID)
	if err != nil {
		return gocql.UUID{},
			s.memberError("check moderator", err)
	}
	if moderator {
		return serverID, nil
	}
	if moderatorOnly {
		return gocql.UUID{},
			status.Error(
				codes.PermissionDenied,
				"moderator permission required")
	}
	if _, err := s.members.GetMember(
		ctx, serverID, userID); err != nil {
		if errors.Is(err, gocql.ErrNotFound) {
			return gocql.UUID{},
				status.Error(
					codes.PermissionDenied,
					"server membership required")
		}
		return gocql.UUID{},
			s.memberError("check membership", err)
	}
	return serverID, nil
}

func (s *serverService) channelError(
	operation string,
	err error) error {
	if errors.Is(err, gocql.ErrNotFound) {
		return status.Error(
			codes.NotFound, "channel not found")
	}
	s.cfg.logger.Error(
		"channel operation failed",
		"operation", operation, "error", err)
	return status.Error(codes.Internal, "could not "+operation)
}

func channelItem(channel db.Channel) *serversv1.ChannelItem {
	return &serversv1.ChannelItem{
		ServerId:    channel.ServerID.String(),
		ChannelId:   channel.ChannelID.String(),
		ChannelName: channel.ChannelName,
	}
}
