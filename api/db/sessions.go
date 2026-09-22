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
	GetSession(context.Context, GetSessionParams) (Session, error)
	GetSessionByUser(context.Context, GetSessionByUserParams) (SessionByUser, error)
	UpdateSession(context.Context, UpdateSessionParams) (Session, error)
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
	ID                    gocql.UUID
	UserID                gocql.UUID
	AccessToken           string
	ExpiresIn             time.Time
	RefreshToken          string
	RefreshTokenExpiresIn time.Time
}

func (db *DB) CreateSession(ctx context.Context, arg CreateSessionParams) (Session, error) {
	sess := Session{
		UserID:                arg.UserID,
		ID:                    arg.ID,
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

	if sess.UserID.String() != "" {
		sessionByUserInsert := qb.Insert(sessionByUserMetadata.Name).
			Columns("user_id", "session_id", "refresh_token", "expires_in").
			Query(*db.Session)
		if err := batch.BindMap(sessionByUserInsert, qb.M{
			"refresh_token": sess.RefreshToken,
			"session_id":    sess.ID,
			"user_id":       sess.UserID,
			"expires_in":    sess.RefreshTokenExpiresIn,
		}); err != nil {
			return Session{}, fmt.Errorf("bind refresh-token session insert: %w", err)
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}

	return sess, nil
}

var sessionByUserMetadata = table.Metadata{
	Name:    "sessions_by_user_id",
	Columns: []string{"user_id", "session_id", "refresh_token", "expires_in", "signed_out_at"},
	PartKey: []string{"user_id"},
	SortKey: []string{"session_id"},
}

var sessionByUserTable = table.New(sessionByUserMetadata)

type SessionByUser struct {
	UserID       gocql.UUID
	SessionID    gocql.UUID
	RefreshToken string
	ExpiresIn    time.Time
	SignedOutAt  time.Time
}

type GetSessionParams struct {
	ID gocql.UUID
}

func (db *DB) GetSession(ctx context.Context, params GetSessionParams) (Session, error) {
	var sess Session
	err := db.Session.Query(sessionTable.Get()).
		WithContext(ctx).
		BindMap(qb.M{"id": params.ID}).
		GetRelease(&sess)
	if err != nil {
		return Session{}, err
	}
	return sess, nil
}

type GetSessionByUserParams struct {
	UserID gocql.UUID
}

func (db *DB) GetSessionByUser(ctx context.Context, params GetSessionByUserParams) (SessionByUser, error) {
	var sess SessionByUser
	err := db.Session.Query(sessionByUserTable.Get()).
		WithContext(ctx).
		BindMap(qb.M{"user_id": params.UserID}).
		GetRelease(&sess)
	if err != nil {
		return SessionByUser{}, err
	}
	return sess, nil
}

type UpdateSessionParams struct {
	ID          gocql.UUID
	UserID      gocql.UUID
	SignedOutAt time.Time
}

func (db *DB) UpdateSession(ctx context.Context, args UpdateSessionParams) (Session, error) {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	query := qb.Update(sessionMetadata.Name).
		Set("signed_out_at").
		Where(qb.Eq("id")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"id":            args.ID,
		"signed_out_at": args.SignedOutAt,
	}); err != nil {
		return Session{}, err
	}

	queryByID := qb.Update(sessionByUserMetadata.Name).
		Set("signed_out_at").
		Where(qb.Eq("user_id")).
		Query(*db.Session)
	if err := batch.BindMap(queryByID, qb.M{
		"user_id":       args.UserID,
		"signed_out_at": args.SignedOutAt,
	}); err != nil {
		return Session{}, err
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return Session{}, err
	}
	session := Session{
		ID:          args.ID,
		UserID:      args.UserID,
		SignedOutAt: args.SignedOutAt,
	}
	return session, nil
}
