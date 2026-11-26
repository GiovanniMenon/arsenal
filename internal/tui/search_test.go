package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/session"
)

func sampleCheats() []*cheats.Cheat {
	mk := func(title, name, command, desc string) *cheats.Cheat {
		c := &cheats.Cheat{Title: title, Name: name, Command: command, Description: desc, Filename: "/home/x/.cheats/" + title + ".md"}
		c.CommandTags.Set("plateform", "linux")
		return c
	}
	return []*cheats.Cheat{
		mk("nmap", "scan specific ports", "nmap -sV -p <ports|22,80> <target>", "Scan the given ports"),
		mk("nmap", "scan all ports", "nmap -p- <target>", ""),
		mk("curl", "download a file", "curl -O <url>", "Fetch a file"),
	}
}

func opts() SearchOptions {
	return SearchOptions{
		Cheats:   sampleCheats(),
		Session:  "acme",
		ShowTags: true,
		Vars: session.Resolved{
			Vars:   session.Vars{"target": "10.10.10.5"},
			Source: map[string]session.Source{"target": session.SourceSession},
		},
	}
}

func send(m tea.Model, msgs ...tea.Msg) tea.Model {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m
}

func typ(s string) tea.Msg        { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }
func press(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func TestSearchFlow(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	view := m.View()
	for _, want := range []string{"ARSENAL", "acme", "scan specific ports", "download a file", "1 / 3", "up/down"} {
		if !strings.Contains(ansi.Strip(view), want) {
			t.Errorf("view missing %q:\n%s", want, ansi.Strip(view))
		}
	}

	m = send(m, typ("nmap"))
	sm := m.(searchModel)
	if len(sm.results) != 2 {
		t.Fatalf("results = %d", len(sm.results))
	}
	m = send(m, press(tea.KeyDown), press(tea.KeyEnter))
	got := m.(searchModel).chosen
	if got == nil || got.Name != "scan all ports" {
		t.Fatalf("chosen = %+v", got)
	}
}

func TestSearchPreviewShowsVariables(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	v := ansi.Strip(m.View())
	for _, want := range []string{
		"nmap -sV -p 22,80 10.10.10.5",
		"target=10.10.10.5 (session)",
		"ports=22,80 (default)",
		"Scan the given ports",
		"Tags: #plateform/linux",
		"nmap - /home/x/.cheats/nmap.md",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("preview missing %q:\n%s", want, v)
		}
	}
	m = send(m, typ("download"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "url (missing)") || !strings.Contains(v, "<url>") {
		t.Errorf("missing variable not shown:\n%s", v)
	}
}

func TestSearchEscQuitsWithoutChoice(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24}, press(tea.KeyEscape))
	if m.(searchModel).chosen != nil {
		t.Error("esc must not choose")
	}
}

func TestSearchNoResultsAndEnter(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24}, typ("zzzzzz"), press(tea.KeyEnter))
	if m.(searchModel).chosen != nil {
		t.Error("enter with no results must not choose")
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "No match") || !strings.Contains(v, "0 / 0") {
		t.Errorf("view:\n%s", v)
	}
}

func TestSearchTabCompletesCommand(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24}, typ("nm"), press(tea.KeyTab))
	if got := m.(searchModel).input.Value(); got != "nmap -" {
		t.Errorf("input = %q", got)
	}
}

func TestSearchInitialQueryAndPosition(t *testing.T) {
	o := opts()
	o.Query, o.Position = "nmap", 1
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 24})
	sm := m.(searchModel)
	if sm.input.Value() != "nmap" || sm.pos() != 1 || len(sm.results) != 2 {
		t.Errorf("value=%q pos=%d results=%d", sm.input.Value(), sm.pos(), len(sm.results))
	}
}

func TestSearchPrefixIndicator(t *testing.T) {
	cases := []struct {
		on     bool
		prefix string
		want   string
	}{
		{true, "proxychains -q", "prefix on"},
		{true, "", "prefix: not set"},
		{false, "proxychains -q", "prefix off"},
	}
	for _, c := range cases {
		o := opts()
		o.PrefixOn, o.Prefix = c.on, c.prefix
		var m tea.Model = newSearchModel(o)
		m = send(m, tea.WindowSizeMsg{Width: 110, Height: 30})
		if v := ansi.Strip(m.View()); !strings.Contains(v, c.want) {
			t.Errorf("on=%v prefix=%q: missing %q in\n%s", c.on, c.prefix, c.want, v)
		}
	}
	o := opts()
	o.PrefixOn, o.Prefix = true, "proxychains -q"
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 30})
	if v := ansi.Strip(m.View()); !strings.Contains(v, "$ proxychains -q nmap -sV") {
		t.Errorf("preview should show the prefix:\n%s", v)
	}
}

func TestSearchViewFitsTheWindow(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {80, 24}, {60, 16}, {40, 12}} {
		var m tea.Model = newSearchModel(opts())
		m = send(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		lines := strings.Split(m.View(), "\n")
		if len(lines) > size[1] {
			t.Errorf("%v: %d lines", size, len(lines))
		}
		for _, l := range lines {
			if w := ansi.StringWidth(l); w > size[0] {
				t.Errorf("%v: line wider than the window (%d): %q", size, w, ansi.Strip(l))
			}
		}
	}
}

func TestCompletePath(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []string{"wordlist-a.txt", "wordlist-b.txt", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, p), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		dir + "/wor":     dir + "/wordlist-",
		dir + "/not":     dir + "/notes.txt",
		dir + "/sub":     dir + "/subdir/",
		dir + "/zzz":     "",
		"":               "",
		dir + "/notes.t": dir + "/notes.txt",
	}
	for in, want := range cases {
		if got := CompletePath(in); got != want {
			t.Errorf("CompletePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func nonASCII(s string) []rune {
	var bad []rune
	for _, r := range ansi.Strip(s) {
		if r > 127 && !(r >= 0x2500 && r <= 0x257F) {
			bad = append(bad, r)
		}
	}
	return bad
}

func TestUIIsPlainASCII(t *testing.T) {
	o := opts()
	o.PrefixOn, o.Prefix = true, "proxychains -q"
	o.Notice = Success("saved ip=10.10.10.5 in session acme")
	long := sampleCheats()
	long[0].Filename = "/home/someone/a/very/long/path/to/the/cheatsheets/folder/nmap-and-friends.md"
	long[0].Command = strings.Repeat("a-very-long-command ", 20)
	o.Cheats = long
	for _, size := range [][2]int{{110, 30}, {60, 16}, {40, 12}} {
		var m tea.Model = newSearchModel(o)
		m = send(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		if bad := nonASCII(m.View()); len(bad) > 0 {
			t.Errorf("%v: non-ASCII characters %q in\n%s", size, string(bad), ansi.Strip(m.View()))
		}
		m = send(m, typ("zzzzzz"))
		if bad := nonASCII(m.View()); len(bad) > 0 {
			t.Errorf("%v no results: non-ASCII %q", size, string(bad))
		}
	}
	for name, s := range map[string]string{
		"success": Success("x"), "error": Error("x"), "warn": Warn("x"), "info": Info("x"),
		"title": Title("x"), "muted": Muted("x"),
	} {
		if bad := nonASCII(s); len(bad) > 0 {
			t.Errorf("%s: non-ASCII %q", name, string(bad))
		}
	}
}

func TestSelectedRowUsesArrowAndPromptIsGreaterThan(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	lines := strings.Split(ansi.Strip(m.View()), "\n")
	if !strings.HasPrefix(lines[1], "│ > ") {
		t.Errorf("search prompt line = %q", lines[1])
	}
	if !strings.HasPrefix(lines[4], "│ -> ") || !strings.HasPrefix(lines[5], "│    ") {
		t.Errorf("rows = %q / %q", lines[4], lines[5])
	}
}

func lines(m tea.Model) []string { return strings.Split(ansi.Strip(m.View()), "\n") }

func TestThreeRoundedPanelsWithTitlesInTheTopBorder(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	ls := lines(m)

	var tops, bottoms []int
	for i, l := range ls {
		if strings.HasPrefix(l, "╭") && strings.HasSuffix(l, "╮") {
			tops = append(tops, i)
		}
		if strings.HasPrefix(l, "╰") && strings.HasSuffix(l, "╯") {
			bottoms = append(bottoms, i)
		}
	}
	if len(tops) != 3 || len(bottoms) != 3 {
		t.Fatalf("want 3 panels, got tops %v bottoms %v:\n%s", tops, bottoms, strings.Join(ls, "\n"))
	}
	if !strings.Contains(ls[tops[0]], "ARSENAL") || !strings.Contains(ls[tops[0]], "session acme") {
		t.Errorf("search top border = %q", ls[tops[0]])
	}
	if bottoms[0]-tops[0] != 2 || !strings.Contains(ls[tops[0]+1], "> ") {
		t.Errorf("search box should be 3 lines with the input inside: %v", ls[tops[0]:bottoms[0]+1])
	}
	if !strings.Contains(ls[tops[1]], "Commands") || !strings.HasSuffix(ls[tops[1]], " 1 / 3 ─╮") {
		t.Errorf("commands top border = %q", ls[tops[1]])
	}
	if !strings.Contains(ls[tops[2]], "Preview") {
		t.Errorf("preview top border = %q", ls[tops[2]])
	}
	if !strings.Contains(strings.Join(ls[tops[2]:bottoms[2]], "\n"), "nmap -sV -p 22,80 10.10.10.5") {
		t.Error("preview content missing")
	}
	last := ls[len(ls)-1]
	if !strings.Contains(last, "enter choose") || !strings.Contains(last, "shift+tab panel") {
		t.Errorf("help bar = %q", last)
	}
	m = send(m, press(tea.KeyDown))
	if !strings.Contains(strings.Join(lines(m), "\n"), " 2 / 3 ─╮") {
		t.Error("counter did not move to 2 / 3")
	}
}

func TestPanelsFillTheWindowExactlyAndResize(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	for _, size := range [][2]int{{120, 40}, {100, 30}, {80, 24}, {61, 17}, {80, 50}, {100, 30}} {
		m = send(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		ls := lines(m)
		if len(ls) != size[1] {
			t.Errorf("%v: %d lines, want %d", size, len(ls), size[1])
		}
		for i, l := range ls[:len(ls)-1] {
			if w := ansi.StringWidth(l); w != size[0] {
				t.Errorf("%v line %d is %d wide: %q", size, i, w, l)
			}
		}
		if w := ansi.StringWidth(ls[len(ls)-1]); w > size[0] {
			t.Errorf("%v help bar too wide (%d)", size, w)
		}
	}
}

func TestFocusFollowsTheUserAndShiftTabCycles(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	f := func() focus { return m.(searchModel).focus }

	if f() != focusSearch {
		t.Fatalf("start focus = %v", f())
	}
	m = send(m, press(tea.KeyDown))
	if f() != focusList {
		t.Errorf("after down: focus = %v, want list", f())
	}
	m = send(m, typ("n"))
	if f() != focusSearch {
		t.Errorf("typing must focus the search box, got %v", f())
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyShiftTab}, tea.KeyMsg{Type: tea.KeyShiftTab})
	if f() != focusPreview {
		t.Errorf("two shift+tab: focus = %v, want preview", f())
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if f() != focusSearch {
		t.Errorf("shift+tab wraps around, got %v", f())
	}
	m = send(m, tea.KeyMsg{Type: tea.KeyShiftTab}, typ("j"))
	if got := m.(searchModel).input.Value(); !strings.HasSuffix(got, "j") {
		t.Errorf("typed letter lost: %q", got)
	}
}

func TestOnlyTheFocusedPanelHasTheAccentBorder(t *testing.T) {
	focused, dim := panel{focused: true}, panel{focused: false}
	if focused.color() != ColorPrimary || dim.color() != ColorMuted {
		t.Errorf("colors: focused %v, dim %v", focused.color(), dim.color())
	}
	render := func(f bool) string { return ansi.Strip(panel{left: "x", right: "y", focused: f}.render("c", 20)) }
	if render(true) != render(false) {
		t.Error("focus must not change the text")
	}
	oldProfile := lipgloss.ColorProfile()
	oldDark := lipgloss.HasDarkBackground()
	lipgloss.SetColorProfile(termenv.TrueColor)
	lipgloss.SetHasDarkBackground(true)
	defer func() { lipgloss.SetColorProfile(oldProfile); lipgloss.SetHasDarkBackground(oldDark) }()

	view := func(f focus) []string {
		var m tea.Model = newSearchModel(opts())
		m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
		sm := m.(searchModel)
		sm.focus = f
		return strings.Split(sm.View(), "\n")
	}
	search, list, preview := view(focusSearch), view(focusList), view(focusPreview)
	var tops []int
	for i, l := range lines(newSearchModelSized(100, 30)) {
		if strings.HasPrefix(l, "╭") {
			tops = append(tops, i)
		}
	}
	differs := func(a, b []string, panel int) bool { return a[tops[panel]] != b[tops[panel]] }
	if !differs(search, list, 0) || !differs(search, list, 1) || differs(search, list, 2) {
		t.Error("search -> list focus must recolor the search and commands borders only")
	}
	if !differs(list, preview, 1) || !differs(list, preview, 2) || differs(list, preview, 0) {
		t.Error("list -> preview focus must recolor the commands and preview borders only")
	}
}

func newSearchModelSized(w, h int) tea.Model {
	return send(newSearchModel(opts()), tea.WindowSizeMsg{Width: w, Height: h})
}

func TestPreviewScrollsWhenFocused(t *testing.T) {
	o := opts()
	long := sampleCheats()
	long[0].Description = strings.Repeat("a long line of description. ", 120)
	o.Cheats = long
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	sm := m.(searchModel)
	if sm.preview.TotalLineCount() <= sm.preview.Height {
		t.Fatalf("test needs a tall preview: %d lines in %d", sm.preview.TotalLineCount(), sm.preview.Height)
	}
	if !strings.Contains(strings.Join(lines(m), "\n"), " 0% ") {
		t.Error("scroll position missing from the preview border")
	}

	m = send(m, press(tea.KeyDown))
	if sm = m.(searchModel); sm.pos() != 1 || sm.preview.YOffset != 0 {
		t.Errorf("pos %d yoffset %d", sm.pos(), sm.preview.YOffset)
	}
	m = send(m, press(tea.KeyUp))

	m = send(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	if m.(searchModel).focus != focusPreview {
		t.Fatalf("focus = %v", m.(searchModel).focus)
	}
	m = send(m, press(tea.KeyDown), press(tea.KeyDown), press(tea.KeyPgDown))
	sm = m.(searchModel)
	if sm.preview.YOffset == 0 {
		t.Error("preview did not scroll")
	}
	if sm.pos() != 0 {
		t.Errorf("the selection must not move while the preview is focused (pos %d)", sm.pos())
	}
	m = send(m, typ("download"))
	if sm = m.(searchModel); sm.preview.YOffset != 0 {
		t.Errorf("yoffset after a new search = %d", sm.preview.YOffset)
	}
}

func TestLettersNeverMoveTheListOrPreview(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30})
	m = send(m, typ("j"), typ("k"), typ("d"))
	sm := m.(searchModel)
	if sm.input.Value() != "jkd" || sm.pos() != 0 {
		t.Errorf("input %q pos %d", sm.input.Value(), sm.pos())
	}
}
