package main

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type profileUpdatedMsg struct{ user userInfo }
type profileFailedMsg struct{ err error }
type accountDeletedMsg struct{}
type accountDeleteFailedMsg struct{ err error }

type profileModel struct {
	name, username   textinput.Model
	userID, grpcPort string
	saving           bool
	confirmDelete    bool
	deleting         bool
	errorText        string
}

func (m profileModel) focusName() (profileModel, tea.Cmd) {
	m.username.Blur()
	cmd := m.name.Focus()
	return m, cmd
}

func (m profileModel) focusUsername() (profileModel, tea.Cmd) {
	m.name.Blur()
	cmd := m.username.Focus()
	return m, cmd
}

func newProfileModel(user userInfo, grpcPort string) profileModel {
	name, username := textinput.New(), textinput.New()
	name.Prompt, username.Prompt = "Name: ", "Username: "
	name.SetVirtualCursor(true)
	username.SetVirtualCursor(true)
	name.SetValue(user.name)
	username.SetValue(user.username)
	return profileModel{name: name, username: username, userID: user.userID, grpcPort: grpcPort}
}

func (m profileModel) Update(msg tea.Msg) (profileModel, tea.Cmd) {
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
	if m.saving || m.deleting {
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		if m.confirmDelete {
			switch key.String() {
			case "y":
				m.confirmDelete = false
				m.deleting = true
				return m, func() tea.Msg {
					if err := deleteUser(m.userID, m.grpcPort); err != nil {
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
		case "tab", "shift+tab":
			if m.name.Focused() {
				return m.focusUsername()
			}
			return m.focusName()
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
				user, err := updateUser(userInfo{userID: m.userID, name: name, username: username}, m.grpcPort)
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

func (m profileModel) View() string {
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
	return view + "\n\nTab to switch fields • Enter to save • d to delete account • Esc to cancel • Ctrl+C to quit"
}
