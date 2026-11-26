package cheats

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToolOf(t *testing.T) {
	for cmd, want := range map[string]string{
		"nmap -sV <target>":                     "nmap",
		"curl -I <url>":                         "curl",
		"psexec.py <user>@<target>":             "psexec",
		"/usr/bin/nmap -p- <target>":            "nmap",
		"sudo nmap -sU <target>":                "nmap",
		"TERM=xterm ssh <user>@<host>":          "ssh",
		"impacket-psexec <target>":              "impacket-psexec",
		"Rubeus.exe kerberoast":                 "Rubeus",
		"nmap -sV\nnmap -p- <target>":           "nmap",
		"<tool> --help":                         "",
		"$(which nmap) -sV":                     "",
		"":                                      "",
		".py":                                   ".py"[:0],
		"python3 -m http.server <port|80>":      "python3",
		`"C:\Tools\x.exe" run`:                  "",
		"for i in $(seq 1 3); do echo $i; done": "for",
	} {
		if got := ToolOf(cmd); got != want {
			t.Errorf("ToolOf(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "my.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func entriesByName(list []*Cheat) map[string]*Cheat {
	m := map[string]*Cheat{}
	for _, c := range list {
		m[strings.TrimSpace(c.Name)] = c
	}
	return m
}

func TestAppendEntryGroupsUnderTheToolSection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.md")
	for _, e := range []Entry{
		{Title: "scan ports", Command: "nmap -p <ports> <target>"},
		{Title: "fetch headers", Command: "curl -I <url>"},
		{Title: "scan all ports", Command: "nmap -p- <target>"},
		{Title: "psexec shell", Command: "psexec.py <user>@<target>"},
		{Title: "odd one", Command: "<tool> --help"},
	} {
		if err := AppendEntry(path, e); err != nil {
			t.Fatal(err)
		}
	}
	got := entriesByName(load1(t, path))
	want := map[string]string{
		"scan ports": "nmap", "scan all ports": "nmap", "fetch headers": "curl",
		"psexec shell": "psexec", "odd one": "My cheats",
	}
	for name, section := range want {
		if c := got[name]; c == nil || c.Title != section {
			t.Errorf("%q %v %q", name, c, section)
		}
	}
	text := readFile(t, path)
	if strings.Count(text, "# nmap\n") != 1 {
		t.Errorf("%s", text)
	}
	if !strings.Contains(text, "## scan ports") || strings.Index(text, "## scan all ports") > strings.Index(text, "# curl") {
		t.Errorf("%s", text)
	}
}

func TestAppendEntryInTheMiddleKeepsTheTagsOfTheOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.md")
	for _, e := range []Entry{
		{Title: "a", Command: "nmap a", Tags: "x"},
		{Title: "b", Command: "curl b", Tags: "y"},
		{Title: "c", Command: "curl c"},
		{Title: "d", Command: "nmap d", Tags: "z, w"},
	} {
		if err := AppendEntry(path, e); err != nil {
			t.Fatal(err)
		}
	}
	got := entriesByName(load1(t, path))
	for name, tags := range map[string]string{"a": "x", "b": "y", "c": "", "d": "z, w"} {
		if got[name].Tags != tags {
			t.Errorf("%s: tags %q, want %q", name, got[name].Tags, tags)
		}
	}
}

func TestAppendEntryKeepsTagsOfHandWrittenNeighbours(t *testing.T) {
	path := writeFile(t, "# nmap\n\n## first\n% old\n\n```\nnmap one\n```\n\n## second\n\n```\nnmap two\n```\n")
	if err := AppendEntry(path, Entry{Title: "third", Command: "nmap three", Tags: "new"}); err != nil {
		t.Fatal(err)
	}
	got := entriesByName(load1(t, path))
	if got["first"].Tags != "old" || got["second"].Tags != "old" || got["third"].Tags != "new" {
		t.Errorf("tags: %q %q %q", got["first"].Tags, got["second"].Tags, got["third"].Tags)
	}
}

func TestEntryTagsWithASlashAreCommandTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), "my.md")
	if err := AppendEntry(path, Entry{Title: "t", Command: "nmap x", Tags: "recon, cat/RECON/SCAN, #os/linux"}); err != nil {
		t.Fatal(err)
	}
	c := load1(t, path)[0]
	if c.Tags != "recon" {
		t.Errorf("tags = %q", c.Tags)
	}
	if v, _ := c.CommandTags.Get("cat"); v != "RECON/SCAN" {
		t.Errorf("cat = %q (%v)", v, c.CommandTags)
	}
	if v, _ := c.CommandTags.Get("os"); v != "linux" {
		t.Errorf("os = %q", v)
	}
	if got := strings.Join(c.TagItems(), ","); got != "recon,cat/RECON/SCAN,os/linux" {
		t.Errorf("TagItems = %q", got)
	}
}

const sectioned = `# nmap

## scan ports
% recon

Scan some ports.

` + "```" + `
nmap -p <ports> <target>
` + "```" + `

## scan all ports

` + "```" + `
nmap -p- <target>
` + "```" + `

# curl

## fetch headers

` + "```" + `
curl -I <url>
` + "```" + `
`

func entryOf(t *testing.T, path, name string) *Cheat {
	t.Helper()
	c := entriesByName(load1(t, path))[name]
	if c == nil {
		t.Fatalf("no entry %q", name)
	}
	return c
}

func TestUpdateEntryChangesOnlyThatEntry(t *testing.T) {
	path := writeFile(t, sectioned)
	c := entryOf(t, path, "scan ports")
	err := UpdateEntry(c, Entry{
		Title: "scan some ports", Command: "nmap -sV -p <ports|22,80> <target>",
		Description: "Better description", Tags: "recon, tcp, cat/RECON",
	})
	if err != nil {
		t.Fatal(err)
	}
	got := entriesByName(load1(t, path))
	e := got["scan some ports"]
	if e == nil || e.Command != "nmap -sV -p <ports|22,80> <target>" || e.Description != "Better description" ||
		e.Tags != "recon, tcp" || e.Title != "nmap" {
		t.Fatalf("edited entry: %+v", e)
	}
	if _, ok := got["scan ports"]; ok {
		t.Error(got["scan ports"])
	}
	text := readFile(t, path)
	for _, keep := range []string{"## scan all ports\n% recon\n\n```\nnmap -p- <target>\n```\n\n# curl\n\n## fetch headers\n\n```\ncurl -I <url>\n```\n"} {
		if !strings.HasSuffix(text, keep) {
			t.Errorf("%s", text)
		}
	}
	if !strings.HasPrefix(text, "# nmap\n\n## scan some ports\n") {
		t.Errorf("%s", text)
	}
	if got["scan all ports"].Tags != "recon" || got["fetch headers"].Tags != "recon" {
		t.Errorf("%q %q", got["scan all ports"].Tags, got["fetch headers"].Tags)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
}

func TestUpdateEntryStaysInItsPlaceWhateverTheCommand(t *testing.T) {
	path := writeFile(t, sectioned)
	if err := UpdateEntry(entryOf(t, path, "fetch headers"), Entry{Title: "fetch headers", Command: "wget -S <url>"}); err != nil {
		t.Fatal(err)
	}
	c := entryOf(t, path, "fetch headers")
	if c.Title != "curl" || c.Command != "wget -S <url>" {
		t.Errorf("%+v", c)
	}
}

func names(list []*Cheat) []string {
	var out []string
	for _, c := range list {
		out = append(out, c.Title+"/"+c.Name)
	}
	return out
}

func TestUpdateEntryKeepsTheTagsOfTheOtherEntries(t *testing.T) {
	path := writeFile(t, sectioned)
	if err := UpdateEntry(entryOf(t, path, "scan ports"), Entry{Title: "scan ports", Command: "curl x"}); err != nil {
		t.Fatal(err)
	}
	got := entriesByName(load1(t, path))
	if got["scan all ports"].Tags != "recon" {
		t.Errorf("%q", got["scan all ports"].Tags)
	}
	if got["scan ports"].Tags != "" {
		t.Errorf("%q", got["scan ports"].Tags)
	}
}

func TestUpdateEntryRefusals(t *testing.T) {
	path := writeFile(t, sectioned)
	c := entryOf(t, path, "scan ports")

	if err := UpdateEntry(c, Entry{Title: "fetch headers", Command: "nmap x"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate title: %v", err)
	}
	if err := UpdateEntry(c, Entry{Title: "", Command: "nmap x"}); err == nil {
		t.Error(err)
	}
	if err := UpdateEntry(c, Entry{Title: "t", Command: " "}); err == nil {
		t.Error(err)
	}
	stale := *c
	stale.Line += 2
	if err := UpdateEntry(&stale, Entry{Title: "t", Command: "nmap x"}); !errors.Is(err, ErrChanged) {
		t.Errorf("stale entry: %v", err)
	}
	yml := *c
	yml.Filename = filepath.Join(t.TempDir(), "x.yml")
	if err := UpdateEntry(&yml, Entry{Title: "t", Command: "nmap x"}); err == nil {
		t.Error(err)
	}
	if readFile(t, path) != sectioned {
		t.Error(readFile(t, path))
	}
}

func TestUpdateEntryKeepsFileVariables(t *testing.T) {
	path := writeFile(t, "# nmap\n\n## scan\n= port: 22\n\n```\nnmap -p <port>\n```\n")
	if err := UpdateEntry(entryOf(t, path, "scan"), Entry{Title: "scan", Command: "nmap -p <port> <target>"}); err != nil {
		t.Fatal(err)
	}
	if c := entryOf(t, path, "scan"); c.Variables["port"] != "22" {
		t.Errorf("variables = %v", c.Variables)
	}
}

func TestDeleteEntryRemovesOnlyThatEntry(t *testing.T) {
	path := writeFile(t, sectioned)
	if err := DeleteEntry(entryOf(t, path, "scan all ports")); err != nil {
		t.Fatal(err)
	}
	got := entriesByName(load1(t, path))
	if len(got) != 2 || got["scan ports"] == nil || got["fetch headers"] == nil {
		t.Fatalf("entries left: %v", names(load1(t, path)))
	}
	want := strings.Replace(sectioned, "## scan all ports\n\n```\nnmap -p- <target>\n```\n\n", "", 1)
	if text := readFile(t, path); text != want {
		t.Errorf("file:\n%s\nwant:\n%s", text, want)
	}
	if got["fetch headers"].Tags != "recon" {
		t.Errorf("%q", got["fetch headers"].Tags)
	}
}

func TestDeleteEntryDropsTheEmptiedSectionAndKeepsTheTagsOfTheOthers(t *testing.T) {
	path := writeFile(t, sectioned)
	if err := DeleteEntry(entryOf(t, path, "fetch headers")); err != nil {
		t.Fatal(err)
	}
	if text := readFile(t, path); strings.Contains(text, "curl") || !strings.HasSuffix(text, "nmap -p- <target>\n```\n") {
		t.Errorf("file:\n%s", text)
	}
	path = writeFile(t, sectioned)
	if err := DeleteEntry(entryOf(t, path, "scan ports")); err != nil {
		t.Fatal(err)
	}
	if got := entriesByName(load1(t, path)); got["scan all ports"].Tags != "recon" || got["fetch headers"].Tags != "recon" {
		t.Errorf("tags: %q %q", got["scan all ports"].Tags, got["fetch headers"].Tags)
	}
}

func TestDeleteEntryRefusals(t *testing.T) {
	path := writeFile(t, sectioned)
	c := entryOf(t, path, "scan ports")
	stale := *c
	stale.Line += 2
	if err := DeleteEntry(&stale); !errors.Is(err, ErrChanged) {
		t.Errorf("stale entry: %v", err)
	}
	yml := *c
	yml.Filename = filepath.Join(t.TempDir(), "x.yml")
	if err := DeleteEntry(&yml); err == nil {
		t.Error(err)
	}
	if readFile(t, path) != sectioned {
		t.Error(readFile(t, path))
	}
}
