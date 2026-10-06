package main

import (
	"context"
	"time"

	"github.com/gocql/gocql"
	dmsV1 "github.com/tobib-dev/frnkstn-proto/dms/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

type Message struct {
	dmID         gocql.UUID
	messageID    gocql.UUID
	friendshipID gocql.UUID
	authorID     gocql.UUID
	text         string
}

func (s *directMessageService) SendMessages(
	ctx context.Context,
	req *dmsV1.SendMessagesRequest,
) (*dmsV1.SendMessagesResponse, error) {
	return &dmsV1.SendMessagesResponse{}, nil
}

func (s *directMessageService) GetMessage(
	ctx context.Context,
	req *dmsV1.GetMessageRequest,
) (*dmsV1.GetMessageResponse, error) {
	return &dmsV1.GetMessageResponse{}, nil
}

func (s *directMessageService) GetMessagesByUser(
	ctx context.Context,
	req *dmsV1.GetMessagesByUserRequest,
) (*dmsV1.GetMessagesByUserResponse, error) {
	return &dmsV1.GetMessagesByUserResponse{}, nil
}

func (s *directMessageService) GetMessages(
	ctx context.Context,
	req *dmsV1.GetMessagesRequest,
) (*dmsV1.GetMessagesResponse, error) {
	return &dmsV1.GetMessagesResponse{}, nil
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
