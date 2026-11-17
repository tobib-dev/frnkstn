package db

import (
	"context"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type ServerStore interface {
	CreateServer(context.Context, Server) (Server, error)
	GetServer(context.Context, gocql.UUID) (Server, error)
	GetServersByUser(context.Context, gocql.UUID) ([]ServerByUser, error)
	EditServer(context.Context, gocql.UUID, EditServerParams) (Server, error)
	DeleteServer(context.Context, gocql.UUID, *time.Time) error
}

type Server struct {
	ServerID          gocql.UUID
	ServerName        string
	ServerBio         string
	Owner             gocql.UUID
	OwnerName         string
	DeletionTimestamp *time.Time
}

type EditServerParams struct {
	ServerName string
	ServerBio  string
	Owner      gocql.UUID
}

type ServerByUser struct {
	UserID            gocql.UUID
	ServerID          gocql.UUID
	ServerName        string
	DeletionTimestamp *time.Time
	Cursor            time.Time
}

var serverMeta = table.Metadata{
	Name:    "servers",
	Columns: []string{"server_id", "server_name", "server_bio", "owner", "deletion_timestamp"},
	PartKey: []string{"server_id"},
}

var serverTable = table.New(serverMeta)

var serverByUserMeta = table.Metadata{
	Name:    "servers_by_user",
	Columns: []string{"user_id", "cursor", "server_id", "server_name", "deletion_timestamp"},
	PartKey: []string{"user_id"},
	SortKey: []string{"cursor", "server_id"},
}

var serverByUserTable = table.New(serverByUserMeta)

var serverModeratorsMeta = table.Metadata{
	Name:    "server_moderators",
	Columns: []string{"server_id", "mod_id", "mod_name", "role"},
	PartKey: []string{"server_id"},
	SortKey: []string{"mod_id"},
}

var serverModeratorsTable = table.New(serverModeratorsMeta)

var serverMembersMeta = table.Metadata{
	Name:    "server_members",
	Columns: []string{"server_id", "member_id", "member_name", "role"},
	PartKey: []string{"server_id"},
	SortKey: []string{"member_id"},
}

var serverMembersTable = table.New(serverMembersMeta)

/*
 * Create server, add owner to member and moderator tables,
 * and insert owner's user id into the
 * servers by user table
 */
func (db *DB) CreateServer(
	ctx context.Context,
	server Server,
) (Server, error) {
	batch, err := db.createServerBatch(ctx, server)
	if err != nil {
		return Server{}, err
	}
	if err := db.Session.ExecuteBatch(batch); err != nil {
		return Server{}, err
	}
	return server, nil
}

func (db *DB) createServerBatch(
	ctx context.Context,
	server Server,
) (*gocqlx.Batch, error) {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	query := qb.Insert(serverMeta.Name).
		Columns("server_id", "server_name", "server_bio", "owner").
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"server_id":   server.ServerID,
		"server_name": server.ServerName,
		"server_bio":  server.ServerBio,
		"owner":       server.Owner,
	}); err != nil {
		return nil, err
	}

	if (server.Owner != gocql.UUID{}) {
		cursor := time.Now().UTC().Truncate(time.Millisecond)
		query = qb.Insert(
			serverUserCursorMeta.Name,
		).
			Columns(serverUserCursorMeta.Columns...).
			Query(*db.Session)
		if err := batch.BindStruct(
			query,
			ServerUserCursor{
				UserID:   server.Owner,
				ServerID: server.ServerID,
				Cursor:   cursor,
			},
		); err != nil {
			return nil, err
		}
		query = qb.Insert(serverByUserMeta.Name).
			Columns("user_id", "server_id", "server_name", "cursor").
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":     server.Owner,
			"server_id":   server.ServerID,
			"server_name": server.ServerName,
			"cursor":      cursor,
		}); err != nil {
			return nil, err
		}

		query = db.Session.Query(serverMembersTable.Insert())
		if err := batch.BindStruct(query, Member{
			ServerID:   server.ServerID,
			MemberID:   server.Owner,
			MemberName: server.OwnerName,
			Role:       "moderator",
		}); err != nil {
			return nil, err
		}

		query = db.Session.Query(serverModeratorsTable.Insert())
		if err := batch.BindStruct(query, Mod{
			ServerID: server.ServerID,
			ModID:    server.Owner,
			ModName:  server.OwnerName,
			Role:     "moderator",
		}); err != nil {
			return nil, err
		}
	}

	return batch, nil
}

// Check if deletion timestamp has a value if so return error
func (db *DB) GetServer(
	ctx context.Context,
	serverID gocql.UUID,
) (Server, error) {
	server := Server{ServerID: serverID}

	if err := db.Session.Query(serverTable.Get()).
		WithContext(ctx).
		BindStruct(server).
		GetRelease(&server); err != nil {
		return Server{}, err
	}
	if server.DeletionTimestamp != nil {
		return Server{}, gocql.ErrNotFound
	}
	return server, nil
}

// Check deletion timestamp
func (db *DB) GetServersByUser(
	ctx context.Context,
	userID gocql.UUID,
) ([]ServerByUser, error) {
	servers := []ServerByUser{}

	if err := db.Session.Query(serverByUserTable.Select()).
		WithContext(ctx).
		BindMap(qb.M{"user_id": userID}).
		SelectRelease(&servers); err != nil {
		return nil, err
	}
	s := filterServers(servers)
	return s, nil
}

/*
 * Filter out deleted servers
 */
func filterServers(servers []ServerByUser) []ServerByUser {
	filtered := make([]ServerByUser, 0, len(servers))

	for _, serv := range servers {
		if serv.DeletionTimestamp == nil {
			filtered = append(filtered, serv)
		}
	}
	return filtered
}

/*
 * Backend should verify if owner is being changed.
 * If owner is being changed make sure the user
 * making this change is the owner. Backend service
* needs to run background task to update a server record.
*/
func (db *DB) EditServer(
	ctx context.Context,
	serverID gocql.UUID,
	args EditServerParams,
) (Server, error) {
	server := Server{
		ServerID:   serverID,
		ServerName: args.ServerName,
		ServerBio:  args.ServerBio,
		Owner:      args.Owner,
	}

	err := db.Session.Query(
		serverTable.Update("server_name", "server_bio", "owner"),
	).
		WithContext(ctx).
		BindMap(qb.M{
			"server_id":   serverID,
			"server_name": args.ServerName,
			"server_bio":  args.ServerBio,
			"owner":       args.Owner,
		}).
		ExecRelease()
	if err != nil {
		return Server{}, err
	}

	return server, nil
}

/*
 * Should delete servers from the server's table, in addition
 * should spin up a background task to clean up variant tables.
 * The backend server service should spin up an async process
 * after this returns to clean up deleted records
 */
func (db *DB) DeleteServer(
	ctx context.Context,
	serverID gocql.UUID,
	deletionTime *time.Time,
) error {
	err := db.Session.Query(
		serverTable.Update("deletion_timestamp"),
	).
		WithContext(ctx).
		BindMap(qb.M{
			"server_id":          serverID,
			"deletion_timestamp": deletionTime,
		}).
		ExecRelease()
	if err != nil {
		return err
	}
	return nil
}
