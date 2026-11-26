package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/ynn3m/arsenal/internal/cmdlog"
)

func intp(n int) *int { return &n }

func hist(day, h, m int, id, cmd string, exit *int, rec string) cmdlog.Entry {
	e := cmdlog.Entry{Start: cmdlog.Record{
		V: 1, Ev: cmdlog.EvStart, ID: id, TS: at(day, h, m).UnixMilli(), Session: "acme",
		CWD: "/work/acme", Cmd: cmd, Sh: "zsh", PID: 4242, Rec: rec,
	}, Via: cmdlog.ViaUser}
	if exit != nil {
		d := int64(3100)
		e.End = &cmdlog.Record{Ev: cmdlog.EvEnd, ID: id, Exit: exit, Dur: &d}
	}
	return e
}

func sampleHistory() []cmdlog.Entry {
	scan := hist(21, 13, 45, "1.2", "nmap -sV -p 22,80 10.10.10.5", intp(0), "20261121-130000-4242")
	scan.Via = cmdlog.ViaArs
	scan.Insert = &cmdlog.Record{Ev: cmdlog.EvInsert, How: cmdlog.HowPrompt, Cheat: "scan specific ports"}
	return []cmdlog.Entry{
		hist(20, 9, 15, "1.1", "ls -la", intp(0), ""),
		scan,
		hist(21, 13, 50, "1.3", "curl -k https://example.local/login", intp(7), "20261121-130000-4242"),
		hist(21, 23, 33, "1.4", "nc -lvnp 4444", nil, "20261121-130000-4242"),
	}
}

func histOpts() HistoryOptions {
	return HistoryOptions{Session: "acme", Path: "/p/acme/commands.jsonl", Entries: sampleHistory()}
}

func newHist(t *testing.T, o HistoryOptions, w, h int) tea.Model {
	t.Helper()
	var m tea.Model = newHistoryModel(o)
	return send(m, tea.WindowSizeMsg{Width: w, Height: h})
}

func TestHistoryThreeBoxesLikeTheNotesViewer(t *testing.T) {
	m := newHist(t, histOpts(), 120, 30)
	ls := lines(m)
	var tops []int
	for i, l := range ls {
		if strings.HasPrefix(l, "╭") && strings.HasSuffix(l, "╮") {
			tops = append(tops, i)
		}
		if w := ansi.StringWidth(l); w > 120 {
			t.Errorf("line wider than the screen (%d): %q", w, l)
		}
	}
	if len(tops) != 3 || len(ls) != 30 {
		t.Fatalf("want 3 boxes on 30 lines, got tops %v, %d lines:\n%s", tops, len(ls), strings.Join(ls, "\n"))
	}
	if !strings.Contains(ls[tops[0]], "ARSENAL") || !strings.Contains(ls[tops[0]], "history of acme") {
		t.Errorf("search box border = %q", ls[tops[0]])
	}
	if !strings.Contains(ls[tops[1]], "Commands") || !strings.HasSuffix(ls[tops[1]], " 1 / 4 ─╮") {
		t.Errorf("list border = %q", ls[tops[1]])
	}
	if !strings.Contains(ls[tops[2]], "Preview") || !strings.Contains(ls[tops[2]], "/p/acme/commands.jsonl") {
		t.Errorf("preview border = %q", ls[tops[2]])
	}
	help := ls[len(ls)-1]
	for _, want := range []string{"up/down select", "type filter", "!word exclude", "enter use", "tab replay", "esc clear / quit"} {
		if !strings.Contains(help, want) {
			t.Errorf("help lacks %q: %q", want, help)
		}
	}
}

func TestHistoryRowsAreGroupedByDayNewestFirstWithClockAndStatus(t *testing.T) {
	ls := lines(newHist(t, histOpts(), 100, 30))
	var rows []string
	for _, l := range ls[4:] {
		if strings.HasPrefix(l, "│") && (strings.Contains(l, "Nov") || strings.Contains(l, ":")) {
			rows = append(rows, l)
		}
	}
	joined := strings.Join(rows, "\n")
	order := []string{"Sat 21 Nov 2026", "nc -lvnp 4444", "23:33 ..", "curl -k", "13:50 !7", "nmap -sV", "13:45 ok", "Fri 20 Nov 2026", "ls -la", "09:15 ok"}
	pos := -1
	for _, s := range order {
		i := strings.Index(joined, s)
		if i < 0 || i < pos {
			t.Fatalf("%q missing or out of order in:\n%s", s, joined)
		}
		pos = i
	}
	if !strings.HasPrefix(selectedRow(m(t)), "│ -> nc -lvnp 4444") {
		t.Errorf("the newest command must be selected: %q", selectedRow(m(t)))
	}
}

func m(t *testing.T) tea.Model { return newHist(t, histOpts(), 100, 30) }

func TestHistoryPreviewDescribesTheCommand(t *testing.T) {
	var model tea.Model = m(t)
	model = send(model, press(tea.KeyDown), press(tea.KeyDown))
	text := strings.Join(lines(model), "\n")
	for _, want := range []string{
		"nmap -sV -p 22,80 10.10.10.5", "Sat 21 Nov 2026, 13:45:00", "/work/acme", "0, 3.1 s",
		"ars: scan specific ports (prompt)", "20261121-130000-4242",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("preview lacks %q:\n%s", want, text)
		}
	}
	full := strings.Join(previewLines(model.(historyModel).selected(), 90, nil), "\n")
	for _, want := range []string{"Session    acme", "zsh (pid 4242)"} {
		if !strings.Contains(full, want) {
			t.Errorf("full preview lacks %q:\n%s", want, full)
		}
	}
	model = send(model, press(tea.KeyUp))
	if t2 := strings.Join(lines(model), "\n"); !strings.Contains(t2, "7 (failed), 3.1 s") || !strings.Contains(t2, "typed") {
		t.Errorf("failed command preview:\n%s", t2)
	}
	model = send(model, press(tea.KeyUp))
	if t3 := strings.Join(lines(model), "\n"); !strings.Contains(t3, "still running") {
		t.Errorf("running command preview:\n%s", t3)
	}
	for i := 0; i < 3; i++ {
		model = send(model, press(tea.KeyDown))
	}
	if t4 := strings.Join(lines(model), "\n"); !strings.Contains(t4, "none for this command") {
		t.Errorf("command without a recording:\n%s", t4)
	}
}

func TestHistoryFilterSearchesCommandDirectoryAndCheatTitle(t *testing.T) {
	cases := []struct {
		query string
		want  []string
	}{
		{"nmap", []string{"nmap"}},
		{"example.local", []string{"curl"}},
		{"scan specific", []string{"nmap"}},
		{"/work/acme", []string{"nc", "curl", "nmap", "ls"}},
		{"!curl !nc", []string{"nmap", "ls"}},
		{"nmap !22", nil},
	}
	for _, c := range cases {
		var model tea.Model = m(t)
		model = send(model, typ(c.query))
		hm := model.(historyModel)
		var got []string
		for _, r := range hm.rows {
			if !r.isHeader() {
				got = append(got, strings.Fields(r.entry.Start.Cmd)[0])
			}
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%q: got %v, want %v", c.query, got, c.want)
		}
	}
}

func TestHistoryEnterUsesTabReplaysAndEscClearsThenQuits(t *testing.T) {
	var model tea.Model = m(t)
	model = send(model, press(tea.KeyDown))
	hm, _ := model.Update(press(tea.KeyEnter))
	if c := hm.(historyModel).chosen; c == nil || !strings.HasPrefix(c.Start.Cmd, "curl") || hm.(historyModel).replay {
		t.Errorf("enter: %+v", hm.(historyModel).chosen)
	}
	hm, cmd := model.Update(press(tea.KeyTab))
	if c := hm.(historyModel).chosen; c == nil || !hm.(historyModel).replay || cmd == nil {
		t.Errorf("tab on a command with a recording: %+v", hm.(historyModel).chosen)
	}

	model = send(m(t), typ("ls"))
	hm, cmd = model.Update(press(tea.KeyTab))
	if hm.(historyModel).chosen != nil || cmd != nil || !strings.Contains(strings.Join(lines(hm), "\n"), "no recording for this command") {
		t.Errorf("tab without a recording must show a notice and stay:\n%s", strings.Join(lines(hm), "\n"))
	}

	model = send(m(t), typ("zzzz"))
	if hm, cmd = model.Update(press(tea.KeyEnter)); hm.(historyModel).chosen != nil || cmd != nil {
		t.Error("enter without a selection chose something")
	}

	model = send(m(t), typ("nmap"))
	model, cmd = model.Update(press(tea.KeyEsc))
	if model.(historyModel).input.Value() != "" || cmd != nil || len(model.(historyModel).rows) != 6 {
		t.Errorf("first esc: %q cmd=%v rows=%d", model.(historyModel).input.Value(), cmd, len(model.(historyModel).rows))
	}
	if _, cmd = model.Update(press(tea.KeyEsc)); cmd == nil {
		t.Error("second esc must quit")
	}
}

func TestHistoryHighlightsTheSearchedWordsInTheListAndThePreview(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(old)

	model := send(m(t), typ("curl example"))
	view := model.View()
	if !strings.Contains(view, matchStyle.Render("curl")) || !strings.Contains(view, matchStyle.Render("example")) {
		t.Errorf("the searched words are not highlighted in the list")
	}
	if strings.Count(view, matchStyle.Render("curl")) < 2 {
		t.Errorf("curl highlighted %d times, want the list row and the preview", strings.Count(view, matchStyle.Render("curl")))
	}
	if got, want := ansi.Strip(view), ansi.Strip(newHist(t, HistoryOptions{Session: "acme", Path: "/p/acme/commands.jsonl", Entries: sampleHistory(), Query: "curl example"}, 100, 30).View()); got != want {
		t.Error("highlighting changed the text")
	}
	model = send(model, press(tea.KeyEsc), typ("nmap !curl"))
	if strings.Contains(model.View(), matchStyle.Render("curl")) {
		t.Error("an excluded word is highlighted")
	}
}

func TestHistoryEmptyAndChromeIsASCII(t *testing.T) {
	empty := newHist(t, HistoryOptions{Session: "acme"}, 80, 20)
	if !strings.Contains(strings.Join(lines(empty), "\n"), "No commands logged yet") {
		t.Errorf("empty history:\n%s", strings.Join(lines(empty), "\n"))
	}
	o := histOpts()
	o.Query = "nmap"
	for _, size := range [][2]int{{100, 30}, {60, 16}, {40, 12}} {
		model := newHist(t, o, size[0], size[1])
		for _, state := range []tea.Model{model, send(model, typ("zzzz")), send(model, press(tea.KeyDown))} {
			ls := lines(state)
			if bad := nonASCII(strings.Join(ls, "\n")); len(bad) > 0 {
				t.Errorf("%v: non-ASCII %q", size, string(bad))
			}
			for _, l := range ls {
				if ansi.StringWidth(l) > size[0] {
					t.Errorf("%v: line wider than the window: %q", size, l)
				}
			}
		}
	}
}

func TestHistoryListScrollsAndKeepsTheDayHeader(t *testing.T) {
	var es []cmdlog.Entry
	for i := 0; i < 60; i++ {
		es = append(es, hist(1+i/20, 8+i%20, 0, fmt.Sprintf("9.%d", i), fmt.Sprintf("echo number-%02d", i), intp(0), ""))
	}
	model := newHist(t, HistoryOptions{Session: "acme", Entries: es}, 100, 20)
	for i := 0; i < 45; i++ {
		model = send(model, press(tea.KeyDown))
	}
	hm := model.(historyModel)
	if sel := hm.selected(); sel == nil || sel.Start.Cmd != "echo number-14" {
		t.Fatalf("selected %+v after 45 steps down", hm.selected())
	}
	if !strings.Contains(selectedRow(model), "echo number-14") {
		t.Errorf("selected row not visible:\n%s", strings.Join(lines(model), "\n"))
	}
}

func TestHistoryMaskHidesSecretsInTheListPreviewAndSearchButReturnsTheRealCommand(t *testing.T) {
	es := []cmdlog.Entry{
		hist(21, 13, 45, "1.1", "mysql -u root --password hunter2 -h 10.10.10.5", intp(0), ""),
		hist(21, 13, 50, "1.2", "ls", intp(0), ""),
	}
	o := HistoryOptions{Session: "acme", Entries: es, Mask: func(s string) string { return strings.ReplaceAll(s, "hunter2", "[REDACTED]") }}
	var model tea.Model = newHist(t, o, 100, 30)
	model = send(model, press(tea.KeyDown))
	text := strings.Join(lines(model), "\n")
	if strings.Contains(text, "hunter2") || !strings.Contains(text, "--password [REDACTED]") {
		t.Errorf("secret on the screen or mask missing:\n%s", text)
	}
	found := send(newHist(t, o, 100, 30), typ("hunter2"))
	if n := len(found.(historyModel).rows); n != 0 {
		t.Errorf("searching the secret matched %d rows", n)
	}
	m2, _ := model.Update(press(tea.KeyEnter))
	if c := m2.(historyModel).chosen; c == nil || !strings.Contains(c.Start.Cmd, "hunter2") {
		t.Errorf("chosen = %+v, want the real command", c)
	}
}
