// Splits terminal output into text and escape sequences.

package replay

import (
	"unicode/utf8"
)

type tokKind int

const (
	tokPrint tokKind = iota
	tokControl
	tokCSI
	tokEsc
	tokString
	tokC1
)

type token struct {
	kind   tokKind
	raw    []byte
	prefix byte
	params []int
	inter  string
	final  byte
	str    byte
}

const maxSeq = 128

const maxString = 8192

type tkState int

const (
	tkGround tkState = iota
	tkEsc
	tkEscInter
	tkCSI
	tkString
	tkStringEsc
	tkC2
)

type tokenizer struct {
	state tkState
	buf   []byte
	utf   []byte
	str   byte
	slen  int
}

func (t *tokenizer) emit(fn func(token), tk token) { fn(tk) }

func (t *tokenizer) Feed(data []byte, fn func(token)) {
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch t.state {
		case tkGround:
			t.ground(b, fn)

		case tkC2:
			t.state = tkGround
			if b >= 0x80 && b <= 0x9f {
				fn(token{kind: tokC1, raw: []byte{0xc2, b}})
				continue
			}
			if b >= 0xa0 && b <= 0xbf {
				fn(token{kind: tokPrint, raw: []byte{0xc2, b}})
				continue
			}
			fn(token{kind: tokPrint, raw: []byte{0xc2}})
			t.ground(b, fn)

		case tkEsc:
			t.buf = append(t.buf, b)
			switch {
			case b == '[':
				t.state = tkCSI
			case b == ']' || b == 'P' || b == '_' || b == '^' || b == 'X':
				t.state, t.str, t.slen = tkString, b, 0
			case b == 0x1b:
				t.buf = t.buf[:1]
			case b >= 0x20 && b <= 0x2f:
				t.state = tkEscInter
			case b >= 0x30 && b <= 0x7e:
				fn(token{kind: tokEsc, raw: t.take(), final: b})
				t.state = tkGround
			default:
				t.buf = nil
				t.state = tkGround
				t.ground(b, fn)
			}

		case tkEscInter:
			t.buf = append(t.buf, b)
			switch {
			case b >= 0x20 && b <= 0x2f:
				if len(t.buf) > maxSeq {
					t.buf, t.state = nil, tkGround
				}
			case b >= 0x30 && b <= 0x7e:
				raw := t.take()
				fn(token{kind: tokEsc, raw: raw, inter: string(raw[1 : len(raw)-1]), final: b})
				t.state = tkGround
			default:
				t.buf, t.state = nil, tkGround
				t.ground(b, fn)
			}

		case tkCSI:
			switch {
			case b >= 0x20 && b <= 0x3f:
				t.buf = append(t.buf, b)
				if len(t.buf) > maxSeq {
					t.buf, t.state = nil, tkGround
				}
			case b >= 0x40 && b <= 0x7e:
				t.buf = append(t.buf, b)
				tk := parseCSI(t.take())
				fn(tk)
				t.state = tkGround
			default:
				t.buf, t.state = nil, tkGround
				if b != 0x18 && b != 0x1a {
					t.ground(b, fn)
				}
			}

		case tkString:
			t.slen++
			switch {
			case b == 0x07:
				fn(token{kind: tokString, str: t.str})
				t.buf, t.state = nil, tkGround
			case b == 0x1b:
				t.state = tkStringEsc
			case b == 0x18 || b == 0x1a:
				t.buf, t.state = nil, tkGround
			case t.slen > maxString:
				fn(token{kind: tokString, str: t.str})
				t.buf, t.state = nil, tkGround
			}

		case tkStringEsc:
			if b == '\\' {
				fn(token{kind: tokString, str: t.str})
				t.buf, t.state = nil, tkGround
				continue
			}
			fn(token{kind: tokString, str: t.str})
			t.buf, t.state = []byte{0x1b}, tkEsc
			i--
		}
	}
}

func (t *tokenizer) take() []byte {
	b := t.buf
	t.buf = nil
	return b
}

func (t *tokenizer) ground(b byte, fn func(token)) {
	if len(t.utf) > 0 {
		t.utf = append(t.utf, b)
		if utf8.FullRune(t.utf) {
			r, size := utf8.DecodeRune(t.utf)
			if r == utf8.RuneError && size <= 1 {
				first := t.utf[0]
				rest := append([]byte{}, t.utf[1:]...)
				t.utf = nil
				fn(token{kind: tokPrint, raw: []byte{first}})
				for _, rb := range rest {
					t.ground(rb, fn)
				}
				return
			}
			raw := append([]byte{}, t.utf...)
			t.utf = nil
			fn(token{kind: tokPrint, raw: raw})
		}
		return
	}
	switch {
	case b == 0x1b:
		t.buf = []byte{0x1b}
		t.state = tkEsc
	case b == 0xc2:
		t.state = tkC2
	case b < 0x20 || b == 0x7f:
		fn(token{kind: tokControl, raw: []byte{b}})
	case b >= 0x80 && b <= 0x9f:
		fn(token{kind: tokC1, raw: []byte{b}})
	case b < 0x80:
		fn(token{kind: tokPrint, raw: []byte{b}})
	default:
		t.utf = []byte{b}
	}
}

func (t *tokenizer) Flush(fn func(token)) {
	for _, b := range t.utf {
		fn(token{kind: tokPrint, raw: []byte{b}})
	}
	t.utf = nil
	t.buf = nil
	t.state = tkGround
}

func parseCSI(raw []byte) token {
	tk := token{kind: tokCSI, raw: raw, final: raw[len(raw)-1]}
	body := raw[2 : len(raw)-1]
	i := 0
	if i < len(body) && body[i] >= '<' && body[i] <= '?' {
		tk.prefix = body[i]
		i++
	}
	cur, have := 0, false
	for ; i < len(body); i++ {
		c := body[i]
		switch {
		case c >= '0' && c <= '9':
			if cur < 1<<20 {
				cur = cur*10 + int(c-'0')
			}
			have = true
		case c == ';' || c == ':':
			if have {
				tk.params = append(tk.params, cur)
			} else {
				tk.params = append(tk.params, -1)
			}
			cur, have = 0, false
		default:
			tk.inter += string(c)
		}
	}
	if have {
		tk.params = append(tk.params, cur)
	} else if len(body) > 0 && (body[len(body)-1] == ';' || body[len(body)-1] == ':') {
		tk.params = append(tk.params, -1)
	}
	return tk
}

func (tk token) param(i, def int) int {
	if i < len(tk.params) && tk.params[i] >= 0 {
		return tk.params[i]
	}
	return def
}
