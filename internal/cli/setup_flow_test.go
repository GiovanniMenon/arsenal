// Walks through the ars setup questions with scripted answers.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ynn3m/arsenal"
	"github.com/ynn3m/arsenal/internal/cheats"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/setup"
	"github.com/ynn3m/arsenal/internal/tui"
)

type stepScript struct {
	t       *testing.T
	answers map[string]any
	cancel  bool

	runs    int
	box     tui.StepBox
	asked   []string
	steps   map[string]tui.Step
	summary []string
}

func newStepScript(t *testing.T, answers map[string]any) *stepScript {
	s := &stepScript{t: t, answers: answers}
	runSteps = func(b tui.StepBox) (tui.Answers, error) {
		s.runs++
		s.box, s.asked, s.steps, s.summary = b, nil, map[string]tui.Step{}, nil
		if s.cancel {
			return nil, tui.ErrCancelled
		}
		ans := tui.Answers{}
		for _, st := range b.Steps {
			if st.When != nil && !st.When(ans) {
				continue
			}
			s.asked = append(s.asked, st.Key)
			s.steps[st.Key] = st
			v, scripted := s.answers[st.Key]
			if st.Kind == tui.StepChoice {
				idx := st.DefaultOption
				if scripted {
					idx = v.(int)
				}
				ans[st.Key] = tui.Answer{Text: st.Options[idx], Index: idx}
				continue
			}
			text := st.Default
			if scripted {
				text = v.(string)
			}
			if st.Validate != nil {
				if err := st.Validate(text); err != nil {
					t.Fatalf("answer %q to %q fails validation: %v", text, st.Key, err)
				}
			}
			if st.ValidateWith != nil {
				if err := st.ValidateWith(ans, text); err != nil {
					t.Fatalf("%q %q %v", text, st.Key, err)
				}
			}
			ans[st.Key] = tui.Answer{Text: text, Index: -1}
		}
		if b.Summary != nil {
			s.summary = b.Summary(ans)
		}
		return ans, nil
	}
	t.Cleanup(func() {
		runSteps = tui.RunSteps
		officialPack = arsenal.Cheats()
		isInteractive = tui.IsInteractive
	})
	return s
}

func (s *stepScript) summaryText() string { return strings.Join(s.summary, "\n") }

func (s *stepScript) boxText() string {
	var b strings.Builder
	b.WriteString(s.box.Title + "\n")
	for _, st := range s.steps {
		b.WriteString(st.Title + "\n" + st.Help + "\n" + strings.Join(st.Options, "\n") + "\n")
	}
	b.WriteString(s.summaryText())
	return b.String()
}

func (s *stepScript) askedKey(k string) bool {
	for _, a := range s.asked {
		if a == k {
			return true
		}
	}
	return false
}

func TestSetupBoxAsksTheFourQuestionsInOrder(t *testing.T) {
	home := isolateSetup(t)
	s := newStepScript(t, map[string]any{"folder": "~/ops/pentest", "cheats": cheatsKeep, "rec": 1, "shell": 0})
	pt := filepath.Join(home, "ops", "pentest")
	out, code := run(t, "setup")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if s.box.Title != "Setup" {
		t.Errorf("box title = %q", s.box.Title)
	}
	if got := strings.Join(s.asked, ","); got != "folder,cheats,rec,shell" {
		t.Errorf("asked = %s", got)
	}
	st := s.steps
	if st["folder"].Kind != tui.StepPath || st["folder"].Default != "~/Documents/pentest" {
		t.Errorf("folder step = %+v", st["folder"])
	}
	if got := st["cheats"].Options; len(got) != 2 || got[0] != "Keep as it is" || got[1] != "Add the Arsenal commands (~1100)" {
		t.Errorf("cheats options = %q", got)
	}
	if got := st["cheats"].Help; got != "No commands yet. Add your own later with ars add" {
		t.Errorf("%q", got)
	}
	if got := st["rec"].Options; len(got) != 2 || got[0] != "Only when I run ars rec" || got[1] != "Always, on every ars init" {
		t.Errorf("recording options = %q", got)
	}
	if st["rec"].DefaultOption != 0 {
		t.Errorf("recording must start on the first option, got %d", st["rec"].DefaultOption)
	}
	if got := st["shell"].Options; len(got) != 2 || got[0] != "Add to ~/.zshrc" || got[1] != "Skip" {
		t.Errorf("shell options = %q", got)
	}
	if strings.Contains(s.boxText(), "my.md") {
		t.Errorf("the box must not mention my.md:\n%s", s.boxText())
	}

	sum := s.summaryText()
	last := -1
	for _, want := range []string{"Pentest folder", "~/ops/pentest", "Cheats", "Recording", "Always, on every ars init", "Shell", "Add to ~/.zshrc"} {
		i := strings.Index(sum, want)
		if i < 0 || i < last {
			t.Errorf("summary lacks %q (or out of order):\n%s", want, sum)
		}
		last = max(last, i)
	}

	if strings.Contains(sum, "Restart the terminal") {
		t.Errorf("the restart line must not be in the box:\n%s", sum)
	}
	if !strings.Contains(out, "Restart the terminal, then:") || !strings.Contains(out, "ars init <name>") {
		t.Errorf("no restart line after the box:\n%s", out)
	}

	rc, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	if strings.Count(string(rc), setup.HookLine("zsh")) != 1 {
		t.Errorf(".zshrc:\n%s", rc)
	}
	f, err := config.LoadFile()
	if err != nil || f.PentestPath != "~/ops/pentest" || !f.RecByDefault {
		t.Errorf("config.yml = %+v, %v", f, err)
	}
	if fi, err := os.Stat(pt); err != nil || fi.Mode().Perm() != 0o700 {
		t.Errorf("pentest folder: %v, %v", fi, err)
	}
	if got, _ := config.PentestPath(); got != pt {
		t.Errorf("PentestPath = %q, want %q", got, pt)
	}
	if _, err := os.Stat(filepath.Join(cheatsDirT(t), "my.md")); err == nil {
		t.Error("setup created my.md; ars add does that when needed")
	}
}

func TestSetupChangesNothingUntilConfirmed(t *testing.T) {
	home := isolateSetup(t)
	removeConfig(t)
	s := newStepScript(t, nil)
	s.cancel = true
	out, code := run(t, "setup")
	if code != 130 || !strings.Contains(out, "Cancelled") {
		t.Errorf("code %d\n%s", code, out)
	}
	if ok, _ := config.FileExists(); ok {
		t.Error("config.yml written although the box was cancelled")
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err == nil {
		t.Error(".zshrc written although the box was cancelled")
	}
	if dirExists(cheatsDirT(t)) {
		t.Error("cheats folder created although the box was cancelled")
	}
}

func TestSetupSkipsEverythingWithoutChangingCheats(t *testing.T) {
	isolateSetup(t)
	s := newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 1})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if dirExists(cheatsDirT(t)) {
		t.Errorf("Skip created the cheats folder")
	}
	if f, _ := config.LoadFile(); len(f.CheatsPaths) != 0 {
		t.Errorf("cheats_paths written by Skip: %v", f.CheatsPaths)
	}
	if !strings.Contains(s.summaryText(), "Keep as it is") {
		t.Errorf("summary:\n%s", s.summaryText())
	}
}

func TestSetupShellStepOnlyWhenTheLineIsMissing(t *testing.T) {
	home := isolateSetup(t)
	s := newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 0})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("first run: %d %s", code, out)
	}
	rcBefore, _ := os.ReadFile(filepath.Join(home, ".zshrc"))

	s = newStepScript(t, map[string]any{"cheats": cheatsKeep})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("second run: %d %s", code, out)
	}
	if s.askedKey("shell") {
		t.Errorf("the shell step was asked although the line exists: %v", s.asked)
	}
	if !strings.Contains(s.summaryText(), "already in ~/.zshrc") {
		t.Errorf("summary:\n%s", s.summaryText())
	}
	if rcAfter, _ := os.ReadFile(filepath.Join(home, ".zshrc")); string(rcAfter) != string(rcBefore) {
		t.Errorf(".zshrc changed:\n%s", rcAfter)
	}
	if got := s.steps["folder"].Default; got != "~/Documents/pentest" {
		t.Errorf("folder default = %q", got)
	}
}

func TestSetupSkipLeavesTheRCFileAlone(t *testing.T) {
	home := isolateSetup(t)
	newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 1})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err == nil {
		t.Error("rc file written although Skip was chosen")
	}
}

func TestSetupUnknownShellOrNoInstallHasNoShellStep(t *testing.T) {
	home := isolateSetup(t)
	t.Setenv("SHELL", "/usr/bin/fish")
	s := newStepScript(t, map[string]any{"cheats": cheatsKeep})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if s.askedKey("shell") || !strings.Contains(s.summaryText(), "not detected") {
		t.Errorf("asked %v\n%s", s.asked, s.summaryText())
	}

	t.Setenv("SHELL", "/bin/zsh")
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	s = newStepScript(t, map[string]any{"cheats": cheatsKeep})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if s.askedKey("shell") || !strings.Contains(s.summaryText(), "make install") || !strings.Contains(s.summaryText(), "not added") {
		t.Errorf("asked %v\n%s", s.asked, s.summaryText())
	}
	if _, err := os.Stat(filepath.Join(home, ".zshrc")); err == nil {
		t.Error(".zshrc written although ars is not installed")
	}
}

func TestSetupReplacesAnOldLineOnlyWhenAsked(t *testing.T) {
	home := isolateSetup(t)
	rcPath := filepath.Join(home, ".zshrc")
	old := "export A=1\neval \"$(command ars init zsh)\"\nalias x=y\n"
	if err := os.WriteFile(rcPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 1})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if !strings.Contains(s.steps["shell"].Help, "ars init zsh") || s.steps["shell"].Options[0] != "Replace in ~/.zshrc" {
		t.Errorf("shell step = %+v", s.steps["shell"])
	}
	if b, _ := os.ReadFile(rcPath); string(b) != old {
		t.Errorf("rc changed although Skip was chosen:\n%s", b)
	}

	newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 0})
	out, code := run(t, "setup")
	if code != 0 || !strings.Contains(out, "Old line replaced") {
		t.Fatalf("%d %s", code, out)
	}
	want := "export A=1\n" + setup.HookLine("zsh") + "\nalias x=y\n"
	if b, _ := os.ReadFile(rcPath); string(b) != want {
		t.Errorf("rc:\n%s\nwant:\n%s", b, want)
	}
	if bk, _ := os.ReadFile(rcPath + ".arsenal-bak"); string(bk) != old {
		t.Errorf("backup = %q", bk)
	}
}

func TestSetupWritesPathExportBeforeTheLine(t *testing.T) {
	home := isolateSetup(t)
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ars"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	s := newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 0})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	if !strings.Contains(s.steps["shell"].Help, "PATH") {
		t.Errorf("the PATH line is not announced: %q", s.steps["shell"].Help)
	}
	rc, _ := os.ReadFile(filepath.Join(home, ".zshrc"))
	exp := `export PATH="$HOME/.local/bin:$PATH"`
	i, j := strings.Index(string(rc), exp), strings.Index(string(rc), setup.HookLine("zsh"))
	if i < 0 || j < 0 || i > j {
		t.Errorf("PATH export must come before the ars line:\n%s", rc)
	}
}

func collectionWith(t *testing.T, files map[string]string) string {
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

func TestSetupAddsTheArsenalCommandsAndRemovesThemKeepingYours(t *testing.T) {
	isolateSetup(t)
	dir := collectionWith(t, map[string]string{"nmap.md": "# nmap\n\n## mine\n\n```\nnmap mine\n```\n"})
	officialPack = packOf(map[string]string{
		"nmap.md": oneCheat,
	})

	s := newStepScript(t, map[string]any{"cheats": cheatsChange, "shell": 1})
	out, code := run(t, "setup")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if got := s.steps["cheats"].Help; got != "No Arsenal commands. 1 command of yours." {
		t.Errorf("%q", got)
	}
	if got := s.steps["cheats"].Options; got[0] != "Keep as it is" || got[1] != "Add the Arsenal commands (~1100)" {
		t.Errorf("%q", got)
	}
	if !strings.Contains(out, "Added 1 command to 1 file") {
		t.Errorf("%s", out)
	}
	for _, noise := range []string{"Loaded ", "files ignored"} {
		if strings.Contains(out, noise) {
			t.Errorf("%q in\n%s", noise, out)
		}
	}
	if a, own := cheats.Count(dir); a != 1 || own != 1 {
		t.Fatalf("%d %d", a, own)
	}
	if _, err := os.Stat(filepath.Join(dir, "arsenal")); err == nil {
		t.Error("arsenal/ folder")
	}

	s = newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 1})
	if out, code := run(t, "setup"); code != 0 || strings.Contains(out, "Added") || strings.Contains(out, "Removed") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if got := s.steps["cheats"].Help; got != "1 Arsenal command, 1 of yours" {
		t.Errorf("%q", got)
	}
	if got := s.steps["cheats"].Options; got[0] != "Keep them" || got[1] != "Remove the Arsenal commands (yours stay)" {
		t.Errorf("%q", got)
	}
	if a, own := cheats.Count(dir); a != 1 || own != 1 {
		t.Errorf("%d %d", a, own)
	}

	s = newStepScript(t, map[string]any{"cheats": cheatsChange, "shell": 1})
	out, code = run(t, "setup")
	if code != 0 || !strings.Contains(out, "Removed 1 command, kept 1 of yours") {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !strings.Contains(s.summaryText(), "Remove 1 command, keep 1 of yours") {
		t.Errorf("%s", s.summaryText())
	}
	list := cheats.Load([]string{dir}).Cheats
	if len(list) != 1 || list[0].Name != "mine" {
		t.Errorf("%v", list)
	}
}

func TestSetupFirstRunStartsEmptyByDefault(t *testing.T) {
	isolateSetup(t)
	removeConfig(t)
	s := newStepScript(t, map[string]any{"cheats": 0, "shell": 1})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	opts := s.steps["cheats"].Options
	if len(opts) != 2 || opts[0] != "Start empty" || opts[1] != "Add the Arsenal commands (~1100)" || s.steps["cheats"].DefaultOption != 0 {
		t.Errorf("%q", opts)
	}
	if dirExists(cheatsDirT(t)) {
		t.Error(cheatsDirT(t))
	}
}

func TestSetupAddingTheArsenalCommandsChangesNothingUntilConfirmed(t *testing.T) {
	isolateSetup(t)
	officialPack = packOf(map[string]string{"nmap.md": oneCheat})
	s := newStepScript(t, map[string]any{"cheats": cheatsChange})
	s.cancel = true
	if _, code := run(t, "setup"); code != 130 {
		t.Fatalf("code %d", code)
	}
	if dirExists(cheatsDirT(t)) {
		t.Error(cheatsDirT(t))
	}
}

func TestSetupNeverWritesCheatsPaths(t *testing.T) {
	isolateSetup(t)
	if err := config.SetFileKeys(map[string]any{"cheats_paths": []string{"~/work/cheats"}}); err != nil {
		t.Fatal(err)
	}
	officialPack = packOf(map[string]string{"nmap.md": oneCheat})
	newStepScript(t, map[string]any{"cheats": cheatsChange, "shell": 1})
	if out, code := run(t, "setup"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if f, _ := config.LoadFile(); len(f.CheatsPaths) != 1 || f.CheatsPaths[0] != "~/work/cheats" {
		t.Errorf("%v", f.CheatsPaths)
	}
	paths, _ := config.CheatsPaths()
	if len(paths) != 2 || paths[1] != cheatsDirT(t) {
		t.Errorf("%v", paths)
	}
}

func TestAnotherXDGConfigHomeOnlyGetsANotice(t *testing.T) {
	home := isolateSetup(t)
	old := filepath.Join(home, ".config", "arsenal")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	vars := filepath.Join(old, "vars")
	if err := os.WriteFile(vars, []byte("target=10.10.10.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, "sessions")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !strings.Contains(out, "XDG_CONFIG_HOME points to") || strings.Contains(out, "Moved") {
		t.Errorf("%s", out)
	}
	if b, err := os.ReadFile(vars); err != nil || string(b) != "target=10.10.10.5\n" {
		t.Errorf("%q %v", b, err)
	}
	cfg, _ := config.ConfigDir()
	if _, err := os.Stat(filepath.Join(cfg, "vars")); err == nil {
		t.Error(cfg)
	}
}

func TestSetupWarnsWhenEnvOverridesPentestPath(t *testing.T) {
	isolateSetup(t)
	t.Setenv("PENTEST_PATH", filepath.Join(t.TempDir(), "envpt"))
	newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 1})
	out, code := run(t, "setup")
	if code != 0 || !strings.Contains(out, "$PENTEST_PATH is set") {
		t.Errorf("code %d, env override not reported:\n%s", code, out)
	}
}

func TestSetupRecordingStartsOnTheSavedValue(t *testing.T) {
	isolateSetup(t)
	if err := config.SetFileKeys(map[string]any{"rec_by_default": true}); err != nil {
		t.Fatal(err)
	}
	s := newStepScript(t, map[string]any{"cheats": cheatsKeep, "shell": 1})
	run(t, "setup")
	if s.steps["rec"].DefaultOption != 1 {
		t.Errorf("recording default option = %d, want the saved Always", s.steps["rec"].DefaultOption)
	}
}

func TestFirstRunRunsSetupThenTheCommand(t *testing.T) {
	isolateSetup(t)
	removeConfig(t)
	s := newStepScript(t, map[string]any{"folder": "~/pt", "cheats": cheatsKeep, "shell": 1})
	out, code := run(t, "sessions")
	if code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	if !strings.Contains(out, "First run: setting up Arsenal") || s.runs != 1 {
		t.Errorf("setup did not run first (runs=%d):\n%s", s.runs, out)
	}
	if ok, _ := config.FileExists(); !ok {
		t.Error("config.yml not created")
	}
	if out, code := run(t, "sessions"); code != 0 || strings.Contains(out, "running the setup") || s.runs != 1 {
		t.Errorf("second run: code %d runs %d\n%s", code, s.runs, out)
	}
}

func TestFirstRunCancelledStopsTheCommand(t *testing.T) {
	isolateSetup(t)
	removeConfig(t)
	s := newStepScript(t, nil)
	s.cancel = true
	out, code := run(t, "sessions")
	if code != 130 || !strings.Contains(out, "Cancelled") {
		t.Errorf("code %d\n%s", code, out)
	}
}

func TestSetupWarnsWhenAVariableDefaultCannotBeWrittenInTheCommand(t *testing.T) {
	isolateSetup(t)
	officialPack = packOf(map[string]string{
		"a.md": "# Web\n\n$ iface: ip route | head -1\n\n## one\n\n```\nping <iface>\n```\n",
		"b.md": "# Web\n\n= iface: eth0 or wlan0\n\n## two\n\n```\nping -c 1 <iface>\n```\n",
	})
	newStepScript(t, map[string]any{"cheats": cheatsChange, "shell": 1})
	out, code := run(t, "setup")
	if code != 0 || !strings.Contains(out, "Added 2 commands to 1 file") || !strings.Contains(out, "1 variable defaults differ") || !strings.Contains(out, "web.md: iface") {
		t.Fatalf("code %d\n%s", code, out)
	}
}
