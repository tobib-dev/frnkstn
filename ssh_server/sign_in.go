package main

import (
	"errors"
	"fmt"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type signInState int

const (
	signInWithGH signInState = iota
	signInFailed
	quit
)

type signInSuccessMsg struct{ token string }
type signInFailureMsg struct{ error error }
type switchToSignInMsg struct{ authLink string }

type signInItem int

const (
	signInWithGHItem signInItem = iota
	signInFailedItem
	quitSignInItem
)

func (i signInItem) FilterValue() string {
	switch i {
	case signInWithGHItem:
		return "Sign in with GitHub"
	case signInFailedItem:
		return "Sign in failed"
	default:
		return "Quit"
	}
}
func (i signInItem) Title() string       { return i.FilterValue() }
func (i signInItem) Description() string { return "" }

type signInModel struct {
	state    signInState
	list     list.Model
	authLink string
	token    string
	lastErr  error
}

func newSignInModel(width, height int) signInModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	l := list.New([]list.Item{
		signInItem(signInWithGHItem), signInItem(signInFailedItem), signInItem(quitSignInItem),
	}, delegate, width, height)
	l.Title = "Sign in"
	return signInModel{state: signInWithGH, list: l}
}

func (m signInModel) Update(msg tea.Msg) (signInModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil
	case switchToSignInMsg:
		m.authLink = msg.authLink
		return m, func() tea.Msg {
			token, err := authorize(m.authLink)
			if err != nil {
				return signInFailureMsg{error: err}
			}
			return signInSuccessMsg{token: token}
		}
	case signInSuccessMsg:
		m.token = msg.token
		m.state = signInWithGH
		return m, func() tea.Msg { return SwitchToHomeMsg{} }
	case signInFailureMsg:
		m.state = signInFailed
		m.lastErr = msg.error
		return m, m.list.NewStatusMessage(fmt.Sprintf("Sign in failed: %v", msg.error))
	}

	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		switch m.list.SelectedItem().(signInItem) {
		case signInWithGHItem:
			m.state = signInWithGH
			return m, func() tea.Msg { return switchToSignInMsg{authLink: authWithGitHub()} }
		case signInFailedItem:
			return m, m.list.NewStatusMessage("Sign in failed; choose GitHub to try again")
		case quitSignInItem:
			m.state = quit
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func authWithGitHub() string {
	return "https://github.com/login/oauth/authorize?client_id=YOUR_CLIENT_ID"
}
func authorize(authLink string) (string, error) {
	return "", errors.New("GitHub authorization is not configured")
}

func (m signInModel) View() tea.View {
	if m.state == quit {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render("Quit"))
	}
	return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.list.View()))
}
