package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/tui"
)

func editFromSearch(t *testing.T, fn func(o tui.SearchOptions)) {
	t.Helper()
	runSearchUI = func(o tui.SearchOptions) (tui.SearchResult, error) {
		fn(o)
		return tui.SearchResult{Query: o.Query}, nil
	}
	t.Cleanup(func() { runSearchUI = tui.RunSearch })
	if out, code := run(t, "--print"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
}

func findCheat(t *testing.T, list []*cheats.Cheat, name string) *cheats.Cheat {
	t.Helper()
	for _, c := range list {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%q %d", name, len(list))
	return nil
}

func writeCollection(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := cheatsDirT(t)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func parseFile(t *testing.T, path string) []*cheats.Cheat {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	list, err := cheats.ParseMarkdown(path, f)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func TestSearchEditsCommandsOfTheCollectionInPlace(t *testing.T) {
	isolate(t)
	dir := writeCollection(t, map[string]string{
		"nmap.md": "# nmap\n\n## scan ports\n\n```\nnmap -p <ports> <target>\n```\n",
		"curl.md": "# curl\n\n## fetch headers\n\n```\ncurl -I <url>\n```\n",
	})

	editFromSearch(t, func(o tui.SearchOptions) {
		c := findCheat(t, o.Cheats, "scan ports")
		if o.ReadOnly(c) {
			t.Error(c.Filename)
		}
		res, err := o.Save(c, cheats.Entry{Title: "scan ports fast", Command: "nmap -T4 -p <ports> <target>", Description: "Fast scan", Tags: "recon"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Message != "Saved in nmap.md" {
			t.Errorf("%q", res.Message)
		}
		if res.Saved == nil || res.Saved.Name != "scan ports fast" || res.Saved.Description != "Fast scan" {
			t.Errorf("%+v", res.Saved)
		}
		if len(res.Cheats) != 2 {
			t.Errorf("%d", len(res.Cheats))
		}
	})
	b, _ := os.ReadFile(filepath.Join(dir, "nmap.md"))
	want := "# nmap\n\n## scan ports fast\n% recon\nFast scan\n\n```\nnmap -T4 -p <ports> <target>\n```\n"
	if string(b) != want {
		t.Errorf("%s\n%s", b, want)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "curl.md")); !strings.Contains(string(b), "curl -I <url>") {
		t.Errorf("%s", b)
	}
}

func TestSearchEditOfAnArsenalCommandStaysInPlaceAndItBecomesYours(t *testing.T) {
	isolate(t)
	dir := writeCollection(t, map[string]string{
		"impacket.md": "# impacket\n\n## psexec shell\n#src/arsenal\n% smb\n\n```\npsexec.py <user>@<target>\n```\n",
	})

	editFromSearch(t, func(o tui.SearchOptions) {
		c := findCheat(t, o.Cheats, "psexec shell")
		if !c.FromArsenal() || o.ReadOnly(c) {
			t.Fatalf("%+v", c)
		}
		res, err := o.Save(c, cheats.Entry{Title: "psexec shell", Command: "psexec.py -hashes <hashes> <user>@<target>", Tags: "smb, lateral"})
		if err != nil {
			t.Fatal(err)
		}
		if res.Saved == nil || res.Saved.FromArsenal() || res.Saved.Tags != "smb, lateral" {
			t.Errorf("%+v", res.Saved)
		}
	})
	b, _ := os.ReadFile(filepath.Join(dir, "impacket.md"))
	if strings.Contains(string(b), "src/arsenal") || !strings.Contains(string(b), "-hashes") {
		t.Errorf("%s", b)
	}
	if a, own := cheats.Count(dir); a != 0 || own != 1 {
		t.Errorf("%d %d", a, own)
	}
}

func TestSearchNeverChangesCommandsOfReadOnlyFolders(t *testing.T) {
	isolate(t)
	writeCollection(t, map[string]string{"nmap.md": "# nmap\n\n## scan\n\n```\nnmap x\n```\n"})
	legacy := filepath.Join(os.Getenv("HOME"), ".cheats")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	text := "# upstream\n\n## fetch\n\n```\ncurl y\n```\n"
	file := filepath.Join(legacy, "upstream.md")
	if err := os.WriteFile(file, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}

	editFromSearch(t, func(o tui.SearchOptions) {
		c := findCheat(t, o.Cheats, "fetch")
		if !o.ReadOnly(c) {
			t.Error(c.Filename)
		}
		if _, err := o.Save(c, cheats.Entry{Title: "fetch", Command: "curl z"}); err == nil {
			t.Error("saved")
		}
		if _, err := o.Delete(c); err == nil {
			t.Error("deleted")
		}
	})
	if b, _ := os.ReadFile(file); string(b) != text {
		t.Errorf("%s", b)
	}
}

func TestSearchKeepsTheEditedListForTheNextScreen(t *testing.T) {
	setupSearch(t)
	var calls int
	runSearchUI = func(o tui.SearchOptions) (tui.SearchResult, error) {
		calls++
		if calls == 1 {
			c := findCheat(t, o.Cheats, "scan all ports")
			if _, err := o.Save(c, cheats.Entry{Title: "scan every port", Command: "nmap -p- -T4 <target>"}); err != nil {
				t.Fatal(err)
			}
			return tui.SearchResult{Cheat: c, Values: map[string]string{"target": "10.10.10.5"}}, nil
		}
		findCheat(t, o.Cheats, "scan every port")
		return tui.SearchResult{}, nil
	}
	t.Cleanup(func() { runSearchUI = tui.RunSearch })
	deliverBackup := deliver
	t.Cleanup(func() { deliver = deliverBackup })
	deliver = func(cmd *cobra.Command, line string, f *searchFlags) (bool, delivery, error) {
		return true, delivery{}, nil
	}
	if out, code := run(t, "--print"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if calls != 2 {
		t.Errorf("%d", calls)
	}
}

func TestSearchDeletesAnyCommandOfTheCollectionArsenalOnesToo(t *testing.T) {
	isolate(t)
	dir := writeCollection(t, map[string]string{
		"nmap.md": "# nmap\n\n## scan ports\n#src/arsenal\n\n```\nnmap -p <ports> <target>\n```\n\n## mine\n%\n\n```\nnmap mine\n```\n",
		"curl.md": "# curl\n\n## fetch headers\n\n```\ncurl -I <url>\n```\n",
	})

	editFromSearch(t, func(o tui.SearchOptions) {
		res, err := o.Delete(findCheat(t, o.Cheats, "scan ports"))
		if err != nil {
			t.Fatal(err)
		}
		if res.Message != "Deleted scan ports from nmap.md" || len(res.Cheats) != 2 {
			t.Errorf("%q %d", res.Message, len(res.Cheats))
		}
		if _, err := o.Delete(findCheat(t, res.Cheats, "fetch headers")); err != nil {
			t.Fatal(err)
		}
	})
	b, _ := os.ReadFile(filepath.Join(dir, "nmap.md"))
	if string(b) != "# nmap\n\n## mine\n%\n\n```\nnmap mine\n```\n" {
		t.Errorf("%s", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "curl.md")); err != nil {
		t.Errorf("%v", err)
	}
}

func TestAddWritesToTheFileOfTheTool(t *testing.T) {
	isolateAdd(t)
	for _, args := range [][]string{
		{"add", "nmap -sV <target>", "--title", "service scan"},
		{"add", "psexec.py <user>@<target>", "--title", "psexec shell"},
		{"add", "nmap -p- <target>", "--title", "all ports"},
		{"add", "<tool> --help", "--title", "odd one"},
	} {
		if out, code := run(t, args...); code != 0 {
			t.Fatalf("%v: code %d: %s", args, code, out)
		}
	}
	dir := cheatsDirT(t)
	for file, titles := range map[string][]string{
		"nmap.md":      {"service scan", "all ports"},
		"psexec.md":    {"psexec shell"},
		"my-cheats.md": {"odd one"},
	} {
		var got []string
		for _, c := range parseFile(t, filepath.Join(dir, file)) {
			got = append(got, c.Name)
			if c.FromArsenal() {
				t.Errorf("%s %s", file, c.Name)
			}
		}
		if strings.Join(got, ",") != strings.Join(titles, ",") {
			t.Errorf("%s: %v, want %v", file, got, titles)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "my.md")); err == nil {
		t.Error("my.md")
	}
}
