// Runs ars init and ars rec in real zsh and bash shells with the integration loaded.

package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/ynn3m/arsenal/internal/cast"
	"github.com/ynn3m/arsenal/internal/cmdlog"
)

type shellProc struct {
	t       *testing.T
	cmd     *exec.Cmd
	ptmx    *os.File
	mu      sync.Mutex
	buf     bytes.Buffer
	seq     int
	log     string
	dir     string
	onWrite func([]byte)
}

func (s *shellProc) Write(p []byte) (int, error) {
	s.mu.Lock()
	n, err := s.buf.Write(p)
	f := s.onWrite
	s.mu.Unlock()
	if f != nil {
		f(p)
	}
	return n, err
}

func (s *shellProc) output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

func (s *shellProc) send(text string) {
	s.t.Helper()
	if _, err := io.WriteString(s.ptmx, text); err != nil {
		s.t.Fatalf("write to shell: %v", err)
	}
}

func (s *shellProc) waitFor(sub string, d time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if strings.Contains(s.output(), sub) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	s.t.Fatalf("timeout waiting for %q; output so far:\n%s", sub, s.output())
}

func (s *shellProc) sync() {
	s.t.Helper()
	s.seq++
	tag := fmt.Sprintf("SYNC%d", s.seq)
	s.send("printf '__%s__\\n' " + tag + "\r")
	s.waitFor("__"+tag+"__", 10*time.Second)
	time.Sleep(30 * time.Millisecond)
}

func (s *shellProc) run(line string) {
	s.t.Helper()
	s.send(line + "\r")
	s.sync()
}

func (s *shellProc) records() ([]cmdlog.Record, int) {
	s.t.Helper()
	raw, err := os.ReadFile(s.log)
	if err != nil {
		s.t.Fatal(err)
	}
	return cmdlog.Parse(raw)
}

func (s *shellProc) entries() []cmdlog.Entry {
	s.t.Helper()
	recs, bad := s.records()
	if bad != 0 {
		raw, _ := os.ReadFile(s.log)
		s.t.Fatalf("%d invalid lines in the log:\n%s", bad, raw)
	}
	var out []cmdlog.Entry
	for _, e := range cmdlog.Fold(recs) {
		if !strings.HasPrefix(e.Start.Cmd, "printf '__%s__") {
			out = append(out, e)
		}
	}
	return out
}

func cmds(es []cmdlog.Entry) []string {
	var out []string
	for _, e := range es {
		out = append(out, e.Start.Cmd)
	}
	return out
}

func startShell(t *testing.T, kind, bin string) *shellProc {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".arsenal"), 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, ".arsenal", "commands.jsonl")
	if err := os.WriteFile(logPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".arsenal", "session.yml"), []byte("name: acme\ncreated: 2026-11-24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hooks := zshHooks
	args := []string{"-f", "-i"}
	if kind == "bash" {
		hooks = bashHooks
		args = []string{"--norc", "--noprofile", "-i"}
	}
	snippet := filepath.Join(dir, "hooks.sh")
	if err := os.WriteFile(snippet, []byte(hooks), 0o600); err != nil {
		t.Fatal(err)
	}
	c := exec.Command(bin, args...)
	c.Dir = dir
	c.Env = []string{
		"HOME=" + dir, "PATH=" + os.Getenv("PATH"), "TERM=xterm", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8",
		"PS1=$ ", "HISTFILE=" + filepath.Join(dir, "hist"),
	}
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 40, Cols: 200})
	if err != nil {
		t.Fatal(err)
	}
	s := &shellProc{t: t, cmd: c, ptmx: ptmx, log: logPath, dir: dir}
	go func() { _, _ = io.Copy(s, ptmx) }()
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
	})
	time.Sleep(400 * time.Millisecond)
	s.send("source " + snippet + "\r")
	time.Sleep(300 * time.Millisecond)
	s.sync()
	return s
}

type shellCase struct{ kind, bin, name string }

func installedShells() []shellCase {
	var out []shellCase
	if p, err := exec.LookPath("zsh"); err == nil {
		out = append(out, shellCase{"zsh", p, "zsh"})
	}
	seen := map[string]bool{}
	for _, p := range []string{"/bin/bash", "/opt/homebrew/bin/bash", "/usr/local/bin/bash", "/usr/bin/bash"} {
		real, err := filepath.EvalSymlinks(p)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		v, err := exec.Command(p, "-c", "echo ${BASH_VERSINFO[0]}.${BASH_VERSINFO[1]}").Output()
		if err != nil {
			continue
		}
		out = append(out, shellCase{"bash", p, "bash-" + strings.TrimSpace(string(v)) + "-" + filepath.Dir(p)})
	}
	return out
}

func buildArs(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("go", "build", "-o", filepath.Join(bin, "ars"), "../../cmd/ars").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}

func spawnShell(t *testing.T, kind, bin, dir string, env []string) *shellProc {
	t.Helper()
	args := []string{"-f", "-i"}
	if kind == "bash" {
		args = []string{"--norc", "--noprofile", "-i"}
	}
	c := exec.Command(bin, args...)
	c.Dir = dir
	c.Env = env
	ptmx, err := pty.StartWithSize(c, &pty.Winsize{Rows: 40, Cols: 200})
	if err != nil {
		t.Fatal(err)
	}
	s := &shellProc{t: t, cmd: c, ptmx: ptmx, dir: dir}
	go func() { _, _ = io.Copy(s, ptmx) }()
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = c.Process.Kill()
		_, _ = c.Process.Wait()
	})
	time.Sleep(400 * time.Millisecond)
	return s
}

func TestInitEndToEnd(t *testing.T) {
	shells := installedShells()
	if len(shells) == 0 {
		t.Skip("no zsh or bash installed")
	}
	binDir := buildArs(t)
	for _, c := range shells {
		c := c
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			seedConfig(t, home)
			pentest := filepath.Join(t.TempDir(), "pentest")
			env := []string{
				"HOME=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "PENTEST_PATH=" + pentest,
				"PATH=" + binDir + ":" + os.Getenv("PATH"), "TERM=dumb", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8",
				"PS1=$ ", "HISTFILE=" + filepath.Join(home, "hist"), "TMPDIR=" + os.TempDir(), "COLORFGBG=15;0",
			}
			load := `eval "$(command ars hook ` + c.kind + `)"`

			s := spawnShell(t, c.kind, c.bin, home, env)
			s.run(load)
			if strings.Contains(s.output(), "[ars]") {
				t.Errorf("a banner was printed:\n%s", s.output())
			}
			s.run("echo before-init")
			s.run("ars init acme")
			s.run("echo PWD_IS=$PWD")
			s.waitFor("PWD_IS=", 5*time.Second)
			sessDir := filepath.Join(pentest, "acme")
			logPath := cmdlog.Path(sessDir)
			fi, err := os.Stat(logPath)
			if err != nil {
				t.Fatalf("command log was not created by ars init: %v", err)
			}
			if fi.Mode().Perm() != 0o600 {
				t.Errorf("command log mode %v, want 0600", fi.Mode().Perm())
			}
			s.log = logPath
			s.run("echo first-terminal-command")
			got := cmds(s.entries())
			if !contains(got, "echo first-terminal-command") {
				t.Errorf("command not logged after ars init: %q", got)
			}
			if contains(got, "echo before-init") {
				t.Errorf("a command typed outside a session was logged: %q", got)
			}

			s.run("mkdir -p scans/nmap && cd scans/nmap")
			s.run("echo from-subfolder")
			if !contains(cmds(s.entries()), "echo from-subfolder") {
				t.Errorf("a subfolder is not part of the session: %q", cmds(s.entries()))
			}
			s.run("ars vars --plain")

			s2 := spawnShell(t, c.kind, c.bin, home, env)
			s2.log = logPath
			s2.run(load)
			if strings.Contains(s2.output(), "[ars]") {
				t.Errorf("a banner was printed:\n%s", s2.output())
			}
			n := len(s2.entries())
			s2.run("echo outside")
			if got := len(s2.entries()); got != n {
				t.Errorf("a terminal outside a session logged %d commands", got-n)
			}
			s2.run("cd " + sessDir)
			s2.run("echo second-terminal-command")
			if !contains(cmds(s2.entries()), "echo second-terminal-command") {
				t.Errorf("cd into the session does not start logging: %q", cmds(s2.entries()))
			}
			s2.run("cd " + home)
			before := len(s2.output())
			s2.run("ars set a=1")
			if out := s2.output()[before:]; !strings.Contains(out, "not in a session folder") {
				t.Errorf("ars set outside a session:\n%s", out)
			}

			s.run("ars pause")
			s.waitFor("Paused:", 5*time.Second)
			s.run("echo hidden-while-paused")
			s.run("ars pause")
			s.waitFor("Resumed:", 5*time.Second)
			s.run("echo visible-after-unpause")
			got = cmds(s.entries())
			if contains(got, "echo hidden-while-paused") || !contains(got, "echo visible-after-unpause") {
				t.Errorf("pause did not work: %q", got)
			}

			s.run("cd " + home)
			s.run("ars init other")
			s.run("echo in-other-session")
			otherLog := cmdlog.Path(filepath.Join(pentest, "other"))
			raw, err := os.ReadFile(otherLog)
			if err != nil || !strings.Contains(string(raw), "echo in-other-session") {
				t.Errorf("other session log: %v\n%s", err, raw)
			}
			if b, _ := os.ReadFile(logPath); strings.Contains(string(b), "in-other-session") {
				t.Error("a command of the other session was logged in acme")
			}
		})
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func TestBashKeepsExistingDebugTrap(t *testing.T) {
	var bash *shellCase
	for _, c := range installedShells() {
		if c.kind == "bash" {
			c := c
			bash = &c
			break
		}
	}
	if bash == nil {
		t.Skip("bash not installed")
	}
	s := startShell(t, "bash", bash.bin)
	s.run("unset _ARS_HOOKS; trap 'OLD_TRAP_RAN=yes' DEBUG")
	s.send("eval \"$(cat " + filepath.Join(s.dir, "hooks.sh") + ")\"\r")
	s.sync()
	s.run("echo chained")
	s.send("echo OLD=${OLD_TRAP_RAN:-no}\r")
	s.waitFor("OLD=yes", 5*time.Second)
	s.sync()
	if !contains(cmds(s.entries()), "echo chained") {
		t.Errorf("command not logged with a previous DEBUG trap: %q", cmds(s.entries()))
	}
}

func TestRecEndToEnd(t *testing.T) {
	shells := installedShells()
	if len(shells) == 0 {
		t.Skip("no zsh or bash installed")
	}
	binDir := buildArs(t)
	for _, c := range shells {
		c := c
		t.Run(c.name, func(t *testing.T) {
			home := t.TempDir()
			seedConfig(t, home)
			pentest := filepath.Join(t.TempDir(), "pentest")
			rc := "PS1='$ '\nPROMPT='$ '\neval \"$(command ars hook " + c.kind + ")\"\n"
			if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(rc), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, ".bashrc"), []byte(rc), 0o600); err != nil {
				t.Fatal(err)
			}
			shellArgs := ""
			if c.kind == "bash" {
				shellArgs = "--rcfile " + filepath.Join(home, ".bashrc")
			}
			wrapper := filepath.Join(home, "myshell")
			if err := os.WriteFile(wrapper, []byte("#!/bin/sh\nexec "+c.bin+" "+shellArgs+" \"$@\"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			env := []string{
				"HOME=" + home, "ZDOTDIR=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "PENTEST_PATH=" + pentest,
				"PATH=" + binDir + ":" + os.Getenv("PATH"), "TERM=dumb", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8",
				"PS1=$ ", "HISTFILE=" + filepath.Join(home, "hist"), "TMPDIR=" + os.TempDir(), "COLORFGBG=15;0",
				"SHELL=" + wrapper,
			}
			s := spawnShell(t, c.kind, c.bin, home, env)
			s.run(`eval "$(command ars hook ` + c.kind + `)"`)
			s.run("ars init acme")
			logPath := cmdlog.Path(filepath.Join(pentest, "acme"))
			s.log = logPath

			s.send("ars rec\r")
			s.waitFor("Recording session acme to", 10*time.Second)
			time.Sleep(1200 * time.Millisecond)
			s.run("echo inside-the-recording")
			s.run("ars pause")
			s.run("echo secret-while-paused")
			s.run("ars pause")
			s.run("echo after-unpause")
			s.run("ars init other")
			s.waitFor("this terminal is recording session acme: type exit to stop, then run ars init other", 5*time.Second)
			s.send("exit\r")
			s.waitFor("Recording saved:", 10*time.Second)
			s.sync()

			casts, _ := filepath.Glob(filepath.Join(pentest, "acme", ".arsenal", "recordings", "*.cast"))
			if len(casts) != 1 {
				t.Fatalf("casts = %v", casts)
			}
			f, err := os.Open(casts[0])
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			r, err := cast.NewReader(f)
			if err != nil {
				t.Fatal(err)
			}
			var shown strings.Builder
			var markers []string
			for _, e := range r.ReadAll() {
				switch e.Code {
				case cast.CodeOutput:
					shown.WriteString(e.Data)
				case cast.CodeMarker:
					markers = append(markers, e.Data)
				}
			}
			if strings.Join(markers, "|") != "recording paused|recording resumed" {
				t.Errorf("markers = %q", markers)
			}
			if !strings.Contains(shown.String(), "inside-the-recording") || !strings.Contains(shown.String(), "after-unpause") {
				t.Errorf("cast lacks the output:\n%q", shown.String())
			}
			if strings.Contains(shown.String(), "secret-while-paused") || strings.Contains(shown.String(), "7777") {
				t.Errorf("the cast holds paused output or our private sequence:\n%q", shown.String())
			}
			if !strings.Contains(s.output(), "secret-while-paused") {
				t.Error("the user did not see the output while paused")
			}

			id := strings.TrimSuffix(filepath.Base(casts[0]), ".cast")
			recs, bad := s.records()
			if bad != 0 {
				t.Fatalf("%d bad lines", bad)
			}
			var gotStart, gotEnd, gotCmd bool
			for _, rc := range recs {
				if rc.Cmd == "echo secret-while-paused" {
					t.Error("a command typed while paused was logged")
				}
				switch {
				case rc.Ev == cmdlog.EvRecStart && rc.Rec == id:
					gotStart = true
				case rc.Ev == cmdlog.EvRecEnd && rc.Rec == id:
					gotEnd = true
				case rc.Ev == cmdlog.EvStart && rc.Cmd == "echo inside-the-recording":
					gotCmd = rc.Rec == id && rc.Session == "acme"
				}
			}
			if !gotStart || !gotEnd || !gotCmd {
				t.Errorf("command log: rec_start=%v rec_end=%v command-with-rec-id=%v\n%+v", gotStart, gotEnd, gotCmd, recs)
			}
			fi, _ := os.Stat(casts[0])
			if fi.Mode().Perm() != 0o600 {
				t.Errorf("cast mode %v", fi.Mode().Perm())
			}
			s.run("ars replay --text " + id + " | grep -c after-unpause")
			s.send("ars replay --speed 1000 " + id + "\r")
			s.waitFor("Playing", 10*time.Second)
			time.Sleep(1500 * time.Millisecond)
			s.sync()
			if n := strings.Count(s.output(), "Playing"); n != 1 {
				t.Errorf("playback header seen %d times", n)
			}
			before := strings.Count(s.output(), "after-unpause")
			if before < 3 {
				t.Errorf("the replay did not show the recorded output (seen %d times)", before)
			}

			s.run("ars history --list | grep -c inside-the-recording")
			out := s.output()
			if !strings.Contains(out, "inside-the-recording") {
				t.Error("history --list did not show the recorded command")
			}
			s.run(`[ "$(ars history --json --all-commands | grep -c 'secret-while-pause[d]')" = 0 ] && echo LOG_CLEAN_$((1+1))`)
			if !strings.Contains(s.output(), "LOG_CLEAN_2") {
				t.Error("a command typed while paused is in the history")
			}

			s.send("ars rec -- true\r")
			s.waitFor("Recording saved:", 10*time.Second)
		})
	}
}

func TestInsertedCommandReachesBothLogs(t *testing.T) {
	var zsh *shellCase
	for _, c := range installedShells() {
		if c.kind == "zsh" {
			c := c
			zsh = &c
		}
	}
	if zsh == nil {
		t.Skip("zsh not installed")
	}
	binDir := buildArs(t)
	home := t.TempDir()
	seedConfig(t, home)
	pentest := filepath.Join(t.TempDir(), "pentest")
	cheats := filepath.Join(home, ".cheats")
	if err := os.MkdirAll(cheats, 0o700); err != nil {
		t.Fatal(err)
	}
	sheet := "# demo\n\n## say hello\n```\necho hello-from-ars\n```\n"
	if err := os.WriteFile(filepath.Join(cheats, "demo.md"), []byte(sheet), 0o600); err != nil {
		t.Fatal(err)
	}
	rc := "PROMPT='$ '\neval \"$(command ars hook zsh)\"\n"
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(rc), 0o600); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"HOME=" + home, "ZDOTDIR=" + home, "XDG_CONFIG_HOME=" + filepath.Join(home, ".config"), "PENTEST_PATH=" + pentest,
		"PATH=" + binDir + ":" + os.Getenv("PATH"), "TERM=dumb", "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8",
		"HISTFILE=" + filepath.Join(home, "hist"), "TMPDIR=" + os.TempDir(), "COLORFGBG=15;0", "SHELL=" + zsh.bin,
	}
	s := spawnShell(t, "zsh", zsh.bin, home, env)
	s.run(`eval "$(command ars hook zsh)"`)
	s.run("ars init acme")
	s.log = cmdlog.Path(filepath.Join(pentest, "acme"))
	s.send("ars rec\r")
	s.waitFor("Recording session acme to", 10*time.Second)
	time.Sleep(1200 * time.Millisecond)

	s.send(`ars -s "say hello"` + "\r")
	time.Sleep(1500 * time.Millisecond)
	s.send("\r")
	time.Sleep(1000 * time.Millisecond)
	s.send("\r")
	s.waitFor("hello-from-ars", 5*time.Second)
	time.Sleep(400 * time.Millisecond)
	s.sync()
	s.send("exit\r")
	s.waitFor("Recording saved:", 10*time.Second)

	recs, bad := s.records()
	if bad != 0 {
		t.Fatalf("%d bad lines", bad)
	}
	var insert, ran *cmdlog.Record
	for i := range recs {
		r := recs[i]
		if r.Ev == cmdlog.EvInsert && r.Cmd == "echo hello-from-ars" {
			insert = &recs[i]
		}
		if r.Ev == cmdlog.EvStart && r.Cmd == "echo hello-from-ars" {
			ran = &recs[i]
		}
	}
	if insert == nil || insert.How != cmdlog.HowPrompt || insert.Cheat != "say hello" || insert.Rec == "" {
		t.Fatalf("insert line: %+v\nlog: %+v", insert, recs)
	}
	if ran == nil || ran.Rec != insert.Rec {
		t.Fatalf("run line: %+v", ran)
	}
	var via string
	for _, e := range cmdlog.Fold(recs) {
		if e.Start.Cmd == "echo hello-from-ars" {
			via = e.Via
		}
	}
	if via != cmdlog.ViaArs {
		t.Errorf("via = %q, want ars", via)
	}

	casts, _ := filepath.Glob(filepath.Join(pentest, "acme", ".arsenal", "recordings", "*.cast"))
	if len(casts) != 1 {
		t.Fatalf("casts = %v", casts)
	}
	f, _ := os.Open(casts[0])
	defer f.Close()
	r, err := cast.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var shown strings.Builder
	var markers []string
	for _, e := range r.ReadAll() {
		switch e.Code {
		case cast.CodeOutput:
			shown.WriteString(e.Data)
		case cast.CodeMarker:
			markers = append(markers, e.Data)
		}
	}
	if strings.Count(shown.String(), "echo hello-from-ars") < 1 || !strings.Contains(shown.String(), "hello-from-ars\r\n") {
		t.Errorf("cast lacks the injected command or its output:\n%q", shown.String())
	}
	if len(markers) != 1 || markers[0] != "ars: prompt: say hello" {
		t.Errorf("markers = %q", markers)
	}
}

func seedConfig(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "arsenal")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("# test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
