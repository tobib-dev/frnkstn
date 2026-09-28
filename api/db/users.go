package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type User struct {
	ID       gocql.UUID `db:"id"`
	GitHubID int64      `db:"github_id"`
	Name     string     `db:"name"`
	Username string     `db:"username"`
}

type UserStore interface {
	GetUserByGitHubID(context.Context, int64) (User, error)
	GetUserIDByUsername(context.Context, string) (gocql.UUID, error)
	GetUserByID(context.Context, gocql.UUID) (User, error)
	CreateUser(context.Context, User) (User, error)
	UpdateUser(context.Context, User, UpdateUserParams) (User, error)
	DeleteUser(context.Context, User) error
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
	GitHubID int64      `db:"github_id"`
	UserID   gocql.UUID `db:"user_id"`
	Name     string     `db:"name"`
	Username string     `db:"username"`
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
		"user_id",
	},
	PartKey: []string{"username"},
}

var usersByUsernameTable = table.New(usersByUsernameMetadata)

type UserByUsername struct {
	username string     `db:"username"`
	userid   gocql.UUID `db:"user_id"`
}

type UpdateUserParams struct {
	Username string
	Name     string
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

func (db *DB) GetUserIDByUsername(ctx context.Context, username string) (gocql.UUID, error) {
	userInfo := UserByUsername{username: username}
	if err := db.Session.Query(usersByUsernameTable.Get()).
		WithContext(ctx).
		BindStruct(userInfo).
		GetRelease(&userInfo); err != nil {
		return gocql.UUID{}, err
	}
	return userInfo.userid, nil
}

func (db *DB) GetUserByID(ctx context.Context, userID gocql.UUID) (User, error) {
	user := User{ID: userID}
	if err := db.Session.Query(usersTable.Get()).
		WithContext(ctx).
		BindStruct(user).
		GetRelease(&user); err != nil {
		return User{}, err
	}
	return user, nil
}

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

func (db *DB) UpdateUser(ctx context.Context, user User, args UpdateUserParams) (User, error) {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	// Update the user's username and name in the users table
	query := qb.Update(usersMetadata.Name).
		Set("username", "name").
		Where(qb.Eq("id")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"id":       user.ID,
		"username": args.Username,
		"name":     args.Name,
	}); err != nil {
		return User{}, err
	}
	// Update the user's username in the users_by_username table
	if user.Username != args.Username {
		deleteQuery := qb.Delete(usersByUsernameMetadata.Name).
			Where(qb.Eq("username")).
			Query(*db.Session)
		if err := batch.BindMap(deleteQuery, qb.M{
			"username": user.Username,
		}); err != nil {
			return User{}, err
		}
		insertQuery := qb.Insert(usersByUsernameMetadata.Name).
			Columns("username", "user_id").
			Query(*db.Session)
		if err := batch.BindMap(insertQuery, qb.M{
			"username": args.Username,
			"user_id":  user.ID,
		}); err != nil {
			return User{}, err
		}
	}
	// Update the user's username in the users_by_github_id table
	if user.GitHubID != 0 {
		query = qb.Update(usersByGitHubIDMetadata.Name).
			Set("username", "name").
			Where(qb.Eq("github_id")).
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"github_id": user.GitHubID,
			"username":  args.Username,
			"name":      args.Name,
		}); err != nil {
			return User{}, err
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return User{}, err
	}
	user.Username = args.Username
	user.Name = args.Name
	return user, nil
}

func (db *DB) DeleteUser(ctx context.Context, user User) error {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)

	// Delete user from users_by_github_id table
	query := qb.Delete(usersByGitHubIDMetadata.Name).
		Where(qb.Eq("github_id")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"github_id": user.GitHubID,
	}); err != nil {
		return err
	}
	// Delete user from users_by_username table
	query = qb.Delete(usersByUsernameMetadata.Name).
		Where(qb.Eq("username")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"username": user.Username,
	}); err != nil {
		return err
	}
	// Delete user from users table
	query = qb.Delete(usersMetadata.Name).
		Where(qb.Eq("id")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"id": user.ID,
	}); err != nil {
		return err
	}
	if err := db.Session.ExecuteBatch(batch); err != nil {
		return err
	}
	return nil
}
