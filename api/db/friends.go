package db

import (
	"context"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3/qb"
	"github.com/scylladb/gocqlx/v3/table"
)

type FriendStore interface {
	AddFriend(context.Context, Friend) (Friend, error)
	AcceptFriend(context.Context, Friend) (Friend, error)
	RejectFriend(context.Context, Friend) (Friend, error)
	RemoveFriend(context.Context, gocql.UUID, gocql.UUID) error
	GetFriend(context.Context, gocql.UUID) (Friend, error)
	GetFriends(context.Context, gocql.UUID) ([]Friend, error)
}

type Friend struct {
	ID         gocql.UUID `db:"id"`
	UserID     gocql.UUID `db:"user_id"`
	FriendID   gocql.UUID `db:"friend_id"`
	FriendName string     `db:"friend_name"`
	Status     string     `db:"status"`
}

var friendsMetadata = table.Metadata{
	Name: "friends",
	Columns: []string{
		"id",
		"user_id",
		"friend_id",
		"friend_name",
		"status",
	},
	PartKey: []string{"id"},
}

var friendsTable = table.New(friendsMetadata)

var friendsByUserIDMetadata = table.Metadata{
	Name: "friends_by_user_id",
	Columns: []string{
		"user_id",
		"friend_id",
		"friendship_id",
		"friend_name",
		"status",
	},
	PartKey: []string{"user_id"},
}

var friendsByUserIDTable = table.New(friendsByUserIDMetadata)

// Add friend is idempotent
func (db *DB) AddFriend(ctx context.Context, friend Friend) (Friend, error) {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	friend.Status = "pending"

	query := qb.Insert(friendsMetadata.Name).
		Columns("id", "user_id", "friend_id", "friend_name", "status").
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"id":          friend.ID,
		"user_id":     friend.UserID,
		"friend_id":   friend.FriendID,
		"friend_name": friend.FriendName,
		"status":      friend.Status,
	}); err != nil {
		return Friend{}, err
	}

	if (friend.UserID != gocql.UUID{}) {
		query := qb.Insert(friendsByUserIDMetadata.Name).
			Columns("user_id", "friend_id", "friendship_id", "friend_name", "status").
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":       friend.UserID,
			"friend_id":     friend.FriendID,
			"friendship_id": friend.ID,
			"friend_name":   friend.FriendName,
			"status":        friend.Status,
		}); err != nil {
			return Friend{}, err
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return Friend{}, err
	}
	return friend, nil
}

// AcceptFriend accepts a friend request
func (db *DB) AcceptFriend(ctx context.Context, friend Friend) (Friend, error) {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	friend.Status = "accepted"

	// Update status to accepted on the friends table
	query := qb.Update(friendsMetadata.Name).
		Set("status").
		Where(qb.Eq("id")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"id":     friend.ID,
		"status": friend.Status,
	}); err != nil {
		return Friend{}, err
	}

	// Update status to accepted on the friendsByUserID table
	if (friend.UserID != gocql.UUID{}) {
		query := qb.Update(friendsByUserIDMetadata.Name).
			Set("status").
			Where(qb.Eq("user_id"), qb.Eq("friendship_id")).
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":       friend.UserID,
			"friendship_id": friend.ID,
			"status":        friend.Status,
		}); err != nil {
			return Friend{}, err
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return Friend{}, err
	}

	return friend, nil
}

func (db *DB) RejectFriend(ctx context.Context, friend Friend) (Friend, error) {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	friend.Status = "rejected"

	// Update status to rejected on the friends table
	query := qb.Update(friendsMetadata.Name).
		Set("status").
		Where(qb.Eq("id")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"id":     friend.ID,
		"status": friend.Status,
	}); err != nil {
		return Friend{}, err
	}

	// Update status to rejected on the friendsByUserID table
	if (friend.UserID != gocql.UUID{}) {
		query := qb.Update(friendsByUserIDMetadata.Name).
			Set("status").
			Where(qb.Eq("user_id"), qb.Eq("friendship_id")).
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":       friend.UserID,
			"friendship_id": friend.ID,
			"status":        friend.Status,
		}); err != nil {
			return Friend{}, err
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return Friend{}, err
	}

	return friend, nil
}

func (db *DB) RemoveFriend(ctx context.Context, friendshipID gocql.UUID, userID gocql.UUID) error {
	batch := db.Session.ContextBatch(ctx, gocql.LoggedBatch)
	status := "removed"

	// Update status to removed on the friends table
	query := qb.Update(friendsMetadata.Name).
		Set("status").
		Where(qb.Eq("id")).
		Query(*db.Session)
	if err := batch.BindMap(query, qb.M{
		"id":     friendshipID,
		"status": status,
	}); err != nil {
		return err
	}

	// Update status to removed on the friendsByUserID table
	if (userID != gocql.UUID{}) {
		query := qb.Update(friendsByUserIDMetadata.Name).
			Set("status").
			Where(qb.Eq("user_id"), qb.Eq("friendship_id")).
			Query(*db.Session)
		if err := batch.BindMap(query, qb.M{
			"user_id":       userID,
			"friendship_id": friendshipID,
			"status":        status,
		}); err != nil {
			return err
		}
	}

	if err := db.Session.ExecuteBatch(batch); err != nil {
		return err
	}

	return nil
}

// return a single friend by friendship ID
func (db *DB) GetFriend(ctx context.Context, friendshipID gocql.UUID) (Friend, error) {
	friend := Friend{ID: friendshipID}
	if err := db.Session.Query(friendsTable.Get()).
		WithContext(ctx).
		BindStruct(friend).
		GetRelease(&friend); err != nil {
		return Friend{}, err
	}
	return friend, nil
}

// return all friends for a given user
func (db *DB) GetFriends(ctx context.Context, userID gocql.UUID) ([]Friend, error) {
	friends := []Friend{}
	if err := db.Session.Query(friendsByUserIDTable.Select()).
		WithContext(ctx).
		BindMap(qb.M{"user_id": userID}).
		SelectRelease(&friends); err != nil {
		return nil, err
	}
	return friends, nil
}
