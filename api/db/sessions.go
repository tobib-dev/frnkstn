package db

import (
	"context"
	"fmt"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type SessionStore interface {
	CreateSession(context.Context, CreateSessionParams) (Session, error)
	GetSession(context.Context, GetSessionParams) (SessionByRefreshToken, error)
}

var sessionMetadata = table.Metadata{
	Name:    "sessions",
	Columns: []string{"id", "access_token", "expires_in", "refresh_token", "refresh_token_expires_in", "user_id"},
	PartKey: []string{"id"},
}

var sessionTable = table.New(sessionMetadata)

type Session struct {
	ID                    gocql.UUID
	AccessToken           string
	ExpiresIn             time.Time
	RefreshToken          string
	RefreshTokenExpiresIn time.Time
	UserID                gocql.UUID
	SignedOutAt           time.Time
}

type CreateSessionParams struct {
	UserID                gocql.UUID
	AccessToken           string
	ExpiresIn             time.Time
	RefreshToken          string
	RefreshTokenExpiresIn time.Time
}

func (db *DB) CreateSession(ctx context.Context, arg CreateSessionParams) (Session, error) {
	sess := Session{
		UserID:                arg.UserID,
		ID:                    gocql.UUIDFromTime(time.Now()),
		AccessToken:           arg.AccessToken,
		ExpiresIn:             arg.ExpiresIn,
		RefreshToken:          arg.RefreshToken,
		RefreshTokenExpiresIn: arg.RefreshTokenExpiresIn,
	}
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	sessionInsert := qb.Insert(sessionMetadata.Name).
		Columns("id", "access_token", "expires_in", "refresh_token", "refresh_token_expires_in", "user_id").
		Query(*db.Session)
	if err := batch.BindMap(sessionInsert, qb.M{
		"id":                       sess.ID,
		"access_token":             sess.AccessToken,
		"expires_in":               sess.ExpiresIn,
		"refresh_token":            sess.RefreshToken,
		"refresh_token_expires_in": sess.RefreshTokenExpiresIn,
		"user_id":                  sess.UserID,
	}); err != nil {
		return Session{}, fmt.Errorf("bind session insert: %w", err)
	}

	if sess.RefreshToken != "" {
		sessionByRefreshTokenInsert := qb.Insert(sessionByRefreshTokenMetadata.Name).
			Columns("refresh_token", "session_id", "user_id", "expires_in", "access_token", "access_token_expires_in").
			Query(*db.Session)
		if err := batch.BindMap(sessionByRefreshTokenInsert, qb.M{
			"refresh_token":           sess.RefreshToken,
			"session_id":              sess.ID,
			"user_id":                 sess.UserID,
			"expires_in":              sess.RefreshTokenExpiresIn,
			"access_token":            sess.AccessToken,
			"access_token_expires_in": sess.ExpiresIn,
		}); err != nil {
			return Session{}, fmt.Errorf("bind refresh-token session insert: %w", err)
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}

	return sess, nil
}

var sessionByRefreshTokenMetadata = table.Metadata{
	Name:    "sessions_by_refresh_tokens",
	Columns: []string{"refresh_token", "session_id", "user_id", "expires_in", "access_token", "access_token_expires_in", "signed_out_at"},
	PartKey: []string{"refresh_token"},
	SortKey: []string{"session_id"},
}

var sessionByRefreshTokenTable = table.New(sessionByRefreshTokenMetadata)

type SessionByRefreshToken struct {
	RefreshToken         string
	SessionID            gocql.UUID
	UserID               gocql.UUID
	ExpiresIn            time.Time
	AccessToken          string
	AccessTokenExpiresIn time.Time
	SignedOutAt          time.Time
}

type GetSessionParams struct {
	RefreshToken string
}

func (db *DB) GetSession(ctx context.Context, params GetSessionParams) (SessionByRefreshToken, error) {
	var sess SessionByRefreshToken
	err := db.Session.Query(sessionByRefreshTokenTable.Get()).
		WithContext(ctx).
		BindMap(qb.M{"refresh_token": params.RefreshToken}).
		GetRelease(&sess)
	if err != nil {
		return SessionByRefreshToken{}, fmt.Errorf("get session: %w", err)
	}
	return sess, nil
}
