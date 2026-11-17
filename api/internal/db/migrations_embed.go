package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/scylladb/gocqlx/v3/migrate"
)

//go:embed migrations/*.cql
var migrationFiles embed.FS

func (db *DB) Migrate(ctx context.Context) error {
	migrations, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("load migrations: %w", err)
	}

	if err := migrate.FromFS(ctx, *db.Session, migrations); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
