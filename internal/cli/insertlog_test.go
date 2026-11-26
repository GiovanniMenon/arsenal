package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/rec"
	"github.com/ynn3m/arsenal/internal/session"
)

func insertsOf(t *testing.T, sess string) []cmdlog.Record {
	t.Helper()
	p := logPathOf(t, sess)
	recs, err := cmdlog.Load(p, 0)
	if err != nil {
		t.Fatal(err)
	}
	return cmdlog.Inserts(recs)
}

func pickAll(t *testing.T, args ...string) string {
	t.Helper()
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	inSession(t, "acme")
	out, code := run(t, args...)
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	return out
}

func TestInsertLineForPrintInAttachedTerminal(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	inSession(t, "acme")
	t.Setenv(EnvRecID, "20261124-140211-4242")
	if out, code := run(t, "-p"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	ins := insertsOf(t, "acme")
	if len(ins) != 1 {
		t.Fatalf("inserts = %+v", ins)
	}
	r := ins[0]
	if r.Cmd != "nmap -p- 10.10.10.5" || r.How != cmdlog.HowPrint || r.Cheat != "scan all ports" || r.Session != "acme" || r.Rec != "20261124-140211-4242" || r.TS == 0 || r.CWD == "" {
		t.Errorf("insert line: %+v", r)
	}
}

func TestInsertLineHowForEachDeliveryMethod(t *testing.T) {
	cases := []struct {
		name string
		args []string
		env  map[string]string
		want string
	}{
		{"shell function fallback", []string{}, map[string]string{"ARSENAL_CMD_FILE": "@file"}, cmdlog.HowShell},
		{"outfile", []string{"-o", "@file"}, nil, cmdlog.HowFile},
		{"tmux unavailable falls back to the prompt", []string{"-t"}, map[string]string{"TMUX": "", "ARSENAL_CMD_FILE": "@file"}, cmdlog.HowShell},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := setupSearch(t, "scan all ports")
			s.answers["target"] = "10.10.10.5"
			inSession(t, "acme")
			f := filepath.Join(t.TempDir(), "out")
			args := append([]string{}, c.args...)
			for i, a := range args {
				if a == "@file" {
					args[i] = f
				}
			}
			for k, v := range c.env {
				if v == "@file" {
					v = f
				}
				t.Setenv(k, v)
			}
			if out, code := run(t, args...); code != 0 {
				t.Fatalf("code %d: %s", code, out)
			}
			ins := insertsOf(t, "acme")
			if len(ins) != 1 || ins[0].How != c.want {
				t.Errorf("inserts = %+v, want how %q", ins, c.want)
			}
		})
	}
}

func TestDeliverCommandReportsExitStatusOfExec(t *testing.T) {
	isolate(t)
	cmd := &cobra.Command{}
	var buf strings.Builder
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	_, d, err := deliverCommand(cmd, "exit 3", &searchFlags{exec: true})
	if err != nil || d.How != cmdlog.HowExec || d.Exit == nil || *d.Exit != 3 {
		t.Errorf("delivery = %+v, %v", d, err)
	}
	_, d, _ = deliverCommand(cmd, "true", &searchFlags{exec: true})
	if d.Exit == nil || *d.Exit != 0 {
		t.Errorf("exit 0 must be reported as 0, got %+v", d.Exit)
	}
}

func TestInsertIsNotLoggedWhenPausedOrOutsideASession(t *testing.T) {
	cases := []struct {
		name string
		prep func(t *testing.T, sessDir string)
	}{
		{"paused", func(t *testing.T, _ string) { t.Setenv(EnvPaused, "1") }},
		{"outside a session folder", func(t *testing.T, _ string) { t.Chdir(t.TempDir()) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := setupSearch(t, "scan all ports")
			s.answers["target"] = "10.10.10.5"
			dir := inSession(t, "acme")
			c.prep(t, dir)
			if out, code := run(t, "-p"); code != 0 {
				t.Fatalf("code %d: %s", code, out)
			}
			if ins := insertsOf(t, "acme"); len(ins) != 0 {
				t.Errorf("logged although it must not: %+v", ins)
			}
		})
	}
}

func TestInsertIsLoggedToTheSessionOfTheFolderWithoutShellIntegration(t *testing.T) {
	pickAll(t, "-p")
	ins := insertsOf(t, "acme")
	if len(ins) != 1 || ins[0].Session != "acme" || ins[0].How != cmdlog.HowPrint {
		t.Errorf("inserts = %+v", ins)
	}
}

func TestInsertInASubfolderGoesToTheSession(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	dir := inSession(t, "acme")
	sub := filepath.Join(dir, "scans")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	if out, code := run(t, "-p"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if ins := insertsOf(t, "acme"); len(ins) != 1 || !strings.HasSuffix(ins[0].CWD, "scans") {
		t.Errorf("inserts = %+v", ins)
	}
}

func TestRecordedTerminalInsertsGoToItsBoundSessionNotTheFolder(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	dirA := inSession(t, "client-a")
	inSession(t, "client-b")
	t.Setenv(rec.EnvRec, "/x/a.cast")
	t.Setenv("ARSENAL_SESSION_DIR", dirA)
	if out, code := run(t, "-p"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if n := len(insertsOf(t, "client-a")); n != 1 {
		t.Errorf("client-a inserts = %d, want 1", n)
	}
	if n := len(insertsOf(t, "client-b")); n != 0 {
		t.Errorf("client-b got %d inserts of a recording bound to client-a", n)
	}
}

func TestBrokenLogWarnsButNeverBlocksTheCommand(t *testing.T) {
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	inSession(t, "acme")
	dir, _ := session.Dir("acme")
	logPath := cmdlog.Path(dir)
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(logPath, 0o700); err != nil {
		t.Fatal(err)
	}
	out, code := run(t, "-p")
	if code != 0 || !strings.Contains(out, "nmap -p- 10.10.10.5") || !strings.Contains(out, "command log") {
		t.Errorf("code %d, want the command printed and a warning: %q", code, out)
	}
}
