// Edits and deletes notes from the notes viewer, like ctrl+e and ctrl+d in the command search.

package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ynn3m/arsenal/internal/session"
)

const maxEditLines = 5000

type noteEdit struct {
	note  *session.Note
	title textinput.Model
	body  textarea.Model
	focus int
	err   string
}

func newNoteEdit(n *session.Note) *noteEdit {
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 500
	ti.SetValue(n.Title)
	ti.CursorEnd()

	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.MaxHeight = 0
	ta.FocusedStyle.CursorLine = ValueStyle
	ta.FocusedStyle.Placeholder = MutedStyle
	ta.BlurredStyle.Placeholder = MutedStyle
	ta.SetValue(n.Body)

	e := &noteEdit{note: n, title: ti, body: ta}
	e.setFocus(0)
	return e
}

func (e *noteEdit) setFocus(i int) {
	e.focus = (i + 2) % 2
	if e.focus == 0 {
		e.title.Focus()
		e.body.Blur()
	} else {
		e.title.Blur()
		e.body.Focus()
	}
}

func (e *noteEdit) content(width, h int) string {
	inputW := max(width-2-editLabelW-2, 8)
	e.title.Width = inputW - 1
	e.body.SetWidth(inputW)
	e.body.SetHeight(max(h-1, 1))

	label := func(i int, name string) string {
		name = padTo(name, editLabelW)
		if i == e.focus {
			return ActiveStyle.Render("> ") + TitleStyle.Render(name) + "  "
		}
		return "  " + KeyStyle.Render(name) + "  "
	}
	rows := []string{label(0, "Title") + padTo(e.title.View(), inputW)}
	blank := strings.Repeat(" ", 2+editLabelW+2)
	for k, l := range strings.Split(e.body.View(), "\n") {
		if k == 0 {
			rows = append(rows, label(1, "Text")+padTo(l, inputW))
		} else {
			rows = append(rows, blank+padTo(l, inputW))
		}
	}
	return strings.Join(rows, "\n")
}

func noteName(n *session.Note) string {
	if n.Title != "" {
		return fit1(n.Title, 40)
	}
	if t := firstWords(n.Body, 40); t != "" {
		return t
	}
	return n.Time.Format("2006-01-02 15:04")
}

func (m notesModel) openNoteEdit() (tea.Model, tea.Cmd) {
	n := m.selected()
	switch {
	case n == nil:
		return m, nil
	case m.opts.Save == nil:
		m.notice = Warn("Editing is not available here")
		return m, nil
	case strings.Count(n.Body, "\n") >= maxEditLines:
		m.notice = Warn("This note is too long to edit here: edit " + shortPath(m.opts.Path) + " by hand")
		return m, nil
	}
	m.edit = newNoteEdit(n)
	m.focus = focusPreview
	m.input.Blur()
	m.resize(m.w, m.h)
	return m, textinput.Blink
}

func (m *notesModel) closeNoteEdit() {
	m.edit = nil
	m.focus = focusList
	m.input.Focus()
	m.resize(m.w, m.h)
}

func (m notesModel) updateNoteEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	e := m.edit
	if km, ok := msg.(tea.KeyMsg); ok {
		e.err = ""
		switch km.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			m.closeNoteEdit()
			return m, textinput.Blink
		case "ctrl+s":
			return m.saveNoteEdit()
		case "tab", "shift+tab":
			e.setFocus(e.focus + 1)
			return m, nil
		case "enter":
			if e.focus == 0 {
				e.setFocus(1)
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	if e.focus == 0 {
		e.title, cmd = e.title.Update(msg)
	} else {
		e.body, cmd = e.body.Update(msg)
	}
	return m, cmd
}

func (m notesModel) saveNoteEdit() (tea.Model, tea.Cmd) {
	e := m.edit
	title, body := e.title.Value(), e.body.Value()
	if strings.TrimSpace(title) == "" && strings.TrimSpace(body) == "" {
		e.err = "the note is empty: write some text"
		return m, nil
	}
	res, err := m.opts.Save(*e.note, title, body)
	if err != nil {
		e.err = err.Error()
		return m, nil
	}
	m.edit = nil
	m.focus = focusList
	m.input.Focus()
	m.applyNotes(res, m.sel)
	return m, textinput.Blink
}

func (m notesModel) openNoteDelete() (tea.Model, tea.Cmd) {
	n := m.selected()
	switch {
	case n == nil:
	case m.opts.Delete == nil:
		m.notice = Warn("Deleting is not available here")
	default:
		m.del = n
		m.focus = focusPreview
	}
	return m, nil
}

func (m notesModel) noteDeleteContent(width int) string {
	n := m.del
	wrap := lipgloss.NewStyle().Width(width)
	return strings.Join([]string{
		wrap.Render("Delete this note from " + shortPath(m.opts.Path) + "?"),
		"",
		wrap.Render(MutedStyle.Render(n.Time.Format(dayFormat+", 15:04")+"  ") + noteName(n)),
		"",
		wrap.Render(MutedStyle.Render("Press ") + HelpKeyStyle.Render("y") + MutedStyle.Render(" to delete it, any other key to keep it.")),
	}, "\n")
}

func (m notesModel) updateNoteDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := m.del
	m.del = nil
	m.focus = focusList
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "y", "Y":
	default:
		return m, nil
	}
	res, err := m.opts.Delete(*n)
	if err != nil {
		m.notice = Error(err.Error())
		return m, nil
	}
	m.applyNotes(res, m.sel)
	return m, nil
}

func (m *notesModel) applyNotes(res NotesResult, near int) {
	m.setNotes(res.Notes)
	m.refilter()
	sel := -1
	for i, r := range m.rows {
		if !r.isHeader() && r.note.Index == res.Select {
			sel = i
			break
		}
	}
	if sel < 0 && len(m.rows) > 0 {
		sel = m.snap(min(near, len(m.rows)-1), 1)
	}
	m.sel = sel
	m.shown = nil
	m.resize(m.w, m.h)
	m.notice = Success(res.Message)
}
