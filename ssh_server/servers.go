package main

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type serversPage int

const (
	serversList serversPage = iota
	serverDetails
	serverCreate
	serverEdit
	serverDeleteConfirmation
	serverInvite
	serverJoin
	serverJoinConfirmation
	serverMembers
	serverLeaveConfirmation
	serverRemoveConfirmation
	serverMods
	serverModDetails
	serverPromoteConfirmation
	serverDemoteConfirmation
	serverChannels
	serverCreateChannel
	serverEditChannel
	serverDeleteChannelConfirmation
)

type serverOperation int

const (
	listServers serverOperation = iota
	getServer
	createServer
	editServer
	deleteServer
	inviteMember
	previewServer
	joinServer
	listMembers
	leaveServer
	removeMember
	listFriends
	listMods
	getMod
	addMod
	deleteMod
	listChannels
	addChannel
	editChannel
	deleteChannel
)

type serverResultMsg struct {
	operation serverOperation
	items     []*serversv1.UserServerItem
	item      *serversv1.ServerItem
	members   []*serversv1.MemberItem
	mods      []*serversv1.ModItem
	channels  []*serversv1.ChannelItem
	mod       *serversv1.ModItem
	friends   []*friendsv1.GetFriendsItem
	err       error
}

type serversModel struct {
	userID, grpcPort string
	accessToken      bearerToken
	page             serversPage
	items            []*serversv1.UserServerItem
	selected         int
	item             *serversv1.ServerItem
	name, bio        textinput.Model
	input            textinput.Model
	members          []*serversv1.MemberItem
	memberSelected   int
	mods             []*serversv1.ModItem
	modSelected      int
	mod              *serversv1.ModItem
	channels         []*serversv1.ChannelItem
	channelSelected  int
	friends          []*friendsv1.GetFriendsItem
	friendSelected   int
	notice           string
	focusBio         bool
	busy             bool
	errorText        string
}

func newServersModel(userID string, accessToken bearerToken, grpcPort string) serversModel {
	name, bio := textinput.New(), textinput.New()
	input := textinput.New()
	input.SetVirtualCursor(true)
	name.Prompt, bio.Prompt = "Name: ", "Bio: "
	name.SetVirtualCursor(true)
	bio.SetVirtualCursor(true)
	return serversModel{
		userID: userID, accessToken: accessToken, grpcPort: grpcPort,
		name: name, bio: bio, input: input, busy: true,
	}
}

func (m serversModel) request(operation serverOperation, item *serversv1.ServerItem) tea.Cmd {
	return func() tea.Msg {
		result := serverResultMsg{operation: operation}
		conn, err := newGRPCClient("localhost:" + m.grpcPort)
		if err != nil {
			result.err = err
			return result
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		ctx = bearerContext(ctx, m.accessToken)
		client := serversv1.NewServerServiceClient(conn)
		switch operation {
		case listServers:
			response, err := client.GetUserServers(ctx, &serversv1.GetUserServersRequest{})
			result.err = err
			if err == nil {
				result.items = response.GetItems()
			}
		case getServer:
			response, err := client.GetServer(ctx, &serversv1.GetServerRequest{ServerId: item.GetServerId()})
			result.err = err
			if err == nil {
				result.item = response.GetItem()
			}
		case createServer:
			response, err := client.CreateServer(ctx, &serversv1.CreateServerRequest{
				ServerName: item.GetServerName(), ServerBio: item.GetServerBio(),
			})
			result.err = err
			if err == nil {
				result.item = response.GetItem()
			}
		case editServer:
			response, err := client.EditServer(ctx, &serversv1.EditServerRequest{Item: item})
			result.err = err
			if err == nil {
				result.item = response.GetItem()
			}
		case deleteServer:
			_, result.err = client.DeleteServer(ctx, &serversv1.DeleteServerRequest{ServerId: item.GetServerId()})
		case listChannels:
			response, err := client.GetChannels(ctx, &serversv1.GetChannelsRequest{ServerId: m.item.GetServerId()})
			result.err = err
			if err == nil {
				result.channels = response.GetItems()
			}
		case addChannel:
			response, err := client.AddChannel(ctx, &serversv1.AddChannelRequest{ServerId: m.item.GetServerId(), ChannelName: strings.TrimSpace(m.name.Value())})
			result.err = err
			if err == nil {
				if response.GetItem() == nil {
					result.err = status.Error(codes.Internal, "channel response is missing its item")
					break
				}
				result.channels = append(m.channels, response.GetItem())
			}
		case editChannel:
			channel := &serversv1.ChannelItem{
				ServerId: m.item.GetServerId(), ChannelId: m.channels[m.channelSelected].GetChannelId(),
				ChannelName: strings.TrimSpace(m.name.Value()),
			}
			response, err := client.EditChannel(ctx, &serversv1.EditChannelRequest{Item: channel})
			result.err = err
			if err == nil {
				if response.GetItem() == nil {
					result.err = status.Error(codes.Internal, "channel response is missing its item")
					break
				}
				result.channels = append([]*serversv1.ChannelItem(nil), m.channels...)
				result.channels[m.channelSelected] = response.GetItem()
			}
		case deleteChannel:
			channel := m.channels[m.channelSelected]
			_, result.err = client.DeleteChannel(ctx, &serversv1.DeleteChannelRequest{ServerId: channel.GetServerId(), ChannelId: channel.GetChannelId()})
			if result.err == nil {
				result.channels = append(append([]*serversv1.ChannelItem(nil), m.channels[:m.channelSelected]...), m.channels[m.channelSelected+1:]...)
			}
		case listFriends:
			response, err := friendsv1.NewFriendServiceClient(conn).GetFriends(ctx, &friendsv1.GetFriendsRequest{})
			result.err = err
			if err == nil {
				for _, friend := range response.GetItems() {
					if friend.GetStatus() == "accepted" {
						result.friends = append(result.friends, friend)
					}
				}
			}
		case listMods, getMod, addMod, deleteMod:
			m.modRequest(ctx, client, &result)
		default:
			m.memberRequest(ctx, client, &result)
		}
		wantsItem := operation == getServer || operation == createServer || operation == editServer ||
			operation == previewServer || operation == joinServer
		if result.err == nil && wantsItem && result.item == nil {
			result.err = status.Error(codes.Internal, "server response is missing its item")
		}
		if result.err == nil && (operation == getMod || operation == addMod) && result.mod == nil {
			result.err = status.Error(codes.Internal, "moderator response is missing its item")
		}
		return result
	}
}

func (m serversModel) Update(msg tea.Msg) (serversModel, tea.Cmd) {
	if result, ok := msg.(serverResultMsg); ok {
		m.busy = false
		if result.err != nil {
			m.errorText = "Could not complete server request. Please try again."
			switch status.Code(result.err) {
			case codes.PermissionDenied:
				m.errorText = "You do not have permission to perform this action."
			case codes.NotFound:
				m.errorText = "Server, member, moderator, or invitation was not found."
			case codes.AlreadyExists:
				m.errorText = "The user already belongs to this server."
				if result.operation == addMod {
					m.errorText = "This member is already a moderator."
				}
			case codes.InvalidArgument, codes.FailedPrecondition:
				m.errorText = status.Convert(result.err).Message()
			case codes.Unauthenticated:
				m.errorText = "Session expired. Sign out and sign in again."
			}
			return m, nil
		}
		m.errorText = ""
		switch result.operation {
		case listServers:
			m.items, m.selected = result.items, 0
		case getServer, createServer, editServer, joinServer:
			m.item, m.page = result.item, serverDetails
			found := false
			for _, row := range m.items {
				if row.ServerId == result.item.ServerId {
					row.ServerName = result.item.ServerName
					found = true
				}
			}
			if !found {
				m.items = append(m.items, &serversv1.UserServerItem{
					ServerId: result.item.ServerId, ServerName: result.item.ServerName,
				})
			}
		case previewServer:
			m.item, m.page = result.item, serverJoinConfirmation
		case inviteMember:
			m.page, m.notice = serverDetails, "Invitation created."
		case listFriends:
			m.friends, m.friendSelected = result.friends, 0
		case listMembers:
			m.members, m.memberSelected, m.page = result.members, 0, serverMembers
		case listMods:
			m.mods, m.modSelected, m.page = result.mods, 0, serverMods
		case getMod:
			m.mod, m.page = result.mod, serverModDetails
		case addMod, deleteMod:
			m.page, m.busy = serverMods, true
			m.notice = "Moderator added."
			if result.operation == deleteMod {
				m.notice = "Moderator demoted. They remain a server member."
			}
			return m, m.request(listMods, nil)
		case removeMember:
			m.page, m.busy = serverMembers, true
			return m, m.request(listMembers, nil)
		case deleteServer, leaveServer:
			for i, row := range m.items {
				if row.ServerId == m.item.ServerId {
					m.items = append(m.items[:i], m.items[i+1:]...)
					break
				}
			}
			m.item, m.page, m.selected = nil, serversList, 0
		case listChannels, addChannel, editChannel, deleteChannel:
			m.channels, m.page, m.channelSelected = result.channels, serverChannels, 0
		}
		return m, nil
	}
	if m.busy {
		return m, nil
	}
	key, isKey := msg.(tea.KeyPressMsg)
	if isKey && key.String() == "esc" {
		m.errorText = ""
		if m.page == serverCreateChannel || m.page == serverEditChannel || m.page == serverDeleteChannelConfirmation {
			m.page = serverChannels
		} else if m.page == serverRemoveConfirmation || m.page == serverPromoteConfirmation {
			m.page = serverMembers
		} else if m.page == serverDemoteConfirmation {
			m.page = serverModDetails
		} else if m.page == serverModDetails {
			m.page = serverMods
		} else if m.page == serverMods {
			m.page = serverDetails
		} else if m.page == serverChannels {
			m.page = serverDetails
		} else if m.page == serverJoinConfirmation {
			m.page = serverJoin
		} else if m.page == serverEdit || m.page == serverDeleteConfirmation ||
			m.page == serverInvite || m.page == serverMembers || m.page == serverLeaveConfirmation {
			m.page = serverDetails
		} else {
			m.page = serversList
		}
		return m, nil
	}
	if updated, cmd, handled := m.updateMods(msg); handled {
		return updated, cmd
	}
	if updated, cmd, handled := m.updateMembers(msg); handled {
		return updated, cmd
	}
	if m.page == serverCreate || m.page == serverEdit || m.page == serverCreateChannel || m.page == serverEditChannel {
		if isKey {
			switch key.String() {
			case "tab", "shift+tab":
				m.focusBio = !m.focusBio
				m.name.Blur()
				m.bio.Blur()
				if m.focusBio {
					return m, m.bio.Focus()
				}
				return m, m.name.Focus()
			case "enter":
				name := strings.TrimSpace(m.name.Value())
				if name == "" {
					m.errorText = "Server name is required."
					if m.page == serverCreateChannel || m.page == serverEditChannel {
						m.errorText = "Channel name is required."
					}
					return m, nil
				}
				if m.page == serverCreateChannel || m.page == serverEditChannel {
					if m.item.Owner != m.userID {
						return m, nil
					}
					op := addChannel
					if m.page == serverEditChannel {
						op = editChannel
					}
					m.busy, m.errorText = true, ""
					return m, m.request(op, nil)
				}
				item := &serversv1.ServerItem{ServerName: name, ServerBio: strings.TrimSpace(m.bio.Value())}
				op := createServer
				if m.page == serverEdit {
					item.ServerId, item.Owner = m.item.ServerId, m.item.Owner
					op = editServer
				}
				m.busy, m.errorText = true, ""
				return m, m.request(op, item)
			}
		}
		var cmd tea.Cmd
		if m.focusBio {
			m.bio, cmd = m.bio.Update(msg)
		} else {
			m.name, cmd = m.name.Update(msg)
		}
		return m, cmd
	}
	if !isKey {
		return m, nil
	}
	switch m.page {
	case serversList:
		switch key.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.items)-1 {
				m.selected++
			}
		case "c":
			m.page, m.notice = serverJoin, ""
			m.input.Prompt = "Share code: "
			m.input.SetValue("")
			return m, m.input.Focus()
		case "r":
			m.busy = true
			return m, m.request(listServers, nil)
		case "n":
			m.page = serverCreate
			m.name.SetValue("")
			m.bio.SetValue("")
			m.focusBio = false
			m.bio.Blur()
			return m, m.name.Focus()
		case "enter":
			if len(m.items) > 0 {
				m.busy = true
				return m, m.request(getServer, &serversv1.ServerItem{ServerId: m.items[m.selected].ServerId})
			}
		}
	case serverDetails:
		m.notice = ""
		switch key.String() {
		case "i":
			m.page, m.busy, m.notice = serverInvite, true, ""
			return m, m.request(listFriends, nil)
		case "o":
			m.mods, m.modSelected, m.mod = nil, 0, nil
			m.page, m.busy, m.errorText = serverMods, true, ""
			return m, m.request(listMods, nil)
		case "m":
			m.busy = true
			return m, m.request(listMembers, nil)
		case "c":
			m.busy, m.page = true, serverChannels
			return m, m.request(listChannels, nil)
		case "l":
			if m.item.Owner != m.userID {
				m.page = serverLeaveConfirmation
			}
		}
		if m.item.Owner == m.userID {
			switch key.String() {
			case "e":
				m.page = serverEdit
				m.name.SetValue(m.item.ServerName)
				m.bio.SetValue(m.item.ServerBio)
				m.focusBio = false
				m.bio.Blur()
				return m, m.name.Focus()
			case "d":
				m.page = serverDeleteConfirmation
			}
		}
	case serverDeleteConfirmation:
		if key.String() == "y" && m.item.Owner == m.userID {
			m.busy = true
			return m, m.request(deleteServer, m.item)
		}
		if key.String() == "n" {
			m.page = serverDetails
		}
	case serverChannels:
		switch key.String() {
		case "up", "k":
			if m.channelSelected > 0 {
				m.channelSelected--
			}
		case "down", "j":
			if m.channelSelected < len(m.channels)-1 {
				m.channelSelected++
			}
		case "r":
			m.busy = true
			return m, m.request(listChannels, nil)
		case "n":
			if m.item.Owner == m.userID {
				m.name.SetValue("")
				m.page = serverCreateChannel
				return m, m.name.Focus()
			}
		case "e":
			if m.item.Owner == m.userID && len(m.channels) > 0 {
				m.name.SetValue(m.channels[m.channelSelected].GetChannelName())
				m.page = serverEditChannel
				return m, m.name.Focus()
			}
		case "d":
			if m.item.Owner == m.userID && len(m.channels) > 0 {
				m.page = serverDeleteChannelConfirmation
			}
		}
	case serverDeleteChannelConfirmation:
		switch key.String() {
		case "y":
			if m.item.Owner == m.userID && len(m.channels) > 0 {
				m.busy = true
				return m, m.request(deleteChannel, nil)
			}
		case "n":
			m.page = serverChannels
		}
	}
	return m, nil
}

func (m serversModel) View() string {
	var b strings.Builder
	b.WriteString("Servers\n\n")
	switch m.page {
	case serversList:
		if len(m.items) == 0 && !m.busy {
			b.WriteString("You have no servers yet.\n")
		}
		for i, row := range m.items {
			if i == m.selected {
				b.WriteString("> ")
			} else {
				b.WriteString("  ")
			}
			b.WriteString(row.ServerName + "\n")
		}
		b.WriteString("\n↑/↓ select · Enter open · n create · c join by code · r refresh · Esc home")
	case serverCreate, serverEdit:
		if m.page == serverCreate {
			b.WriteString("Create server\n\n")
		} else {
			b.WriteString("Edit server\n\n")
		}
		b.WriteString(m.name.View() + "\n" + m.bio.View() + "\n\nTab switch field · Enter save · Esc cancel")
	case serverDetails:
		b.WriteString(m.item.ServerName + "\n\n" + m.item.ServerBio)
		if m.item.Owner == m.userID {
			b.WriteString("\n\ne edit · d delete")
		}
		b.WriteString("\ni invite · m members · o moderators")
		b.WriteString(" · c channels")
		if m.item.Owner != m.userID {
			b.WriteString(" · l leave")
		}
		b.WriteString("\nEsc server list")
	case serverDeleteConfirmation:
		b.WriteString("Delete " + m.item.ServerName + "?\n\ny confirm · n cancel · Esc cancel")
	case serverChannels:
		b.WriteString("#" + m.item.ServerName + " channels\n\n")
		if len(m.channels) == 0 {
			b.WriteString("No channels yet.\n")
		}
		for i, channel := range m.channels {
			if i == m.channelSelected {
				b.WriteString("> ")
			} else {
				b.WriteString("  ")
			}
			b.WriteString("#" + channel.GetChannelName() + "\n")
		}
		b.WriteString("\n↑/↓ select · r refresh")
		if m.item.Owner == m.userID {
			b.WriteString(" · n create · e rename · d delete")
		}
		b.WriteString(" · Esc server")
	case serverCreateChannel, serverEditChannel:
		if m.page == serverCreateChannel {
			b.WriteString("Create channel\n\n")
		} else {
			b.WriteString("Rename channel\n\n")
		}
		b.WriteString(m.name.View() + "\n\nEnter save · Esc cancel")
	case serverDeleteChannelConfirmation:
		b.WriteString("Delete #" + m.channels[m.channelSelected].GetChannelName() + "?\n\ny confirm · n cancel · Esc cancel")
	case serverMods, serverModDetails, serverPromoteConfirmation, serverDemoteConfirmation:
		b.WriteString(m.modsView())
	default:
		b.WriteString(m.membersView())
	}
	if m.notice != "" {
		b.WriteString("\n\n" + m.notice)
	}
	if m.busy {
		b.WriteString("\n\nLoading…")
	}
	if m.errorText != "" {
		b.WriteString("\n\n" + m.errorText)
	}
	return b.String()
}
