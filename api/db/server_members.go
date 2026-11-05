package db

import (
	"context"
	"errors"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type ServerMemberStore interface {
	JoinServer(context.Context, JoinServerArgs) (Server, error)
	LeaveServer(context.Context, gocql.UUID, gocql.UUID) error
	GetMembers(context.Context, gocql.UUID) ([]Member, error)
	ShareServer(context.Context, ShareCode) error
	GetMember(context.Context, gocql.UUID, gocql.UUID) (Member, error)
	IsServerModerator(context.Context, gocql.UUID, gocql.UUID) (bool, error)
	GetServerByCode(context.Context, string, gocql.UUID) (Server, error)
}

var (
	ErrInvitationRecipient = errors.New("invitation belongs to another user")
	ErrInvitationUsed      = errors.New("invitation has been used")
)

type Member struct {
	ServerID   gocql.UUID
	MemberID   gocql.UUID
	MemberName string
	Role       string
}

type ShareCode struct {
	Code     string `db:"share_code"`
	Sender   gocql.UUID
	Receiver gocql.UUID
	ServerID gocql.UUID
	Used     bool
}

type JoinServerArgs struct {
	UserID   gocql.UUID
	UserName string
	Code     string
}

var serverShareCodeMeta = table.Metadata{
	Name:    "server_share_code",
	Columns: []string{"share_code", "sender", "receiver", "server_id", "used"},
	PartKey: []string{"share_code"},
}

var serverShareCodeTable = table.New(serverShareCodeMeta)

func (db *DB) ShareServer(
	ctx context.Context, shareCode ShareCode,
) error {
	if shareCode.Code == "" || shareCode.Sender == (gocql.UUID{}) || shareCode.Receiver == (gocql.UUID{}) || shareCode.ServerID == (gocql.UUID{}) {
		return errors.New("share code, sender, receiver and server are required")
	}
	shareCode.Used = false
	applied, err := qb.Insert(serverShareCodeMeta.Name).Columns(serverShareCodeMeta.Columns...).Unique().
		Query(*db.Session).WithContext(ctx).BindStruct(shareCode).ExecCASRelease()
	if err != nil {
		return err
	}

	if !applied {
		return ErrShareCodeExists
	}
	return nil
}

func (db *DB) JoinServer(
	ctx context.Context, args JoinServerArgs,
) (Server, error) {
	if args.UserID == (gocql.UUID{}) || args.Code == "" {
		return Server{}, errors.New("user and share code are required")
	}
	shareInfo := ShareCode{Code: args.Code}
	err := db.Session.Query(
		serverShareCodeTable.Get(),
	).WithContext(ctx).
		BindStruct(shareInfo).
		GetRelease(&shareInfo)
	if err != nil {
		return Server{}, err
	}
	if args.UserID != shareInfo.Receiver {
		return Server{}, ErrInvitationRecipient
	}
	if shareInfo.Used {
		return Server{}, ErrInvitationUsed
	}

	server, err := db.GetServer(ctx, shareInfo.ServerID)
	if err != nil {
		return Server{}, err
	}

	if server.Owner == args.UserID {
		return Server{}, ErrAlreadyServerMember
	}
	var moderator struct {
		ServerID gocql.UUID `db:"server_id"`
		ModID    gocql.UUID `db:"mod_id"`
		ModName  string     `db:"mod_name"`
		Role     string     `db:"role"`
	}
	err = db.Session.Query(serverModeratorsTable.Get()).WithContext(ctx).
		BindMap(qb.M{"server_id": server.ServerID, "mod_id": args.UserID}).GetRelease(&moderator)
	if err == nil {
		return Server{}, ErrAlreadyServerMember
	}
	if !errors.Is(err, gocql.ErrNotFound) {
		return Server{}, err
	}
	claim, err := db.claimServerInvitation(ctx, args.UserID, server.ServerID, args.Code)
	if err != nil {
		return Server{}, err
	}

	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	query := qb.Update(serverShareCodeMeta.Name).
		Set("used").
		Where(qb.Eq("share_code")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"share_code": shareInfo.Code,
		"used":       true,
	}); err != nil {
		return Server{}, err
	}

	if (args.UserID != gocql.UUID{}) {
		query = qb.Insert(serverByUserMeta.Name).
			Columns("user_id", "server_id", "server_name", "cursor").
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":     args.UserID,
			"server_id":   server.ServerID,
			"server_name": server.ServerName,
			"cursor":      claim.Cursor,
		}); err != nil {
			return Server{}, err
		}

		query = qb.Insert(serverMembersMeta.Name).
			Columns("server_id", "member_id", "member_name", "role").
			Query(*db.Session)
		if err = batch.BindMap(query, qb.M{
			"server_id":   server.ServerID,
			"member_id":   args.UserID,
			"member_name": args.UserName,
			"role":        "member",
		}); err != nil {
			return Server{}, err
		}
	}

	if err = db.Session.ExecuteBatch(batch); err != nil {
		return Server{}, err
	}
	return server, nil
}

func (db *DB) GetMembers(
	ctx context.Context, serverID gocql.UUID,
) ([]Member, error) {
	members := []Member{}

	if err := db.Session.Query(
		serverMembersTable.Select(),
	).WithContext(ctx).
		BindMap(qb.M{"server_id": serverID}).
		SelectRelease(&members); err != nil {
		return nil, err
	}
	return members, nil
}

func (db *DB) LeaveServer(
	ctx context.Context,
	userID, serverID gocql.UUID,
) error {
	if userID == (gocql.UUID{}) || serverID == (gocql.UUID{}) {
		return errors.New("user and server are required")
	}
	cursor, err := db.GetServerUserCursor(ctx, userID, serverID)
	if err != nil {
		return err
	}
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	if (userID != gocql.UUID{}) {
		query := qb.Delete(
			serverMembersMeta.Name,
		).Where(qb.Eq("server_id"), qb.Eq("member_id")).
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"server_id": serverID,
			"member_id": userID,
		}); err != nil {
			return err
		}

		query = qb.Delete(
			serverByUserMeta.Name,
		).Where(qb.Eq("user_id"), qb.Eq("cursor"), qb.Eq("server_id")).
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":   userID,
			"server_id": serverID,
			"cursor":    cursor.Cursor,
		}); err != nil {
			return err
		}
	}
	query := qb.Delete(serverUserCursorMeta.Name).Where(qb.Eq("user_id"), qb.Eq("server_id")).Query(*db.Session)
	if err := batch.BindMap(query, qb.M{"user_id": userID, "server_id": serverID}); err != nil {
		return err
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return err
	}
	return nil
}
