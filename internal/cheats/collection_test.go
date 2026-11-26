package cheats

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"nmap":                "nmap",
		"psexec":              "psexec",
		"Active Directory":    "active-directory",
		"Linux, Network":      "linux-network",
		"  My cheats ":        "my-cheats",
		"impacket-psexec":     "impacket-psexec",
		"C++ / .NET":          "c-net",
		"":                    "cheats",
		"???":                 "cheats",
		"Rubeus_Kerberoast 2": "rubeus-kerberoast-2",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
	if got := CommandFile("/c", "psexec.py <user>@<target>"); got != "/c/psexec.md" {
		t.Errorf("CommandFile = %q", got)
	}
	if got := CommandFile("/c", "<tool> --help"); got != "/c/my-cheats.md" {
		t.Errorf("CommandFile = %q", got)
	}
}

func collection(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "cheats")
}

func load(t *testing.T, dir string) []*Cheat {
	t.Helper()
	res := Load([]string{dir})
	for _, w := range res.Warnings {
		t.Errorf("%v", w)
	}
	return res.Cheats
}

func packDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	md := "# nmap\n\n% scan\n\n#plateform/linux #cat/RECON\n\n## scan ports\nScan the given ports.\n\n```\nnmap -p <ports|22,80> <target>\n```\n\n## all ports\n\n```\nnmap -p- <target>\n```\n\n# Web\n\n= wordlist: /usr/share/wordlists/dirb/common.txt\n$ iface: ip route | awk '{print $5}'\n\n## brute force dirs\n\n```\ngobuster dir -u <url> -w <wordlist> -i <iface>\n```\n"
	yml := "curl:\n  name: curl\n  tags: [http]\n  cmds:\n    headers:\n      name: fetch headers\n      cmd: curl -I <url>\n      description: Only the headers\n"
	for name, text := range map[string]string{"nmap.md": md, "curl.yml": yml} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestAddEntriesFromAPackGroupsByToolAndTagsThem(t *testing.T) {
	dir := collection(t)
	pack := Load([]string{packDir(t)})
	if len(pack.Warnings) != 0 {
		t.Fatal(pack.Warnings)
	}
	res, err := AddEntries(dir, Additions(pack.Cheats, true))
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 4 || res.Files != 3 {
		t.Fatalf("%+v", res)
	}
	for _, f := range []string{"nmap.md", "web.md", "curl.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%v", err)
		}
	}

	got := entriesByName(load(t, dir))
	for _, c := range got {
		if !c.FromArsenal() {
			t.Errorf("%s %v", c.Name, c.CommandTags)
		}
	}
	scan := got["scan ports"]
	if scan.Title != "nmap" || scan.Command != "nmap -p <ports|22,80> <target>" || scan.Description != "Scan the given ports." ||
		scan.Tags != "scan" {
		t.Errorf("%+v", scan)
	}
	if v, _ := scan.CommandTags.Get("cat"); v != "RECON" {
		t.Errorf("%v", scan.CommandTags)
	}
	if v, _ := scan.CommandTags.Get("plateform"); v != "linux" {
		t.Errorf("%v", scan.CommandTags)
	}
	brute := got["brute force dirs"]
	if brute.Variables["wordlist"] != "/usr/share/wordlists/dirb/common.txt" || brute.Variables["iface"] != "$(ip route | awk '{print $5}')" {
		t.Errorf("%v", brute.Variables)
	}
	if got["fetch headers"].Title != "curl" || got["fetch headers"].Tags != "http" || got["fetch headers"].Description != "Only the headers" {
		t.Errorf("%+v", got["fetch headers"])
	}
}

func TestAddEntriesSkipsWhatIsThereAndKeepsTheRest(t *testing.T) {
	dir := collection(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	mine := "# nmap\n\n## scan ports\n% mine\n\n```\nnmap -p <ports|22,80> <target>\n```\n\n## all ports\n\n```\nnmap -p 1-1000 <target>\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "nmap.md"), []byte(mine), 0o600); err != nil {
		t.Fatal(err)
	}
	pack := Load([]string{packDir(t)}).Cheats
	res, err := AddEntries(dir, Additions(pack, true))
	if err != nil {
		t.Fatal(err)
	}
	if res.Added != 3 || res.Files != 3 {
		t.Fatalf("%+v", res)
	}
	got := entriesByName(load(t, dir))
	if got["scan ports"].FromArsenal() || got["scan ports"].Tags != "mine" || got["all ports"].FromArsenal() {
		t.Errorf("%+v %+v", got["scan ports"], got["all ports"])
	}
	if c := got["all ports (2)"]; c == nil || !c.FromArsenal() || c.Command != "nmap -p- <target>" {
		t.Errorf("%+v", c)
	}
	text := readFile(t, filepath.Join(dir, "nmap.md"))
	if !strings.HasPrefix(text, mine[:strings.Index(mine, "## all ports")]) {
		t.Errorf("%s", text)
	}
	if fi, _ := os.Stat(filepath.Join(dir, "nmap.md")); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", fi.Mode().Perm())
	}
	again, err := AddEntries(dir, Additions(pack, true))
	if err != nil || again.Added != 0 || again.Files != 0 {
		t.Errorf("%+v %v", again, err)
	}
	if after := readFile(t, filepath.Join(dir, "nmap.md")); after != text {
		t.Error(after)
	}
}

func TestAddEntriesTagsDoNotLeakBetweenEntries(t *testing.T) {
	dir := collection(t)
	items := []Addition{
		{"nmap", Entry{Title: "a", Command: "nmap a", Tags: "x"}},
		{"nmap", Entry{Title: "b", Command: "nmap b"}},
		{"nmap", Entry{Title: "c", Command: "nmap c", Tags: "y, src/arsenal"}},
	}
	if _, err := AddEntries(dir, items); err != nil {
		t.Fatal(err)
	}
	got := entriesByName(load(t, dir))
	if got["a"].Tags != "x" || got["b"].Tags != "" || got["c"].Tags != "y" || !got["c"].FromArsenal() || got["b"].FromArsenal() {
		t.Errorf("%+v %+v %+v", got["a"], got["b"], got["c"])
	}
}

func TestAddEntriesWritesNothingWhenAFileHasErrors(t *testing.T) {
	dir := collection(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "curl.md"), []byte("### no title above\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	items := []Addition{
		{"nmap", Entry{Title: "a", Command: "nmap a"}},
		{"curl", Entry{Title: "b", Command: "curl b"}},
	}
	if _, err := AddEntries(dir, items); err == nil {
		t.Fatal("no error")
	}
	if _, err := os.Stat(filepath.Join(dir, "nmap.md")); err == nil {
		t.Error(dir)
	}
}

func TestCountAndRemoveArsenalKeepYourEntries(t *testing.T) {
	dir := collection(t)
	items := []Addition{
		{"nmap", Entry{Title: "mine one", Command: "nmap one", Tags: "x"}},
		{"nmap", Entry{Title: "official one", Command: "nmap two", Tags: "recon, src/arsenal"}},
		{"nmap", Entry{Title: "mine two", Command: "nmap three"}},
		{"nmap", Entry{Title: "official two", Command: "nmap four", Tags: "src/arsenal"}},
		{"curl", Entry{Title: "official three", Command: "curl a", Tags: "src/arsenal"}},
		{"web", Entry{Title: "official four", Command: "gobuster b", Tags: "src/arsenal"}},
		{"web", Entry{Title: "mine three", Command: "gobuster c"}},
	}
	if _, err := AddEntries(dir, items); err != nil {
		t.Fatal(err)
	}
	if a, o := Count(dir); a != 4 || o != 3 {
		t.Fatalf("Count = %d, %d", a, o)
	}
	before := entriesByName(load(t, dir))

	res, err := RemoveArsenal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if res.Removed != 4 || res.Kept != 3 {
		t.Fatalf("%+v", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "curl.md")); err == nil {
		t.Error(dir)
	}
	got := entriesByName(load(t, dir))
	if len(got) != 3 {
		t.Fatalf("%v", names(load(t, dir)))
	}
	for _, name := range []string{"mine one", "mine two", "mine three"} {
		c := got[name]
		if c == nil || c.Command != before[name].Command || c.Tags != before[name].Tags || c.FromArsenal() {
			t.Errorf("%s: %+v, was %+v", name, c, before[name])
		}
	}
	if a, o := Count(dir); a != 0 || o != 3 {
		t.Errorf("Count = %d, %d", a, o)
	}
	if again, err := RemoveArsenal(dir); err != nil || again.Removed != 0 || again.Kept != 3 {
		t.Errorf("%+v %v", again, err)
	}
}

func TestRemoveArsenalDeletesFilesThatHoldNothingElse(t *testing.T) {
	dir := collection(t)
	if _, err := AddEntries(dir, Additions(Load([]string{packDir(t)}).Cheats, true)); err != nil {
		t.Fatal(err)
	}
	res, err := RemoveArsenal(dir)
	if err != nil || res.Removed != 4 || res.Kept != 0 {
		t.Fatalf("%+v %v", res, err)
	}
	left, _ := os.ReadDir(dir)
	if len(left) != 0 {
		t.Errorf("%v", left)
	}
}
