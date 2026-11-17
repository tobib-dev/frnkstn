package db

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3"
)

func TestModeratorRoleBatch(t *testing.T) {
	session := gocqlx.NewSession(&gocql.Session{})
	store := DB{Session: &session}
	member := Member{
		ServerID: gocql.TimeUUID(), MemberID: gocql.TimeUUID(), MemberName: "alice", Role: "member",
	}
	for _, promote := range []bool{true, false} {
		batch, err := store.moderatorRoleBatch(context.Background(), member, promote)
		if err != nil {
			t.Fatal(err)
		}
		if batch.Type != gocql.LoggedBatch || len(batch.Entries) != 2 {
			t.Fatal("role and moderator writes must share a logged batch")
		}
		role, operation := "member", "DELETE FROM server_moderators"
		modArgs := []interface{}{member.ServerID, member.MemberID}
		if promote {
			role, operation = "moderator", "INSERT INTO server_moderators"
			modArgs = append(modArgs, member.MemberName, role)
		}
		modWrite, memberWrite := batch.Entries[0], batch.Entries[1]
		if !strings.HasPrefix(modWrite.Stmt, operation) || !reflect.DeepEqual(modWrite.Args, modArgs) {
			t.Fatalf("incorrect moderator write: %+v", modWrite)
		}
		if memberWrite.Stmt != "UPDATE server_members SET role=? WHERE server_id=? AND member_id=? " ||
			!reflect.DeepEqual(memberWrite.Args, []interface{}{role, member.ServerID, member.MemberID}) {
			t.Fatalf("incorrect member role write: %+v", memberWrite)
		}
	}
}
