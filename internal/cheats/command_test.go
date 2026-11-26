package cheats

import (
	"reflect"
	"testing"
)

func cheat(command string, vars map[string]string) *Cheat {
	return &Cheat{Name: "n", Title: "t", Command: command, Variables: vars}
}

func TestSavedVariableIsPrefilled(t *testing.T) {
	c := NewCommand(cheat("nmap -sV <target>", nil), map[string]string{"target": "10.10.10.5"})
	if !reflect.DeepEqual(c.Args, []Arg{{"target", "10.10.10.5", SourceSaved}}) {
		t.Errorf("args = %+v", c.Args)
	}
}

func TestDefaultValueSyntax(t *testing.T) {
	c := NewCommand(cheat("nmap -p <ports|22,80> <target>", nil), map[string]string{"target": "10.10.10.5"})
	want := []Arg{{"ports", "22,80", SourceDefault}, {"target", "10.10.10.5", SourceSaved}}
	if !reflect.DeepEqual(c.Args, want) {
		t.Errorf("args = %+v", c.Args)
	}
	if c.Template != "nmap -p <ports> <target>" {
		t.Errorf("template = %q", c.Template)
	}
}

func TestBuildWithAllValuesSet(t *testing.T) {
	c := NewCommand(cheat("nmap -sV -p <ports> <target>", nil), map[string]string{"target": "10.10.10.5", "ports": "22,80"})
	got, err := c.Build()
	if err != nil || got != "nmap -sV -p 22,80 10.10.10.5" {
		t.Errorf("Build = %q, %v", got, err)
	}
}

func TestBuildWithMissingValueFails(t *testing.T) {
	c := NewCommand(cheat("nmap -p <ports> <target>", nil), map[string]string{"target": "10.10.10.5"})
	if _, err := c.Build(); err == nil {
		t.Error("expected an error")
	}
	if !reflect.DeepEqual(c.Missing(), []string{"ports"}) {
		t.Errorf("missing = %v", c.Missing())
	}
}

func TestPrecedenceSavedBeatsDefaultBeatsFile(t *testing.T) {
	vars := map[string]string{"a": "file-a", "b": "file-b", "c": "file-c"}
	c := NewCommand(cheat("x <a|def-a> <b|def-b> <c> <d>", vars), map[string]string{"a": "saved-a"})
	want := []Arg{
		{"a", "saved-a", SourceSaved},
		{"b", "def-b", SourceDefault},
		{"c", "file-c", SourceFile},
		{"d", "", SourceNone},
	}
	if !reflect.DeepEqual(c.Args, want) {
		t.Errorf("args = %+v", c.Args)
	}
	if !reflect.DeepEqual(c.NeedsInput(), []int{1, 2, 3}) {
		t.Errorf("needs input = %v", c.NeedsInput())
	}
}

func TestSameNameIsOneArgumentFilledEverywhere(t *testing.T) {
	c := NewCommand(cheat("ping <ip> && curl <ip>:<port|80>/<ip>", nil), nil)
	if len(c.Args) != 2 {
		t.Fatalf("args = %+v", c.Args)
	}
	c.Set("ip", "10.10.10.5")
	got, err := c.Build()
	if err != nil || got != "ping 10.10.10.5 && curl 10.10.10.5:80/10.10.10.5" {
		t.Errorf("Build = %q, %v", got, err)
	}
}

func TestDefaultOnLaterOccurrenceCounts(t *testing.T) {
	c := NewCommand(cheat("a <x> b <x|fallback>", nil), nil)
	if len(c.Args) != 1 || c.Args[0].Value != "fallback" {
		t.Errorf("args = %+v", c.Args)
	}
}

func TestPreviewKeepsMissingPlaceholders(t *testing.T) {
	c := NewCommand(cheat("nmap -p <ports> <target>", nil), map[string]string{"target": "10.10.10.5"})
	if got := c.Preview(); got != "nmap -p <ports> 10.10.10.5" {
		t.Errorf("Preview = %q", got)
	}
}

func TestValuesAreNotReExpanded(t *testing.T) {
	c := NewCommand(cheat("echo <a> <b>", nil), map[string]string{"a": "<b>", "b": "x"})
	if got, _ := c.Build(); got != "echo <b> x" {
		t.Errorf("Build = %q", got)
	}
}

func TestExecutableVariableIsInsertedAsText(t *testing.T) {
	c := NewCommand(cheat("scp <file> <host>:", map[string]string{"host": "$(cat hosts | head -1)"}), nil)
	c.Set("file", "a.txt")
	if got, _ := c.Build(); got != "scp a.txt $(cat hosts | head -1):" {
		t.Errorf("Build = %q", got)
	}
}

func TestCommandWithoutPlaceholders(t *testing.T) {
	c := NewCommand(cheat("ls -la", nil), nil)
	if got, err := c.Build(); err != nil || got != "ls -la" || len(c.NeedsInput()) != 0 {
		t.Errorf("Build = %q, %v", got, err)
	}
	c = NewCommand(cheat("cat < in.txt > out.txt", nil), nil)
	if len(c.Args) != 0 {
		t.Errorf("args = %+v", c.Args)
	}
}

func TestPrefixed(t *testing.T) {
	if got := Prefixed("nmap x", "proxychains -q"); got != "proxychains -q nmap x" {
		t.Error(got)
	}
	if got := Prefixed("nmap x", ""); got != "nmap x" {
		t.Error(got)
	}
}
