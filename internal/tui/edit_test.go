package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ynn3m/arsenal/internal/cheats"
)

type saveCall struct {
	cheat *cheats.Cheat
	entry cheats.Entry
}

func editOpts(calls *[]saveCall, readOnly bool) SearchOptions {
	o := opts()
	o.Cheats = sampleCheats()
	o.Cheats[0].Tags = "recon, tcp"
	o.Cheats[0].Description = "Scan the given ports\nsecond line"
	o.ReadOnly = func(*cheats.Cheat) bool { return readOnly }
	o.Save = func(c *cheats.Cheat, e cheats.Entry) (SaveResult, error) {
		*calls = append(*calls, saveCall{c, e})
		saved := &cheats.Cheat{Title: "nmap", Name: strings.TrimSpace(e.Title), Command: e.Command, Description: e.Description, Tags: e.Tags, Filename: c.Filename}
		list := []*cheats.Cheat{saved}
		for _, other := range o.Cheats {
			if other != c {
				list = append(list, other)
			}
		}
		return SaveResult{Message: "Saved in nmap.md", Cheats: list, Saved: saved}, nil
	}
	return o
}

func ctrl(t tea.KeyType) tea.Msg { return tea.KeyMsg{Type: t} }

func TestCtrlEOpensTheEditFormInThePreviewBox(t *testing.T) {
	var calls []saveCall
	var m tea.Model = newSearchModel(editOpts(&calls, false))
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ctrl(tea.KeyCtrlE))
	ls := lines(m)
	text := strings.Join(ls, "\n")

	var top string
	for _, l := range ls {
		if strings.HasPrefix(l, "╭─ Edit: ") {
			top = l
		}
	}
	if !strings.Contains(top, "Edit: scan specific ports") || !strings.Contains(top, "/home/x/.cheats/nmap.md") {
		t.Fatalf("%q %s", top, text)
	}
	if strings.Contains(text, "Preview") {
		t.Errorf("%s", text)
	}
	for _, want := range []string{
		"> Command      nmap -sV -p <ports|22,80> <target>",
		"  Title        scan specific ports",
		"  Tags         recon, tcp, plateform/linux",
		"  Description  Scan the given ports",
		"               second line",
		"tab/shift+tab next/previous field",
		"ctrl+s save",
		"esc cancel",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%q %s", want, text)
		}
	}
	if len(calls) != 0 {
		t.Error(calls)
	}
}

func TestEditRefusesCommandsOfAReadOnlyFolder(t *testing.T) {
	var calls []saveCall
	var m tea.Model = newSearchModel(editOpts(&calls, true))
	m = send(m, tea.WindowSizeMsg{Width: 130, Height: 34}, ctrl(tea.KeyCtrlE))
	if sm := m.(searchModel); sm.edit != nil || !strings.Contains(ansi.Strip(sm.View()), "read-only folder: /home/x/.cheats/nmap.md") {
		t.Errorf("%s", ansi.Strip(m.View()))
	}
}

func TestEditFormLeavesOutTheArsenalTagSoSavingMakesTheCommandYours(t *testing.T) {
	var calls []saveCall
	o := editOpts(&calls, false)
	o.Cheats[0].CommandTags.Set("src", "arsenal")
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 130, Height: 34}, ctrl(tea.KeyCtrlE))
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Tags         recon, tcp, plateform/linux ") || strings.Contains(v, "src/arsenal") {
		t.Errorf("%s", v)
	}
	m = send(m, ctrl(tea.KeyCtrlS))
	if len(calls) != 1 || strings.Contains(calls[0].entry.Tags, "src") {
		t.Errorf("%+v", calls)
	}
}

func TestEditTabMovesBetweenFieldsAndTypingEditsInPlace(t *testing.T) {
	var calls []saveCall
	var m tea.Model = newSearchModel(editOpts(&calls, false))
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ctrl(tea.KeyCtrlE))
	if f := m.(searchModel).edit.focus; f != fieldCommand {
		t.Fatalf("focus = %d", f)
	}
	m = send(m, typ(" --open"), ctrl(tea.KeyTab), typ("!"))
	e := m.(searchModel).edit
	if e.focus != fieldTitle {
		t.Errorf("%d", e.focus)
	}
	if e.inputs[fieldCommand].Value() != "nmap -sV -p <ports|22,80> <target> --open" || e.inputs[fieldTitle].Value() != "scan specific ports!" {
		t.Errorf("values: %q %q", e.inputs[fieldCommand].Value(), e.inputs[fieldTitle].Value())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "> Title        scan specific ports!") || !strings.Contains(v, "  Command      nmap -sV") {
		t.Errorf("view = \n%s", v)
	}
	m = send(m, ctrl(tea.KeyShiftTab), ctrl(tea.KeyShiftTab))
	if f := m.(searchModel).edit.focus; f != fieldDescription {
		t.Errorf("%d", f)
	}
	m = send(m, ctrl(tea.KeyEnter), typ("third"))
	if d := m.(searchModel).edit.desc.Value(); !strings.HasSuffix(d, "third") || strings.Count(d, "\n") < 2 {
		t.Errorf("description = %q", d)
	}
}

func TestEditEscCancelsWithoutChanges(t *testing.T) {
	var calls []saveCall
	var m tea.Model = newSearchModel(editOpts(&calls, false))
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ctrl(tea.KeyCtrlE), typ("zzz"), ctrl(tea.KeyEscape))
	sm := m.(searchModel)
	if sm.edit != nil || sm.chosen != nil || len(calls) != 0 {
		t.Fatalf("%v %v %d", sm.edit, sm.chosen, len(calls))
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Preview") || strings.Contains(v, "zzz") || !strings.Contains(v, "ctrl+e edit") {
		t.Errorf("%s", v)
	}
	if sm.focus != focusList {
		t.Errorf("focus = %v", sm.focus)
	}
	m = send(m, ctrl(tea.KeyEscape))
	if _, cmd := m.Update(ctrl(tea.KeyEscape)); cmd == nil {
		t.Error(cmd)
	}
}

func TestEditSaveCallsSaveShowsTheNoticeAndTheUpdatedList(t *testing.T) {
	var calls []saveCall
	var m tea.Model = newSearchModel(editOpts(&calls, false))
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ctrl(tea.KeyCtrlE), ctrl(tea.KeyTab), typ(" v2"), ctrl(tea.KeyCtrlS))
	if len(calls) != 1 {
		t.Fatalf("saves = %d", len(calls))
	}
	got := calls[0]
	if got.cheat.Name != "scan specific ports" || got.entry.Title != "scan specific ports v2" ||
		got.entry.Command != "nmap -sV -p <ports|22,80> <target>" || got.entry.Tags != "recon, tcp, plateform/linux" ||
		got.entry.Description != "Scan the given ports\nsecond line" {
		t.Errorf("saved %+v", got.entry)
	}
	sm := m.(searchModel)
	if sm.edit != nil || sm.focus != focusList {
		t.Fatalf("%v %v", sm.edit, sm.focus)
	}
	v := ansi.Strip(m.View())
	for _, want := range []string{"Saved in nmap.md", "scan specific ports v2", "Preview"} {
		if !strings.Contains(v, want) {
			t.Errorf("view missing %q:\n%s", want, v)
		}
	}
	if sel := sm.selected(); sel == nil || sel.Name != "scan specific ports v2" {
		t.Errorf("selected = %v", sel)
	}
	if strings.Contains(v, "ctrl+e edit") {
		t.Error(v)
	}
	m = send(m, press(tea.KeyDown))
	if v := ansi.Strip(m.View()); strings.Contains(v, "Saved in nmap.md") || !strings.Contains(v, "ctrl+e edit") {
		t.Errorf("%s", v)
	}
}

func TestEditRefusesWhatCannotBeSavedAndStaysInTheForm(t *testing.T) {
	var calls []saveCall
	o := editOpts(&calls, false)
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 34}, ctrl(tea.KeyCtrlE))

	for range "nmap -sV -p <ports|22,80> <target>" {
		m = send(m, press(tea.KeyBackspace))
	}
	m = send(m, ctrl(tea.KeyCtrlS))
	if len(calls) != 0 || m.(searchModel).edit == nil {
		t.Fatalf("%d", len(calls))
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "error: a command is required") {
		t.Errorf("error missing:\n%s", v)
	}

	o.Save = func(*cheats.Cheat, cheats.Entry) (SaveResult, error) {
		return SaveResult{}, errors.New("an entry with this title already exists: choose another title")
	}
	m = send(newSearchModel(o), tea.WindowSizeMsg{Width: 110, Height: 34}, ctrl(tea.KeyCtrlE), typ("X"), ctrl(tea.KeyCtrlS))
	sm := m.(searchModel)
	if sm.edit == nil {
		t.Fatal(sm.edit)
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "error: an entry with this title already exists") || !strings.Contains(v, "<target>X") {
		t.Errorf("error view:\n%s", v)
	}
	m = send(m, typ("y"))
	if v := ansi.Strip(m.View()); strings.Contains(v, "error:") {
		t.Error(v)
	}
}

func TestEditWithoutSaveSaysSo(t *testing.T) {
	var m tea.Model = newSearchModel(opts())
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30}, ctrl(tea.KeyCtrlE))
	if sm := m.(searchModel); sm.edit != nil || !strings.Contains(ansi.Strip(sm.View()), "Editing is not available") {
		t.Errorf("view:\n%s", ansi.Strip(m.View()))
	}
}

func TestCtrlEWithNothingSelectedDoesNothing(t *testing.T) {
	var calls []saveCall
	var m tea.Model = newSearchModel(editOpts(&calls, false))
	m = send(m, tea.WindowSizeMsg{Width: 100, Height: 30}, typ("zzzzqqqq"), ctrl(tea.KeyCtrlE))
	if m.(searchModel).edit != nil {
		t.Error(m.(searchModel).edit)
	}
}

func TestEditViewFitsTheWindow(t *testing.T) {
	var calls []saveCall
	for _, readOnly := range []bool{false, true} {
		for _, size := range [][2]int{{100, 30}, {80, 24}, {60, 18}, {120, 50}} {
			var m tea.Model = newSearchModel(editOpts(&calls, readOnly))
			m = send(m, tea.WindowSizeMsg{Width: size[0], Height: size[1]}, ctrl(tea.KeyCtrlE))
			ls := lines(m)
			if len(ls) != size[1] {
				t.Errorf("%v %v %d %d", size, readOnly, len(ls), size[1])
			}
			for _, l := range ls {
				if w := ansi.StringWidth(l); w > size[0] {
					t.Errorf("%v %d %q", size, w, l)
				}
			}
		}
	}
}

func TestEditCommandKeepsMultiLineCommandsUntilChanged(t *testing.T) {
	c := &cheats.Cheat{Name: "two steps", Command: "echo one;\\\necho two", Filename: "/x/my.md"}
	e := newEdit(c)
	if got := e.inputs[fieldCommand].Value(); got != "echo one;echo two" {
		t.Errorf("command field = %q", got)
	}
	if got := e.entry().Command; got != "echo one;\necho two" {
		t.Errorf("unchanged command = %q", got)
	}
	e.inputs[fieldCommand].SetValue("echo one")
	if got := e.entry().Command; got != "echo one" {
		t.Errorf("changed command = %q", got)
	}
}

type deleteCalls struct{ deleted []string }

func deleteOpts(d *deleteCalls, readOnly bool) SearchOptions {
	o := opts()
	o.Cheats = sampleCheats()
	o.ReadOnly = func(*cheats.Cheat) bool { return readOnly }
	o.Delete = func(c *cheats.Cheat) (SaveResult, error) {
		d.deleted = append(d.deleted, c.Name)
		var list []*cheats.Cheat
		for _, other := range o.Cheats {
			if other != c {
				list = append(list, other)
			}
		}
		return SaveResult{Message: "Deleted " + c.Name + " from nmap.md", Cheats: list}, nil
	}
	return o
}

func TestCtrlDAsksBeforeDeleting(t *testing.T) {
	var d deleteCalls
	var m tea.Model = newSearchModel(deleteOpts(&d, false))
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 30}, press(tea.KeyDown), ctrl(tea.KeyCtrlD))
	v := ansi.Strip(m.View())
	for _, want := range []string{"Delete: scan all ports", "Delete this command from nmap.md?", "$ nmap -p- <target>", "y delete", "any other key cancel", "/home/x/.cheats/nmap.md"} {
		if !strings.Contains(v, want) {
			t.Errorf("confirmation missing %q:\n%s", want, v)
		}
	}
	if len(d.deleted) != 0 {
		t.Fatal(d.deleted)
	}
	for _, k := range []tea.Msg{typ("n"), ctrl(tea.KeyEnter), ctrl(tea.KeyEscape)} {
		m = send(m, k, ctrl(tea.KeyCtrlD))
		m = send(m, k)
		if sm := m.(searchModel); sm.del != nil || len(d.deleted) != 0 || len(sm.results) != 3 {
			t.Fatalf("%v: del=%v deleted=%v", k, sm.del, d.deleted)
		}
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Preview") || !strings.Contains(v, "ctrl+d delete") {
		t.Errorf("%s", v)
	}
}

func TestDeleteWithYRemovesTheCommandAndKeepsThePosition(t *testing.T) {
	var d deleteCalls
	var m tea.Model = newSearchModel(deleteOpts(&d, false))
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 30}, press(tea.KeyDown), press(tea.KeyDown), ctrl(tea.KeyCtrlD), typ("y"))
	sm := m.(searchModel)
	if len(d.deleted) != 1 || d.deleted[0] != "download a file" {
		t.Fatalf("deleted %v", d.deleted)
	}
	if len(sm.results) != 2 || sm.del != nil || sm.pos() != 1 || sm.selected().Name != "scan all ports" {
		t.Errorf("%d %v %d", len(sm.results), sm.del, sm.pos())
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "Deleted download a file from nmap.md") || strings.Contains(v, "download a file  ") {
		t.Errorf("view:\n%s", v)
	}
}

func TestDeleteRefusesReadOnlyAndUnavailable(t *testing.T) {
	var d deleteCalls
	var m tea.Model = newSearchModel(deleteOpts(&d, true))
	m = send(m, tea.WindowSizeMsg{Width: 140, Height: 30}, ctrl(tea.KeyCtrlD))
	if sm := m.(searchModel); sm.del != nil || !strings.Contains(ansi.Strip(sm.View()), "read-only folder") {
		t.Errorf("view:\n%s", ansi.Strip(m.View()))
	}
	m = send(newSearchModel(opts()), tea.WindowSizeMsg{Width: 100, Height: 30}, ctrl(tea.KeyCtrlD))
	if sm := m.(searchModel); sm.del != nil || !strings.Contains(ansi.Strip(sm.View()), "Deleting is not available") {
		t.Errorf("view:\n%s", ansi.Strip(m.View()))
	}
}

func TestDeleteTheLastCommandLeavesAnEmptyList(t *testing.T) {
	var d deleteCalls
	o := deleteOpts(&d, false)
	o.Cheats = o.Cheats[:1]
	o.Delete = func(*cheats.Cheat) (SaveResult, error) { return SaveResult{Message: "Deleted", Cheats: nil}, nil }
	var m tea.Model = newSearchModel(o)
	m = send(m, tea.WindowSizeMsg{Width: 110, Height: 30}, ctrl(tea.KeyCtrlD), typ("y"))
	if sm := m.(searchModel); len(sm.results) != 0 || sm.selected() != nil {
		t.Errorf("results = %d", len(sm.results))
	}
	if v := ansi.Strip(m.View()); strings.Count(v, "No commands yet. Run ars add to save one, or ars setup to add the Arsenal commands.") != 1 {
		t.Errorf("view:\n%s", v)
	}
}
