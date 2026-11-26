// Helpers shared by the cli tests, and tests of ars init, sessions and the variables.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

func isolate(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "pentest")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PENTEST_PATH", root)
	t.Chdir(t.TempDir())
	if err := config.SetFileKeys(map[string]any{}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{EnvSession, "ARSENAL_SESSION_DIR", EnvPaused, EnvRecID, EnvCDFile, "ARSENAL_CMD_FILE"} {
		t.Setenv(k, "")
	}
	isInteractive = func() bool { return false }
	askText = func(string, string, string, func(string) error) (string, error) {
		t.Fatal("unexpected text prompt")
		return "", nil
	}
	confirm = func(string, string) (bool, error) {
		t.Fatal("unexpected confirm prompt")
		return false, nil
	}
	t.Cleanup(func() {
		isInteractive = tui.IsInteractive
		askText = tui.AskText
		confirm = tui.Confirm
	})
	return root
}

func cheatsDirT(t *testing.T) string {
	t.Helper()
	d, err := config.CheatsDir()
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func removeConfig(t *testing.T) {
	t.Helper()
	p, _ := config.FilePath()
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
}

func run(t *testing.T, args ...string) (stdout string, code int) {
	t.Helper()
	root := newRootCmd("ars")
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	var cdFile string
	if len(args) > 0 && args[0] == "init" && os.Getenv(EnvCDFile) == "" {
		cdFile = filepath.Join(t.TempDir(), "cd")
		t.Setenv(EnvCDFile, cdFile)
		defer t.Setenv(EnvCDFile, "")
	}
	code = execute(root, &errb)
	if cdFile != "" {
		if b, err := os.ReadFile(cdFile); err == nil && len(b) > 0 {
			t.Chdir(string(b))
		}
	}
	return out.String() + errb.String(), code
}

func logPathOf(t *testing.T, name string) string {
	t.Helper()
	in, err := session.ByName(name)
	if err != nil {
		t.Fatal(err)
	}
	return cmdlog.Path(in.Dir)
}

func sessionVars(t *testing.T, name string) session.Vars {
	t.Helper()
	in, err := session.ByName(name)
	if err != nil {
		t.Fatal(err)
	}
	v, err := session.LoadVars(in.Dir)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func wd(t *testing.T) string {
	t.Helper()
	d, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	r, err := filepath.EvalSymlinks(d)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func inSession(t *testing.T, name string) string {
	t.Helper()
	if out, code := run(t, "init", name); code != 0 {
		t.Fatalf("init %s: code %d: %s", name, code, out)
	}
	return wd(t)
}

func TestInitSetVarsFlow(t *testing.T) {
	root := isolate(t)

	if out, code := run(t, "init", "acme"); code != 0 {
		t.Fatalf("init: code %d: %s", code, out)
	}
	if fi, err := os.Stat(filepath.Join(root, "acme")); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("session folder missing or wrong mode: %v", err)
	}
	if fi, err := os.Stat(filepath.Join(root, "acme", ".arsenal")); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf(".arsenal must be a 0700 directory: %v %v", fi, err)
	}
	if fi, err := os.Stat(filepath.Join(root, "acme", ".arsenal", "session.yml")); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("session.yml: %v %v", fi, err)
	}
	if got, want := wd(t), realPath(t, filepath.Join(root, "acme")); got != want {
		t.Fatalf("not in the session folder: %s, want %s", got, want)
	}

	if out, code := run(t, "set", "target=10.10.10.5", "ports=22,80"); code != 0 {
		t.Fatalf("set: code %d: %s", code, out)
	}
	out, code := run(t, "vars", "--plain")
	if code != 0 || out != "ports=22,80\ntarget=10.10.10.5\n" {
		t.Fatalf("vars --plain: code %d, %q", code, out)
	}
	if b, _ := os.ReadFile(filepath.Join(root, "acme", ".arsenal", "session.yml")); !strings.Contains(string(b), "target: 10.10.10.5") {
		t.Errorf("marker: %s", b)
	}
	if _, err := os.Stat(filepath.Join(root, "acme", "vars")); err == nil {
		t.Error("a vars file was written")
	}
	if entries, _ := os.ReadDir(filepath.Join(root, "acme")); len(entries) != 1 || entries[0].Name() != ".arsenal" {
		t.Errorf("session folder holds %v, want only .arsenal", entries)
	}
	if out, code := run(t, "unset", "ports"); code != 0 {
		t.Fatalf("unset: code %d: %s", code, out)
	}
	if out, _ := run(t, "vars", "--plain"); out != "target=10.10.10.5\n" {
		t.Fatalf("after unset: %q", out)
	}
	if out, code := run(t, "unset", "nope"); code == 0 || !strings.Contains(out, "not set: nope") {
		t.Fatalf("unset missing: code %d: %s", code, out)
	}
}

func TestSubfoldersBelongToTheSessionAndOutsideThereIsNone(t *testing.T) {
	root := isolate(t)
	dir := inSession(t, "acme")
	run(t, "set", "target=10.10.10.5")
	sub := filepath.Join(dir, "scans", "nmap")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	if out, _ := run(t, "vars", "--plain"); out != "target=10.10.10.5\n" {
		t.Fatalf("in a subfolder: %q", out)
	}
	if out, code := run(t, "set", "ports=22"); code != 0 {
		t.Fatalf("set in a subfolder: %d %s", code, out)
	}
	t.Chdir(root)
	if out, code := run(t, "set", "x=1"); code == 0 || !strings.Contains(out, noSessionMsg) {
		t.Fatalf("outside: code %d: %s", code, out)
	}
	if out, _ := run(t, "vars", "--plain"); out != "" {
		t.Fatalf("outside a session only globals show: %q", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".arsenal")); err == nil {
		t.Error("the pentest root became a session")
	}
}

func TestSetRejectsBadPairBeforeWriting(t *testing.T) {
	isolate(t)
	inSession(t, "acme")
	if _, code := run(t, "set", "ok=1", "bad"); code == 0 {
		t.Fatal("expected failure")
	}
	if out, _ := run(t, "vars", "--plain"); out != "" {
		t.Fatalf("nothing should have been saved, got %q", out)
	}
}

const noSessionMsg = "not in a session folder: run ars init <name> or cd into one"

func TestSessionCommandsOutsideASessionFolderFailClearly(t *testing.T) {
	isolate(t)
	for _, c := range [][]string{{"set", "a=1"}, {"unset", "a"}, {"note", "x"}, {"notes"}, {"history"}, {"history", "--list"}, {"rec"}, {"replay"}} {
		out, code := run(t, c...)
		if code == 0 || !strings.Contains(out, noSessionMsg) {
			t.Errorf("%v: code %d: %s", c, code, out)
		}
	}
	for _, c := range [][]string{{"set", "-g", "a=1"}, {"unset", "-g", "a"}, {"vars"}, {"sessions"}, {"history", "--all", "--list"}, {"status"}, {"config"}} {
		if out, code := run(t, c...); code != 0 {
			t.Errorf("%v: code %d: %s", c, code, out)
		}
	}
}

func TestInitExistingNonInteractiveRefusesWithoutResume(t *testing.T) {
	root := isolate(t)
	run(t, "init", "acme")
	run(t, "init", "other")
	if out, code := run(t, "init", "acme"); code == 0 || !strings.Contains(out, "already exists") {
		t.Fatalf("code %d: %s", code, out)
	}
	if got, want := wd(t), realPath(t, filepath.Join(root, "other")); got != want {
		t.Fatalf("cwd changed to %q", got)
	}
	if out, code := run(t, "init", "acme", "--resume"); code != 0 {
		t.Fatalf("resume: code %d: %s", code, out)
	}
	if got, want := wd(t), realPath(t, filepath.Join(root, "acme")); got != want {
		t.Fatalf("cwd = %q, want %q", got, want)
	}
}

func TestInitExistingInteractiveAsksToResume(t *testing.T) {
	root := isolate(t)
	run(t, "init", "acme")
	run(t, "init", "other")
	marker := filepath.Join(root, "acme", "keep.txt")
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	other := realPath(t, filepath.Join(root, "other"))

	isInteractive = func() bool { return true }
	answer := false
	confirm = func(string, string) (bool, error) { return answer, nil }

	if out, code := run(t, "init", "acme"); code != 0 || !strings.Contains(out, "Nothing changed") {
		t.Fatalf("decline: code %d: %s", code, out)
	}
	if wd(t) != other {
		t.Fatalf("declining must not move: %q", wd(t))
	}

	answer = true
	if out, code := run(t, "init", "acme"); code != 0 {
		t.Fatalf("accept: code %d: %s", code, out)
	}
	if got, want := wd(t), realPath(t, filepath.Join(root, "acme")); got != want {
		t.Fatalf("cwd = %q, want %q", got, want)
	}
	if b, _ := os.ReadFile(marker); string(b) != "x" {
		t.Fatal("existing session content changed")
	}
}

func TestInitNeverOverwritesAndAdoptsOnlyAfterAsking(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "legacy")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, code := run(t, "init", "legacy"); code == 0 || !strings.Contains(out, "not a session") {
		t.Fatalf("code %d: %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".arsenal")); err == nil {
		t.Fatal("marker written without asking")
	}
	isInteractive = func() bool { return true }
	confirm = func(string, string) (bool, error) { return false, nil }
	if _, code := run(t, "init", "legacy"); code != 0 {
		t.Fatal("declining is not an error")
	}
	if _, err := os.Stat(filepath.Join(dir, ".arsenal")); err == nil {
		t.Fatal("marker written after declining")
	}
	confirm = func(string, string) (bool, error) { return true, nil }
	if out, code := run(t, "init", "legacy"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".arsenal")); err != nil {
		t.Fatalf("marker missing: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "data.txt")); string(b) != "keep" {
		t.Error("existing file changed")
	}
}

func TestInitAsksForNameWhenInteractive(t *testing.T) {
	isolate(t)
	isInteractive = func() bool { return true }
	askText = func(string, string, string, func(string) error) (string, error) { return "typed-name", nil }
	if out, code := run(t, "init"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if s, ok, _ := session.Current(); !ok || s.Name != "typed-name" {
		t.Fatalf("session = %+v %v", s, ok)
	}
}

func TestInitNoNameNonInteractiveFails(t *testing.T) {
	isolate(t)
	if out, code := run(t, "init"); code == 0 || !strings.Contains(out, "session name required") {
		t.Fatalf("code %d: %s", code, out)
	}
}

func TestInitCancelledPrompt(t *testing.T) {
	isolate(t)
	isInteractive = func() bool { return true }
	askText = func(string, string, string, func(string) error) (string, error) { return "", tui.ErrCancelled }
	if _, code := run(t, "init"); code != 130 {
		t.Fatalf("code = %d, want 130", code)
	}
}

func TestInitRejectsBadName(t *testing.T) {
	root := isolate(t)
	if _, code := run(t, "init", "../evil"); code == 0 {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "evil")); err == nil {
		t.Fatal("escaped PENTEST_PATH")
	}
}

func TestSessionsListsAllAndMarksTheCurrentFolder(t *testing.T) {
	isolate(t)
	if out, code := run(t, "sessions"); code != 0 || !strings.Contains(out, "No sessions") {
		t.Fatalf("empty: code %d: %s", code, out)
	}
	run(t, "init", "alpha")
	run(t, "init", "beta")

	out, _ := run(t, "sessions")
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "beta") {
		t.Fatalf("list: %s", out)
	}
	var here string
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "(this folder)") {
			here = l
		}
	}
	if !strings.Contains(here, "beta") || strings.Contains(out, "alpha (this folder)") {
		t.Fatalf("beta should be marked, got line %q", here)
	}
	t.Chdir(t.TempDir())
	if out, _ := run(t, "sessions"); !strings.Contains(out, "alpha") || strings.Contains(out, "(this folder)") {
		t.Fatalf("outside: %s", out)
	}
}

func TestUseAndDetachAreGone(t *testing.T) {
	isolate(t)
	for _, c := range []string{"use", "detach"} {
		if out, code := run(t, c); code == 0 || !strings.Contains(out, "unknown command") {
			t.Errorf("%s must not exist: code %d: %s", c, code, out)
		}
	}
}

func TestInitWritesCDFileOnlyWhenItGetsThere(t *testing.T) {
	root := isolate(t)
	cdf := filepath.Join(t.TempDir(), "cd")
	t.Setenv(EnvCDFile, cdf)

	read := func() string {
		b, _ := os.ReadFile(cdf)
		_ = os.Remove(cdf)
		return string(b)
	}

	run(t, "init", "alpha")
	if got := read(); got != filepath.Join(root, "alpha") {
		t.Fatalf("init: cd file = %q", got)
	}
	run(t, "init", "beta")
	read()
	run(t, "init", "alpha", "--resume")
	if got := read(); got != filepath.Join(root, "alpha") {
		t.Fatalf("resume: cd file = %q", got)
	}

	isInteractive = func() bool { return true }
	confirm = func(string, string) (bool, error) { return false, nil }
	run(t, "init", "alpha")
	run(t, "init", "../evil")
	if got := read(); got != "" {
		t.Fatalf("unexpected cd request: %q", got)
	}
}

func TestHookPrintsShellFunction(t *testing.T) {
	isolate(t)
	for _, sh := range []string{"zsh", "bash"} {
		out, code := run(t, "hook", sh)
		if code != 0 || !strings.Contains(out, "ars()") || !strings.Contains(out, "ARSENAL_CD_FILE") {
			t.Fatalf("%s: code %d: %s", sh, code, out)
		}
		for _, gone := range []string{"detach", "terminal attached", "ARSENAL_SESSION_DIR=\"$_d\""} {
			if strings.Contains(out, gone) {
				t.Errorf("%s: the script still has %q", sh, gone)
			}
		}
		if !strings.Contains(out, ".arsenal") {
			t.Errorf("%s: the hooks do not look for .arsenal", sh)
		}
	}
	if _, code := run(t, "hook", "fish"); code == 0 {
		t.Fatal("fish should be unsupported")
	}
}

func TestNoCompletionCommand(t *testing.T) {
	isolate(t)
	if _, code := run(t, "completion", "zsh"); code == 0 {
		t.Fatal("completion command should not exist")
	}
}

func TestGlobalVarsLayerAndOverride(t *testing.T) {
	isolate(t)

	if out, code := run(t, "set", "-g", "arsenal_prefix_cmd=proxychains -q", "target=10.10.10.1"); code != 0 {
		t.Fatalf("set -g: code %d: %s", code, out)
	}
	if out, code := run(t, "vars", "--plain"); code != 0 || out != "arsenal_prefix_cmd=proxychains -q\ntarget=10.10.10.1\n" {
		t.Fatalf("vars without session: code %d: %q", code, out)
	}
	if out, code := run(t, "set", "x=1"); code == 0 || !strings.Contains(out, noSessionMsg) {
		t.Fatalf("non-global set without session: code %d: %s", code, out)
	}

	inSession(t, "acme")
	run(t, "set", "target=10.10.10.5", "ports=22")
	out, _ := run(t, "vars", "--plain")
	if out != "arsenal_prefix_cmd=proxychains -q\nports=22\ntarget=10.10.10.5\n" {
		t.Fatalf("merged: %q", out)
	}
	out, _ = run(t, "vars")
	for _, want := range []string{"target", "10.10.10.5", "(session)", "arsenal_prefix_cmd", "(global)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("vars output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "10.10.10.1") {
		t.Fatalf("overridden global value should not be shown:\n%s", out)
	}
	if out, _ := run(t, "vars", "-g", "--plain"); out != "arsenal_prefix_cmd=proxychains -q\ntarget=10.10.10.1\n" {
		t.Fatalf("vars -g: %q", out)
	}

	run(t, "unset", "target")
	if out, _ := run(t, "vars", "--plain"); !strings.Contains(out, "target=10.10.10.1") {
		t.Fatalf("after unset: %q", out)
	}
	if out, code := run(t, "unset", "-g", "target"); code != 0 {
		t.Fatalf("unset -g: code %d: %s", code, out)
	}
	if out, _ := run(t, "vars", "--plain"); strings.Contains(out, "target") {
		t.Fatalf("target should be gone: %q", out)
	}
}

func TestSessionsOutputIsPlainASCII(t *testing.T) {
	isolate(t)
	run(t, "init", "alpha")
	run(t, "init", "beta")
	out, _ := run(t, "sessions")
	for _, r := range out {
		if r > 127 {
			t.Fatalf("non-ASCII %q in %q", r, out)
		}
	}
	if !strings.Contains(out, "> beta") {
		t.Errorf("current session marker: %q", out)
	}
}
