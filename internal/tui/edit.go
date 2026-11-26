// The form that edits an entry from the search screen.

package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/ynn3m/arsenal/internal/cheats"
)

type SaveResult struct {
	Message string
	Cheats  []*cheats.Cheat
	Saved   *cheats.Cheat
}

const (
	fieldCommand = iota
	fieldTitle
	fieldTags
	fieldDescription
	fieldCount
)

var fieldLabels = [fieldCount]string{"Command", "Title", "Tags", "Description"}

const (
	editLabelW  = 11
	descRowsMax = 4
)

type editState struct {
	cheat  *cheats.Cheat
	inputs [fieldDescription]textinput.Model
	desc   textarea.Model
	focus  int
	err    string
}

func newEdit(c *cheats.Cheat) *editState {
	e := &editState{cheat: c}
	var tags []string
	for _, t := range c.TagItems() {
		if !strings.EqualFold(t, cheats.SourceTag) {
			tags = append(tags, t)
		}
	}
	values := [fieldDescription]string{c.PrintableCommand(), strings.TrimSpace(c.Name), strings.Join(tags, ", ")}
	for i := range e.inputs {
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = 1000
		ti.SetValue(values[i])
		ti.CursorEnd()
		e.inputs[i] = ti
	}
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 4000
	ta.FocusedStyle.CursorLine = ValueStyle
	ta.FocusedStyle.Placeholder = MutedStyle
	ta.BlurredStyle.Placeholder = MutedStyle
	ta.SetHeight(descRowsMax)
	ta.SetValue(c.Description)
	ta.CursorEnd()
	e.desc = ta
	e.setFocus(fieldCommand)
	return e
}

func (e *editState) setFocus(i int) {
	e.focus = (i + fieldCount) % fieldCount
	for k := range e.inputs {
		if k == e.focus {
			e.inputs[k].Focus()
		} else {
			e.inputs[k].Blur()
		}
	}
	if e.focus == fieldDescription {
		e.desc.Focus()
	} else {
		e.desc.Blur()
	}
}

func (e *editState) entry() cheats.Entry {
	command := strings.TrimSpace(e.inputs[fieldCommand].Value())
	if command == e.cheat.PrintableCommand() {
		command = strings.ReplaceAll(e.cheat.Command, "\\\n", "\n")
	}
	return cheats.Entry{
		Command:     command,
		Title:       e.inputs[fieldTitle].Value(),
		Tags:        e.inputs[fieldTags].Value(),
		Description: e.desc.Value(),
	}
}

func (e *editState) height() int { return fieldDescription + descRowsMax }

func (e *editState) content(width, h int) string {
	inputW := max(width-2-editLabelW-2, 8)
	descRows := descRowsMax
	if extra := e.height() - h; extra > 0 {
		descRows = max(descRows-extra, 1)
	}
	e.desc.SetWidth(inputW)
	e.desc.SetHeight(descRows)

	label := func(i int) string {
		name := padTo(fieldLabels[i], editLabelW)
		if i == e.focus {
			return ActiveStyle.Render("> ") + TitleStyle.Render(name) + "  "
		}
		return "  " + KeyStyle.Render(name) + "  "
	}
	var rows []string
	for i := range e.inputs {
		in := e.inputs[i]
		in.Width = inputW - 1
		rows = append(rows, label(i)+padTo(in.View(), inputW))
	}
	blank := strings.Repeat(" ", 2+editLabelW+2)
	for k, l := range strings.Split(e.desc.View(), "\n") {
		if k == 0 {
			rows = append(rows, label(fieldDescription)+padTo(l, inputW))
		} else {
			rows = append(rows, blank+padTo(l, inputW))
		}
	}
	return strings.Join(rows, "\n")
}

type editKeys struct{ Move, Save, Cancel key.Binding }

func newEditKeys() editKeys {
	return editKeys{
		Move:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab/shift+tab", "next/previous field")),
		Save:   key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "save")),
		Cancel: key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "cancel")),
	}
}

func (k editKeys) ShortHelp() []key.Binding  { return []key.Binding{k.Move, k.Save, k.Cancel} }
func (k editKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func (m searchModel) openEdit() (tea.Model, tea.Cmd) {
	c := m.selected()
	if c == nil {
		return m, nil
	}
	if m.opts.Save == nil {
		m.notice = Warn("Editing is not available here")
		return m, nil
	}
	if m.opts.ReadOnly != nil && m.opts.ReadOnly(c) {
		m.notice = Warn("This command is in a read-only folder: " + shortPath(c.Filename))
		return m, nil
	}
	m.edit = newEdit(c)
	m.focus = focusPreview
	m.input.Blur()
	m.applyLayout()
	return m, textinput.Blink
}

func (m *searchModel) closeEdit() {
	m.edit = nil
	m.focus = focusList
	m.input.Focus()
	m.shown = nil
	m.applyLayout()
	m.syncPreview()
}

func (m searchModel) updateEdit(msg tea.Msg) (tea.Model, tea.Cmd) {
	e := m.edit
	km, isKey := msg.(tea.KeyMsg)
	if isKey {
		e.err = ""
		switch km.String() {
		case "ctrl+c", "f10":
			return m, tea.Quit
		case "esc":
			m.closeEdit()
			return m, textinput.Blink
		case "ctrl+s":
			return m.saveEdit()
		case "tab":
			e.setFocus(e.focus + 1)
			return m, nil
		case "shift+tab":
			e.setFocus(e.focus - 1)
			return m, nil
		case "enter":
			if e.focus != fieldDescription {
				e.setFocus(e.focus + 1)
				return m, nil
			}
		}
	}
	var cmd tea.Cmd
	if e.focus == fieldDescription {
		e.desc, cmd = e.desc.Update(msg)
	} else {
		e.inputs[e.focus], cmd = e.inputs[e.focus].Update(msg)
	}
	return m, cmd
}

func (m searchModel) saveEdit() (tea.Model, tea.Cmd) {
	e := m.edit
	entry := e.entry()
	if err := entry.Validate(); err != nil {
		e.err = err.Error()
		return m, nil
	}
	res, err := m.opts.Save(e.cheat, entry)
	if err != nil {
		e.err = err.Error()
		return m, nil
	}
	m.opts.Cheats = res.Cheats
	m.setCheats(res.Cheats)
	m.closeEdit()
	m.refilter()
	for i, c := range m.results {
		if c == res.Saved {
			m.list.Select(i)
			break
		}
	}
	m.shown = nil
	m.syncPreview()
	m.notice = Success(res.Message)
	return m, textinput.Blink
}
