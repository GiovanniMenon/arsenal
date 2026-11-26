package history

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseFormats(t *testing.T) {
	cases := map[string]struct {
		text string
		want []string
	}{
		"bash plain":      {"ls\nnmap -sV 10.10.10.5\n", []string{"ls", "nmap -sV 10.10.10.5"}},
		"bash timestamps": {"#1700000000\nls\n#1700000001\nid\n", []string{"ls", "id"}},
		"zsh extended": {": 1700000000:0;ls -la\n: 1700000005:2;curl http://example.local\n",
			[]string{"ls -la", "curl http://example.local"}},
		"zsh multi-line": {": 1700000000:0;echo a \\\nb\n: 1700000009:0;id\n", []string{"echo a \\\nb", "id"}},
		"fish":           {"- cmd: ls\n  when: 1700000000\n- cmd: echo a\\nb\n  when: 1700000001\n", []string{"ls", "echo a\nb"}},
	}
	for name, c := range cases {
		if got := Parse(c.text); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestLastSkipsArsenalCommandsAndBlank(t *testing.T) {
	p := filepath.Join(t.TempDir(), "h")
	os.WriteFile(p, []byte(": 1:0;nmap -p- 10.10.10.5\n: 2:0;ars add\n: 3:0;   \n: 4:0;command arsenal add --last\n: 5:0;ars -s nmap\n"), 0o600)
	got, err := Last(p, IsArsenalCmd)
	if err != nil || got != "nmap -p- 10.10.10.5" {
		t.Errorf("Last = %q, %v", got, err)
	}
	if _, err := Last(filepath.Join(t.TempDir(), "missing"), nil); err != ErrNone {
		t.Errorf("missing file: %v", err)
	}
	os.WriteFile(p, []byte("ars add\n"), 0o600)
	if _, err := Last(p, IsArsenalCmd); err != ErrNone {
		t.Errorf("only ars add: %v", err)
	}
}

func TestLastReadsOnlyTheTailOfAHugeFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "h")
	big := strings.Repeat("echo filler line\n", 40000) + "whoami\n"
	os.WriteFile(p, []byte(big), 0o600)
	if got, err := Last(p, nil); err != nil || got != "whoami" {
		t.Errorf("Last = %q, %v", got, err)
	}
}

func TestFilePrefersHISTFILEThenShell(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("ZDOTDIR", "")
	t.Setenv("HISTFILE", "/x/y")
	if File() != "/x/y" {
		t.Errorf("HISTFILE ignored: %s", File())
	}
	t.Setenv("HISTFILE", "")
	t.Setenv("SHELL", "/bin/zsh")
	os.WriteFile(filepath.Join(home, ".zsh_history"), nil, 0o600)
	os.WriteFile(filepath.Join(home, ".bash_history"), nil, 0o600)
	if File() != filepath.Join(home, ".zsh_history") {
		t.Errorf("zsh: %s", File())
	}
	t.Setenv("SHELL", "/bin/bash")
	if File() != filepath.Join(home, ".bash_history") {
		t.Errorf("bash: %s", File())
	}
}

func TestIsArsenalCmd(t *testing.T) {
	for c, want := range map[string]bool{
		"ars add": true, "arsenal add --last": true, "command ars add": true, "ars": true, "arsenal": true,
		"ars -s add": true, "ars init acme": true, "./bin/ars note x": true, "/usr/local/bin/arsenal": true, "sudo ars rec": true,
		"echo ars add": false, "ars-tool": false, "ars2 x": false, "nmap -sV ars": false, "": false,
	} {
		if IsArsenalCmd(c) != want {
			t.Errorf("IsArsenalCmd(%q) = %v", c, !want)
		}
	}
}
