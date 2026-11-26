package cli

import (
	"errors"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

const testSheet = `# nmap

% scan

#plateform/linux #target/remote

## scan specific ports
` + "```" + `
nmap -sV -p <ports|22,80> <target>
` + "```" + `

## scan all ports
` + "```" + `
nmap -p- <target>
` + "```" + `
`

type uiScript struct {
	t       *testing.T
	steps   []string
	calls   []tui.SearchOptions
	asked   [][]string
	answers map[string]string
}

func setupSearch(t *testing.T, steps ...string) *uiScript {
	t.Helper()
	isolate(t)
	cheatDir := cheatsDirT(t)
	if err := os.MkdirAll(cheatDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cheatDir, "nmap.md"), []byte(testSheet), 0o600); err != nil {
		t.Fatal(err)
	}

	s := &uiScript{t: t, steps: steps, answers: map[string]string{}}
	runSearchUI = func(o tui.SearchOptions) (tui.SearchResult, error) {
		s.calls = append(s.calls, o)
		if len(s.steps) == 0 {
			t.Fatalf("unexpected extra search screen")
		}
		name := s.steps[0]
		s.steps = s.steps[1:]
		res := tui.SearchResult{Query: o.Query, Position: o.Position}
		if name == "" {
			return res, nil
		}
		for _, c := range o.Cheats {
			if c.Name == name {
				res.Cheat = c
				cmd := cheats.NewCommand(c, o.Vars.Vars)
				if idx := cmd.NeedsInput(); len(idx) > 0 {
					var names []string
					res.Values = map[string]string{}
					for _, i := range idx {
						n := cmd.Args[i].Name
						names = append(names, n)
						if v, ok := s.answers[n]; ok {
							res.Values[n] = v
						}
					}
					s.asked = append(s.asked, names)
				}
				return res, nil
			}
		}
		t.Fatalf("no cheat named %q", name)
		return res, nil
	}
	t.Cleanup(func() {
		runSearchUI = tui.RunSearch
	})
	return s
}

func TestSearchAsksOnlyForMissingVariables(t *testing.T) {
	s := setupSearch(t, "scan specific ports")
	inSession(t, "acme")
	run(t, "set", "target=10.10.10.5")
	s.answers["ports"] = "22,80"

	out, code := run(t, "-p")
	if code != 0 || out != "nmap -sV -p 22,80 10.10.10.5\n" {
		t.Fatalf("code %d: %q", code, out)
	}
	if len(s.asked) != 1 || len(s.asked[0]) != 1 || s.asked[0][0] != "ports" {
		t.Errorf("asked = %v, want only ports", s.asked)
	}
	if s.calls[0].Session != "acme" || !s.calls[0].ShowTags {
		t.Errorf("options = %+v", s.calls[0])
	}
	if s.calls[0].Vars.Source["target"] != session.SourceSession {
		t.Errorf("sources = %v", s.calls[0].Vars.Source)
	}
}

func TestSearchFlagPrefillsQueryAndNoTags(t *testing.T) {
	s := setupSearch(t, "")
	if out, code := run(t, "-s", "nmap", "--no-tags"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if s.calls[0].Query != "nmap" || s.calls[0].ShowTags {
		t.Errorf("options = %+v", s.calls[0])
	}
	s.steps = []string{""}
	run(t, "-s", "curl")
	if s.calls[1].Query != "curl" {
		t.Errorf("query = %q", s.calls[1].Query)
	}
}

func TestPrefixOnlyWithFlag(t *testing.T) {
	s := setupSearch(t, "scan all ports", "scan all ports", "scan all ports")
	run(t, "set", "-g", "arsenal_prefix_cmd=proxychains -q")
	s.answers["target"] = "10.10.10.5"

	if out, _ := run(t, "-p"); out != "nmap -p- 10.10.10.5\n" {
		t.Errorf("without -f: %q", out)
	}
	if s.calls[0].PrefixOn || s.calls[0].Prefix != "proxychains -q" {
		t.Errorf("UI should know the prefix is set but off: %+v", s.calls[0])
	}
	if out, _ := run(t, "-p", "-f"); out != "proxychains -q nmap -p- 10.10.10.5\n" {
		t.Errorf("with -f: %q", out)
	}
	if !s.calls[1].PrefixOn {
		t.Error("UI should show the prefix as active")
	}
}

func TestNoCheatsOpensAnEmptySearch(t *testing.T) {
	isolate(t)
	opened := false
	runSearchUI = func(o tui.SearchOptions) (tui.SearchResult, error) {
		opened = len(o.Cheats) == 0
		return tui.SearchResult{}, nil
	}
	t.Cleanup(func() { runSearchUI = tui.RunSearch })
	if out, code := run(t); code != 0 || !opened {
		t.Fatalf("code %d opened %v: %s", code, opened, out)
	}
}

func TestBrokenCheatFileWarnsAndOthersLoad(t *testing.T) {
	s := setupSearch(t, "")
	broken := filepath.Join(cheatsDirT(t), "broken.md")
	if err := os.WriteFile(broken, []byte("# b\n### skipped\n```\nx\n```\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, "-p")
	if code != 0 || !strings.Contains(out, "broken.md:2") {
		t.Fatalf("code %d: %s", code, out)
	}
	found := false
	for _, c := range s.calls[0].Cheats {
		found = found || c.Name == "scan all ports"
	}
	if !found {
		t.Error("good cheats were not loaded")
	}
}

func TestConfiguredCheatsPath(t *testing.T) {
	s := setupSearch(t, "")
	other := t.TempDir()
	if err := os.WriteFile(filepath.Join(other, "x.md"), []byte("# x\n## only here\n```\necho hi\n```\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, _ := config.ConfigDir()
	_ = os.MkdirAll(dir, 0o700)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("cheats_paths: ["+other+"]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	run(t, "-p")
	names := map[string]bool{}
	for _, c := range s.calls[0].Cheats {
		names[c.Name] = true
	}
	if !names["only here"] || !names["scan all ports"] {
		t.Errorf("names = %v", names)
	}
}

func TestVersionShorthand(t *testing.T) {
	isolate(t)
	if out, code := run(t, "-V"); code != 0 || !strings.Contains(out, "\nars ") || !strings.Contains(out, "▄▄▄") {
		t.Fatalf("code %d: %q", code, out)
	}
}

func TestUIFailureIsReported(t *testing.T) {
	setupSearch(t)
	runSearchUI = func(tui.SearchOptions) (tui.SearchResult, error) {
		return tui.SearchResult{}, errors.New("no tty")
	}
	if out, code := run(t, "-p"); code == 0 || !strings.Contains(out, "needs a terminal") {
		t.Fatalf("code %d: %s", code, out)
	}
}

func TestOutputFlags(t *testing.T) {
	s := setupSearch(t, "scan all ports", "scan all ports", "scan all ports")
	s.answers["target"] = "10.10.10.5"

	f := filepath.Join(t.TempDir(), "cmd.txt")
	if out, code := run(t, "-o", f); code != 0 || out != "" {
		t.Fatalf("-o: code %d: %q", code, out)
	}
	if b, _ := os.ReadFile(f); string(b) != "nmap -p- 10.10.10.5" {
		t.Errorf("file = %q", b)
	}

	bin := t.TempDir()
	clip := filepath.Join(t.TempDir(), "clip")
	script := "#!/bin/sh\n/bin/cat > '" + clip + "'\n"
	for _, tool := range []string{"pbcopy", "xclip"} {
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	if out, code := run(t, "-x"); code != 0 || !strings.Contains(out, "Copied") {
		t.Fatalf("-x: code %d: %q", code, out)
	}
	if b, _ := os.ReadFile(clip); string(b) != "nmap -p- 10.10.10.5" {
		t.Errorf("clipboard = %q", b)
	}
}

func TestPrintWinsOverOtherOutputs(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	f := filepath.Join(t.TempDir(), "never")
	out, _ := run(t, "-p", "-o", f)
	if out != "nmap -p- 10.10.10.5\n" {
		t.Errorf("out = %q", out)
	}
	if _, err := os.Stat(f); err == nil {
		t.Error("-o must not run when -p is given (upstream priority)")
	}
}

func TestExecRunsTheCommand(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	marker := filepath.Join(t.TempDir(), "ran")
	s.answers["target"] = "x"
	orig := runSearchUI
	runSearchUI = func(o tui.SearchOptions) (tui.SearchResult, error) {
		c := &cheats.Cheat{Name: "touch", Command: "touch " + marker}
		return tui.SearchResult{Cheat: c}, nil
	}
	t.Cleanup(func() { runSearchUI = orig })
	if out, code := run(t, "-e"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Error("-e did not run the command")
	}
}

func TestDefaultDeliveryFailureStillShowsTheCommand(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	t.Setenv("PATH", t.TempDir())
	t.Setenv("ARSENAL_CMD_FILE", "")
	out, code := run(t)
	if code == 0 || !strings.Contains(out, "nmap -p- 10.10.10.5") || !strings.Contains(out, "cannot put the command in your prompt") {
		t.Fatalf("code %d: %q", code, out)
	}
}

func TestDefaultDeliveryUsesShellFunctionFile(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	f := filepath.Join(t.TempDir(), "cmdfile")
	t.Setenv("ARSENAL_CMD_FILE", f)
	if out, code := run(t); code != 0 {
		t.Fatalf("code %d: %q", code, out)
	}
	if b, _ := os.ReadFile(f); string(b) != "nmap -p- 10.10.10.5" {
		t.Errorf("cmd file = %q", b)
	}
}

func TestTmuxModeKeepsTheSearchOpen(t *testing.T) {
	s := setupSearch(t, "scan all ports", "scan all ports", "scan all ports", "")
	s.answers["target"] = "10.10.10.5"
	var sent []string
	deliver = func(cmd *cobra.Command, line string, f *searchFlags) (bool, delivery, error) {
		sent = append(sent, line)
		if !f.tmux {
			t.Error("tmux flag not passed")
		}
		return len(sent) < 3, delivery{How: "tmux"}, nil
	}
	t.Cleanup(func() { deliver = deliverCommand })
	if out, code := run(t, "-t"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if len(sent) != 3 || len(s.calls) != 3 {
		t.Errorf("sent %d commands, search opened %d times", len(sent), len(s.calls))
	}
}

func TestTmuxFailureFallsBackToThePrompt(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	t.Setenv("TMUX", "")
	f := filepath.Join(t.TempDir(), "cmdfile")
	t.Setenv("ARSENAL_CMD_FILE", f)
	out, code := run(t, "-t")
	if code != 0 || !strings.Contains(out, "tmux") {
		t.Fatalf("code %d: %q", code, out)
	}
	if b, _ := os.ReadFile(f); string(b) != "nmap -p- 10.10.10.5" {
		t.Errorf("prompt fallback did not get the command: %q", b)
	}
}

func TestCheckFlag(t *testing.T) {
	setupSearch(t)
	checkFinder = func(b string) (string, bool) {
		if b == "nmap" {
			return "nmap is /usr/bin/nmap", true
		}
		return "", false
	}
	t.Cleanup(func() { checkFinder = cheats.ShellFinder })
	out, code := run(t, "-c")
	if code != 0 || !strings.Contains(out, "OK   :") || !strings.Contains(out, "nmap is /usr/bin/nmap") {
		t.Fatalf("code %d: %q", code, out)
	}
}
