package main

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	github "github.com/tobib-dev/frnkstn/internal/github"
)

type usernameCreatedMsg struct{ userID string }
type usernameFailedMsg struct{ err error }

type usernameModel struct {
	input      textinput.Model
	nameInput  textinput.Model
	grpcPort   string
	identity   github.User
	submitting bool
	errorText  string
}

func newUsernameModel(grpcPort string, identity github.User) usernameModel {
	input := textinput.New()
	input.Prompt = "enter username: "
	input.SetVirtualCursor(true)
	nameInput := textinput.New()
	nameInput.Prompt = "enter name: "
	nameInput.SetVirtualCursor(true)
	return usernameModel{nameInput: nameInput, input: input, grpcPort: grpcPort, identity: identity}
}

func (m usernameModel) Update(msg tea.Msg) (usernameModel, tea.Cmd) {
	if _, ok := msg.(usernameFailedMsg); ok {
		m.submitting = false
		m.errorText = "Could not create account. Please try again."
		return m, m.input.Focus()
	}
	if key, ok := msg.(tea.KeyPressMsg); ok {
		switch key.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "tab", "shift+tab":
			if m.submitting {
				return m, nil
			}
			if m.nameInput.Focused() {
				m.nameInput.Blur()
				return m, m.input.Focus()
			}
			m.input.Blur()
			return m, m.nameInput.Focus()
		case "enter":
			if m.submitting {
				return m, nil
			}
			name := strings.TrimSpace(m.nameInput.Value())
			if name == "" {
				m.errorText = "Name is required."
				m.input.Blur()
				return m, m.nameInput.Focus()
			}
			if m.nameInput.Focused() {
				m.errorText = ""
				m.nameInput.Blur()
				return m, m.input.Focus()
			}
			username := strings.TrimSpace(m.input.Value())
			if username == "" {
				m.errorText = "Username is required."
				return m, nil
			}
			m.submitting = true
			m.errorText = ""
			m.input.Blur()
			m.nameInput.Blur()
			return m, func() tea.Msg {
				userID, err := createUser(name, username, m.identity, m.grpcPort)
				if err != nil {
					return usernameFailedMsg{err: err}
				}
				return usernameCreatedMsg{userID: userID}
			}
		}
	}
	if m.submitting {
		return m, nil
	}
	var cmd tea.Cmd
	if m.nameInput.Focused() {
		m.nameInput, cmd = m.nameInput.Update(msg)
	} else {
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m usernameModel) View() string {
	view := "Create account\n\n" + m.nameInput.View() + "\n" + m.input.View()
	if m.submitting {
		return view + "\n\nCreating account…"
	}
	if m.errorText != "" {
		view += "\n\n" + m.errorText
	}
	return view + "\n\nTab to switch fields • Enter to continue • Ctrl+C to quit"
}
