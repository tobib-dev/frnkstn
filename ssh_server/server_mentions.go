package main

import (
	"context"
	"github.com/gocql/gocql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"

	tea "charm.land/bubbletea/v2"
	serversv1 "github.com/tobib-dev/frnkstn-proto/servers/v1"
)

type mentionMembersMsg struct {
	serverID string
	items    []*serversv1.MemberItem
	err      error
}

func (m serversModel) loadMentionMembers() tea.Cmd {
	return func() tea.Msg {
		result := mentionMembersMsg{serverID: m.item.GetServerId()}
		conn, err := newGRPCClient("localhost:" + m.grpcPort)
		if err != nil {
			result.err = err
			return result
		}
		defer conn.Close()
		ctx, cancel := context.WithTimeout(context.Background(), authRequestTimeout)
		defer cancel()
		response, err := serversv1.NewServerServiceClient(conn).GetMembers(bearerContext(ctx, m.accessToken), &serversv1.GetMembersRequest{ServerId: result.serverID})
		result.err = err
		if err == nil {
			result.items = response.GetItems()
		}
		return result
	}
}

func (m serversModel) mentionMatches() []*serversv1.MemberItem {
	value := []rune(m.messageInput.Value())
	if m.mentionStart < 0 || m.mentionStart >= len(value) {
		return nil
	}
	query := strings.ToLower(string(value[m.mentionStart+1 : m.messageInput.Position()]))
	var matches []*serversv1.MemberItem
	for _, member := range m.mentionMembers {
		if strings.HasPrefix(strings.ToLower(member.GetMemberName()), query) {
			matches = append(matches, member)
		}
	}
	return matches
}

func (m serversModel) updateMentions(msg tea.Msg) (serversModel, tea.Cmd, bool) {
	if result, ok := msg.(mentionMembersMsg); ok {
		if m.mentionOpen && m.item.GetServerId() == result.serverID {
			m.mentionMembers = result.items
			if result.err != nil {
				m.errorText = "Could not load server members."
				m.mentionOpen = false
			}
		}
		return m, nil, true
	}
	if !m.mentionOpen {
		return m, nil, false
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil, false
	}
	matches := m.mentionMatches()
	switch key.String() {
	case "esc":
		m.mentionOpen = false
	case "up":
		m.mentionSelected = max(0, m.mentionSelected-1)
	case "down":
		m.mentionSelected = min(max(0, len(matches)-1), m.mentionSelected+1)
	case "enter", "tab":
		if len(matches) > 0 {
			member := matches[min(m.mentionSelected, len(matches)-1)]
			value := []rune(m.messageInput.Value())
			prefix := string(value[:m.mentionStart]) + "@" + member.GetMemberName() + " "
			m.messageInput.SetValue(prefix + string(value[m.messageInput.Position():]))
			m.messageInput.SetCursor(len([]rune(prefix)))
			m.mentionOpen = false
		}
	default:
		var cmd tea.Cmd
		m.messageInput, cmd = m.messageInput.Update(msg)
		m.mentionSelected = 0
		if m.messageInput.Position() <= m.mentionStart || key.String() == "space" {
			m.mentionOpen = false
		}
		return m, cmd, true
	}
	return m, nil, true
}

func (m serversModel) mentionsView() string {
	view := "\n\nTag a server member\n"
	for i, member := range m.mentionMatches() {
		cursor := "  "
		if i == m.mentionSelected {
			cursor = "> "
		}
		view += cursor + "@" + member.GetMemberName() + "\n"
	}
	return view + "↑/↓ select · Enter/Tab insert · Esc close"
}

func resolveMentionedUsers(ctx context.Context, client serversv1.ServerServiceClient, serverID, content string) ([]string, error) {
	mentioned := map[string]struct{}{}
	for _, word := range strings.Fields(content) {
		word = strings.Trim(word, "()[]{}.,!?;:")
		if strings.HasPrefix(word, "@") && len(word) > 1 {
			mentioned[strings.ToLower(strings.TrimPrefix(word, "@"))] = struct{}{}
		}
	}
	if len(mentioned) == 0 {
		return nil, nil
	}
	members, err := client.GetMembers(ctx, &serversv1.GetMembersRequest{ServerId: serverID})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(mentioned))
	for _, member := range members.GetItems() {
		name := strings.ToLower(member.GetMemberName())
		if _, ok := mentioned[name]; ok {
			ids = append(ids, member.GetMemberId())
			delete(mentioned, name)
		}
	}
	if len(mentioned) != 0 {
		return nil, status.Error(codes.InvalidArgument, "tagged user was not found in this server")
	}
	return ids, nil
}

func serverMessageTimestamp(value string) string {
	id, err := gocql.ParseUUID(value)
	if err != nil || id.Version() != 1 {
		return ""
	}
	return id.Time().Local().Format("Jan 2 15:04")
}
