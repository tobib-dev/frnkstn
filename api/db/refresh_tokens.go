package db

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type RefreshTokenStore interface {
	GetRefreshTokenByGitHubID(context.Context, int64) (RefreshToken, error)
	CreateRefreshToken(context.Context, RefreshToken) error
}

type RefreshToken struct {
	Token      string     `db:"refresh_token"`
	UserID     gocql.UUID `db:"user_id"`
	GitHubID   int64      `db:"github_id"`
	Expiration time.Time  `db:"expiration"`
	RevokedAt  time.Time  `db:"revoked_at"`
}

var refreshTokenMetadata = table.Metadata{
	Name:    "refresh_tokens",
	Columns: []string{"refresh_token", "user_id", "expiration", "revoked_at"},
	PartKey: []string{"refresh_token"},
}

var refreshTokenByGitHubIDMetadata = table.Metadata{
	Name:    "refresh_tokens_by_github_id",
	Columns: []string{"github_id", "user_id", "refresh_token", "expiration", "revoked_at"},
	PartKey: []string{"github_id"},
}

var refreshTokenByGitHubIDTable = table.New(refreshTokenByGitHubIDMetadata)

func (db *DB) GetRefreshTokenByGitHubID(ctx context.Context, githubID int64) (RefreshToken, error) {
	var token RefreshToken
	err := db.Session.Query(refreshTokenByGitHubIDTable.Get()).WithContext(ctx).
		BindMap(qb.M{"github_id": githubID}).GetRelease(&token)
	return token, err
}

func (db *DB) CreateRefreshToken(ctx context.Context, token RefreshToken) error {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	query := qb.Insert(refreshTokenMetadata.Name).Columns("refresh_token", "user_id", "expiration", "revoked_at").Query(*db.Session)
	if err := batch.BindMap(query, qb.M{"refresh_token": token.Token, "user_id": token.UserID, "expiration": token.Expiration, "revoked_at": token.RevokedAt}); err != nil {
		return fmt.Errorf("bind refresh token insert: %w", err)
	}
	query = qb.Insert(refreshTokenByGitHubIDMetadata.Name).Columns("github_id", "user_id", "refresh_token", "expiration", "revoked_at").Query(*db.Session)
	if err := batch.BindMap(query, qb.M{"github_id": token.GitHubID, "user_id": token.UserID, "refresh_token": token.Token, "expiration": token.Expiration, "revoked_at": token.RevokedAt}); err != nil {
		return fmt.Errorf("bind refresh token index insert: %w", err)
	}
	if err := db.Session.ExecuteBatch(batch); err != nil {
		return fmt.Errorf("create refresh token: %w", err)
	}
	return nil
}
