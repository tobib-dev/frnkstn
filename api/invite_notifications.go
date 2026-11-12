package main

import (
	"context"
	"strings"
	"time"

	"github.com/gocql/gocql"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *serverService) notifyServerInvite(
	invite db.ShareCode,
	serverName string,
) {
	item := &ntfnV1.InviteN10NItem{
		Id:          gocql.TimeUUID().String(),
		RecipientId: invite.Receiver.String(),
		ServerId:    invite.ServerID.String(),
		ServerName:  serverName,
		SenderId:    invite.Sender.String(),
		ShareCode:   invite.Code,
	}
	logger := s.cfg.logger.With("operation", "notifyServerInvite", "server_id", invite.ServerID)
	notifications := s.n10n
	// shortcut: async publish can be lost on shutdown; use an outbox for guaranteed delivery.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		notifications.publishNotification(ctx, item.RecipientId, &ntfnV1.SubscribeResponse{
			Item: &ntfnV1.SubscribeResponse_Invite{Invite: item},
		}, logger, func() {
			if err := notifications.addInviteNotification(ctx, item); err != nil {
				logger.Error("failed to store invite notification", "error", err)
			}
		})
	}()
}

func (s *notificationService) addInviteNotification(
	ctx context.Context, item *ntfnV1.InviteN10NItem,
) error {
	userID, notificationID, err := notificationIDs(item.GetRecipientId(), item.GetId())
	if err != nil {
		return err
	}
	serverID, err := parseID(item.GetServerId(), "server ID")
	if err != nil {
		return err
	}
	senderID, err := parseID(item.GetSenderId(), "sender ID")
	if err != nil {
		return err
	}
	if strings.TrimSpace(item.GetShareCode()) == "" {
		return status.Error(codes.InvalidArgument, "share code is required")
	}
	err = s.store.AddInviteNotification(ctx, db.InviteNotification{
		UserID: userID, NotificationID: notificationID,
		ServerID: serverID, ServerName: item.GetServerName(),
		SenderID: senderID, ShareCode: item.GetShareCode(),
	})
	if err != nil {
		s.cfg.logger.Error("failed to add invite notification", "error", err)
		return status.Error(codes.Internal, "could not add invite notification")
	}
	return nil
}

func (s *notificationService) UpdateInviteN10N(
	ctx context.Context, req *ntfnV1.UpdateInviteN10NRequest,
) (*ntfnV1.UpdateInviteN10NResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, "")
	if err != nil {
		return nil, err
	}
	id, err := parseID(req.GetId(), "notification ID")
	if err != nil {
		return nil, err
	}
	if err := s.store.UpdateInviteNotification(ctx, userID, id); err != nil {
		s.cfg.logger.Error("failed to update invite notification", "user_id", userID, "error", err)
		return nil, notificationUpdateError(err)
	}
	return &ntfnV1.UpdateInviteN10NResponse{}, nil
}

func inviteNotificationItem(item db.InviteNotification) *ntfnV1.InviteN10NItem {
	return &ntfnV1.InviteN10NItem{
		Id: item.NotificationID.String(), RecipientId: item.UserID.String(),
		ServerId: item.ServerID.String(), ServerName: item.ServerName,
		SenderId: item.SenderID.String(), ShareCode: item.ShareCode,
	}
}
