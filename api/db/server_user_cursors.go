package db

import (
	"context"
	"errors"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

var (
	ErrAlreadyServerMember = errors.New("user already belongs to server")
	ErrShareCodeExists     = errors.New("share code already exists")
)

type ServerUserCursor struct {
	UserID   gocql.UUID `db:"user_id"`
	ServerID gocql.UUID `db:"server_id"`
	Cursor   time.Time  `db:"cursor"`
	JoinCode string     `db:"join_code"`
}

var serverUserCursorMeta = table.Metadata{
	Name:    "server_user_cursors",
	Columns: []string{"user_id", "server_id", "cursor", "join_code"},
	PartKey: []string{"user_id"},
	SortKey: []string{"server_id"},
}

var serverUserCursorTable = table.New(serverUserCursorMeta)

func (db *DB) GetServerUserCursor(ctx context.Context, userID, serverID gocql.UUID) (ServerUserCursor, error) {
	row := ServerUserCursor{UserID: userID, ServerID: serverID}
	err := db.Session.Query(serverUserCursorTable.Get()).WithContext(ctx).BindStruct(row).GetRelease(&row)
	return row, err
}

func (db *DB) claimServerInvitation(ctx context.Context, userID, serverID gocql.UUID, code string) (ServerUserCursor, error) {
	row := ServerUserCursor{UserID: userID, ServerID: serverID, Cursor: time.Now().UTC().Truncate(time.Millisecond), JoinCode: code}
	applied, err := qb.Insert(serverUserCursorMeta.Name).Columns(serverUserCursorMeta.Columns...).Unique().
		Query(*db.Session).WithContext(ctx).BindStruct(row).ExecCASRelease()
	if err != nil {
		return ServerUserCursor{}, err
	}
	if applied {
		return row, nil
	}
	// Reuse the winning invite's cursor so a failed join batch can be retried without another index row.
	existing, err := db.GetServerUserCursor(ctx, userID, serverID)
	if err != nil {
		return ServerUserCursor{}, err
	}
	if err := validateInvitationClaim(existing, code); err != nil {
		return ServerUserCursor{}, err
	}
	return existing, nil
}

func validateInvitationClaim(row ServerUserCursor, code string) error {
	if code == "" || row.JoinCode != code {
		return ErrAlreadyServerMember
	}
	return nil
}
