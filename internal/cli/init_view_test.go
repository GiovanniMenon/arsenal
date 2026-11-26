package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/rec"
	"github.com/ynn3m/arsenal/internal/setup"
)

func onlyBorderIsNonASCII(t *testing.T, out string) {
	t.Helper()
	for _, r := range out {
		if r > 127 && (r < 0x2500 || r > 0x257f) {
			t.Fatalf("non-ASCII %q in %q", r, out)
		}
	}
}

func TestInitNewSessionShowsABoxAndStartHere(t *testing.T) {
	root := isolate(t)
	out, code := run(t, "init", "acme")
	if code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	onlyBorderIsNonASCII(t, out)
	for _, want := range []string{
		"New session: acme",
		"Folder", filepath.Join(root, "acme"),
		"Recording", "off (ars rec starts it, exit stops it)",
		"Start here:",
		"1. ars set target=<ip>", "the target, used to fill commands",
		"2. ars -s <tool>", "find a command, e.g. ars -s nmap",
		`3. ars note "<text>"`, "keep notes while you work",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "\u256d") || !strings.Contains(out, "\u2570") {
		t.Errorf("no rounded box:\n%s", out)
	}
	if strings.Index(out, "New session") > strings.Index(out, "Start here:") {
		t.Errorf("the box must come first:\n%s", out)
	}
	for _, old := range []string{"created", "[ok]", "[ars]", "Target", "So far"} {
		if strings.Contains(out, old) {
			t.Errorf("output must not contain %q for a new session:\n%s", old, out)
		}
	}
}

func TestInitResumedSessionShowsCountsAndNext(t *testing.T) {
	isolate(t)
	run(t, "init", "acme")
	if out, code := run(t, "note", "first"); code != 0 {
		t.Fatalf("note: %d %s", code, out)
	}
	if out, code := run(t, "note", "second"); code != 0 {
		t.Fatalf("note: %d %s", code, out)
	}

	out, code := run(t, "init", "acme", "--resume")
	if code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	onlyBorderIsNonASCII(t, out)
	for _, want := range []string{
		"Resumed: acme", "Folder", "So far", "0 commands, 2 notes, 0 recordings",
		"Recording", "Next: ars set target=<ip> | ars -s <tool> | ars notes | ars history | ars status",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Target") || strings.Contains(out, "Start here") {
		t.Errorf("no Target row without a target, no Start here when resuming:\n%s", out)
	}
	if strings.Contains(out, "off (ars rec") {
		t.Errorf("a resumed session only says on/off:\n%s", out)
	}

	if out, code := run(t, "set", "target=10.10.10.5"); code != 0 {
		t.Fatalf("set: %d %s", code, out)
	}
	out, _ = run(t, "init", "acme", "--resume")
	for _, want := range []string{"Target", "10.10.10.5", "Next: ars -s <tool> | ars notes | ars history | ars status"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "ars set target") {
		t.Errorf("target is set, do not suggest setting it:\n%s", out)
	}
}

func TestInitHintsCanBeTurnedOff(t *testing.T) {
	isolate(t)
	writeConfig(t, "hints: false\n")
	out, code := run(t, "init", "acme")
	if code != 0 || !strings.Contains(out, "New session: acme") || strings.Contains(out, "Start here") {
		t.Fatalf("new session, code %d:\n%s", code, out)
	}
	out, _ = run(t, "init", "acme", "--resume")
	if !strings.Contains(out, "Resumed: acme") || strings.Contains(out, "Next:") {
		t.Errorf("resumed, hints off:\n%s", out)
	}
	if err := config.SetFileKeys(map[string]any{"hints": true}); err != nil {
		t.Fatal(err)
	}
	if out, _ := run(t, "init", "acme", "--resume"); !strings.Contains(out, "Next:") {
		t.Errorf("hints: true:\n%s", out)
	}
	if h, err := config.Hints(); err != nil || !h {
		t.Errorf("Hints() = %v, %v", h, err)
	}
}

func TestInitRecordingRowAndTheFilePathOnlyWithVerbose(t *testing.T) {
	isolate(t)
	f := fakeRecorderFor(t, "/tmp/acme/.arsenal/recordings/20261124-101500-1.cast")

	out, code := run(t, "init", "acme", "--rec")
	if code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	if !strings.Contains(out, "on (exit or ctrl-d stops it)") {
		t.Errorf("recording row:\n%s", out)
	}
	if strings.Contains(out, ".cast") {
		t.Errorf("the recording file is only shown with -v:\n%s", out)
	}
	if len(f.calls) != 1 || f.calls[0].Announce == nil {
		t.Fatalf("the recorder was not started with an announcer: %+v", f.calls)
	}

	out, code = run(t, "init", "acme", "--resume", "--rec", "-v")
	if code != 0 || !strings.Contains(out, "Recording file: /tmp/acme/.arsenal/recordings/20261124-101500-1.cast") {
		t.Errorf("-v must show the file, code %d:\n%s", code, out)
	}
	if !strings.Contains(out, "Resumed: acme") {
		t.Errorf("resumed box:\n%s", out)
	}
	if strings.Contains(out, "exit or ctrl-d") {
		t.Errorf("resumed recording row:\n%s", out)
	}
}

func fakeRecorderFor(t *testing.T, castPath string) *fakeRecorder {
	t.Helper()
	f := &fakeRecorder{t: t}
	t.Setenv(rec.EnvRec, "")
	runRecorder = func(o rec.Options) (rec.Result, error) {
		f.calls = append(f.calls, o)
		if o.Announce != nil {
			o.Announce(castPath)
		}
		return rec.Result{}, nil
	}
	t.Cleanup(func() { runRecorder = rec.Run })
	return f
}

func TestPrintInitWithoutShellIntegrationSaysSo(t *testing.T) {
	isolate(t)
	dir := filepath.Join(t.TempDir(), "acme")
	if err := os.MkdirAll(filepath.Join(dir, ".arsenal"), 0o700); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := printInit(&buf, initInfo{New: true, Name: "acme", Dir: dir, Hints: false, Integration: false}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "cd there yourself") {
		t.Errorf("missing integration row:\n%s", out)
	}
	if strings.Contains(out, "Start here") {
		t.Errorf("hints are off:\n%s", out)
	}
	buf.Reset()
	if err := printInit(&buf, initInfo{New: true, Name: "acme", Dir: dir, Integration: true}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "shell integration") {
		t.Errorf("integration present, no row expected:\n%s", buf.String())
	}
}

func TestPrintInitSaysRestartWhenTheRCLineExists(t *testing.T) {
	isolate(t)
	dir := filepath.Join(t.TempDir(), "acme")
	if err := os.MkdirAll(filepath.Join(dir, ".arsenal"), 0o700); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := printInit(&buf, initInfo{New: true, Name: "acme", Dir: dir, RCInstalled: true}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "restart the terminal to enable shell integration") || strings.Contains(out, "run ars setup to do it automatically") {
		t.Errorf("rc line present, terminal not restarted:\n%s", out)
	}
	buf.Reset()
	if err := printInit(&buf, initInfo{New: true, Name: "acme", Dir: dir}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "run ars setup to do it automatically") {
		t.Errorf("no rc line:\n%s", buf.String())
	}
}

func TestRCHasIntegrationDecidesRestartOrSetup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	if rcHasIntegration() {
		t.Error("no rc file: not installed")
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("export A=1\n"+setup.HookLine("zsh")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !rcHasIntegration() {
		t.Error("the line is there: installed")
	}
	t.Setenv("SHELL", "/usr/bin/fish")
	if rcHasIntegration() {
		t.Error("an unknown shell has no rc line")
	}
}

func TestStatusAndPauseSayRestartWhenTheRCLineExists(t *testing.T) {
	home := isolate(t)
	_ = home
	h := t.TempDir()
	t.Setenv("HOME", h)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	inSession(t, "acme")
	t.Setenv(EnvCDFile, "")

	_, next := commandsSummary(".", false)
	if next != "ars setup" {
		t.Errorf("next = %q", next)
	}
	if out, code := run(t, "pause"); code == 0 || !strings.Contains(out, "run ars setup") {
		t.Errorf("pause without the line: code %d: %s", code, out)
	}

	if err := os.WriteFile(filepath.Join(h, ".zshrc"), []byte(setup.HookLine("zsh")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	v, next := commandsSummary(".", false)
	if !strings.Contains(v, "restart the terminal to enable shell integration") || next != "" {
		t.Errorf("status: %q -> %q", v, next)
	}
	if out, code := run(t, "pause"); code == 0 || !strings.Contains(out, "restart the terminal") || strings.Contains(out, "ars setup") {
		t.Errorf("pause with the line: code %d: %s", code, out)
	}
}
