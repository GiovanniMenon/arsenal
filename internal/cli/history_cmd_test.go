package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/tui"
)

func logCmd(t *testing.T, sess, id string, ts time.Time, cmd, cwd string, exit *int, rec string) {
	t.Helper()
	p := logPathOf(t, sess)
	if err := cmdlog.Append(p, cmdlog.Record{Ev: cmdlog.EvStart, ID: id, TS: ts.UnixMilli(), Session: sess, CWD: cwd, Cmd: cmd, Sh: "zsh", PID: 7, Rec: rec}); err != nil {
		t.Fatal(err)
	}
	if exit != nil {
		d := int64(1500)
		if err := cmdlog.Append(p, cmdlog.Record{Ev: cmdlog.EvEnd, ID: id, TS: ts.UnixMilli() + 1500, Exit: exit, Dur: &d}); err != nil {
			t.Fatal(err)
		}
	}
}

func ip(n int) *int { return &n }

func setupHistory(t *testing.T) time.Time {
	t.Helper()
	isolate(t)
	inSession(t, "acme")
	now := time.Now()
	logCmd(t, "acme", "1.1", now.Add(-72*time.Hour), "ls -la", "/w", ip(0), "")
	logCmd(t, "acme", "1.2", now.Add(-3*time.Hour), "nmap -sV 10.10.10.5", "/w/scans", ip(0), "20261124-140211-4242")
	logCmd(t, "acme", "1.3", now.Add(-2*time.Hour), "curl -k https://example.local", "/w", ip(7), "20261124-140211-4242")
	logCmd(t, "acme", "1.4", now.Add(-time.Hour), "ars set target=10.10.10.5", "/w", ip(0), "")
	logCmd(t, "acme", "1.5", now.Add(-time.Minute), "nc -lvnp 4444", "/w", nil, "20261124-140211-4242")
	return now
}

func TestHistoryListPrintsTheCommandsOldestFirstWithStatusAndHidesArs(t *testing.T) {
	setupHistory(t)
	out, code := run(t, "history")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d:\n%s", len(lines), out)
	}
	for i, want := range []string{"ok    ls -la", "ok    nmap -sV 10.10.10.5", "!7    curl -k https://example.local", "..    nc -lvnp 4444"} {
		if !strings.Contains(lines[i], want) {
			t.Errorf("line %d = %q, want %q in it", i, lines[i], want)
		}
	}
	if strings.Contains(out, "ars set") {
		t.Error("ars's own command lines must be hidden by default")
	}
	all, _ := run(t, "history", "--all-commands")
	if !strings.Contains(all, "ars set target=10.10.10.5") {
		t.Errorf("--all-commands:\n%s", all)
	}
}

func TestHistoryFilters(t *testing.T) {
	setupHistory(t)
	cases := []struct {
		args []string
		want []string
	}{
		{[]string{"--failed"}, []string{"curl"}},
		{[]string{"--since", "4h"}, []string{"nmap", "curl", "nc"}},
		{[]string{"--since", "90m"}, []string{"nc"}},
		{[]string{"--cwd", "/w/scans"}, []string{"nmap"}},
		{[]string{"--since", "1d", "--failed"}, []string{"curl"}},
	}
	for _, c := range cases {
		out, code := run(t, append([]string{"history"}, c.args...)...)
		if code != 0 {
			t.Fatalf("%v: code %d: %s", c.args, code, out)
		}
		var got []string
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			f := strings.Fields(l)
			got = append(got, f[3])
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%v: got %v, want %v", c.args, got, c.want)
		}
	}
	if out, code := run(t, "history", "--since", "soonish"); code == 0 || !strings.Contains(out, "--since") {
		t.Errorf("bad --since: code %d %q", code, out)
	}
}

func TestHistoryJSON(t *testing.T) {
	setupHistory(t)
	out, code := run(t, "history", "--json", "--failed")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	var j jsonEntry
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &j); err != nil {
		t.Fatalf("%v: %q", err, out)
	}
	if j.Cmd != "curl -k https://example.local" || j.Exit == nil || *j.Exit != 7 || !j.Ended || j.Rec != "20261124-140211-4242" || j.Via != cmdlog.ViaUser || j.Session != "acme" || j.Dur == nil {
		t.Errorf("json = %+v", j)
	}
	if strings.Contains(out, `\u003c`) {
		t.Error("HTML escaping in the JSON")
	}
}

func TestHistoryAllMergesSessionsByTime(t *testing.T) {
	now := setupHistory(t)
	inSession(t, "other")
	logCmd(t, "other", "2.1", now.Add(-150*time.Minute), "whoami", "/o", ip(0), "")
	out, _ := run(t, "history", "--all", "--since", "4h")
	if !strings.Contains(out, "acme  nmap") || !strings.Contains(out, "other  whoami") {
		t.Fatalf("sessions not shown:\n%s", out)
	}
	if strings.Index(out, "nmap") > strings.Index(out, "whoami") || strings.Index(out, "whoami") > strings.Index(out, "curl") {
		t.Errorf("not merged by time (nmap -3h, whoami -2.5h, curl -2h):\n%s", out)
	}
	if out, _ := run(t, "history", "--session", "other"); strings.Contains(out, "nmap") || !strings.Contains(out, "whoami") {
		t.Errorf("--session other:\n%s", out)
	}
}

func TestHistoryErrorsAndEmpty(t *testing.T) {
	isolate(t)
	if out, code := run(t, "history"); code == 0 || !strings.Contains(out, noSessionMsg) {
		t.Errorf("no session: code %d %q", code, out)
	}
	inSession(t, "acme")
	if out, code := run(t, "history"); code != 0 || !strings.Contains(out, "No commands logged") {
		t.Errorf("empty: code %d %q", code, out)
	}
	if out, code := run(t, "history", "--session", "ghost"); code == 0 || !strings.Contains(out, "not found") {
		t.Errorf("ghost: code %d %q", code, out)
	}
}

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 24, 15, 0, 0, 0, time.Local)
	for in, want := range map[string]time.Time{
		"90m":              now.Add(-90 * time.Minute),
		"2h":               now.Add(-2 * time.Hour),
		"3d":               now.Add(-72 * time.Hour),
		"1w":               now.Add(-7 * 24 * time.Hour),
		"2026-11-20":       time.Date(2026, 11, 20, 0, 0, 0, 0, time.Local),
		"2026-11-24 14:30": time.Date(2026, 11, 24, 14, 30, 0, 0, time.Local),
	} {
		if got, err := parseSince(in, now); err != nil || !got.Equal(want) {
			t.Errorf("parseSince(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "h", "3x", "yesterday", "2026-13-01"} {
		if _, err := parseSince(bad, now); err == nil {
			t.Errorf("parseSince(%q) accepted", bad)
		}
	}
}

func fakeHistoryUI(t *testing.T, pick func(o tui.HistoryOptions) tui.HistoryResult) *tui.HistoryOptions {
	t.Helper()
	var seen tui.HistoryOptions
	runHistoryUI = func(o tui.HistoryOptions) (tui.HistoryResult, error) {
		seen = o
		return pick(o), nil
	}
	isInteractive = func() bool { return true }
	t.Cleanup(func() { runHistoryUI = tui.RunHistory })
	return &seen
}

func TestHistoryEnterPutsTheCommandInThePromptAndLogsTheInsert(t *testing.T) {
	setupHistory(t)
	cmdFile := filepath.Join(t.TempDir(), "cmd")
	t.Setenv("ARSENAL_CMD_FILE", cmdFile)
	seen := fakeHistoryUI(t, func(o tui.HistoryOptions) tui.HistoryResult {
		return tui.HistoryResult{Entry: &o.Entries[1]}
	})
	out, code := run(t, "history", "-s", "nmap")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if seen.Session != "acme" || seen.Query != "nmap" || len(seen.Entries) != 4 || !strings.HasSuffix(seen.Path, "commands.jsonl") {
		t.Errorf("options = %+v", *seen)
	}
	if b, _ := os.ReadFile(cmdFile); string(b) != "nmap -sV 10.10.10.5" {
		t.Errorf("prompt got %q", b)
	}
	ins := insertsOf(t, "acme")
	if len(ins) != 1 || ins[0].Cheat != "history" || ins[0].How != cmdlog.HowShell || ins[0].Cmd != "nmap -sV 10.10.10.5" {
		t.Errorf("insert lines = %+v", ins)
	}
}

func TestHistoryPrintAndQuit(t *testing.T) {
	setupHistory(t)
	fakeHistoryUI(t, func(o tui.HistoryOptions) tui.HistoryResult { return tui.HistoryResult{Entry: &o.Entries[0]} })
	if out, code := run(t, "history", "--print"); code != 0 || strings.TrimSpace(out) != "ls -la" {
		t.Errorf("--print: code %d %q", code, out)
	}
	fakeHistoryUI(t, func(o tui.HistoryOptions) tui.HistoryResult { return tui.HistoryResult{} })
	if out, code := run(t, "history", "--print"); code != 0 || strings.TrimSpace(out) != "" {
		t.Errorf("quit: code %d %q", code, out)
	}
}

func TestHistoryTabReplaysTheRecordingFromThatCommand(t *testing.T) {
	now := setupHistory(t)
	fp := setupReplayPlayer(t)
	started := now.Add(-4 * time.Hour)
	makeCastAt(t, "acme", "20261124-140211-4242", started.Unix(), map[float64]string{
		1: "before\r\n", 3600.0: "around the nmap command\r\n", 3601.0: "after\r\n",
	})
	fakeHistoryUI(t, func(o tui.HistoryOptions) tui.HistoryResult {
		return tui.HistoryResult{Entry: &o.Entries[1], Replay: true}
	})
	out, code := run(t, "history")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if !strings.Contains(out, "Playing 20261124-140211-4242") || !strings.Contains(fp.out.String(), "around the nmap command") {
		t.Errorf("did not play the recording: %q / %q", out, fp.out.String())
	}

	fakeHistoryUI(t, func(o tui.HistoryOptions) tui.HistoryResult {
		e := o.Entries[1]
		e.Start.Rec = "20200101-000000-1"
		return tui.HistoryResult{Entry: &e, Replay: true}
	})
	if out, code := run(t, "history"); code == 0 || !strings.Contains(out, "not available") {
		t.Errorf("missing recording: code %d %q", code, out)
	}
}

func TestHistoryRedactMasksTheOutputButNotTheLog(t *testing.T) {
	now := setupHistory(t)
	logCmd(t, "acme", "1.9", now, "mysql -u root --password hunter2 -h 10.10.10.5", "/w", ip(0), "")
	out, _ := run(t, "history", "--redact")
	if strings.Contains(out, "hunter2") || !strings.Contains(out, "--password [REDACTED]") || !strings.Contains(out, "nmap -sV 10.10.10.5") {
		t.Errorf("--redact:\n%s", out)
	}
	if out, _ := run(t, "history", "--redact", "--json"); strings.Contains(out, "hunter2") {
		t.Errorf("--redact --json leaked:\n%s", out)
	}
	if out, _ := run(t, "history"); !strings.Contains(out, "hunter2") {
		t.Error("without --redact the command must be shown as it was typed")
	}
	p := logPathOf(t, "acme")
	if raw, _ := os.ReadFile(p); !strings.Contains(string(raw), "hunter2") {
		t.Error("the log file was changed")
	}

	writeConfig(t, "redact_patterns:\n  - '10\\.10\\.10\\.\\d+'\n")
	if out, _ := run(t, "history", "--redact"); strings.Contains(out, "10.10.10.5") {
		t.Errorf("redact_patterns not applied:\n%s", out)
	}
	writeConfig(t, "redact_patterns:\n  - '(unclosed'\n")
	if out, code := run(t, "history", "--redact"); code == 0 || !strings.Contains(out, "redact_patterns") {
		t.Errorf("invalid pattern: code %d %q", code, out)
	}
}

func TestHistoryRedactInTheViewerMasksTheDisplayOnly(t *testing.T) {
	now := setupHistory(t)
	logCmd(t, "acme", "1.9", now, "mysql --password hunter2", "/w", ip(0), "")
	var mask func(string) string
	runHistoryUI = func(o tui.HistoryOptions) (tui.HistoryResult, error) {
		mask = o.Mask
		return tui.HistoryResult{}, nil
	}
	isInteractive = func() bool { return true }
	t.Cleanup(func() { runHistoryUI = tui.RunHistory })
	run(t, "history", "--redact")
	if mask == nil || strings.Contains(mask("mysql --password hunter2"), "hunter2") {
		t.Errorf("the viewer got no working mask")
	}
	run(t, "history")
	if mask != nil {
		t.Error("a mask without --redact")
	}
}

func TestReplayRedactNeedsText(t *testing.T) {
	setupReplay(t)
	makeCast(t, "acme", "r1", 1000, map[float64]string{1: "login with password=hunter2 ok\r\n"})
	out, code := run(t, "replay", "r1", "--text", "--redact")
	if code != 0 || strings.Contains(out, "hunter2") || !strings.Contains(out, "password=[REDACTED]") {
		t.Errorf("--text --redact: code %d %q", code, out)
	}
	if out, _ := run(t, "replay", "r1", "--text"); !strings.Contains(out, "hunter2") {
		t.Error("--text without --redact must show the text as recorded")
	}
	if out, code := run(t, "replay", "r1", "--redact"); code == 0 || !strings.Contains(out, "--text") {
		t.Errorf("--redact without --text: code %d %q", code, out)
	}
}
