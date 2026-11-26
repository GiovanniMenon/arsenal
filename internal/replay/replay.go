// Package replay lists, plays and exports session recordings.
// This file finds the recordings of a session and plays them back.
package replay

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ynn3m/arsenal/internal/cast"
	"github.com/ynn3m/arsenal/internal/cmdlog"
	"github.com/ynn3m/arsenal/internal/session"
)

const (
	StateComplete    = "complete"
	StateInterrupted = "interrupted"
	StateUnknown     = ""
)

type Info struct {
	ID       string
	Parts    []string
	Header   cast.Header
	Started  time.Time
	Duration time.Duration
	Size     int64
	State    string
	offsets  []time.Duration
}

var partRe = regexp.MustCompile(`^(.+)-(\d{3})$`)

func List(sessionDir, logPath string) ([]Info, error) {
	dir := session.RecordingsDir(sessionDir)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".cast") {
			names[strings.TrimSuffix(e.Name(), ".cast")] = true
		}
	}
	parts := map[string][]string{}
	var bases []string
	for id := range names {
		if m := partRe.FindStringSubmatch(id); m != nil && m[2] != "000" && m[2] != "001" && names[m[1]] {
			parts[m[1]] = append(parts[m[1]], id)
			continue
		}
		bases = append(bases, id)
	}

	recs, _ := cmdlog.Load(logPath, 0)
	started, ended := map[string]bool{}, map[string]bool{}
	for _, r := range recs {
		switch r.Ev {
		case cmdlog.EvRecStart:
			started[r.Rec] = true
		case cmdlog.EvRecEnd:
			ended[r.Rec] = true
		}
	}

	var out []Info
	for _, id := range bases {
		cont := parts[id]
		sort.Strings(cont)
		files := []string{filepath.Join(dir, id+".cast")}
		for _, c := range cont {
			files = append(files, filepath.Join(dir, c+".cast"))
		}
		info, err := build(id, files)
		if err != nil {
			continue
		}
		switch {
		case ended[id]:
			info.State = StateComplete
		case started[id]:
			info.State = StateInterrupted
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Started.Equal(out[j].Started) {
			return out[i].Started.After(out[j].Started)
		}
		return out[i].ID > out[j].ID
	})
	return out, nil
}

func build(id string, files []string) (Info, error) {
	info := Info{ID: id, Parts: files}
	var off time.Duration
	for i, f := range files {
		s, err := cast.Summarize(f)
		if err != nil {
			if i == 0 {
				return Info{}, err
			}
			info.Parts = files[:i]
			break
		}
		if i == 0 {
			info.Header = s.Header
			info.Started = time.Unix(s.Header.Timestamp, 0)
			if s.Header.Timestamp == 0 {
				if fi, err := os.Stat(f); err == nil {
					info.Started = fi.ModTime()
				}
			}
		}
		info.offsets = append(info.offsets, off)
		d := time.Duration(s.Duration * float64(time.Second))
		off += d
		info.Size += s.Size
	}
	info.Duration = off
	return info, nil
}

func Find(sessionDir, logPath, arg string) (Info, error) {
	if arg == "" {
		return Info{}, errors.New("no recording given")
	}
	if strings.ContainsRune(arg, os.PathSeparator) || strings.HasSuffix(arg, ".cast") {
		if fi, err := os.Stat(arg); err == nil && !fi.IsDir() {
			abs, _ := filepath.Abs(arg)
			dir := filepath.Dir(abs)
			id := strings.TrimSuffix(filepath.Base(abs), ".cast")
			if m := partRe.FindStringSubmatch(id); m != nil && m[2] != "000" && m[2] != "001" {
				if _, err := os.Stat(filepath.Join(dir, m[1]+".cast")); err == nil {
					id = m[1]
				}
			}
			files := []string{filepath.Join(dir, id+".cast")}
			for n := 2; ; n++ {
				next := filepath.Join(dir, fmt.Sprintf("%s-%03d.cast", id, n))
				if _, err := os.Stat(next); err != nil {
					break
				}
				files = append(files, next)
			}
			return build(id, files)
		}
	}
	list, err := List(sessionDir, logPath)
	if err != nil {
		return Info{}, err
	}
	var matches []Info
	for _, in := range list {
		if in.ID == arg {
			return in, nil
		}
		if strings.HasPrefix(in.ID, arg) {
			matches = append(matches, in)
		}
	}
	switch len(matches) {
	case 0:
		return Info{}, fmt.Errorf("no recording %q in %s: ars replay --list shows them", arg, session.RecordingsDir(sessionDir))
	case 1:
		return matches[0], nil
	}
	var ids []string
	for _, m := range matches {
		ids = append(ids, m.ID)
	}
	return Info{}, fmt.Errorf("%q matches %d recordings (%s): type more of the id", arg, len(matches), strings.Join(ids, ", "))
}

func (in Info) Walk(fn func(at time.Duration, ev cast.Event) error) error {
	for i, p := range in.Parts {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		r, err := cast.NewReader(f)
		if err != nil {
			f.Close()
			return err
		}
		var off time.Duration
		if i < len(in.offsets) {
			off = in.offsets[i]
		}
		for {
			ev, err := r.Next()
			if err != nil {
				break
			}
			if err := fn(off+time.Duration(ev.Time*float64(time.Second)), ev); err != nil {
				f.Close()
				return err
			}
		}
		f.Close()
	}
	return nil
}

func Clock(d time.Duration) string {
	s := int(d.Round(time.Second) / time.Second)
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
}

func (in Info) Markers() (at []time.Duration, labels []string, err error) {
	err = in.Walk(func(t time.Duration, ev cast.Event) error {
		if ev.Code == cast.CodeMarker {
			at = append(at, t)
			labels = append(labels, ev.Data)
		}
		return nil
	})
	return
}

func (in Info) Text() (string, error) {
	var x Text
	err := in.Walk(func(t time.Duration, ev cast.Event) error {
		switch ev.Code {
		case cast.CodeOutput:
			x.Write([]byte(ev.Data))
		case cast.CodeMarker:
			x.Marker(Clock(t), ev.Data)
		}
		return nil
	})
	return x.String(), err
}

var ErrStopped = errors.New("playback stopped")

type PlayOptions struct {
	Out       io.Writer
	Keys      <-chan byte
	Speed     float64
	IdleLimit time.Duration
	From      time.Duration
	Raw       bool
}

func (in Info) Play(o PlayOptions) error {
	speed := o.Speed
	if speed <= 0 {
		speed = 1
	}
	var san Sanitizer
	paused := false
	var prev time.Duration

	handle := func(k byte) bool {
		switch k {
		case ' ':
			paused = !paused
		case '+', '=':
			if speed < 64 {
				speed *= 2
			}
		case '-', '_':
			if speed > 0.125 {
				speed /= 2
			}
		case '0':
			speed = 1
		case 'q', 'Q', 0x03, 0x04:
			return true
		}
		return false
	}

	wait := func(d time.Duration) bool {
		if o.Keys == nil {
			time.Sleep(d)
			return false
		}
		timer := time.NewTimer(d)
		defer timer.Stop()
		for {
			if paused {
				k, ok := <-o.Keys
				if !ok || handle(k) {
					return true
				}
				continue
			}
			select {
			case <-timer.C:
				return false
			case k, ok := <-o.Keys:
				if !ok {
					return true
				}
				if handle(k) {
					return true
				}
			}
		}
	}

	err := in.Walk(func(t time.Duration, ev cast.Event) error {
		if ev.Code != cast.CodeOutput {
			return nil
		}
		if t >= o.From {
			gap := t - prev
			if o.IdleLimit > 0 && gap > o.IdleLimit {
				gap = o.IdleLimit
			}
			if d := time.Duration(float64(gap) / speed); d > 0 || paused {
				if wait(d) {
					return ErrStopped
				}
			}
		}
		prev = t
		data := []byte(ev.Data)
		if !o.Raw {
			data = san.Filter(data)
		}
		_, err := o.Out.Write(data)
		return err
	})
	if !o.Raw {
		_, _ = o.Out.Write(san.Flush())
	}
	_, _ = io.WriteString(o.Out, Reset)
	return err
}
