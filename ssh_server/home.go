package main

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type homeItem int

const (
	messagesHomeItem homeItem = iota
	groupsHomeItem
	profileHomeItem
	signOutHomeItem
	exitHomeItem
)

func (i homeItem) FilterValue() string {
	switch i {
	case messagesHomeItem:
		return "Messages"
	case groupsHomeItem:
		return "Groups"
	case profileHomeItem:
		return "Profile"
	case signOutHomeItem:
		return "Sign out"
	default:
		return "Exit"
	}
}
func (i homeItem) Title() string       { return i.FilterValue() }
func (i homeItem) Description() string { return "" }

type homeState int

const (
	homeReady homeState = iota
	homeSigningOut
)

type homeModel struct {
	list     list.Model
	state    homeState
	session  sessionInfo
	grpcPort string
}
type SwitchToHomeMsg struct{}
type signOutSuccessMsg struct{ quit bool }
type signOutFailureMsg struct{ err error }

func newHomeModel(width, height int) homeModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	l := list.New([]list.Item{messagesHomeItem, groupsHomeItem, profileHomeItem, signOutHomeItem, exitHomeItem}, delegate, width, height)
	l.Title = "Home"
	l.StatusMessageLifetime = 4 * time.Second
	return homeModel{list: l}
}

func (m homeModel) Update(msg tea.Msg) (homeModel, tea.Cmd) {
	if _, ok := msg.(signOutFailureMsg); ok {
		m.state = homeReady
		return m, m.list.NewStatusMessage("Sign out failed. Please try again.")
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.list.SetSize(size.Width, size.Height)
		return m, nil
	}
	if m.state == homeSigningOut {
		return m, nil
	}
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(msg, m.list.KeyMap.ForceQuit) ||
			(m.list.FilterState() != list.Filtering && !key.Matches(msg, m.list.KeyMap.ClearFilter) && key.Matches(msg, m.list.KeyMap.Quit)) {
			return m.signOut(true)
		}
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		switch m.list.SelectedItem().(homeItem) {
		case signOutHomeItem:
			return m.signOut(false)
		case exitHomeItem:
			return m.signOut(true)
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m homeModel) signOut(quit bool) (homeModel, tea.Cmd) {
	if quit && m.session.sessionID == "" && m.session.userID == "" {
		return m, tea.Quit
	}
	m.state = homeSigningOut
	return m, func() tea.Msg {
		_, err := updateSession(m.session.sessionID, m.session.userID, m.grpcPort)
		if err != nil {
			return signOutFailureMsg{err: err}
		}
		return signOutSuccessMsg{quit: quit}
	}
}

func (m homeModel) View() tea.View {
	if m.state == homeSigningOut {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render("Signing out…"))
	}
	return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.list.View()))
}
