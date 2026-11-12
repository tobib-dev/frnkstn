package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type InviteNotification struct {
	UserID         gocql.UUID
	NotificationID gocql.UUID
	ServerID       gocql.UUID
	ServerName     string
	SenderID       gocql.UUID
	ShareCode      string
}

var inviteNotificationTable = table.New(table.Metadata{
	Name: "invite_notifications",
	Columns: []string{
		"user_id", "notification_id", "server_id", "server_name", "sender_id", "share_code",
	},
	PartKey: []string{"user_id"},
	SortKey: []string{"notification_id"},
})

func (db *DB) AddInviteNotification(
	ctx context.Context,
	item InviteNotification,
) error {
	return db.Session.Query(
		inviteNotificationTable.Insert(),
	).WithContext(ctx).
		BindStruct(item).ExecRelease()
}

func (db *DB) GetInviteNotifications(
	ctx context.Context, userID gocql.UUID,
) ([]InviteNotification, error) {
	var items []InviteNotification
	err := db.Session.Query(
		inviteNotificationTable.Select(),
	).WithContext(ctx).
		BindMap(qb.M{"user_id": userID}).
		SelectRelease(&items)
	return items, err
}

func (db *DB) UpdateInviteNotification(
	ctx context.Context, userID,
	notificationID gocql.UUID,
) error {
	return db.Session.Query(
		inviteNotificationTable.Delete(),
	).WithContext(ctx).
		BindMap(
			qb.M{
				"user_id":         userID,
				"notification_id": notificationID,
			},
		).ExecRelease()
}
