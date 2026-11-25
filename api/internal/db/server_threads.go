package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type ThreadStore interface {
	CreateReply(context.Context, gocql.UUID, ThreadMessage) (ThreadMessage, error)
	ReplyThread(context.Context, ThreadMessage) (ThreadMessage, error)
	GetThread(context.Context, gocql.UUID, gocql.UUID, gocql.UUID) ([]ThreadMessage, error)
	GetThreads(context.Context, gocql.UUID, gocql.UUID) ([]ThreadMessage, error)
}

type ThreadMessage struct {
	ServerID   gocql.UUID
	ChannelID  gocql.UUID
	ThreadID   gocql.UUID
	MessageID  gocql.UUID
	AuthorID   gocql.UUID
	AuthorName string
	Content    string
	Edited     bool
	Tags       []gocql.UUID
}

var threadTable = table.New(table.Metadata{
	Name:    "server_channel_threads",
	Columns: []string{"server_id", "channel_id", "thread_id", "message_id", "author_id", "author_name", "content", "edited", "tags"},
	PartKey: []string{"server_id", "channel_id"},
	SortKey: []string{"thread_id", "message_id"},
})

var threadByChannelTable = table.New(table.Metadata{
	Name:    "threads_by_channel",
	Columns: []string{"server_id", "channel_id", "thread_id", "message_id", "author_id", "author_name", "content", "edited", "tags"},
	PartKey: []string{"server_id"},
	SortKey: []string{"channel_id", "thread_id", "message_id"},
})

/*
 * create reply creates a thread, batch writes to
 * thread and channel message table
 */
func (db *DB) CreateReply(
	ctx context.Context,
	originalMessageID gocql.UUID,
	threadMsg ThreadMessage,
) (ThreadMessage, error) {
	msg := ChannelMessage{
		ServerID:   threadMsg.ServerID,
		ChannelID:  threadMsg.ChannelID,
		MessageID:  threadMsg.MessageID,
		AuthorID:   threadMsg.AuthorID,
		AuthorName: &threadMsg.AuthorName,
		Content:    threadMsg.Content,
		Edited:     threadMsg.Edited,
		Tags:       threadMsg.Tags,
	}
	ogMsg := ChannelMessage{
		ServerID:  threadMsg.ServerID,
		ChannelID: threadMsg.ChannelID,
		MessageID: originalMessageID,
	}
	if err := db.Session.Query(
		channelMsgs.Get(),
	).WithContext(ctx).
		BindStruct(ogMsg).
		GetRelease(&ogMsg); err != nil {
		return ThreadMessage{}, err
	}
	var originalAuthorName string
	if ogMsg.AuthorName != nil {
		originalAuthorName = *ogMsg.AuthorName
	}
	ogThreadMsg := ThreadMessage{
		ServerID: threadMsg.ServerID, ChannelID: threadMsg.ChannelID,
		ThreadID: threadMsg.ThreadID, MessageID: originalMessageID,
		AuthorID: ogMsg.AuthorID, AuthorName: originalAuthorName,
		Content: ogMsg.Content, Edited: ogMsg.Edited, Tags: ogMsg.Tags,
	}
	if err := db.enforceSlowMode(ctx, threadMsg.ServerID, threadMsg.ChannelID, threadMsg.AuthorID); err != nil {
		return ThreadMessage{}, err
	}

	b := db.Session.ContextBatch(ctx,
		gocql.LoggedBatch)
	if err := b.BindStruct(
		channelMsgs.InsertQuery(
			*db.Session), msg,
	); err != nil {
		return ThreadMessage{}, err
	}

	if err := b.BindStruct(
		threadTable.InsertQuery(
			*db.Session), ogThreadMsg,
	); err != nil {
		return ThreadMessage{}, err
	}

	if err := b.BindStruct(
		threadTable.InsertQuery(
			*db.Session), threadMsg,
	); err != nil {
		return ThreadMessage{}, err
	}

	if err := b.BindStruct(
		threadByChannelTable.InsertQuery(
			*db.Session), ogThreadMsg,
	); err != nil {
		return ThreadMessage{}, err
	}

	if err := db.Session.ExecuteBatch(b); err != nil {
		return ThreadMessage{}, err
	}
	return threadMsg, nil
}

func (db *DB) ReplyThread(ctx context.Context, threadMsg ThreadMessage) (ThreadMessage, error) {
	if err := db.enforceSlowMode(ctx, threadMsg.ServerID, threadMsg.ChannelID, threadMsg.AuthorID); err != nil {
		return ThreadMessage{}, err
	}
	message := ChannelMessage{
		ServerID: threadMsg.ServerID, ChannelID: threadMsg.ChannelID,
		MessageID: threadMsg.MessageID, AuthorID: threadMsg.AuthorID,
		AuthorName: &threadMsg.AuthorName, Content: threadMsg.Content,
		Edited: threadMsg.Edited, Tags: threadMsg.Tags,
	}
	b := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	if err := b.BindStruct(channelMsgs.InsertQuery(*db.Session), message); err != nil {
		return ThreadMessage{}, err
	}
	if err := b.BindStruct(threadTable.InsertQuery(*db.Session), threadMsg); err != nil {
		return ThreadMessage{}, err
	}
	if err := db.Session.ExecuteBatch(b); err != nil {
		return ThreadMessage{}, err
	}
	return threadMsg, nil
}

func (db *DB) GetThread(
	ctx context.Context, serverID,
	channelID, threadID gocql.UUID,
) ([]ThreadMessage, error) {
	var thread []ThreadMessage
	if err := db.Session.Query(
		qb.Select(threadTable.Name()).Where(
			qb.Eq("server_id"),
			qb.Eq("channel_id"),
			qb.Eq("thread_id"),
		).OrderBy("message_id", qb.DESC).ToCql(),
	).WithContext(ctx).
		BindMap(qb.M{
			"server_id":  serverID,
			"channel_id": channelID,
			"thread_id":  threadID,
		}).
		SelectRelease(&thread); err != nil {
		return []ThreadMessage{}, err
	}
	return thread, nil
}

func (db *DB) GetThreads(
	ctx context.Context,
	serverID, channelID gocql.UUID,
) ([]ThreadMessage, error) {
	var threadMsgs []ThreadMessage
	if err := db.Session.Query(
		qb.Select(threadByChannelTable.Name()).Where(
			qb.Eq("server_id"),
			qb.Eq("channel_id"),
		).OrderBy("thread_id", qb.DESC).OrderBy("message_id", qb.DESC).ToCql(),
	).WithContext(ctx).
		BindMap(qb.M{
			"server_id":  serverID,
			"channel_id": channelID,
		}).SelectRelease(&threadMsgs); err != nil {
		return []ThreadMessage{}, err
	}
	return threadMsgs, nil
}
