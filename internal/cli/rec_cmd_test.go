package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/rec"
	"github.com/ynn3m/arsenal/internal/session"
)

type fakeRecorder struct {
	t      *testing.T
	calls  []rec.Options
	result rec.Result
	err    error
	onCall func(rec.Options)
}

func setupRec(t *testing.T) *fakeRecorder {
	t.Helper()
	isolate(t)
	t.Setenv(rec.EnvRec, "")
	f := &fakeRecorder{t: t}
	runRecorder = func(o rec.Options) (rec.Result, error) {
		f.calls = append(f.calls, o)
		if f.onCall != nil {
			f.onCall(o)
		}
		return f.result, f.err
	}
	t.Cleanup(func() { runRecorder = rec.Run })
	return f
}

func writeConfig(t *testing.T, text string) {
	t.Helper()
	d, _ := config.ConfigDir()
	if err := os.MkdirAll(d, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "config.yml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestRecUsesTheSessionOfTheFolderAndPassesTheOptions(t *testing.T) {
	f := setupRec(t)
	inSession(t, "acme")
	if out, code := run(t, "rec"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	dir, _ := session.Dir("acme")
	o := f.calls[0]
	if o.Session != "acme" || o.Dir != dir || len(o.Command) != 0 || o.NoLog || o.Input || o.MaxSize != 256<<20 || o.WorkDir != "" {
		t.Errorf("options = %+v", o)
	}
	run(t, "rec", "--no-log", "--input", "--max-size", "5", "--", "nmap", "-p-", "10.10.10.5")
	o = f.calls[1]
	if !o.NoLog || !o.Input || o.MaxSize != 5<<20 || strings.Join(o.Command, " ") != "nmap -p- 10.10.10.5" {
		t.Errorf("options = %+v", o)
	}
}

func TestRecMaxSizeComesFromTheConfigFile(t *testing.T) {
	f := setupRec(t)
	inSession(t, "acme")
	writeConfig(t, "max_cast_mb: 64\n")
	run(t, "rec")
	if f.calls[0].MaxSize != 64<<20 {
		t.Errorf("MaxSize = %d", f.calls[0].MaxSize)
	}
}

func TestRecOfACommandReturnsItsStatusButARecordedShellReturnsZero(t *testing.T) {
	f := setupRec(t)
	inSession(t, "acme")
	f.result = rec.Result{ExitCode: 3}
	if _, code := run(t, "rec", "--", "false"); code != 3 {
		t.Errorf("command: exit code %d, want 3", code)
	}
	if _, code := run(t, "rec"); code != 0 {
		t.Errorf("shell: exit code %d, want 0", code)
	}
}

func TestRecNeedsASessionFolderAndStaysBoundAfterACd(t *testing.T) {
	f := setupRec(t)
	if out, code := run(t, "rec"); code == 0 || !strings.Contains(out, noSessionMsg) {
		t.Errorf("without a session: code %d %q", code, out)
	}
	dirA := inSession(t, "client-a")
	inSession(t, "client-b")
	run(t, "rec")
	if f.calls[0].Session != "client-b" {
		t.Errorf("recorded %q, want the session of the folder, client-b", f.calls[0].Session)
	}
	t.Setenv(rec.EnvRec, "/x/a.cast")
	t.Setenv("ARSENAL_SESSION_DIR", dirA)
	t.Setenv(EnvSession, "client-a")
	run(t, "rec")
	if f.calls[1].Session != "client-a" || f.calls[1].Dir != dirA {
		t.Errorf("recorded %+v, want the bound client-a", f.calls[1])
	}
}

func TestRecExplainsWhenThereIsNoTerminal(t *testing.T) {
	f := setupRec(t)
	inSession(t, "acme")
	f.err = rec.ErrNotTerminal
	out, code := run(t, "rec")
	if code == 0 || !strings.Contains(out, "needs a terminal") {
		t.Errorf("code %d %q", code, out)
	}
}

func TestStartRecStartsARecordedShellInTheSessionFolderAfterTheCD(t *testing.T) {
	f := setupRec(t)
	cdFile := filepath.Join(t.TempDir(), "cd")
	t.Setenv(EnvCDFile, cdFile)
	var cdAtCall string
	f.onCall = func(rec.Options) { b, _ := os.ReadFile(cdFile); cdAtCall = string(b) }

	if out, code := run(t, "init", "acme", "--rec"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	dir, _ := session.Dir("acme")
	if len(f.calls) != 1 || f.calls[0].WorkDir != dir || f.calls[0].Session != "acme" {
		t.Fatalf("calls = %+v", f.calls)
	}
	if cdAtCall != dir {
		t.Errorf("the cd request was %q when the recording started, want %q", cdAtCall, dir)
	}
	if out, code := run(t, "init", "acme", "--resume", "--rec"); code != 0 || len(f.calls) != 2 {
		t.Errorf("resume: code %d calls %d: %s", code, len(f.calls), out)
	}
}

func TestStartRecordsByDefaultOnlyWhenConfiguredAndNoRecOverrides(t *testing.T) {
	f := setupRec(t)
	run(t, "init", "plain")
	if len(f.calls) != 0 {
		t.Fatal("recorded without being asked")
	}
	writeConfig(t, "rec_by_default: true\n")
	run(t, "init", "auto")
	if len(f.calls) != 1 || f.calls[0].Session != "auto" {
		t.Fatalf("rec_by_default did not record: %+v", f.calls)
	}
	run(t, "init", "manual", "--no-rec")
	if len(f.calls) != 1 {
		t.Errorf("--no-rec did not override rec_by_default")
	}
}

func TestSwitchingSessionInsideARecordingIsRefused(t *testing.T) {
	setupRec(t)
	dir := inSession(t, "acme")
	run(t, "init", "other")
	t.Chdir(dir)
	t.Setenv(rec.EnvRec, "/x/acme.cast")
	t.Setenv(EnvSession, "acme")
	for _, args := range [][]string{{"init", "other", "--resume"}, {"init", "brand-new"}} {
		out, code := run(t, args...)
		want := "this terminal is recording session acme: type exit to stop, then run ars init " + args[1]
		if code == 0 || !strings.Contains(out, want) {
			t.Errorf("%v: code %d %q", args, code, out)
		}
	}
	if ok, _ := session.Exists("brand-new"); ok {
		t.Error("a session was created inside a recording")
	}
	if out, code := run(t, "init", "acme", "--resume"); code != 0 {
		t.Errorf("init of the recorded session itself must work: %q", out)
	}
}
