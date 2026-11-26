package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ynn3m/arsenal/internal/tui"
)

func isolateNotes(t *testing.T) string {
	t.Helper()
	root := isolate(t)
	fixed := time.Date(2026, 11, 21, 13, 40, 59, 0, time.Local)
	now = func() time.Time { return fixed }
	runNotes = func(tui.NotesOptions) error {
		t.Fatal("unexpected viewer")
		return nil
	}
	t.Cleanup(func() { now = time.Now; runNotes = tui.RunNotes })
	return root
}

func runIn(t *testing.T, stdin string, args ...string) (string, int) {
	t.Helper()
	root := newRootCmd("ars")
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	code := execute(root, &errb)
	return out.String() + errb.String(), code
}

func TestNoteAppendsTimestampedNotesWithTitleAndTarget(t *testing.T) {
	root := isolateNotes(t)
	if out, code := run(t, "note", "too early"); code == 0 || !strings.Contains(out, noSessionMsg) {
		t.Errorf("without a session: code %d: %s", code, out)
	}
	inSession(t, "acme")

	out, code := run(t, "note", "found", "the login page")
	if code != 0 || !strings.Contains(out, "Note added to acme: 2026-11-21 13:40") {
		t.Fatalf("code %d: %s", code, out)
	}
	run(t, "set", "target=10.10.10.5")
	if out, code := run(t, "note", "-t", "Admin panel", "default creds rejected"); code != 0 ||
		!strings.Contains(out, "2026-11-21 13:40 | Admin panel | target: 10.10.10.5") {
		t.Fatalf("with title: code %d: %s", code, out)
	}
	if out, code := runIn(t, "output of a tool\nsecond line\n", "note", "--title", "nmap scan"); code != 0 {
		t.Fatalf("piped: code %d: %s", code, out)
	}
	b, err := os.ReadFile(filepath.Join(root, "acme", ".arsenal", "notes.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# Notes - acme\n\n" +
		"## 2026-11-21 13:40\n\nfound the login page\n\n" +
		"## 2026-11-21 13:40 | Admin panel | target: 10.10.10.5\n\ndefault creds rejected\n\n" +
		"## 2026-11-21 13:40 | nmap scan | target: 10.10.10.5\n\noutput of a tool\nsecond line\n\n"
	if string(b) != want {
		t.Errorf("notes.md:\n%s\nwant:\n%s", b, want)
	}
}

func TestNoteWithoutTextOpensTheFormOrReadsAPipe(t *testing.T) {
	root := isolateNotes(t)
	inSession(t, "acme")
	if out, code := runIn(t, "  \n", "note"); code == 0 || !strings.Contains(out, "no note text") {
		t.Errorf("empty pipe: code %d: %s", code, out)
	}
	if out, code := runIn(t, "", "note", "-t", "Remember to retest"); code != 0 {
		t.Errorf("title only: code %d: %s", code, out)
	}

	isInteractive = func() bool { return true }
	st := newStepScript(t, map[string]any{"title": "From the box", "text": "first line\nsecond line"})
	out, code := run(t, "note", "-t", "prefilled")
	if code != 0 {
		t.Fatalf("box: code %d: %s", code, out)
	}
	if got := strings.Join(st.asked, ","); got != "title,text" {
		t.Errorf("asked = %s", got)
	}
	if st.steps["title"].Default != "prefilled" || st.steps["title"].Kind != tui.StepText {
		t.Errorf("title step = %+v", st.steps["title"])
	}
	if st.steps["text"].Kind != tui.StepMultiline || st.steps["text"].Validate("   ") == nil || st.steps["text"].Validate("x") != nil {
		t.Errorf("text step = %+v", st.steps["text"])
	}
	sum := st.summaryText()
	if !strings.Contains(sum, "From the box") || !strings.Contains(sum, "acme at 13:40") {
		t.Errorf("summary:\n%s", sum)
	}
	done := strings.Join(st.box.Confirmed(tui.Answers{"title": {Text: "From the box"}, "text": {Text: "x"}}), "\n")
	if !strings.Contains(done, "Note added to acme at 13:40") || !strings.Contains(done, "From the box") {
		t.Errorf("confirmed box:\n%s", done)
	}
	b, _ := os.ReadFile(filepath.Join(root, "acme", ".arsenal", "notes.md"))
	if !strings.Contains(string(b), "## 2026-11-21 13:40 | From the box\n\nfirst line\nsecond line\n") ||
		!strings.Contains(string(b), "| Remember to retest\n") {
		t.Errorf("notes.md = %q", b)
	}

	newStepScript(t, map[string]any{"title": "", "text": "only text"})
	if out, code := run(t, "note"); code != 0 {
		t.Fatalf("no title: code %d: %s", code, out)
	}
	b, _ = os.ReadFile(filepath.Join(root, "acme", ".arsenal", "notes.md"))
	if !strings.Contains(string(b), "## 2026-11-21 13:40\n\nonly text\n") {
		t.Errorf("notes.md = %q", b)
	}

	before, _ := os.ReadFile(filepath.Join(root, "acme", ".arsenal", "notes.md"))
	c := newStepScript(t, nil)
	c.cancel = true
	if out, code := run(t, "note"); code != 130 || !strings.Contains(out, "Cancelled") {
		t.Errorf("cancel: code %d: %s", code, out)
	}
	if after, _ := os.ReadFile(filepath.Join(root, "acme", ".arsenal", "notes.md")); string(after) != string(before) {
		t.Error("a cancelled box changed the notes")
	}

	runSteps = func(tui.StepBox) (tui.Answers, error) { t.Fatal("unexpected step box"); return nil, nil }
	if _, code := run(t, "note", "inline text"); code != 0 {
		t.Error("note with arguments failed")
	}
	if _, code := run(t, "note", "-t", "T", "inline text"); code != 0 {
		t.Error("note -t with a text failed")
	}
}

func TestNotesViewerGetsTheParsedNotesAndSession(t *testing.T) {
	root := isolateNotes(t)
	inSession(t, "acme")
	if out, code := run(t, "notes"); code != 0 || strings.TrimSpace(out) != "" {
		t.Errorf("no notes: code %d: %s", code, out)
	}
	isInteractive = func() bool { return true }
	var empty tui.NotesOptions
	runNotes = func(o tui.NotesOptions) error { empty = o; return nil }
	if out, code := run(t, "notes"); code != 0 || empty.Session != "acme" || len(empty.Notes) != 0 {
		t.Errorf("no notes, viewer: code %d: %s %+v", code, out, empty)
	}
	isInteractive = func() bool { return false }
	run(t, "note", "-t", "Hello", "hello notes")
	run(t, "note", "second note")

	out, code := run(t, "notes")
	if code != 0 || !strings.Contains(out, "## 2026-11-21 13:40 | Hello\n\nhello notes") {
		t.Errorf("raw print: code %d: %s", code, out)
	}

	isInteractive = func() bool { return true }
	var got tui.NotesOptions
	runNotes = func(o tui.NotesOptions) error { got = o; return nil }
	if out, code := run(t, "notes", "-s", "hello"); code != 0 {
		t.Fatalf("viewer: code %d: %s", code, out)
	}
	if got.Session != "acme" || got.Query != "hello" || got.Path != filepath.Join(root, "acme", ".arsenal", "notes.md") ||
		len(got.Notes) != 2 || got.Notes[0].Title != "Hello" || got.Notes[0].Body != "hello notes" || got.Notes[1].Body != "second note" {
		t.Errorf("viewer options = %+v", got)
	}
}

func TestNotesViewerEditsAndDeletesInTheFile(t *testing.T) {
	root := isolateNotes(t)
	inSession(t, "acme")
	run(t, "note", "-t", "Hello", "hello notes")
	run(t, "note", "second note")
	isInteractive = func() bool { return true }
	var got tui.NotesOptions
	runNotes = func(o tui.NotesOptions) error { got = o; return nil }
	if out, code := run(t, "notes"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	path := filepath.Join(root, "acme", ".arsenal", "notes.md")

	res, err := got.Save(got.Notes[0], "Hello again", "edited text")
	if err != nil || res.Message != "Note saved" || res.Select != 0 || len(res.Notes) != 2 || res.Notes[0].Title != "Hello again" || res.Notes[0].Body != "edited text" {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := got.Save(got.Notes[0], "x", "y"); err == nil {
		t.Error("saving a stale note: want an error")
	}
	res, err = got.Delete(res.Notes[1])
	if err != nil || res.Message != "Note deleted" || len(res.Notes) != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "| Hello again") || strings.Contains(string(b), "second note") {
		t.Errorf("notes.md:\n%s", b)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
}
