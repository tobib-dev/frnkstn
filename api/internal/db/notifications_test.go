package db

import (
	"strings"
	"testing"
)

func TestNotificationQueriesFilterBySeen(t *testing.T) {
	messageQuery, _ := msgN10nTable.Select()
	friendQuery, _ := friendN10nTable.Select()
	messageGet, _ := msgN10nTable.Get()
	friendGet, _ := friendN10nTable.Get()
	for name, query := range map[string]string{
		"message list": messageQuery,
		"friend list":  friendQuery,
		"message get":  messageGet,
		"friend get":   friendGet,
	} {
		if !strings.Contains(query, "WHERE user_id=? AND seen=?") {
			t.Errorf("%s notification query does not filter by seen: %s", name, query)
		}
	}
}
