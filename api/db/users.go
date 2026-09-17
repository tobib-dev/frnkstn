package db

import (
	"context"

	"github.com/gocql/gocql"
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

func (db *DB) GetUserByGitHubID(ctx context.Context, githubID int64) (User, error) {
	user := User{GitHubID: githubID}
	err := db.Session.Session.Query(
		"SELECT user_id, name, username FROM users_by_github_id WHERE github_id = ?", githubID,
	).WithContext(ctx).Scan(&user.ID, &user.Name, &user.Username)
	return user, err
}

// CreateUser is idempotent by GitHub identity, including concurrent sign-ins.
func (db *DB) CreateUser(ctx context.Context, user User) (User, error) {
	user.ID = gocql.TimeUUID()
	applied, err := db.Session.Session.Query(
		"INSERT INTO users_by_github_id (github_id, user_id, name, username) VALUES (?, ?, ?, ?) IF NOT EXISTS",
		user.GitHubID, user.ID, user.Name, user.Username,
	).WithContext(ctx).MapScanCAS(map[string]interface{}{})
	if err != nil {
		return User{}, err
	}
	if !applied {
		return db.GetUserByGitHubID(ctx, user.GitHubID)
	}
	return user, nil
}
