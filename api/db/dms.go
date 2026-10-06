package db

import (
	"context"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

// For now we foces on just sending and receiving messages
type DMStore interface {
	SendMessage(context.Context, DM) error
	GetMessages(context.Context, gocql.UUID) ([]DM, error)
	GetMessagesByUser(context.Context, gocql.UUID) ([]DM, error)
	GetMessage(context.Context, gocql.UUID, gocql.UUID) (DM, error)
	SubscribeMessages(context.Context, MessageStreamParams) ([]DM, error)
}

type DM struct {
	ID              gocql.UUID `db:"id"`
	MessageID       gocql.UUID `db:"message_id"`
	AuthorID        gocql.UUID `db:"author_id"`
	ReceiverID      gocql.UUID `db:"receiver_id"`
	Content         string     `db:"content"`
	LastMessageTime time.Time  `db:"last_message_time"`
}

var dmMetadata = table.Metadata{
	Name: "direct_messages",
	Columns: []string{
		"id",
		"message_id",
		"author_id",
		"receiver_id",
		"content",
	},
	PartKey: []string{"id"},
	SortKey: []string{"message_id"},
}

var dmTable = table.New(dmMetadata)

var dmByUserMetadata = table.Metadata{
	Name: "direct_messages_by_user",
	Columns: []string{
		"user_id",
		"last_message_time",
		"id",
	},
	PartKey: []string{"user_id"},
	SortKey: []string{"last_message_time"},
}

var dmByUserTable = table.New(dmByUserMetadata)

var dmByMessageMetadata = table.Metadata{
	Name: "direct_messages_by_message",
	Columns: []string{
		"message_id",
		"id",
		"author_id",
		"receiver_id",
		"content",
	},
	PartKey: []string{"message_id"},
	SortKey: []string{"id"},
}

var dmByMessageTable = table.New(dmByMessageMetadata)

func (db *DB) SendMessage(ctx context.Context, dm DM) error {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	query := qb.Insert(dmMetadata.Name).
		Columns("id", "message_id", "author_id", "receiver_id", "content").
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"id":          dm.ID,
		"message_id":  dm.MessageID,
		"author_id":   dm.AuthorID,
		"receiver_id": dm.ReceiverID,
		"content":     dm.Content,
	}); err != nil {
		return err
	}

	if (dm.AuthorID != gocql.UUID{} || dm.ReceiverID != gocql.UUID{}) {
		query := qb.Insert(dmByUserMetadata.Name).
			Columns("user_id", "last_message_time", "id").
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":           dm.AuthorID,
			"last_message_time": dm.LastMessageTime,
			"id":                dm.ID,
		}); err != nil {
			return err
		}
		query = qb.Insert(dmByUserMetadata.Name).
			Columns("user_id", "last_message_time", "id").
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":           dm.ReceiverID,
			"last_message_time": dm.LastMessageTime,
			"id":                dm.ID,
		}); err != nil {
			return err
		}
	}

	if (dm.MessageID != gocql.UUID{}) {
		query := qb.Insert(dmByMessageMetadata.Name).
			Columns("message_id", "id", "author_id", "receiver_id", "content").
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"message_id":  dm.MessageID,
			"id":          dm.ID,
			"author_id":   dm.AuthorID,
			"receiver_id": dm.ReceiverID,
			"content":     dm.Content,
		}); err != nil {
			return err
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return err
	}

	return nil
}

func (db *DB) GetMessage(ctx context.Context, id, messageID gocql.UUID) (DM, error) {
	message := DM{ID: id, MessageID: messageID}
	if err := db.Session.Query(dmByMessageTable.Get()).
		WithContext(ctx).
		BindStruct(message).
		GetRelease(&message); err != nil {
		return DM{}, err
	}
	return message, nil
}

func (db *DB) GetMessagesByUser(ctx context.Context, userID gocql.UUID) ([]DM, error) {
	messages := []DM{}
	if err := db.Session.Query(dmByUserTable.Select()).
		WithContext(ctx).
		BindMap(qb.M{"user_id": userID}).
		SelectRelease(&messages); err != nil {
		return nil, err
	}
	return messages, nil
}

func (db *DB) GetMessages(ctx context.Context, id gocql.UUID) ([]DM, error) {
	messages := []DM{}
	if err := db.Session.Query(dmTable.Select()).
		WithContext(ctx).
		BindMap(qb.M{"id": id}).
		SelectRelease(&messages); err != nil {
		return nil, err
	}
	return messages, nil
}

type MessageStreamParams struct {
	ID              gocql.UUID
	LastMessageTime gocql.UUID
}

func (db *DB) SubscribeMessages(ctx context.Context, args MessageStreamParams) ([]DM, error) {
	query := qb.Select(dmMetadata.Name).
		Columns("id", "message_id", "author_id", "receiver_id", "content").
		Where(qb.Eq("id"), qb.Gt("message_id")).
		Query(*db.Session)

	messages := []DM{}
	if err := query.
		WithContext(ctx).
		BindMap(qb.M{
			"id":         args.ID,
			"message_id": args.LastMessageTime,
		}).
		SelectRelease(&messages); err != nil {
		return nil, err
	}
	return messages, nil
}
