package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type ChannelMessages interface {
	PostChannelMessage(context.Context, ChannelMessage) (ChannelMessage, error)
	GetChannelMessage(context.Context, gocql.UUID, gocql.UUID, gocql.UUID) (ChannelMessage, error)
	GetChannelMessages(context.Context, GetChannelMsgsParams) ([]ChannelMessage, error)
	EditChannelMessage(context.Context, EditChannelMsgParams) (ChannelMessage, error)
	DeleteChannelMessage(context.Context, gocql.UUID, gocql.UUID, gocql.UUID) error
}

type ChannelMessage struct {
	ServerID  gocql.UUID
	ChannelID gocql.UUID
	MessageID gocql.UUID
	AuthorID  gocql.UUID
	Content   string
	Edited    bool
}

type EditChannelMsgParams struct {
	ServerID  gocql.UUID
	ChannelID gocql.UUID
	MessageID gocql.UUID
	AuthorID  gocql.UUID
	Content   string
}

type GetChannelMsgsParams struct {
	ServerID      gocql.UUID
	ChannelID     gocql.UUID
	LastMessageID gocql.UUID
	Limit         int
	SortDirection string
}

var channelMsgs = table.New(table.Metadata{
	Name:    "channel_messages",
	Columns: []string{"server_id", "channel_id", "message_id", "author_id", "content", "edited"},
	PartKey: []string{"server_id", "channel_id"},
	SortKey: []string{"message_id"},
})

func (db *DB) PostChannelMessage(
	ctx context.Context,
	msg ChannelMessage,
) (ChannelMessage, error) {
	if err := db.Session.Query(
		channelMsgs.Insert(),
	).WithContext(ctx).
		BindStruct(&msg).
		ExecRelease(); err != nil {
		return ChannelMessage{}, err
	}
	return msg, nil
}

func (db *DB) GetChannelMessage(
	ctx context.Context,
	serverID, channelID,
	messageID gocql.UUID,
) (ChannelMessage, error) {
	msg := ChannelMessage{
		ServerID:  serverID,
		ChannelID: channelID,
		MessageID: messageID,
	}

	if err := db.Session.Query(
		channelMsgs.Get(),
	).
		WithContext(ctx).
		BindStruct(&msg).
		GetRelease(&msg); err != nil {
		return ChannelMessage{}, err
	}
	return msg, nil
}

// Paginate get messages - pass pagination arguments
func (db *DB) GetChannelMessages(
	ctx context.Context,
	args GetChannelMsgsParams,
) ([]ChannelMessage, error) {
	msgs := []ChannelMessage{}
	var (
		stmt  string
		names []string
	)

	switch args.SortDirection {
	case "ASC":
		stmt, names = qb.Select(channelMsgs.Name()).
			Where(
				qb.Eq("server_id"),
				qb.Eq("channel_id"),
				qb.Gt("message_id")).
			OrderBy("message_id", qb.ASC).
			Limit(uint(args.Limit)).
			ToCql()
	case "DESC":
		stmt, names = qb.Select(channelMsgs.Name()).
			Where(
				qb.Eq("server_id"),
				qb.Eq("channel_id"),
				qb.Lt("message_id")).
			OrderBy("message_id", qb.DESC).
			Limit(uint(args.Limit)).
			ToCql()
	}

	if err := db.Session.Query(
		stmt, names).
		WithContext(ctx).
		BindMap(qb.M{
			"server_id":  args.ServerID,
			"channel_id": args.ChannelID,
			"message_id": args.LastMessageID,
		}).
		SelectRelease(&msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (db *DB) EditChannelMessage(
	ctx context.Context,
	args EditChannelMsgParams,
) (ChannelMessage, error) {
	stmt, names := qb.Update(channelMsgs.Name()).
		Set("content", "edited").
		Where(qb.Eq("server_id"),
			qb.Eq("channel_id"),
			qb.Eq("message_id"),
		).ToCql()

	if err := db.Session.Query(stmt, names).
		WithContext(ctx).
		BindMap(qb.M{
			"server_id":  args.ServerID,
			"channel_id": args.ChannelID,
			"message_id": args.MessageID,
			"content":    args.Content,
			"edited":     true,
		}).ExecRelease(); err != nil {
		return ChannelMessage{}, err
	}
	return ChannelMessage{
		ServerID:  args.ServerID,
		ChannelID: args.ChannelID,
		MessageID: args.MessageID,
		AuthorID:  args.AuthorID,
		Content:   args.Content,
		Edited:    true,
	}, nil
}

func (db *DB) DeleteChannelMessage(
	ctx context.Context,
	serverID, channelID,
	messageID gocql.UUID,
) error {
	stmt, names := qb.Delete(
		channelMsgs.Name(),
	).Where(
		qb.Eq("server_id"),
		qb.Eq("channel_id"),
		qb.Eq("message_id"),
	).ToCql()

	if err := db.Session.Query(stmt, names).
		WithContext(ctx).
		BindMap(qb.M{
			"server_id":  serverID,
			"channel_id": channelID,
			"message_id": messageID,
		}).ExecRelease(); err != nil {
		return err
	}

	return nil
}
