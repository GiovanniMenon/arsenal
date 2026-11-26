// Turns a recording into plain text, for ars replay --text.

package replay

import (
	"fmt"
	"strings"
)

type Text struct {
	tk    tokenizer
	lines []string
	cur   []rune
	col   int
	alt   bool
}

func (x *Text) Write(data []byte) {
	x.tk.Feed(data, x.token)
}

func (x *Text) Marker(at, label string) {
	x.endLine()
	x.lines = append(x.lines, fmt.Sprintf("[marker %s] %s", at, label))
}

func (x *Text) endLine() {
	if len(x.cur) > 0 || x.col > 0 {
		x.lines = append(x.lines, strings.TrimRight(string(x.cur), " "))
	}
	x.cur, x.col = nil, 0
}

func (x *Text) newline() {
	x.lines = append(x.lines, strings.TrimRight(string(x.cur), " "))
	x.cur, x.col = nil, 0
}

func (x *Text) put(r rune) {
	for len(x.cur) < x.col {
		x.cur = append(x.cur, ' ')
	}
	if x.col < len(x.cur) {
		x.cur[x.col] = r
	} else {
		x.cur = append(x.cur, r)
	}
	x.col++
}

func (x *Text) token(t token) {
	switch t.kind {
	case tokPrint:
		if x.alt {
			return
		}
		r := []rune(string(t.raw))
		if len(r) == 1 && r[0] == 0xfffd && len(t.raw) == 1 {
			return
		}
		for _, c := range r {
			x.put(c)
		}
	case tokControl:
		if x.alt {
			return
		}
		switch t.raw[0] {
		case '\n':
			x.newline()
		case '\r':
			x.col = 0
		case '\b':
			if x.col > 0 {
				x.col--
			}
		case '\t':
			next := (x.col/8 + 1) * 8
			for x.col < next {
				x.put(' ')
			}
		}
	case tokCSI:
		x.csi(t)
	}
}

func (x *Text) csi(t token) {
	if t.prefix == '?' && (t.final == 'h' || t.final == 'l') {
		for _, p := range t.params {
			if p == 1049 || p == 1047 || p == 47 {
				if t.final == 'h' && !x.alt {
					x.endLine()
					x.lines = append(x.lines, "[full-screen program output omitted]")
					x.alt = true
				} else if t.final == 'l' {
					x.alt = false
					x.cur, x.col = nil, 0
				}
			}
		}
		return
	}
	if x.alt || t.prefix != 0 || t.inter != "" {
		return
	}
	n := t.param(0, 1)
	if n == 0 {
		n = 1
	}
	switch t.final {
	case 'K':
		switch t.param(0, 0) {
		case 0:
			if x.col < len(x.cur) {
				x.cur = x.cur[:x.col]
			}
		case 1:
			for i := 0; i <= x.col && i < len(x.cur); i++ {
				x.cur[i] = ' '
			}
		case 2:
			x.cur = nil
		}
	case 'C':
		x.col += n
	case 'D':
		x.col -= n
		if x.col < 0 {
			x.col = 0
		}
	case 'G':
		x.col = t.param(0, 1) - 1
		if x.col < 0 {
			x.col = 0
		}
	case 'P':
		if x.col < len(x.cur) {
			end := x.col + n
			if end > len(x.cur) {
				end = len(x.cur)
			}
			x.cur = append(x.cur[:x.col], x.cur[end:]...)
		}
	case 'X':
		for i := 0; i < n && x.col+i < len(x.cur); i++ {
			x.cur[x.col+i] = ' '
		}
	}
}

func (x *Text) String() string {
	x.tk.Flush(x.token)
	out := append([]string{}, x.lines...)
	if len(x.cur) > 0 {
		out = append(out, strings.TrimRight(string(x.cur), " "))
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}
