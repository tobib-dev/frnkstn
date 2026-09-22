package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "embed"

	tea "charm.land/bubbletea/v2"

	"charm.land/log/v2"
	"charm.land/wish/v2"
	"charm.land/wish/v2/activeterm"
	"charm.land/wish/v2/bubbletea"
	"charm.land/wish/v2/logging"
	"github.com/charmbracelet/ssh"

	//"charm.land/wish/elapsed"
	"github.com/joho/godotenv"
)

const host = "localhost"

var banner string

type sessionState int

const (
	listView sessionState = iota
	formView
	signInView
	homeView
)

type config struct {
	grpcPort   string
	ghClientID string
}

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading environment variables")
	}

	bannerBytes, err := os.ReadFile("banner.txt")
	if err != nil {
		log.Fatal("Error loading banner file:", err)
	}
	banner = string(bannerBytes)

	port := os.Getenv("TUI_PORT")
	grpcPort := os.Getenv("API_PORT")
	clientID := os.Getenv("GITHUB_CLIENT_ID")

	cfg := config{
		grpcPort:   grpcPort,
		ghClientID: clientID,
	}

	srv, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithBannerHandler(func(ctx ssh.Context) string {
			return fmt.Sprintf(banner, ctx.User())
		}),
		wish.WithMiddleware(
			bubbletea.Middleware(func(s ssh.Session) (tea.Model, []tea.ProgramOption) {
				return teaHandler(s, cfg)
			}),
			activeterm.Middleware(),
			logging.Middleware(),
			//elapsed.Middleware(),
		),
	)
	if err != nil {
		log.Error("Could not start server", "error", err)
	}

	done := make(chan os.Signal, 1)
	signal.Notify(done, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	log.Info("Starting SSH server", "host", host, "port", port)
	go func() {
		if err = srv.ListenAndServe(); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
			log.Error("Could not start server", "error", err)
			done <- nil
		}
	}()

	<-done
	log.Info("Stopping SSH server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer func() { cancel() }()
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, ssh.ErrServerClosed) {
		log.Error("Could not stop server", "error", err)
	}
}

func teaHandler(s ssh.Session, cfg config) (tea.Model, []tea.ProgramOption) {
	pty, _, _ := s.Pty()

	m := mainModel{
		state:  signInView,
		width:  pty.Window.Width,
		height: pty.Window.Height,
		cfg:    cfg,
	}
	m.signIn = newSignInModel(m.width, m.height, cfg.ghClientID, cfg.grpcPort)
	m.home = newHomeModel(m.width, m.height)
	return m, []tea.ProgramOption{}
}

type mainModel struct {
	state         sessionState
	signIn        signInModel
	home          homeModel
	width, height int
	cfg           config
}

func (m mainModel) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
	)
}

func (m mainModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.signIn.list.SetSize(msg.Width, msg.Height)
		m.home.list.SetSize(msg.Width, msg.Height)

	case SwitchToHomeMsg:
		m.state = homeView
		m.home.session = m.signIn.session
		m.home.grpcPort = m.signIn.grpcPort
		return m, m.home.list.NewStatusMessage("Sign in successful")

	case signOutSuccessMsg:
		m.signIn = newSignInModel(m.width, m.height, m.signIn.clientID, m.signIn.grpcPort)
		m.home = newHomeModel(m.width, m.height)
		m.state = signInView
		return m, m.signIn.list.NewStatusMessage("Signed out")

	case switchToSignInMsg:
		m.state = signInView
		if msg.authLink == "" {
			return m, nil
		}
	}

	var cmd tea.Cmd
	switch m.state {
	case signInView:
		m.signIn, cmd = m.signIn.Update(msg)
	case homeView:
		m.home, cmd = m.home.Update(msg)
	}
	return m, cmd
}

func (m mainModel) View() tea.View {
	var view tea.View
	switch m.state {
	case signInView:
		view = m.signIn.View()
	case homeView:
		view = m.home.View()
	default:
		view = m.signIn.View()
	}
	view.AltScreen = true
	return view
}
