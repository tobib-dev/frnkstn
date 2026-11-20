package db

import (
	"context"
	"fmt"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type ChannelMessageStore interface {
	PostChannelMessage(context.Context, ChannelMessage) (ChannelMessage, error)
	GetChannelMessage(context.Context, gocql.UUID, gocql.UUID, gocql.UUID) (ChannelMessage, error)
	GetChannelMessages(context.Context, GetChannelMsgsParams) ([]ChannelMessage, error)
	EditChannelMessage(context.Context, EditChannelMsgParams) (ChannelMessage, error)
	DeleteChannelMessage(context.Context, gocql.UUID, gocql.UUID, gocql.UUID) error
}

var _ ChannelMessageStore = (*DB)(nil)

type ChannelMessage struct {
	ServerID   gocql.UUID
	ChannelID  gocql.UUID
	MessageID  gocql.UUID
	AuthorID   gocql.UUID
	AuthorName *string
	Content    string
	Edited     bool
}

type EditChannelMsgParams struct {
	ServerID   gocql.UUID
	ChannelID  gocql.UUID
	MessageID  gocql.UUID
	AuthorID   gocql.UUID
	AuthorName string
	Content    string
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
	Columns: []string{"server_id", "channel_id", "message_id", "author_id", "author_name", "content", "edited"},
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
	query := qb.Select(channelMsgs.Name()).Where(
		qb.Eq("server_id"),
		qb.Eq("channel_id"),
	)
	switch args.SortDirection {
	case "ASC":
		if args.LastMessageID != (gocql.UUID{}) {
			query = query.Where(qb.Gt("message_id"))
		}
		query = query.OrderBy("message_id", qb.ASC)
	case "DESC":
		if args.LastMessageID != (gocql.UUID{}) {
			query = query.Where(qb.Lt("message_id"))
		}
		query = query.OrderBy("message_id", qb.DESC)
	default:
		return nil,
			fmt.Errorf(
				"invalid message sort direction %q",
				args.SortDirection)
	}
	stmt, names := query.Limit(
		uint(args.Limit)).ToCql()
	params := qb.M{
		"server_id":  args.ServerID,
		"channel_id": args.ChannelID,
	}
	if args.LastMessageID != (gocql.UUID{}) {
		params["message_id"] = args.LastMessageID
	}

	if err := db.Session.Query(
		stmt, names).
		WithContext(ctx).
		BindMap(params).
		SelectRelease(&msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

func (db *DB) EditChannelMessage(
	ctx context.Context,
	args EditChannelMsgParams,
) (ChannelMessage, error) {
	applied, err := qb.Update(channelMsgs.Name()).
		Set("content", "edited").
		Where(qb.Eq("server_id"),
			qb.Eq("channel_id"),
			qb.Eq("message_id"),
		).Existing().Query(*db.Session).
		WithContext(ctx).
		BindMap(qb.M{
			"server_id":  args.ServerID,
			"channel_id": args.ChannelID,
			"message_id": args.MessageID,
			"content":    args.Content,
			"edited":     true,
		}).ExecCASRelease()
	if err != nil {
		return ChannelMessage{}, err
	}
	if !applied {
		return ChannelMessage{}, gocql.ErrNotFound
	}
	return ChannelMessage{
		ServerID:   args.ServerID,
		ChannelID:  args.ChannelID,
		MessageID:  args.MessageID,
		AuthorID:   args.AuthorID,
		AuthorName: &args.AuthorName,
		Content:    args.Content,
		Edited:     true,
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
