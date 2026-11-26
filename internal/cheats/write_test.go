package cheats

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func load1(t *testing.T, path string) []*Cheat {
	t.Helper()
	res := Load([]string{filepath.Dir(path)})
	for _, w := range res.Warnings {
		t.Errorf("warning: %v", w)
	}
	return res.Cheats
}

func TestAppendEntryCreatesFileThatParsesBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cheats", "my.md")
	e := Entry{
		Title:       "scan specific ports",
		Command:     "nmap -sV -p <ports|22,80> <target>",
		Description: "Scan ports on a host\nsecond line",
		Tags:        "nmap,  recon",
	}
	if err := AppendEntry(path, e); err != nil {
		t.Fatal(err)
	}
	got := load1(t, path)
	if len(got) != 1 {
		t.Fatalf("got %d cheats", len(got))
	}
	c := got[0]
	if c.Name != e.Title || c.Title != "nmap" || c.Command != e.Command ||
		c.Description != e.Description || c.Tags != "nmap, recon" {
		t.Errorf("round trip: %+v", c)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

func TestAppendEntryKeepsExistingContentAndMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.md")
	orig := "# Mine\n\n% old, tags\n\n## first\n\n```\necho one\n```\n"
	if err := os.WriteFile(path, []byte(orig), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := AppendEntry(path, Entry{Title: "second", Command: "echo two\necho three"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(b), orig) {
		t.Errorf("existing text changed:\n%s", b)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	got := load1(t, path)
	if len(got) != 2 {
		t.Fatalf("got %d cheats", len(got))
	}
	if got[0].Tags != "old, tags" || got[1].Tags != "" {
		t.Errorf("tags: first %q second %q", got[0].Tags, got[1].Tags)
	}
	if got[1].Command != "echo two;\\\necho three" {
		t.Errorf("multi-line command = %q", got[1].Command)
	}
}

func TestAppendEntryTagsDoNotLeakToTheNextEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.md")
	for _, e := range []Entry{
		{Title: "a", Command: "echo a", Tags: "x"},
		{Title: "b", Command: "echo b"},
		{Title: "c", Command: "echo c", Tags: "y"},
	} {
		if err := AppendEntry(path, e); err != nil {
			t.Fatal(err)
		}
	}
	got := load1(t, path)
	if len(got) != 3 || got[0].Tags != "x" || got[1].Tags != "" || got[2].Tags != "y" {
		t.Errorf("tags: %q %q %q", got[0].Tags, got[1].Tags, got[2].Tags)
	}
}

func TestAppendEntryDescriptionLinesStayDescription(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.md")
	desc := "# not a title\n% not tags\n= not: a variable\n```not a fence"
	if err := AppendEntry(path, Entry{Title: "tricky", Command: "echo hi", Description: desc}); err != nil {
		t.Fatal(err)
	}
	got := load1(t, path)
	if len(got) != 1 || got[0].Name != "tricky" || got[0].Command != "echo hi" || got[0].Tags != "" {
		t.Fatalf("parsed %+v", got)
	}
	for _, want := range []string{"# not a title", "% not tags", "= not: a variable", "```not a fence"} {
		if !strings.Contains(got[0].Description, want) {
			t.Errorf("description %q lost %q", got[0].Description, want)
		}
	}
	if len(got[0].Variables) != 0 {
		t.Errorf("variables = %v", got[0].Variables)
	}
}

func TestAppendEntryRefusals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "my.md")
	if err := AppendEntry(path, Entry{Title: "dup", Command: "echo 1"}); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	if err := AppendEntry(path, Entry{Title: "dup", Command: "echo 2"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate: %v", err)
	}
	for name, e := range map[string]Entry{
		"no title":     {Command: "x"},
		"no command":   {Title: "t"},
		"title hash":   {Title: "# t", Command: "x"},
		"title lines":  {Title: "a\nb", Command: "x"},
		"fence in cmd": {Title: "t", Command: "echo\n```\nrm"},
	} {
		if err := AppendEntry(filepath.Join(dir, "other.md"), e); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if err := AppendEntry(filepath.Join(dir, "x.yml"), Entry{Title: "t", Command: "x"}); err == nil {
		t.Error("yml target: want an error")
	}
	bad := filepath.Join(dir, "bad.md")
	os.WriteFile(bad, []byte("# a\n### skipped\n"), 0o644)
	if err := AppendEntry(bad, Entry{Title: "t", Command: "x"}); err == nil {
		t.Error("unparsable file: want an error")
	}
	if b, _ := os.ReadFile(bad); string(b) != "# a\n### skipped\n" {
		t.Errorf("unparsable file modified: %q", b)
	}
	after, _ := os.ReadFile(path)
	if string(after) != string(before) {
		t.Error("file changed after refused duplicates")
	}
	if _, err := os.Stat(filepath.Join(dir, "other.md")); err == nil {
		t.Error("invalid entries created a file")
	}
}

func TestTitleExists(t *testing.T) {
	p := filepath.Join(t.TempDir(), "my.md")
	if TitleExists(p, "x") {
		t.Error("a missing file has no titles")
	}
	if err := AppendEntry(p, Entry{Title: "scan ports", Command: "nmap <target>"}); err != nil {
		t.Fatal(err)
	}
	if !TitleExists(p, "scan ports") || !TitleExists(p, "  scan ports ") {
		t.Error("existing title not found")
	}
	if TitleExists(p, "other") {
		t.Error("unknown title found")
	}
}
