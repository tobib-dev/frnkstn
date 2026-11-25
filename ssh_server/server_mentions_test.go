package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
)

func TestMentionPickerInsertsMemberAndPreservesMessage(t *testing.T) {
	m := newServersModel("user", bearerToken{}, "")
	m.messageInput.SetValue("Hi @to please help")
	m.messageInput.SetCursor(len([]rune("Hi @to")))
	m.mentionStart, m.mentionOpen = 3, true
	m.mentionMembers = []*serversv1.MemberItem{
		{MemberId: "alice", MemberName: "alice"},
		{MemberId: "tobi", MemberName: "tobib-dev"},
	}
	updated, _, handled := m.updateMentions(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled || updated.mentionOpen || updated.messageInput.Value() != "Hi @tobib-dev  please help" {
		t.Fatalf("unexpected mention insertion: %q, open=%v", updated.messageInput.Value(), updated.mentionOpen)
	}
}
