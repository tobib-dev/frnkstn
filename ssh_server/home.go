package main

import (
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
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
	homeEditingProfile
)

type homeModel struct {
	list     list.Model
	state    homeState
	session  sessionInfo
	grpcPort string
	user     userInfo
	profile  profileModel
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
	if updated, ok := msg.(profileUpdatedMsg); ok && m.state == homeEditingProfile {
		log.Info("updating profile",
			"old_user", profileLogUser(m.user),
			"new_user", profileLogUser(updated.user),
		)
		m.user = updated.user
		m.state = homeReady
		return m, m.list.NewStatusMessage("Profile updated")
	}
	if _, ok := msg.(accountDeletedMsg); ok && m.state == homeEditingProfile {
		log.Info("deleted user account", "user", profileLogUser(m.user))
		return m, func() tea.Msg { return signOutSuccessMsg{} }
	}
	if m.state == homeEditingProfile {
		if key, ok := msg.(tea.KeyPressMsg); ok && !m.profile.saving {
			switch key.String() {
			case "esc":
				m.state = homeReady
				return m, nil
			case "ctrl+c":
				return m.signOut(true)
			}
		}
		var cmd tea.Cmd
		m.profile, cmd = m.profile.Update(msg)
		return m, cmd
	}
	if msg, ok := msg.(tea.KeyPressMsg); ok {
		if key.Matches(msg, m.list.KeyMap.ForceQuit) ||
			(m.list.FilterState() != list.Filtering && !key.Matches(msg, m.list.KeyMap.ClearFilter) && key.Matches(msg, m.list.KeyMap.Quit)) {
			return m.signOut(true)
		}
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		switch m.list.SelectedItem().(homeItem) {
		case profileHomeItem:
			m.state = homeEditingProfile
			m.profile = newProfileModel(m.user, m.grpcPort)
			var cmd tea.Cmd
			m.profile, cmd = m.profile.focusName()
			return m, cmd
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

func profileLogUser(user userInfo) map[string]string {
	return map[string]string{
		"user_id":  user.userID,
		"name":     user.name,
		"username": user.username,
	}
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
	if m.state == homeEditingProfile {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.profile.View()))
	}
	if m.state == homeSigningOut {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render("Signing out…"))
	}
	return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.list.View()))
}
