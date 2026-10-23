package main

import (
	"fmt"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
)

type homeItemKind int

const (
	messagesHomeItem homeItemKind = iota
	groupsHomeItem
	addFriendHomeItem
	notificationsHomeItem
	profileHomeItem
	signOutHomeItem
	exitHomeItem
)

type homeItem struct {
	kind    homeItemKind
	counter *atomic.Int64
}

func (i homeItem) FilterValue() string {
	switch i.kind {
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
func (i homeItem) Title() string {
	label := i.FilterValue()
	if i.counter != nil {
		return fmt.Sprintf("%s (%d)", label, i.counter.Load())
	}
	return label
}
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
	messageCounter      *atomic.Int64
	notificationCounter *atomic.Int64
	unreadAuthors       map[string]struct{}
}
type SwitchToHomeMsg struct{}
type signOutSuccessMsg struct{ quit bool }
type signOutFailureMsg struct{ err error }

func newHomeModel(width, height int) homeModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	messageCounter, notificationCounter := &atomic.Int64{}, &atomic.Int64{}
	l := list.New([]list.Item{
		homeItem{kind: messagesHomeItem, counter: messageCounter},
		homeItem{kind: groupsHomeItem},
		homeItem{kind: addFriendHomeItem},
		homeItem{kind: notificationsHomeItem, counter: notificationCounter},
		homeItem{kind: profileHomeItem},
		homeItem{kind: signOutHomeItem},
		homeItem{kind: exitHomeItem},
	}, delegate, width, height)
	l.Title = "Home"
	l.StatusMessageLifetime = 4 * time.Second
	return homeModel{list: l, messageCounter: messageCounter, notificationCounter: notificationCounter, unreadAuthors: make(map[string]struct{})}
}

func (m homeModel) Update(msg tea.Msg) (homeModel, tea.Cmd) {
	switch msg := msg.(type) {
	case notificationsPolledMsg:
		hasNew := hasNewNotification(msg.items, m.notificationItems)
		m.notificationItems = msg.items
		m.setNotificationCounters(msg.items)
		if m.state == homeViewingNotifications && !m.notifications.updating && m.notifications.selected < 0 {
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
		return m, pollNotifications(m.session.internalAccessToken, m.grpcPort)
	case enterNotificationConversationMsg:
		metadata := m.messages.metadataForAuthor(msg.authorID)
		if metadata == nil {
			m.notifications.errorText = "Could not open conversation."
			return m, nil
		}
		m.state = homeViewingMessages
		var cmd tea.Cmd
		m.messages, cmd = m.messages.openConversation(metadata)
		m.markDMRead(metadata.GetDmId())
		return m, cmd
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
		previousState := m.messages.state
		m.messages, cmd = m.messages.Update(msg)
		if previousState != messagesViewingConversation && m.messages.state == messagesViewingConversation && m.messages.selectedDM != nil {
			m.markDMRead(m.messages.selectedDM.GetDmId())
		}
		return m, cmd
	}
	if m.state == homeViewingNotifications {
		if key, ok := msg.(tea.KeyPressMsg); ok && !m.notifications.updating {
			switch key.String() {
			case "esc":
				if m.notifications.selected < 0 {
					m.state = homeReady
					return m, nil
				}
			case "ctrl+c":
				return m.signOut(true)
			}
		}
		var cmd tea.Cmd
		m.notifications, cmd = m.notifications.Update(msg)
		m.notificationItems = m.notifications.items
		m.setNotificationCounters(m.notificationItems)
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
		switch m.list.SelectedItem().(homeItem).kind {
		case messagesHomeItem:
			m.state = homeViewingMessages
			if m.messages.userID == "" {
				m.messages = newMessagesModel(m.list.Width(), m.list.Height(), m.user.userID, m.session.internalAccessToken, m.grpcPort)
				return m, fetchMessagesByUser(m.session.internalAccessToken, m.grpcPort)
			}
			return m, nil
		case addFriendHomeItem:
			m.state = homeAddingFriend
			m.addFriend = newAddFriendModel(m.user.userID, m.session.internalAccessToken, m.grpcPort)
			return m, m.addFriend.input.Focus()
		case notificationsHomeItem:
			m.state = homeViewingNotifications
			m.notifications = newNotificationsModel(m.user.userID, m.session.internalAccessToken, m.grpcPort)
			if m.notificationPolling {
				m.notifications.items = m.notificationItems
				m.notifications.loading = false
				return m, nil
			}
			m.notificationPolling = true
			return m, pollNotifications(m.session.internalAccessToken, m.grpcPort)
		case profileHomeItem:
			m.state = homeEditingProfile
			m.profile = newProfileModel(m.user, m.session.internalAccessToken, m.grpcPort)
			var cmd tea.Cmd
			m.profile, cmd = m.profile.focusName()
			return m, tea.Batch(cmd, fetchPendingFriendRequests(m.session.internalAccessToken, m.grpcPort))
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

func (m *homeModel) markDMRead(dmID string) {
	friend := m.messages.friendsByDM[dmID]
	if friend == nil {
		return
	}
	authorID := (messageFriendItem{friend: friend, userID: m.user.userID}).receiverID()
	if _, unread := m.unreadAuthors[authorID]; !unread {
		return
	}
	delete(m.unreadAuthors, authorID)
	m.messageCounter.Add(-1)
}

func (m *homeModel) setNotificationCounters(items []notificationItem) {
	m.unreadAuthors = make(map[string]struct{})
	for _, item := range items {
		if item.kind == messageNotification && item.message.GetAuthorId() != "" {
			m.unreadAuthors[item.message.GetAuthorId()] = struct{}{}
		}
	}
	m.messageCounter.Store(int64(len(m.unreadAuthors)))
	m.notificationCounter.Store(int64(len(items)))
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
		_, err := updateSession(m.session.sessionID, m.session.internalAccessToken, m.grpcPort)
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
