package db

import (
	"context"
	"errors"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3"
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
	if err != nil {
		return nil, err
	}
	pending := make([]InviteNotification, 0, len(items))
	for _, item := range items {
		invite := ShareCode{Code: item.ShareCode}
		err := db.Session.Query(serverShareCodeTable.Get()).
			WithContext(ctx).BindStruct(invite).GetRelease(&invite)
		// A queued notification can arrive after joining consumed its share code.
		if errors.Is(err, gocql.ErrNotFound) || (err == nil && invite.Used) {
			if err := db.UpdateInviteNotification(ctx, userID, item.NotificationID); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		pending = append(pending, item)
	}
	return pending, nil
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

func (db *DB) bindSeenInvites(
	batch *gocqlx.Batch, items []InviteNotification, invite ShareCode,
) error {
	for _, item := range items {
		if item.UserID != invite.Receiver || item.ServerID != invite.ServerID || item.ShareCode != invite.Code {
			continue
		}
		if err := batch.BindStruct(db.Session.Query(inviteNotificationTable.Delete()), item); err != nil {
			return err
		}
	}
	return nil
}
