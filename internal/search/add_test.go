package search

import (
	"path/filepath"
	"testing"

	"github.com/ynn3m/arsenal/internal/cheats"
)

func TestAddedEntryIsFoundByItsDescription(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.md")
	err := cheats.AppendEntry(path, cheats.Entry{
		Title:       "sweep a host",
		Command:     "nmap -sV <target>",
		Description: "Use it to scan ports quickly on one host",
		Tags:        "recon",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := cheats.AppendEntry(path, cheats.Entry{Title: "unrelated", Command: "ls -la", Description: "list files"}); err != nil {
		t.Fatal(err)
	}
	res := cheats.Load([]string{filepath.Dir(path)})
	if len(res.Warnings) != 0 || len(res.Cheats) != 2 {
		t.Fatalf("loaded %d cheats, warnings %v", len(res.Cheats), res.Warnings)
	}
	ix := NewIndex(res.Cheats)

	for _, q := range []string{"scan port", "scan ports", "SCAN PORT", "quickly host"} {
		got := ix.Filter(q)
		if len(got) != 1 || got[0].Name != "sweep a host" {
			t.Errorf("Filter(%q) = %v, want the entry whose description says \"scan ports\"", q, names(got))
		}
	}
	if got := ix.Filter("scan port !recon"); len(got) != 0 {
		t.Errorf("exclusion by tag: %v", names(got))
	}
}

func TestTextIndexUsesTheSameRulesAsTheCommandSearch(t *testing.T) {
	docs := [][]string{
		{"Login page", "found the admin login"},
		{"", "default creds on the admin form"},
		{"Cookie flags", "no HttpOnly on the session cookie"},
	}
	ix := NewTextIndex(docs)
	for _, c := range []struct {
		input string
		want  []int
	}{
		{"", []int{0, 1, 2}},
		{"admin", []int{0, 1}},
		{"ADMIN login", []int{0}},
		{"admin !login", []int{1}},
		{"cookie", []int{2}},
		{"creds form", []int{1}},
		{"nothing here", nil},
		{"httponly sesion", []int{2}},
	} {
		got := ix.Filter(c.input)
		if len(got) != len(c.want) {
			t.Errorf("Filter(%q) = %v, want %v", c.input, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("Filter(%q) = %v, want %v", c.input, got, c.want)
			}
		}
	}
	if got := ix.Filter("cooki"); len(got) != 1 || got[0] != 2 {
		t.Errorf("prefix: %v", got)
	}
}

func TestTerms(t *testing.T) {
	got := Terms("Admin  !login creds !")
	if len(got) != 3 || got[0] != "admin" || got[1] != "creds" || got[2] != "!" {
		t.Errorf("Terms = %q", got)
	}
}
