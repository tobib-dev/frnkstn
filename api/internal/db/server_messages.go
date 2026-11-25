package db

import (
	"context"
	"fmt"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type ChannelMessageStore interface {
	PostChannelMessage(context.Context, ChannelMessage) (ChannelMessageResponse, error)
	GetChannelMessage(context.Context, gocql.UUID, gocql.UUID, gocql.UUID) (ChannelMessageResponse, error)
	GetChannelMessages(context.Context, GetChannelMsgsParams) ([]ChannelMessageResponse, error)
	EditChannelMessage(context.Context, EditChannelMsgParams) (ChannelMessageResponse, error)
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
	Tags       []gocql.UUID `db:"tags"`
}

type TaggedUser struct {
	ID   gocql.UUID
	Name string
}

type ChannelMessageResponse struct {
	ServerID    gocql.UUID
	ChannelID   gocql.UUID
	MessageID   gocql.UUID
	AuthorID    gocql.UUID
	AuthorName  *string
	Content     string
	Edited      bool
	TaggedUsers []TaggedUser
}

type EditChannelMsgParams struct {
	ServerID   gocql.UUID
	ChannelID  gocql.UUID
	MessageID  gocql.UUID
	AuthorID   gocql.UUID
	AuthorName string
	Content    string
	Tags       []gocql.UUID `db:"tags"`
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
	Columns: []string{"server_id", "channel_id", "message_id", "author_id", "author_name", "content", "edited", "tags"},
	PartKey: []string{"server_id", "channel_id"},
	SortKey: []string{"message_id"},
})

func (db *DB) PostChannelMessage(
	ctx context.Context,
	msg ChannelMessage,
) (ChannelMessageResponse, error) {
	if err := db.enforceSlowMode(ctx, msg.ServerID, msg.ChannelID, msg.AuthorID); err != nil {
		return ChannelMessageResponse{}, err
	}
	if err := db.Session.Query(
		channelMsgs.Insert(),
	).WithContext(ctx).
		BindStruct(&msg).
		ExecRelease(); err != nil {
		return ChannelMessageResponse{}, err
	}

	resp, err := messageResponse(ctx, db, msg)
	if err != nil {
		return ChannelMessageResponse{}, err
	}
	return resp, nil
}

func (db *DB) GetChannelMessage(
	ctx context.Context,
	serverID, channelID,
	messageID gocql.UUID,
) (ChannelMessageResponse, error) {
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
		return ChannelMessageResponse{}, err
	}

	resp, err := messageResponse(ctx, db, msg)
	if err != nil {
		return ChannelMessageResponse{}, err
	}
	return resp, nil
}

// Paginate get messages - pass pagination arguments
func (db *DB) GetChannelMessages(
	ctx context.Context,
	args GetChannelMsgsParams,
) ([]ChannelMessageResponse, error) {
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

	resp := make([]ChannelMessageResponse, len(msgs))
	for i, msg := range msgs {
		msgRes, err := messageResponse(ctx, db, msg)
		if err != nil {
			return []ChannelMessageResponse{}, err
		}
		resp[i] = msgRes
	}
	return resp, nil
}

func (db *DB) EditChannelMessage(
	ctx context.Context,
	args EditChannelMsgParams,
) (ChannelMessageResponse, error) {
	if err := db.enforceSlowMode(ctx, args.ServerID, args.ChannelID, args.AuthorID); err != nil {
		return ChannelMessageResponse{}, err
	}
	applied, err := qb.Update(channelMsgs.Name()).
		Set("content", "edited", "tags").
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
			"tags":       args.Tags,
		}).ExecCASRelease()
	if err != nil {
		return ChannelMessageResponse{}, err
	}
	if !applied {
		return ChannelMessageResponse{}, gocql.ErrNotFound
	}

	resp, err := messageResponse(
		ctx, db, ChannelMessage{
			ServerID:   args.ServerID,
			ChannelID:  args.ChannelID,
			MessageID:  args.MessageID,
			AuthorID:   args.AuthorID,
			AuthorName: &args.AuthorName,
			Content:    args.Content,
			Edited:     true,
			Tags:       args.Tags,
		},
	)
	if err != nil {
		return ChannelMessageResponse{}, err
	}
	return resp, nil
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

func resolveTaggedUsers(
	ctx context.Context,
	tags []gocql.UUID,
	users UserStore,
) ([]TaggedUser, error) {
	taggedUsers := make([]TaggedUser, len(tags))
	for i, userID := range tags {
		user, err := users.GetUserByID(ctx, userID)
		if err != nil {
			return []TaggedUser{},
				fmt.Errorf(
					"error resolving tagged users: %w",
					err)
		}
		taggedUsers[i] = TaggedUser{
			ID:   user.ID,
			Name: user.Username,
		}
	}
	return taggedUsers, nil
}

func messageResponse(
	ctx context.Context,
	users UserStore,
	msg ChannelMessage,
) (ChannelMessageResponse, error) {
	resp := ChannelMessageResponse{
		ServerID:   msg.ServerID,
		ChannelID:  msg.ChannelID,
		MessageID:  msg.MessageID,
		AuthorID:   msg.AuthorID,
		AuthorName: msg.AuthorName,
		Content:    msg.Content,
		Edited:     msg.Edited,
	}

	if len(msg.Tags) != 0 {
		taggedUsers, err := resolveTaggedUsers(
			ctx, msg.Tags, users,
		)
		if err != nil {
			return ChannelMessageResponse{}, err
		}
		resp.TaggedUsers = taggedUsers
	}
	return resp, nil
}
