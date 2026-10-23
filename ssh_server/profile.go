package main

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	friendsv1 "github.com/tobib-dev/frnkstn-proto/friends/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type profileUpdatedMsg struct{ user userInfo }
type profileFailedMsg struct{ err error }
type accountDeletedMsg struct{}
type accountDeleteFailedMsg struct{ err error }

type profileModel struct {
	name, username                textinput.Model
	userID, accessToken, grpcPort string
	saving                        bool
	confirmDelete                 bool
	deleting                      bool
	loadingRequests               bool
	pendingRequests               []*friendsv1.GetFriendsItem
	requestCursor                 int
	selectedRequest               *friendsv1.GetFriendsItem
	actionCursor                  int
	acting                        bool
	requestsError                 string
	errorText                     string
}

func (m profileModel) focusName() (profileModel, tea.Cmd) {
	m.requestCursor = -1
	m.username.Blur()
	cmd := m.name.Focus()
	return m, cmd
}

func (m profileModel) focusUsername() (profileModel, tea.Cmd) {
	m.requestCursor = -1
	m.name.Blur()
	cmd := m.username.Focus()
	return m, cmd
}

func newProfileModel(user userInfo, accessToken, grpcPort string) profileModel {
	name, username := textinput.New(), textinput.New()
	name.Prompt, username.Prompt = "Name: ", "Username: "
	name.SetVirtualCursor(true)
	username.SetVirtualCursor(true)
	name.SetValue(user.name)
	username.SetValue(user.username)
	return profileModel{name: name, username: username, userID: user.userID, accessToken: accessToken, grpcPort: grpcPort, loadingRequests: true, requestCursor: -1}
}

func (m profileModel) Update(msg tea.Msg) (profileModel, tea.Cmd) {
	switch msg := msg.(type) {
	case pendingFriendRequestsLoadedMsg:
		m.loadingRequests = false
		m.pendingRequests = nil
		for _, item := range msg.items {
			if item.GetStatus() == pendingFriendStatus {
				m.pendingRequests = append(m.pendingRequests, item)
			}
		}
		m.requestsError = ""
		return m, nil
	case pendingFriendRequestsFailedMsg:
		m.loadingRequests = false
		m.requestsError = "Could not load friend requests."
		return m, nil
	case friendRequestFailedMsg:
		m.acting = false
		m.errorText = "Could not update friend request. Please try again."
		return m, nil
	}
	if m.selectedRequest != nil {
		return m.updateSelectedRequest(msg)
	}
	if failure, ok := msg.(profileFailedMsg); ok {
		m.saving = false
		m.errorText = "Could not update profile. Please try again."
		if status.Code(failure.err) == codes.AlreadyExists {
			m.errorText = "Username already exists. Choose a different one."
		}
		return m.focusUsername()
	}
	if failure, ok := msg.(accountDeleteFailedMsg); ok {
		m.deleting = false
		m.confirmDelete = false
		m.errorText = "Could not delete account. Please try again."
		if status.Code(failure.err) == codes.NotFound {
			m.errorText = "Account no longer exists."
		}
		return m, nil
	}
	if m.saving || m.deleting || m.acting {
		return m, nil
	}
	if m.requestCursor >= 0 {
		if key, ok := msg.(tea.KeyPressMsg); ok {
			switch key.String() {
			case "up":
				if m.requestCursor == 0 {
					return m.focusUsername()
				}
				m.requestCursor--
			case "down":
				if m.requestCursor < len(m.pendingRequests)-1 {
					m.requestCursor++
				}
			case "tab", "shift+tab":
				return m.focusName()
			case "enter":
				m.selectedRequest = m.pendingRequests[m.requestCursor]
				m.actionCursor, m.errorText = 0, ""
			}
		}
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.confirmDelete {
			switch key.String() {
			case "y":
				m.confirmDelete = false
				m.deleting = true
				return m, func() tea.Msg {
					if err := deleteUser(m.accessToken, m.grpcPort); err != nil {
						return accountDeleteFailedMsg{err: err}
					}
					return accountDeletedMsg{}
				}
			case "n", "esc":
				m.confirmDelete = false
				return m, nil
			}
			return m, nil
		}
		switch key.String() {
		case "d":
			m.errorText = ""
			m.confirmDelete = true
			m.name.Blur()
			m.username.Blur()
			return m, nil
		case "tab":
			if m.name.Focused() {
				return m.focusUsername()
			}
			if len(m.pendingRequests) > 0 {
				m.name.Blur()
				m.username.Blur()
				m.requestCursor = 0
				return m, nil
			}
			return m.focusName()
		case "shift+tab":
			if m.username.Focused() {
				return m.focusName()
			}
			return m.focusUsername()
		case "enter":
			name, username := strings.TrimSpace(m.name.Value()), strings.TrimSpace(m.username.Value())
			if name == "" {
				m.errorText = "Name is required."
				return m.focusName()
			}
			if username == "" {
				m.errorText = "Username is required."
				return m.focusUsername()
			}
			m.errorText = ""
			m.saving = true
			m.name.Blur()
			m.username.Blur()
			return m, func() tea.Msg {
				user, err := updateUser(userInfo{userID: m.userID, name: name, username: username}, m.accessToken, m.grpcPort)
				if err != nil {
					return profileFailedMsg{err: err}
				}
				return profileUpdatedMsg{user: user}
			}
		}
	}
	var cmd tea.Cmd
	if m.name.Focused() {
		m.name, cmd = m.name.Update(msg)
	} else {
		m.username, cmd = m.username.Update(msg)
	}
	return m, cmd
}

func (m profileModel) updateSelectedRequest(msg tea.Msg) (profileModel, tea.Cmd) {
	if m.acting {
		return m, nil
	}
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	actions := m.requestActions()
	switch key.String() {
	case "up":
		if m.actionCursor > 0 {
			m.actionCursor--
		}
	case "down":
		if m.actionCursor < len(actions)-1 {
			m.actionCursor++
		}
	case "esc":
		m.selectedRequest = nil
		m.errorText = ""
	case "enter":
		action := actions[m.actionCursor]
		if action.label == "Home" {
			return m, func() tea.Msg { return friendRequestResolvedMsg{} }
		}
		m.acting, m.errorText = true, ""
		request := m.selectedRequest
		return m, func() tea.Msg {
			if err := resolveFriendRequest(m.userID, m.accessToken, m.grpcPort, request, action.action); err != nil {
				return friendRequestFailedMsg{err: err}
			}
			return friendRequestResolvedMsg{message: action.message}
		}
	}
	return m, nil
}

func (m profileModel) requestActions() []friendRequestOption {
	if m.selectedRequest.GetUserId() == m.userID {
		return []friendRequestOption{{label: "Cancel", action: cancelFriendRequest, message: "Friend request canceled"}, {label: "Home"}}
	}
	return []friendRequestOption{
		{label: "Accept", action: acceptFriendRequest, message: "Friend request accepted"},
		{label: "Decline", action: declineFriendRequest, message: "Friend request declined"},
		{label: "Home"},
	}
}

func (m profileModel) View() string {
	if m.selectedRequest != nil {
		username := m.selectedRequest.GetFriendUsername()
		if username == "" {
			username = m.selectedRequest.GetFriendId()
		}
		view := "Friend request: " + username + "\n\n"
		for i, action := range m.requestActions() {
			cursor := "  "
			if i == m.actionCursor {
				cursor = "> "
			}
			view += cursor + action.label + "\n"
		}
		if m.acting {
			view += "\nUpdating friend request…"
		} else if m.errorText != "" {
			view += "\n" + m.errorText
		}
		return view + "\n\n↑/↓ to select • Enter to confirm • Esc to go back • Ctrl+C to quit"
	}
	view := "Edit profile\n\n" + m.name.View() + "\n" + m.username.View()
	if m.confirmDelete {
		return view + "\n\nDelete this account? This cannot be undone. Press y to delete or n to cancel."
	}
	if m.deleting {
		return view + "\n\nDeleting account…"
	}
	if m.saving {
		return view + "\n\nSaving…"
	}
	if m.errorText != "" {
		view += "\n\n" + m.errorText
	}
	view += "\n\nUnresponded requests\n"
	switch {
	case m.loadingRequests:
		view += "Loading…"
	case m.requestsError != "":
		view += m.requestsError
	case len(m.pendingRequests) == 0:
		view += "No unresponded requests."
	default:
		for i, request := range m.pendingRequests {
			username := request.GetFriendUsername()
			if username == "" {
				username = request.GetFriendId()
			}
			cursor := "  "
			if i == m.requestCursor {
				cursor = "> "
			}
			view += "\n" + cursor + username
		}
	}
	return view + "\n\nTab to switch fields and requests • Enter to continue • d to delete account • Esc to cancel • Ctrl+C to quit"
}
