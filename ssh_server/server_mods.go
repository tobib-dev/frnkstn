package main

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
)

func (m serversModel) modRequest(
	ctx context.Context, client serversv1.ServerServiceClient, result *serverResultMsg,
) {
	switch result.operation {
	case listMods:
		response, err := client.GetMods(ctx, &serversv1.GetModsRequest{ServerId: m.item.GetServerId()})
		result.err, result.mods = err, response.GetItems()
	case getMod:
		response, err := client.GetMod(ctx, &serversv1.GetModRequest{
			ServerId: m.item.GetServerId(), ModId: m.mods[m.modSelected].GetModId(),
		})
		result.err, result.mod = err, response.GetItem()
	case addMod:
		response, err := client.AddMod(ctx, &serversv1.AddModRequest{
			ServerId: m.item.GetServerId(), ModUserId: m.members[m.memberSelected].GetMemberId(),
		})
		result.err, result.mod = err, response.GetItem()
	case deleteMod:
		_, result.err = client.DeleteMod(ctx, &serversv1.DeleteModRequest{
			ServerId: m.item.GetServerId(), ModId: m.mod.GetModId(),
			ModUserId: m.mod.GetModId(),
		})
	}
}

func (m serversModel) updateMods(msg tea.Msg) (serversModel, tea.Cmd, bool) {
	key, isKey := msg.(tea.KeyPressMsg)
	switch m.page {
	case serverMods:
		if !isKey {
			return m, nil, true
		}
		switch key.String() {
		case "up", "k":
			if m.modSelected > 0 {
				m.modSelected--
			}
		case "down", "j":
			if m.modSelected < len(m.mods)-1 {
				m.modSelected++
			}
		case "r":
			m.busy, m.errorText = true, ""
			return m, m.request(listMods, nil), true
		case "enter":
			if len(m.mods) > 0 {
				m.busy, m.errorText, m.notice = true, "", ""
				return m, m.request(getMod, nil), true
			}
		}
		return m, nil, true
	case serverModDetails:
		if isKey && key.String() == "d" && m.userID == m.item.GetOwner() &&
			m.mod.GetModId() != m.item.GetOwner() {
			m.page, m.errorText, m.notice = serverDemoteConfirmation, "", ""
		}
		return m, nil, true
	case serverPromoteConfirmation, serverDemoteConfirmation:
		if !isKey {
			return m, nil, true
		}
		switch key.String() {
		case "n":
			m.errorText = ""
			if m.page == serverPromoteConfirmation {
				m.page = serverMembers
			} else {
				m.page = serverModDetails
			}
		case "y":
			operation := addMod
			if m.page == serverDemoteConfirmation {
				if m.userID != m.item.GetOwner() || m.mod.GetModId() == m.item.GetOwner() {
					return m, nil, true
				}
				operation = deleteMod
			}
			m.busy, m.errorText = true, ""
			return m, m.request(operation, nil), true
		}
		return m, nil, true
	}
	return m, nil, false
}

func (m serversModel) modsView() string {
	switch m.page {
	case serverMods:
		var b strings.Builder
		b.WriteString(m.item.GetServerName() + " moderators\n\n")
		if len(m.mods) == 0 && !m.busy {
			b.WriteString("No moderators.\n")
		}
		for i, mod := range m.mods {
			if i == m.modSelected {
				b.WriteString("> ")
			} else {
				b.WriteString("  ")
			}
			b.WriteString(mod.GetModName())
			if mod.GetModId() == m.item.GetOwner() {
				b.WriteString(" (owner)")
			}
			b.WriteString("\n")
		}
		b.WriteString("\n↑/↓ select · Enter details · r refresh · Esc back")
		return b.String()
	case serverModDetails:
		view := "Moderator: " + m.mod.GetModName()
		if m.mod.GetModId() == m.item.GetOwner() {
			view += " (owner)"
		}
		if m.userID == m.item.GetOwner() && m.mod.GetModId() != m.item.GetOwner() {
			view += "\n\nd demote"
		}
		return view + "\nEsc moderators"
	case serverPromoteConfirmation:
		return "Promote " + m.members[m.memberSelected].GetMemberName() +
			" to moderator?\n\ny promote · n cancel · Esc back"
	case serverDemoteConfirmation:
		return "Demote " + m.mod.GetModName() +
			"?\nThey will remain a server member.\n\ny demote · n cancel · Esc back"
	}
	return ""
}
