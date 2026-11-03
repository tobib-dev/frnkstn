package db

import (
	"context"
	"errors"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type ServerStore interface {
	CreateServer(context.Context, Server) (Server, error)
	GetServer(context.Context, gocql.UUID) (Server, error)
	GetServerByUser(context.Context, gocql.UUID) ([]ServerByUser, error)
	EditServer(context.Context, gocql.UUID, EditServerParams) (Server, error)
	DeleteServer(context.Context, Server) error
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
 * Create server, add owner to mods table,
 * and insert owner's user id into the
 * servers by user table
 */
func (db *DB) CreateServer(
	ctx context.Context,
	server Server,
) (Server, error) {
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
		return Server{}, err
	}

	if (server.Owner != gocql.UUID{}) {
		query = qb.Insert(serverByUserMeta.Name).
			Columns("user_id", "server_id", "server_name", "cursor").
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":     server.Owner,
			"server_id":   server.ServerID,
			"server_name": server.ServerName,
			"cursor":      time.Now(),
		}); err != nil {
			return Server{}, err
		}

		query = qb.Insert(serverModeratorsMeta.Name).
			Columns("server_id", "mod_id", "mod_name", "role").
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"server_id": server.ServerID,
			"mod_id":    server.Owner,
			"mod_name":  server.OwnerName,
			"role":      "moderator",
		}); err != nil {
			return Server{}, err
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return Server{}, err
	}
	return server, nil
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
		return Server{}, errors.New("server is deleted!!!")
	}
	return server, nil
}

// Check deletion timestamp
func (db *DB) GetServerByUser(
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
	server Server,
) error {
	now := time.Now()
	err := db.Session.Query(
		serverTable.Update("deletion_timestamp"),
	).
		WithContext(ctx).
		BindMap(qb.M{
			"server_id":          server.ServerID,
			"deletion_timestamp": &now,
		}).
		ExecRelease()
	if err != nil {
		return err
	}
	return nil
}
