// Tests for the Markdown cheatsheet parser.

package cheats

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func parseMD(t *testing.T, src string) []*Cheat {
	t.Helper()
	l, err := ParseMarkdown("test.md", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func byName(l []*Cheat, name string) *Cheat {
	for _, c := range l {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestMarkdownBasics(t *testing.T) {
	l := parseMD(t, "# nmap\n\n% scan, network\n\n#plateform/linux #cat/RECON\n\n## scan ports\nDescription line\n```\nnmap -p <ports> <target>\n```\n")
	if len(l) != 1 {
		t.Fatalf("got %d cheats", len(l))
	}
	c := l[0]
	if c.Name != "scan ports" || c.Title != "nmap" || c.Tags != "scan, network" {
		t.Errorf("name/title/tags = %q %q %q", c.Name, c.Title, c.Tags)
	}
	if c.Command != "nmap -p <ports> <target>" || c.Description != "Description line" {
		t.Errorf("command/description = %q %q", c.Command, c.Description)
	}
	if v, _ := c.CommandTags.Get("plateform"); v != "linux" {
		t.Errorf("tags = %v", c.CommandTags)
	}
	if v, _ := c.CommandTags.Get("cat"); v != "RECON" {
		t.Errorf("tags = %v", c.CommandTags)
	}
	if c.Key() != "nmapscan ports" {
		t.Errorf("key = %q", c.Key())
	}
}

func TestMarkdownMultiLineCommand(t *testing.T) {
	l := parseMD(t, "# t\n## a\n```\none\ntwo;\nthree\n```\n")
	if want := "one;\\\ntwo;\\\nthree"; l[0].Command != want {
		t.Errorf("command = %q, want %q", l[0].Command, want)
	}
	if got := l[0].PrintableCommand(); got != "one;two;three" {
		t.Errorf("printable = %q", got)
	}
}

func TestMarkdownCommandTagsInheritance(t *testing.T) {
	l := parseMD(t, "# t\n#plateform/linux #target/remote\n\n## a\n#target/local\n```\nx\n```\n\n## b\n```\ny\n```\n")
	a, b := byName(l, "a"), byName(l, "b")
	if v, _ := a.CommandTags.Get("target"); v != "local" {
		t.Errorf("a target = %q, want local", v)
	}
	if v, _ := b.CommandTags.Get("target"); v != "remote" {
		t.Errorf("b target = %q, want remote (no leak from a)", v)
	}
	if v, _ := b.CommandTags.Get("plateform"); v != "linux" {
		t.Errorf("b plateform = %q", v)
	}
	if tag, ok := a.CommandTags.Get("cat"); ok {
		t.Errorf("unexpected tag cat=%q", tag)
	}
}

func TestMarkdownNestedTitlesAndFirstTitle(t *testing.T) {
	l := parseMD(t, "# top\n## mid\n### leaf\n```\nc1\n```\n## other\n```\nc2\n```\n")
	if c := byName(l, "leaf"); c == nil || c.Title != "top, mid" {
		t.Errorf("leaf = %+v", c)
	}
	if c := byName(l, "other"); c == nil || c.Title != "top" {
		t.Errorf("other = %+v", c)
	}
	l = parseMD(t, "# only\n```\ncmd\n```\n")
	if len(l) != 1 || l[0].Title != "only" || l[0].Name != "only" {
		t.Errorf("single level: %+v", l[0])
	}
}

func TestMarkdownDescriptionOnlyBecomesCommand(t *testing.T) {
	l := parseMD(t, "# t\n## note\nfirst line\nsecond line\n## next\n```\nx\n```\n")
	c := byName(l, "note")
	if c == nil || c.Command != "first line;\\\nsecond line" || c.Description != "" {
		t.Errorf("note = %+v", c)
	}
	l = parseMD(t, "# t\n## note\nonly text\n")
	if len(l) != 1 || l[0].Command != "only text" {
		t.Errorf("eof: %+v", l)
	}
}

func TestMarkdownVariables(t *testing.T) {
	l := parseMD(t, "# t\n= wordlist: /tmp/w.txt\n= url: http://example.local:8080/x\n$ ip: ip route | head -1\n## a\n```\nx <wordlist>\n```\n")
	v := l[0].Variables
	want := map[string]string{
		"wordlist": "/tmp/w.txt",
		"url":      "http://example.local:8080/x",
		"ip":       "$(ip route | head -1)",
	}
	if !reflect.DeepEqual(v, want) {
		t.Errorf("variables = %v, want %v", v, want)
	}
}

func TestMarkdownVariablesAreScopedPerFile(t *testing.T) {
	a := parseMD(t, "# a\n= shared: from-a\n## x\n```\ncmd\n```\n")
	b := parseMD(t, "# b\n## y\n```\ncmd\n```\n")
	if _, ok := b[0].Variables["shared"]; ok {
		t.Error("variables of one file leaked into another")
	}
	if a[0].Variables["shared"] != "from-a" {
		t.Error("variable missing in its own file")
	}
}

func TestMarkdownFenceContentIsLiteral(t *testing.T) {
	l := parseMD(t, "# t\n## a\n```\n#!/bin/sh\n% not tags\n## not a title\n```\n")
	if len(l) != 1 || l[0].Command != "#!/bin/sh;\\\n% not tags;\\\n## not a title" {
		t.Errorf("got %+v", l)
	}
	if l[0].Tags != "" {
		t.Errorf("tags = %q", l[0].Tags)
	}
}

func TestMarkdownUnclosedFenceRunsToEnd(t *testing.T) {
	l := parseMD(t, "# t\n## a\n```\ncmd1\ncmd2\n")
	if len(l) != 1 || l[0].Command != "cmd1;\\\ncmd2" {
		t.Errorf("got %+v", l)
	}
}

func TestMarkdownUnicodeWhitespaceInTitles(t *testing.T) {
	l := parseMD(t, "# t\n##\u00a0file\n```\nx\n```\n")
	if l[0].Name != "file" {
		t.Errorf("name = %q", l[0].Name)
	}
}

func TestMarkdownErrors(t *testing.T) {
	cases := map[string]string{
		"title level skipped":        "# a\n### c\n```\nx\n```\n",
		"code block outside a title": "```\nx\n```\n",
	}
	for want, src := range cases {
		_, err := ParseMarkdown("f.md", strings.NewReader(src))
		pe, ok := err.(*ParseError)
		if !ok || !strings.Contains(pe.Msg, strings.Fields(want)[0]) || pe.File != "f.md" || pe.Line == 0 {
			t.Errorf("%s: err = %v", want, err)
		}
	}
}

func TestYAML(t *testing.T) {
	src := `
nmap:
  name: nmap
  tags: [scan, net]
  const_variables:
    - wordlist: /tmp/w.txt
    - port: 8080
  exec_variables:
    - ip: ip route
  command_tags: [plateform/multiple, cat/RECON]
  cmds:
    a:
      name: scan
      cmd: nmap <target>
      description: desc
      command_tags: [target/remote, cat/ATTACK]
    b:
      name: scan all
      cmd: nmap -p- <target>
other:
  name: no cmds
`
	l, err := ParseYAML("t.yml", strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(l) != 2 {
		t.Fatalf("got %d cheats", len(l))
	}
	a, b := l[0], l[1]
	if a.Name != "scan" || a.Title != "nmap" || a.Tags != "scan, net" || a.Command != "nmap <target>" || a.Description != "desc" {
		t.Errorf("a = %+v", a)
	}
	if v, _ := a.CommandTags.Get("cat"); v != "ATTACK" {
		t.Errorf("command tag override: %v", a.CommandTags)
	}
	if v, _ := b.CommandTags.Get("cat"); v != "RECON" {
		t.Errorf("tool-level tag: %v", b.CommandTags)
	}
	wantVars := map[string]string{"wordlist": "/tmp/w.txt", "port": "8080", "ip": "$(ip route)"}
	if !reflect.DeepEqual(a.Variables, wantVars) {
		t.Errorf("variables = %v", a.Variables)
	}
}

func TestYAMLErrors(t *testing.T) {
	if _, err := ParseYAML("t.yml", strings.NewReader("a: [unclosed")); err == nil {
		t.Error("expected syntax error")
	}
	if _, err := ParseYAML("t.yml", strings.NewReader("a:\n  name: x\n  cmds:\n    c:\n      name: n\n")); err == nil {
		t.Error("expected error for command without cmd")
	}
	if l, err := ParseYAML("t.yml", strings.NewReader("")); err != nil || len(l) != 0 {
		t.Errorf("empty file: %v %v", l, err)
	}
}

func TestTagList(t *testing.T) {
	c := &Cheat{Tags: "recon, #tcp, active directory, RECON"}
	for _, kv := range [][2]string{{"plateform", "linux"}, {"cat", "ATTACK/MITM"}} {
		c.CommandTags.Set(kv[0], kv[1])
	}
	if got := strings.Join(c.TagList(), " "); got != "recon tcp active-directory plateform/linux cat/ATTACK/MITM" {
		t.Errorf("TagList = %q", got)
	}
	if got := strings.Join(c.TagItems(), ","); got != "recon,tcp,active directory,plateform/linux,cat/ATTACK/MITM" {
		t.Errorf("TagItems = %q", got)
	}
}

func writeFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	fence := "```"
	lines := func(l ...string) string { return strings.Join(l, "\n") + "\n" }
	files := map[string]string{
		"broken.md":        lines("# broken", "", "### level skipped", fence, "never loaded", fence),
		"README.md":        lines("# readme", "## x", fence, "should not load", fence),
		"sub/.hidden/h.md": lines("# hidden", "## x", fence, "should not load", fence),
		"nmap.md": lines(
			"# nmap", "", "% scan, network", "", "#plateform/multiple #target/remote #cat/RECON", "",
			"= wordlist: /usr/share/wordlists/example.txt", "$ my_ip: ip route get 1 | awk '{print $7}'", "",
			"## scan specific ports", "Scan the given ports and detect service versions.", fence,
			"nmap -sV -p <ports|22,80> <target>", fence, "",
			"## scan all ports", "#cat/ATTACK", fence+"bash", "nmap -p- -T4 <target>", fence, "",
			"## multi line example", fence, "mkdir -p <outdir|out>", "nmap -oA <outdir|out>/scan <target>;", "echo done", fence, "",
			"## Notes", "This entry has only a description and becomes the command itself.", "",
			"# curl", "", "## download a file", fence, "curl -O <url|http://example.local/file.txt>", fence),
		"tools.yml": lines(
			"nmap:", "  name: nmap", "  tags: [scan, network]", "  const_variables:", "    - wordlist: /usr/share/wordlists/example.txt",
			"  exec_variables:", "    - my_ip: ip route get 1 | awk '{print $7}'", "  command_tags:", "    - plateform/multiple", "    - cat/RECON",
			"  cmds:", "    ports:", "      name: scan specific ports", "      cmd: nmap -sV -p <ports|22,80> <target>",
			"      description: Scan the given ports", "      command_tags:", "        - target/remote", "    all:",
			"      name: scan all ports", "      cmd: nmap -p- <target>", "      description: \"\"", "skipped:", "  name: no cmds here"),
	}
	for rel, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadCheatsFolder(t *testing.T) {
	dir := writeFixtures(t)
	res := Load([]string{dir, filepath.Join(dir, "does-not-exist")})

	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0].Error(), "broken.md:3") {
		t.Errorf("warnings = %v", res.Warnings)
	}
	names := map[string]bool{}
	for _, c := range res.Cheats {
		names[c.Key()] = true
		if strings.Contains(c.Command, "should not load") {
			t.Errorf("loaded an excluded or hidden file: %+v", c)
		}
	}
	for _, want := range []string{"nmapscan specific ports", "nmapscan all ports", "curldownload a file"} {
		if !names[want] {
			t.Errorf("missing %q; have %v", want, names)
		}
	}
	count := 0
	for _, c := range res.Cheats {
		if c.Key() == "nmapscan specific ports" {
			count++
			if !strings.HasSuffix(c.Filename, "tools.yml") {
				t.Errorf("later file should win, got %s", c.Filename)
			}
		}
	}
	if count != 1 {
		t.Errorf("duplicate key loaded %d times", count)
	}
}

func TestMarkdownSkipsHTMLCommentBlocks(t *testing.T) {
	src := "# T\n\n<!-- one line -->\n<!--\n## Hidden\n% x\n```\nrm -rf /\n```\n-->\n\n## Shown\n\n```\nid\n```\n"
	list, err := ParseMarkdown("c.md", strings.NewReader(src))
	if err != nil || len(list) != 1 || list[0].Name != "Shown" || list[0].Command != "id" {
		t.Fatalf("got %+v, %v", list, err)
	}
	src = "# T\n\n## A\n\n```\n<!-- not a comment\nid\n```\n"
	list, err = ParseMarkdown("c.md", strings.NewReader(src))
	if err != nil || len(list) != 1 || !strings.Contains(list[0].Command, "<!--") {
		t.Fatalf("code block: %+v, %v", list, err)
	}
}
