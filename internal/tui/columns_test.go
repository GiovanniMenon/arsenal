package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ynn3m/arsenal/internal/cheats"
)

func richCheats() []*cheats.Cheat {
	mk := func(tool, name, command, desc, tags string) *cheats.Cheat {
		c := &cheats.Cheat{Title: tool, Name: name, Command: command, Description: desc, Tags: tags, Filename: "/home/x/.cheats/" + tool + ".md"}
		c.CommandTags.Set("cat", "RECON/SCAN")
		return c
	}
	return []*cheats.Cheat{
		mk("impacket", "psexec shell", "psexec.py <domain>/<user>:<password>@<target> -hashes <hashes> cmd.exe", "Get a remote shell\nsecond line", "smb, windows, lateral, extra"),
		mk("nmap", "scan ports", "nmap -sV -p <ports|22,80> <target>", "Scan the given ports", "recon"),
	}
}

func rowText(c *cheats.Cheat, selected bool, width int, st columnStats) string {
	return ansi.Strip(renderRow(c, selected, width, st))
}

func TestRowHasToolTitleTagsCommandInOrder(t *testing.T) {
	list := richCheats()
	st := measure(list, true)
	row := rowText(list[0], false, 200, st)
	order := []string{"impacket", "psexec shell", "smb, windows, lateral", "psexec.py"}
	last := -1
	for _, want := range order {
		i := strings.Index(row, want)
		if i <= last {
			t.Fatalf("%q %q", want, row)
		}
		last = i
	}
	if strings.Contains(row, "extra") || strings.Contains(row, "cat/RECON") || strings.Contains(row, "remote shell") {
		t.Errorf("row = %q", row)
	}
	if !strings.HasPrefix(rowText(list[0], true, 200, st), "-> impacket") {
		t.Errorf("selected row = %q", rowText(list[0], true, 200, st))
	}
}

func TestRowsFitTheirWidthAtEverySize(t *testing.T) {
	list := richCheats()
	st := measure(list, true)
	for width := 12; width <= 220; width += 7 {
		for i, c := range list {
			for _, sel := range []bool{false, true} {
				if got := ansi.StringWidth(renderRow(c, sel, width, st)); got > width {
					t.Fatalf("%d %d %v %d", width, i, sel, got)
				}
			}
		}
	}
}

func TestColumnsShrinkTagsBeforeTheCommand(t *testing.T) {
	list := richCheats()
	st := measure(list, true)
	wide := st.layout(200)
	if wide.tool == 0 || wide.title == 0 || wide.tags == 0 || wide.cmd == 0 {
		t.Fatalf("wide layout: %+v", wide)
	}
	if wide.tool < len("impacket") || wide.title < len("psexec shell") {
		t.Errorf("%+v", wide)
	}
	if used := wide.tool + wide.title + wide.tags + wide.cmd + 3*colGap; used != 200 {
		t.Errorf("%d %+v", used, wide)
	}

	tagsGone := 0
	for avail := 200; avail >= 30; avail-- {
		l := st.layout(avail)
		if l.tags == 0 && tagsGone == 0 {
			tagsGone = avail
		}
		if l.cmd == 0 || l.tool == 0 || l.title == 0 {
			t.Fatalf("avail %d: %+v", avail, l)
		}
		if used := l.tool + l.title + l.tags + l.cmd; used > avail {
			t.Errorf("%d %d %+v", avail, used, l)
		}
	}
	if tagsGone == 0 {
		t.Error(tagsGone)
	}
}

func TestNoEmptyColumns(t *testing.T) {
	list := richCheats()
	for _, c := range list {
		c.Tags = ""
		c.CommandTags = nil
	}
	if l := measure(list, true).layout(200); l.tags != 0 {
		t.Errorf("%+v", l)
	}
	if l := measure(richCheats(), false).layout(200); l.tags != 0 {
		t.Errorf("%+v", l)
	}
}

func TestLongCellsEndInDots(t *testing.T) {
	list := richCheats()
	st := measure(list, true)
	row := rowText(list[0], false, 90, st)
	if !strings.Contains(row, "...") {
		t.Errorf("%q", row)
	}
	if got := cell("abcdefghij", 6); got != "abc..." {
		t.Errorf("cell = %q", got)
	}
	if got := cell("abc", 6); got != "abc   " {
		t.Errorf("cell = %q", got)
	}
}

func TestSearchListShowsTheNewColumns(t *testing.T) {
	o := opts()
	o.Cheats = richCheats()
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 120, Height: 30})
	v := ansi.Strip(m.View())
	for _, want := range []string{"-> impacket  psexec shell", "nmap", "scan ports", "smb, windows, lateral", "search: words, #tag, !exclude"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	o.ShowTags = false
	m = send(newSearchModel(o), tea.WindowSizeMsg{Width: 120, Height: 30})
	if v := ansi.Strip(m.View()); strings.Contains(v, "smb, windows") {
		t.Errorf("%s", v)
	}
}

func TestPreviewLayout(t *testing.T) {
	o := opts()
	o.Cheats = richCheats()
	o.Cheats[1].Description = "Scan the given ports\nand more"
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 40}, press(tea.KeyDown))
	ls := strings.Split(ansi.Strip(m.View()), "\n")
	var body []string
	in := false
	for _, l := range ls {
		if strings.HasPrefix(l, "╭─ Preview") {
			in = true
			continue
		}
		if in && strings.HasPrefix(l, "╰") {
			break
		}
		if in {
			body = append(body, strings.TrimSpace(strings.Trim(l, "│")))
		}
	}
	want := []string{
		"scan ports",
		"nmap - /home/x/.cheats/nmap.md",
		"$ nmap -sV -p 22,80 10.10.10.5",
		"ports=22,80 (default)  |  target=10.10.10.5 (session)",
		"Tags: #recon #cat/RECON/SCAN",
		"",
		"Scan the given ports",
		"and more",
	}
	if len(body) < len(want) {
		t.Fatalf("%d %q", len(body), body)
	}
	for i, w := range want {
		if body[i] != w {
			t.Errorf("%d %q %q", i, body[i], w)
		}
	}
}

func TestHashTermFiltersByTagInTheSearchBox(t *testing.T) {
	o := opts()
	o.Cheats = richCheats()
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 120, Height: 30}, typ("#smb"))
	if got := m.(searchModel).results; len(got) != 1 || got[0].Name != "psexec shell" {
		t.Errorf("#smb: %v", got)
	}
	m = send(m, press(tea.KeyBackspace), press(tea.KeyBackspace), press(tea.KeyBackspace), press(tea.KeyBackspace), typ("!#smb"))
	if got := m.(searchModel).results; len(got) != 1 || got[0].Name != "scan ports" {
		t.Errorf("!#smb: %v", got)
	}
}

func TestLongTitlesAreShownWhole(t *testing.T) {
	list := richCheats()
	list[1].Name = "scan common ports and detect the service versions"
	st := measure(list, true)
	l := st.layout(130)
	if l.title != len(list[1].Name) {
		t.Fatalf("%d %d %+v", l.title, len(list[1].Name), l)
	}
	row := rowText(list[1], false, 133, st)
	if !strings.Contains(row, list[1].Name) {
		t.Errorf("title cut: %q", row)
	}
	if l := st.layout(100); l.title != len(list[1].Name) || l.tags != 0 {
		t.Errorf("layout(100) = %+v", l)
	}
}

func TestSourceTagIsInThePreviewAndTheFilterButNotInTheTagsColumn(t *testing.T) {
	o := opts()
	o.Cheats = richCheats()
	o.Cheats[0].CommandTags.Set("src", "arsenal")
	if got := rowTags(o.Cheats[0]); strings.Contains(got, "src") {
		t.Errorf("%q", got)
	}
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 40})
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "#src/arsenal") {
		t.Errorf("%s", v)
	}
	if list := ansi.Strip(strings.Split(v, "Preview")[0]); strings.Contains(list, "src/arsenal") {
		t.Errorf("%s", list)
	}
	m = send(m, typ("#src"))
	if got := m.(searchModel).results; len(got) != 1 || got[0].Name != "psexec shell" {
		t.Errorf("%v", got)
	}
	m = send(m, press(tea.KeyBackspace), press(tea.KeyBackspace), press(tea.KeyBackspace), press(tea.KeyBackspace), typ("!#src/arsenal"))
	if got := m.(searchModel).results; len(got) != 1 || got[0].Name != "scan ports" {
		t.Errorf("%v", got)
	}
}
