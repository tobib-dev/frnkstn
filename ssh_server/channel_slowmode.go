package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	moderationsv1 "github.com/tobib-dev/frnkstn-proto/moderations"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type slowModeSavedMsg struct {
	seconds uint32
	err     error
}

func (m serversModel) updateSlowMode(msg tea.Msg) (serversModel, tea.Cmd, bool) {
	if result, ok := msg.(slowModeSavedMsg); ok {
		m.busy = false
		if result.err != nil {
			m.errorText = "Could not set slowmode. Please try again."
			if status.Code(result.err) == codes.PermissionDenied {
				m.errorText = "Only the server owner or a moderator can set slowmode."
			}
			return m, m.input.Focus(), true
		}
		m.page, m.errorText = serverChannels, ""
		m.notice = fmt.Sprintf("Channel slowmode set to %d seconds.", result.seconds)
		if result.seconds == 0 {
			m.notice = "Channel slowmode disabled."
		}
		m.input.Blur()
		return m, nil, true
	}
	key, isKey := msg.(tea.KeyPressMsg)
	if m.page == serverChannels && !m.busy && isKey && key.String() == "s" && len(m.channels) > 0 {
		m.page, m.notice, m.errorText = serverSlowMode, "", ""
		m.input.Prompt = "Cooldown seconds: "
		m.input.Reset()
		return m, m.input.Focus(), true
	}
	if m.page != serverSlowMode {
		return m, nil, false
	}
	if m.busy {
		return m, nil, true
	}
	if isKey {
		switch key.String() {
		case "esc":
			m.page, m.errorText = serverChannels, ""
			m.input.Blur()
			return m, nil, true
		case "enter":
			seconds, err := strconv.ParseUint(strings.TrimSpace(m.input.Value()), 10, 31)
			if err != nil {
				m.errorText = "Enter whole seconds from 0 to 2147483647 (0 disables slowmode)."
				return m, nil, true
			}
			m.busy, m.errorText = true, ""
			return m, m.setSlowMode(uint32(seconds)), true
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd, true
}

func (m serversModel) setSlowMode(seconds uint32) tea.Cmd {
	return func() tea.Msg {
		conn, err := newGRPCClient("localhost:" + m.grpcPort)
		if err != nil {
			return slowModeSavedMsg{err: err}
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		response, err := moderationsv1.NewModerationServiceClient(conn).SetSlowMode(
			bearerContext(ctx, m.accessToken), &moderationsv1.SetSlowModeRequest{
				ServerId: m.item.GetServerId(), ChannelId: m.channels[m.channelSelected].GetChannelId(),
				CooldownSeconds: seconds,
			})
		return slowModeSavedMsg{seconds: response.GetCooldownSeconds(), err: err}
	}
}
