package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/tui"
)

func isolateAdd(t *testing.T) (home string) {
	t.Helper()
	isolate(t)
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HISTFILE", filepath.Join(home, "hist"))
	t.Cleanup(func() { runSteps = tui.RunSteps })
	runSteps = func(tui.StepBox) (tui.Answers, error) {
		t.Fatal("unexpected step box")
		return nil, nil
	}
	return home
}

func realPath(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAddWithFlagsWritesToTheFileOfTheTool(t *testing.T) {
	isolateAdd(t)
	out, code := run(t, "add", "nmap -sV -p <ports|22,80> <target>", "--title", "scan ports",
		"--description", "Scan ports of a host", "--tags", "nmap,recon")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	file := realPath(t, filepath.Join(cheatsDirT(t), "nmap.md"))
	if !strings.Contains(out, "Added") || !strings.Contains(out, "nmap.md") {
		t.Errorf("output = %q", out)
	}
	res := cheats.Load([]string{cheatsDirT(t)})
	if len(res.Cheats) != 1 || res.Cheats[0].Filename != file || res.Cheats[0].Name != "scan ports" || res.Cheats[0].Tags != "nmap, recon" {
		t.Fatalf("loaded %+v (%v)", res.Cheats, res.Warnings)
	}
	before, _ := os.ReadFile(file)
	if out, code := run(t, "add", "nmap -p- <target>", "--title", "scan ports"); code == 0 {
		t.Errorf("duplicate accepted: %s", out)
	}
	if after, _ := os.ReadFile(file); string(after) != string(before) {
		t.Error("file changed by a refused duplicate")
	}
}

func TestAddLastUsesTheShellHistory(t *testing.T) {
	home := isolateAdd(t)
	os.WriteFile(filepath.Join(home, "hist"), []byte(": 1:0;curl -I http://example.local\n: 2:0;ars add --last\n"), 0o600)
	if out, code := run(t, "add", "--last", "--title", "headers"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	res := cheats.Load([]string{cheatsDirT(t)})
	if len(res.Cheats) != 1 || res.Cheats[0].Command != "curl -I http://example.local" {
		t.Fatalf("loaded %+v", res.Cheats)
	}
	os.WriteFile(filepath.Join(home, "hist"), nil, 0o600)
	if out, code := run(t, "add", "--last", "--title", "x"); code == 0 || !strings.Contains(out, "history") {
		t.Errorf("empty history: code %d: %s", code, out)
	}
}

func TestAddNeedsInputWhenNotInteractive(t *testing.T) {
	isolateAdd(t)
	if out, code := run(t, "add"); code == 0 || !strings.Contains(out, "--title") {
		t.Errorf("code %d: %s", code, out)
	}
	if out, code := run(t, "add", "x"); code == 0 {
		t.Errorf("missing title accepted: %s", out)
	}
	if out, code := run(t, "add", "x", "--title", "t", "--file", "notes.yml"); code == 0 {
		t.Errorf("yml accepted: %s", out)
	}
}

func TestResolveCheatFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for in, want := range map[string]string{
		"my.md":        "/c/my.md",
		"sub/a.md":     "sub/a.md",
		"/abs/x.md":    "/abs/x.md",
		"~/notes/x.md": filepath.Join(home, "notes", "x.md"),
	} {
		if got, err := resolveCheatFile(in, "/c"); err != nil || got != want {
			t.Errorf("resolveCheatFile(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestAddBoxAsksFourStepsPrefilledFromHistory(t *testing.T) {
	home := isolateAdd(t)
	os.WriteFile(filepath.Join(home, "hist"), []byte("nmap -p- 10.10.10.5\nars add\nars -s nmap\narsenal notes\n./bin/ars init acme\n"), 0o600)
	isInteractive = func() bool { return true }
	s := newStepScript(t, map[string]any{
		"title": "scan every port", "description": "Scan ports of one host\nall of them", "tags": "nmap, recon",
	})
	out, code := run(t, "add")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if got := strings.Join(s.asked, ","); got != "command,title,description,tags" {
		t.Errorf("asked = %s", got)
	}
	cmd := s.steps["command"]
	if cmd.Default != "nmap -p- 10.10.10.5" || !strings.Contains(cmd.Help, "history") {
		t.Errorf("command step = %+v", cmd)
	}
	if s.steps["description"].Kind != tui.StepMultiline {
		t.Errorf("the description must be multi-line")
	}
	if s.box.Title == "" {
		t.Error("the box has no title")
	}
	sum := s.summaryText()
	for _, want := range []string{"## scan every port", "% nmap, recon", "Scan ports of one host", "nmap -p- 10.10.10.5", "Saves to nmap.md"} {
		if !strings.Contains(sum, want) {
			t.Errorf("summary lacks %q:\n%s", want, sum)
		}
	}
	if done := strings.Join(s.box.Confirmed(tui.Answers{
		"command": {Text: "x"}, "title": {Text: "t"}, "description": {}, "tags": {},
	}), "\n"); !strings.Contains(done, "Saved to x.md") {
		t.Errorf("%s", done)
	}
	res := cheats.Load([]string{cheatsDirT(t)})
	if len(res.Cheats) != 1 || res.Cheats[0].Filename != realPath(t, filepath.Join(cheatsDirT(t), "nmap.md")) ||
		res.Cheats[0].Command != "nmap -p- 10.10.10.5" || res.Cheats[0].Name != "scan every port" {
		t.Fatalf("loaded %+v", res.Cheats)
	}
}

func TestAddArgumentPrefillsTheCommand(t *testing.T) {
	home := isolateAdd(t)
	os.WriteFile(filepath.Join(home, "hist"), []byte("echo from-history\n"), 0o600)
	isInteractive = func() bool { return true }
	s := newStepScript(t, map[string]any{"title": "scan versions"})
	if out, code := run(t, "add", "nmap -sV <target>"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if got := s.steps["command"].Default; got != "nmap -sV <target>" {
		t.Errorf("command prefill = %q, want the argument (not the history)", got)
	}
	s = newStepScript(t, map[string]any{"title": "curl headers"})
	if out, code := run(t, "add", "--", "curl", "-I", "http://example.local"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if got := s.steps["command"].Default; got != "curl -I http://example.local" {
		t.Errorf("command prefill = %q", got)
	}
}

func TestAddArgumentWithATitleNeedsNoBox(t *testing.T) {
	isolateAdd(t)
	if out, code := run(t, "add", "nmap -sV <target>", "--title", "scan versions"); code != 0 || !strings.Contains(out, "Added") {
		t.Fatalf("code %d: %s", code, out)
	}
	res := cheats.Load([]string{cheatsDirT(t)})
	if len(res.Cheats) != 1 || res.Cheats[0].Command != "nmap -sV <target>" {
		t.Fatalf("loaded %+v", res.Cheats)
	}
}

func TestAddTitleStepRefusesADuplicateBeforeTheSummary(t *testing.T) {
	isolateAdd(t)
	if out, code := run(t, "add", "id", "--title", "who am i"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	isInteractive = func() bool { return true }
	s := newStepScript(t, map[string]any{"title": "other title"})
	if out, code := run(t, "add", "id -u"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	title := s.steps["title"]
	same := tui.Answers{"command": {Text: "id -u"}}
	if err := title.ValidateWith(same, "who am i"); err == nil || !strings.Contains(err.Error(), "id.md already has an entry") {
		t.Errorf("%v", err)
	}
	if err := title.ValidateWith(tui.Answers{"command": {Text: "whoami"}}, "who am i"); err != nil {
		t.Errorf("%v", err)
	}
	if err := title.Validate(""); err == nil {
		t.Error(err)
	}
	if err := title.ValidateWith(same, "something new"); err != nil {
		t.Errorf("%v", err)
	}
	if s.steps["command"].Validate("") == nil {
		t.Error("an empty command was accepted")
	}
}

func TestAddCancelledWritesNothing(t *testing.T) {
	isolateAdd(t)
	isInteractive = func() bool { return true }
	s := newStepScript(t, nil)
	s.cancel = true
	out, code := run(t, "add", "id")
	if code != 130 || !strings.Contains(out, "Cancelled") {
		t.Errorf("code %d: %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(cheatsDirT(t), "id.md")); err == nil {
		t.Error(cheatsDirT(t))
	}
}

func TestAddFileFlagIsAdvancedAndShownInTheSummary(t *testing.T) {
	isolateAdd(t)
	isInteractive = func() bool { return true }
	s := newStepScript(t, map[string]any{"command": "id", "title": "who am i"})
	if out, code := run(t, "add", "--file", "work.md"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if !strings.Contains(s.summaryText(), "Saves to work.md") {
		t.Errorf("summary:\n%s", s.summaryText())
	}
	if _, err := os.Stat(filepath.Join(cheatsDirT(t), "work.md")); err != nil {
		t.Errorf("work.md not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cheatsDirT(t), "my.md")); err == nil {
		t.Error("my.md written although --file was given")
	}
}
