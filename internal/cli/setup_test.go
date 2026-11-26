package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/session"
)

func isolateSetup(t *testing.T) (home string) {
	t.Helper()
	isolate(t)
	home = t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SHELL", "/bin/zsh")
	t.Setenv("ZDOTDIR", "")
	t.Setenv("PENTEST_PATH", "")
	isInteractive = func() bool { return true }
	lookPath = func(string) (string, error) { return "/usr/local/bin/ars", nil }
	t.Cleanup(func() { lookPath = exec.LookPath })
	return home
}

func packOf(files map[string]string) fstest.MapFS {
	pack := fstest.MapFS{}
	for name, body := range files {
		pack[name] = &fstest.MapFile{Data: []byte(body)}
	}
	return pack
}

const oneCheat = "# Nmap\n\n## scan\n% nmap\n\n```\nnmap -sV <target>\n```\n"

func TestSetupNeedsATerminal(t *testing.T) {
	isolateSetup(t)
	isInteractive = func() bool { return false }
	out, code := run(t, "setup")
	if code == 0 || !strings.Contains(out, "interactive") {
		t.Errorf("code %d: %s", code, out)
	}
}

func TestHookIsHiddenAndInitIsTheSessionCommand(t *testing.T) {
	isolateSetup(t)
	out, code := run(t, "--help")
	if code != 0 {
		t.Fatal(out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "hook ") {
			t.Errorf("hook is listed in --help: %q", l)
		}
		if strings.HasPrefix(strings.TrimSpace(l), "start ") {
			t.Errorf("start must not exist any more: %q", l)
		}
	}
	if !strings.Contains(out, "  init ") {
		t.Error("init is not listed in --help")
	}
	if !strings.Contains(out, "setup") {
		t.Error("setup is not listed in --help")
	}
	if out, code := run(t, "hook", "zsh"); code != 0 || !strings.Contains(out, "ars()") {
		t.Errorf("hidden hook broken: code %d", code)
	}
}

func TestSetupRejectsRelativePentestPath(t *testing.T) {
	if validatePentestPath("pentest") == nil || validatePentestPath("") == nil {
		t.Error("relative or empty path accepted")
	}
	if validatePentestPath("~/x") != nil || validatePentestPath("/srv/x") != nil {
		t.Error("valid path rejected")
	}
}

func TestAddWritesToTheCollectionNotToOtherFolders(t *testing.T) {
	isolateAdd(t)
	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.SetFileKeys(map[string]any{"cheats_paths": []string{other, "~/.cheats"}}); err != nil {
		t.Fatal(err)
	}
	if out, code := run(t, "add", "id", "--title", "who am i"); code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(cheatsDirT(t), "id.md")); err != nil {
		t.Errorf("%v", err)
	}
	if entries, _ := os.ReadDir(other); len(entries) != 0 {
		t.Errorf("%v", entries)
	}
}

func TestFirstRunSkippedWithoutTerminalAndForInitSetupHelp(t *testing.T) {
	isolateSetup(t)
	removeConfig(t)
	steps := newStepScript(t, nil)
	isInteractive = func() bool { return false }
	if out, code := run(t, "sessions"); code != 0 || strings.Contains(out, "running the setup") {
		t.Errorf("no terminal: code %d\n%s", code, out)
	}
	isInteractive = func() bool { return true }
	for _, args := range [][]string{{"hook", "zsh"}, {"--help"}, {"config", "--help"}, {"--version"}} {
		if out, code := run(t, args...); code != 0 || strings.Contains(out, "running the setup") {
			t.Errorf("%v: code %d\n%s", args, code, out)
		}
	}
	if ok, _ := config.FileExists(); ok || steps.runs != 0 {
		t.Errorf("the wizard ran (runs=%d) or config.yml was created by a command that must not run it", steps.runs)
	}
}

func TestUpstreamCheatsFolderIsReadButNeverMoved(t *testing.T) {
	home := isolateSetup(t)
	if err := os.MkdirAll(filepath.Join(home, ".cheats"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(home, ".cheats", "my.md")
	if err := os.WriteFile(file, []byte("# My cheats\n\n## x\n\n```\nid\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, "sessions")
	if code != 0 || strings.Contains(out, "Moved") {
		t.Fatalf("code %d:\n%s", code, out)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("%v", err)
	}
	paths, _ := config.CheatsPaths()
	found := false
	for _, p := range paths {
		found = found || p == filepath.Join(home, ".cheats")
	}
	if !found {
		t.Errorf("%v", paths)
	}
}

func TestConfigPrintsPaths(t *testing.T) {
	home := isolateSetup(t)
	isInteractive = func() bool { return false }
	t.Setenv("PENTEST_PATH", filepath.Join(home, "pt"))
	if out, code := run(t, "init", "acme"); code != 0 {
		t.Fatalf("start: %d %s", code, out)
	}
	out, code := run(t, "config")
	if code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	cfgDir, _ := config.ConfigDir()
	for _, want := range []string{
		"Config file:", filepath.Join(cfgDir, "config.yml"), "(missing)",
		"Global vars:", filepath.Join(cfgDir, "vars"),
		"Cheatsheet folder:", filepath.Join(cfgDir, "cheats"),
		"Reads cheatsheets from:", "Pentest path:", "$PENTEST_PATH",
		"Session:", "acme", "Session data:", filepath.Join("acme", ".arsenal"),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, filepath.Join(home, ".cheats")) {
		t.Errorf("~/.cheats listed although it does not exist:\n%s", out)
	}
}

func TestConfigEditIsGoneAndConfigEndsWithAHint(t *testing.T) {
	isolateSetup(t)
	isInteractive = func() bool { return false }
	if out, code := run(t, "config", "edit"); code == 0 || !strings.Contains(out, "unknown command") {
		t.Errorf("config edit must be gone: code %d\n%s", code, out)
	}
	out, code := run(t, "config")
	if code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if last := lines[len(lines)-1]; last != "Edit config.yml to change settings." {
		t.Errorf("last line = %q", last)
	}
	if strings.Contains(out, "ars config edit") {
		t.Errorf("the removed command is still mentioned:\n%s", out)
	}
	if help, _ := run(t, "config", "--help"); strings.Contains(help, "edit") && strings.Contains(help, "Available Commands") {
		t.Errorf("config --help lists a subcommand:\n%s", help)
	}
}

func TestHelpGroupsPutConfigUnderSetup(t *testing.T) {
	isolateSetup(t)
	out, code := run(t, "--help")
	if code != 0 {
		t.Fatal(out)
	}
	i := strings.Index(out, "Setup:")
	if i < 0 {
		t.Fatalf("no Setup group:\n%s", out)
	}
	section := out[i:]
	if k := strings.Index(section[len("Setup:"):], "\n\n"); k >= 0 {
		section = section[:len("Setup:")+k]
	}
	for _, want := range []string{"config", "setup"} {
		if !strings.Contains(section, "  "+want+" ") {
			t.Errorf("%q is not in the Setup group:\n%s", want, section)
		}
	}
	for _, g := range []string{"Sessions:", "Variables:", "Cheatsheets and notes:", "Command log and recording:"} {
		if !strings.Contains(out, g) {
			t.Errorf("missing group %q", g)
		}
	}
}

func TestInitRefusesShellNamesAndStartIsGone(t *testing.T) {
	root := isolate(t)
	out, code := run(t, "init", "zsh")
	if code == 0 || !strings.Contains(out, "ars init zsh") || !strings.Contains(out, "ars setup") {
		t.Errorf("code %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "zsh")); err == nil {
		t.Error("a session named zsh was created")
	}
	if out, code := run(t, "start", "acme"); code == 0 || !strings.Contains(out, "unknown command") {
		t.Errorf("start must be gone: code %d:\n%s", code, out)
	}
}

func TestInitAsksForTheNameWhenOmitted(t *testing.T) {
	root := isolate(t)
	isInteractive = func() bool { return true }
	askText = func(string, string, string, func(string) error) (string, error) { return "acme", nil }
	if out, code := run(t, "init"); code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	if _, err := os.Stat(filepath.Join(root, "acme")); err != nil {
		t.Errorf("session not created: %v", err)
	}
	if s, ok, _ := session.Current(); !ok || s.Name != "acme" {
		t.Errorf("session of the folder = %+v %v", s, ok)
	}
}

func TestLegacySessionsAreMigratedBeforeTheFirstCommand(t *testing.T) {
	root := isolate(t)
	cfg, _ := config.ConfigDir()
	if err := os.WriteFile(filepath.Join(cfg, "current"), []byte("acme\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "acme")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vars"), []byte("target=10.10.10.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, "sessions")
	if code != 0 || !strings.Contains(out, "session acme: created .arsenal/session.yml") || !strings.Contains(out, "variables moved into it") ||
		!strings.Contains(out, "the session is the folder you are in") {
		t.Fatalf("code %d, the user was not told what moved:\n%s", code, out)
	}
	t.Chdir(dir)
	if out, _ := run(t, "vars", "--plain"); out != "target=10.10.10.5\n" {
		t.Errorf("vars after migration: %q", out)
	}
	if _, err := os.Stat(filepath.Join(cfg, "current")); err == nil {
		t.Error("the current file is still there")
	}
	if out, _ := run(t, "sessions"); strings.Contains(out, "created .arsenal/session.yml") {
		t.Errorf("reported again:\n%s", out)
	}
}

func TestOldMarkerFileSessionIsMigratedToAFolderAndTheUserIsTold(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "acme")
	if err := os.MkdirAll(filepath.Join(dir, "recordings"), 0o700); err != nil {
		t.Fatal(err)
	}
	for f, body := range map[string]string{
		".arsenal":       "name: acme\ncreated: 2026-11-01\nvars:\n    target: 10.10.10.5\n",
		"notes.md":       "# Notes - acme\n",
		"commands.jsonl": "",
		"scan.txt":       "mine",
	} {
		if err := os.WriteFile(filepath.Join(dir, f), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(dir)
	out, code := run(t, "vars", "--plain")
	if code != 0 || !strings.Contains(out, "session acme: moved the old .arsenal file to .arsenal/session.yml") ||
		!strings.Contains(out, "moved notes.md, commands.jsonl, recordings/ into .arsenal/") || !strings.Contains(out, "target=10.10.10.5") {
		t.Fatalf("code %d:\n%s", code, out)
	}
	for _, f := range []string{"session.yml", "notes.md", "commands.jsonl", "recordings"} {
		if _, err := os.Stat(filepath.Join(dir, ".arsenal", f)); err != nil {
			t.Errorf(".arsenal/%s: %v", f, err)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "scan.txt")); string(b) != "mine" {
		t.Error("a user file was touched")
	}
	if out, _ := run(t, "vars", "--plain"); strings.Contains(out, "moved") {
		t.Errorf("reported again:\n%s", out)
	}
	if out, code := run(t, "note", "after migration"); code != 0 {
		t.Fatalf("note: %d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.md")); err == nil {
		t.Error("notes.md came back in the session folder")
	}
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}
