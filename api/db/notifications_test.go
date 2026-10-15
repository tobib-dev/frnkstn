package db

import (
	"strings"
	"testing"
)

func TestNotificationQueriesFilterBySeen(t *testing.T) {
	messageQuery, _ := msgN10nTable.Select()
	friendQuery, _ := friendN10nTable.Select()
	for name, query := range map[string]string{
		"message": messageQuery,
		"friend":  friendQuery,
	} {
		if !strings.Contains(query, "WHERE user_id=? AND seen=?") {
			t.Errorf("%s notification query does not filter by seen: %s", name, query)
		}
	}
}
