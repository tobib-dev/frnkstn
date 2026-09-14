package main

import (
	"time"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type homeItem int

const (
	messagesHomeItem homeItem = iota
	groupsHomeItem
	profileHomeItem
	exitHomeItem
)

func (i homeItem) FilterValue() string {
	switch i {
	case messagesHomeItem:
		return "Messages"
	case groupsHomeItem:
		return "Groups"
	case profileHomeItem:
		return "Profile"
	default:
		return "Exit"
	}
}
func (i homeItem) Title() string       { return i.FilterValue() }
func (i homeItem) Description() string { return "" }

type homeModel struct {
	list list.Model
}
type SwitchToHomeMsg struct{}

func newHomeModel(width, height int) homeModel {
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = false
	l := list.New([]list.Item{messagesHomeItem, groupsHomeItem, profileHomeItem, exitHomeItem}, delegate, width, height)
	l.Title = "Home"
	l.StatusMessageLifetime = 4 * time.Second
	return homeModel{list: l}
}

func (m homeModel) Update(msg tea.Msg) (homeModel, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.list.SetSize(size.Width, size.Height)
		return m, nil
	}
	if key, ok := msg.(tea.KeyPressMsg); ok && key.String() == "enter" {
		switch m.list.SelectedItem().(homeItem) {
		case exitHomeItem:
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
