package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3"
	"github.com/scylladb/gocqlx/v3/qb"
)

type ServerModsStore interface {
	AddMod(context.Context, gocql.UUID, gocql.UUID) (Mod, error)
	GetMod(context.Context, gocql.UUID, gocql.UUID) (Mod, error)
	GetMods(context.Context, gocql.UUID) ([]Mod, error)
	DeleteMod(context.Context, gocql.UUID, gocql.UUID) error
}

type Mod struct {
	ServerID gocql.UUID
	ModID    gocql.UUID
	ModName  string
	Role     string
}

func (db *DB) AddMod(
	ctx context.Context,
	serverID, modID gocql.UUID,
) (Mod, error) {
	member, err := db.GetMember(ctx, serverID, modID)
	if err != nil {
		return Mod{}, err
	}
	mod := Mod{
		ServerID: serverID,
		ModID:    modID,
		ModName:  member.MemberName,
		Role:     "moderator",
	}
	batch, err := db.moderatorRoleBatch(ctx, member, true)
	if err != nil {
		return Mod{}, err
	}
	return mod, db.Session.ExecuteBatch(batch)
}

func (db *DB) GetMod(
	ctx context.Context,
	serverID, modID gocql.UUID,
) (Mod, error) {
	mod := Mod{ServerID: serverID, ModID: modID}
	err := db.Session.Query(serverModeratorsTable.Get()).
		WithContext(ctx).BindStruct(mod).GetRelease(&mod)
	return mod, err
}

func (db *DB) GetMods(
	ctx context.Context,
	serverID gocql.UUID,
) ([]Mod, error) {
	mods := []Mod{}
	err := db.Session.Query(
		serverModeratorsTable.Select(),
	).
		WithContext(ctx).
		BindMap(qb.M{"server_id": serverID}).
		SelectRelease(&mods)
	return mods, err
}

func (db *DB) DeleteMod(
	ctx context.Context,
	serverID, modID gocql.UUID,
) error {
	member, err := db.GetMember(ctx, serverID, modID)
	if err != nil {
		return err
	}
	batch, err := db.moderatorRoleBatch(ctx, member, false)
	if err != nil {
		return err
	}
	return db.Session.ExecuteBatch(batch)
}

func (db *DB) moderatorRoleBatch(
	ctx context.Context, member Member, promote bool,
) (*gocqlx.Batch, error) {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	mod := Mod{
		ServerID: member.ServerID,
		ModID:    member.MemberID,
		ModName:  member.MemberName,
		Role:     "moderator",
	}
	member.Role = "member"
	statement, names := serverModeratorsTable.Delete()
	if promote {
		member.Role = "moderator"
		statement, names = serverModeratorsTable.Insert()
	}
	if err := batch.BindStruct(
		db.Session.Query(
			statement, names,
		), mod); err != nil {
		return nil, err
	}
	query := db.Session.Query(
		serverMembersTable.Update("role"),
	)
	if err := batch.BindStruct(
		query, member); err != nil {
		return nil, err
	}
	return batch, nil
}
