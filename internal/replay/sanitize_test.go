package replay

import (
	"strings"
	"testing"
)

func sanitize(chunks ...string) string {
	var s Sanitizer
	var out strings.Builder
	for _, c := range chunks {
		out.Write(s.Filter([]byte(c)))
	}
	out.Write(s.Flush())
	return out.String()
}

func TestSanitizerKeepsWhatDrawsText(t *testing.T) {
	keep := []string{
		"plain text, unicode: héllo ✓ 日本語 \u00a0 é\r\n\ttab\b",
		"\x1b[0m\x1b[1;31;48;5;200mcolour\x1b[39;49m",
		"\x1b[2J\x1b[H\x1b[10;20Hmove\x1b[K\x1b[2K\x1b[3A\x1b[4B\x1b[5C\x1b[6D\x1b[3L\x1b[2M\x1b[P\x1b[2X\x1b[1S\x1b[1T",
		"\x1b[1;24r\x1b7\x1b8\x1b[s\x1b[u\x1b=\x1b>\x1bM\x1bD\x1bE\x1bH",
		"\x1b(B\x1b)0\x1b(0lqk\x1b(B",
		"\x1b[?1049h\x1b[?25l screen \x1b[?25h\x1b[?1049l\x1b[?2004h\x1b[?1000h\x1b[?1000l",
		"\x1b[6 q",
	}
	for _, in := range keep {
		if got := sanitize(in); got != in {
			t.Errorf("changed:\n in  %q\n out %q", in, got)
		}
	}
}

func TestSanitizerDropsWhatTalksToTheTerminalOrBeyond(t *testing.T) {
	drop := map[string]string{
		"OSC 52 clipboard write (BEL)":        "\x1b]52;c;ZXZpbA==\x07",
		"OSC 52 clipboard write (ST)":         "\x1b]52;c;ZXZpbA==\x1b\\",
		"OSC title":                           "\x1b]0;owned\x07",
		"OSC colour query":                    "\x1b]11;?\x07",
		"OSC hyperlink":                       "\x1b]8;;http://example.local\x07",
		"OSC notification":                    "\x1b]777;notify;t;b\x07",
		"our own marker sequence":             "\x1b]7777;ars;marker;x\x07",
		"DCS (sixel, DECRQSS)":                "\x1bP$qm\x1b\\",
		"APC":                                 "\x1b_Gf=24;AAAA\x1b\\",
		"PM":                                  "\x1b^privacy\x1b\\",
		"SOS":                                 "\x1bXstring\x1b\\",
		"device attributes":                   "\x1b[c\x1b[0c",
		"secondary device attributes":         "\x1b[>c",
		"cursor position report request":      "\x1b[6n",
		"status report request":               "\x1b[5n",
		"window report (title)":               "\x1b[21t",
		"window operation":                    "\x1b[8;50;200t",
		"mode query":                          "\x1b[?25$p",
		"keyboard protocol push":              "\x1b[>1u",
		"keyboard protocol query":             "\x1b[?u",
		"DECID":                               "\x1bZ",
		"RIS":                                 "\x1bc",
		"enquiry":                             "\x05",
		"private prefixed sequence":           "\x1b[=1c",
		"DECSTR soft reset with intermediate": "\x1b[!p",
	}
	for name, in := range drop {
		got := sanitize("A" + in + "B")
		if strings.Contains(got, "\x1b") && strings.Contains(strings.ReplaceAll(got, "A", ""), "\x1b") && got != "AB" {
			t.Errorf("%s: left %q", name, got)
		}
		if got != "AB" {
			t.Errorf("%s: %q -> %q, want AB", name, in, got)
		}
	}
}

func TestSanitizerRemovesC1IntroducersSoWhatFollowsIsOnlyText(t *testing.T) {
	cases := map[string]string{
		"8-bit CSI":    "A\x9b6nB",
		"UTF-8 C1 CSI": "A\u009b6nB",
		"UTF-8 C1 OSC": "A\u009d52;c;ZXZpbA==\u009cB",
	}
	for name, in := range cases {
		got := sanitize(in)
		for _, c := range []string{"\x9b", "\x9d", "\x9c", "\u009b", "\u009d", "\u009c", "\x1b"} {
			if strings.Contains(got, c) {
				t.Errorf("%s: control left in %q", name, got)
			}
		}
		if !strings.HasPrefix(got, "A") || !strings.HasSuffix(got, "B") {
			t.Errorf("%s: text around lost: %q", name, got)
		}
	}
}

func TestSanitizerWorksWhenSequencesAreSplitAnywhere(t *testing.T) {
	in := "a\x1b]52;c;ZXZpbA==\x07b\x1b[1;31mc\x1b[6nd\u009be é\x1b]0;t\x1b\\f"
	want := "ab\x1b[1;31mcd" + "e é" + "f"
	for cut := 1; cut < len(in); cut++ {
		if got := sanitize(in[:cut], in[cut:]); got != want {
			t.Fatalf("cut at %d:\n got  %q\n want %q", cut, got, want)
		}
	}
	var parts []string
	for i := 0; i < len(in); i++ {
		parts = append(parts, in[i:i+1])
	}
	if got := sanitize(parts...); got != want {
		t.Errorf("byte by byte:\n got  %q\n want %q", got, want)
	}
}

func TestSanitizerSurvivesGarbage(t *testing.T) {
	for _, in := range []string{
		"\x1b[" + strings.Repeat("9;", 500) + "m after",
		"\x1b]" + strings.Repeat("x", 20000) + "\x07 after",
		"\x1b]52;never ends " + strings.Repeat("y", 100),
		"\x1b[\x1b[31mred",
		"\x1b\x1b\x1b[0m",
		"\xff\xfe\xc3\x28 text",
		"\x1b[31",
	} {
		_ = sanitize(in, in)
	}
	if got := sanitize("\x1b[\x1b[31mred"); got != "\x1b[31mred" {
		t.Errorf("an aborted sequence must not eat the next one: %q", got)
	}
	if got := sanitize("\x1b]52;never ends ", "x", "BEL later\x07done"); got != "done" {
		t.Errorf("a string ends at its terminator, in a later chunk: %q", got)
	}
}

func TestResetUndoesTheModesOfARecording(t *testing.T) {
	for _, want := range []string{"\x1b[?1049l", "\x1b[?25h", "\x1b[?1000l", "\x1b[?2004l", "\x1b[0m"} {
		if !strings.Contains(Reset, want) {
			t.Errorf("Reset lacks %q", want)
		}
	}
	if sanitize(Reset) != Reset {
		t.Error("Reset must survive the sanitizer (it is written after it, but must be harmless in it)")
	}
}
