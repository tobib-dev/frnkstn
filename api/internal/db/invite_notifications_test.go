package db

import (
	"context"
	"reflect"
	"testing"

	"github.com/gocql/gocql"
	"github.com/scylladb/gocqlx/v3"
)

func TestJoinMarksOnlyMatchingInvitesSeen(t *testing.T) {
	session := gocqlx.NewSession(&gocql.Session{})
	store := DB{Session: &session}
	invite := ShareCode{Receiver: gocql.TimeUUID(), ServerID: gocql.TimeUUID(), Code: "accepted"}
	matched := InviteNotification{
		UserID: invite.Receiver, ServerID: invite.ServerID, ShareCode: invite.Code,
		NotificationID: gocql.TimeUUID(),
	}
	otherUser, otherServer, otherCode, duplicate := matched, matched, matched, matched
	otherUser.UserID = gocql.TimeUUID()
	otherServer.ServerID = gocql.TimeUUID()
	otherCode.ShareCode = "unaccepted"
	duplicate.NotificationID = gocql.TimeUUID()
	batch := session.ContextBatch(context.Background(), gocql.LoggedBatch)
	err := store.bindSeenInvites(batch, []InviteNotification{
		matched, otherUser, otherServer, otherCode, duplicate,
	}, invite)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Entries) != 2 {
		t.Fatalf("wrong invitations removed: %+v", batch.Entries)
	}
	statement, _ := inviteNotificationTable.Delete()
	for i, item := range []InviteNotification{matched, duplicate} {
		entry := batch.Entries[i]
		if entry.Stmt != statement || !reflect.DeepEqual(entry.Args, []any{item.UserID, item.NotificationID}) {
			t.Fatalf("wrong notification deletion: %+v", entry)
		}
	}
}
