package main

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gocql/gocql"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"github.com/tobib-dev/frnkstn/api/messaging"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type notificationBroker interface {
	Publish(context.Context, string, string, []byte) (bool, error)
}

type rabbitNotificationBroker struct{ uri string }

func (b rabbitNotificationBroker) Publish(ctx context.Context, exchange, routingKey string, body []byte) (bool, error) {
	return messaging.Publish(ctx, exchange, routingKey, b.uri, body)
}

type notificationService struct {
	cfg    *Config
	store  db.NotificationStore
	broker notificationBroker
	ntfnV1.UnimplementedN10NServiceServer
}

const (
	messageNotificationPreviewLength = 80
	notificationExchange             = "notifications"
	notificationTopic                = "notifications."
)

func NewNotificationService(cfg *Config) *notificationService {
	service := &notificationService{cfg: cfg, store: &cfg.db.session}
	if cfg.brokerURI != "" {
		service.broker = rabbitNotificationBroker{uri: cfg.brokerURI}
	}
	return service
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
	notificationID := gocql.TimeUUID()
	if item.GetId() != "" {
		notificationID, err = parseID(item.GetId(), "notification ID")
		if err != nil {
			logger.Warn("invalid notification ID", "error", err)
			return nil, err
		}
	}
	notification := db.MessageNotification{
		NotificationID: notificationID,
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
	notificationID := gocql.TimeUUID()
	if item.GetId() != "" {
		notificationID, err = parseID(item.GetId(), "notification ID")
		if err != nil {
			logger.Warn("invalid notification ID", "error", err)
			return nil, err
		}
	}
	notification := db.FriendNotification{
		NotificationID: notificationID,
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

func (s *notificationService) publishMessageNotification(ctx context.Context, item *ntfnV1.MsgN10NItem, logger *slog.Logger) {
	item.Content = messageNotificationPreview(item.GetContent())
	s.publishNotification(ctx, item.GetUserId(), &ntfnV1.SubscribeResponse{Item: &ntfnV1.SubscribeResponse_Message{Message: item}}, logger, func() {
		s.storeMessageNotification(ctx, item, logger)
	})
}

func (s *notificationService) publishFriendNotification(ctx context.Context, item *ntfnV1.FriendN10NItem, logger *slog.Logger) {
	s.publishNotification(ctx, item.GetUserId(), &ntfnV1.SubscribeResponse{Item: &ntfnV1.SubscribeResponse_Friend{Friend: item}}, logger, func() {
		s.storeFriendNotification(ctx, item, logger)
	})
}

func (s *notificationService) publishNotification(ctx context.Context, userID string, item *ntfnV1.SubscribeResponse, logger *slog.Logger, store func()) {
	body, err := proto.Marshal(item)
	if err != nil {
		logger.Error("failed to encode notification", "error", err)
		store()
		return
	}
	if s.broker == nil {
		store()
		return
	}
	routed, err := s.broker.Publish(ctx, notificationExchange, notificationRoutingKey(userID), body)
	if err != nil {
		logger.Error("failed to publish notification", "error", err)
		store()
		return
	}
	if !routed {
		logger.Warn("notification was not routed; storing directly")
		store()
		return
	}
	logger.Info("notification published")
}

func (s *notificationService) storeMessageNotification(ctx context.Context, item *ntfnV1.MsgN10NItem, logger *slog.Logger) {
	if _, err := s.AddMsgN10N(ctx, &ntfnV1.AddMsgN10NRequest{Item: item}); err != nil {
		logger.Error("failed to store message notification", "error", err)
	}
}

func (s *notificationService) storeFriendNotification(ctx context.Context, item *ntfnV1.FriendN10NItem, logger *slog.Logger) {
	if _, err := s.AddFriendN10N(ctx, &ntfnV1.AddFriendN10NRequest{Item: item}); err != nil {
		logger.Error("failed to store friend notification", "error", err)
	}
}

func notificationRoutingKey(userID string) string {
	return notificationTopic + userID
}

func (s *notificationService) StartNotificationConsumer(ctx context.Context) error {
	if s.cfg.brokerURI == "" {
		return nil
	}
	consumer, closeConsumer, err := messaging.DeclareAndBindConsumer(ctx, notificationExchange, notificationTopic+"#", s.cfg.brokerURI)
	if err != nil {
		return err
	}
	go func() {
		defer func() {
			closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_ = closeConsumer(closeCtx)
		}()
		logger := s.cfg.logger.With("location", "StartNotificationConsumer", "topic", notificationTopic+"#")
		for {
			delivery, err := consumer.Receive(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					logger.Error("failed to receive notification", "error", err)
				}
				return
			}
			if delivery == nil || delivery.Message() == nil {
				logger.Error("received empty notification")
				continue
			}
			item := &ntfnV1.SubscribeResponse{}
			if err := proto.Unmarshal(delivery.Message().GetData(), item); err != nil {
				logger.Error("failed to decode notification", "error", err)
				_ = delivery.Discard(ctx, nil)
				continue
			}
			var addErr error
			switch {
			case item.GetMessage() != nil:
				_, addErr = s.AddMsgN10N(ctx, &ntfnV1.AddMsgN10NRequest{Item: item.GetMessage()})
			case item.GetFriend() != nil:
				_, addErr = s.AddFriendN10N(ctx, &ntfnV1.AddFriendN10NRequest{Item: item.GetFriend()})
			default:
				logger.Error("received notification without an item")
				_ = delivery.Discard(ctx, nil)
				continue
			}
			if addErr != nil {
				logger.Error("failed to add subscribed notification", "error", addErr)
				_ = delivery.Requeue(ctx)
				time.Sleep(time.Second)
				continue
			}
			if err := delivery.Accept(ctx); err != nil {
				logger.Error("failed to accept message notification", "error", err)
			}
		}
	}()
	return nil
}

func (s *notificationService) Subscribe(req *ntfnV1.SubscribeRequest, stream grpc.ServerStreamingServer[ntfnV1.SubscribeResponse]) error {
	userID, err := parseID(req.GetUserId(), "user ID")
	if err != nil {
		return err
	}
	type result struct {
		item *ntfnV1.SubscribeResponse
		err  error
		done bool
	}
	ctx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	results := make(chan result)
	send := func(value result) bool {
		select {
		case results <- value:
			return true
		case <-ctx.Done():
			return false
		}
	}
	go func() {
		messages, err := s.store.GetMessageNotification(ctx, userID)
		if err != nil {
			send(result{err: status.Error(codes.Internal, "could not get message notifications")})
			return
		}
		for _, notification := range messages {
			if !send(result{item: &ntfnV1.SubscribeResponse{Item: &ntfnV1.SubscribeResponse_Message{Message: messageNotificationItem(notification)}}}) {
				return
			}
		}
		send(result{done: true})
	}()
	go func() {
		friends, err := s.store.GetFriendNotification(ctx, userID)
		if err != nil {
			send(result{err: status.Error(codes.Internal, "could not get friend notifications")})
			return
		}
		for _, notification := range friends {
			if !send(result{item: &ntfnV1.SubscribeResponse{Item: &ntfnV1.SubscribeResponse_Friend{Friend: friendNotificationItem(notification)}}}) {
				return
			}
		}
		send(result{done: true})
	}()

	for completed := 0; completed < 2; {
		var result result
		select {
		case result = <-results:
		case <-ctx.Done():
			return ctx.Err()
		}
		if result.err != nil {
			return result.err
		}
		if result.done {
			completed++
			continue
		}
		if err := stream.Send(result.item); err != nil {
			return err
		}
	}
	return nil
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
