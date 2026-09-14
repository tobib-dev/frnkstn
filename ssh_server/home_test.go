package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestSuccessfulSignInOpensHome(t *testing.T) {
	m := mainModel{
		state:  signInView,
		signIn: newSignInModel(80, 24),
		home:   newHomeModel(80, 24),
	}
	updated, cmd := m.Update(signInSuccessMsg{token: "test-token"})
	if cmd == nil {
		t.Fatal("expected home navigation command")
	}
	updated, _ = updated.Update(cmd())
	home := updated.(mainModel)
	if home.state != homeView || home.signIn.token != "test-token" {
		t.Fatal("successful sign-in did not open home with the token retained")
	}
	view := home.View().Content
	for _, label := range []string{"Sign in successful", "Messages", "Groups", "Profile", "Exit"} {
		if !strings.Contains(view, label) {
			t.Errorf("home view is missing %q", label)
		}
	}
	if len(home.home.list.Items()) != 4 {
		t.Fatal("expected exactly four home options")
	}
}

func TestHomeExitQuits(t *testing.T) {
	m := newHomeModel(80, 24)
	m.list.Select(3)
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected exit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("exit did not quit")
	}
}
