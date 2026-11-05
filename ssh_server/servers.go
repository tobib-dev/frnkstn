package main

import (
	"context"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
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
)

type serverOperation int

const (
	listServers serverOperation = iota
	getServer
	createServer
	editServer
	deleteServer
)

type serverResultMsg struct {
	operation serverOperation
	items     []*serversv1.UserServerItem
	item      *serversv1.ServerItem
	err       error
}

type serversModel struct {
	userID, accessToken, grpcPort string
	page                          serversPage
	items                         []*serversv1.UserServerItem
	selected                      int
	item                          *serversv1.ServerItem
	name, bio                     textinput.Model
	focusBio                      bool
	busy                          bool
	errorText                     string
}

func newServersModel(userID, accessToken, grpcPort string) serversModel {
	name, bio := textinput.New(), textinput.New()
	name.Prompt, bio.Prompt = "Name: ", "Bio: "
	name.SetVirtualCursor(true)
	bio.SetVirtualCursor(true)
	return serversModel{userID: userID, accessToken: accessToken, grpcPort: grpcPort, name: name, bio: bio, busy: true}
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
			response, err := client.CreateServer(ctx, &serversv1.CreateServerRequest{ServerName: item.GetServerName(), ServerBio: item.GetServerBio()})
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
		}
		if result.err == nil && operation != listServers && operation != deleteServer && result.item == nil {
			result.err = status.Error(codes.Internal, "server response is missing its item")
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
				m.errorText = "Only the server owner can make this change."
			case codes.NotFound:
				m.errorText = "Server no longer exists. Refresh the server list."
			case codes.Unauthenticated:
				m.errorText = "Session expired. Sign out and sign in again."
			}
			return m, nil
		}
		m.errorText = ""
		switch result.operation {
		case listServers:
			m.items, m.selected = result.items, 0
		case getServer, createServer, editServer:
			m.item, m.page = result.item, serverDetails
			found := false
			for _, row := range m.items {
				if row.ServerId == result.item.ServerId {
					row.ServerName = result.item.ServerName
					found = true
				}
			}
			if !found {
				m.items = append(m.items, &serversv1.UserServerItem{ServerId: result.item.ServerId, ServerName: result.item.ServerName})
			}
		case deleteServer:
			for i, row := range m.items {
				if row.ServerId == m.item.ServerId {
					m.items = append(m.items[:i], m.items[i+1:]...)
					break
				}
			}
			m.item, m.page, m.selected = nil, serversList, 0
		}
		return m, nil
	}
	if m.busy {
		return m, nil
	}
	key, isKey := msg.(tea.KeyPressMsg)
	if isKey && key.String() == "esc" {
		m.errorText = ""
		if m.page == serverEdit || m.page == serverDeleteConfirmation {
			m.page = serverDetails
		} else {
			m.page = serversList
		}
		return m, nil
	}
	if m.page == serverCreate || m.page == serverEdit {
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
					return m, nil
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
		b.WriteString("\n↑/↓ select · Enter open · n create · r refresh · Esc home")
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
		b.WriteString("\nEsc server list")
	case serverDeleteConfirmation:
		b.WriteString("Delete " + m.item.ServerName + "?\n\ny confirm · n cancel · Esc cancel")
	}
	if m.busy {
		b.WriteString("\n\nLoading…")
	}
	if m.errorText != "" {
		b.WriteString("\n\n" + m.errorText)
	}
	return b.String()
}
