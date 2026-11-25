package main

import (
	"strings"

	"charm.land/bubbles/v2/viewport"
	"github.com/charmbracelet/x/ansi"
)

// Home adds two cells of margin on each side of the conversation.
const conversationMargin = 4

func conversationView(title, history, input, notice, help string, width, height, scroll int) string {
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	width, height = max(1, width-conversationMargin), max(1, height-conversationMargin)
	footer := strings.Repeat("─", width) + "\n" + ansi.Truncate(input, width, "")
	if notice != "" {
		footer += "\n" + ansi.Wrap(notice, width, "")
	}
	footer += "\n" + ansi.Wrap(help, width, "")
	header := ansi.Truncate(title, width, "")
	available := height - strings.Count(footer, "\n") - 2
	if available < 1 {
		lines := strings.Split(footer, "\n")
		// Preserve the composer even in a very short terminal.
		if height == 1 {
			return ansi.Truncate(input, width, "")
		}
		return strings.Join(lines[:min(height, len(lines))], "\n")
	}
	v := viewport.New(viewport.WithWidth(width), viewport.WithHeight(available))
	v.SetContent(strings.TrimRight(ansi.Wrap(history, width, ""), "\n"))
	v.GotoBottom()
	v.ScrollUp(scroll)
	return header + "\n" + v.View() + "\n" + footer
}

func (m *serversModel) setSize(width, height int) {
	m.width, m.height = width, height
	m.messageInput.SetWidth(max(1, width-conversationMargin-10))
}

func (m serversModel) conversationVisible() bool {
	return m.page == serverChannelMessages || m.page == serverThreadMessages ||
		m.page == serverComposeMessage || m.page == serverEditMessage
}

func (m serversModel) conversationView() string {
	title := m.item.GetServerName() + " · #" + m.channels[m.channelSelected].GetChannelName()
	var history strings.Builder
	thread := m.page == serverThreadMessages ||
		(m.page == serverComposeMessage && m.composeThreadID != "" && !m.creatingThread)
	if thread {
		title += " · Thread"
		for i := len(m.threadMessages) - 1; i >= 0; i-- {
			item := m.threadMessages[i]
			history.WriteString(serverMessageTimestamp(item.GetMessageId()) + " " + item.GetAuthorName() + ": " + item.GetContent() + "\n")
		}
	} else {
		for i := len(m.messages) - 1; i >= 0; i-- {
			item := m.messages[i]
			cursor := "  "
			if i == m.messageSelected {
				cursor = "> "
			}
			history.WriteString(cursor + serverMessageTimestamp(item.GetMessageId()) + " " + item.GetAuthorUsername() + ": " + item.GetContent())
			if item.GetEdited() {
				history.WriteString(" (edited)")
			}
			history.WriteByte('\n')
		}
	}
	if history.Len() == 0 {
		history.WriteString("No messages yet.")
	}
	notice := m.errorText
	if notice == "" {
		notice = m.notice
	}
	if m.busy {
		notice = "Sending/loading…"
	}
	if m.cooldown.seconds > 0 && m.cooldown.serverID == m.item.GetServerId() && m.cooldown.channelID == m.channels[m.channelSelected].GetChannelId() {
		wait := "Slowmode: ready to send"
		if m.cooldown.timer.Running() {
			wait = "Next message in " + m.cooldown.timer.View()
		}
		if notice != "" {
			notice += "\n"
		}
		notice += wait
	}
	help := "↑/↓ select · Enter open thread · n compose · T reply · t threads · PgUp/PgDn scroll · Esc back"
	if thread {
		help = "r reply · PgUp/PgDn scroll · Esc threads"
	}
	if m.page == serverComposeMessage || m.page == serverEditMessage {
		help = "Enter send · @ tag · Tab navigate · PgUp/PgDn scroll · Esc cancel"
		if m.creatingThread {
			title += " · New thread reply"
		}
		if m.page == serverEditMessage {
			title += " · Edit message"
		}
	}
	if m.mentionOpen {
		// Keep the member picker inside the history budget, above the composer.
		history.WriteString(m.mentionsView())
	}
	return conversationView(title, history.String(), m.messageInput.View(), notice, help, m.width, m.height, m.historyScroll)
}
