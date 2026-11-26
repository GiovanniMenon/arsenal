// Asks for confirmation before deleting an entry from the search screen.

package tui

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type deleteKeys struct{ Yes, No key.Binding }

func newDeleteKeys() deleteKeys {
	return deleteKeys{
		Yes: key.NewBinding(key.WithKeys("y"), key.WithHelp("y", "delete")),
		No:  key.NewBinding(key.WithKeys("esc"), key.WithHelp("any other key", "cancel")),
	}
}

func (k deleteKeys) ShortHelp() []key.Binding  { return []key.Binding{k.Yes, k.No} }
func (k deleteKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func (m searchModel) openDelete() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil {
		return m, nil
	}
	switch {
	case m.opts.Delete == nil:
		m.notice = Warn("Deleting is not available here")
	case m.opts.ReadOnly != nil && m.opts.ReadOnly(c):
		m.notice = Warn("This command is in a read-only folder: " + shortPath(c.Filename))
	default:
		m.del = c
		m.focus = focusPreview
	}
	return m, nil
}

func (m searchModel) deleteContent(width int) string {
	c := m.del
	wrap := lipgloss.NewStyle().Width(width)
	return strings.Join([]string{
		wrap.Render("Delete this command from " + filepath.Base(c.Filename) + "?"),
		"",
		wrap.Render(MutedStyle.Render("$ ") + c.PrintableCommand()),
		"",
		wrap.Render(MutedStyle.Render("Press ") + HelpKeyStyle.Render("y") + MutedStyle.Render(" to delete it, any other key to keep it.")),
	}, "\n")
}

func (m searchModel) updateDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	c := m.del
	m.del = nil
	m.focus = focusList
	switch msg.String() {
	case "ctrl+c", "f10":
		return m, tea.Quit
	case "y", "Y":
	default:
		return m, nil
	}
	res, err := m.opts.Delete(c)
	if err != nil {
		m.notice = Error(err.Error())
		return m, nil
	}
	pos := m.pos()
	m.opts.Cheats = res.Cheats
	m.setCheats(res.Cheats)
	m.refilter()
	if len(m.results) > 0 {
		m.list.Select(min(pos, len(m.results)-1))
	}
	m.shown = nil
	m.syncPreview()
	m.notice = Success(res.Message)
	return m, nil
}
