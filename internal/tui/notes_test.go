package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ynn3m/arsenal/internal/session"
)

func plainRender(md string, width int) (string, error) {
	var out []string
	for _, l := range strings.Split(md, "\n") {
		for len([]rune(l)) > width {
			out = append(out, string([]rune(l)[:width]))
			l = string([]rune(l)[width:])
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n"), nil
}

func at(day, h, m int) time.Time { return time.Date(2026, 11, day, h, m, 0, 0, time.Local) }

func sampleNotes() []session.Note {
	return []session.Note{
		{Time: at(20, 9, 15), Title: "Port scan", Body: "nmap found 22 and 80 open"},
		{Time: at(21, 13, 40), Body: "found the login page"},
		{Time: at(21, 13, 45), Title: "Weak creds", Target: "10.10.10.5", Body: "admin form accepts admin/admin"},
		{Time: at(21, 23, 33), Body: "cookie has no HttpOnly flag and the session id is predictable and long enough to not fit on one row"},
	}
}

func notesOpts() NotesOptions {
	return NotesOptions{Session: "acme", Path: "/p/acme/notes.md", Notes: sampleNotes()}
}

func newNotes(t *testing.T, o NotesOptions, w, h int) tea.Model {
	t.Helper()
	var m tea.Model = newNotesModel(o, plainRender)
	return send(m, tea.WindowSizeMsg{Width: w, Height: h})
}

func selectedRow(m tea.Model) string {
	for _, l := range lines(m) {
		if strings.HasPrefix(l, "│ -> ") {
			return l
		}
	}
	return ""
}

func TestNotesThreeBoxesLikeTheCommandSearch(t *testing.T) {
	m := newNotes(t, notesOpts(), 100, 30)
	ls := lines(m)
	var tops, bottoms []int
	for i, l := range ls {
		if strings.HasPrefix(l, "╭") && strings.HasSuffix(l, "╮") {
			tops = append(tops, i)
		}
		if strings.HasPrefix(l, "╰") && strings.HasSuffix(l, "╯") {
			bottoms = append(bottoms, i)
		}
		if w := ansi.StringWidth(l); w > 100 {
			t.Errorf("line wider than the screen (%d): %q", w, l)
		}
	}
	if len(tops) != 3 || len(bottoms) != 3 || len(ls) != 30 {
		t.Fatalf("want 3 boxes on 30 lines, got tops %v bottoms %v, %d lines:\n%s", tops, bottoms, len(ls), strings.Join(ls, "\n"))
	}
	if !strings.Contains(ls[tops[0]], "ARSENAL") || !strings.Contains(ls[tops[0]], "acme") ||
		bottoms[0]-tops[0] != 2 || !strings.Contains(ls[tops[0]+1], "> ") {
		t.Errorf("search box: %v", ls[tops[0]:bottoms[0]+1])
	}
	if !strings.Contains(ls[tops[1]], "Notes") || !strings.HasSuffix(ls[tops[1]], " 1 / 4 ─╮") {
		t.Errorf("notes top border = %q", ls[tops[1]])
	}
	if !strings.Contains(ls[tops[2]], "Preview") || !strings.Contains(ls[tops[2]], "/p/acme/notes.md") {
		t.Errorf("preview top border = %q", ls[tops[2]])
	}
	if !strings.Contains(ls[len(ls)-1], "up/down select") || !strings.Contains(ls[len(ls)-1], "ctrl+e edit") ||
		!strings.Contains(ls[len(ls)-1], "esc clear / quit") {
		t.Errorf("help bar = %q", ls[len(ls)-1])
	}
}

func TestNotesAreGroupedByDayNewestFirst(t *testing.T) {
	m := newNotes(t, notesOpts(), 100, 30)
	body := strings.Join(lines(m)[4:12], "\n")
	order := []string{"Sat 21 Nov 2026", "cookie has no HttpOnly", "Weak creds", "found the login page", "Fri 20 Nov 2026", "Port scan"}
	last := -1
	for _, want := range order {
		i := strings.Index(body, want)
		if i < 0 || i < last {
			t.Fatalf("%q missing or out of order in:\n%s", want, body)
		}
		last = i
	}
	for _, want := range []string{"23:33", "13:45", "13:40", "09:15"} {
		if !strings.Contains(body, want) {
			t.Errorf("time %s missing", want)
		}
	}
	same := []session.Note{{Time: at(21, 10, 0), Body: "first written"}, {Time: at(21, 10, 0), Body: "second written"}}
	m = newNotes(t, NotesOptions{Session: "s", Notes: same}, 100, 30)
	if !strings.Contains(selectedRow(m), "second written") {
		t.Errorf("selected = %q", selectedRow(m))
	}
}

func TestNotesRowHasTitleLeftTimeRightAndComputedTitleFits(t *testing.T) {
	m := newNotes(t, notesOpts(), 60, 30)
	var rows []string
	for _, l := range lines(m) {
		if strings.HasPrefix(l, "│ -> ") || strings.HasPrefix(l, "│    ") {
			rows = append(rows, l)
		}
	}
	got := strings.Join(rows, "\n")
	for _, l := range rows {
		if ansi.StringWidth(l) != 60 {
			t.Errorf("row width %d: %q", ansi.StringWidth(l), l)
		}
	}
	first := selectedRow(m)
	if !strings.HasPrefix(first, "│ -> cookie has no HttpOnly flag") || !strings.Contains(first, "...") || !strings.HasSuffix(first, " 23:33 │") {
		t.Errorf("computed title does not fit the row: %q", first)
	}
	if !strings.Contains(got, "Weak creds") || !strings.Contains(got, "found the login page") {
		t.Errorf("rows:\n%s", got)
	}
	if strings.Contains(got, "admin form") {
		t.Errorf("a note with a title shows its body in the row:\n%s", got)
	}
}

func TestFirstWords(t *testing.T) {
	for _, c := range []struct {
		body  string
		width int
		want  string
	}{
		{"short note", 20, "short note"},
		{"exactly ten", 11, "exactly ten"},
		{"one two three four five", 14, "one two..."},
		{"one\ntwo\n\nthree", 40, "one two three"},
		{"supercalifragilistic", 10, "superca..."},
		{"", 10, ""},
		{"a b c", 0, ""},
	} {
		got := firstWords(c.body, c.width)
		if got != c.want || ansi.StringWidth(got) > max(c.width, 0) {
			t.Errorf("firstWords(%q, %d) = %q, want %q", c.body, c.width, got, c.want)
		}
	}
}

func TestNotesDayHeadersAreNotSelectable(t *testing.T) {
	m := newNotes(t, notesOpts(), 100, 30)
	sel := func() string { return selectedRow(m) }
	want := []string{"cookie has no", "Weak creds", "found the login page", "Port scan"}
	for i, w := range want {
		if !strings.Contains(sel(), w) {
			t.Fatalf("step %d: selected %q, want %q", i, sel(), w)
		}
		m = send(m, press(tea.KeyDown))
	}
	if !strings.Contains(sel(), "Port scan") {
		t.Errorf("down past the end: %q", sel())
	}
	for i := len(want) - 2; i >= 0; i-- {
		m = send(m, press(tea.KeyUp))
		if !strings.Contains(sel(), want[i]) {
			t.Fatalf("up: selected %q, want %q", sel(), want[i])
		}
	}
	m = send(m, press(tea.KeyUp))
	if !strings.Contains(sel(), "cookie has no") {
		t.Errorf("up past the start: %q", sel())
	}
	nm := m.(notesModel)
	if nm.rows[nm.sel].isHeader() {
		t.Error("a header is selected")
	}
	m = send(m, press(tea.KeyPgDown))
	if nm = m.(notesModel); nm.rows[nm.sel].isHeader() || !strings.Contains(sel(), "Port scan") {
		t.Errorf("pgdown: %q", sel())
	}
	m = send(m, press(tea.KeyPgUp))
	if nm = m.(notesModel); nm.rows[nm.sel].isHeader() || !strings.Contains(sel(), "cookie has no") {
		t.Errorf("pgup: %q", sel())
	}
}

func TestNotesPreviewShowsDateTimeTargetAndBody(t *testing.T) {
	m := newNotes(t, notesOpts(), 100, 30)
	m = send(m, press(tea.KeyDown))
	view := strings.Join(lines(m), "\n")
	pv := view[strings.Index(view, "Preview"):]
	for _, want := range []string{"Weak creds", "Sat 21 Nov 2026, 13:45", "target: 10.10.10.5", "admin form accepts admin/admin"} {
		if !strings.Contains(pv, want) {
			t.Errorf("preview lacks %q:\n%s", want, pv)
		}
	}
	m = send(m, press(tea.KeyDown))
	view = strings.Join(lines(m), "\n")
	pv = view[strings.Index(view, "Preview"):]
	if !strings.Contains(pv, "Sat 21 Nov 2026, 13:40") || !strings.Contains(pv, "found the login page") || strings.Contains(pv, "target:") {
		t.Errorf("preview of the untitled note:\n%s", pv)
	}
}

func TestNotesFilterAndsWordsExcludesAndHidesEmptyDays(t *testing.T) {
	m := newNotes(t, notesOpts(), 100, 30)
	m = send(m, typ("admin"))
	v := strings.Join(lines(m), "\n")
	if !strings.Contains(v, "Weak creds") || strings.Contains(v, "found the login page") || strings.Contains(v, "Fri 20 Nov") {
		t.Errorf("filter admin:\n%s", v)
	}
	if !strings.Contains(v, " 1 / 1 ─╮") {
		t.Errorf("counter after filter:\n%s", v)
	}
	m = send(m, press(tea.KeyEsc), typ("weak admin"))
	if v = strings.Join(lines(m), "\n"); !strings.Contains(v, "Weak creds") || !strings.Contains(v, " 1 / 1 ─╮") {
		t.Errorf("filter 'weak admin':\n%s", v)
	}
	m = send(m, press(tea.KeyEsc), typ("weak login"))
	if v = strings.Join(lines(m), "\n"); !strings.Contains(v, "No match") {
		t.Errorf("filter 'weak login' must match nothing:\n%s", v)
	}
	m = send(m, press(tea.KeyEsc), typ("found !login"))
	if v = strings.Join(lines(m), "\n"); strings.Contains(v, "found the login page") || !strings.Contains(v, "Port scan") {
		t.Errorf("filter 'found !login':\n%s", v)
	}
	m = send(m, press(tea.KeyEsc), typ("port"))
	if v = strings.Join(lines(m), "\n"); strings.Contains(v, "Sat 21 Nov") || !strings.Contains(v, "Fri 20 Nov 2026") {
		t.Errorf("empty day not hidden:\n%s", v)
	}
	m = send(m, press(tea.KeyEsc), typ("cokie"))
	if v = strings.Join(lines(m), "\n"); !strings.Contains(v, "cookie has no") {
		t.Errorf("typo not forgiven:\n%s", v)
	}
	m = send(m, press(tea.KeyEsc), typ("port"))
	if v = strings.Join(lines(m), "\n"); !strings.Contains(v[strings.Index(v, "Preview"):], "nmap found 22 and 80 open") {
		t.Errorf("preview does not follow the filter:\n%s", v)
	}
}

func TestNotesEscClearsTheSearchThenQuits(t *testing.T) {
	m := newNotes(t, notesOpts(), 100, 30)
	m = send(m, typ("admin"))
	m, cmd := m.Update(press(tea.KeyEsc))
	if cmd != nil {
		t.Fatal("first esc must only clear the search")
	}
	if nm := m.(notesModel); nm.input.Value() != "" || len(nm.rows) != 6 {
		t.Errorf("search not cleared: %q, %d rows", nm.input.Value(), len(nm.rows))
	}
	if _, cmd = m.Update(press(tea.KeyEsc)); cmd == nil {
		t.Error("second esc must quit")
	}
	if _, cmd = m.Update(press(tea.KeyCtrlC)); cmd == nil {
		t.Error("ctrl+c must quit")
	}
	m = newNotes(t, notesOpts(), 100, 30)
	m = send(m, typ("jkq"))
	if got := m.(notesModel).input.Value(); got != "jkq" {
		t.Errorf("input = %q", got)
	}
}

func TestNotesHighlightTheSearchedWordsInTheListAndThePreview(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	m := newNotes(t, notesOpts(), 100, 30)
	m = send(m, typ("admin creds"))
	view := m.View()
	hl := matchStyle.Render("creds")
	if !strings.Contains(view, hl) {
		t.Errorf("the title word is not highlighted in the list")
	}
	if n := strings.Count(view, matchStyle.Render("admin")); n != 3 {
		t.Errorf("\"admin\" highlighted %d times, want 3 (all in the preview)", n)
	}
	if strings.Count(view, hl) < 2 {
		t.Errorf("creds highlighted %d times, want at least 2 (list and preview)", strings.Count(view, hl))
	}
	if got, want := ansi.Strip(view), ansi.Strip(newNotes(t, NotesOptions{Session: "acme", Path: "/p/acme/notes.md", Notes: sampleNotes(), Query: "admin creds"}, 100, 30).View()); got != want {
		t.Error("view text differs")
	}
	plain := lines(m)
	if !strings.Contains(strings.Join(plain, "\n"), "Weak creds") {
		t.Error("text lost")
	}
	m = send(m, press(tea.KeyEsc), typ("admin !login"))
	if strings.Contains(m.View(), matchStyle.Render("login")) {
		t.Error("an excluded word is highlighted")
	}
}

func TestHighlightHelpersKeepTextAndStyling(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	styled := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff0000")).Render("scan ports of the host")
	ls := []string{"plain scan line", styled}
	terms := termRunes([]string{"scan", "ports", "of"})
	var ms []match
	for i, l := range ls {
		ms = append(ms, columnMatches([]rune(ansi.Strip(l)), terms, i)...)
	}
	out := highlight(ls, ms)
	for i := range ls {
		if ansi.Strip(out[i]) != ansi.Strip(ls[i]) {
			t.Errorf("line %d text changed: %q -> %q", i, ansi.Strip(ls[i]), ansi.Strip(out[i]))
		}
		if out[i] == ls[i] {
			t.Errorf("line %d not highlighted", i)
		}
	}
	if r := termRanges([]rune("scan ports"), termRunes([]string{"scan", "an p", "ports"})); len(r) != 1 || r[0] != [2]int{0, 10} {
		t.Errorf("merged ranges = %v", r)
	}
	if m := columnMatches([]rune("日本語 SCAN"), termRunes([]string{"scan"}), 0); len(m) != 1 || m[0].start != 7 || m[0].end != 11 {
		t.Errorf("wide chars: %+v", m)
	}
}

func longNote() []session.Note {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "line number %d\n\n", i)
	}
	b.WriteString("the NEEDLE is here\n")
	return []session.Note{{Time: at(21, 10, 0), Title: "Long note", Body: b.String()}}
}

func TestNotesPreviewScrollsAndRevealsAMatchDeepInTheNote(t *testing.T) {
	o := NotesOptions{Session: "acme", Notes: longNote()}
	m := newNotes(t, o, 100, 24)
	if nm := m.(notesModel); nm.preview.YOffset != 0 || !strings.Contains(strings.Join(lines(m), "\n"), "line number 0") {
		t.Fatalf("starts at offset %d", nm.preview.YOffset)
	}
	m = send(m, press(tea.KeyShiftTab), press(tea.KeyShiftTab), press(tea.KeyPgDown))
	if nm := m.(notesModel); nm.focus != focusPreview || nm.preview.YOffset == 0 {
		t.Errorf("preview did not scroll: focus %v offset %d", nm.focus, nm.preview.YOffset)
	}
	m = send(m, typ("needle"))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "the NEEDLE is here") {
		t.Errorf("match not scrolled into view:\n%s", v)
	}
}

func manyNotes(n int) []session.Note {
	var out []session.Note
	for i := 0; i < n; i++ {
		out = append(out, session.Note{Time: at(1+i/4, 8+i%4, 0), Title: fmt.Sprintf("note %02d", i), Body: "text"})
	}
	return out
}

func TestNotesListScrollsAndKeepsTheDayHeaderOfTheSelection(t *testing.T) {
	m := newNotes(t, NotesOptions{Session: "acme", Notes: manyNotes(40)}, 80, 18)
	for i := 0; i < 30; i++ {
		m = send(m, press(tea.KeyDown))
		nm := m.(notesModel)
		listH, _ := nm.layout()
		if nm.sel < nm.offset || nm.sel >= nm.offset+listH {
			t.Fatalf("step %d: selection %d outside the window [%d, %d)", i, nm.sel, nm.offset, nm.offset+listH)
		}
		if !strings.Contains(selectedRow(m), "note") {
			t.Fatalf("step %d: no selected row on screen:\n%s", i, strings.Join(lines(m), "\n"))
		}
		if nm.sel > 0 && nm.rows[nm.sel-1].isHeader() && nm.offset > nm.sel-1 {
			t.Fatalf("step %d: the day header of the selection is scrolled away", i)
		}
	}
	for i := 0; i < 60; i++ {
		m = send(m, press(tea.KeyUp))
	}
	if nm := m.(notesModel); nm.sel != 1 || nm.offset != 0 {
		t.Errorf("back at the top: sel %d offset %d", nm.sel, nm.offset)
	}
}

func TestNotesChromeIsASCIIAndResizeKeepsTheSearch(t *testing.T) {
	o := notesOpts()
	o.Query = "admin"
	for _, size := range [][2]int{{100, 30}, {60, 16}, {40, 12}} {
		m := newNotes(t, o, size[0], size[1])
		ls := lines(m)
		if bad := nonASCII(strings.Join(ls, "\n")); len(bad) > 0 {
			t.Errorf("%v: non-ASCII %q in\n%s", size, string(bad), strings.Join(ls, "\n"))
		}
		for _, l := range ls {
			if ansi.StringWidth(l) > size[0] {
				t.Errorf("%v: line wider than the window: %q", size, l)
			}
		}
		m = send(m, typ("zzzz"))
		if bad := nonASCII(strings.Join(lines(m), "\n")); len(bad) > 0 {
			t.Errorf("%v no match: non-ASCII %q", size, string(bad))
		}
	}
	m := newNotes(t, o, 100, 30)
	m = send(m, tea.WindowSizeMsg{Width: 50, Height: 14})
	if nm := m.(notesModel); nm.input.Value() != "admin" || len(nm.rows) != 2 || nm.preview.Width != 46 {
		t.Errorf("resize: %q, %d rows, preview width %d", nm.input.Value(), len(nm.rows), nm.preview.Width)
	}
}

func TestNotesRenderWithGlamour(t *testing.T) {
	var m tea.Model = newNotesModel(notesOpts(), markdownRenderer(termenv.Ascii))
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 30}, press(tea.KeyDown))
	view := ansi.Strip(m.View())
	pv := view[strings.Index(view, "Preview"):]
	for _, want := range []string{"Weak creds", "Sat 21 Nov 2026, 13:45", "target: 10.10.10.5", "admin form accepts admin/admin"} {
		if !strings.Contains(pv, want) {
			t.Errorf("rendered preview lacks %q:\n%s", want, pv)
		}
	}
	if strings.Contains(view, "cannot render") || strings.Contains(pv, "**") {
		t.Errorf("Markdown not rendered:\n%s", pv)
	}
}

func TestNotesInitialQueryAndFocusCycle(t *testing.T) {
	o := notesOpts()
	o.Query = "port"
	m := newNotes(t, o, 100, 30)
	if nm := m.(notesModel); nm.input.Value() != "port" || len(nm.rows) != 2 {
		t.Fatalf("initial query: %q, %d rows", nm.input.Value(), len(nm.rows))
	}
	nm := m.(notesModel)
	seen := map[focus]bool{nm.focus: true}
	for i := 0; i < 3; i++ {
		m = send(m, press(tea.KeyShiftTab))
		seen[m.(notesModel).focus] = true
	}
	if len(seen) != 3 {
		t.Errorf("shift+tab visits %v", seen)
	}
	m = send(m, typ("x"))
	if m.(notesModel).focus != focusSearch {
		t.Error("typing must focus the search box")
	}
}

func TestPreviewMarkdown(t *testing.T) {
	n := &session.Note{Time: at(21, 13, 45), Title: "T", Target: "10.10.10.5", Body: "first line\nsecond line"}
	want := "# T\n\n**Sat 21 Nov 2026, 13:45** | target: 10.10.10.5\n\nfirst line\nsecond line\n"
	if got := previewMarkdown(n); got != want {
		t.Errorf("previewMarkdown:\n%q\nwant\n%q", got, want)
	}
	if got := previewMarkdown(&session.Note{Time: at(21, 13, 45), Body: "x"}); strings.Contains(got, "#") || strings.Contains(got, "target") {
		t.Errorf("untitled note without target: %q", got)
	}
}

func TestNotesPreviewShowsEveryLineOfTheNote(t *testing.T) {
	var m tea.Model = newNotesModel(NotesOptions{Session: "s", Notes: []session.Note{
		{Time: at(21, 10, 0), Title: "Two lines", Body: "no HttpOnly flag\nsecond line of the note"},
	}}, markdownRenderer(termenv.Ascii))
	m = send(m, tea.WindowSizeMsg{Width: 80, Height: 40})
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "no HttpOnly flag") {
		t.Fatalf("first line missing:\n%s", v)
	}
	var l1, l2 int
	for i, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "no HttpOnly flag") {
			l1 = i
		}
		if strings.Contains(l, "second line of the note") {
			l2 = i
		}
	}
	if l1 == 0 || l2 != l1+1 {
		t.Errorf("the two lines of the note must be on two rows, got rows %d and %d:\n%s", l1, l2, v)
	}
}
