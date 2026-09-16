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
	AccessToken           string
	ExpiresIn             time.Time
	RefreshToken          string
	RefreshTokenExpiresIn time.Time
}

func (db *DB) CreateSession(ctx context.Context, arg CreateSessionParams) (Session, error) {
	sess := Session{
		ID:                    gocql.UUIDFromTime(time.Now()),
		AccessToken:           arg.AccessToken,
		ExpiresIn:             arg.ExpiresIn,
		RefreshToken:          arg.RefreshToken,
		RefreshTokenExpiresIn: arg.RefreshTokenExpiresIn,
	}
	err := db.Session.Query(sessionTable.Insert()).WithContext(ctx).BindStruct(sess).ExecRelease()
	if err != nil {
		return Session{}, fmt.Errorf("error creating session: %w", err)
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
	var sess []SessionByRefreshToken
	err := db.Session.Query(sessionByRefreshTokenTable.Get()).
		WithContext(ctx).
		BindMap(qb.M{"refresh_token": params.RefreshToken}).
		SelectRelease(&sess)
	if err != nil {
		return SessionByRefreshToken{}, fmt.Errorf("No sessions for refresh token - %s: %w", params.RefreshToken, err)
	}
	return sess[0], nil
}
