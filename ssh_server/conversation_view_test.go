package main

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestConversationViewKeepsComposerVisible(t *testing.T) {
	history := "oldest message\n" + strings.Repeat("A very long message that wraps across the terminal. ", 200) + "\nnewest message"
	for _, size := range [][2]int{{80, 24}, {30, 12}, {12, 6}, {8, 5}} {
		view := conversationView("Channel", history, "Message: draft", "Sending…", "PgUp/PgDn scroll", size[0], size[1], 0)
		if lipgloss.Height(view) > size[1]-conversationMargin || lipgloss.Width(view) > size[0]-conversationMargin {
			t.Fatalf("view exceeds %dx%d: %dx%d", size[0], size[1], lipgloss.Width(view), lipgloss.Height(view))
		}
		if !strings.Contains(view, "Mess") {
			t.Fatalf("composer missing at size %v: %q", size, view)
		}
	}
	view := conversationView("Channel", history, "Message: draft", "", "Scroll", 80, 24, 0)
	if !strings.Contains(view, "newest message") || !strings.Contains(view, "─") {
		t.Fatal("latest message or divider missing")
	}
	view = conversationView("Channel", history, "Message: draft", "", "Scroll", 80, 24, 10000)
	if !strings.Contains(view, "oldest message") || !strings.Contains(view, "Message: draft") {
		t.Fatal("scrolling lost history or composer")
	}
}
