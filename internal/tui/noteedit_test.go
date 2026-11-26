package tui

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/ynn3m/arsenal/internal/session"
)

type noteCalls struct {
	saved   []session.Note
	titles  []string
	bodies  []string
	deleted []session.Note
}

func editableNotes(c *noteCalls) NotesOptions {
	o := notesOpts()
	for i := range o.Notes {
		o.Notes[i].Index = i
	}
	o.Save = func(n session.Note, title, body string) (NotesResult, error) {
		c.saved, c.titles, c.bodies = append(c.saved, n), append(c.titles, title), append(c.bodies, body)
		list := append([]session.Note(nil), o.Notes...)
		list[n.Index].Title, list[n.Index].Body = title, body
		return NotesResult{Message: "Note saved", Notes: list, Select: n.Index}, nil
	}
	o.Delete = func(n session.Note) (NotesResult, error) {
		c.deleted = append(c.deleted, n)
		var list []session.Note
		for _, x := range o.Notes {
			if x.Index != n.Index {
				x.Index = len(list)
				list = append(list, x)
			}
		}
		return NotesResult{Message: "Note deleted", Notes: list, Select: -1}, nil
	}
	return o
}

func view(m tea.Model) string { return ansi.Strip(m.View()) }

func TestNotesWithoutNotesShowAnEmptyList(t *testing.T) {
	var c noteCalls
	o := editableNotes(&c)
	o.Notes = nil
	m := newNotes(t, o, 100, 30)
	if v := view(m); !strings.Contains(v, "No notes yet. Run ars note to add one.") || !strings.Contains(v, "0 / 0") {
		t.Fatalf("view:\n%s", v)
	}
	m = send(m, ctrl(tea.KeyCtrlE), ctrl(tea.KeyCtrlD))
	if nm := m.(notesModel); nm.edit != nil || nm.del != nil {
		t.Error("edit or delete opened without a note")
	}
}

func TestCtrlEEditsTheSelectedNoteInThePreviewBox(t *testing.T) {
	var c noteCalls
	m := newNotes(t, editableNotes(&c), 100, 30)
	m = send(m, tea.KeyMsg{Type: tea.KeyDown}, ctrl(tea.KeyCtrlE))
	nm := m.(notesModel)
	if nm.edit == nil || nm.edit.note.Index != 2 || nm.edit.title.Value() != "Weak creds" || nm.edit.body.Value() != "admin form accepts admin/admin" {
		t.Fatalf("edit = %+v", nm.edit)
	}
	v := view(m)
	for _, want := range []string{"Edit: Weak creds", "> Title", "Text", "ctrl+s save", "esc cancel"} {
		if !strings.Contains(v, want) {
			t.Errorf("%q missing:\n%s", want, v)
		}
	}
	for _, l := range lines(m) {
		if w := ansi.StringWidth(l); w > 100 {
			t.Errorf("line wider than the screen (%d): %q", w, l)
		}
	}

	m = send(m, typ(" again"), tea.KeyMsg{Type: tea.KeyEnter}, typ("!"), ctrl(tea.KeyCtrlS))
	if len(c.saved) != 1 || c.saved[0].Index != 2 || c.titles[0] != "Weak creds again" || c.bodies[0] != "admin form accepts admin/admin!" {
		t.Fatalf("save calls = %+v %q %q", c.saved, c.titles, c.bodies)
	}
	nm = m.(notesModel)
	if nm.edit != nil || nm.selected() == nil || nm.selected().Title != "Weak creds again" {
		t.Fatalf("after save: edit %v selected %+v", nm.edit, nm.selected())
	}
	if v := view(m); !strings.Contains(v, "Note saved") || !strings.Contains(v, "Weak creds again") {
		t.Errorf("view:\n%s", v)
	}
}

func TestNoteEditEscCancelsAndEmptyNotesAreRefused(t *testing.T) {
	var c noteCalls
	m := newNotes(t, editableNotes(&c), 100, 30)
	m = send(m, ctrl(tea.KeyCtrlE), typ("x"), tea.KeyMsg{Type: tea.KeyEsc})
	if nm := m.(notesModel); nm.edit != nil || len(c.saved) != 0 {
		t.Fatalf("esc: edit %v saves %d", nm.edit, len(c.saved))
	}
	if v := view(m); !strings.Contains(v, "Preview") || strings.Contains(v, "Edit:") {
		t.Errorf("view:\n%s", v)
	}

	m = send(m, ctrl(tea.KeyCtrlE), ctrl(tea.KeyCtrlU), tea.KeyMsg{Type: tea.KeyTab})
	nm := m.(notesModel)
	nm.edit.body.SetValue("   ")
	m = send(nm, ctrl(tea.KeyCtrlS))
	if nm := m.(notesModel); nm.edit == nil || len(c.saved) != 0 || !strings.Contains(view(m), "error: the note is empty") {
		t.Errorf("empty note saved or form closed:\n%s", view(m))
	}
}

func TestNoteSaveErrorStaysInTheForm(t *testing.T) {
	var c noteCalls
	o := editableNotes(&c)
	o.Save = func(session.Note, string, string) (NotesResult, error) {
		return NotesResult{}, errors.New("the note changed in notes.md since it was opened: open ars notes again")
	}
	m := send(newNotes(t, o, 120, 30), ctrl(tea.KeyCtrlE), ctrl(tea.KeyCtrlS))
	if nm := m.(notesModel); nm.edit == nil || !strings.Contains(view(m), "error: the note changed") {
		t.Errorf("view:\n%s", view(m))
	}
}

func TestCtrlDAsksBeforeDeletingANote(t *testing.T) {
	var c noteCalls
	m := newNotes(t, editableNotes(&c), 100, 30)
	m = send(m, ctrl(tea.KeyCtrlD))
	if v := view(m); !strings.Contains(v, "Delete this note from /p/acme/notes.md?") || !strings.Contains(v, "y delete") {
		t.Fatalf("view:\n%s", v)
	}
	m = send(m, typ("n"))
	if nm := m.(notesModel); nm.del != nil || len(c.deleted) != 0 {
		t.Fatal("n deleted the note")
	}
	m = send(m, ctrl(tea.KeyCtrlD), typ("y"))
	nm := m.(notesModel)
	if len(c.deleted) != 1 || c.deleted[0].Index != 3 || len(nm.notes) != 3 || nm.selected() == nil {
		t.Fatalf("deleted %+v, %d notes left, selected %v", c.deleted, len(nm.notes), nm.selected())
	}
	if v := view(m); !strings.Contains(v, "Note deleted") || !strings.Contains(v, "1 / 3") || strings.Contains(v, "cookie has no HttpOnly") {
		t.Errorf("view:\n%s", v)
	}
}

func TestNotesWithoutSaveOrDeleteSaySo(t *testing.T) {
	m := send(newNotes(t, notesOpts(), 100, 30), ctrl(tea.KeyCtrlE))
	if !strings.Contains(view(m), "Editing is not available here") {
		t.Errorf("view:\n%s", view(m))
	}
	m = send(m, ctrl(tea.KeyCtrlD))
	if !strings.Contains(view(m), "Deleting is not available here") {
		t.Errorf("view:\n%s", view(m))
	}
}
