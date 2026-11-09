package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
)

func (db *DB) serverCleanupTargets(
	ctx context.Context, serverID gocql.UUID,
) (Server, map[gocql.UUID]struct{}, error) {
	server := Server{ServerID: serverID}
	if err := db.Session.Query(
		serverTable.Get(),
	).
		WithContext(ctx).
		BindStruct(server).
		GetRelease(&server); err != nil {
		return Server{}, nil, err
	}
	members, err := db.GetMembers(ctx, serverID)
	if err != nil {
		return Server{}, nil, err
	}
	var mods []struct {
		ModID gocql.UUID `db:"mod_id"`
	}
	err = qb.Select(serverModeratorsMeta.Name).
		Columns("mod_id").
		Where(qb.Eq("server_id")).
		Query(*db.Session).
		WithContext(ctx).
		BindMap(qb.M{"server_id": serverID}).
		SelectRelease(&mods)
	if err != nil {
		return Server{}, nil, err
	}
	users := map[gocql.UUID]struct{}{server.Owner: {}}
	for _, member := range members {
		users[member.MemberID] = struct{}{}
	}
	for _, mod := range mods {
		users[mod.ModID] = struct{}{}
	}
	return server, users, nil
}

func (db *DB) UpdateServerLookups(
	ctx context.Context,
	serverID gocql.UUID,
) error {
	return db.updateServerLookups(ctx, serverID, false)
}

func (db *DB) MarkServerLookupsDeleted(
	ctx context.Context, serverID gocql.UUID,
) error {
	return db.updateServerLookups(ctx, serverID, true)
}

func (db *DB) updateServerLookups(
	ctx context.Context,
	serverID gocql.UUID,
	deletion bool,
) error {
	server, users, err := db.serverCleanupTargets(ctx, serverID)
	if err != nil {
		return err
	}
	if deletion && server.DeletionTimestamp == nil {
		return fmt.Errorf("server must be marked deleted before cleanup")
	}
	// A delayed edit must never clear a deletion marker.
	if !deletion && server.DeletionTimestamp != nil {
		return nil
	}
	for userID := range users {
		cursor, err := db.GetServerUserCursor(ctx, userID, serverID)
		if errors.Is(err, gocql.ErrNotFound) {
			continue // Membership was removed while cleanup was queued.
		}
		if err != nil {
			return err
		}
		query := qb.Update(serverByUserMeta.Name)
		values := qb.M{
			"user_id":   userID,
			"server_id": serverID,
			"cursor":    cursor.Cursor,
		}
		if deletion {
			query.Set("deletion_timestamp")
			values["deletion_timestamp"] = server.DeletionTimestamp
		} else {
			query.Set("server_name")
			values["server_name"] = server.ServerName
		}
		_, err = query.
			Where(qb.Eq("user_id"), qb.Eq("cursor"), qb.Eq("server_id")).
			Existing().Query(*db.Session).WithContext(ctx).
			BindMap(values).ExecCASRelease()
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) PurgeServer(
	ctx context.Context,
	serverID gocql.UUID,
) error {
	server, users, err := db.serverCleanupTargets(ctx, serverID)
	if err != nil {
		return err
	}
	if server.DeletionTimestamp == nil {
		return fmt.Errorf("refusing to purge an active server")
	}
	for userID := range users {
		if err := db.deleteServerUserRows(
			ctx, userID, serverID, time.Time{},
		); err != nil {
			return err
		}
		if err := db.Session.Query(
			serverUserCursorTable.Delete(),
		).
			WithContext(ctx).
			BindMap(qb.M{
				"user_id":   userID,
				"server_id": serverID,
			}).ExecRelease(); err != nil {
			return err
		}
	}
	// Delete the canonical record last so retries can still enumerate recipients.
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	for _, name := range []string{
		serverMembersMeta.Name,
		serverModeratorsMeta.Name,
		"server_channels",
		serverMeta.Name,
	} {
		query := qb.Delete(name).
			Where(qb.Eq("server_id")).
			Query(*db.Session)
		if err := batch.BindMap(query,
			qb.M{"server_id": serverID}); err != nil {
			return err
		}
	}
	return db.Session.ExecuteBatch(batch)
}

func (db *DB) CleanupRemovedMember(
	ctx context.Context, userID,
	serverID gocql.UUID,
	removedAt time.Time,
) error {
	if removedAt.IsZero() {
		return fmt.Errorf("removal time is required")
	}
	return db.deleteServerUserRows(
		ctx, userID, serverID, removedAt)
}

func (db *DB) deleteServerUserRows(
	ctx context.Context, userID,
	serverID gocql.UUID,
	removedAt time.Time,
) error {
	var rows []ServerByUser
	err := db.Session.Query(serverByUserTable.Select()).
		WithContext(ctx).
		BindMap(qb.M{"user_id": userID}).
		SelectRelease(&rows)
	if err != nil {
		return err
	}
	current, err := db.GetServerUserCursor(
		ctx, userID, serverID)
	if err != nil &&
		!errors.Is(err, gocql.ErrNotFound) {
		return err
	}
	for _, row := range rows {
		if !shouldRemoveServerUserRow(
			row, current, serverID, removedAt,
		) {
			continue
		}
		if err := db.Session.Query(
			serverByUserTable.Delete(),
		).
			WithContext(ctx).
			BindStruct(row).
			ExecRelease(); err != nil {
			return err
		}
	}
	return nil
}

func shouldRemoveServerUserRow(
	row ServerByUser, current ServerUserCursor,
	serverID gocql.UUID, removedAt time.Time,
) bool {
	if row.ServerID != serverID {
		return false
	}
	if removedAt.IsZero() {
		return true
	}
	// Delayed removals must preserve a rejoin, even within timestamp precision.
	return row.Cursor.Before(removedAt) &&
		!row.Cursor.Equal(current.Cursor)
}
