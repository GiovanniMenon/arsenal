// Reads the command log back and pairs each start with its end.

package cmdlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"sort"
	"strings"
	"time"
)

const DefaultTailBytes = 8 << 20

func Parse(data []byte) ([]Record, int) {
	var out []Record
	bad := 0
	for len(data) > 0 {
		var line []byte
		if i := bytes.IndexByte(data, '\n'); i >= 0 {
			line, data = data[:i], data[i+1:]
		} else {
			line, data = data, nil
		}
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var r Record
		if err := json.Unmarshal(line, &r); err != nil || r.Ev == "" || r.V < 1 {
			bad++
			continue
		}
		out = append(out, r)
	}
	return out, bad
}

func Load(path string, maxBytes int64) ([]Record, error) {
	if maxBytes <= 0 {
		maxBytes = DefaultTailBytes
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	partial := false
	if fi.Size() > maxBytes {
		if _, err := f.Seek(fi.Size()-maxBytes, io.SeekStart); err != nil {
			return nil, err
		}
		partial = true
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	if partial {
		if i := bytes.IndexByte(raw, '\n'); i >= 0 {
			raw = raw[i+1:]
		} else {
			raw = nil
		}
	}
	recs, _ := Parse(raw)
	return recs, nil
}

const ViaWindow = 6 * time.Hour

type Entry struct {
	Start  Record
	End    *Record
	Via    string
	Insert *Record
}

func (e Entry) Time() time.Time { return time.UnixMilli(e.Start.TS) }

func (e Entry) Ended() bool { return e.End != nil }

func (e Entry) ExitCode() (int, bool) {
	if e.End == nil || e.End.Exit == nil {
		return 0, false
	}
	return *e.End.Exit, true
}

func (e Entry) Failed() bool {
	c, ok := e.ExitCode()
	return ok && c != 0
}

func (e Entry) Duration() (time.Duration, bool) {
	if e.End == nil || e.End.Dur == nil {
		return 0, false
	}
	return time.Duration(*e.End.Dur) * time.Millisecond, true
}

func Fold(recs []Record) []Entry {
	var entries []Entry
	index := map[string]int{}
	inserted := map[string][]Record{}
	for _, r := range recs {
		switch r.Ev {
		case EvInsert:
			k := r.Session + "\x00" + r.Cmd
			inserted[k] = append(inserted[k], r)
		case EvStart:
			if r.ID != "" {
				index[r.ID] = len(entries)
			}
			e := Entry{Start: r, Via: ViaUser}
			k := r.Session + "\x00" + r.Cmd
			if list := inserted[k]; len(list) > 0 {
				last := list[len(list)-1]
				if r.TS >= last.TS && r.TS-last.TS <= ViaWindow.Milliseconds() {
					e.Via = ViaArs
					ins := last
					e.Insert = &ins
					inserted[k] = list[:len(list)-1]
				}
			}
			entries = append(entries, e)
		case EvEnd:
			if i, ok := index[r.ID]; ok && r.ID != "" && entries[i].End == nil {
				end := r
				entries[i].End = &end
			}
		}
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Start.TS < entries[j].Start.TS })
	return entries
}

type Filter struct {
	Session  string
	Since    time.Time
	Failed   bool
	CWD      string
	HideArs  bool
	Contains string
}

func (f Filter) Match(e Entry) bool {
	if f.Session != "" && e.Start.Session != f.Session {
		return false
	}
	if !f.Since.IsZero() && e.Time().Before(f.Since) {
		return false
	}
	if f.Failed && !e.Failed() {
		return false
	}
	if f.CWD != "" && e.Start.CWD != f.CWD {
		return false
	}
	if f.HideArs && IsArsCommand(e.Start.Cmd) {
		return false
	}
	if f.Contains != "" && !strings.Contains(strings.ToLower(e.Start.Cmd), strings.ToLower(f.Contains)) {
		return false
	}
	return true
}

func Select(entries []Entry, f Filter) []Entry {
	var out []Entry
	for _, e := range entries {
		if f.Match(e) {
			out = append(out, e)
		}
	}
	return out
}

func IsArsCommand(cmd string) bool {
	f := strings.Fields(cmd)
	if len(f) > 0 && f[0] == "command" {
		f = f[1:]
	}
	return len(f) > 0 && (f[0] == "ars" || f[0] == "arsenal")
}

func Inserts(recs []Record) []Record {
	var out []Record
	for _, r := range recs {
		if r.Ev == EvInsert {
			out = append(out, r)
		}
	}
	return out
}
