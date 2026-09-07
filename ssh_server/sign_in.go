package main

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type signInState int

const (
	signInWithGH signInState = iota
	signInFailed
	quit
	signInAuthenticating
)

type signInSuccessMsg struct{ token string }
type signInFailureMsg struct{ error error }
type signInDeviceAuthMsg struct{ response GHResponse }
type switchToSignInMsg struct{ authLink string }

type signInItem int

const (
	signInWithGHItem signInItem = iota
	quitSignInItem
)

func (i signInItem) FilterValue() string {
	switch i {
	case signInWithGHItem:
		return "Sign in with GitHub"
	default:
		return "Quit"
	}
}
func (i signInItem) Title() string       { return i.FilterValue() }
func (i signInItem) Description() string { return "" }

type signInModel struct {
	deviceAuth GHResponse
	state      signInState
	list       list.Model
	authLink   string
	token      string
	lastErr    error
}

func newSignInModel(width, height int) signInModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	l := list.New(signInItems(), delegate, width, height)
	l.Title = "Sign in"
	return signInModel{state: signInWithGH, list: l}
}

func signInItems() []list.Item {
	return []list.Item{signInItem(signInWithGHItem), signInItem(quitSignInItem)}
}

func (m signInModel) Update(msg tea.Msg) (signInModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, msg.Height)
		return m, nil
	case switchToSignInMsg:
		if m.state == signInAuthenticating {
			return m, nil
		}
		m.authLink = msg.authLink
		m.state = signInAuthenticating
		m.lastErr = nil
		m.deviceAuth = GHResponse{}
		return m, func() tea.Msg {
			response, err := authenticate()
			if err != nil {
				return signInFailureMsg{error: err}
			}
			return signInDeviceAuthMsg{response: response}
		}
	case signInDeviceAuthMsg:
		m.deviceAuth = msg.response
		m.state = signInAuthenticating
		return m, func() tea.Msg {
			response, err := pollForToken(clientID, msg.response.DeviceCode,
				time.Duration(msg.response.Interval)*time.Second, msg.response.ExpiresIn)
			if err != nil {
				return signInFailureMsg{error: err}
			}
			return signInSuccessMsg{token: response.AccessToken}
		}
	case signInSuccessMsg:
		m.deviceAuth = GHResponse{}
		m.token = msg.token
		m.state = signInWithGH
		return m, func() tea.Msg { return SwitchToHomeMsg{} }
	case signInFailureMsg:
		m.deviceAuth = GHResponse{}
		m.state = signInFailed
		m.list.Title = "Sign in failed"
		m.lastErr = msg.error
		return m, m.list.NewStatusMessage(fmt.Sprintf("Sign in failed: %v", msg.error))
	}

	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		if m.state == signInAuthenticating {
			return m, nil
		}
		switch m.list.SelectedItem().(signInItem) {
		case signInWithGHItem:
			m.state = signInWithGH
			m.list.Title = "Sign in"
			return m, func() tea.Msg { return switchToSignInMsg{authLink: authWithGitHub()} }
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
func (m signInModel) View() tea.View {
	if m.state == quit {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render("Quit"))
	}
	if m.state == signInAuthenticating {
		if m.deviceAuth.VerificationURI != "" {
			return tea.NewView(lipgloss.NewStyle().Margin(2).Render(
				"Sign in with GitHub\n\nOpen this URL in your browser:\n" +
					m.deviceAuth.VerificationURI + "\n\nEnter this code:\n" +
					m.deviceAuth.UserCode + "\n\nWaiting for authorization…",
			))
		}
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render("Signing in…"))
	}
	return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.list.View()))
}
