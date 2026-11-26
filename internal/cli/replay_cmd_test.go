package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/ynn3m/arsenal/internal/cast"
	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/replay"
	"github.com/ynn3m/arsenal/internal/session"
	"github.com/ynn3m/arsenal/internal/tui"
)

func makeCast(t *testing.T, sess, id string, ts int64, events map[float64]string) string {
	return makeCastAt(t, sess, id, ts, events)
}

func makeCastAt(t *testing.T, sess, id string, ts int64, events map[float64]string) string {
	t.Helper()
	dir, _ := session.Dir(sess)
	rd := session.RecordingsDir(dir)
	if err := os.MkdirAll(rd, 0o700); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	start := time.Unix(ts, 0)
	w, _ := cast.NewWriter(&buf, cast.Header{Width: 100, Height: 30, Timestamp: ts}, start)
	var times []float64
	for at := range events {
		times = append(times, at)
	}
	for i := range times {
		for j := i + 1; j < len(times); j++ {
			if times[j] < times[i] {
				times[i], times[j] = times[j], times[i]
			}
		}
	}
	for _, at := range times {
		d := start.Add(time.Duration(at * float64(time.Second)))
		if strings.HasPrefix(events[at], "marker:") {
			_ = w.Marker(d, strings.TrimPrefix(events[at], "marker:"))
		} else {
			_ = w.Output(d, []byte(events[at]))
		}
	}
	p := filepath.Join(rd, id+".cast")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

type fakePlayer struct {
	out  bytes.Buffer
	cols int
	rows int
}

func setupReplayPlayer(t *testing.T) *fakePlayer {
	t.Helper()
	fp := &fakePlayer{cols: 200, rows: 50}
	openPlayer = func(cmd *cobra.Command) (player, error) {
		return player{out: &fp.out, done: func() {}, cols: fp.cols, rows: fp.rows}, nil
	}
	chooseRecording = func(string, string, []string) (int, error) {
		t.Fatal("unexpected picker")
		return 0, nil
	}
	t.Cleanup(func() { openPlayer = defaultPlayer; chooseRecording = tui.AskChoice })
	return fp
}

func setupReplay(t *testing.T) *fakePlayer {
	t.Helper()
	isolate(t)
	fp := setupReplayPlayer(t)
	inSession(t, "acme")
	return fp
}

func TestReplayListsTheRecordingsNewestFirstWithTheirState(t *testing.T) {
	setupReplay(t)
	makeCast(t, "acme", "20261124-100000-4242", 1795860000, map[float64]string{1: "a", 61: "b"})
	makeCast(t, "acme", "20261124-130000-99", 1795870800, map[float64]string{2: "c"})
	p := logPathOf(t, "acme")
	_ = cmdlog.Append(p, cmdlog.Record{Ev: cmdlog.EvRecStart, Rec: "20261124-100000-4242"})
	_ = cmdlog.Append(p, cmdlog.Record{Ev: cmdlog.EvRecEnd, Rec: "20261124-100000-4242"})
	_ = cmdlog.Append(p, cmdlog.Record{Ev: cmdlog.EvRecStart, Rec: "20261124-130000-99"})

	out, code := run(t, "replay")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	i1, i2 := strings.Index(out, "20261124-130000-99"), strings.Index(out, "20261124-100000-4242")
	if i1 < 0 || i2 < 0 || i1 > i2 {
		t.Errorf("not newest first:\n%s", out)
	}
	for _, want := range []string{"interrupted", "complete", "0:01:01", "Recordings of acme"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q:\n%s", want, out)
		}
	}
	for _, r := range out {
		if r > 126 {
			t.Fatalf("non-ASCII %q", r)
		}
	}
}

func TestReplayWithoutRecordingsSaysSo(t *testing.T) {
	setupReplay(t)
	if out, code := run(t, "replay"); code != 0 || !strings.Contains(out, "No recordings") || !strings.Contains(out, "ars rec") {
		t.Errorf("code %d: %q", code, out)
	}
}

func TestReplayPicksInteractivelyWhenNoIdIsGiven(t *testing.T) {
	fp := setupReplay(t)
	makeCast(t, "acme", "older", 1000, map[float64]string{0.01: "old text"})
	makeCast(t, "acme", "newer", 2000, map[float64]string{0.01: "new text"})
	isInteractive = func() bool { return true }
	var labels []string
	chooseRecording = func(title, desc string, l []string) (int, error) { labels = l; return 1, nil }
	out, code := run(t, "replay", "--speed", "1000")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if len(labels) != 2 || !strings.HasPrefix(labels[0], "newer") || !strings.HasPrefix(labels[1], "older") {
		t.Errorf("labels = %q", labels)
	}
	if !strings.Contains(fp.out.String(), "old text") || strings.Contains(fp.out.String(), "new text") {
		t.Errorf("played %q", fp.out.String())
	}
}

func TestReplayPlaysAnIdSanitizedAndResetsTheTerminal(t *testing.T) {
	fp := setupReplay(t)
	makeCast(t, "acme", "20261124-100000-4242", 1000, map[float64]string{
		0.01: "hello \x1b]52;c;ZXZpbA==\x07world\x1b[6n", 0.02: "marker:ars: prompt: x", 0.03: " bye",
	})
	out, code := run(t, "replay", "20261124-1000", "--speed", "1000")
	if code != 0 {
		t.Fatalf("code %d: %s", code, out)
	}
	if !strings.HasPrefix(fp.out.String(), "hello world bye") || !strings.HasSuffix(fp.out.String(), replay.Reset) {
		t.Errorf("played %q", fp.out.String())
	}
	if strings.Contains(fp.out.String(), "\x1b]52") || strings.Contains(fp.out.String(), "6n") {
		t.Errorf("unsafe sequences reached the terminal: %q", fp.out.String())
	}
	if !strings.Contains(out, "Playing") || !strings.Contains(out, "20261124-100000-4242") {
		t.Errorf("header line missing: %q", out)
	}

	fp.out.Reset()
	if _, code := run(t, "replay", "20261124-1000", "--speed", "1000", "--raw"); code != 0 || !strings.Contains(fp.out.String(), "\x1b]52;c;ZXZpbA==\x07") {
		t.Errorf("--raw must play unchanged: %q", fp.out.String())
	}
}

func TestReplayTextMarkersAndAtFrom(t *testing.T) {
	fp := setupReplay(t)
	makeCast(t, "acme", "r1", 1795860000, map[float64]string{1: "one\r\n", 2: "marker:ars: prompt: scan", 3: "two\r\n", 90: "ninety\r\n"})

	out, code := run(t, "replay", "r1", "--text")
	if code != 0 || out != "one\n[marker 0:00:02] ars: prompt: scan\ntwo\nninety\n" {
		t.Errorf("--text: code %d %q", code, out)
	}
	out, code = run(t, "replay", "r1", "--markers")
	if code != 0 || !strings.Contains(out, "0:00:02  ars: prompt: scan") {
		t.Errorf("--markers: code %d %q", code, out)
	}
	if len(fp.out.String()) != 0 {
		t.Error("--text and --markers must not play")
	}

	begin := time.Now()
	out, code = run(t, "replay", "r1", "--speed", "1000", "--at", "1795860089000")
	if code != 0 || time.Since(begin) > 3*time.Second || !strings.Contains(fp.out.String(), "ninety") {
		t.Errorf("--at: code %d after %v: %q %q", code, time.Since(begin), out, fp.out.String())
	}
	if out, code := run(t, "replay", "r1", "--from", "10m"); code == 0 || !strings.Contains(out, "only") {
		t.Errorf("--from past the end: code %d %q", code, out)
	}
}

func TestReplayWarnsAboutASmallTerminalAndNeedsATerminalToPlay(t *testing.T) {
	fp := setupReplay(t)
	makeCast(t, "acme", "r1", 1000, map[float64]string{0.01: "x"})
	fp.cols, fp.rows = 80, 24
	out, _ := run(t, "replay", "r1", "--speed", "1000")
	if !strings.Contains(out, "80x24") || !strings.Contains(out, "smaller than the recording") {
		t.Errorf("no size warning: %q", out)
	}
	openPlayer = defaultPlayer
	if out, code := run(t, "replay", "r1"); code == 0 || !strings.Contains(out, "needs a terminal") {
		t.Errorf("code %d: %q", code, out)
	}
}

func TestReplayUnknownRecordingAndSessionOption(t *testing.T) {
	setupReplay(t)
	inSession(t, "other")
	makeCast(t, "acme", "mine", 1000, map[float64]string{1: "x\r\n"})
	if out, code := run(t, "replay", "nothing"); code == 0 || !strings.Contains(out, "no recording") {
		t.Errorf("code %d %q", code, out)
	}
	if out, code := run(t, "replay", "mine", "--session", "acme", "--text"); code != 0 || out != "x\n" {
		t.Errorf("--session: code %d %q", code, out)
	}
	if out, code := run(t, "replay", "mine", "--text"); code == 0 {
		t.Errorf("found a recording of another session: %q", out)
	}
	if _, code := run(t, "replay", "--session", "../x"); code == 0 {
		t.Error("invalid session name accepted")
	}
}
