package main

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/gocql/gocql"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type notificationService struct {
	cfg   *Config
	store db.NotificationStore
	ntfnV1.UnimplementedN10NServiceServer
}

const messageNotificationPreviewLength = 80

func NewNotificationService(cfg *Config) *notificationService {
	return &notificationService{cfg: cfg, store: &cfg.db.session}
}

func (s *notificationService) AddMsgN10N(ctx context.Context, req *ntfnV1.AddMsgN10NRequest) (*emptypb.Empty, error) {
	logger := s.cfg.logger.With("location", "AddMsgN10N")
	item := req.GetItem()
	if item == nil {
		logger.Warn("message notification is required")
		return nil, status.Error(codes.InvalidArgument, "message notification is required")
	}
	messageID, err := parseID(item.GetMessageId(), "message ID")
	if err != nil {
		logger.Warn("invalid message ID", "error", err)
		return nil, err
	}
	userID, err := parseID(item.GetUserId(), "user ID")
	if err != nil {
		logger.Warn("invalid user ID", "error", err)
		return nil, err
	}
	authorID, err := parseID(item.GetAuthorId(), "author ID")
	if err != nil {
		logger.Warn("invalid author ID", "error", err)
		return nil, err
	}
	logger = logger.With("user_id", userID.String(), "message_id", messageID.String(), "author_id", authorID.String())
	notification := db.MessageNotification{
		NotificationID: gocql.TimeUUID(),
		UserID:         userID,
		MessageID:      messageID,
		AuthorID:       authorID,
		AuthorName:     strings.TrimSpace(item.GetAuthorName()),
		Content:        messageNotificationPreview(item.GetContent()),
	}
	if err := s.store.AddMessageNotification(ctx, notification); err != nil {
		logger.Error("failed to add message notification", "error", err)
		return nil, status.Error(codes.Internal, "could not add message notification")
	}
	return &emptypb.Empty{}, nil
}

func messageNotificationPreview(content string) string {
	content = strings.TrimSpace(content)
	runes := []rune(content)
	if len(runes) <= messageNotificationPreviewLength {
		return content
	}
	return string(runes[:messageNotificationPreviewLength]) + "…"
}

func (s *notificationService) GetMsgN10N(ctx context.Context, req *ntfnV1.GetMsgN10NRequest) (*ntfnV1.GetMsgN10NResponse, error) {
	logger := s.cfg.logger.With("location", "GetMsgN10N")
	item := req.GetItem()
	if item == nil {
		logger.Warn("message notification is required")
		return nil, status.Error(codes.InvalidArgument, "message notification is required")
	}
	userID, err := parseID(item.GetUserId(), "user ID")
	if err != nil {
		logger.Warn("invalid user ID", "error", err)
		return nil, err
	}
	logger = logger.With("user_id", userID.String())
	notifications, err := s.store.GetMessageNotification(ctx, userID)
	if err != nil {
		logger.Error("failed to get message notification", "error", err)
		return nil, status.Error(codes.Internal, "could not get message notification")
	}
	if len(notifications) == 0 {
		return nil, status.Error(codes.NotFound, "message notification not found")
	}
	return &ntfnV1.GetMsgN10NResponse{Item: messageNotificationItem(notifications[0])}, nil
}

func (s *notificationService) UpdateMsgN10N(ctx context.Context, req *ntfnV1.UpdateMsgN10NRequest) (*emptypb.Empty, error) {
	logger := s.cfg.logger.With("location", "UpdateMsgN10N")
	item := req.GetItem()
	if item == nil {
		logger.Warn("message notification is required")
		return nil, status.Error(codes.InvalidArgument, "message notification is required")
	}
	userID, notificationID, err := notificationIDs(item.GetUserId(), item.GetId())
	if err != nil {
		logger.Warn("invalid message notification", "error", err)
		return nil, err
	}
	logger = logger.With("user_id", userID.String(), "notification_id", notificationID.String())
	if err := s.store.UpdateMessageNotification(ctx, userID, notificationID); err != nil {
		logger.Error("failed to update message notification", "error", err)
		return nil, notificationUpdateError(err)
	}
	return &emptypb.Empty{}, nil
}

func (s *notificationService) AddFriendN10N(ctx context.Context, req *ntfnV1.AddFriendN10NRequest) (*emptypb.Empty, error) {
	logger := s.cfg.logger.With("location", "AddFriendN10N")
	item := req.GetItem()
	if item == nil {
		logger.Warn("friend notification is required")
		return nil, status.Error(codes.InvalidArgument, "friend notification is required")
	}
	friendshipID, err := parseID(item.GetFriendshipId(), "friendship ID")
	if err != nil {
		logger.Warn("invalid friendship ID", "error", err)
		return nil, err
	}
	userID, err := parseID(item.GetUserId(), "user ID")
	if err != nil {
		logger.Warn("invalid user ID", "error", err)
		return nil, err
	}
	friendID, err := parseID(item.GetFriendId(), "friend ID")
	if err != nil {
		logger.Warn("invalid friend ID", "error", err)
		return nil, err
	}
	logger = logger.With("user_id", userID.String(), "friendship_id", friendshipID.String(), "friend_id", friendID.String())
	notification := db.FriendNotification{
		NotificationID: gocql.TimeUUID(),
		UserID:         userID,
		FriendshipID:   friendshipID,
		FriendID:       friendID,
		FriendName:     strings.TrimSpace(item.GetFriendName()),
		Status:         item.GetStatus(),
	}
	if err := s.store.AddFriendNotification(ctx, notification); err != nil {
		logger.Error("failed to add friend notification", "error", err)
		return nil, status.Error(codes.Internal, "could not add friend notification")
	}
	return &emptypb.Empty{}, nil
}

func (s *notificationService) GetFriendN10N(ctx context.Context, req *ntfnV1.GetFriendN10NRequest) (*ntfnV1.GetFriendN10NResponse, error) {
	logger := s.cfg.logger.With("location", "GetFriendN10N")
	item := req.GetItem()
	if item == nil {
		logger.Warn("friend notification is required")
		return nil, status.Error(codes.InvalidArgument, "friend notification is required")
	}
	userID, err := parseID(item.GetUserId(), "user ID")
	if err != nil {
		logger.Warn("invalid user ID", "error", err)
		return nil, err
	}
	logger = logger.With("user_id", userID.String())
	notifications, err := s.store.GetFriendNotification(ctx, userID)
	if err != nil {
		logger.Error("failed to get friend notification", "error", err)
		return nil, status.Error(codes.Internal, "could not get friend notification")
	}
	if len(notifications) == 0 {
		return nil, status.Error(codes.NotFound, "friend notification not found")
	}
	return &ntfnV1.GetFriendN10NResponse{Item: friendNotificationItem(notifications[0])}, nil
}

func (s *notificationService) UpdateFriendN10N(ctx context.Context, req *ntfnV1.UpdateFriendN10NRequest) (*emptypb.Empty, error) {
	logger := s.cfg.logger.With("location", "UpdateFriendN10N")
	item := req.GetItem()
	if item == nil {
		logger.Warn("friend notification is required")
		return nil, status.Error(codes.InvalidArgument, "friend notification is required")
	}
	userID, notificationID, err := notificationIDs(item.GetUserId(), item.GetId())
	if err != nil {
		logger.Warn("invalid friend notification", "error", err)
		return nil, err
	}
	logger = logger.With("user_id", userID.String(), "notification_id", notificationID.String())
	if err := s.store.UpdateFriendNotification(ctx, userID, notificationID); err != nil {
		logger.Error("failed to update friend notification", "error", err)
		return nil, notificationUpdateError(err)
	}
	return &emptypb.Empty{}, nil
}

func notificationIDs(userID, notificationID string) (gocql.UUID, gocql.UUID, error) {
	user, err := parseID(userID, "user ID")
	if err != nil {
		return gocql.UUID{}, gocql.UUID{}, err
	}
	notification, err := parseID(notificationID, "notification ID")
	if err != nil {
		return gocql.UUID{}, gocql.UUID{}, err
	}
	return user, notification, nil
}

func notificationUpdateError(err error) error {
	if errors.Is(err, gocql.ErrNotFound) {
		return status.Error(codes.NotFound, "notification not found")
	}
	return status.Error(codes.Internal, "could not update notification")
}

func messageNotificationItem(notification db.MessageNotification) *ntfnV1.MsgN10NItem {
	return &ntfnV1.MsgN10NItem{
		Id:          notification.NotificationID.String(),
		MessageId:   notification.MessageID.String(),
		UserId:      notification.UserID.String(),
		AuthorId:    notification.AuthorID.String(),
		AuthorName:  notification.AuthorName,
		Content:     notification.Content,
		Seen:        strconv.FormatBool(notification.Seen),
		MessageTime: timestamppb.New(notification.MessageID.Time()),
	}
}

func friendNotificationItem(notification db.FriendNotification) *ntfnV1.FriendN10NItem {
	return &ntfnV1.FriendN10NItem{
		Id:           notification.NotificationID.String(),
		FriendshipId: notification.FriendshipID.String(),
		UserId:       notification.UserID.String(),
		FriendId:     notification.FriendID.String(),
		FriendName:   notification.FriendName,
		Status:       notification.Status,
		Seen:         strconv.FormatBool(notification.Seen),
		RequestTime:  timestamppb.New(notification.FriendshipID.Time()),
	}
}
