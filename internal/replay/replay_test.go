package replay

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ynn3m/arsenal/internal/cast"
	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/session"
)

type ev struct {
	at   float64
	code string
	data string
}

func writeCast(t *testing.T, path string, ts int64, events []ev) {
	t.Helper()
	var buf bytes.Buffer
	start := time.Unix(1000, 0)
	w, err := cast.NewWriter(&buf, cast.Header{Width: 100, Height: 30, Timestamp: ts, Title: "arsenal: acme"}, start)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range events {
		at := start.Add(time.Duration(e.at * float64(time.Second)))
		switch e.code {
		case "o":
			_ = w.Output(at, []byte(e.data))
		case "m":
			_ = w.Marker(at, e.data)
		case "r":
			_ = w.Resize(at, 120, 40)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func newSession(t *testing.T) (dir, log string) {
	t.Helper()
	dir = t.TempDir()
	if err := os.MkdirAll(session.MetaDir(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	return dir, cmdlog.Path(dir)
}

func TestListGroupsPartsSortsNewestFirstAndReadsTheState(t *testing.T) {
	dir, log := newSession(t)
	rd := session.RecordingsDir(dir)
	writeCast(t, filepath.Join(rd, "20261124-100000-4242.cast"), 2000, []ev{{1, "o", "a"}, {10, "o", "b"}})
	writeCast(t, filepath.Join(rd, "20261124-120000-77.cast"), 5000, []ev{{1, "o", "c"}, {4, "o", "d"}})
	writeCast(t, filepath.Join(rd, "20261124-120000-77-002.cast"), 5100, []ev{{2, "o", "e"}, {6, "o", "f"}})
	writeCast(t, filepath.Join(rd, "20261124-110000-123.cast"), 3000, []ev{{1, "o", "g"}})
	_ = os.WriteFile(filepath.Join(rd, "notes.cast"), []byte("not a cast\n"), 0o600)
	_ = os.WriteFile(filepath.Join(rd, "other.txt"), []byte("x"), 0o600)
	for _, r := range []cmdlog.Record{
		{Ev: cmdlog.EvRecStart, TS: 1, Rec: "20261124-100000-4242"},
		{Ev: cmdlog.EvRecEnd, TS: 2, Rec: "20261124-100000-4242"},
		{Ev: cmdlog.EvRecStart, TS: 3, Rec: "20261124-120000-77"},
	} {
		if err := cmdlog.Append(log, r); err != nil {
			t.Fatal(err)
		}
	}

	list, err := List(dir, log)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, in := range list {
		ids = append(ids, in.ID)
	}
	if strings.Join(ids, " ") != "20261124-120000-77 20261124-110000-123 20261124-100000-4242" {
		t.Fatalf("ids = %q", ids)
	}
	multi := list[0]
	if len(multi.Parts) != 2 || multi.Duration != 10*time.Second || multi.State != StateInterrupted {
		t.Errorf("multi-part recording: %+v", multi)
	}
	if list[1].State != StateUnknown || list[1].Duration != time.Second {
		t.Errorf("recording without log lines: %+v", list[1])
	}
	if list[2].State != StateComplete || list[2].Duration != 10*time.Second || list[2].Header.Width != 100 {
		t.Errorf("complete recording: %+v", list[2])
	}
	if got, _ := List(filepath.Join(dir, "nothing"), log); got != nil {
		t.Errorf("a session without recordings gave %v", got)
	}
}

func TestFindByIDPrefixPathAndContinuationFile(t *testing.T) {
	dir, log := newSession(t)
	rd := session.RecordingsDir(dir)
	writeCast(t, filepath.Join(rd, "20261124-100000-4242.cast"), 2000, []ev{{1, "o", "a"}})
	writeCast(t, filepath.Join(rd, "20261124-100500-4243.cast"), 2100, []ev{{1, "o", "b"}})
	writeCast(t, filepath.Join(rd, "20261124-100500-4243-002.cast"), 2200, []ev{{3, "o", "c"}})

	if in, err := Find(dir, log, "20261124-100000-4242"); err != nil || in.ID != "20261124-100000-4242" {
		t.Errorf("exact: %+v %v", in, err)
	}
	if in, err := Find(dir, log, "20261124-1005"); err != nil || len(in.Parts) != 2 {
		t.Errorf("prefix: %+v %v", in, err)
	}
	if _, err := Find(dir, log, "20261124-10"); err == nil || !strings.Contains(err.Error(), "matches 2") {
		t.Errorf("ambiguous: %v", err)
	}
	if _, err := Find(dir, log, "nope"); err == nil {
		t.Error("unknown id accepted")
	}
	for _, p := range []string{filepath.Join(rd, "20261124-100500-4243.cast"), filepath.Join(rd, "20261124-100500-4243-002.cast")} {
		if in, err := Find(dir, log, p); err != nil || len(in.Parts) != 2 || in.ID != "20261124-100500-4243" {
			t.Errorf("path %s: %+v %v", p, in, err)
		}
	}
}

func TestPlayWritesSanitizedOutputWithTimingAndResets(t *testing.T) {
	dir, log := newSession(t)
	writeCast(t, filepath.Join(session.RecordingsDir(dir), "r1.cast"), 1, []ev{
		{0.1, "o", "start "},
		{0.2, "m", "a marker"},
		{0.3, "r", ""},
		{0.4, "o", "\x1b]52;c;ZXZpbA==\x07clip\x1b[31mred\x1b[0m "},
		{0.5, "o", "\x1b[6nend"},
	})
	in, err := Find(dir, log, "r1")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	begin := time.Now()
	if err := in.Play(PlayOptions{Out: &out, Speed: 1}); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(begin); took < 400*time.Millisecond || took > 3*time.Second {
		t.Errorf("playback took %v, want about 0.5 s", took)
	}
	want := "start clip\x1b[31mred\x1b[0m end" + Reset
	if out.String() != want {
		t.Errorf("output %q\nwant   %q", out.String(), want)
	}

	var raw bytes.Buffer
	_ = in.Play(PlayOptions{Out: &raw, Speed: 1000, Raw: true})
	if !strings.Contains(raw.String(), "\x1b]52;c;ZXZpbA==\x07") || !strings.Contains(raw.String(), "\x1b[6n") {
		t.Errorf("--raw must not filter: %q", raw.String())
	}
}

func TestPlaySpeedIdleLimitAndFrom(t *testing.T) {
	dir, log := newSession(t)
	writeCast(t, filepath.Join(session.RecordingsDir(dir), "r2.cast"), 1, []ev{
		{1, "o", "one "}, {2, "o", "two "}, {60, "o", "three "}, {61, "o", "four"},
	})
	in, _ := Find(dir, log, "r2")

	var out bytes.Buffer
	begin := time.Now()
	if err := in.Play(PlayOptions{Out: &out, Speed: 2000, IdleLimit: time.Second}); err != nil {
		t.Fatal(err)
	}
	if time.Since(begin) > 2*time.Second {
		t.Errorf("speed or idle limit not applied: %v", time.Since(begin))
	}
	if !strings.HasPrefix(out.String(), "one two three four") {
		t.Errorf("out %q", out.String())
	}

	out.Reset()
	begin = time.Now()
	if err := in.Play(PlayOptions{Out: &out, Speed: 1000, From: 59 * time.Second}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "one two three four") || time.Since(begin) > 2*time.Second {
		t.Errorf("from: %q after %v", out.String(), time.Since(begin))
	}
}

func TestPlayKeysPauseSpeedAndQuit(t *testing.T) {
	dir, log := newSession(t)
	writeCast(t, filepath.Join(session.RecordingsDir(dir), "r3.cast"), 1, []ev{{0.3, "o", "a"}, {0.6, "o", "b"}, {5, "o", "c"}})
	in, _ := Find(dir, log, "r3")

	keys := make(chan byte, 8)
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- in.Play(PlayOptions{Out: &out, Keys: keys, Speed: 1}) }()
	time.Sleep(900 * time.Millisecond)
	keys <- 'q'
	select {
	case err := <-done:
		if !errors.Is(err, ErrStopped) {
			t.Errorf("err = %v, want ErrStopped", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("q did not stop the playback")
	}
	if !strings.HasPrefix(out.String(), "ab") || strings.Contains(strings.TrimSuffix(out.String(), Reset), "c") {
		t.Errorf("out %q", out.String())
	}
	if !strings.HasSuffix(out.String(), Reset) {
		t.Error("the terminal modes were not reset after quitting")
	}

	keys = make(chan byte, 8)
	out.Reset()
	go func() { done <- in.Play(PlayOptions{Out: &out, Keys: keys, Speed: 20}) }()
	keys <- ' '
	time.Sleep(600 * time.Millisecond)
	if strings.Contains(out.String(), "c") {
		t.Error("output while paused")
	}
	keys <- ' '
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("playback did not finish after resume")
	}
	if !strings.HasPrefix(out.String(), "abc") {
		t.Errorf("out %q", out.String())
	}
}

func TestTextAndMarkersOfAMultiPartRecording(t *testing.T) {
	dir, log := newSession(t)
	rd := session.RecordingsDir(dir)
	writeCast(t, filepath.Join(rd, "r4.cast"), 1, []ev{{1, "o", "first part\r\n"}, {4, "m", "ars: prompt: scan"}, {5, "o", "x\r\n"}})
	writeCast(t, filepath.Join(rd, "r4-002.cast"), 2, []ev{{2, "o", "second part\r\n"}})
	in, _ := Find(dir, log, "r4")
	text, err := in.Text()
	if err != nil {
		t.Fatal(err)
	}
	if text != "first part\n[marker 0:00:04] ars: prompt: scan\nx\nsecond part\n" {
		t.Errorf("text = %q", text)
	}
	at, labels, _ := in.Markers()
	if len(at) != 1 || at[0] != 4*time.Second || labels[0] != "ars: prompt: scan" {
		t.Errorf("markers %v %v", at, labels)
	}
	if in.Duration != 7*time.Second {
		t.Errorf("duration = %v, want the parts end to end (7 s)", in.Duration)
	}
	if Clock(3725*time.Second) != "1:02:05" {
		t.Errorf("Clock = %s", Clock(3725*time.Second))
	}
}

func TestKeysDropsEscapeSequencesAndKeepsPlainKeys(t *testing.T) {
	in := "q" + "\x1b]11;rgb:0000/0000/0000\x1b\\" + "\x1b[24;1R" + "\x1b[A" + " " + "\x1b[?62;4c" + "+" + "\x03"
	ch := Keys(strings.NewReader(in))
	var got []byte
	for b := range ch {
		got = append(got, b)
	}
	if string(got) != "q +\x03" {
		t.Errorf("keys = %q, want \"q +\\x03\"", got)
	}
}
