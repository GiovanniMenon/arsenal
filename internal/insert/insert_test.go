package insert

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeTool(t *testing.T, dir, name, out string) {
	t.Helper()
	script := "#!/bin/sh\n/bin/cat > '" + out + "'\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func isolatePath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("WAYLAND_DISPLAY", "")
	return dir
}

func TestCopyUsesAvailableTool(t *testing.T) {
	dir := isolatePath(t)
	out := filepath.Join(t.TempDir(), "clip")
	fakeTool(t, dir, "xclip", out)
	if err := Copy("nmap -sV 10.10.10.5"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "nmap -sV 10.10.10.5" {
		t.Errorf("clipboard = %q", b)
	}
}

func TestCopyPrefersPbcopyThenWayland(t *testing.T) {
	dir := isolatePath(t)
	o1, o2 := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
	fakeTool(t, dir, "pbcopy", o1)
	fakeTool(t, dir, "xclip", o2)
	if err := Copy("x"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(o1); err != nil {
		t.Error("pbcopy should win")
	}

	os.Remove(filepath.Join(dir, "pbcopy"))
	fakeTool(t, dir, "wl-copy", o1)
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	os.Remove(o1)
	if err := Copy("y"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(o1); string(b) != "y" {
		t.Error("wl-copy should win under Wayland")
	}
}

func TestCopyWithoutToolFails(t *testing.T) {
	isolatePath(t)
	if err := Copy("x"); !errors.Is(err, ErrNoClipboard) {
		t.Errorf("err = %v", err)
	}
}

func TestCopyFallsThroughAFailingTool(t *testing.T) {
	dir := isolatePath(t)
	if err := os.WriteFile(filepath.Join(dir, "pbcopy"), []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "clip")
	fakeTool(t, dir, "xclip", out)
	if err := Copy("z"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(out); string(b) != "z" {
		t.Errorf("clipboard = %q", b)
	}
}

func stubPrompt(t *testing.T, inject func(string) error, cp func(string) error) {
	t.Helper()
	oi, oc := injectFn, copyFn
	injectFn, copyFn = inject, cp
	t.Cleanup(func() { injectFn, copyFn = oi, oc })
}

func TestToPromptUsesTIOCSTIFirst(t *testing.T) {
	var got string
	stubPrompt(t, func(s string) error { got = s; return nil }, func(string) error { t.Error("clipboard used"); return nil })
	t.Setenv(EnvCmdFile, filepath.Join(t.TempDir(), "cmd"))
	m, hint, err := ToPrompt("ls -la")
	if err != nil || m != MethodTIOCSTI || hint != "" || got != "ls -la" {
		t.Fatalf("%v %q %v got=%q", m, hint, err, got)
	}
	if b, _ := os.ReadFile(os.Getenv(EnvCmdFile)); len(b) != 0 {
		t.Error("shell function file must stay empty when TIOCSTI works")
	}
}

func TestToPromptFallsBackToShellFunction(t *testing.T) {
	stubPrompt(t, func(string) error { return ErrInjectBlocked }, func(string) error { t.Error("clipboard used"); return nil })
	f := filepath.Join(t.TempDir(), "cmd")
	t.Setenv(EnvCmdFile, f)
	m, hint, err := ToPrompt("nmap -p 22 10.10.10.5")
	if err != nil || m != MethodShell || hint != "" {
		t.Fatalf("%v %q %v", m, hint, err)
	}
	if b, _ := os.ReadFile(f); string(b) != "nmap -p 22 10.10.10.5" {
		t.Errorf("cmd file = %q", b)
	}
}

func TestToPromptFallsBackToClipboardWithHint(t *testing.T) {
	var copied string
	stubPrompt(t, func(string) error { return ErrInjectBlocked }, func(s string) error { copied = s; return nil })
	t.Setenv(EnvCmdFile, "")
	m, hint, err := ToPrompt("id")
	if err != nil || m != MethodClipboard || copied != "id" {
		t.Fatalf("%v %v copied=%q", m, err, copied)
	}
	if !strings.Contains(hint, "copied") || !strings.Contains(hint, "ars setup") {
		t.Errorf("hint = %q", hint)
	}
}

func TestToPromptReportsWhenEverythingFails(t *testing.T) {
	stubPrompt(t, func(string) error { return ErrInjectBlocked }, func(string) error { return ErrNoClipboard })
	t.Setenv(EnvCmdFile, "")
	if _, hint, err := ToPrompt("id"); err == nil || hint == "" {
		t.Errorf("err=%v hint=%q", err, hint)
	}
}

func TestToPromptDoesNotMaskOtherErrors(t *testing.T) {
	boom := errors.New("boom")
	stubPrompt(t, func(string) error { return boom }, func(string) error { t.Error("clipboard used"); return nil })
	if _, _, err := ToPrompt("id"); !errors.Is(err, boom) {
		t.Errorf("err = %v", err)
	}
}

func TestInjectWithoutTerminalIsBlocked(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "notatty")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := Inject(f, "x"); !errors.Is(err, ErrInjectBlocked) {
		t.Errorf("err = %v", err)
	}
}

func TestWriteFileIsPrivateAndHasNoNewline(t *testing.T) {
	p := filepath.Join(t.TempDir(), "out")
	if err := WriteFile(p, "ls -la"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	fi, _ := os.Stat(p)
	if string(b) != "ls -la" || fi.Mode().Perm() != 0o600 {
		t.Errorf("content %q mode %o", b, fi.Mode().Perm())
	}
}

func TestRunExecutesAndIgnoresExitStatus(t *testing.T) {
	var out bytes.Buffer
	if err := Run("echo hi; exit 3", strings.NewReader(""), &out, &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hi\n" {
		t.Errorf("out = %q", out.String())
	}
	out.Reset()
	if err := Run("echo one;\\\necho two", strings.NewReader(""), &out, &out); err != nil || out.String() != "one\ntwo\n" {
		t.Errorf("multi-line: %q %v", out.String(), err)
	}
}
