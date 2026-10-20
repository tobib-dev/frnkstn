package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type NotificationStore interface {
	AddMessageNotification(context.Context, MessageNotification) error
	GetMessageNotification(context.Context, gocql.UUID) ([]MessageNotification, error)
	UpdateMessageNotification(context.Context, gocql.UUID, gocql.UUID) error
	DeleteMessageNotifications(context.Context, gocql.UUID, []gocql.UUID) error
	AddFriendNotification(context.Context, FriendNotification) error
	GetFriendNotification(context.Context, gocql.UUID) ([]FriendNotification, error)
	UpdateFriendNotification(context.Context, gocql.UUID, gocql.UUID) error
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

func (db *DB) UpdateMessageNotification(ctx context.Context, userID, notificationID gocql.UUID) error {
	notification := MessageNotification{UserID: userID, NotificationID: notificationID, Seen: false}
	return db.Session.Query(msgN10nTable.Delete()).WithContext(ctx).BindStruct(notification).ExecRelease()
}

const notificationDeleteBatchSize = 25

func (db *DB) DeleteMessageNotifications(ctx context.Context, userID gocql.UUID, messageIDs []gocql.UUID) error {
	if len(messageIDs) == 0 {
		return nil
	}
	notifications, err := db.GetMessageNotification(ctx, userID)
	if err != nil {
		return err
	}
	ids := make(map[gocql.UUID]struct{}, len(messageIDs))
	for _, messageID := range messageIDs {
		ids[messageID] = struct{}{}
	}
	matches := make([]MessageNotification, 0, len(messageIDs))
	for _, notification := range notifications {
		if _, ok := ids[notification.MessageID]; ok {
			matches = append(matches, notification)
		}
	}
	for start := 0; start < len(matches); start += notificationDeleteBatchSize {
		end := min(start+notificationDeleteBatchSize, len(matches))
		batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
		for _, notification := range matches[start:end] {
			if err := batch.BindStruct(msgN10nTable.DeleteQuery(*db.Session), notification); err != nil {
				return err
			}
		}
		if err := db.Session.ExecuteBatch(batch); err != nil {
			return err
		}
	}
	return nil
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

func (db *DB) UpdateFriendNotification(ctx context.Context, userID, notificationID gocql.UUID) error {
	notification := FriendNotification{UserID: userID, NotificationID: notificationID}
	if err := db.Session.Query(friendN10nTable.Get()).
		WithContext(ctx).
		BindStruct(notification).
		GetRelease(&notification); err != nil {
		return err
	}

	// shortcut: reinserting resets the table TTL; preserve remaining TTL if exact expiry matters.
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	if err := batch.BindStruct(friendN10nTable.DeleteQuery(*db.Session), notification); err != nil {
		return err
	}
	notification.Seen = true
	if err := batch.BindStruct(friendN10nTable.InsertQuery(*db.Session), notification); err != nil {
		return err
	}
	return db.Session.ExecuteBatch(batch)
}
