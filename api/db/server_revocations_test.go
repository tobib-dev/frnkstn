package db

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3"
)

func TestMemberRemovalBatchRevokesOnlyForcedRemoval(t *testing.T) {
	session := gocqlx.NewSession(&gocql.Session{})
	store := DB{Session: &session}
	cursor := ServerUserCursor{UserID: gocql.TimeUUID(), ServerID: gocql.TimeUUID(), Cursor: time.Now()}
	for _, revoke := range []bool{false, true} {
		batch, err := store.memberRemovalBatch(context.Background(), cursor, revoke)
		if err != nil {
			t.Fatal(err)
		}
		if batch.Type != gocql.LoggedBatch {
			t.Fatal("removal must use a logged batch")
		}
		removed := map[string]bool{}
		revoked := false
		for _, entry := range batch.Entries {
			if strings.HasPrefix(entry.Stmt, "INSERT INTO revoked_server_members") {
				revoked = true
				if !reflect.DeepEqual(entry.Args, []any{cursor.ServerID, cursor.UserID}) {
					t.Fatalf("wrong revocation identity: %v", entry.Args)
				}
			}
			for _, table := range []string{
				"server_members", "server_moderators", "servers_by_user", "server_user_cursors",
			} {
				if strings.HasPrefix(entry.Stmt, "DELETE FROM "+table+" ") {
					removed[table] = true
				}
			}
		}
		if revoked != revoke || len(removed) != 4 {
			t.Fatalf("revoke=%v: revoked=%v, removed=%v", revoke, revoked, removed)
		}
	}
}
