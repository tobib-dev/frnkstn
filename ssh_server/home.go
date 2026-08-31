package main

import (
	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type homeState int

const (
	directMessage homeState = iota
	servers
	signOut
)

type homeItem int

const (
	signInHomeItem homeItem = iota
	exitHomeItem
)

func (i homeItem) FilterValue() string {
	if i == signInHomeItem {
		return "Sign in"
	}
	return "Exit"
}
func (i homeItem) Title() string       { return i.FilterValue() }
func (i homeItem) Description() string { return "" }

type homeModel struct {
	state homeState
	list  list.Model
}
type SwitchToHomeMsg struct{}

func newHomeModel(width, height int) homeModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	l := list.New([]list.Item{homeItem(signInHomeItem), homeItem(exitHomeItem)}, delegate, width, height)
	l.Title = "Home"
	return homeModel{list: l}
}

func (m homeModel) Update(msg tea.Msg) (homeModel, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.list.SetSize(size.Width, size.Height)
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		switch m.list.SelectedItem().(homeItem) {
		case signInHomeItem:
			return m, func() tea.Msg { return switchToSignInMsg{} }
		case exitHomeItem:
			m.state = signOut
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m homeModel) View() tea.View {
	return tea.NewView(lipgloss.NewStyle().Margin(2).Render(m.list.View()))
}
