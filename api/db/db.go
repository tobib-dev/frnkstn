package db

import (
	"fmt"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3"
)

const keyspace = "frnkstn"

type DB struct {
	Session *gocqlx.Session
}

func New(dbURL, dbUser, dbPassword string) (*DB, error) {
	cluster := gocql.NewCluster(dbURL)
	cluster.Keyspace = keyspace
	cluster.Consistency = gocql.Quorum
	cluster.Authenticator = gocql.PasswordAuthenticator{
		Username: dbUser,
		Password: dbPassword,
	}

	session, err := gocqlx.WrapSession(cluster.CreateSession())
	if err != nil {
		return &DB{}, fmt.Errorf("error initializing DB: %v", err)
	}

	return &DB{Session: &session}, nil
}

func (db *DB) Close() {
	db.Session.Close()
}
