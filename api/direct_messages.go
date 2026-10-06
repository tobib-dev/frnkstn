package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gocql/gocql"
	dmsV1 "github.com/tobib-dev/frnkstn-proto/dms/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type directMessageService struct {
	cfg   *Config
	store db.DMStore
	users db.UserStore
	dmsV1.UnimplementedDMServiceServer
}

func NewDMService(cfg *Config) *directMessageService {
	return &directMessageService{
		cfg:   cfg,
		store: &cfg.db.session,
		users: &cfg.db.session,
	}
}

func (s *directMessageService) SendMessages(
	ctx context.Context,
	req *dmsV1.SendMessagesRequest,
) (*dmsV1.SendMessagesResponse, error) {
	logger := s.cfg.logger.With("location", "SendMessages")
	item := req.GetItem()
	if item == nil {
		logger.Warn("message is required")
		return nil, status.Error(codes.InvalidArgument, "message is required")
	}

	dmID, err := parseID(item.GetDmId(), "DM ID")
	if err != nil {
		logger.Warn("invalid DM ID", "error", err)
		return nil, err
	}
	authorID, err := parseID(item.GetAuthorId(), "author ID")
	if err != nil {
		logger.Warn("invalid author ID", "error", err)
		return nil, err
	}
	receiverID, err := parseID(item.GetReceiverId(), "receiver ID")
	if err != nil {
		logger.Warn("invalid receiver ID", "error", err)
		return nil, err
	}
	if authorID == receiverID {
		return nil, status.Error(codes.InvalidArgument, "author and receiver must be different users")
	}
	content := strings.TrimSpace(item.GetContent())
	if content == "" {
		return nil, status.Error(codes.InvalidArgument, "message content is required")
	}

	logger = logger.With("dm_id", dmID.String(), "author_id", authorID.String(), "receiver_id", receiverID.String())
	if _, err := s.users.GetUserByID(ctx, authorID); err != nil {
		logger.Error("failed to get message author", "error", err)
		return nil, userLookupError(err)
	}
	if _, err := s.users.GetUserByID(ctx, receiverID); err != nil {
		logger.Error("failed to get message receiver", "error", err)
		return nil, userLookupError(err)
	}

	messageID := gocql.TimeUUID()
	message := db.DM{
		ID:              dmID,
		MessageID:       messageID,
		AuthorID:        authorID,
		ReceiverID:      receiverID,
		Content:         content,
		LastMessageTime: messageID.Time(),
	}
	if err := s.store.SendMessage(ctx, message); err != nil {
		logger.Error("failed to send message", "message_id", messageID.String(), "error", err)
		return nil, status.Error(codes.Internal, "could not send message")
	}

	logger.Info("message sent", "message_id", messageID.String())
	return &dmsV1.SendMessagesResponse{Item: messageItem(message)}, nil
}

func (s *directMessageService) GetMessage(
	ctx context.Context,
	req *dmsV1.GetMessageRequest,
) (*dmsV1.GetMessageResponse, error) {
	logger := s.cfg.logger.With("location", "GetMessage")
	item := req.GetItem()
	if item == nil {
		return nil, status.Error(codes.InvalidArgument, "message is required")
	}
	dmID, err := parseID(item.GetDmId(), "DM ID")
	if err != nil {
		return nil, err
	}
	messageID, err := parseID(item.GetMessageId(), "message ID")
	if err != nil {
		return nil, err
	}
	logger = logger.With("dm_id", dmID.String(), "message_id", messageID.String())

	message, err := s.store.GetMessage(ctx, dmID, messageID)
	if errors.Is(err, gocql.ErrNotFound) {
		logger.Info("message not found")
		return nil, status.Error(codes.NotFound, "message not found")
	}
	if err != nil {
		logger.Error("failed to get message", "error", err)
		return nil, status.Error(codes.Internal, "could not get message")
	}

	logger.Info("message retrieved")
	return &dmsV1.GetMessageResponse{Item: messageItem(message)}, nil
}

func (s *directMessageService) GetMessagesByUser(
	ctx context.Context,
	req *dmsV1.GetMessagesByUserRequest,
) (*dmsV1.GetMessagesByUserResponse, error) {
	logger := s.cfg.logger.With("location", "GetMessagesByUser", "user_id", req.GetUserId())
	userID, err := parseID(req.GetUserId(), "user ID")
	if err != nil {
		return nil, err
	}
	if _, err := s.users.GetUserByID(ctx, userID); err != nil {
		logger.Error("failed to get user", "error", err)
		return nil, userLookupError(err)
	}

	metadata, err := s.store.GetMessagesByUser(ctx, userID)
	if err != nil {
		logger.Error("failed to get direct message metadata", "error", err)
		return nil, status.Error(codes.Internal, "could not get direct messages")
	}

	logger.Info("direct message metadata retrieved", "count", len(metadata))
	return &dmsV1.GetMessagesByUserResponse{
		Items: messageMetadataItems(metadata),
	}, nil
}

func (s *directMessageService) GetMessages(
	ctx context.Context,
	req *dmsV1.GetMessagesRequest,
) (*dmsV1.GetMessagesResponse, error) {
	item := req.GetItem()
	if item == nil {
		return nil, status.Error(codes.InvalidArgument, "DM is required")
	}
	dmID, err := parseID(item.GetDmId(), "DM ID")
	if err != nil {
		return nil, err
	}
	logger := s.cfg.logger.With("location", "GetMessages", "dm_id", dmID.String())

	messages, err := s.store.GetMessages(ctx, dmID)
	if err != nil {
		logger.Error("failed to get messages", "error", err)
		return nil, status.Error(codes.Internal, "could not get messages")
	}

	logger.Info("messages retrieved", "count", len(messages))
	return &dmsV1.GetMessagesResponse{Items: messageItems(messages)}, nil
}

// Server side streaming, stream messages if user is online
func (s *directMessageService) SubscribeMessages(
	req *dmsV1.SubscribeMessagesRequest,
	stream grpc.ServerStreamingServer[dmsV1.DirectMessageItem],
) error {
	ctx := stream.Context()

	logger := s.cfg.logger
	lastMsgTime := req.GetLastMsgTime()
	if lastMsgTime == nil {
		logger.Error("Last message time is required", "location", "SubscribeMessages")
		return status.Error(codes.InvalidArgument, "last message time is required")
	}

	if err := lastMsgTime.CheckValid(); err != nil {
		logger.Error("Invalid time format, must be RFC3339Nano", "location", "SubscribeMessages", "error", err)
		return status.Errorf(codes.InvalidArgument, "invalid last_msg_time, must be RFC3339: %v", err)
	}
	dmID, err := gocql.ParseUUID(req.DmId)
	if err != nil {
		logger.Error("Invalid DM ID", "location", "SubscribeMessages", "error", err)
		return status.Errorf(codes.InvalidArgument, "invalid DM ID: %v", err)
	}

	cursor := gocql.MinTimeUUID(lastMsgTime.AsTime())
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	logger.Info("message subscription started", "dm_id", dmID.String(), "location", "SubscribeMessages")

	for {
		msgs, err := s.store.SubscribeMessages(
			ctx,
			db.MessageStreamParams{
				ID:              dmID,
				LastMessageTime: cursor,
			})
		if err != nil && err != gocql.ErrNotFound {
			logger.Error("failed to get subscribed messages", "dm_id", dmID.String(), "location", "SubscribeMessages", "error", err)
			return status.Error(codes.Internal, "failed to subscribe to messages")
		}

		// Scylla returns the newest messages first; stream them oldest first.
		for i := len(msgs) - 1; i >= 0; i-- {
			msg := msgs[i]
			resp := messageItem(msg)
			if err := stream.Send(resp); err != nil {
				logger.Error("failed to stream message", "dm_id", dmID.String(), "message_id", msg.MessageID.String(), "location", "SubscribeMessages", "error", err)
				return err
			}
			cursor = msg.MessageID
			logger.Info("message streamed", "dm_id", dmID.String(), "message_id", msg.MessageID.String(), "location", "SubscribeMessages")
		}

		select {
		case <-ctx.Done():
			logger.Info("message subscription closed", "dm_id", dmID.String(), "location", "SubscribeMessages", "reason", ctx.Err())
			return status.FromContextError(ctx.Err()).Err()
		case <-ticker.C:
		}
	}
}

func messageItem(msg db.DM) *dmsV1.DirectMessageItem {
	return &dmsV1.DirectMessageItem{
		DmId:       msg.ID.String(),
		MessageId:  msg.MessageID.String(),
		AuthorId:   msg.AuthorID.String(),
		ReceiverId: msg.ReceiverID.String(),
		Content:    msg.Content,
	}
}

func messageItems(messages []db.DM) []*dmsV1.DirectMessageItem {
	items := make([]*dmsV1.DirectMessageItem, 0, len(messages))
	for _, message := range messages {
		items = append(items, messageItem(message))
	}
	return items
}

func messageMetadataItems(metadata []db.DMMetadata) []*dmsV1.DirectMessageMetadata {
	items := make([]*dmsV1.DirectMessageMetadata, 0, len(metadata))
	for _, item := range metadata {
		items = append(items, &dmsV1.DirectMessageMetadata{
			DmId:            item.ID.String(),
			LastMessageTime: timestamppb.New(item.LastMessageTime),
		})
	}
	return items
}

func parseID(value, name string) (gocql.UUID, error) {
	id, err := gocql.ParseUUID(value)
	if err != nil {
		return gocql.UUID{}, status.Error(codes.InvalidArgument, "valid "+name+" is required")
	}
	return id, nil
}
