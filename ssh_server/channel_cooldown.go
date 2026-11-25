package main

import (
	"context"
	"time"

	"charm.land/bubbles/v2/timer"
	tea "charm.land/bubbletea/v2"
	moderationsv1 "github.com/tobib-dev/frnkstn-proto/moderations"
	"google.golang.org/grpc"
)

type channelCooldown struct {
	timer               timer.Model
	serverID, channelID string
	seconds             uint32
}

type channelCooldownStatus struct {
	serverID, channelID string
	seconds             uint32
	wait                time.Duration
}

func (m serversModel) readCooldown(ctx context.Context, conn *grpc.ClientConn, result *serverResultMsg) {
	switch result.operation {
	case listChannelMessages, loadOlderChannelMessages, loadThread, openMessageThread,
		postChannelMessage, editChannelMessage, createThread, replyThread:
	default:
		return
	}
	if m.channelSelected < 0 || m.channelSelected >= len(m.channels) {
		return
	}
	channelID := m.channels[m.channelSelected].GetChannelId()
	serverID := m.item.GetServerId()
	response, err := moderationsv1.NewModerationServiceClient(conn).GetSlowMode(
		ctx, &moderationsv1.GetSlowModeRequest{ServerId: serverID, ChannelId: channelID})
	if err != nil {
		return
	}
	result.cooldown = &channelCooldownStatus{
		serverID: serverID, channelID: channelID, seconds: response.GetCooldownSeconds(),
		wait: time.Duration(response.GetRemainingMillis()) * time.Millisecond,
	}
}

func (m *serversModel) applyCooldown(state *channelCooldownStatus) tea.Cmd {
	if state == nil {
		return nil
	}
	// Round up so the countdown never announces readiness early.
	wait := ((max(0, state.wait) + time.Second - 1) / time.Second) * time.Second
	m.cooldown = channelCooldown{timer: timer.New(wait), serverID: state.serverID, channelID: state.channelID, seconds: state.seconds}
	if wait > 0 {
		return m.cooldown.timer.Init()
	}
	return nil
}

func (m serversModel) updateCooldownTick(msg tea.Msg) (serversModel, tea.Cmd, bool) {
	switch msg.(type) {
	case timer.TickMsg, timer.StartStopMsg, timer.TimeoutMsg:
		var cmd tea.Cmd
		m.cooldown.timer, cmd = m.cooldown.timer.Update(msg)
		return m, cmd, true
	}
	return m, nil, false
}
