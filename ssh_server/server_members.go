package main

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
)

func (m serversModel) memberRequest(
	ctx context.Context, client serversv1.ServerServiceClient, result *serverResultMsg,
) {
	switch result.operation {
	case inviteMember:
		_, result.err = client.InviteMember(ctx, &serversv1.InviteMemberRequest{
			ServerId: m.item.GetServerId(), MemberId: strings.TrimSpace(m.input.Value()),
		})
	case previewServer:
		response, err := client.GetServerByCode(ctx, &serversv1.GetServerByCodeRequest{
			ShareCode: strings.TrimSpace(m.input.Value()),
		})
		result.err, result.item = err, response.GetItem()
	case joinServer:
		response, err := client.JoinServer(ctx, &serversv1.JoinServerRequest{
			ShareCode: strings.TrimSpace(m.input.Value()),
		})
		result.err, result.item = err, response.GetItem()
	case listMembers:
		response, err := client.GetMembers(ctx, &serversv1.GetMembersRequest{
			ServerId: m.item.GetServerId(),
		})
		result.err, result.members = err, response.GetItems()
	case leaveServer:
		_, result.err = client.LeaveServer(ctx, &serversv1.LeaveServerRequest{
			ServerId: m.item.GetServerId(),
		})
	case removeMember:
		_, result.err = client.DeleteMember(ctx, &serversv1.DeleteMemberRequest{
			ServerId: m.item.GetServerId(), MemberId: m.members[m.memberSelected].GetMemberId(),
		})
	}
}

func (m serversModel) updateMembers(msg tea.Msg) (serversModel, tea.Cmd, bool) {
	key, isKey := msg.(tea.KeyPressMsg)
	switch m.page {
	case serverInvite, serverJoin:
		if isKey && key.String() == "enter" {
			if strings.TrimSpace(m.input.Value()) == "" {
				m.errorText = "Please enter a value."
				return m, nil, true
			}
			op := inviteMember
			if m.page == serverJoin {
				op = previewServer
			}
			m.busy, m.errorText = true, ""
			return m, m.request(op, nil), true
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd, true
	case serverJoinConfirmation, serverLeaveConfirmation, serverRemoveConfirmation:
		if isKey {
			switch key.String() {
			case "y":
				op := joinServer
				if m.page == serverLeaveConfirmation {
					op = leaveServer
				} else if m.page == serverRemoveConfirmation {
					op = removeMember
				}
				m.busy, m.errorText = true, ""
				return m, m.request(op, nil), true
			case "n":
				switch m.page {
				case serverJoinConfirmation:
					m.page = serverJoin
				case serverLeaveConfirmation:
					m.page = serverDetails
				case serverRemoveConfirmation:
					m.page = serverMembers
				}
				m.errorText = ""
			}
		}
		return m, nil, true
	case serverMembers:
		if isKey {
			switch key.String() {
			case "up", "k":
				if m.memberSelected > 0 {
					m.memberSelected--
				}
			case "down", "j":
				if m.memberSelected < len(m.members)-1 {
					m.memberSelected++
				}
			case "r":
				m.busy = true
				return m, m.request(listMembers, nil), true
			case "d":
				// The API authorizes removal for owners and moderators.
				if len(m.members) > 0 {
					m.page = serverRemoveConfirmation
				}
			}
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m serversModel) membersView() string {
	switch m.page {
	case serverInvite:
		return "Invite member\n\n" + m.input.View() + "\n\nEnter invite · Esc cancel"
	case serverJoin:
		return "Join server\n\n" + m.input.View() + "\n\nEnter preview · Esc cancel"
	case serverJoinConfirmation:
		return "Join " + m.item.GetServerName() + "?\n\n" + m.item.GetServerBio() +
			"\n\ny join · n cancel · Esc back"
	case serverLeaveConfirmation:
		return "Leave " + m.item.GetServerName() + "?\n\ny leave · n cancel · Esc back"
	case serverRemoveConfirmation:
		return "Remove " + m.members[m.memberSelected].GetMemberName() +
			"?\n\ny remove · n cancel · Esc back"
	case serverMembers:
		var b strings.Builder
		b.WriteString(m.item.GetServerName() + " members\n\n")
		if len(m.members) == 0 {
			b.WriteString("No members.\n")
		}
		for i, member := range m.members {
			if i == m.memberSelected {
				b.WriteString("> ")
			} else {
				b.WriteString("  ")
			}
			b.WriteString(member.GetMemberName() + "\n")
		}
		b.WriteString("\n↑/↓ select · r refresh · d remove (owner/mod) · Esc back")
		return b.String()
	}
	return ""
}
