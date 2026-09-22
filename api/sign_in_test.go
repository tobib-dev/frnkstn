package main

import (
	"context"
	"errors"
	"testing"

	"github.com/gocql/gocql"
	sessionsV1 "github.com/tobib-dev/frnkstn-proto/sessions/v1"
	usersV1 "github.com/tobib-dev/frnkstn-proto/users/v1"
	"github.com/tobib-dev/frnkstn/api/db"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type testUserStore struct {
	user     db.User
	err      error
	lookedUp int64
	creates  int
}

func (s *testUserStore) GetUserByGitHubID(_ context.Context, id int64) (db.User, error) {
	s.lookedUp = id
	return s.user, s.err
}
func (s *testUserStore) CreateUser(_ context.Context, user db.User) (db.User, error) {
	s.creates++
	user.ID = gocql.TimeUUID()
	s.user = user
	return user, s.err
}

type testSessionStore struct {
	params  db.CreateSessionParams
	creates int
}

func (s *testSessionStore) CreateSession(_ context.Context, params db.CreateSessionParams) (db.Session, error) {
	s.params = params
	s.creates++
	return db.Session{ID: gocql.TimeUUID(), UserID: params.UserID}, nil
}
func (s *testSessionStore) GetSession(context.Context, db.GetSessionParams) (db.Session, error) {
	return db.Session{}, gocql.ErrNotFound
}

func (s *testSessionStore) GetSessionByUser(context.Context, db.GetSessionByUserParams) (db.SessionByUser, error) {
	return db.SessionByUser{}, gocql.ErrNotFound
}

func (s *testSessionStore) UpdateSession(context.Context, db.UpdateSessionParams) (db.Session, error) {
	return db.Session{}, gocql.ErrNotFound
}

func TestGetUserByGitHubID(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		code codes.Code
	}{
		{"found", nil, codes.OK}, {"missing", gocql.ErrNotFound, codes.NotFound}, {"database failure", errors.New("database down"), codes.Internal},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &testUserStore{user: db.User{ID: gocql.TimeUUID(), Username: "alice"}, err: tt.err}
			service := &UserService{store: store}
			resp, err := service.GetUserByGHID(context.Background(), &usersV1.GetUserByGHIDRequest{GithubUserId: "123"})
			if status.Code(err) != tt.code || store.lookedUp != 123 {
				t.Fatalf("lookup=%d err=%v", store.lookedUp, err)
			}
			if err == nil && resp.UserId != store.user.ID.String() {
				t.Fatal("wrong local user returned")
			}
		})
	}
}

func TestCreateUserGeneratesLocalID(t *testing.T) {
	store := &testUserStore{}
	service := &UserService{store: store}
	resp, err := service.CreateUser(context.Background(), &usersV1.CreateUserRequest{GithubUserId: "123", Username: " alice ", Name: "Alice", UserId: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if store.user.GitHubID != 123 || store.user.Username != "alice" || store.user.Name != "Alice" || resp.UserId != store.user.ID.String() {
		t.Fatal("user was not created with a backend-generated ID")
	}
}

func TestCreateSessionRequiresUserID(t *testing.T) {
	id := gocql.TimeUUID()
	for _, tt := range []struct {
		name, requestedID string
		code              codes.Code
	}{
		{"owner", id.String(), codes.OK}, {"missing user", "", codes.InvalidArgument},
	} {
		t.Run(tt.name, func(t *testing.T) {
			store := &testSessionStore{}
			service := &SessionService{store: store}
			resp, err := service.CreateSession(context.Background(), &sessionsV1.CreateSessionRequest{UserId: tt.requestedID, AccessToken: "token", AccessTokenExpiresAt: "3600", RefreshToken: "refresh", RefreshTokenExpiresAt: "7200"})
			if status.Code(err) != tt.code {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.code != codes.OK && store.creates != 0 {
				t.Fatal("session created without user ID")
			}
			if tt.code == codes.OK && (resp.UserId != id.String() || store.params.UserID != id || store.params.RefreshToken != "refresh" || store.params.ExpiresIn.IsZero()) {
				t.Fatal("session missing user or token data")
			}
		})
	}
}
