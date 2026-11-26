// The full-screen command history viewer.

package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/search"
)

type HistoryOptions struct {
	Session string
	Path    string
	Entries []cmdlog.Entry
	Query   string
	Mask    func(string) string
}

type HistoryResult struct {
	Entry  *cmdlog.Entry
	Replay bool
	Query  string
}

func RunHistory(o HistoryOptions) (HistoryResult, error) {
	in, out, closeTTY := TTY()
	defer closeTTY()
	final, err := newProgram(newHistoryModel(o), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen()).Run()
	if err != nil {
		return HistoryResult{}, err
	}
	m := final.(historyModel)
	return HistoryResult{Entry: m.chosen, Replay: m.replay, Query: m.input.Value()}, nil
}

func (o HistoryOptions) maskText(s string) string {
	if o.Mask == nil {
		return s
	}
	return o.Mask(s)
}

type historyKeys struct {
	Select, Type, Exclude, Use, Replay, Panel, Quit key.Binding
}

func newHistoryKeys() historyKeys {
	return historyKeys{
		Select: key.NewBinding(key.WithKeys("up", "down", "pgup", "pgdown"), key.WithHelp("up/down", "select")),
		Use:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "use")),
		Replay: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "replay")),
		Panel:  key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "panel")),
		Quit:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear / quit")),

		Type:    key.NewBinding(key.WithKeys("a"), key.WithHelp("type", "filter")),
		Exclude: key.NewBinding(key.WithKeys("!"), key.WithHelp("!word", "exclude")),
	}
}

func (k historyKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Type, k.Exclude, k.Use, k.Replay, k.Quit, k.Panel}
}
func (k historyKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

type histRow struct {
	day   string
	entry *cmdlog.Entry
}

func (r histRow) isHeader() bool { return r.entry == nil }

type historyModel struct {
	opts HistoryOptions

	entries []*cmdlog.Entry
	idx     *search.TextIndex

	input   textinput.Model
	preview viewport.Model
	help    help.Model
	keys    historyKeys
	focus   focus
	w, h    int

	rows   []histRow
	sel    int
	offset int
	terms  []string
	shown  *cmdlog.Entry
	notice string

	chosen *cmdlog.Entry
	replay bool
}

func newHistoryModel(o HistoryOptions) historyModel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.PromptStyle = HelpKeyStyle
	ti.Placeholder = "search the commands: words, !exclude"
	ti.CharLimit = 200
	ti.SetValue(o.Query)
	ti.CursorEnd()
	ti.Focus()

	hp := help.New()
	hp.ShortSeparator = "  |  "
	hp.Ellipsis = "..."
	hp.Styles.ShortKey = HelpKeyStyle
	hp.Styles.ShortDesc = MutedStyle
	hp.Styles.ShortSeparator = MutedStyle
	hp.Styles.Ellipsis = MutedStyle

	vp := viewport.New(0, 0)
	vp.KeyMap = viewport.KeyMap{
		Up:       key.NewBinding(key.WithKeys("up")),
		Down:     key.NewBinding(key.WithKeys("down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup")),
		PageDown: key.NewBinding(key.WithKeys("pgdown")),
	}

	var entries []*cmdlog.Entry
	for i := len(o.Entries) - 1; i >= 0; i-- {
		entries = append(entries, &o.Entries[i])
	}
	sort.SliceStable(entries, func(a, b int) bool { return entries[a].Start.TS > entries[b].Start.TS })
	docs := make([][]string, len(entries))
	for i, e := range entries {
		cheat := ""
		if e.Insert != nil {
			cheat = e.Insert.Cheat
		}
		docs[i] = []string{o.maskText(e.Start.Cmd), o.maskText(e.Start.CWD), cheat}
	}

	m := historyModel{
		opts: o, entries: entries, idx: search.NewTextIndex(docs),
		input: ti, preview: vp, help: hp, keys: newHistoryKeys(),
	}
	m.refilter()
	return m
}

func (m historyModel) Init() tea.Cmd { return textinput.Blink }

func (m historyModel) innerWidth() int { return max(m.w-4, 10) }

func (m historyModel) layout() (listH, previewH int) {
	content := m.h - 3 - 2 - 2 - 1
	previewH = min(max(content/3, 3), 10)
	listH = content - previewH
	if listH < 3 {
		listH = 3
		previewH = max(content-listH, 1)
	}
	return
}

func (m *historyModel) resize(w, h int) {
	m.w, m.h = w, h
	inner := m.innerWidth()
	_, previewH := m.layout()
	m.input.Width = max(inner-3, 5)
	m.preview.Width, m.preview.Height = inner, previewH
	m.help.Width = w
	m.shown = nil
	m.fixOffset()
	m.syncPreview()
}

func (m *historyModel) refilter() {
	query := m.input.Value()
	ids := m.idx.Filter(query)
	sort.Ints(ids)
	m.terms = search.Terms(query)

	m.rows = m.rows[:0]
	day := ""
	for _, i := range ids {
		e := m.entries[i]
		if d := e.Time().Format("2006-01-02"); d != day {
			day = d
			m.rows = append(m.rows, histRow{day: e.Time().Format(dayFormat)})
		}
		m.rows = append(m.rows, histRow{entry: e})
	}
	m.sel = m.nextEntry(-1, 1)
	m.offset = 0
	m.shown = nil
	m.syncPreview()
}

func (m historyModel) nextEntry(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(m.rows); i += dir {
		if !m.rows[i].isHeader() {
			return i
		}
	}
	return -1
}

func (m historyModel) snap(i, dir int) int {
	i = min(max(i, 0), len(m.rows)-1)
	if i < 0 {
		return -1
	}
	if !m.rows[i].isHeader() {
		return i
	}
	if n := m.nextEntry(i, dir); n >= 0 {
		return n
	}
	return m.nextEntry(i, -dir)
}

func (m *historyModel) moveSel(delta int) {
	if m.sel < 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	var n int
	if delta == 1 || delta == -1 {
		n = m.nextEntry(m.sel, dir)
	} else {
		n = m.snap(m.sel+delta, dir)
	}
	if n >= 0 {
		m.sel = n
		m.notice = ""
		m.fixOffset()
		m.syncPreview()
	}
}

func (m *historyModel) fixOffset() {
	listH, _ := m.layout()
	if m.sel < 0 {
		m.offset = 0
		return
	}
	top := m.sel
	if top > 0 && m.rows[top-1].isHeader() {
		top--
	}
	if top < m.offset {
		m.offset = top
	}
	if m.sel >= m.offset+listH {
		m.offset = m.sel - listH + 1
	}
	m.offset = min(max(m.offset, 0), max(len(m.rows)-listH, 0))
}

func (m historyModel) selected() *cmdlog.Entry {
	if m.sel < 0 {
		return nil
	}
	return m.rows[m.sel].entry
}

func previewLines(e *cmdlog.Entry, width int, mask func(string) string) []string {
	if mask == nil {
		mask = func(s string) string { return s }
	}
	var lines []string
	add := func(label, value string) {
		for i, l := range strings.Split(ansi.Wrap(value, max(width-11, 10), ""), "\n") {
			lead := strings.Repeat(" ", 10)
			if i == 0 {
				lead = MutedStyle.Render(fmt.Sprintf("%-10s", label))
			}
			lines = append(lines, lead+" "+l)
		}
	}
	for _, l := range strings.Split(ansi.Wrap(mask(e.Start.Cmd), width, ""), "\n") {
		lines = append(lines, lipgloss.NewStyle().Bold(true).Render(l))
	}
	lines = append(lines, "")
	add("Time", e.Time().Format(dayFormat+", 15:04:05"))
	if e.Start.CWD != "" {
		add("Where", mask(e.Start.CWD))
	}
	switch code, ok := e.ExitCode(); {
	case !ok:
		add("Exit", "still running, or the terminal was closed")
	case code == 0:
		add("Exit", "0"+durationText(e))
	default:
		add("Exit", fmt.Sprintf("%d (failed)%s", code, durationText(e)))
	}
	if e.Via == cmdlog.ViaArs && e.Insert != nil {
		from := "ars"
		if e.Insert.Cheat != "" {
			from += ": " + e.Insert.Cheat
		}
		if e.Insert.How != "" {
			from += " (" + e.Insert.How + ")"
		}
		add("From", from)
	} else {
		add("From", "typed")
	}
	if e.Start.Rec != "" {
		add("Recording", e.Start.Rec)
	} else {
		add("Recording", "none for this command")
	}
	if e.Start.Session != "" {
		add("Session", e.Start.Session)
	}
	if e.Start.Sh != "" {
		sh := e.Start.Sh
		if e.Start.PID != 0 {
			sh += fmt.Sprintf(" (pid %d)", e.Start.PID)
		}
		add("Shell", sh)
	}
	if e.Start.Trunc != 0 {
		add("Note", "shortened in the log (too long)")
	}
	return lines
}

func durationText(e *cmdlog.Entry) string {
	d, ok := e.Duration()
	if !ok {
		return ""
	}
	switch {
	case d < time.Second:
		return fmt.Sprintf(", %d ms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf(", %.1f s", d.Seconds())
	}
	return ", " + d.Round(time.Second).String()
}

func (m *historyModel) syncPreview() {
	sel := m.selected()
	if sel == m.shown && m.shown != nil {
		return
	}
	m.shown = sel
	if sel == nil {
		m.preview.SetContent("")
		return
	}
	lines := previewLines(sel, m.innerWidth(), m.opts.Mask)
	var matches []match
	terms := termRunes(m.terms)
	for i, l := range lines {
		matches = append(matches, columnMatches([]rune(ansi.Strip(l)), terms, i)...)
	}
	m.preview.SetContent(strings.Join(highlight(lines, matches), "\n"))
	m.preview.GotoTop()
}

func (m historyModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.refilter()
				m.fixOffset()
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			if e := m.selected(); e != nil {
				m.chosen = e
				return m, tea.Quit
			}
			return m, nil
		case "tab":
			e := m.selected()
			switch {
			case e == nil:
			case e.Start.Rec == "":
				m.notice = "no recording for this command (start one with ars rec)"
			default:
				m.chosen, m.replay = e, true
				return m, tea.Quit
			}
			return m, nil
		case "shift+tab":
			m.focus = (m.focus + 1) % 3
			return m, nil
		case "up", "down", "pgup", "pgdown":
			if m.focus == focusPreview {
				var cmd tea.Cmd
				m.preview, cmd = m.preview.Update(msg)
				return m, cmd
			}
			m.focus = focusList
			listH, _ := m.layout()
			switch msg.String() {
			case "up":
				m.moveSel(-1)
			case "down":
				m.moveSel(1)
			case "pgup":
				m.moveSel(-max(listH-1, 2))
			case "pgdown":
				m.moveSel(max(listH-1, 2))
			}
			return m, nil
		}
		m.focus = focusSearch
		m.notice = ""
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.refilter()
			m.fixOffset()
		}
		return m, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func statusText(e *cmdlog.Entry) string {
	code, ok := e.ExitCode()
	switch {
	case !ok:
		return ".."
	case code == 0:
		return "ok"
	}
	return fmt.Sprintf("!%d", code)
}

func renderHistoryRow(e *cmdlog.Entry, selected bool, width int, terms [][]rune, mask func(string) string) string {
	clock := e.Time().Format("15:04")
	status := statusText(e)
	right := clock + " " + fmt.Sprintf("%-4s", status)
	avail := width - 3
	textW := max(avail-len(right)-1, 1)
	cmdText := e.Start.Cmd
	if mask != nil {
		cmdText = mask(cmdText)
	}
	text := fit1(cmdText, textW)
	pad := strings.Repeat(" ", max(textW-ansi.StringWidth(text), 0))

	base, hl := lipgloss.NewStyle(), matchStyle
	marker, clockStyle, statusStyle := "   ", MutedStyle, MutedStyle
	if e.Failed() {
		statusStyle = ErrorStyle
	}
	if selected {
		base, marker, clockStyle, statusStyle = SelectedRowStyle, "-> ", SelectedRowStyle, SelectedRowStyle
	}
	return base.Render(marker) +
		styleRanges(text, termRanges([]rune(text), terms), base, hl) +
		base.Render(pad+" ") + clockStyle.Render(clock+" ") + statusStyle.Render(fmt.Sprintf("%-4s", status))
}

func (m historyModel) listView(listH int) string {
	if len(m.rows) == 0 {
		msg := "No match. Try fewer words or remove a !term."
		if len(m.entries) == 0 {
			msg = "No commands logged yet."
		}
		lines := []string{MutedStyle.Render(msg)}
		for len(lines) < listH {
			lines = append(lines, "")
		}
		return strings.Join(lines, "\n")
	}
	width := m.innerWidth()
	terms := termRunes(m.terms)
	var lines []string
	for i := m.offset; i < len(m.rows) && len(lines) < listH; i++ {
		r := m.rows[i]
		if r.isHeader() {
			lines = append(lines, HeadingStyle.Render(ansi.Truncate(r.day, width, "...")))
			continue
		}
		lines = append(lines, renderHistoryRow(r.entry, i == m.sel, width, terms, m.opts.Mask))
	}
	for len(lines) < listH {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m historyModel) View() string {
	if m.w == 0 {
		return ""
	}
	listH, _ := m.layout()

	left := Title("ARSENAL") + MutedStyle.Render("  history of ") + KeyStyle.Render(m.opts.Session)
	searchBox := panel{left: left, focused: m.focus == focusSearch}.render(m.input.View(), m.w)

	shown, pos := 0, 0
	for i, r := range m.rows {
		if !r.isHeader() {
			shown++
			if i == m.sel {
				pos = shown
			}
		}
	}
	listBox := panel{
		left:    Title("Commands"),
		right:   MutedStyle.Render(fmt.Sprintf("%d / %d", pos, shown)),
		focused: m.focus == focusList,
	}.render(m.listView(listH), m.w)

	info := MutedStyle.Render(shortPath(m.opts.Path))
	if m.notice != "" {
		info = WarnStyle.Render(m.notice)
	} else if m.preview.TotalLineCount() > m.preview.Height {
		info += MutedStyle.Render(fmt.Sprintf("  %d%%", int(m.preview.ScrollPercent()*100)))
	}
	previewBox := panel{left: Title("Preview"), right: info, focused: m.focus == focusPreview}.render(m.preview.View(), m.w)

	help := ansi.Truncate(m.help.View(m.keys), m.w, "...")
	return strings.Join([]string{searchBox, listBox, previewBox, help}, "\n")
}
