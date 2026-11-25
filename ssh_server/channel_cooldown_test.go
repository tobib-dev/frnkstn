package main

import (
	"testing"
	"time"

	"charm.land/bubbles/v2/timer"
)

func TestChannelCooldownCountdown(t *testing.T) {
	m := serversModel{}
	cmd := m.applyCooldown(&channelCooldownStatus{
		serverID: "server", channelID: "channel", seconds: 30, wait: 1500 * time.Millisecond,
	})
	if cmd == nil || m.cooldown.timer.Timeout != 2*time.Second {
		t.Fatal("countdown must round up the remaining wait")
	}
	m, _, _ = m.updateCooldownTick(timer.TickMsg{ID: m.cooldown.timer.ID()})
	if !m.cooldown.timer.Running() || m.cooldown.timer.Timeout != time.Second {
		t.Fatal("countdown expired early")
	}
	m, _, _ = m.updateCooldownTick(timer.TickMsg{ID: m.cooldown.timer.ID()})
	if m.cooldown.timer.Running() {
		t.Fatal("countdown did not expire")
	}
	if m.applyCooldown(&channelCooldownStatus{seconds: 0}) != nil || m.cooldown.timer.Running() {
		t.Fatal("disabled slowmode should not run a timer")
	}
}
