package cli

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/rec"
)

func statusEnv(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range []string{EnvSession, EnvPaused, EnvCDFile, rec.EnvRec, "ARSENAL_SESSION_DIR"} {
		t.Setenv(k, "")
	}
	for k, v := range env {
		t.Setenv(k, v)
	}
}

func rowOf(out, label string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), label+" ") {
			return l
		}
	}
	return ""
}

const sshSentence = "SSH or a reverse shell"

func TestStatusOutsideASessionShowsOnlySessionNoneGlobalsAndTheHint(t *testing.T) {
	isolate(t)
	statusEnv(t, map[string]string{EnvCDFile: "/tmp/x"})
	run(t, "set", "-g", "a=1", "b=2")
	out, code := run(t, "status")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if r := rowOf(out, "Session"); !strings.Contains(r, "none") || !strings.Contains(r, "-> ars init <name>") {
		t.Errorf("Session row = %q\n%s", r, out)
	}
	if r := rowOf(out, "Variables"); !strings.Contains(r, "2 global") {
		t.Errorf("Variables row = %q", r)
	}
	for _, gone := range []string{"Paused", "Commands", "Recording", "Notes", "commands.jsonl", "Data", sshSentence} {
		if strings.Contains(out, gone) {
			t.Errorf("outside a session, %q must not be shown:\n%s", gone, out)
		}
	}
}

func TestStatusInASessionHasOneAlignedRowPerItemWithTheNextCommand(t *testing.T) {
	isolate(t)
	dir := inSession(t, "acme")
	statusEnv(t, map[string]string{EnvCDFile: "/tmp/x"})
	run(t, "set", "-g", "g=1")
	run(t, "set", "target=10.10.10.5", "ports=22")
	run(t, "note", "first")
	run(t, "note", "second")
	p := logPathOf(t, "acme")
	now := time.Now()
	for i, c := range []string{"nmap 10.10.10.5", "id"} {
		ts := now.Add(time.Duration(i-2) * time.Second).UnixMilli()
		if err := cmdlog.Append(p, cmdlog.Record{Ev: cmdlog.EvStart, ID: "1." + string(rune('1'+i)), TS: ts, Cmd: c}); err != nil {
			t.Fatal(err)
		}
	}
	makeCast(t, "acme", "20261124-100000-1", now.Add(-time.Hour).Unix(), map[float64]string{0.1: "hello"})
	makeCast(t, "acme", "20261124-110000-2", now.Add(-30*time.Minute).Unix(), map[float64]string{0.1: "hello"})

	out, code := run(t, "status")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	last := now.Add(-1 * time.Second).Format("15:04")
	want := map[string][]string{
		"Session":   {"acme", dir[len(dir)-4:]},
		"Paused":    {"no", "-> ars pause"},
		"Commands":  {"2 logged, last " + last, "-> ars history"},
		"Recording": {"no - 2 recordings, ", "last ", "-> ars rec / ars replay"},
		"Notes":     {"2 notes, last ", "-> ars notes"},
		"Variables": {"2 local, 1 global", "-> ars vars / ars set"},
	}
	for label, subs := range want {
		r := rowOf(out, label)
		if r == "" {
			t.Errorf("no %s row:\n%s", label, out)
			continue
		}
		for _, sub := range subs {
			if !strings.Contains(r, sub) {
				t.Errorf("%s row %q lacks %q", label, r, sub)
			}
		}
	}
	if r := rowOf(out, "Data"); !strings.Contains(r, filepath.Join("acme", ".arsenal")) {
		t.Errorf("Data row = %q", r)
	}
	if strings.Contains(out, "Recordings") || strings.Contains(out, "(not created") || strings.Contains(out, "commands.jsonl\n") {
		t.Errorf("old lines are still there:\n%s", out)
	}
	col := -1
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, "->"); i >= 0 {
			c := utf8.RuneCountInString(l[:i])
			if col >= 0 && c != col {
				t.Errorf("hints are not aligned (column %d vs %d):\n%s", c, col, out)
			}
			col = c
		}
	}
	if n := strings.Count(out, sshSentence); n != 1 {
		t.Errorf("SSH sentence shown %d times:\n%s", n, out)
	}
}

func TestStatusEmptySessionAndLoggingStates(t *testing.T) {
	isolate(t)
	inSession(t, "acme")
	statusEnv(t, map[string]string{EnvCDFile: "/tmp/x"})
	out, _ := run(t, "status")
	for label, want := range map[string]string{
		"Commands": "none logged yet", "Recording": "no - none yet", "Notes": "none yet", "Variables": "0 local, 0 global",
	} {
		if r := rowOf(out, label); !strings.Contains(r, want) {
			t.Errorf("%s row = %q, want %q", label, r, want)
		}
	}
	statusEnv(t, nil)
	out, _ = run(t, "status")
	if r := rowOf(out, "Commands"); !strings.Contains(r, "shell integration is off") || !strings.Contains(r, "-> ars setup") {
		t.Errorf("Commands row without integration = %q", r)
	}
}

func TestStatusPausedAndRecordingStates(t *testing.T) {
	isolate(t)
	dirA := inSession(t, "acme")
	root := filepath.Dir(dirA)

	statusEnv(t, map[string]string{EnvCDFile: "/tmp/x", EnvPaused: "1"})
	if out, _ := run(t, "status"); !strings.Contains(rowOf(out, "Paused"), "yes") {
		t.Errorf("paused:\n%s", out)
	}

	statusEnv(t, map[string]string{EnvCDFile: "/tmp/x", rec.EnvRec: "/x/y.cast", "ARSENAL_SESSION_DIR": dirA})
	out, _ := run(t, "status")
	if r := rowOf(out, "Recording"); !strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(r), "Recording")), "yes") {
		t.Errorf("Recording row = %q", r)
	}
	if strings.Contains(out, sshSentence) {
		t.Errorf("the SSH sentence must only show when not recording:\n%s", out)
	}

	t.Chdir(root)
	out, _ = run(t, "status")
	if r := rowOf(out, "Session"); !strings.Contains(r, "recording session") || !strings.Contains(r, "acme") {
		t.Errorf("Session row = %q", r)
	}
}

func TestStatusInASubfolderIsTheSessionOfTheFolder(t *testing.T) {
	isolate(t)
	dirA := inSession(t, "acme")
	sub := filepath.Join(dirA, "scans")
	os.MkdirAll(sub, 0o700)
	inSession(t, "other")
	statusEnv(t, map[string]string{EnvCDFile: "/tmp/x"})
	t.Chdir(sub)
	out, _ := run(t, "status")
	if r := rowOf(out, "Session"); !strings.Contains(r, "acme") || strings.Contains(out, "other") {
		t.Errorf("a subfolder of acme:\n%s", out)
	}
}

func TestPauseWithoutShellIntegrationExplainsAndUnpauseIsGone(t *testing.T) {
	isolate(t)
	out, code := run(t, "pause")
	if code == 0 || !strings.Contains(out, "shell integration") {
		t.Errorf("pause: code %d %q", code, out)
	}
	if out, code := run(t, "unpause"); code == 0 || !strings.Contains(out, "unknown command") {
		t.Errorf("unpause must be gone: code %d %q", code, out)
	}
}

func TestShellFunctionPauseIsAToggle(t *testing.T) {
	for _, sh := range []string{"zsh", "bash"} {
		out, err := hookScript(sh)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out, "unpause") {
			t.Errorf("%s: the function still knows unpause", sh)
		}
		for _, want := range []string{`= pause ]`, `[ -z "${ARSENAL_PAUSED:-}" ]`, "Paused:", "Resumed:"} {
			if !strings.Contains(out, want) {
				t.Errorf("%s: the toggle lacks %q", sh, want)
			}
		}
	}
}

func TestMarkerIsSentOnlyInsideARecordingThatIsNotPaused(t *testing.T) {
	var sent []string
	orig := writeTTY
	writeTTY = func(s string) { sent = append(sent, s) }
	t.Cleanup(func() { writeTTY = orig })
	t.Setenv(rec.EnvRec, "")
	t.Setenv(EnvPaused, "")
	emitMarker("prompt", "scan all ports")
	if len(sent) != 0 {
		t.Fatalf("sent %q outside a recording", sent)
	}
	t.Setenv(rec.EnvRec, "/x/y.cast")
	emitMarker("prompt", "scan all ports")
	if len(sent) != 1 || sent[0] != rec.Sequence(rec.VerbMarker, "ars: prompt: scan all ports") {
		t.Fatalf("sent %q", sent)
	}
	t.Setenv(EnvPaused, "1")
	emitMarker("prompt", "scan all ports")
	if len(sent) != 1 {
		t.Errorf("sent a marker while paused: %q", sent)
	}
}

func TestInsertSendsAMarkerToTheRecorder(t *testing.T) {
	var sent []string
	orig := writeTTY
	writeTTY = func(s string) { sent = append(sent, s) }
	t.Cleanup(func() { writeTTY = orig })
	s := setupSearch(t, "scan all ports")
	s.answers["target"] = "10.10.10.5"
	inSession(t, "acme")
	t.Setenv(EnvSession, "acme")
	t.Setenv(rec.EnvRec, "/x/y.cast")
	if out, code := run(t, "-p"); code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if len(sent) != 1 || !strings.Contains(sent[0], "ars: print: scan all ports") {
		t.Errorf("sent %q", sent)
	}
}

func TestStatusCountsTheRecordings(t *testing.T) {
	isolate(t)
	inSession(t, "acme")
	if out, _ := run(t, "status"); !strings.Contains(rowOf(out, "Recording"), "no - none yet") {
		t.Errorf("no recordings:\n%s", out)
	}
	makeCast(t, "acme", "r1", 1000, map[float64]string{1: "x"})
	makeCast(t, "acme", "r2", 2000, map[float64]string{1: "y"})
	p := logPathOf(t, "acme")
	_ = cmdlog.Append(p, cmdlog.Record{Ev: cmdlog.EvRecStart, Rec: "r2"})
	out, _ := run(t, "status")
	if r := rowOf(out, "Recording"); !strings.Contains(r, "2 recordings, ") || !strings.Contains(r, "1 interrupted") || !strings.Contains(r, "ars replay") {
		t.Errorf("status:\n%s", out)
	}
}

func TestStatusColumnsAlignWithColors(t *testing.T) {
	old := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(old) })

	isolate(t)
	inSession(t, "acme")
	run(t, "set", "target=10.10.10.5")
	run(t, "note", "x")
	out, code := run(t, "status")
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if !strings.Contains(out, "\x1b[") {
		t.Fatal("colors were not forced on: the test would prove nothing")
	}
	plain := ansi.Strip(out)
	valueCol, hintCol := -1, -1
	rows := 0
	for _, l := range strings.Split(plain, "\n") {
		trimmed := strings.TrimSpace(l)
		var label string
		for _, want := range []string{"Session", "Data", "Paused", "Commands", "Recording", "Notes", "Variables"} {
			if strings.HasPrefix(trimmed, want+" ") {
				label = want
			}
		}
		if label == "" {
			continue
		}
		rows++
		rest := strings.TrimLeft(strings.TrimPrefix(trimmed, label), " ")
		col := len(l) - len(rest)
		if valueCol >= 0 && col != valueCol {
			t.Errorf("values are not aligned (column %d vs %d) in %q\n%s", col, valueCol, l, plain)
		}
		valueCol = col
		if i := strings.Index(l, "->"); i >= 0 {
			c := utf8.RuneCountInString(l[:i])
			if hintCol >= 0 && c != hintCol {
				t.Errorf("hints are not aligned (column %d vs %d) in %q\n%s", c, hintCol, l, plain)
			}
			hintCol = c
		}
	}
	if rows < 6 || hintCol < 0 {
		t.Fatalf("rows=%d hintCol=%d\n%s", rows, hintCol, plain)
	}
}
