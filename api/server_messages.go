package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/gocql/gocql"
	ntfnV1 "github.com/tobib-dev/frnkstn-proto/notifications/v1"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/internal/auth"
	"github.com/tobib-dev/frnkstn/api/internal/data"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultServerMessagePageSize = 50

func (s *serverService) PostMessage(
	ctx context.Context,
	req *serversv1.PostMessageRequest,
) (response *serversv1.PostMessageResponse, rpcErr error) {
	logger := s.cfg.logger.With("operation", "PostMessage", "server_id", req.GetServerId(), "channel_id", req.GetChannelId())
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionWriteMessages)
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	serverID, err := s.authorizeChannels(ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(req.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	authorID, err := parseID(req.GetAuthorId(), "author ID")
	if err != nil {
		return nil, err
	}
	if authorID != userID {
		return nil, status.Error(codes.PermissionDenied, "message author does not match bearer token")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, status.Error(codes.InvalidArgument, "message content is required")
	}
	tags, err := parseTaggedUserIDs(req.GetTaggedUserIds())
	if err != nil {
		return nil, err
	}
	if _, err := s.channels.GetChannel(ctx, serverID, channelID); err != nil {
		return nil, s.channelError("get channel", err)
	}
	author, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		logger.Error("could not get message author", "error", err)
		return nil, userLookupError(err)
	}
	message, err := s.messages.PostChannelMessage(ctx, db.ChannelMessage{
		ServerID: serverID, ChannelID: channelID, MessageID: gocql.TimeUUID(),
		AuthorID: userID, AuthorName: &author.Username, Content: content, Tags: tags,
	})
	if err != nil {
		return nil, s.serverMessageError("post message", err)
	}
	logger.Info("server message posted", "message_id", message.MessageID)
	s.publishTaggedMessageNotifications(ctx, message, logger)
	return &serversv1.PostMessageResponse{Item: serverMessageItem(message)}, nil
}

func (s *serverService) GetMessage(
	ctx context.Context,
	req *serversv1.GetMessageRequest,
) (response *serversv1.GetMessageResponse, rpcErr error) {
	logger := s.cfg.logger.With("operation", "GetMessage", "server_id", req.GetServerId(), "channel_id", req.GetChannelId(), "message_id", req.GetMessageId())
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionReadMessages)
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	serverID, err := s.authorizeChannels(ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(req.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	messageID, err := parseID(req.GetMessageId(), "message ID")
	if err != nil {
		return nil, err
	}
	message, err := s.messages.GetChannelMessage(ctx, serverID, channelID, messageID)
	if err != nil {
		return nil, s.serverMessageError("get message", err)
	}
	logger.Info("server message retrieved")
	return &serversv1.GetMessageResponse{Item: serverMessageItem(message)}, nil
}

func (s *serverService) GetMessages(
	ctx context.Context,
	req *serversv1.GetMessagesRequest,
) (response *serversv1.GetMessagesResponse, rpcErr error) {
	logger := s.cfg.logger.With("operation", "GetMessages", "server_id", req.GetServerId(), "channel_id", req.GetChannelId())
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionReadMessages)
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
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
	pageSize := int(req.GetPageSize())
	if pageSize == 0 {
		pageSize = defaultServerMessagePageSize
	}
	filters := data.Filters{Page: 1, PageSize: pageSize, Sort: "-message_id", SortSafeList: []string{"-message_id"}}
	if err := data.ValidateFilters(filters); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	var lastMessageID gocql.UUID
	if req.GetPageToken() != "" {
		lastMessageID, err = parseID(req.GetPageToken(), "page token")
		if err != nil {
			return nil, err
		}
	}
	messages, err := s.messages.GetChannelMessages(ctx, db.GetChannelMsgsParams{
		ServerID: serverID, ChannelID: channelID, LastMessageID: lastMessageID,
		Limit: int(filters.Limit() + 1), SortDirection: filters.SortDirection(),
	})
	if err != nil {
		return nil, s.serverMessageError("get messages", err)
	}
	var nextPageToken string
	if len(messages) > pageSize {
		nextPageToken = messages[pageSize-1].MessageID.String()
		messages = messages[:pageSize]
	}
	items := serverMessageItems(messages)
	logger.Info("server messages retrieved", "count", len(items))
	return &serversv1.GetMessagesResponse{Items: items, NextPageToken: nextPageToken}, nil
}

func (s *serverService) EditMessage(
	ctx context.Context,
	req *serversv1.EditMessageRequest,
) (response *serversv1.EditMessageResponse, rpcErr error) {
	logger := s.cfg.logger.With("operation", "EditMessage", "server_id", req.GetServerId(), "channel_id", req.GetChannelId(), "message_id", req.GetMessageId())
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionWriteMessages)
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	serverID, err := s.authorizeChannels(ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(req.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	messageID, err := parseID(req.GetMessageId(), "message ID")
	if err != nil {
		return nil, err
	}
	message, err := s.messages.GetChannelMessage(ctx, serverID, channelID, messageID)
	if err != nil {
		return nil, s.serverMessageError("get message", err)
	}
	if message.AuthorID != userID {
		return nil, status.Error(codes.PermissionDenied, "only the message author can edit it")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, status.Error(codes.InvalidArgument, "message content is required")
	}
	tags, err := parseTaggedUserIDs(req.GetTaggedUserIds())
	if err != nil {
		return nil, err
	}
	message, err = s.messages.EditChannelMessage(ctx, db.EditChannelMsgParams{
		ServerID: serverID, ChannelID: channelID, MessageID: messageID,
		AuthorID: userID, AuthorName: serverMessageAuthorName(message), Content: content,
		Tags: tags,
	})
	if err != nil {
		return nil, s.serverMessageError("edit message", err)
	}
	logger.Info("server message edited")
	s.publishTaggedMessageNotifications(ctx, message, logger)
	return &serversv1.EditMessageResponse{Item: serverMessageItem(message)}, nil
}

func (s *serverService) DeleteMessage(
	ctx context.Context,
	req *serversv1.DeleteMessageRequest,
) (response *serversv1.DeleteMessageResponse, rpcErr error) {
	logger := s.cfg.logger.With("operation", "DeleteMessage", "server_id", req.GetServerId(), "channel_id", req.GetChannelId(), "message_id", req.GetMessageId())
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionWriteMessages)
	if err != nil {
		return nil, err
	}
	logger = logger.With("user_id", userID)
	serverID, err := s.authorizeChannels(ctx, req.GetServerId(), false)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(req.GetChannelId(), "channel ID")
	if err != nil {
		return nil, err
	}
	messageID, err := parseID(req.GetMessageId(), "message ID")
	if err != nil {
		return nil, err
	}
	message, err := s.messages.GetChannelMessage(ctx, serverID, channelID, messageID)
	if err != nil {
		return nil, s.serverMessageError("get message", err)
	}
	if message.AuthorID != userID {
		return nil, status.Error(codes.PermissionDenied, "only the message author can delete it")
	}
	if err := s.messages.DeleteChannelMessage(ctx, serverID, channelID, messageID); err != nil {
		return nil, s.serverMessageError("delete message", err)
	}
	logger.Info("server message deleted")
	return &serversv1.DeleteMessageResponse{}, nil
}

func (s *serverService) serverMessageError(operation string, err error) error {
	if errors.Is(err, db.ErrSlowMode) {
		return status.Error(codes.ResourceExhausted, "Channel slowmode is active. Wait before writing again.")
	}
	if errors.Is(err, gocql.ErrNotFound) {
		return status.Error(codes.NotFound, "message not found")
	}
	s.cfg.logger.Error("server message operation failed", "operation", operation, "error", err)
	return status.Error(codes.Internal, "could not "+operation)
}

func (s *serverService) publishTaggedMessageNotifications(ctx context.Context, message db.ChannelMessageResponse, logger *slog.Logger) {
	for _, user := range message.TaggedUsers {
		item := &ntfnV1.ServerMessageN10NItem{
			Id: gocql.TimeUUID().String(), RecipientId: user.ID.String(),
			ServerId: message.ServerID.String(), ChannelId: message.ChannelID.String(),
			MessageId: message.MessageID.String(), Content: message.Content,
		}
		s.n10n.publishNotification(ctx, item.RecipientId, &ntfnV1.SubscribeResponse{
			Item: &ntfnV1.SubscribeResponse_ServerMessage{ServerMessage: item},
		}, logger, func() {
			id, err := parseID(item.Id, "notification ID")
			if err == nil {
				err = s.n10n.store.AddServerMessageNotification(ctx, db.ServerMessageNotification{
					NotificationID: id, UserID: user.ID, ServerID: message.ServerID,
					ChannelID: message.ChannelID, MessageID: message.MessageID, Content: message.Content,
				})
			}
			if err != nil {
				logger.Error("failed to store server message notification", "error", err)
			}
		})
	}
}

func serverMessageItem(message db.ChannelMessageResponse) *serversv1.MessageItem {
	return serverMessageItems([]db.ChannelMessageResponse{message})[0]
}

func serverMessageItems(messages []db.ChannelMessageResponse) []*serversv1.MessageItem {
	items := make([]*serversv1.MessageItem, len(messages))
	for i, message := range messages {
		items[i] = &serversv1.MessageItem{
			ServerId: message.ServerID.String(), ChannelId: message.ChannelID.String(),
			MessageId: message.MessageID.String(), AuthorId: message.AuthorID.String(),
			Content: message.Content, Edited: message.Edited, AuthorUsername: serverMessageAuthorName(message),
			TaggedUsers: taggedUserItems(message.TaggedUsers),
		}
	}
	return items
}

func serverMessageAuthorName(message db.ChannelMessageResponse) string {
	if message.AuthorName == nil {
		return ""
	}
	return *message.AuthorName
}

func taggedUserItems(users []db.TaggedUser) []*serversv1.TaggedUser {
	items := make([]*serversv1.TaggedUser, len(users))
	for i, user := range users {
		items[i] = &serversv1.TaggedUser{Id: user.ID.String(), Name: user.Name}
	}
	return items
}

func parseTaggedUserIDs(values []string) ([]gocql.UUID, error) {
	ids := make([]gocql.UUID, len(values))
	for i, value := range values {
		id, err := parseID(value, "tagged user ID")
		if err != nil {
			return nil, err
		}
		ids[i] = id
	}
	return ids, nil
}

func (s *serverService) SubscribeChannelMessages(
	req *serversv1.SubscribeChannelMessagesRequest,
	stream grpc.ServerStreamingServer[serversv1.MessageItem],
) (rpcErr error) {
	ctx := stream.Context()
	logger := s.cfg.logger.With(
		"operation", "SubscribeChannelMessages",
		"server_id", req.GetServerId(),
		"channel_id", req.GetChannelId(),
	)
	defer func() { logMemberRPCError(logger, rpcErr) }()
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionReadMessages)
	if err != nil {
		return err
	}
	logger = logger.With("user_id", userID)
	serverID, err := s.authorizeChannels(ctx, req.GetServerId(), false)
	if err != nil {
		return err
	}
	channelID, err := parseID(req.GetChannelId(), "channel ID")
	if err != nil {
		return err
	}
	if _, err := s.channels.GetChannel(ctx, serverID, channelID); err != nil {
		return s.channelError("get channel", err)
	}
	cursor := gocql.TimeUUID()
	if req.GetLastMessageId() != "" {
		cursor, err = parseID(req.GetLastMessageId(), "last message ID")
		if err != nil {
			return err
		}
		if cursor.Version() != 1 {
			return status.Error(codes.InvalidArgument, "last message ID must be a time UUID")
		}
	}
	logger = logger.With("last_message_id", cursor)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	logger.Info("server channel message subscription started")
	for {
		messages, err := s.messages.GetChannelMessages(ctx, db.GetChannelMsgsParams{
			ServerID: serverID, ChannelID: channelID, LastMessageID: cursor,
			Limit: 100, SortDirection: "ASC",
		})
		if err != nil {
			if ctx.Err() != nil {
				return status.FromContextError(ctx.Err()).Err()
			}
			s.cfg.logger.Error("failed to read subscribed channel messages", "error", err)
			return status.Error(codes.Internal, "could not subscribe to channel messages")
		}
		items := serverMessageItems(messages)
		for i, message := range messages {
			if err := stream.Send(items[i]); err != nil {
				return err
			}
			cursor = message.MessageID
			logger.Info("server channel message streamed", "message_id", cursor)
		}
		if len(messages) == 100 {
			continue
		}
		select {
		case <-ctx.Done():
			logger.Info("server channel message subscription closed", "reason", ctx.Err())
			return status.FromContextError(ctx.Err()).Err()
		case <-ticker.C:
		}
	}
}
