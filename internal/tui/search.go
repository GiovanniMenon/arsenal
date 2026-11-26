// The full-screen command search.

package tui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/search"
	"github.com/ynn3m/arsenal/internal/session"
)

type SearchOptions struct {
	Cheats   []*cheats.Cheat
	Vars     session.Resolved
	Session  string
	Query    string
	Position int
	ShowTags bool
	Prefix   string
	PrefixOn bool
	Notice   string

	Save     func(c *cheats.Cheat, e cheats.Entry) (SaveResult, error)
	ReadOnly func(c *cheats.Cheat) bool
	Delete   func(c *cheats.Cheat) (SaveResult, error)
}

type SearchResult struct {
	Cheat    *cheats.Cheat
	Query    string
	Position int
	Values   map[string]string
}

func RunSearch(o SearchOptions) (SearchResult, error) {
	m := newSearchModel(o)
	in, out, closeTTY := TTY()
	defer closeTTY()
	final, err := newProgram(m, tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen()).Run()
	if err != nil {
		return SearchResult{}, err
	}
	fm := final.(searchModel)
	return SearchResult{Cheat: fm.chosen, Query: fm.input.Value(), Position: fm.pos(), Values: fm.values}, nil
}

type focus int

const (
	focusSearch focus = iota
	focusList
	focusPreview
)

type item struct{ c *cheats.Cheat }

func (i item) FilterValue() string { return i.c.Name }

type rowDelegate struct{ stats columnStats }

func (d rowDelegate) Height() int                         { return 1 }
func (d rowDelegate) Spacing() int                        { return 0 }
func (d rowDelegate) Update(tea.Msg, *list.Model) tea.Cmd { return nil }
func (d rowDelegate) Render(w io.Writer, m list.Model, index int, it list.Item) {
	fmt.Fprint(w, renderRow(it.(item).c, index == m.Index(), m.Width(), d.stats))
}

type searchKeys struct {
	Select, Choose, Complete, Panel, Edit, Delete, Quit, Exclude key.Binding
}

func newSearchKeys() searchKeys {
	return searchKeys{
		Select:   key.NewBinding(key.WithKeys("up", "down", "pgup", "pgdown"), key.WithHelp("up/down", "select")),
		Choose:   key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "choose")),
		Complete: key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "complete")),
		Panel:    key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "panel")),
		Edit:     key.NewBinding(key.WithKeys("ctrl+e"), key.WithHelp("ctrl+e", "edit")),
		Delete:   key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "delete")),
		Quit:     key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "quit")),
		Exclude:  key.NewBinding(key.WithKeys("!"), key.WithHelp("!word", "exclude")),
	}
}

func (k searchKeys) ShortHelp() []key.Binding {
	return []key.Binding{k.Select, k.Choose, k.Complete, k.Panel, k.Edit, k.Delete, k.Quit, k.Exclude}
}
func (k searchKeys) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

type searchModel struct {
	opts    SearchOptions
	idx     *search.Index
	input   textinput.Model
	list    list.Model
	preview viewport.Model
	help    help.Model
	keys    searchKeys
	results []*cheats.Cheat
	focus   focus
	w, h    int
	chosen  *cheats.Cheat
	notice  string
	shown   *cheats.Cheat
	fill    *fillState
	edit    *editState
	del     *cheats.Cheat
	stats   columnStats
	values  map[string]string
}

func newSearchModel(o SearchOptions) searchModel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.PromptStyle = HelpKeyStyle
	ti.Placeholder = "search: words, #tag, !exclude"
	ti.CharLimit = 200
	ti.SetValue(o.Query)
	ti.CursorEnd()
	ti.Focus()

	stats := measure(o.Cheats, o.ShowTags)
	l := list.New(nil, rowDelegate{stats}, 0, 0)
	l.SetShowTitle(false)
	l.SetShowFilter(false)
	l.SetShowStatusBar(false)
	l.SetShowPagination(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)
	l.DisableQuitKeybindings()

	km := list.DefaultKeyMap()
	km.CursorUp = key.NewBinding(key.WithKeys("up"))
	km.CursorDown = key.NewBinding(key.WithKeys("down"))
	km.PrevPage = key.NewBinding(key.WithKeys("pgup"))
	km.NextPage = key.NewBinding(key.WithKeys("pgdown"))
	for _, b := range []*key.Binding{&km.GoToStart, &km.GoToEnd, &km.Filter, &km.ClearFilter,
		&km.CancelWhileFiltering, &km.AcceptWhileFiltering, &km.ShowFullHelp, &km.CloseFullHelp} {
		b.SetEnabled(false)
	}
	l.KeyMap = km

	vp := viewport.New(0, 0)
	vp.KeyMap = viewport.KeyMap{
		Up:       key.NewBinding(key.WithKeys("up")),
		Down:     key.NewBinding(key.WithKeys("down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup")),
		PageDown: key.NewBinding(key.WithKeys("pgdown")),
	}

	hp := help.New()
	hp.ShortSeparator = "  |  "
	hp.Ellipsis = "..."
	hp.Styles.ShortKey = HelpKeyStyle
	hp.Styles.ShortDesc = MutedStyle
	hp.Styles.ShortSeparator = MutedStyle
	hp.Styles.Ellipsis = MutedStyle

	m := searchModel{
		opts: o, idx: search.NewIndex(o.Cheats), input: ti, list: l, preview: vp,
		help: hp, keys: newSearchKeys(), notice: o.Notice, stats: stats,
	}
	m.setResults(m.idx.Filter(ti.Value()))
	if len(m.results) > 0 {
		m.list.Select(min(max(o.Position, 0), len(m.results)-1))
	}
	m.syncPreview()
	return m
}

func (m searchModel) Init() tea.Cmd { return textinput.Blink }

func (m searchModel) pos() int { return m.list.Index() }

func (m searchModel) selected() *cheats.Cheat {
	if len(m.results) == 0 {
		return nil
	}
	return m.results[m.pos()]
}

func (m *searchModel) setResults(r []*cheats.Cheat) {
	m.results = r
	items := make([]list.Item, len(r))
	for i, c := range r {
		items[i] = item{c}
	}
	m.list.SetItems(items)
}

func (m *searchModel) setCheats(list []*cheats.Cheat) {
	m.idx = search.NewIndex(list)
	m.stats = measure(list, m.opts.ShowTags)
	m.list.SetDelegate(rowDelegate{m.stats})
}

func (m searchModel) layout() (listH, previewH int) {
	content := m.h - 3 - 2 - 2 - 1
	if m.fill != nil || m.edit != nil {
		var want int
		if m.edit != nil {
			want = m.edit.height()
		} else {
			want = m.fill.height(m.fillPrefix(), m.innerWidth())
		}
		previewH = min(max(want, 3), max(content-3, 3))
		listH = max(content-previewH, 1)
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

func (m searchModel) innerWidth() int { return max(m.w-4, 10) }

func (m *searchModel) resize(w, h int) {
	m.w, m.h = w, h
	m.applyLayout()
	m.shown = nil
	m.syncPreview()
}

func (m searchModel) fillPrefix() string {
	if m.fill != nil && m.opts.PrefixOn {
		return m.opts.Prefix
	}
	return ""
}

func (m *searchModel) applyLayout() {
	inner := m.innerWidth()
	listH, previewH := m.layout()
	m.input.Width = max(inner-3, 5)
	m.list.SetSize(inner, listH)
	m.preview.Width, m.preview.Height = inner, previewH
	m.help.Width = m.w
}

func (m *searchModel) syncPreview() {
	sel := m.selected()
	if sel == m.shown && m.shown != nil {
		return
	}
	m.shown = sel
	m.preview.SetContent(m.previewText(max(m.preview.Width, 10)))
	m.preview.GotoTop()
}

func (m searchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil

	case tea.KeyMsg:
		m.notice = ""
		if m.del != nil {
			return m.updateDelete(msg)
		}
		if m.edit != nil {
			return m.updateEdit(msg)
		}
		if m.fill != nil {
			return m.updateFill(msg)
		}
		switch msg.String() {
		case "ctrl+c", "esc", "f10":
			return m, tea.Quit
		case "ctrl+e":
			return m.openEdit()
		case "ctrl+d":
			return m.openDelete()
		case "enter":
			if c := m.selected(); c != nil {
				f := newFill(c, m.opts.Vars.Vars, func(name string) string { return string(m.opts.Vars.Source[name]) })
				if len(f.cmd.NeedsInput()) > 0 {
					m.fill = f
					m.focus = focusPreview
					m.input.Blur()
					m.applyLayout()
					return m, textinput.Blink
				}
				m.chosen = c
				return m, tea.Quit
			}
			return m, nil
		case "up", "down", "pgup", "pgdown":
			var cmd tea.Cmd
			if m.focus == focusPreview {
				m.preview, cmd = m.preview.Update(msg)
				return m, cmd
			}
			m.focus = focusList
			m.list, cmd = m.list.Update(msg)
			m.syncPreview()
			return m, cmd
		case "shift+tab":
			m.focus = (m.focus + 1) % 3
			return m, nil
		case "tab":
			m.focus = focusSearch
			if v := m.input.Value(); v != "" {
				if c := m.idx.CommonPrefix(v); c != v {
					m.input.SetValue(c)
					m.input.CursorEnd()
					m.refilter()
				}
			}
			return m, nil
		}
		m.focus = focusSearch
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.refilter()
		}
		return m, cmd
	}
	var cmd tea.Cmd
	if m.edit != nil {
		return m.updateEdit(msg)
	}
	if m.fill != nil {
		return m.updateFill(msg)
	}
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *searchModel) refilter() {
	m.setResults(m.idx.Filter(m.input.Value()))
	m.list.Select(0)
	m.syncPreview()
}

func (m searchModel) searchLabels() (left, right string) {
	left = Title("ARSENAL")
	if m.opts.Session != "" {
		left += MutedStyle.Render("  session ") + KeyStyle.Render(m.opts.Session)
	} else {
		left += MutedStyle.Render("  no session (global variables only)")
	}
	switch {
	case m.opts.PrefixOn && m.opts.Prefix != "":
		right = ChipOnStyle.Render("prefix on") + " " + ValueStyle.Render(m.opts.Prefix)
	case m.opts.PrefixOn:
		right = ChipWarnStyle.Render("prefix: not set (ars set -g arsenal_prefix_cmd=...)")
	case m.opts.Prefix != "":
		right = ChipOffStyle.Render("prefix off (run ars -f to use: " + m.opts.Prefix + ")")
	}
	return
}

func (m searchModel) emptyText() string {
	if len(m.opts.Cheats) == 0 {
		return Muted("No commands yet. Run ") + Title("ars add") + Muted(" to save one, or ") +
			Title("ars setup") + Muted(" to add the Arsenal commands.")
	}
	return Muted("No match. Try fewer words or remove a !term.")
}

func shortPath(file string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(file, home) {
		return "~" + strings.TrimPrefix(file, home)
	}
	return file
}

func (m searchModel) previewText(width int) string {
	c := m.selected()
	if c == nil && len(m.opts.Cheats) == 0 {
		return ""
	}
	if c == nil {
		return m.emptyText()
	}
	cmd := cheats.NewCommand(c, m.opts.Vars.Vars)
	wrap := lipgloss.NewStyle().Width(width)

	var out []string
	out = append(out, wrap.Render(lipgloss.NewStyle().Bold(true).Render(strings.TrimSpace(c.Name))))
	source := shortPath(c.Filename)
	if t := c.Tool(); t != "" {
		source = t + " - " + source
	}
	out = append(out, wrap.Render(MutedStyle.Render(source)))

	var cb strings.Builder
	if m.opts.PrefixOn && m.opts.Prefix != "" {
		cb.WriteString(ActiveStyle.Render(m.opts.Prefix) + " ")
	}
	for _, s := range cmd.Segments() {
		if s.Arg < 0 {
			cb.WriteString(ValueStyle.Render(s.Text))
			continue
		}
		a := cmd.Args[s.Arg]
		switch {
		case a.Source == cheats.SourceSaved:
			cb.WriteString(ActiveStyle.Render(a.Value))
		case a.Value != "":
			cb.WriteString(KeyStyle.Render(a.Value))
		default:
			cb.WriteString(WarnStyle.Underline(true).Render("<" + a.Name + ">"))
		}
	}
	out = append(out, wrap.Render(MutedStyle.Render("$ ")+cb.String()))

	if len(cmd.Args) > 0 {
		var vs []string
		for _, a := range cmd.Args {
			switch a.Source {
			case cheats.SourceSaved:
				vs = append(vs, ActiveStyle.Render(a.Name)+MutedStyle.Render("="+a.Value+" ("+string(m.opts.Vars.Source[a.Name])+")"))
			case cheats.SourceDefault, cheats.SourceFile:
				vs = append(vs, KeyStyle.Render(a.Name)+MutedStyle.Render("="+a.Value+" (default)"))
			default:
				vs = append(vs, WarnStyle.Render(a.Name)+MutedStyle.Render(" (missing)"))
			}
		}
		out = append(out, wrap.Render(strings.Join(vs, MutedStyle.Render("  |  "))))
	}

	if tags := c.TagList(); len(tags) > 0 {
		out = append(out, wrap.Render(MutedStyle.Render("Tags: ")+lipgloss.NewStyle().Foreground(ColorPrimary).Render("#"+strings.Join(tags, " #"))))
	}

	if d := strings.TrimSpace(c.Description); d != "" {
		out = append(out, "", wrap.Render(d))
	}
	return strings.Join(out, "\n")
}

func (m searchModel) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	listH, previewH := m.layout()

	left, right := m.searchLabels()
	searchBox := panel{left: left, right: right, focused: m.focus == focusSearch}.
		render(m.input.View(), m.w)

	var rows string
	if len(m.results) == 0 {
		lines := []string{m.emptyText()}
		for len(lines) < listH {
			lines = append(lines, "")
		}
		rows = strings.Join(lines, "\n")
	} else {
		rows = m.list.View()
	}
	counter := fmt.Sprintf("%d / %d", min(m.pos()+1, len(m.results)), len(m.results))
	cmdBox := panel{left: Title("Commands"), right: MutedStyle.Render(counter), focused: m.focus == focusList}.
		render(rows, m.w)

	var info string
	if m.preview.TotalLineCount() > previewH {
		info = MutedStyle.Render(fmt.Sprintf("%d%%", int(m.preview.ScrollPercent()*100)))
	}
	pvLeft, pvContent := Title("Preview"), m.preview.View()
	if m.fill != nil {
		pvLeft, info = m.fillLabels()
		pvContent = m.fill.content(m.fillPrefix(), m.innerWidth(), previewH)
	}
	if m.del != nil {
		pvLeft = Title("Delete: " + strings.TrimSpace(m.del.Name))
		info = MutedStyle.Render(shortPath(m.del.Filename))
		pvContent = m.deleteContent(m.innerWidth())
	}
	if m.edit != nil {
		pvLeft = Title("Edit: " + strings.TrimSpace(m.edit.cheat.Name))
		info = MutedStyle.Render(shortPath(m.edit.cheat.Filename))
		pvContent = m.edit.content(m.innerWidth(), previewH)
	}
	pvBox := panel{left: pvLeft, right: info, focused: m.focus == focusPreview}.
		render(pvContent, m.w)

	var bottom string
	if m.del != nil {
		bottom = ansi.Truncate(m.help.View(newDeleteKeys()), m.w, "...")
	} else if m.edit != nil {
		if m.edit.err != "" {
			bottom = ErrorStyle.Render("error: " + m.edit.err)
		} else {
			bottom = ansi.Truncate(m.help.View(newEditKeys()), m.w, "...")
		}
	} else if m.fill != nil {
		if m.fill.err != "" {
			bottom = ErrorStyle.Render("error: " + m.fill.err)
		} else {
			bottom = ansi.Truncate(m.help.View(newFillKeys()), m.w, "...")
		}
	} else if m.notice != "" {
		bottom = ansi.Truncate(m.notice, m.w, "...")
	} else {
		bottom = ansi.Truncate(m.help.View(m.keys), m.w, "...")
	}
	return strings.Join([]string{searchBox, cmdBox, pvBox, bottom}, "\n")
}
