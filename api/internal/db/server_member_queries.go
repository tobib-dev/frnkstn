package db

import (
	"context"
	"errors"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
)

func (db *DB) GetMember(
	ctx context.Context, serverID, userID gocql.UUID,
) (Member, error) {
	member := Member{ServerID: serverID, MemberID: userID}
	err := db.Session.Query(serverMembersTable.Get()).
		WithContext(ctx).BindStruct(member).GetRelease(&member)
	return member, err
}

func (db *DB) IsServerModerator(
	ctx context.Context, serverID, userID gocql.UUID,
) (bool, error) {
	var id gocql.UUID
	query := qb.Select(serverModeratorsMeta.Name).
		Columns("mod_id").
		Where(qb.Eq("server_id"), qb.Eq("mod_id")).
		Query(*db.Session)
	err := query.WithContext(ctx).BindMap(qb.M{
		"server_id": serverID,
		"mod_id":    userID,
	}).GetRelease(&id)
	if errors.Is(err, gocql.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (db *DB) GetServerByCode(
	ctx context.Context, code string, userID gocql.UUID,
) (Server, error) {
	invite := ShareCode{Code: code}
	err := db.Session.Query(serverShareCodeTable.Get()).
		WithContext(ctx).BindStruct(invite).GetRelease(&invite)
	if err != nil {
		return Server{}, err
	}
	if invite.Receiver != userID {
		return Server{}, ErrInvitationRecipient
	}
	if invite.Used {
		return Server{}, ErrInvitationUsed
	}
	if err := db.checkServerRevocation(ctx, invite.ServerID, userID); err != nil {
		return Server{}, err
	}
	return db.GetServer(ctx, invite.ServerID)
}
