package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestStartupFilterDropsEarlyEscAndCtrlC(t *testing.T) {
	start := time.Now()
	now := start
	f := startupFilterAt(func() time.Time { return now }, start)

	esc := tea.KeyMsg{Type: tea.KeyEsc}
	ctrlc := tea.KeyMsg{Type: tea.KeyCtrlC}
	enter := tea.KeyMsg{Type: tea.KeyEnter}
	typed := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")}

	now = start.Add(20 * time.Millisecond)
	if f(nil, esc) != nil || f(nil, ctrlc) != nil {
		t.Error("an early Esc / Ctrl+C was not dropped")
	}
	for _, m := range []tea.Msg{enter, typed, tea.WindowSizeMsg{Width: 80, Height: 24}} {
		if f(nil, m) == nil {
			t.Errorf("%T was dropped", m)
		}
	}
	now = start.Add(startupGrace + time.Millisecond)
	if f(nil, esc) == nil || f(nil, ctrlc) == nil {
		t.Error("Esc / Ctrl+C dropped after the grace period")
	}
}
