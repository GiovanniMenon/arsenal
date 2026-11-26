// Checks that the full-screen views work inside a recorded shell.

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type terminalEmulator struct {
	s *shellProc
}

func (e terminalEmulator) feed(p []byte) {
	if bytes.Contains(p, []byte("\x1b]11;?")) {
		_, _ = e.s.ptmx.Write([]byte("\x1b]11;rgb:1c1c/1c1c/1c1c\x1b\\"))
	}
	if bytes.Contains(p, []byte("\x1b[6n")) {
		_, _ = e.s.ptmx.Write([]byte("\x1b[1;1R"))
	}
}

func (s *shellProc) runAfter(line, waitFor string) {
	s.t.Helper()
	before := s.count(waitFor)
	s.send(line + "\r")
	s.waitCount(waitFor, before+1, 10*time.Second)
	time.Sleep(150 * time.Millisecond)
	s.sync()
}

func (s *shellProc) count(sub string) int { return strings.Count(s.output(), sub) }

func (s *shellProc) waitCount(sub string, n int, d time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if s.count(sub) >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("timeout waiting for %d x %q (saw %d); output tail:\n%s", n, sub, s.count(sub), tail(s.output(), 1500))
}

func tail(s string, n int) string {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

const (
	altOn  = "\x1b[?1049h"
	altOff = "\x1b[?1049l"
)

func TestTUIsWorkInsideARecordedShell(t *testing.T) {
	var zsh *shellCase
	for _, c := range installedShells() {
		if c.kind == "zsh" {
			c := c
			zsh = &c
			break
		}
	}
	if zsh == nil {
		t.Skip("zsh not installed")
	}
	binDir := buildArs(t)
	home := t.TempDir()
	seedConfig(t, home)
	pentest := filepath.Join(t.TempDir(), "pentest")
	cheats := filepath.Join(home, ".config", "arsenal", "cheats")
	if err := os.MkdirAll(cheats, 0o700); err != nil {
		t.Fatal(err)
	}
	sheet := "# demo\n\n## say hello\n```\necho hello-from-ars\n```\n"
	if err := os.WriteFile(filepath.Join(cheats, "demo.md"), []byte(sheet), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte("PROMPT='$ '\neval \"$(command ars hook zsh)\"\nprintf '\\033[?1004h'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"HOME=" + home, "ZDOTDIR=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "PENTEST_PATH=" + pentest,
		"PATH=" + binDir + ":" + os.Getenv("PATH"), "TERM=xterm-ghostty", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8",
		"HISTFILE=" + filepath.Join(home, "hist"), "TMPDIR=" + os.TempDir(), "SHELL=" + zsh.bin,
	}
	s := spawnShell(t, "zsh", zsh.bin, home, env)
	em := terminalEmulator{s: s}
	s.mu.Lock()
	s.onWrite = em.feed
	s.mu.Unlock()

	s.run(`eval "$(command ars hook zsh)"`)
	s.send("ars init acme --rec\r")
	s.waitFor("on (exit or ctrl-d stops it)", 10*time.Second)
	time.Sleep(1500 * time.Millisecond)
	s.log = filepath.Join(pentest, "acme", ".arsenal", "commands.jsonl")
	s.sync()
	s.runAfter(`ars note "first note"`, "Note added")
	s.run("echo something-for-the-history")

	check := func(name, cmd string, wantInFrame string, leave string) {
		t.Helper()
		on, off := s.count(altOn), s.count(altOff)
		from := len(s.output())
		s.send(cmd + "\r")
		s.waitCount(altOn, on+1, 10*time.Second)
		time.Sleep(900 * time.Millisecond)
		if s.count(altOff) != off {
			t.Fatalf("%s: left the alt screen by itself:\n%s", name, tail(s.output(), 1500))
		}
		if !strings.Contains(s.output()[from:], wantInFrame) {
			t.Fatalf("%s: nothing drawn (want %q):\n%s", name, wantInFrame, tail(s.output(), 1500))
		}
		s.send(leave)
		s.waitCount(altOff, off+1, 10*time.Second)
		s.sync()
	}
	check("search", `ars -s "say hello"`, "ARSENAL", "\x1b")
	check("notes", "ars notes", "notes of session", "\x1b")
	check("history", "ars history", "history", "\x1b")

	s.send(`ars -s "say hello"` + "\r")
	s.waitCount(altOn, 4, 10*time.Second)
	time.Sleep(700 * time.Millisecond)
	s.send("\r")
	s.waitCount(altOff, 4, 10*time.Second)
	s.waitFor("echo hello-from-ars", 5*time.Second)

	s.send("exit\r")
	s.waitFor("Recording saved:", 15*time.Second)
	time.Sleep(300 * time.Millisecond)
}
