package main

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestSlowModeForm(t *testing.T) {
	m := newServersModel("mod", bearerToken{}, "")
	m.busy, m.page = false, serverChannels
	m.channels = []*serversv1.ChannelItem{{ChannelId: "channel", ChannelName: "general"}}
	m, cmd := m.Update(tea.KeyPressMsg{Code: 's', Text: "s"})
	if m.page != serverSlowMode || cmd == nil {
		t.Fatal("slowmode form did not open")
	}
	for _, value := range []string{"", "-1", "1.5", "2147483648"} {
		m.input.SetValue(value)
		updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd != nil || updated.busy || updated.errorText == "" {
			t.Fatalf("accepted invalid cooldown %q", value)
		}
	}
	m.input.SetValue("0")
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil || !m.busy {
		t.Fatal("zero must submit disable request")
	}
	m, _ = m.Update(slowModeSavedMsg{err: status.Error(codes.PermissionDenied, "denied")})
	if m.page != serverSlowMode || m.busy || m.errorText == "" {
		t.Fatal("permission failure lost the form")
	}
	m, _ = m.Update(slowModeSavedMsg{seconds: 0})
	if m.page != serverChannels || m.notice != "Channel slowmode disabled." {
		t.Fatal("disable confirmation missing")
	}
}
