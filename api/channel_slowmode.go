package main

import (
	"context"
	"math"
	"time"

	moderationsv1 "github.com/tobib-dev/frnkstn-proto/moderations"
	"github.com/tobib-dev/frnkstn/api/internal/auth"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *serverService) SetSlowMode(
	ctx context.Context, req *moderationsv1.SetSlowModeRequest,
) (*moderationsv1.SetSlowModeResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionWriteMessages)
	if err != nil {
		return nil, err
	}
	serverID, err := s.authorizeChannels(ctx, req.GetServerId(), true)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(req.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	if req.GetCooldownSeconds() > math.MaxInt32 {
		return nil, status.Error(codes.InvalidArgument, "cooldown exceeds the maximum supported seconds")
	}
	if _, err := s.channels.GetChannel(ctx, serverID, channelID); err != nil {
		return nil, s.channelError("get channel", err)
	}
	seconds := int(req.GetCooldownSeconds())
	_, err = s.settings.AddSlowMode(ctx, db.ChannelSetting{
		ServerID: serverID, ChannelID: channelID, SlowModeSeconds: &seconds,
		UpdatedBy: userID, UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		return nil, s.serverMessageError("set slowmode", err)
	}
	return &moderationsv1.SetSlowModeResponse{CooldownSeconds: uint32(seconds)}, nil
}

func (s *serverService) GetSlowMode(
	ctx context.Context, req *moderationsv1.GetSlowModeRequest,
) (*moderationsv1.GetSlowModeResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionReadMessages)
	if err != nil {
		return nil, err
	}
	serverID, err := s.authorizeChannels(ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(req.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	if _, err := s.channels.GetChannel(ctx, serverID, channelID); err != nil {
		return nil, s.channelError("get channel", err)
	}
	seconds, err := s.settings.GetCoolDownTime(ctx, serverID, channelID)
	if err != nil {
		return nil, s.serverMessageError("get slowmode", err)
	}
	response := &moderationsv1.GetSlowModeResponse{}
	if seconds == nil || *seconds == 0 {
		return response, nil
	}
	response.CooldownSeconds = uint32(*seconds)
	wait, err := s.settings.GetChannelWait(ctx, serverID, channelID, userID)
	if err != nil {
		return nil, s.serverMessageError("get channel wait", err)
	}
	response.RemainingMillis = int64((wait + time.Millisecond - 1) / time.Millisecond)
	return response, nil
}
