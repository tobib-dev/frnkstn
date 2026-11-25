package main

import (
	"context"
	"strings"

	"github.com/gocql/gocql"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"github.com/tobib-dev/frnkstn/api/internal/auth"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *serverService) CreateReply(ctx context.Context, req *serversv1.CreateReplyRequest) (*serversv1.CreateReplyResponse, error) {
	return s.createThreadMessage(ctx, req.GetServerId(), req.GetChannelId(), req.GetOriginalMessageId(), req.GetAuthorId(), req.GetContent(), req.GetTaggedUserIds())
}

func (s *serverService) ReplyThread(ctx context.Context, req *serversv1.ReplyThreadRequest) (*serversv1.ReplyThreadResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionWriteMessages)
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
	threadID, err := parseID(req.GetThreadId(), "thread ID")
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
	thread, err := s.threads.GetThread(ctx, serverID, channelID, threadID)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not get thread")
	}
	if len(thread) == 0 {
		return nil, status.Error(codes.NotFound, "thread not found")
	}
	author, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, userLookupError(err)
	}
	tags, err := parseTaggedUserIDs(req.GetTaggedUserIds())
	if err != nil {
		return nil, err
	}
	item, err := s.threads.ReplyThread(ctx, db.ThreadMessage{ServerID: serverID, ChannelID: channelID, ThreadID: threadID, MessageID: gocql.TimeUUID(), AuthorID: userID, AuthorName: author.Username, Content: content, Tags: tags})
	if err != nil {
		return nil, s.serverMessageError("reply to thread", err)
	}
	s.publishThreadTags(ctx, item)
	return &serversv1.ReplyThreadResponse{Item: serverThreadItem(item)}, nil
}

func (s *serverService) GetThread(ctx context.Context, req *serversv1.GetThreadRequest) (*serversv1.GetThreadResponse, error) {
	_, err := authenticatedUserID(ctx, s.cfg, auth.PermissionReadMessages)
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
	threadID, err := parseID(req.GetThreadId(), "thread ID")
	if err != nil {
		return nil, err
	}
	items, err := s.threads.GetThread(ctx, serverID, channelID, threadID)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not get thread")
	}
	if len(items) == 0 {
		return nil, status.Error(codes.NotFound, "thread not found")
	}
	response := &serversv1.GetThreadResponse{Items: make([]*serversv1.ThreadItem, len(items))}
	for i, item := range items {
		response.Items[i] = serverThreadItem(item)
	}
	return response, nil
}

func (s *serverService) GetThreads(ctx context.Context, req *serversv1.GetThreadsRequest) (*serversv1.GetThreadsResponse, error) {
	if _, err := authenticatedUserID(ctx, s.cfg, auth.PermissionReadMessages); err != nil {
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
	threads, err := s.threads.GetThreads(ctx, serverID, channelID)
	if err != nil {
		return nil, status.Error(codes.Internal, "could not get threads")
	}
	response := &serversv1.GetThreadsResponse{Items: make([]*serversv1.ThreadItem, len(threads))}
	for i, thread := range threads {
		response.Items[i] = serverThreadItem(thread)
	}
	return response, nil
}

func (s *serverService) createThreadMessage(
	ctx context.Context, serverIDValue,
	channelIDValue, originalMessageIDValue,
	authorIDValue, contentValue string,
	tagValues []string,
) (*serversv1.CreateReplyResponse, error) {
	userID, err := authenticatedUserID(ctx, s.cfg, auth.PermissionWriteMessages)
	if err != nil {
		return nil, err
	}
	serverID, err := s.authorizeChannels(ctx, serverIDValue, false)
	if err != nil {
		return nil, err
	}
	channelID, err := parseID(channelIDValue, "channel ID")
	if err != nil {
		return nil, err
	}
	originalMessageID, err := parseID(originalMessageIDValue, "original message ID")
	if err != nil {
		return nil, err
	}
	authorID, err := parseID(authorIDValue, "author ID")
	if err != nil {
		return nil, err
	}
	if authorID != userID {
		return nil, status.Error(codes.PermissionDenied, "message author does not match bearer token")
	}
	content := strings.TrimSpace(contentValue)
	if content == "" {
		return nil, status.Error(codes.InvalidArgument, "message content is required")
	}
	if _, err := s.channels.GetChannel(ctx, serverID, channelID); err != nil {
		return nil, s.channelError("get channel", err)
	}
	author, err := s.users.GetUserByID(ctx, userID)
	if err != nil {
		return nil, userLookupError(err)
	}
	tags, err := parseTaggedUserIDs(tagValues)
	if err != nil {
		return nil, err
	}
	thread, err := s.threads.CreateReply(ctx, originalMessageID, db.ThreadMessage{ServerID: serverID, ChannelID: channelID, ThreadID: originalMessageID, MessageID: gocql.TimeUUID(), AuthorID: userID, AuthorName: author.Username, Content: content, Tags: tags})
	if err != nil {
		return nil, s.serverMessageError("create thread reply", err)
	}
	s.publishThreadTags(ctx, thread)
	return &serversv1.CreateReplyResponse{Item: serverThreadItem(thread)}, nil
}

func serverThreadItem(item db.ThreadMessage) *serversv1.ThreadItem {
	ids := make([]string, len(item.Tags))
	for i, id := range item.Tags {
		ids[i] = id.String()
	}
	return &serversv1.ThreadItem{TaggedUserIds: ids, ServerId: item.ServerID.String(), ChannelId: item.ChannelID.String(), ThreadId: item.ThreadID.String(), MessageId: item.MessageID.String(), AuthorId: item.AuthorID.String(), AuthorName: item.AuthorName, Content: item.Content, Edited: item.Edited}
}

func (s *serverService) publishThreadTags(ctx context.Context, item db.ThreadMessage) {
	users := make([]db.TaggedUser, len(item.Tags))
	for i, id := range item.Tags {
		users[i] = db.TaggedUser{ID: id}
	}
	s.publishTaggedMessageNotifications(ctx, db.ChannelMessageResponse{
		ServerID: item.ServerID, ChannelID: item.ChannelID, MessageID: item.MessageID,
		AuthorID: item.AuthorID, Content: item.Content, TaggedUsers: users,
	}, s.cfg.logger.With("operation", "thread notification", "thread_id", item.ThreadID))
}
