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

type profileModel struct {
	name, username   textinput.Model
	userID, grpcPort string
	saving           bool
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
	if m.saving {
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
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
	if m.saving {
		return view + "\n\nSaving…"
	}
	if m.errorText != "" {
		view += "\n\n" + m.errorText
	}
	return view + "\n\nTab to switch fields • Enter to save • Esc to cancel • Ctrl+C to quit"
}
