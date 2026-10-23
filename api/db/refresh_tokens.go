package db

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type InternalRefreshTokenStore interface {
	GetInternalRefreshTokenByGitHubID(context.Context, int64) (InternalRefreshToken, error)
	CreateInternalRefreshToken(context.Context, InternalRefreshToken) error
}

type InternalRefreshToken struct {
	Token      string     `db:"refresh_token"`
	UserID     gocql.UUID `db:"user_id"`
	GitHubID   int64      `db:"github_id"`
	Expiration time.Time  `db:"expiration"`
	RevokedAt  time.Time  `db:"revoked_at"`
}

var internalRefreshTokenMetadata = table.Metadata{
	Name:    "refresh_tokens",
	Columns: []string{"refresh_token", "user_id", "expiration", "revoked_at"},
	PartKey: []string{"refresh_token"},
}

var internalRefreshTokenByGitHubIDMetadata = table.Metadata{
	Name:    "refresh_tokens_by_github_id",
	Columns: []string{"github_id", "user_id", "refresh_token", "expiration", "revoked_at"},
	PartKey: []string{"github_id"},
}

var internalRefreshTokenByGitHubIDTable = table.New(internalRefreshTokenByGitHubIDMetadata)

func (db *DB) GetInternalRefreshTokenByGitHubID(ctx context.Context, githubID int64) (InternalRefreshToken, error) {
	var token InternalRefreshToken
	err := db.Session.Query(internalRefreshTokenByGitHubIDTable.Get()).WithContext(ctx).
		BindMap(qb.M{"github_id": githubID}).GetRelease(&token)
	return token, err
}

func (db *DB) CreateInternalRefreshToken(ctx context.Context, token InternalRefreshToken) error {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	query := qb.Insert(internalRefreshTokenMetadata.Name).Columns("refresh_token", "user_id", "expiration", "revoked_at").Query(*db.Session)
	if err := batch.BindMap(query, qb.M{"refresh_token": token.Token, "user_id": token.UserID, "expiration": token.Expiration, "revoked_at": token.RevokedAt}); err != nil {
		return fmt.Errorf("bind refresh token insert: %w", err)
	}
	query = qb.Insert(internalRefreshTokenByGitHubIDMetadata.Name).Columns("github_id", "user_id", "refresh_token", "expiration", "revoked_at").Query(*db.Session)
	if err := batch.BindMap(query, qb.M{"github_id": token.GitHubID, "user_id": token.UserID, "refresh_token": token.Token, "expiration": token.Expiration, "revoked_at": token.RevokedAt}); err != nil {
		return fmt.Errorf("bind refresh token index insert: %w", err)
	}
	if err := db.Session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}
