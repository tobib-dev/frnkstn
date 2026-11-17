package main

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/gocql/gocql"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	"github.com/tobib-dev/frnkstn/api/internal/auth"
	"github.com/tobib-dev/frnkstn/api/internal/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type refreshSessionStore struct {
	db.SessionStore
	session db.Session
	err     error
}

func (s refreshSessionStore) GetSession(_ context.Context, req db.GetSessionParams) (db.Session, error) {
	if req.ID != s.session.ID {
		return db.Session{}, gocql.ErrNotFound
	}
	return s.session, s.err
}

func TestRefreshSession(t *testing.T) {
	userID, sessionID := gocql.TimeUUID(), gocql.TimeUUID()
	for _, name := range []string{
		"valid", "expired", "revoked", "missing", "signed out", "wrong user", "deleted user", "database error",
	} {
		t.Run(name, func(t *testing.T) {
			token := db.InternalRefreshToken{Token: "refresh", UserID: userID, Expiration: time.Now().Add(time.Hour)}
			session := refreshSessionStore{session: db.Session{ID: sessionID, UserID: userID}}
			users := &testUserStore{users: map[gocql.UUID]db.User{userID: {ID: userID}}}
			want := codes.Unauthenticated
			switch name {
			case "valid":
				want = codes.OK
			case "expired":
				token.Expiration = time.Now().Add(-time.Second)
			case "revoked":
				token.RevokedAt = time.Now()
			case "missing":
				token.Token = "other"
			case "signed out":
				session.session.SignedOutAt = time.Now()
			case "wrong user":
				session.session.UserID = gocql.TimeUUID()
			case "deleted user":
				delete(users.users, userID)
			case "database error":
				session.err = errors.New("offline")
				want = codes.Internal
			}
			service := &SessionService{
				cfg:   &Config{logger: slog.Default(), signingKey: apiTestSigningKey},
				store: session, users: users, tokens: &testInternalRefreshTokenStore{token: token},
			}
			// No bearer token and no GitHub verifier: renewal authenticates independently.
			response, err := service.RefreshSession(context.Background(), &sessionsV1.RefreshSessionRequest{
				SessionId: sessionID.String(), InternalRefreshToken: "refresh",
			})
			if status.Code(err) != want {
				t.Fatalf("want %v, got %v", want, err)
			}
			if want != codes.OK {
				return
			}
			claims, err := auth.VerifyToken(response.InternalAccessToken, apiTestSigningKey)
			if err != nil || claims.Subject != userID.String() || !claims.HasPermission(auth.PermissionReadMessages) {
				t.Fatalf("invalid renewed token: %v", err)
			}
			remaining := time.Until(claims.ExpiresAt.Time)
			if remaining < 14*time.Minute || remaining > internalAccessTokenLifetime {
				t.Fatalf("incorrect token lifetime: %s", remaining)
			}
			if _, err := time.Parse(time.RFC3339Nano, response.InternalAccessTokenExpiresAt); err != nil {
				t.Fatal(err)
			}
		})
	}
}
