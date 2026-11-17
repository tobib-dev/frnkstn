package db

import (
	"testing"
	"time"

	"github.com/gocql/gocql"
)

func TestRemovedMemberCleanupPreservesRejoin(t *testing.T) {
	serverID := gocql.TimeUUID()
	removedAt := time.Now().UTC()
	old := removedAt.Add(-time.Hour)
	for _, test := range []struct {
		name    string
		row     ServerByUser
		current time.Time
		cutoff  time.Time
		remove  bool
	}{
		{"old orphan", ServerByUser{ServerID: serverID, Cursor: old},
			time.Time{}, removedAt, true},
		{"rejoin", ServerByUser{ServerID: serverID, Cursor: removedAt},
			removedAt, removedAt, false},
		{"current cursor wins", ServerByUser{ServerID: serverID, Cursor: old},
			old, removedAt, false},
		{"different server", ServerByUser{ServerID: gocql.TimeUUID(), Cursor: old},
			time.Time{}, removedAt, false},
		{"purge includes soft-deleted rows", ServerByUser{
			ServerID: serverID, Cursor: old, DeletionTimestamp: &removedAt,
		}, old, time.Time{}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			current := ServerUserCursor{Cursor: test.current}
			got := shouldRemoveServerUserRow(test.row, current, serverID, test.cutoff)
			if got != test.remove {
				t.Fatalf("remove=%v, want %v", got, test.remove)
			}
		})
	}
}
