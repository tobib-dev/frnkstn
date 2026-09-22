package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type User struct {
	ID       gocql.UUID
	GitHubID int64
	Name     string
	Username string
}

type UserStore interface {
	GetUserByGitHubID(context.Context, int64) (User, error)
	CreateUser(context.Context, User) (User, error)
}

var usersByGitHubIDMetadata = table.Metadata{
	Name: "users_by_github_id",
	Columns: []string{
		"github_id",
		"user_id",
		"name",
		"username",
	},
	PartKey: []string{"github_id"},
}

var usersByGitHubIDTable = table.New(usersByGitHubIDMetadata)

type UsersByGitHubID struct {
	GitHubID int64 `db:"github_id"`
	UserID   gocql.UUID
	Name     string
	Username string
}

func (db *DB) GetUserByGitHubID(ctx context.Context, githubID int64) (User, error) {
	usr := UsersByGitHubID{
		GitHubID: githubID,
	}
	err := db.Session.Query(usersByGitHubIDTable.Get()).WithContext(ctx).
		BindStruct(usr).GetRelease(&usr)
	if err != nil {
		return User{}, err
	}
	return User{
		ID:       usr.UserID,
		Name:     usr.Name,
		Username: usr.Username,
	}, nil
}

var usersMetadata = table.Metadata{
	Name: "users",
	Columns: []string{
		"id",
		"github_id",
		"name",
		"username",
	},
	PartKey: []string{"id"},
}

var usersTable = table.New(usersMetadata)

var usersByUsernameMetadata = table.Metadata{
	Name: "users_by_username",
	Columns: []string{
		"username",
		"id",
		"github_id",
		"name",
	},
	PartKey: []string{"username"},
}

var usersByUsernameTable = table.New(usersByUsernameMetadata)

// CreateUser is idempotent by GitHub identity, including concurrent sign-ins.
func (db *DB) CreateUser(ctx context.Context, user User) (User, error) {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	userInsert := qb.Insert(usersMetadata.Name).
		Columns("id", "github_id", "name", "username").
		Query(*db.Session)
	if err := batch.BindMap(userInsert, qb.M{
		"id":        user.ID,
		"github_id": user.GitHubID,
		"name":      user.Name,
		"username":  user.Username,
	}); err != nil {
		return User{}, err
	}

	if user.GitHubID != 0 {
		usersByGitHubIDInsert := qb.Insert(usersByGitHubIDMetadata.Name).
			Columns("github_id", "user_id", "name", "username").
			Query(*db.Session)
		if err := batch.BindMap(usersByGitHubIDInsert, qb.M{
			"github_id": user.GitHubID,
			"user_id":   user.ID,
			"name":      user.Name,
			"username":  user.Username,
		}); err != nil {
			return User{}, err
		}
	}

	if user.Username != "" {
		usersByUsernameInsert := qb.Insert(usersByUsernameMetadata.Name).
			Columns("username", "user_id").
			Query(*db.Session)
		if err := batch.BindMap(usersByUsernameInsert, qb.M{
			"username": user.Username,
			"user_id":  user.ID,
		}); err != nil {
			return User{}, err
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return User{}, err
	}
	return user, nil
}
