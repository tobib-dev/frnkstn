package main

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	github "github.com/tobib-dev/frnkstn/internal/github"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type signInState int

const (
	signInWithGH signInState = iota
	signInFailed
	quit
	signInAuthenticating
	signInLoadingSession
	signInLoadingUser
	signInUsername
)

type tokenInfo struct {
	AccessToken           string
	ExpiresIn             int
	RefreshToken          string
	RefreshTokenExpiresIn int
}

type signInUserMsg struct{ user userInfo }
type signInNewUserMsg struct{ identity github.User }

type signInSessionMsg struct{ session sessionInfo }

type signInSuccessMsg struct{ token tokenInfo }
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
	clientID   string
	deviceAuth GHResponse
	state      signInState
	list       list.Model
	authLink   string
	token      tokenInfo
	grpcPort   string
	session    sessionInfo
	lastErr    error
	username   usernameModel
	user       userInfo
}

func newSignInModel(width, height int, clientID, grpcPort string) signInModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	l := list.New(signInItems(), delegate, width, height)
	l.Title = "Sign in"
	return signInModel{state: signInWithGH, list: l, clientID: clientID, grpcPort: grpcPort}
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
			response, err := authenticate(m.clientID)
			if err != nil {
				return signInFailureMsg{error: err}
			}
			return signInDeviceAuthMsg{response: response}
		}
	case signInDeviceAuthMsg:
		m.deviceAuth = msg.response
		m.state = signInAuthenticating
		return m, func() tea.Msg {
			response, err := pollForToken(m.clientID, msg.response.DeviceCode,
				time.Duration(msg.response.Interval)*time.Second, msg.response.ExpiresIn)
			if err != nil {
				return signInFailureMsg{error: err}
			}

			tkn := tokenInfo{
				AccessToken:           response.AccessToken,
				ExpiresIn:             response.ExpiresIn,
				RefreshToken:          response.RefreshToken,
				RefreshTokenExpiresIn: response.RefreshExpiresIn,
			}
			return signInSuccessMsg{token: tkn}
		}
	case signInSuccessMsg:
		m.deviceAuth = GHResponse{}
		m.token = msg.token
		m.state = signInLoadingUser
		m.user = userInfo{}
		m.session = sessionInfo{}
		return m, func() tea.Msg {
			identity, err := getGitHubUser(msg.token.AccessToken)
			if err != nil {
				return signInFailureMsg{error: err}
			}
			user, err := getUser(identity.ID, m.grpcPort)
			if status.Code(err) == codes.NotFound {
				return signInNewUserMsg{identity: identity}
			}
			if err != nil {
				return signInFailureMsg{error: err}
			}
			return signInUserMsg{user: user}
		}
	case signInNewUserMsg:
		m.state = signInUsername
		m.username = newUsernameModel(m.grpcPort, msg.identity)
		return m, m.username.input.Focus()
	case signInUserMsg:
		m.user = msg.user
		return m.startSession()
	case usernameCreatedMsg:
		if m.state != signInUsername {
			return m, nil
		}
		m.user = userInfo{userID: msg.userID, name: strings.TrimSpace(m.username.nameInput.Value()), username: strings.TrimSpace(m.username.input.Value())}
		return m.startSession()
	case signInSessionMsg:
		m.session = msg.session
		return m, func() tea.Msg { return SwitchToHomeMsg{} }

	case signInFailureMsg:
		log.Error("failed login", "error", msg.error)
		m.deviceAuth = GHResponse{}
		m.state = signInFailed
		m.list.Title = "login failed"
		m.lastErr = msg.error

		return m, m.list.NewStatusMessage("login failed")
	}

	if m.state == signInUsername {
		var cmd tea.Cmd
		m.username, cmd = m.username.Update(msg)
		return m, cmd
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "ctrl+c" {
		return m, tea.Quit
	}
	if m.state == signInLoadingSession || m.state == signInLoadingUser {
		return m, nil
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
	if m.state == signInUsername {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.username.View()))
	}
	if m.state == signInLoadingSession || m.state == signInLoadingUser {
		return tea.NewView(lipgloss.NewStyle().Margin(2).Render("Loading account…"))
	}
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

func (m signInModel) startSession() (signInModel, tea.Cmd) {
	m.state = signInLoadingSession
	return m, func() tea.Msg {
		session, err := createSession(m.token, m.user.userID, m.grpcPort)
		if err != nil {
			return signInFailureMsg{error: err}
		}
		return signInSessionMsg{session: session}
	}
}
