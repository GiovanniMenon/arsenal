// Tests for variable defaults when commands are added to the collection.

package cheats

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func resolved(c *Cheat, name string) string {
	for _, a := range NewCommand(c, nil).Args {
		if a.Name == name {
			return a.Value
		}
	}
	return "?"
}

func fromSource(wordlist, title string) Addition {
	return Addition{Group: "web", Entry: Entry{
		Title:   title,
		Command: "gobuster dir -u <url> -w <wordlist>",
		Vars:    []string{"= wordlist: " + wordlist},
		Tags:    "src/arsenal",
	}}
}

func TestDifferentFileDefaultsAreWrittenInlineSoNeitherWins(t *testing.T) {
	dir := collection(t)
	res, err := AddEntries(dir, []Addition{fromSource("/a/words.txt", "dirs one"), fromSource("/b/words.txt", "dirs two")})
	if err != nil || res.Added != 2 || len(res.Conflicts) != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	got := entriesByName(load(t, dir))
	one, two := got["dirs one"], got["dirs two"]
	if one.Command != "gobuster dir -u <url> -w <wordlist>" || two.Command != "gobuster dir -u <url> -w <wordlist|/b/words.txt>" {
		t.Errorf("%q / %q", one.Command, two.Command)
	}
	if resolved(one, "wordlist") != "/a/words.txt" || resolved(two, "wordlist") != "/b/words.txt" {
		t.Errorf("%q / %q", resolved(one, "wordlist"), resolved(two, "wordlist"))
	}
	text := readFile(t, filepath.Join(dir, "web.md"))
	if strings.Count(text, "= wordlist:") != 1 {
		t.Errorf("%s", text)
	}

	again, err := AddEntries(dir, []Addition{fromSource("/a/words.txt", "dirs one"), fromSource("/b/words.txt", "dirs two")})
	if err != nil || again.Added != 0 || len(again.Conflicts) != 0 {
		t.Errorf("%+v %v", again, err)
	}
	if after := readFile(t, filepath.Join(dir, "web.md")); after != text {
		t.Error(after)
	}
	if _, err := AddEntries(dir, []Addition{fromSource("/a/words.txt", "dirs three")}); err != nil {
		t.Fatal(err)
	}
	if c := entriesByName(load(t, dir))["dirs three"]; c.Command != "gobuster dir -u <url> -w <wordlist>" {
		t.Errorf("%q", c.Command)
	}
}

func TestInlineDefaultIsKeptWhenTheCommandAlreadyHasOne(t *testing.T) {
	dir := collection(t)
	first := fromSource("/a/words.txt", "dirs one")
	second := fromSource("/b/words.txt", "dirs two")
	second.Entry.Command = "gobuster dir -u <url> -w <wordlist|/own.txt>"
	if _, err := AddEntries(dir, []Addition{first, second}); err != nil {
		t.Fatal(err)
	}
	if c := entriesByName(load(t, dir))["dirs two"]; c.Command != "gobuster dir -u <url> -w <wordlist|/own.txt>" {
		t.Errorf("%q", c.Command)
	}
}

func TestDefaultsThatCannotBeInlinedAreReportedNotLost(t *testing.T) {
	dir := collection(t)
	a := Addition{Group: "net", Entry: Entry{Title: "one", Command: "ping <iface>", Vars: []string{"$ iface: ip route | awk '{print $5}'"}}}
	b := Addition{Group: "net", Entry: Entry{Title: "two", Command: "ping -c 1 <iface>", Vars: []string{"= iface: eth0 or wlan0"}}}
	res, err := AddEntries(dir, []Addition{a, b})
	if err != nil || res.Added != 2 {
		t.Fatalf("%+v %v", res, err)
	}
	if len(res.Conflicts) != 1 || res.Conflicts[0] != "net.md: iface" {
		t.Errorf("%v", res.Conflicts)
	}
	if again, _ := AddEntries(dir, []Addition{a, b}); again.Added != 0 || len(again.Conflicts) != 0 {
		t.Errorf("%+v", again)
	}
}

func TestExistingFileDefaultIsRespectedByNewEntries(t *testing.T) {
	dir := collection(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "# web\n\n= wordlist: /mine.txt\n\n## mine\n\n```\ngobuster dir -w <wordlist>\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "web.md"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := AddEntries(dir, []Addition{fromSource("/a/words.txt", "dirs one")}); err != nil {
		t.Fatal(err)
	}
	got := entriesByName(load(t, dir))
	if resolved(got["mine"], "wordlist") != "/mine.txt" || resolved(got["dirs one"], "wordlist") != "/a/words.txt" {
		t.Errorf("%q / %q", resolved(got["mine"], "wordlist"), resolved(got["dirs one"], "wordlist"))
	}
}
