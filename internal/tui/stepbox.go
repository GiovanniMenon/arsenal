// The step box asks one question at a time in a single box, for setup, add and note.

package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type StepKind int

const (
	StepText StepKind = iota
	StepPath
	StepChoice
	StepMultiline
)

type Step struct {
	Key   string
	Title string
	Help  string

	Kind StepKind

	Default string

	Options       []string
	DefaultOption int

	Validate     func(string) error
	ValidateWith func(Answers, string) error
	When         func(Answers) bool
}

type Answer struct {
	Text  string
	Index int
}

type Answers map[string]Answer

func (a Answers) Text(key string) string { return a[key].Text }

func (a Answers) Choice(key string) int {
	if ans, ok := a[key]; ok {
		return ans.Index
	}
	return -1
}

type StepBox struct {
	Title     string
	Steps     []Step
	Summary   func(Answers) []string
	Confirmed func(Answers) []string
}

func (b StepBox) Active(a Answers) []Step {
	var out []Step
	for _, s := range b.Steps {
		if s.When == nil || s.When(a) {
			out = append(out, s)
		}
	}
	return out
}

func Rows(rows ...[2]string) []string {
	w := 0
	for _, r := range rows {
		w = max(w, ansi.StringWidth(r[0]))
	}
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = KeyStyle.Render(r[0]) + strings.Repeat(" ", w-ansi.StringWidth(r[0])) + "  " + r[1]
	}
	return out
}

func RunSteps(b StepBox) (Answers, error) {
	in, out, closeTTY := TTY()
	defer closeTTY()
	final, err := newProgram(newStepModel(b), tea.WithInput(in), tea.WithOutput(out)).Run()
	if err != nil {
		return nil, err
	}
	m := final.(stepModel)
	if m.canceled {
		return nil, ErrCancelled
	}
	return m.answers, nil
}

type stepModel struct {
	box     StepBox
	answers Answers
	cur     int
	summary bool
	w       int

	input textinput.Model
	area  textarea.Model
	list  int
	err   string

	done, canceled bool
}

func newStepModel(b StepBox) stepModel {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	ta.CharLimit = 4000
	ta.FocusedStyle.CursorLine = ValueStyle
	ta.FocusedStyle.Placeholder = MutedStyle
	ta.BlurredStyle.Placeholder = MutedStyle
	ta.KeyMap.InsertNewline = key.NewBinding(key.WithKeys("alt+enter", "ctrl+j"))
	ta.SetHeight(4)

	ti := textinput.New()
	ti.Prompt = "> "
	ti.PromptStyle = HelpKeyStyle
	ti.CharLimit = 1024

	m := stepModel{box: b, answers: Answers{}, input: ti, area: ta, w: 80}
	m.cur = m.firstActive()
	if m.cur < 0 {
		m.summary = true
	} else {
		m.load()
	}
	return m
}

func (m stepModel) Init() tea.Cmd { return textinput.Blink }

func (m stepModel) firstActive() int {
	for i, s := range m.box.Steps {
		if s.When == nil || s.When(m.answers) {
			return i
		}
	}
	return -1
}

func (m stepModel) isActive(i int) bool {
	s := m.box.Steps[i]
	return s.When == nil || s.When(m.answers)
}

func (m stepModel) next(i int) int {
	for j := i + 1; j < len(m.box.Steps); j++ {
		if m.isActive(j) {
			return j
		}
	}
	return -1
}

func (m stepModel) prev(i int) int {
	for j := i - 1; j >= 0; j-- {
		if m.isActive(j) {
			return j
		}
	}
	return -1
}

func (m stepModel) position() (n, total int) {
	for i := range m.box.Steps {
		if !m.isActive(i) {
			continue
		}
		total++
		if !m.summary && i <= m.cur {
			n = total
		}
	}
	if m.summary {
		n = total
	}
	return
}

func (m *stepModel) load() {
	s := m.box.Steps[m.cur]
	m.err = ""
	prev, had := m.answers[s.Key]
	switch s.Kind {
	case StepChoice:
		m.list = min(max(s.DefaultOption, 0), max(len(s.Options)-1, 0))
		if had {
			m.list = min(max(prev.Index, 0), max(len(s.Options)-1, 0))
		}
	case StepMultiline:
		v := s.Default
		if had {
			v = prev.Text
		}
		m.area.SetValue(v)
		m.area.Focus()
	default:
		v := s.Default
		if had {
			v = prev.Text
		}
		m.input.SetValue(v)
		m.input.CursorEnd()
		m.input.ShowSuggestions = s.Kind == StepPath
		if s.Kind == StepPath {
			m.input.KeyMap.AcceptSuggestion = key.NewBinding(key.WithKeys("tab", "ctrl+e"))
			m.input.CompletionStyle = MutedStyle
		}
		m.input.Focus()
		m.refreshSuggestion()
	}
}

func (m *stepModel) refreshSuggestion() {
	if m.box.Steps[m.cur].Kind != StepPath {
		return
	}
	v := m.input.Value()
	if c := CompletePath(v); c != "" && c != v {
		m.input.SetSuggestions([]string{c})
		return
	}
	m.input.SetSuggestions(nil)
}

func (m *stepModel) store() bool {
	s := m.box.Steps[m.cur]
	switch s.Kind {
	case StepChoice:
		m.answers[s.Key] = Answer{Text: s.Options[m.list], Index: m.list}
	case StepMultiline:
		v := strings.TrimSpace(m.area.Value())
		if err := m.validate(s, v); err != nil {
			m.err = err.Error()
			return false
		}
		m.answers[s.Key] = Answer{Text: v, Index: -1}
	default:
		v := strings.TrimSpace(m.input.Value())
		if err := m.validate(s, v); err != nil {
			m.err = err.Error()
			return false
		}
		m.answers[s.Key] = Answer{Text: v, Index: -1}
	}
	m.err = ""
	return true
}

func (m *stepModel) validate(s Step, v string) error {
	if s.Validate != nil {
		if err := s.Validate(v); err != nil {
			return err
		}
	}
	if s.ValidateWith != nil {
		return s.ValidateWith(m.answers, v)
	}
	return nil
}

func (m stepModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w = msg.Width
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	}
	return m.forward(msg)
}

func (m stepModel) forward(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.summary || m.done {
		return m, nil
	}
	var cmd tea.Cmd
	switch m.box.Steps[m.cur].Kind {
	case StepChoice:
	case StepMultiline:
		m.area, cmd = m.area.Update(msg)
	default:
		m.input, cmd = m.input.Update(msg)
	}
	return m, cmd
}

func (m stepModel) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		m.canceled, m.done = true, true
		return m, tea.Quit
	case "esc":
		if m.summary {
			m.summary = false
			m.cur = m.prev(len(m.box.Steps))
			if m.cur < 0 {
				m.canceled, m.done = true, true
				return m, tea.Quit
			}
			m.load()
			return m, textinput.Blink
		}
		if p := m.prev(m.cur); p >= 0 {
			m.cur = p
			m.load()
			return m, textinput.Blink
		}
		m.canceled, m.done = true, true
		return m, tea.Quit
	case "enter":
		if m.summary {
			m.done = true
			return m, tea.Quit
		}
		if !m.store() {
			return m, nil
		}
		if n := m.next(m.cur); n >= 0 {
			m.cur = n
			m.load()
			return m, textinput.Blink
		}
		m.summary = true
		return m, nil
	}
	if m.summary {
		return m, nil
	}
	s := m.box.Steps[m.cur]
	m.err = ""
	if s.Kind == StepChoice {
		switch msg.String() {
		case "up", "k", "shift+tab":
			m.list = (m.list + len(s.Options) - 1) % len(s.Options)
		case "down", "j", "tab":
			m.list = (m.list + 1) % len(s.Options)
		}
		return m, nil
	}
	var cmd tea.Cmd
	if s.Kind == StepMultiline {
		m.area, cmd = m.area.Update(msg)
		return m, cmd
	}
	m.input, cmd = m.input.Update(msg)
	m.refreshSuggestion()
	return m, cmd
}

func (m stepModel) boxWidth() int { return min(max(m.w, 40), 90) }

func (m stepModel) View() string {
	inner := m.boxWidth() - 4
	var body []string
	var help string
	label := "Summary"
	n, total := m.position()
	right := MutedStyle.Render(fmt.Sprintf("%d/%d", n, total))

	switch {
	case m.summary || m.done:
		var lines []string
		switch {
		case m.done && !m.canceled && m.box.Confirmed != nil:
			lines = m.box.Confirmed(m.answers)
		case m.box.Summary != nil:
			lines = m.box.Summary(m.answers)
		}
		body = lines
		if m.done {
			label, right = "Done", ""
		} else {
			help = "enter: confirm  |  esc: back  |  ctrl+c: cancel"
		}
	default:
		s := m.box.Steps[m.cur]
		label = s.Title
		if s.Help != "" {
			body = append(body, MutedStyle.Render(s.Help), "")
		}
		switch s.Kind {
		case StepChoice:
			for i, o := range s.Options {
				if i == m.list {
					body = append(body, ActiveStyle.Render("> "+o))
				} else {
					body = append(body, "  "+o)
				}
			}
		case StepMultiline:
			m.area.SetWidth(inner)
			body = append(body, m.area.View())
		default:
			m.input.Width = max(inner-3, 8)
			body = append(body, m.input.View())
		}
		if m.err != "" {
			body = append(body, ErrorStyle.Render("error: "+m.err))
		}
		help = m.helpFor(s)
	}
	box := panel{left: Title(m.box.Title) + MutedStyle.Render(" - ") + Title(label), right: right, focused: !m.done}.
		render(strings.Join(body, "\n"), m.boxWidth())
	if help == "" {
		return box + "\n"
	}
	return box + "\n" + MutedStyle.Render(ansi.Truncate(help, m.boxWidth(), "...")) + "\n"
}

func (m stepModel) helpFor(s Step) string {
	back := "esc: back"
	if m.prev(m.cur) < 0 {
		back = "esc: cancel"
	}
	switch s.Kind {
	case StepChoice:
		return "up/down: choose  |  enter: next  |  " + back + "  |  ctrl+c: cancel"
	case StepPath:
		return "tab: complete path  |  enter: next  |  " + back + "  |  ctrl+c: cancel"
	case StepMultiline:
		return "enter: next  |  alt+enter or ctrl+j: new line  |  " + back + "  |  ctrl+c: cancel"
	}
	return "enter: next  |  " + back + "  |  ctrl+c: cancel"
}
