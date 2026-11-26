// Package cast reads and writes asciicast v2 recordings.
package cast

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	CodeOutput = "o"
	CodeInput  = "i"
	CodeMarker = "m"
	CodeResize = "r"
)

type Header struct {
	Version   int               `json:"version"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	Timestamp int64             `json:"timestamp,omitempty"`
	Duration  float64           `json:"duration,omitempty"`
	Command   string            `json:"command,omitempty"`
	Title     string            `json:"title,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
}

type Event struct {
	Time float64
	Code string
	Data string
}

func (e Event) Resize() (cols, rows int, ok bool) {
	if e.Code != CodeResize {
		return 0, 0, false
	}
	c, r, found := strings.Cut(e.Data, "x")
	if !found {
		return 0, 0, false
	}
	cols, err1 := strconv.Atoi(c)
	rows, err2 := strconv.Atoi(r)
	return cols, rows, err1 == nil && err2 == nil && cols > 0 && rows > 0
}

type Writer struct {
	w     io.Writer
	start time.Time
	carry []byte
	n     int64
	err   error
}

func NewWriter(w io.Writer, h Header, start time.Time) (*Writer, error) {
	h.Version = 2
	if h.Width <= 0 {
		h.Width = 80
	}
	if h.Height <= 0 {
		h.Height = 24
	}
	line, err := json.Marshal(h)
	if err != nil {
		return nil, err
	}
	cw := &Writer{w: w, start: start}
	if err := cw.write(append(line, '\n')); err != nil {
		return nil, err
	}
	return cw, nil
}

func (w *Writer) Size() int64 { return w.n }

func (w *Writer) Err() error { return w.err }

func (w *Writer) write(b []byte) error {
	if w.err != nil {
		return w.err
	}
	n, err := w.w.Write(b)
	w.n += int64(n)
	if err == nil && n < len(b) {
		err = io.ErrShortWrite
	}
	w.err = err
	return err
}

func jsonString(s string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func (w *Writer) event(at time.Time, code string, data string) error {
	t := at.Sub(w.start).Seconds()
	if t < 0 {
		t = 0
	}
	line := make([]byte, 0, len(data)+32)
	line = append(line, '[')
	line = strconv.AppendFloat(line, t, 'f', 6, 64)
	line = append(line, ',', '"')
	line = append(line, code...)
	line = append(line, '"', ',')
	line = append(line, jsonString(data)...)
	line = append(line, ']', '\n')
	return w.write(line)
}

func splitComplete(b []byte) (done, rest []byte) {
	for i := len(b) - 1; i >= 0 && i >= len(b)-3; i-- {
		if utf8.RuneStart(b[i]) {
			if !utf8.FullRune(b[i:]) {
				return b[:i], b[i:]
			}
			break
		}
	}
	return b, nil
}

func (w *Writer) Output(at time.Time, data []byte) error {
	if len(w.carry) > 0 {
		data = append(append([]byte{}, w.carry...), data...)
		w.carry = nil
	}
	done, rest := splitComplete(data)
	w.carry = append(w.carry[:0], rest...)
	if len(done) == 0 {
		return w.err
	}
	return w.event(at, CodeOutput, string(done))
}

func (w *Writer) Flush(at time.Time) error {
	if len(w.carry) == 0 {
		return w.err
	}
	rest := w.carry
	w.carry = nil
	return w.event(at, CodeOutput, string(rest))
}

func (w *Writer) Input(at time.Time, data []byte) error {
	return w.event(at, CodeInput, strings.ToValidUTF8(string(data), "\uFFFD"))
}

func (w *Writer) Marker(at time.Time, label string) error {
	return w.event(at, CodeMarker, label)
}

func (w *Writer) Resize(at time.Time, cols, rows int) error {
	return w.event(at, CodeResize, fmt.Sprintf("%dx%d", cols, rows))
}

type Reader struct {
	r      *bufio.Reader
	Header Header
}

var ErrNotCast = errors.New("not a recording file (asciicast v2)")

func NewReader(r io.Reader) (*Reader, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	line, err := br.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return nil, ErrNotCast
	}
	var h Header
	if err := json.Unmarshal(bytes.TrimSpace(line), &h); err != nil || h.Version != 2 || h.Width <= 0 || h.Height <= 0 {
		return nil, ErrNotCast
	}
	return &Reader{r: br, Header: h}, nil
}

func (r *Reader) Next() (Event, error) {
	for {
		line, err := r.r.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			if ev, ok := parseEvent(bytes.TrimSpace(line)); ok {
				return ev, nil
			}
		}
		if err != nil {
			return Event{}, io.EOF
		}
	}
}

func parseEvent(line []byte) (Event, bool) {
	if len(line) == 0 || line[0] != '[' {
		return Event{}, false
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil || len(raw) != 3 {
		return Event{}, false
	}
	var ev Event
	if json.Unmarshal(raw[0], &ev.Time) != nil || json.Unmarshal(raw[1], &ev.Code) != nil || json.Unmarshal(raw[2], &ev.Data) != nil {
		return Event{}, false
	}
	return ev, true
}

func (r *Reader) ReadAll() []Event {
	var out []Event
	for {
		ev, err := r.Next()
		if err != nil {
			return out
		}
		out = append(out, ev)
	}
}

type Summary struct {
	Header   Header
	Duration float64
	Size     int64
}

const tailBytes = 64 << 10

func Summarize(path string) (Summary, error) {
	f, err := os.Open(path)
	if err != nil {
		return Summary{}, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return Summary{}, err
	}
	r, err := NewReader(f)
	if err != nil {
		return Summary{}, fmt.Errorf("%s: %w", path, err)
	}
	s := Summary{Header: r.Header, Size: fi.Size()}

	start := fi.Size() - tailBytes
	if start < 0 {
		start = 0
	}
	buf := make([]byte, fi.Size()-start)
	if _, err := f.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return s, err
	}
	if start > 0 {
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		} else {
			buf = nil
		}
	}
	if i := bytes.LastIndexByte(buf, '\n'); i >= 0 {
		buf = buf[:i+1]
	} else {
		buf = nil
	}
	lines := bytes.Split(bytes.TrimRight(buf, "\n"), []byte{'\n'})
	for i := len(lines) - 1; i >= 0; i-- {
		if ev, ok := parseEvent(bytes.TrimSpace(lines[i])); ok {
			s.Duration = ev.Time
			break
		}
	}
	return s, nil
}
