//go:build darwin || linux

// The private escape sequence ars uses to send markers, pause and resume to the recorder.

package rec

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	oscPrefix = "\x1b]7777;ars;"

	VerbMarker = "marker"
	VerbPause  = "pause"
	VerbResume = "resume"

	maxOSC = 512
)

func Sequence(verb, text string) string {
	return oscPrefix + verb + ";" + cleanText(text) + "\a"
}

func cleanText(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(s, ""))
	s = strings.TrimSpace(s)
	const max = 200
	if len(s) > max {
		cut := max
		for cut > 0 && !utf8.RuneStart(s[cut]) {
			cut--
		}
		s = s[:cut]
	}
	return s
}

type Command struct {
	Verb string
	Text string
}

type oscFilter struct {
	hold []byte
}

type Item struct {
	Pass []byte
	Cmd  *Command
}

func (f *oscFilter) Process(chunk []byte) []Item {
	data := chunk
	if len(f.hold) > 0 {
		data = append(append([]byte{}, f.hold...), chunk...)
		f.hold = nil
	}
	var items []Item
	var out []byte
	flush := func() {
		if len(out) > 0 {
			items = append(items, Item{Pass: out})
			out = nil
		}
	}
	for len(data) > 0 {
		i := bytes.IndexByte(data, 0x1b)
		if i < 0 {
			out = append(out, data...)
			break
		}
		out = append(out, data[:i]...)
		data = data[i:]
		switch {
		case bytes.HasPrefix(data, []byte(oscPrefix)):
			end, termLen := oscEnd(data)
			switch {
			case end >= 0:
				body := string(data[len(oscPrefix):end])
				verb, text, _ := strings.Cut(body, ";")
				flush()
				items = append(items, Item{Cmd: &Command{Verb: verb, Text: text}})
				data = data[end+termLen:]
			case len(data) > maxOSC:
				out = append(out, data[0])
				data = data[1:]
			default:
				f.hold = append([]byte{}, data...)
				flush()
				return items
			}
		case len(data) < len(oscPrefix) && bytes.HasPrefix([]byte(oscPrefix), data):
			f.hold = append([]byte{}, data...)
			flush()
			return items
		default:
			out = append(out, data[0])
			data = data[1:]
		}
	}
	flush()
	return items
}

func (f *oscFilter) Flush() []byte {
	h := f.hold
	f.hold = nil
	return h
}

func oscEnd(data []byte) (int, int) {
	for i := len(oscPrefix); i < len(data) && i <= maxOSC; i++ {
		switch data[i] {
		case 0x07:
			return i, 1
		case 0x1b:
			if i+1 < len(data) && data[i+1] == '\\' {
				return i, 2
			}
			if i+1 >= len(data) {
				return -1, 0
			}
		}
	}
	return -1, 0
}
