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
	"io/ioutil"

	_ "embed"

	tea "charm.land/bubbletea/v2"

	"charm.land/lipgloss/v2"
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

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		log.Fatal("Error loading environment variables")
	}

	bannerBytes, err := ioutil.ReadFile("banner.txt")
	if err != nil {
		log.Fatal("Error loading banner file:", err)
	}
	banner = string(bannerBytes)

	port := os.Getenv("TUI_PORT")
	srv, err := wish.NewServer(
		wish.WithAddress(net.JoinHostPort(host, port)),
		wish.WithHostKeyPath(".ssh/id_ed25519"),
		wish.WithBannerHandler(func(ctx ssh.Context) string {
			return fmt.Sprintf(banner, ctx.User())
		}),
		wish.WithPasswordAuth(func(ctx ssh.Context, password string) bool {
			return password == os.Getenv("TUI_PASSWORD")
		}),
		wish.WithMiddleware(
			bubbletea.Middleware(teaHandler),
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

func teaHandler(s ssh.Session) (tea.Model, []tea.ProgramOption) {
	pty, _, _ := s.Pty()
	m := model{
		term:      pty.Term,
		width:     pty.Window.Width,
		height:    pty.Window.Height,
		txtStyle:  lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		quitStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		bg:        "light",
	}
	return m, []tea.ProgramOption{}
}

type model struct {
	term      string
	profile   string
	width     int
	height    int
	bg        string
	txtStyle  lipgloss.Style
	quitStyle lipgloss.Style
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tea.RequestBackgroundColor,
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.ColorProfileMsg:
		m.profile = msg.String()
	case tea.BackgroundColorMsg:
		if msg.IsDark() {
			m.bg = "dark"
		} else {
			m.bg = "light"
		}
	case tea.WindowSizeMsg:
		m.height = msg.Height
		m.width = msg.Width
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() tea.View {
	s := fmt.Sprintf("Term: %s\nWindow size: %dx%d\nBackground: %s\nProfile: %s",
		m.term, m.width, m.height, m.bg, m.profile)
	v := tea.NewView(m.txtStyle.Render(s) + "\n\n" + m.quitStyle.Render("Press 'q' to quit\n"))
	v.AltScreen = true
	return v
}
