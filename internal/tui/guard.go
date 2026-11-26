// Ignores esc and ctrl+c just after a screen opens, when they are usually terminal noise.

package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const startupGrace = 200 * time.Millisecond

func startupFilter() func(tea.Model, tea.Msg) tea.Msg {
	return startupFilterAt(time.Now, time.Now())
}

func startupFilterAt(now func() time.Time, start time.Time) func(tea.Model, tea.Msg) tea.Msg {
	return func(_ tea.Model, msg tea.Msg) tea.Msg {
		k, ok := msg.(tea.KeyMsg)
		if !ok {
			return msg
		}
		if (k.Type == tea.KeyEsc || k.Type == tea.KeyCtrlC) && now().Sub(start) < startupGrace {
			return nil
		}
		return msg
	}
}

func newProgram(m tea.Model, opts ...tea.ProgramOption) *tea.Program {
	return tea.NewProgram(m, append(opts, tea.WithFilter(startupFilter()))...)
}
