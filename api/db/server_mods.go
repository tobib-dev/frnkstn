package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/table"
)

type ServerModsStore interface {
	AddMod(context.Context, gocql.UUID, gocql.UUID) (Mod, error)
	GetMod(context.Context, gocql.UUID, gocql.UUID) (Mod, error)
	GetMods(context.Context, gocql.UUID) ([]Mod, error)
	DeleteMod(context.Context, gocql.UUID) error
	DeleteMember(context.Context, gocql.UUID, gocql.UUID) error
}

type Mod struct {
	ServerID gocql.UUID
	ModID    gocql.UUID
	ModName  string
}

var modMeta = table.Metadata{
	Name:    "server_moderators",
	Columns: []string{"server_id", "mod_id", "mod_name", "role"},
	PartKey: []string{"server_id"},
	SortKey: []string{"mod_id"},
}

var modTable = table.New(modMeta)

func (db *DB) AddMod(
	ctx context.Context, serverID, modID gocql.UUID,
) (Mod, error) {
	return Mod{}, nil
}

func (db *DB) GetMod(
	ctx context.Context, serverID, modID gocql.UUID,
) (Mod, error) {
	return Mod{}, nil
}

func (db *DB) GetMods(
	ctx context.Context, serverID gocql.UUID,
) ([]Mod, error) {
	mods := []Mod{}
	return mods, nil
}

// only owner should delete a moderator
func (db *DB) DeleteMods(
	ctx context.Context, serverID, modID gocql.UUID,
) error {
	return nil
}
