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
)

func openFill(t *testing.T, name string, w, h int) tea.Model {
	t.Helper()
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: w, Height: h})
	m = send(m, typ(name), press(tea.KeyEnter))
	return m
}

func plain(m tea.Model) string { return ansi.Strip(m.View()) }

func TestFillShowsTheFinalCommandAndOneRowPerVariable(t *testing.T) {
	m := openFill(t, "scan specific", 100, 30)
	sm := m.(searchModel)
	if sm.fill == nil || sm.chosen != nil {
		t.Fatalf("the fill mode did not open: fill=%v chosen=%v", sm.fill, sm.chosen)
	}
	v := plain(m)
	for _, want := range []string{
		"$ nmap -sV -p 22,80 10.10.10.5",
		"> ports", "(default)",
		"target", "(session)",
		"2 variables",
		"tab/shift+tab", "enter", "esc",
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	if strings.Index(v, "$ nmap") > strings.Index(v, "> ports") {
		t.Errorf("the command must be at the top of the box:\n%s", v)
	}
	if sm.fill.focus != 0 || sm.fill.cmd.Args[0].Name != "ports" {
		t.Errorf("focus = %d", sm.fill.focus)
	}
}

func TestFillCommandUpdatesLiveWhileTyping(t *testing.T) {
	m := openFill(t, "scan specific", 100, 30)
	m = send(m, press(tea.KeyCtrlU), typ("443"))
	v := plain(m)
	if !strings.Contains(v, "$ nmap -sV -p 443 10.10.10.5") {
		t.Errorf("command not updated live:\n%s", v)
	}
	if !strings.Contains(v, "(typed)") {
		t.Errorf("a changed value must be marked typed:\n%s", v)
	}
	if got := m.(searchModel).input.Value(); got != "scan specific" {
		t.Errorf("search box changed to %q", got)
	}
}

func TestFillMissingVariablesAreHighlightedAndBlockInsert(t *testing.T) {
	m := openFill(t, "download", 100, 30)
	v := plain(m)
	if !strings.Contains(v, "$ curl -O <url>") || !strings.Contains(v, "(missing)") {
		t.Errorf("missing variable not shown:\n%s", v)
	}
	m = send(m, press(tea.KeyEnter))
	sm := m.(searchModel)
	if sm.chosen != nil || sm.fill == nil || sm.fill.err != "missing value for url" {
		t.Fatalf("chosen=%v err=%q", sm.chosen, sm.fill.err)
	}
	if !strings.Contains(plain(m), "error: missing value for url") {
		t.Errorf("error not shown:\n%s", plain(m))
	}
	m = send(m, typ("http://example.local/a.zip"))
	if !strings.Contains(plain(m), "$ curl -O http://example.local/a.zip") {
		t.Errorf("command not updated:\n%s", plain(m))
	}
	m = send(m, press(tea.KeyEnter))
	sm = m.(searchModel)
	if sm.chosen == nil || sm.chosen.Name != "download a file" || sm.values["url"] != "http://example.local/a.zip" {
		t.Fatalf("not inserted: chosen=%v values=%v", sm.chosen, sm.values)
	}
}

func TestFillMissingValuesAreStyledDifferently(t *testing.T) {
	forceColors(t)
	m := openFill(t, "download", 100, 30)
	missing := m.View()
	m = send(m, typ("x"))
	if filled := m.View(); filled == missing {
		t.Error("filled and missing rows look the same")
	}
	if !strings.Contains(missing, "\x1b[") {
		t.Error("no styling at all")
	}
}

func TestFillTabAndShiftTabMoveBetweenVariables(t *testing.T) {
	m := openFill(t, "scan specific", 100, 30)
	m = send(m, press(tea.KeyTab))
	if f := m.(searchModel).fill; f.focus != 1 || !strings.Contains(plain(m), "> target") {
		t.Errorf("tab: focus = %d\n%s", f.focus, plain(m))
	}
	m = send(m, press(tea.KeyTab))
	if m.(searchModel).fill.focus != 0 {
		t.Errorf("wrap: focus = %d", m.(searchModel).fill.focus)
	}
	m = send(m, press(tea.KeyShiftTab))
	if m.(searchModel).fill.focus != 1 {
		t.Errorf("shift+tab: focus = %d", m.(searchModel).fill.focus)
	}
	m = send(m, press(tea.KeyCtrlU), typ("10.10.10.9"), press(tea.KeyEnter))
	sm := m.(searchModel)
	if sm.values["target"] != "10.10.10.9" || sm.values["ports"] != "22,80" {
		t.Errorf("values = %v", sm.values)
	}
}

func TestFillEnterInsertsAndEscGoesBackToTheList(t *testing.T) {
	m := openFill(t, "scan specific", 100, 30)
	m = send(m, press(tea.KeyEsc))
	sm := m.(searchModel)
	if sm.fill != nil || sm.chosen != nil {
		t.Fatalf("esc: fill=%v chosen=%v", sm.fill, sm.chosen)
	}
	if sm.input.Value() != "scan specific" || sm.selected() == nil || sm.selected().Name != "scan specific ports" {
		t.Errorf("list not restored: %q %v", sm.input.Value(), sm.selected())
	}
	if !strings.Contains(plain(m), "Preview") || strings.Contains(plain(m), "variables") {
		t.Errorf("preview box not restored:\n%s", plain(m))
	}
	m = send(m, press(tea.KeyEnter), press(tea.KeyEnter))
	sm = m.(searchModel)
	if sm.chosen == nil || sm.values["ports"] != "22,80" || sm.values["target"] != "10.10.10.5" {
		t.Errorf("chosen=%v values=%v", sm.chosen, sm.values)
	}
}

func TestFillSkippedWhenEveryVariableIsSaved(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30}, typ("all ports"), press(tea.KeyEnter))
	sm := m.(searchModel)
	if sm.fill != nil || sm.chosen == nil || sm.values != nil {
		t.Errorf("a fully resolved command is inserted right away: fill=%v chosen=%v values=%v", sm.fill, sm.chosen, sm.values)
	}
}

func TestFillTabCompletesAPathElseMoves(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "wordlist.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	m := openFill(t, "download", 100, 30)
	m = send(m, typ("word"), press(tea.KeyTab))
	sm := m.(searchModel)
	if got := sm.fill.value(0); got != "wordlist.txt" || sm.fill.focus != 0 {
		t.Errorf("tab did not complete the path: value %q focus %d", got, sm.fill.focus)
	}
	m = send(m, press(tea.KeyTab))
	if m.(searchModel).fill.focus != 0 {
		t.Errorf("focus = %d", m.(searchModel).fill.focus)
	}
}

func TestFillPrefixIsShownInTheCommand(t *testing.T) {
	o := opts()
	o.PrefixOn, o.Prefix = true, "proxychains -q"
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30}, typ("scan specific"), press(tea.KeyEnter))
	if !strings.Contains(plain(m), "$ proxychains -q nmap -sV -p 22,80 10.10.10.5") {
		t.Errorf("prefix missing:\n%s", plain(m))
	}
}

func TestFillFitsTheTerminalAndIsASCII(t *testing.T) {
	many := &cheats.Cheat{Title: "x", Name: "many variables", Filename: "/h/x.md",
		Command: "tool <aa> <bb> <cc> <dd> <ee> <ff> <gg> <hh> --long-option-with-a-long-name <ii>"}
	for _, size := range [][2]int{{120, 40}, {80, 24}, {50, 16}, {30, 12}} {
		o := opts()
		o.Cheats = append(sampleCheats(), many)
		var m tea.Model = newSearchModel(o)
		m = send(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}, typ("many"), press(tea.KeyEnter))
		if m.(searchModel).fill == nil {
			t.Fatalf("%v: fill did not open", size)
		}
		for i := 0; i < 10; i++ {
			m = send(m, typ("v"), press(tea.KeyTab))
			v := m.View()
			lines := strings.Split(v, "\n")
			if len(lines) > size[1] {
				t.Errorf("%v: %d lines", size, len(lines))
			}
			for _, l := range lines {
				if ansi.StringWidth(l) > size[0] {
					t.Errorf("%v: line wider than the terminal: %q", size, ansi.Strip(l))
				}
			}
			for _, r := range nonASCII(v) {
				if r < 0x2500 || r > 0x257f {
					t.Errorf("%v: non-ASCII %q", size, r)
				}
			}
		}
	}
}

func forceColors(t *testing.T) {
	t.Helper()
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })
}

func TestFillNotOpenedForACommandWithoutVariables(t *testing.T) {
	plainCheat := &cheats.Cheat{Title: "demo", Name: "say hello", Command: "echo hello", Filename: "/h/demo.md"}
	o := opts()
	o.Cheats = append(sampleCheats(), plainCheat)
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30}, typ("say hello"), press(tea.KeyEnter))
	sm := m.(searchModel)
	if sm.fill != nil || sm.chosen != plainCheat || sm.values != nil {
		t.Errorf("fill=%v chosen=%v values=%v", sm.fill, sm.chosen, sm.values)
	}
}
