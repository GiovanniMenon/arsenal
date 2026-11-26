package replay

import (
	"strings"
	"testing"
)

func render(chunks ...string) string {
	var x Text
	for _, c := range chunks {
		x.Write([]byte(c))
	}
	return x.String()
}

func TestTextRendersLinesCarriageReturnsAndBackspaces(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello\r\nworld\r\n", "hello\nworld\n"},
		{"no newline at the end", "no newline at the end\n"},
		{"abc\rX", "Xbc\n"},
		{"abc\b\bX", "aXc\n"},
		{"a\tb", "a       b\n"},
		{"\x1b[31mred\x1b[0m plain", "red plain\n"},
		{"hello\x1b[3D\x1b[KXY", "heXY\n"},
		{"abcdef\x1b[2K\rZ", "Z\n"},
		{"abcdef\x1b[1K", "      \n"[:0] + "\n"},
		{"ab\x1b[5Cc", "ab     c\n"},
		{"abcdef\r\x1b[2P", "cdef\n"},
		{"abcdef\r\x1b[2X", "  cdef\n"},
		{"\x1b]0;title\x07text \x1b]52;c;ZXZpbA==\x07here", "text here\n"},
		{"é ✓ 日本語\u00a0x", "é ✓ 日本語\u00a0x\n"},
		{"", ""},
	}
	for _, c := range cases {
		if got := render(c.in); got != c.want {
			t.Errorf("render(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTextOmitsFullScreenPrograms(t *testing.T) {
	got := render("before\r\n\x1b[?1049h\x1b[H\x1b[2Jscreen text\r\nmore screen\x1b[?1049l", "after\r\n")
	if got != "before\n[full-screen program output omitted]\nafter\n" {
		t.Errorf("got %q", got)
	}
}

func TestTextHandlesSplitSequencesAndMarkers(t *testing.T) {
	var x Text
	for _, b := range []byte("ab\x1b[3") {
		x.Write([]byte{b})
	}
	x.Write([]byte("1mcd\x1b[0m\r\n"))
	x.Marker("00:01:05", "ars: prompt: scan specific ports")
	x.Write([]byte("tail"))
	want := "abcd\n[marker 00:01:05] ars: prompt: scan specific ports\ntail\n"
	if got := x.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if strings.Contains(x.String(), "\x1b") {
		t.Error("escape left in the text")
	}
}
