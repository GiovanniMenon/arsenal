package cast

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriterProducesAsciicastV2(t *testing.T) {
	var buf bytes.Buffer
	start := time.Unix(1795874531, 0)
	w, err := NewWriter(&buf, Header{Width: 120, Height: 40, Timestamp: 1795874531, Title: "arsenal: acme", Command: "/bin/zsh", Env: map[string]string{"TERM": "xterm-256color", "SHELL": "/bin/zsh"}}, start)
	if err != nil {
		t.Fatal(err)
	}
	_ = w.Output(start.Add(1500*time.Millisecond), []byte("hello <b> & \"q\" \x1b[31mred\x1b[0m\r\n"))
	_ = w.Resize(start.Add(2*time.Second), 100, 50)
	_ = w.Marker(start.Add(3*time.Second), "scan specific ports")
	_ = w.Input(start.Add(4*time.Second), []byte("ls\r"))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("lines = %d:\n%s", len(lines), buf.String())
	}
	var h map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &h); err != nil {
		t.Fatal(err)
	}
	if h["version"].(float64) != 2 || h["width"].(float64) != 120 || h["height"].(float64) != 40 || h["title"] != "arsenal: acme" {
		t.Errorf("header = %v", h)
	}
	for _, k := range []string{"duration", "idle_time_limit"} {
		if _, ok := h[k]; ok {
			t.Errorf("header has %q, which is unknown while writing", k)
		}
	}
	for i, wantCode := range []string{"o", "r", "m", "i"} {
		var ev []any
		if err := json.Unmarshal([]byte(lines[i+1]), &ev); err != nil || len(ev) != 3 || ev[1] != wantCode {
			t.Errorf("event %d = %s (%v)", i, lines[i+1], err)
		}
	}
	if !strings.HasPrefix(lines[1], "[1.500000,") || !strings.Contains(lines[1], `<b> & \"q\" \u001b[31mred`) {
		t.Errorf("output event = %s", lines[1])
	}
	if lines[2] != `[2.000000,"r","100x50"]` {
		t.Errorf("resize event = %s", lines[2])
	}
	if w.Size() != int64(buf.Len()) {
		t.Errorf("Size() = %d, want %d", w.Size(), buf.Len())
	}
}

func TestRoundTripThroughReader(t *testing.T) {
	var buf bytes.Buffer
	start := time.Now()
	w, _ := NewWriter(&buf, Header{Width: 80, Height: 24}, start)
	_ = w.Output(start.Add(time.Second), []byte("a\r\nb"))
	_ = w.Resize(start.Add(2*time.Second), 132, 43)
	_ = w.Marker(start.Add(3*time.Second), "")

	r, err := NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	evs := r.ReadAll()
	if len(evs) != 3 || evs[0].Data != "a\r\nb" || evs[0].Time != 1 {
		t.Fatalf("events = %+v", evs)
	}
	if c, rows, ok := evs[1].Resize(); !ok || c != 132 || rows != 43 {
		t.Errorf("resize = %d %d %v", c, rows, ok)
	}
	if _, _, ok := evs[0].Resize(); ok {
		t.Error("an output event is not a resize")
	}
	if evs[2].Code != "m" || evs[2].Data != "" {
		t.Errorf("empty marker = %+v", evs[2])
	}
}

func TestOutputKeepsSplitUTF8TogetherAndReplacesGarbage(t *testing.T) {
	var buf bytes.Buffer
	start := time.Now()
	w, _ := NewWriter(&buf, Header{Width: 80, Height: 24}, start)
	euro := []byte("€")
	_ = w.Output(start, append([]byte("a"), euro[:2]...))
	_ = w.Output(start, append(append([]byte{}, euro[2:]...), 'b'))
	_ = w.Output(start, []byte{0xff, 'c'})
	_ = w.Output(start, euro[:1])
	_ = w.Flush(start)

	r, err := NewReader(&buf)
	if err != nil {
		t.Fatal(err)
	}
	var got strings.Builder
	for _, e := range r.ReadAll() {
		got.WriteString(e.Data)
	}
	if got.String() != "a€b\uFFFDc\uFFFD" {
		t.Errorf("text = %q", got.String())
	}
}

func TestReaderIgnoresTruncatedLastLine(t *testing.T) {
	text := `{"version":2,"width":80,"height":24}` + "\n" +
		`[0.5,"o","one"]` + "\n" +
		`[1.0,"o","tw`
	r, err := NewReader(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	evs := r.ReadAll()
	if len(evs) != 1 || evs[0].Data != "one" {
		t.Errorf("events = %+v", evs)
	}
}

func TestReaderRejectsOtherFormats(t *testing.T) {
	for _, in := range []string{"", "not json\n", `{"version":3,"term":{"cols":80,"rows":24}}` + "\n", `{"version":2}` + "\n", `[1,"o","x"]` + "\n"} {
		if _, err := NewReader(strings.NewReader(in)); !errors.Is(err, ErrNotCast) {
			t.Errorf("NewReader(%q) error = %v, want ErrNotCast", in, err)
		}
	}
}

type failAfter struct {
	n   int
	buf bytes.Buffer
}

func (f *failAfter) Write(p []byte) (int, error) {
	if f.n <= 0 {
		return 0, io.ErrClosedPipe
	}
	f.n--
	return f.buf.Write(p)
}

func TestWriterKeepsFirstErrorAndDropsLaterEvents(t *testing.T) {
	f := &failAfter{n: 2}
	start := time.Now()
	w, err := NewWriter(f, Header{Width: 80, Height: 24}, start)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Output(start, []byte("ok")); err != nil {
		t.Fatal(err)
	}
	if err := w.Output(start, []byte("lost")); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("error = %v", err)
	}
	if !errors.Is(w.Err(), io.ErrClosedPipe) || w.Marker(start, "x") == nil {
		t.Error("the error must stay")
	}
	if strings.Contains(f.buf.String(), "lost") {
		t.Error("an event was written after the error")
	}
}

func TestSummarizeReadsTheLastEventFromTheEnd(t *testing.T) {
	var buf bytes.Buffer
	start := time.Now()
	w, _ := NewWriter(&buf, Header{Width: 90, Height: 30, Title: "t"}, start)
	for i := 1; i <= 5000; i++ {
		_ = w.Output(start.Add(time.Duration(i)*time.Second), []byte(strings.Repeat("x", 100)))
	}
	buf.WriteString(`[9999.5,"o","cut by a crash`)
	p := filepath.Join(t.TempDir(), "a.cast")
	if err := os.WriteFile(p, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Summarize(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Duration != 5000 || s.Header.Width != 90 || s.Header.Title != "t" || s.Size != int64(buf.Len()) {
		t.Errorf("summary = %+v", s)
	}
	small := filepath.Join(t.TempDir(), "b.cast")
	_ = os.WriteFile(small, []byte(`{"version":2,"width":80,"height":24}`+"\n"), 0o600)
	if s, err := Summarize(small); err != nil || s.Duration != 0 {
		t.Errorf("empty: %+v %v", s, err)
	}
	notCast := filepath.Join(t.TempDir(), "c.cast")
	_ = os.WriteFile(notCast, []byte("hello\n"), 0o600)
	if _, err := Summarize(notCast); err == nil {
		t.Error("a file that is not a cast must be rejected")
	}
}
