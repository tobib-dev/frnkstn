package main

import (
	"context"
	"strconv"

	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"github.com/tobib-dev/frnkstn/api/internal/auth"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *notificationService) GetServerMessageN10N(ctx context.Context, _ *ntfnV1.GetServerMessageN10NRequest) (*ntfnV1.GetServerMessageN10NResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionReadMessages)
	if err != nil {
		return nil, err
	}
	items, err := s.store.GetServerMessageNotifications(ctx, userID)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not get server message notifications")
	}
	if len(items) == 0 {
		return nil, status.Error(codes.NotFound, "server message notification not found")
	}
	return &ntfnV1.GetServerMessageN10NResponse{Item: serverMessageNotificationItem(items[0])}, nil
}

func (s *notificationService) UpdateServerMessageN10N(ctx context.Context, req *ntfnV1.UpdateServerMessageN10NRequest) (*ntfnV1.UpdateServerMessageN10NResponse, error) {
	if req.GetItem() == nil {
		return nil, status.Error(codes.InvalidArgument, "server message notification is required")
	}
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionReadMessages)
	if err != nil {
		return nil, err
	}
	_, notificationID, err := notificationIDs(userID.String(), req.GetItem().GetId())
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateServerMessageNotification(ctx, userID, notificationID); err != nil {
		return nil, notificationUpdateError(err)
	}
	return &ntfnV1.UpdateServerMessageN10NResponse{}, nil
}

func serverMessageNotificationItem(n db.ServerMessageNotification) *ntfnV1.ServerMessageN10NItem {
	return &ntfnV1.ServerMessageN10NItem{
		Id: n.NotificationID.String(), RecipientId: n.UserID.String(),
		ServerId: n.ServerID.String(), ChannelId: n.ChannelID.String(),
		MessageId: n.MessageID.String(), Content: n.Content,
		Seen: strconv.FormatBool(n.Seen), MessageTime: timestamppb.New(n.MessageID.Time()),
	}
}

func (s *notificationService) addServerMessageNotification(ctx context.Context, item *ntfnV1.ServerMessageN10NItem) error {
	userID, err := parseID(item.GetRecipientId(), "recipient ID")
	if err != nil {
		return err
	}
	serverID, err := parseID(item.GetServerId(), "server ID")
	if err != nil {
		return err
	}
	channelID, err := parseID(item.GetChannelId(), "channel ID")
	if err != nil {
		return err
	}
	messageID, err := parseID(item.GetMessageId(), "message ID")
	if err != nil {
		return err
	}
	notificationID, err := parseID(item.GetId(), "notification ID")
	if err != nil {
		return err
	}
	return s.store.AddServerMessageNotification(ctx, db.ServerMessageNotification{
		NotificationID: notificationID, UserID: userID, ServerID: serverID,
		ChannelID: channelID, MessageID: messageID, Content: messageNotificationPreview(item.GetContent()),
	})
}
