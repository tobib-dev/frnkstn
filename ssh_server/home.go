package main

import (
	"fmt"
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
	addFriendHomeItem
	notificationsHomeItem
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
	case addFriendHomeItem:
		return "Add friend"
	case notificationsHomeItem:
		return "Notifications"
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
	homeAddingFriend
	homeViewingMessages
	homeViewingNotifications
)

type homeModel struct {
	list                list.Model
	state               homeState
	session             sessionInfo
	grpcPort            string
	user                userInfo
	profile             profileModel
	addFriend           addFriendModel
	messages            messagesModel
	notifications       notificationsModel
	notificationItems   []notificationItem
	notificationPolling bool
}
type SwitchToHomeMsg struct{}
type signOutSuccessMsg struct{ quit bool }
type signOutFailureMsg struct{ err error }

func newHomeModel(width, height int) homeModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	l := list.New([]list.Item{messagesHomeItem, groupsHomeItem, addFriendHomeItem, notificationsHomeItem, profileHomeItem, signOutHomeItem, exitHomeItem}, delegate, width, height)
	l.Title = "Home"
	l.StatusMessageLifetime = 4 * time.Second
	return homeModel{list: l}
}

func (m homeModel) Update(msg tea.Msg) (homeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case notificationsPolledMsg:
		hasNew := hasNewNotification(msg.items, m.notificationItems)
		m.notificationItems = msg.items
		if m.state == homeViewingNotifications && !m.notifications.updating {
			m.notifications.items = msg.items
			m.notifications.loading = false
			m.notifications.errorText = ""
		}
		poll := scheduleNotificationPoll()
		if hasNew {
			return m, tea.Batch(poll, m.list.NewStatusMessage(fmt.Sprintf("You have %d new notification(s)", len(msg.items))))
		}
		return m, poll
	case notificationsPollFailedMsg:
		if m.state == homeViewingNotifications {
			m.notifications.loading = false
			m.notifications.errorText = "Could not load notifications."
		}
		return m, scheduleNotificationPoll()
	case notificationsPollTickMsg:
		return m, pollNotifications(m.user.userID, m.grpcPort, m.messages.seenMessageIDs)
	}
	if started, ok := msg.(messageStreamStartedMsg); ok && m.state != homeViewingMessages {
		started.subscription.close()
		return m, nil
	}
	if _, ok := msg.(signOutFailureMsg); ok {
		m.state = homeReady
		return m, m.list.NewStatusMessage("Sign out failed. Please try again.")
	}
	if m.state != homeViewingMessages {
		switch msg.(type) {
		case messagesLoadedMsg, messagesLoadFailedMsg, messageFriendsLoadedMsg, messageFriendsLoadFailedMsg:
			var cmd tea.Cmd
			m.messages, cmd = m.messages.Update(msg)
			return m, cmd
		}
	}
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.list.SetSize(size.Width, size.Height)
		if m.state == homeViewingMessages {
			m.messages.setSize(size.Width, size.Height)
		}
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
	if added, ok := msg.(friendAddedMsg); ok && m.state == homeAddingFriend {
		m.state = homeReady
		return m, m.list.NewStatusMessage("Friend request sent to " + added.username)
	}
	if resolved, ok := msg.(friendRequestResolvedMsg); ok && m.state == homeEditingProfile {
		m.state = homeReady
		if resolved.message == "" {
			return m, nil
		}
		return m, m.list.NewStatusMessage(resolved.message)
	}
	if m.state == homeAddingFriend {
		if key, ok := msg.(tea.KeyPressMsg); ok && !m.addFriend.adding {
			switch key.String() {
			case "esc":
				m.state = homeReady
				return m, nil
			case "ctrl+c":
				return m.signOut(true)
			}
		}
		var cmd tea.Cmd
		m.addFriend, cmd = m.addFriend.Update(msg)
		return m, cmd
	}
	if m.state == homeViewingMessages {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "esc":
				if m.messages.state == messagesBrowsing {
					m.state = homeReady
					return m, nil
				}
			case "ctrl+c":
				m.messages.stopSubscription()
				return m.signOut(true)
			}
		}
		var cmd tea.Cmd
		m.messages, cmd = m.messages.Update(msg)
		return m, cmd
	}
	if m.state == homeViewingNotifications {
		if key, ok := msg.(tea.KeyPressMsg); ok && !m.notifications.updating {
			switch key.String() {
			case "esc":
				m.state = homeReady
				return m, nil
			case "ctrl+c":
				return m.signOut(true)
			}
		}
		var cmd tea.Cmd
		m.notifications, cmd = m.notifications.Update(msg)
		m.notificationItems = m.notifications.items
		return m, cmd
	}
	if m.state == homeEditingProfile {
		if key, ok := msg.(tea.KeyPressMsg); ok && !m.profile.saving && !m.profile.acting {
			switch key.String() {
			case "esc":
				if m.profile.selectedRequest == nil {
					m.state = homeReady
					return m, nil
				}
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
		case messagesHomeItem:
			m.state = homeViewingMessages
			if m.messages.userID == "" {
				m.messages = newMessagesModel(m.list.Width(), m.list.Height(), m.user.userID, m.grpcPort)
				return m, fetchMessagesByUser(m.user.userID, m.grpcPort)
			}
			return m, nil
		case addFriendHomeItem:
			m.state = homeAddingFriend
			m.addFriend = newAddFriendModel(m.user.userID, m.grpcPort)
			return m, m.addFriend.input.Focus()
		case notificationsHomeItem:
			m.state = homeViewingNotifications
			m.notifications = newNotificationsModel(m.user.userID, m.grpcPort, m.messages.seenMessageIDs)
			if m.notificationPolling {
				m.notifications.items = m.notificationItems
				m.notifications.loading = false
				return m, nil
			}
			m.notificationPolling = true
			return m, pollNotifications(m.user.userID, m.grpcPort, m.notifications.seenMessageIDs)
		case profileHomeItem:
			m.state = homeEditingProfile
			m.profile = newProfileModel(m.user, m.grpcPort)
			var cmd tea.Cmd
			m.profile, cmd = m.profile.focusName()
			return m, tea.Batch(cmd, fetchPendingFriendRequests(m.user.userID, m.grpcPort))
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

func hasNewNotification(items, previous []notificationItem) bool {
	for _, item := range items {
		found := false
		for _, old := range previous {
			if item.kind == old.kind && item.id() == old.id() {
				found = true
				break
			}
		}
		if !found {
			return true
		}
	}
	return false
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
	if m.state == homeAddingFriend {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.addFriend.View()))
	}
	if m.state == homeViewingMessages {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.messages.View()))
	}
	if m.state == homeViewingNotifications {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.notifications.View()))
	}
	if m.state == homeSigningOut {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render("Signing out…"))
	}
	return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.list.View()))
}
