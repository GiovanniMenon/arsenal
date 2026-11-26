package session

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var noteTime = time.Date(2026, 11, 21, 13, 40, 59, 0, time.Local)

func TestNoteHeader(t *testing.T) {
	for _, c := range []struct{ title, target, want string }{
		{"", "", "## 2026-11-21 13:40"},
		{"", "10.10.10.5", "## 2026-11-21 13:40 | target: 10.10.10.5"},
		{"Login page", "", "## 2026-11-21 13:40 | Login page"},
		{"Login page", "10.10.10.5", "## 2026-11-21 13:40 | Login page | target: 10.10.10.5"},
		{"  a\n b ", "  x\n y", "## 2026-11-21 13:40 | a b | target: x y"},
		{"a | b", "", "## 2026-11-21 13:40 | a / b"},
		{"target: oops", "", "## 2026-11-21 13:40 | target - oops"},
	} {
		if got := NoteHeader(noteTime, c.title, c.target); got != c.want {
			t.Errorf("NoteHeader(%q, %q) = %q, want %q", c.title, c.target, got, c.want)
		}
	}
}

func TestParseNotesRoundTripAndCompatibility(t *testing.T) {
	old := "# Notes - acme\n\n" +
		"## 2026-11-21 13:40\n\nfound the login page\n\n" +
		"## 2026-11-21 13:45 | target: 10.10.10.5\n\nweak creds\non the admin form\n\n"
	cur := FormatNote(noteTime.Add(time.Hour), "Cookie flags", "10.10.10.5", "no HttpOnly\n\n- item") +
		FormatNote(noteTime.Add(2*time.Hour), "Only a title", "", "") +
		FormatNote(noteTime.Add(3*time.Hour), "", "", "a | b in the body")

	got := ParseNotes(old + cur)
	at := func(h, m int) time.Time { return time.Date(2026, 11, 21, h, m, 0, 0, time.Local) }
	want := []Note{
		{Time: at(13, 40), Body: "found the login page"},
		{Index: 1, Time: at(13, 45), Target: "10.10.10.5", Body: "weak creds\non the admin form"},
		{Index: 2, Time: at(14, 40), Title: "Cookie flags", Target: "10.10.10.5", Body: "no HttpOnly\n\n- item"},
		{Index: 3, Time: at(15, 40), Title: "Only a title"},
		{Index: 4, Time: at(16, 40), Body: "a | b in the body"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ParseNotes:\n got %+v\nwant %+v", got, want)
	}
}

func TestBodyLinesThatLookLikeHeadersStayInTheBody(t *testing.T) {
	body := "see below\n## 2026-01-02 03:04 | not a note\nend"
	notes := ParseNotes(FormatNote(noteTime, "T", "", body) + FormatNote(noteTime.Add(time.Minute), "Next", "", "x"))
	if len(notes) != 2 || notes[0].Title != "T" || !strings.Contains(notes[0].Body, "not a note") || notes[1].Title != "Next" {
		t.Fatalf("notes = %+v", notes)
	}
	if n := ParseNotes("## 2026-11-21 13:40\n\nx\n\n## 2026-13-45 99:99\n\ny\n"); len(n) != 1 || !strings.Contains(n[0].Body, "2026-13-45") {
		t.Errorf("bad date header: %+v", n)
	}
	if n := ParseNotes("# Notes - a\n\nstray text\n"); len(n) != 0 {
		t.Errorf("text before the first note: %+v", n)
	}
}

func TestAddNoteWritesEntriesWithTitleAndTarget(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")
	if _, err := AddNote(acme, "", "found login page", noteTime); err != nil {
		t.Fatal(err)
	}
	if err := SetVars(acme.Dir, Vars{"target": "10.10.10.5"}); err != nil {
		t.Fatal(err)
	}
	h, err := AddNote(acme, "Weak creds", "on the admin form\nsecond line", noteTime.Add(5*time.Minute))
	if err != nil || h != "## 2026-11-21 13:45 | Weak creds | target: 10.10.10.5" {
		t.Fatalf("header %q, %v", h, err)
	}
	path := NotesPath(acme.Dir)
	b, _ := os.ReadFile(path)
	want := "# Notes - acme\n\n" +
		"## 2026-11-21 13:40\n\nfound login page\n\n" +
		"## 2026-11-21 13:45 | Weak creds | target: 10.10.10.5\n\non the admin form\nsecond line\n\n"
	if string(b) != want {
		t.Errorf("notes.md:\n%s\nwant:\n%s", b, want)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

func TestAddNoteUsesTheGlobalTargetUnlessTheSessionOverridesIt(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")
	if err := SetGlobalVars(Vars{"target": "global.example.local"}); err != nil {
		t.Fatal(err)
	}
	if h, _ := AddNote(acme, "", "a", noteTime); !strings.HasSuffix(h, "target: global.example.local") {
		t.Errorf("global: %q", h)
	}
	SetVars(acme.Dir, Vars{"target": "10.10.10.5"})
	if h, _ := AddNote(acme, "", "b", noteTime); !strings.HasSuffix(h, "target: 10.10.10.5") {
		t.Errorf("session: %q", h)
	}
}

func TestAddNoteRefusals(t *testing.T) {
	isolate(t)
	if _, err := AddNote(Info{Name: "ghost", Dir: filepath.Join(t.TempDir(), "ghost")}, "", "x", noteTime); err == nil {
		t.Error("missing session: want an error")
	}
	acme := mk(t, "acme")
	if _, err := AddNote(acme, "  ", "  \n ", noteTime); err == nil {
		t.Error("empty note: want an error")
	}
	if _, err := ReadNotes(acme.Dir); !errors.Is(err, ErrNoNotes) {
		t.Errorf("ReadNotes without notes: %v", err)
	}
	if p := NotesPath(acme.Dir); fileExists(p) {
		t.Error("an empty note created the file")
	}
	if _, err := AddNote(acme, "Just a title", "", noteTime); err != nil {
		t.Errorf("title only: %v", err)
	}
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

func TestAddNoteSeparatesEntriesInHandEditedFiles(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")
	path := NotesPath(acme.Dir)
	for name, tail := range map[string]string{"no newline": "last line", "one newline": "last line\n", "blank line": "last line\n\n"} {
		os.WriteFile(path, []byte("# Notes - acme\n\n## 2026-11-20 09:15\n\n"+tail), 0o600)
		if _, err := AddNote(acme, "T", "next", noteTime); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(path)
		notes := ParseNotes(string(b))
		if len(notes) != 2 || notes[0].Body != "last line" || notes[1].Title != "T" || notes[1].Body != "next" {
			t.Errorf("%s: notes = %+v\n%s", name, notes, b)
		}
		if strings.Contains(string(b), "line## 2026") || strings.Contains(string(b), "line\n## 2026") {
			t.Errorf("%s: header glued to the previous text:\n%s", name, b)
		}
	}
}

func TestUpdateAndDeleteNoteKeepTheRestOfTheFile(t *testing.T) {
	isolate(t)
	acme := mk(t, "acme")
	path := NotesPath(acme.Dir)
	text := "# Notes - acme\n\nfree text before the notes\n\n" +
		"## 2026-11-20 09:15 | First\n\none\n\n" +
		"## 2026-11-20 09:20 | Second | target: 10.10.10.5\n\ntwo\n\n" +
		"## 2026-11-20 09:25\n\nthree\n"
	os.WriteFile(path, []byte(text), 0o600)
	notes := ParseNotes(text)

	if err := UpdateNote(acme.Dir, notes[1], "Second, edited", "two\nmore"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	want := "# Notes - acme\n\nfree text before the notes\n\n" +
		"## 2026-11-20 09:15 | First\n\none\n\n" +
		"## 2026-11-20 09:20 | Second, edited | target: 10.10.10.5\n\ntwo\nmore\n\n" +
		"## 2026-11-20 09:25\n\nthree\n"
	if string(b) != want {
		t.Fatalf("after update:\n%s", b)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}

	if err := UpdateNote(acme.Dir, notes[1], "x", "y"); !errors.Is(err, ErrNoteChanged) {
		t.Errorf("stale note: %v", err)
	}
	if err := UpdateNote(acme.Dir, notes[0], " ", " "); err == nil {
		t.Error("empty note: want an error")
	}

	notes = ParseNotes(string(b))
	if err := DeleteNote(acme.Dir, notes[0]); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(path)
	left := ParseNotes(string(b))
	if len(left) != 2 || left[0].Title != "Second, edited" || left[1].Body != "three" || !strings.HasPrefix(string(b), "# Notes - acme\n\nfree text before the notes\n\n## 2026-11-20 09:20") {
		t.Fatalf("after delete:\n%s", b)
	}
	for _, n := range left {
		if err := DeleteNote(acme.Dir, ParseNotes(mustRead(t, path))[0]); err != nil {
			t.Fatal(n, err)
		}
	}
	if got := mustRead(t, path); len(ParseNotes(got)) != 0 || !strings.HasPrefix(got, "# Notes - acme") {
		t.Errorf("all deleted:\n%s", got)
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
