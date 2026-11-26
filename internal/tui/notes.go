// The full-screen notes viewer.

package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ynn3m/arsenal/internal/search"
	"github.com/ynn3m/arsenal/internal/session"
)

type NotesOptions struct {
	Session string
	Path    string
	Notes   []session.Note
	Query   string

	Save   func(n session.Note, title, body string) (NotesResult, error)
	Delete func(n session.Note) (NotesResult, error)
}

type NotesResult struct {
	Message string
	Notes   []session.Note
	Select  int
}

func RunNotes(o NotesOptions) error {
	in, out, closeTTY := TTY()
	defer closeTTY()
	m := newNotesModel(o, markdownRenderer(termenv.NewOutput(out).ColorProfile()))
	_, err := newProgram(m, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen()).Run()
	return err
}

type renderFunc func(md string, width int) (string, error)

func markdownRenderer(p termenv.Profile) renderFunc {
	return func(md string, width int) (string, error) {
		style := "dark"
		if !lipgloss.HasDarkBackground() {
			style = "light"
		}
		r, err := glamour.NewTermRenderer(
			glamour.WithStandardStyle(style),
			glamour.WithColorProfile(p),
			glamour.WithWordWrap(width),
			glamour.WithPreservedNewLines(),
		)
		if err != nil {
			return "", err
		}
		return r.Render(md)
	}
}

type notesKeys struct {
	Select, Type, Exclude, Panel, Edit, Delete, Quit key.Binding
}

func newNotesKeys() notesKeys {
	return notesKeys{
		Select:  key.NewBinding(key.WithKeys("up", "down", "pgup", "pgdown"), key.WithHelp("up/down", "select")),
		Panel:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "panel")),
		Edit:    key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "edit")),
		Delete:  key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete")),
		Quit:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "clear / quit")),
		Type:    key.NewBinding(key.WithKeys("a"), key.WithHelp("type", "filter")),
		Exclude: key.NewBinding(key.WithKeys("!"), key.WithHelp("!word", "exclude")),
	}
}

func (k notesKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Edit, k.Delete, k.Panel, k.Quit, k.Type, k.Exclude}
}
func (k notesKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var matchStyle = lipgloss.NewStyle().
	Foreground(ColorInk).
	Background(ColorWarn)

const dayFormat = "Mon 02 Jan 2006"

type noteRow struct {
	day  string
	note *session.Note
}

func (r noteRow) isHeader() bool { return r.note == nil }

type notesModel struct {
	opts   NotesOptions
	render renderFunc

	notes []*session.Note
	idx   *search.TextIndex

	input   textinput.Model
	preview viewport.Model
	help    help.Model
	keys    notesKeys
	focus   focus
	w, h    int

	rows   []noteRow
	sel    int
	offset int
	terms  []string
	shown  *session.Note
	err    error
	notice string
	edit   *noteEdit
	del    *session.Note
}

func newNotesModel(o NotesOptions, render renderFunc) notesModel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.PromptStyle = HelpKeyStyle
	ti.Placeholder = "search the notes: words, !exclude"
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

	m := notesModel{opts: o, render: render, input: ti, preview: vp, help: hp, keys: newNotesKeys()}
	m.setNotes(o.Notes)
	m.refilter()
	return m
}

func (m *notesModel) setNotes(list []session.Note) {
	m.opts.Notes = list
	m.notes = m.notes[:0]
	for i := len(list) - 1; i >= 0; i-- {
		m.notes = append(m.notes, &list[i])
	}
	sort.SliceStable(m.notes, func(a, b int) bool { return m.notes[a].Time.After(m.notes[b].Time) })
	docs := make([][]string, len(m.notes))
	for i, n := range m.notes {
		docs[i] = []string{n.Title, n.Body}
	}
	m.idx = search.NewTextIndex(docs)
}

func (m notesModel) Init() tea.Cmd { return textinput.Blink }

func (m notesModel) innerWidth() int { return max(m.w-4, 10) }

func (m notesModel) layout() (listH, previewH int) {
	content := m.h - 3 - 2 - 2 - 1
	if m.edit != nil {
		listH = min(3, max(content-3, 1))
		previewH = max(content-listH, 1)
		return
	}
	previewH = min(max(content/3, 3), 10)
	listH = content - previewH
	if listH < 3 {
		listH = 3
		previewH = max(content-listH, 1)
	}
	return
}

func (m *notesModel) resize(w, h int) {
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

func (m *notesModel) refilter() {
	query := m.input.Value()
	ids := m.idx.Filter(query)
	sort.Ints(ids)
	m.terms = search.Terms(query)

	m.rows = m.rows[:0]
	day := ""
	for _, i := range ids {
		n := m.notes[i]
		if d := n.Time.Format("2006-01-02"); d != day {
			day = d
			m.rows = append(m.rows, noteRow{day: n.Time.Format(dayFormat)})
		}
		m.rows = append(m.rows, noteRow{note: n})
	}
	m.sel = m.nextNote(-1, 1)
	m.offset = 0
	m.shown = nil
	m.syncPreview()
}

func (m notesModel) nextNote(from, dir int) int {
	for i := from + dir; i >= 0 && i < len(m.rows); i += dir {
		if !m.rows[i].isHeader() {
			return i
		}
	}
	return -1
}

func (m notesModel) snap(i, dir int) int {
	i = min(max(i, 0), len(m.rows)-1)
	if i < 0 {
		return -1
	}
	if !m.rows[i].isHeader() {
		return i
	}
	if n := m.nextNote(i, dir); n >= 0 {
		return n
	}
	return m.nextNote(i, -dir)
}

func (m *notesModel) moveSel(delta int) {
	if m.sel < 0 {
		return
	}
	dir := 1
	if delta < 0 {
		dir = -1
	}
	var n int
	if delta == 1 || delta == -1 {
		n = m.nextNote(m.sel, dir)
	} else {
		n = m.snap(m.sel+delta, dir)
	}
	if n >= 0 {
		m.sel = n
		m.fixOffset()
		m.syncPreview()
	}
}

func (m *notesModel) fixOffset() {
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

func (m notesModel) selected() *session.Note {
	if m.sel < 0 {
		return nil
	}
	return m.rows[m.sel].note
}

func previewMarkdown(n *session.Note) string {
	var b strings.Builder
	if n.Title != "" {
		b.WriteString("# " + n.Title + "\n\n")
	}
	b.WriteString("**" + n.Time.Format(dayFormat+", 15:04") + "**")
	if n.Target != "" {
		b.WriteString(" | target: " + n.Target)
	}
	if n.Body != "" {
		b.WriteString("\n\n" + n.Body)
	}
	return b.String() + "\n"
}

func (m *notesModel) syncPreview() {
	sel := m.selected()
	if sel == m.shown && m.shown != nil {
		return
	}
	m.shown = sel
	m.err = nil
	if sel == nil {
		m.preview.SetContent("")
		return
	}
	text, err := m.render(previewMarkdown(sel), m.innerWidth())
	m.err = err
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")

	var matches []match
	terms := termRunes(m.terms)
	for i, l := range lines {
		matches = append(matches, columnMatches([]rune(ansi.Strip(l)), terms, i)...)
	}
	m.preview.SetContent(strings.Join(highlight(lines, matches), "\n"))
	m.preview.GotoTop()
	if len(matches) > 0 && matches[0].line >= m.preview.Height {
		m.preview.SetYOffset(matches[0].line - m.preview.Height/3)
	}
}

func (m notesModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		m.notice = ""
		if m.del != nil {
			return m.updateNoteDelete(msg)
		}
		if m.edit != nil {
			return m.updateNoteEdit(msg)
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "ctrl+e":
			return m.openNoteEdit()
		case "ctrl+d":
			return m.openNoteDelete()
		case "esc":
			if m.input.Value() != "" {
				m.input.SetValue("")
				m.refilter()
				m.fixOffset()
				return m, nil
			}
			return m, tea.Quit
		case "enter", "tab":
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
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.refilter()
			m.fixOffset()
		}
		return m, cmd
	}
	if m.edit != nil {
		return m.updateNoteEdit(msg)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func firstWords(body string, width int) string {
	words := strings.Fields(body)
	if len(words) == 0 || width <= 0 {
		return ""
	}
	if all := strings.Join(words, " "); ansi.StringWidth(all) <= width {
		return all
	}
	out := ""
	for _, w := range words {
		next := w
		if out != "" {
			next = out + " " + w
		}
		if ansi.StringWidth(next)+3 > width {
			break
		}
		out = next
	}
	if out == "" {
		return ansi.Truncate(words[0], width, "...")
	}
	return out + "..."
}

func rowTitle(n *session.Note, width int) string {
	if n.Title != "" {
		return fit1(n.Title, width)
	}
	if t := firstWords(n.Body, width); t != "" {
		return t
	}
	return ""
}

func fit1(s string, width int) string {
	s = strings.Join(strings.Fields(s), " ")
	if ansi.StringWidth(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "...")
}

func renderNoteRow(n *session.Note, selected bool, width int, terms [][]rune) string {
	clock := n.Time.Format("15:04")
	avail := width - 3
	titleW := max(avail-len(clock)-1, 1)
	title := rowTitle(n, titleW)
	pad := strings.Repeat(" ", max(titleW-ansi.StringWidth(title), 0))

	base, hl := lipgloss.NewStyle(), matchStyle
	marker, clockStyle := "   ", MutedStyle
	if selected {
		base, marker, clockStyle = SelectedRowStyle, "-> ", SelectedRowStyle
	}
	return base.Render(marker) +
		styleRanges(title, termRanges([]rune(title), terms), base, hl) +
		base.Render(pad+" ") + clockStyle.Render(clock)
}

func styleRanges(text string, ranges [][2]int, base, hl lipgloss.Style) string {
	r := []rune(text)
	var b strings.Builder
	pos := 0
	for _, x := range ranges {
		if x[0] > pos {
			b.WriteString(base.Render(string(r[pos:x[0]])))
		}
		b.WriteString(hl.Render(string(r[x[0]:x[1]])))
		pos = x[1]
	}
	if pos < len(r) {
		b.WriteString(base.Render(string(r[pos:])))
	}
	return b.String()
}

func (m notesModel) listView(listH int) string {
	if len(m.rows) == 0 {
		empty := Muted("No match. Try fewer words or remove a !term.")
		if len(m.notes) == 0 {
			empty = Muted("No notes yet. Run ") + Title("ars note") + Muted(" to add one.")
		}
		lines := []string{empty}
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
		lines = append(lines, renderNoteRow(r.note, i == m.sel, width, terms))
	}
	for len(lines) < listH {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m notesModel) View() string {
	if m.w == 0 {
		return ""
	}
	listH, _ := m.layout()

	left := Title("ARSENAL") + MutedStyle.Render("  notes of session ") + KeyStyle.Render(m.opts.Session)
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
		left:    Title("Notes"),
		right:   MutedStyle.Render(fmt.Sprintf("%d / %d", pos, shown)),
		focused: m.focus == focusList,
	}.render(m.listView(listH), m.w)

	info := MutedStyle.Render(shortPath(m.opts.Path))
	if m.preview.TotalLineCount() > m.preview.Height {
		info += MutedStyle.Render(fmt.Sprintf("  %d%%", int(m.preview.ScrollPercent()*100)))
	}
	pvLeft, body := Title("Preview"), m.preview.View()
	if m.err != nil {
		body = ErrorStyle.Render("cannot render the note: " + m.err.Error())
	}
	_, previewH := m.layout()
	if m.del != nil {
		pvLeft, body = Title("Delete: "+noteName(m.del)), m.noteDeleteContent(m.innerWidth())
	}
	if m.edit != nil {
		pvLeft, body = Title("Edit: "+noteName(m.edit.note)), m.edit.content(m.innerWidth(), previewH)
	}
	previewBox := panel{left: pvLeft, right: info, focused: m.focus == focusPreview}.render(body, m.w)

	var bottom string
	switch {
	case m.del != nil:
		bottom = m.help.View(newDeleteKeys())
	case m.edit != nil && m.edit.err != "":
		bottom = ErrorStyle.Render("error: " + m.edit.err)
	case m.edit != nil:
		bottom = m.help.View(newEditKeys())
	case m.notice != "":
		bottom = m.notice
	default:
		bottom = m.help.View(m.keys)
	}
	return strings.Join([]string{searchBox, listBox, previewBox, ansi.Truncate(bottom, m.w, "...")}, "\n")
}

type match struct {
	line       int
	start, end int
}

func termRunes(terms []string) [][]rune {
	var out [][]rune
	for _, t := range terms {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, []rune(strings.ToLower(t)))
		}
	}
	return out
}

func runeMatches(line, q []rune) [][2]int {
	var out [][2]int
	if len(q) == 0 {
		return nil
	}
	low := make([]rune, len(line))
	for i, r := range line {
		low[i] = unicode.ToLower(r)
	}
	for i := 0; i+len(q) <= len(low); {
		ok := true
		for j := range q {
			if low[i+j] != q[j] {
				ok = false
				break
			}
		}
		if !ok {
			i++
			continue
		}
		out = append(out, [2]int{i, i + len(q)})
		i += len(q)
	}
	return out
}

func termRanges(line []rune, terms [][]rune) [][2]int {
	var all [][2]int
	for _, q := range terms {
		all = append(all, runeMatches(line, q)...)
	}
	sort.Slice(all, func(a, b int) bool { return all[a][0] < all[b][0] })
	var out [][2]int
	for _, r := range all {
		if n := len(out); n > 0 && r[0] <= out[n-1][1] {
			out[n-1][1] = max(out[n-1][1], r[1])
			continue
		}
		out = append(out, r)
	}
	return out
}

func columnMatches(line []rune, terms [][]rune, lineNo int) []match {
	var out []match
	for _, r := range termRanges(line, terms) {
		out = append(out, match{
			line:  lineNo,
			start: ansi.StringWidth(string(line[:r[0]])),
			end:   ansi.StringWidth(string(line[:r[1]])),
		})
	}
	return out
}

func highlight(lines []string, matches []match) []string {
	out := append([]string(nil), lines...)
	byLine := map[int][]match{}
	for _, x := range matches {
		byLine[x.line] = append(byLine[x.line], x)
	}
	for ln, ms := range byLine {
		line := lines[ln]
		width := ansi.StringWidth(line)
		var b strings.Builder
		pos := 0
		for _, x := range ms {
			b.WriteString(ansi.Cut(line, pos, x.start))
			b.WriteString(matchStyle.Render(ansi.Strip(ansi.Cut(line, x.start, x.end))))
			pos = x.end
		}
		b.WriteString(ansi.Cut(line, pos, width))
		out[ln] = b.String()
	}
	return out
}
