// Strips the escape sequences of a recording that could act on the viewer's terminal.

package replay

const Reset = "\x1b[?1049l\x1b[?25h\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?2004l\x1b[0m\r\n"

type Sanitizer struct {
	tk tokenizer
}

func (s *Sanitizer) Filter(chunk []byte) []byte {
	var out []byte
	s.tk.Feed(chunk, func(t token) {
		if keep(t) {
			out = append(out, t.raw...)
		}
	})
	return out
}

func (s *Sanitizer) Flush() []byte {
	var out []byte
	s.tk.Flush(func(t token) {
		if keep(t) {
			out = append(out, t.raw...)
		}
	})
	return out
}

func keep(t token) bool {
	switch t.kind {
	case tokPrint:
		return true
	case tokControl:
		return t.raw[0] != 0x05
	case tokEsc:
		return t.final != 'Z' && !(t.final == 'c' && t.inter == "") && keepEsc(t)
	case tokCSI:
		return keepCSI(t)
	}
	return false
}

func keepEsc(t token) bool {
	if t.inter != "" {
		return t.inter == "(" || t.inter == ")" || t.inter == "*" || t.inter == "+" || t.inter == "#"
	}
	switch t.final {
	case '7', '8', '=', '>', 'D', 'E', 'H', 'M':
		return true
	}
	return false
}

func keepCSI(t token) bool {
	if t.prefix == '<' || t.prefix == '=' || t.prefix == '>' {
		return false
	}
	if t.prefix == '?' {
		return (t.final == 'h' || t.final == 'l') && t.inter == ""
	}
	switch t.final {
	case 'm', 'A', 'B', 'C', 'D', 'E', 'F', 'G', 'H', 'f', 'd', 'J', 'K', 'L', 'M', 'P', '@', 'X', 'S', 'T', 'r', 's', 'a', 'e', '`', 'b':
		return t.inter == ""
	case 'u':
		return t.inter == "" && len(t.params) == 0
	case 'q':
		return t.inter == " "
	case 'h', 'l':
		return t.inter == ""
	}
	return false
}
