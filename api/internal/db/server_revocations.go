package db

import (
	"context"
	"errors"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/table"
)

var ErrServerMemberRevoked = errors.New("membership in this server has been revoked")

var revokedServerMembersTable = table.New(table.Metadata{
	Name:    "revoked_server_members",
	Columns: []string{"server_id", "member_id"},
	PartKey: []string{"server_id"},
	SortKey: []string{"member_id"},
})

func (db *DB) checkServerRevocation(
	ctx context.Context,
	serverID, userID gocql.UUID,
) error {
	member := Member{
		ServerID: serverID,
		MemberID: userID,
	}
	err := db.Session.Query(
		revokedServerMembersTable.Get(),
	).
		WithContext(ctx).
		BindStruct(member).
		GetRelease(&member)
	if errors.Is(err, gocql.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return ErrServerMemberRevoked
}

func (db *DB) RevokeMember(
	ctx context.Context,
	userID, serverID gocql.UUID,
) error {
	return db.removeServerMember(
		ctx, userID, serverID, true)
}
