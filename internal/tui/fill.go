// Fills a command's variables in the preview box of the search screen.

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ynn3m/arsenal/internal/cheats"
)

type fillState struct {
	cheat   *cheats.Cheat
	cmd     *cheats.Command
	inputs  []textinput.Model
	origin  []string
	focus   int
	err     string
	sources map[string]string
}

type sourceNames func(name string) string

func newFill(c *cheats.Cheat, saved map[string]string, source sourceNames) *fillState {
	cmd := cheats.NewCommand(c, saved)
	f := &fillState{cheat: c, cmd: cmd, sources: map[string]string{}}
	for _, a := range cmd.Args {
		ti := textinput.New()
		ti.Prompt = ""
		ti.Placeholder = "value needed"
		ti.PlaceholderStyle = MutedStyle
		ti.CharLimit = 500
		ti.ShowSuggestions = true
		ti.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("tab", "ctrl+e"))
		ti.SetValue(a.Value)
		ti.CursorEnd()
		f.inputs = append(f.inputs, ti)
		f.origin = append(f.origin, a.Value)
		if a.Source == cheats.SourceSaved && source != nil {
			f.sources[a.Name] = source(a.Name)
		}
	}
	if len(f.inputs) == 0 {
		return f
	}
	if idx := cmd.NeedsInput(); len(idx) > 0 {
		f.focus = idx[0]
	}
	f.setFocus(f.focus)
	return f
}

func (f *fillState) setFocus(i int) {
	if len(f.inputs) == 0 {
		return
	}
	f.focus = (i + len(f.inputs)) % len(f.inputs)
	for k := range f.inputs {
		if k == f.focus {
			f.inputs[k].Focus()
		} else {
			f.inputs[k].Blur()
		}
	}
	f.refreshSuggestion()
}

func (f *fillState) refreshSuggestion() {
	in := &f.inputs[f.focus]
	v := in.Value()
	if c := CompletePath(v); c != "" && c != v {
		in.SetSuggestions([]string{c})
		return
	}
	in.SetSuggestions(nil)
}

func (f *fillState) value(i int) string { return strings.TrimSpace(f.inputs[i].Value()) }

func (f *fillState) values() map[string]string {
	out := map[string]string{}
	for i, a := range f.cmd.Args {
		out[a.Name] = f.value(i)
	}
	return out
}

func (f *fillState) firstMissing() int {
	for i := range f.inputs {
		if f.value(i) == "" {
			return i
		}
	}
	return -1
}

func (f *fillState) live() *cheats.Command {
	tmp := *f.cmd
	tmp.Args = append([]cheats.Arg(nil), f.cmd.Args...)
	for i := range tmp.Args {
		tmp.Args[i].Value = f.value(i)
	}
	return &tmp
}

func (f *fillState) sourceLabel(i int) string {
	v := f.value(i)
	if v == "" {
		return "missing"
	}
	if v != strings.TrimSpace(f.origin[i]) {
		return "typed"
	}
	switch f.cmd.Args[i].Source {
	case cheats.SourceSaved:
		if s := f.sources[f.cmd.Args[i].Name]; s != "" {
			return s
		}
		return "saved"
	case cheats.SourceDefault, cheats.SourceFile:
		return "default"
	}
	return "typed"
}

func (f *fillState) commandText(prefix string, width int) string {
	live := f.live()
	var b strings.Builder
	if prefix != "" {
		b.WriteString(ActiveStyle.Render(prefix) + " ")
	}
	for _, s := range live.Segments() {
		if s.Arg < 0 {
			b.WriteString(ValueStyle.Render(s.Text))
			continue
		}
		a := live.Args[s.Arg]
		switch {
		case s.Arg == f.focus && a.Value != "":
			b.WriteString(SelectedRowStyle.Render(a.Value))
		case s.Arg == f.focus:
			b.WriteString(SelectedRowStyle.Render("<" + a.Name + ">"))
		case a.Value == "":
			b.WriteString(WarnStyle.Underline(true).Render("<" + a.Name + ">"))
		case f.sourceLabel(s.Arg) == "typed" || f.sourceLabel(s.Arg) == "default":
			b.WriteString(KeyStyle.Render(a.Value))
		default:
			b.WriteString(ActiveStyle.Render(a.Value))
		}
	}
	return lipgloss.NewStyle().Width(width).Render(MutedStyle.Render("$ ") + b.String())
}

func (f *fillState) rows(width int) []string {
	nameW := 0
	for _, a := range f.cmd.Args {
		nameW = max(nameW, ansi.StringWidth(a.Name))
	}
	nameW = min(nameW, 20)
	const srcW = 9
	inputW := max(width-2-nameW-2-2-srcW, 8)

	rows := make([]string, len(f.inputs))
	for i, a := range f.cmd.Args {
		in := f.inputs[i]
		in.Width = inputW - 1
		label := f.sourceLabel(i)
		name := padTo(a.Name, nameW)
		var nameStyled, cursor string
		switch {
		case label == "missing":
			nameStyled = WarnStyle.Bold(true).Render(name)
		case i == f.focus:
			nameStyled = TitleStyle.Render(name)
		default:
			nameStyled = KeyStyle.Render(name)
		}
		cursor = "  "
		if i == f.focus {
			cursor = ActiveStyle.Render("> ")
		}
		src := MutedStyle.Render("(" + label + ")")
		if label == "missing" {
			src = WarnStyle.Render("(" + label + ")")
		}
		rows[i] = cursor + nameStyled + "  " + padTo(in.View(), inputW) + "  " + src
	}
	return rows
}

func (f *fillState) height(prefix string, width int) int {
	return strings.Count(f.commandText(prefix, width), "\n") + 1 + 1 + len(f.inputs)
}

func (f *fillState) content(prefix string, width, h int) string {
	cmdLines := strings.Split(f.commandText(prefix, width), "\n")
	rows := f.rows(width)
	avail := h - len(cmdLines) - 1
	if avail < 1 {
		return strings.Join(cmdLines[:min(len(cmdLines), max(h, 1))], "\n")
	}
	start := 0
	if len(rows) > avail {
		start = min(max(f.focus-avail+1, 0), len(rows)-avail)
		rows = rows[start : start+avail]
	}
	return strings.Join(cmdLines, "\n") + "\n\n" + strings.Join(rows, "\n")
}

type fillKeys struct{ Move, Insert, Back key.Binding }

func newFillKeys() fillKeys {
	return fillKeys{
		Move:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab/shift+tab", "next/previous variable")),
		Insert: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "insert the command")),
		Back:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back to the list")),
	}
}

func (k fillKeys) ShortHelp() []key.Binding  { return []key.Binding{k.Move, k.Insert, k.Back} }
func (k fillKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

func (m searchModel) updateFill(msg tea.Msg) (tea.Model, tea.Cmd) {
	f := m.fill
	km, isKey := msg.(tea.KeyMsg)
	if !isKey {
		var cmd tea.Cmd
		f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
		return m, cmd
	}
	f.err = ""
	switch km.String() {
	case "ctrl+c", "f10":
		return m, tea.Quit
	case "esc":
		m.fill = nil
		m.focus = focusList
		m.input.Focus()
		m.shown = nil
		m.applyLayout()
		m.syncPreview()
		return m, textinput.Blink
	case "enter":
		if i := f.firstMissing(); i >= 0 {
			f.setFocus(i)
			f.err = "missing value for " + f.cmd.Args[i].Name
			return m, nil
		}
		m.chosen = f.cheat
		m.values = f.values()
		return m, tea.Quit
	case "tab":
		in := f.inputs[f.focus]
		if s := in.CurrentSuggestion(); s != "" && s != in.Value() {
			break
		}
		f.setFocus(f.focus + 1)
		return m, nil
	case "shift+tab", "up":
		f.setFocus(f.focus - 1)
		return m, nil
	case "down":
		f.setFocus(f.focus + 1)
		return m, nil
	}
	var cmd tea.Cmd
	f.inputs[f.focus], cmd = f.inputs[f.focus].Update(msg)
	f.refreshSuggestion()
	m.applyLayout()
	return m, cmd
}

func (m searchModel) fillLabels() (left, right string) {
	f := m.fill
	left = Title(strings.TrimSpace(f.cheat.Name))
	n := len(f.inputs)
	right = MutedStyle.Render(fmt.Sprintf("%d variable", n))
	if n != 1 {
		right = MutedStyle.Render(fmt.Sprintf("%d variables", n))
	}
	return
}

func padTo(s string, width int) string {
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, "")
	}
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}
