package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type NoficationStore interface {
	AddMessageNotification(context.Context, MessageNotification) error
	GetMessageNotification(context.Context, gocql.UUID) ([]MessageNotification, error)
	AddFriendNotification(context.Context, FriendNotification) error
	GetFriendNotification(context.Context, gocql.UUID) ([]FriendNotification, error)
}

type MessageNotification struct {
	NotificationID gocql.UUID
	UserID         gocql.UUID
	MessageID      gocql.UUID
	AuthorID       gocql.UUID
	AuthorName     string
	Content        string
	Seen           bool
}

type FriendNotification struct {
	NotificationID gocql.UUID
	UserID         gocql.UUID
	FriendshipID   gocql.UUID
	FriendID       gocql.UUID
	FriendName     string
	Status         string
	Seen           bool
}

var MsgN10nMeta = table.Metadata{
	Name: "message_notifications",
	Columns: []string{
		"user_id",
		"notification_id",
		"message_id",
		"author_id",
		"author_name",
		"content",
		"seen",
	},
	PartKey: []string{"user_id", "seen"},
	SortKey: []string{"notification_id"},
}

var msgN10nTable = table.New(MsgN10nMeta)

var FriendN10nMeta = table.Metadata{
	Name: "friend_notifications",
	Columns: []string{
		"user_id",
		"notification_id",
		"friendship_id",
		"friend_id",
		"friend_name",
		"status",
		"seen",
	},
	PartKey: []string{"user_id", "seen"},
	SortKey: []string{"notification_id"},
}

var friendN10nTable = table.New(FriendN10nMeta)

func (db *DB) AddMessageNotification(ctx context.Context, msgN10n MessageNotification) error {
	query := db.Session.Query(msgN10nTable.Insert()).
		WithContext(ctx).
		BindStruct(msgN10n)
	if err := query.ExecRelease(); err != nil {
		return err
	}
	return nil
}

func (db *DB) GetMessageNotification(ctx context.Context, userID gocql.UUID) ([]MessageNotification, error) {
	var notifications []MessageNotification
	if err := db.Session.Query(msgN10nTable.Select()).
		WithContext(ctx).
		BindMap(qb.M{"user_id": userID, "seen": false}).
		SelectRelease(&notifications); err != nil {
		return nil, err
	}
	return notifications, nil
}

func (db *DB) AddFriendNotification(ctx context.Context, friendN10n FriendNotification) error {
	query := db.Session.Query(friendN10nTable.Insert()).
		WithContext(ctx).
		BindStruct(friendN10n)
	if err := query.ExecRelease(); err != nil {
		return err
	}
	return nil
}

func (db *DB) GetFriendNotification(ctx context.Context, userID gocql.UUID) ([]FriendNotification, error) {
	var nots []FriendNotification
	if err := db.Session.Query(friendN10nTable.Select()).
		WithContext(ctx).
		BindMap(qb.M{"user_id": userID, "seen": false}).
		SelectRelease(&nots); err != nil {
		return nil, err
	}
	return nots, nil
}
