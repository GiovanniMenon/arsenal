package cheats

import (
	"reflect"
	"testing"
)

func TestCheck(t *testing.T) {
	mk := func(file, name, command string) *Cheat {
		return &Cheat{Filename: file, Name: name, Command: command}
	}
	list := []*Cheat{
		mk("/c/nmap.md", "scan", "nmap -sV <target>"),
		mk("/c/nmap.md", "scan again", "nmap -p- <target>"),
		mk("/c/tools.md", "missing", "not-installed --help"),
		mk("/c/win.md", "exe", "mimikatz.exe privilege::debug"),
		mk("/c/ps.md", "run powershell thing", "iex (x)"),
		mk("/c/windows.md", "skipped file", "dir"),
		mk("/c/laps.md", "skipped file", "Get-X"),
		mk("/c/x.md", "placeholder", "<tool> --version"),
		mk("/c/multi.md", "multi", "echo one;\\\necho two"),
	}
	found := map[string]string{"nmap": "nmap is /usr/bin/nmap", "echo": "echo is a shell builtin"}
	res := Check(list, func(b string) (string, bool) { w, ok := found[b]; return w, ok })

	type short struct {
		Binary string
		Status CheckStatus
	}
	var got []short
	for _, r := range res {
		got = append(got, short{r.Binary, r.Status})
	}
	want := []short{
		{"nmap", CheckOK},
		{"not-installed", CheckMissing},
		{"mimikatz.exe", CheckSkipped},
		{"iex", CheckSkipped},
		{"echo", CheckOK},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	if res[0].Detail != "nmap is /usr/bin/nmap" {
		t.Errorf("detail = %q", res[0].Detail)
	}
	if res[1].Detail != "not-installed --help" {
		t.Errorf("missing detail should be the full command, got %q", res[1].Detail)
	}
}

func TestShellFinderFindsRealAndMissingCommands(t *testing.T) {
	if _, ok := ShellFinder("sh"); !ok {
		t.Error("sh should be found")
	}
	if _, ok := ShellFinder("definitely-not-a-real-command-xyz"); ok {
		t.Error("nonexistent command found")
	}
}
