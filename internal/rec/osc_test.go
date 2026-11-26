//go:build darwin || linux

package rec

import (
	"strings"
	"testing"
)

func process(f *oscFilter, chunks ...string) (string, []Command) {
	var out strings.Builder
	var cmds []Command
	for _, c := range chunks {
		for _, it := range f.Process([]byte(c)) {
			if it.Cmd != nil {
				cmds = append(cmds, *it.Cmd)
			} else {
				out.Write(it.Pass)
			}
		}
	}
	out.Write(f.Flush())
	return out.String(), cmds
}

func TestFilterConsumesOurSequencesAndPassesTheRestUntouched(t *testing.T) {
	plain := "hello \x1b[31mred\x1b[0m \x1b]0;a title\x07 \x1b]52;c;ZGF0YQ==\x07 end"
	got, cmds := process(&oscFilter{}, plain)
	if got != plain || len(cmds) != 0 {
		t.Errorf("foreign sequences changed: %q %v", got, cmds)
	}

	in := "a" + Sequence(VerbMarker, "scan specific ports") + "b" + Sequence(VerbPause, "") + "c"
	got, cmds = process(&oscFilter{}, in)
	if got != "abc" {
		t.Errorf("passed %q, want abc", got)
	}
	if len(cmds) != 2 || cmds[0] != (Command{VerbMarker, "scan specific ports"}) || cmds[1] != (Command{VerbPause, ""}) {
		t.Errorf("commands = %+v", cmds)
	}
}

func TestFilterHandlesSequencesSplitOverChunksAtEveryPosition(t *testing.T) {
	seq := "x" + Sequence(VerbMarker, "tool: nmap -sV") + "y\x1b[0m"
	for cut := 1; cut < len(seq); cut++ {
		got, cmds := process(&oscFilter{}, seq[:cut], seq[cut:])
		if got != "xy\x1b[0m" || len(cmds) != 1 || cmds[0].Text != "tool: nmap -sV" {
			t.Fatalf("cut at %d: passed %q, commands %+v", cut, got, cmds)
		}
	}
	var parts []string
	for i := 0; i < len(seq); i++ {
		parts = append(parts, seq[i:i+1])
	}
	got, cmds := process(&oscFilter{}, parts...)
	if got != "xy\x1b[0m" || len(cmds) != 1 {
		t.Errorf("byte by byte: %q %+v", got, cmds)
	}
}

func TestFilterAcceptsStringTerminator(t *testing.T) {
	got, cmds := process(&oscFilter{}, "a\x1b]7777;ars;marker;label\x1b\\b")
	if got != "ab" || len(cmds) != 1 || cmds[0].Text != "label" {
		t.Errorf("%q %+v", got, cmds)
	}
}

func TestFilterDoesNotEatTextWhenASequenceNeverEnds(t *testing.T) {
	junk := oscPrefix + "marker;" + strings.Repeat("z", 2*maxOSC)
	got, cmds := process(&oscFilter{}, junk, "tail")
	if len(cmds) != 0 || got != junk+"tail" {
		t.Errorf("an unterminated sequence swallowed output: %d bytes out of %d, commands %v", len(got), len(junk)+4, cmds)
	}
	got, _ = process(&oscFilter{}, "text\x1b")
	if got != "text\x1b" {
		t.Errorf("lone escape: %q", got)
	}
}

func TestSequenceCleansTheText(t *testing.T) {
	s := Sequence(VerbMarker, "evil\x1b]0;x\x07 text\n"+strings.Repeat("é", 300))
	body := strings.TrimSuffix(strings.TrimPrefix(s, oscPrefix), "\a")
	if strings.ContainsAny(body, "\x1b\x07\n") {
		t.Errorf("control characters in %q", body)
	}
	if len(body) > 200+len("marker;") {
		t.Errorf("text not cut: %d bytes", len(body))
	}
	_, cmds := process(&oscFilter{}, s)
	if len(cmds) != 1 || !strings.HasPrefix(cmds[0].Text, "evil") {
		t.Errorf("round trip: %+v", cmds)
	}
}

func TestFilterKeepsMessagesBetweenTheBytesTheyArrivedWith(t *testing.T) {
	var f oscFilter
	items := f.Process([]byte("a" + Sequence(VerbPause, "") + "b" + Sequence(VerbResume, "") + "c"))
	var order []string
	for _, it := range items {
		if it.Cmd != nil {
			order = append(order, "<"+it.Cmd.Verb+">")
		} else {
			order = append(order, string(it.Pass))
		}
	}
	if strings.Join(order, "") != "a<pause>b<resume>c" {
		t.Errorf("order = %q", order)
	}
}
