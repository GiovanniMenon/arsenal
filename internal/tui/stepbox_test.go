package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func sampleBox() StepBox {
	return StepBox{
		Title: "Demo",
		Steps: []Step{
			{Key: "folder", Title: "Folder", Help: "where it goes", Kind: StepText, Default: "~/pt",
				Validate: func(s string) error {
					if s == "" {
						return errors.New("a folder is required")
					}
					return nil
				}},
			{Key: "mode", Title: "Mode", Kind: StepChoice, Options: []string{"Install", "Import", "Skip"}},
			{Key: "url", Title: "URL", Kind: StepText, When: func(a Answers) bool { return a.Choice("mode") == 1 }},
			{Key: "text", Title: "Text", Kind: StepMultiline},
		},
		Summary: func(a Answers) []string {
			return Rows([2]string{"Folder", a.Text("folder")}, [2]string{"Mode", a.Text("mode")})
		},
	}
}

func stepKey(m tea.Model, msgs ...tea.Msg) stepModel {
	for _, msg := range msgs {
		m, _ = m.Update(msg)
	}
	return m.(stepModel)
}

func stepType(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	kEnter = tea.KeyMsg{Type: tea.KeyEnter}
	kEsc   = tea.KeyMsg{Type: tea.KeyEsc}
	kDown  = tea.KeyMsg{Type: tea.KeyDown}
	kUp    = tea.KeyMsg{Type: tea.KeyUp}
	kCtrlC = tea.KeyMsg{Type: tea.KeyCtrlC}
	kAltEn = tea.KeyMsg{Type: tea.KeyEnter, Alt: true}
	kCtrlJ = tea.KeyMsg{Type: tea.KeyCtrlJ}
	kTab   = tea.KeyMsg{Type: tea.KeyTab}
)

func plainView(m stepModel) string { return ansi.Strip(m.View()) }

func TestStepBoxAsksOneQuestionAtATimeInOneBox(t *testing.T) {
	var m tea.Model = newStepModel(sampleBox())
	v := plainView(m.(stepModel))
	for _, want := range []string{"Demo", "1/3", "Folder", "where it goes", "> ~/pt"} {
		if !strings.Contains(v, want) {
			t.Errorf("first step lacks %q:\n%s", want, v)
		}
	}
	if strings.Count(v, "\u256d") != 1 || strings.Count(v, "\u2570") != 1 {
		t.Errorf("want exactly one rounded box:\n%s", v)
	}
	if strings.Contains(v, "Mode") || strings.Contains(v, "Install") {
		t.Errorf("a later question is already visible:\n%s", v)
	}
	lines := strings.Split(strings.TrimRight(v, "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "enter: next") || !strings.Contains(last, "esc: cancel") || !strings.Contains(last, "ctrl+c") {
		t.Errorf("help line under the box = %q", last)
	}
	for _, r := range nonASCII(v) {
		if r < 0x2500 || r > 0x257f {
			t.Errorf("non-ASCII %q in the box", r)
		}
	}

	if top := strings.Split(v, "\n")[0]; !strings.Contains(top, "Demo - Folder") || !strings.Contains(top, "1/3") {
		t.Errorf("top border = %q, want it to hold %q and the counter", top, "Demo - Folder")
	}
	if body := strings.Join(strings.Split(v, "\n")[1:], "\n"); strings.Contains(body, "Folder") {
		t.Errorf("the question title is repeated inside the box:\n%s", v)
	}

	sm := stepKey(m, kEnter)
	v = plainView(sm)
	if top := strings.Split(v, "\n")[0]; !strings.Contains(top, "Demo - Mode") {
		t.Errorf("second top border = %q", top)
	}
	if strings.Contains(v, "where it goes") || !strings.Contains(v, "Mode") || !strings.Contains(v, "2/3") {
		t.Errorf("second step:\n%s", v)
	}
	if strings.Count(v, "\u256d") != 1 {
		t.Errorf("still one box expected:\n%s", v)
	}
}

func TestStepBoxChoiceUsesGreaterThanCursor(t *testing.T) {
	sm := stepKey(newStepModel(sampleBox()), kEnter)
	v := plainView(sm)
	if !strings.Contains(v, "> Install") || !strings.Contains(v, "  Import") || !strings.Contains(v, "  Skip") {
		t.Errorf("choice list:\n%s", v)
	}
	sm = stepKey(sm, kDown)
	v = plainView(sm)
	if !strings.Contains(v, "> Import") || strings.Contains(v, "> Install") {
		t.Errorf("down did not move the cursor:\n%s", v)
	}
	sm = stepKey(sm, kUp, kUp)
	if !strings.Contains(plainView(sm), "> Skip") {
		t.Errorf("up did not wrap:\n%s", plainView(sm))
	}
}

func TestStepBoxFullFlowSummaryAndConfirm(t *testing.T) {
	var m tea.Model = newStepModel(sampleBox())
	sm := stepKey(m, kCtrlC)
	if !sm.canceled {
		t.Fatal("ctrl+c did not cancel")
	}

	m = newStepModel(sampleBox())
	sm = stepKey(m, kEnter)
	sm = stepKey(sm, kEnter)
	sm = stepKey(sm, stepType("hello"), kEnter)
	if !sm.summary {
		t.Fatalf("not at the summary:\n%s", plainView(sm))
	}
	v := plainView(sm)
	for _, want := range []string{"Demo", "3/3", "Folder", "~/pt", "Mode", "Install", "enter: confirm", "esc: back"} {
		if !strings.Contains(v, want) {
			t.Errorf("summary lacks %q:\n%s", want, v)
		}
	}
	if strings.Contains(v, "URL") {
		t.Errorf("a step that was not asked is in the summary:\n%s", v)
	}
	sm = stepKey(sm, kEnter)
	if !sm.done || sm.canceled {
		t.Fatalf("enter on the summary did not confirm: done=%v canceled=%v", sm.done, sm.canceled)
	}
	final := plainView(sm)
	if !strings.Contains(final, "Folder") || !strings.Contains(final, "Install") || strings.Contains(final, "enter:") {
		t.Errorf("final view:\n%s", final)
	}
	if got := sm.answers; got.Text("folder") != "~/pt" || got.Choice("mode") != 0 || got.Text("text") != "hello" {
		t.Errorf("answers = %+v", got)
	}
}

func TestStepBoxEscGoesBackAndCancelsOnTheFirst(t *testing.T) {
	sm := stepKey(newStepModel(sampleBox()), stepType("2"), kEnter)
	sm = stepKey(sm, kDown, kEnter)
	if v := plainView(sm); !strings.Contains(v, "URL") || !strings.Contains(v, "3/4") {
		t.Fatalf("conditional step not asked / counter not updated:\n%s", v)
	}
	sm = stepKey(sm, stepType("https://example.local/r.git"), kEsc)
	if v := plainView(sm); !strings.Contains(v, "Mode") || !strings.Contains(v, "> Import") {
		t.Errorf("esc did not go back with the answer kept:\n%s", v)
	}
	sm = stepKey(sm, kEsc)
	if v := plainView(sm); !strings.Contains(v, "> ~/pt2") {
		t.Errorf("the earlier answer is not prefilled:\n%s", v)
	}
	if sm.canceled {
		t.Fatal("cancelled too early")
	}
	sm = stepKey(sm, kEnter, kDown, kEnter)
	if v := plainView(sm); strings.Contains(v, "URL") || !strings.Contains(v, "3/3") {
		t.Errorf("the URL step must be gone:\n%s", v)
	}
	sm = stepKey(newStepModel(sampleBox()), kEsc)
	if !sm.canceled || !sm.done {
		t.Error("esc on the first step must cancel")
	}
	sm = stepKey(newStepModel(sampleBox()), kEnter, kEnter, kEnter)
	if !sm.summary {
		t.Fatal("expected the summary")
	}
	sm = stepKey(sm, kEsc)
	if sm.summary || sm.box.Steps[sm.cur].Key != "text" {
		t.Errorf("esc on the summary: summary=%v cur=%d", sm.summary, sm.cur)
	}
}

func TestStepBoxValidationKeepsTheQuestion(t *testing.T) {
	sm := stepKey(newStepModel(sampleBox()), tea.KeyMsg{Type: tea.KeyCtrlU}, kEnter)
	v := plainView(sm)
	if !strings.Contains(v, "error: a folder is required") || !strings.Contains(v, "1/3") {
		t.Errorf("empty answer accepted:\n%s", v)
	}
	sm = stepKey(sm, stepType("~/x"), kEnter)
	if !strings.Contains(plainView(sm), "2/3") {
		t.Errorf("valid answer refused:\n%s", plainView(sm))
	}
}

func TestStepBoxMultilineEnterIsNextAltEnterAndCtrlJAreNewLine(t *testing.T) {
	b := StepBox{Title: "Note", Steps: []Step{{Key: "text", Title: "Text", Kind: StepMultiline}}, Summary: func(a Answers) []string { return strings.Split(a.Text("text"), "\n") }}
	sm := stepKey(newStepModel(b), stepType("one"), kAltEn, stepType("two"), kCtrlJ, stepType("three"))
	if sm.summary {
		t.Fatal("alt+enter / ctrl+j must not advance")
	}
	sm = stepKey(sm, kEnter)
	if !sm.summary || sm.answers.Text("text") != "one\ntwo\nthree" {
		t.Errorf("summary=%v text=%q", sm.summary, sm.answers.Text("text"))
	}
	if v := plainView(sm); !strings.Contains(v, "one") || !strings.Contains(v, "three") {
		t.Errorf("summary lacks the text:\n%s", v)
	}
}

func TestStepBoxPathTabCompletes(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "engagements"), 0o700); err != nil {
		t.Fatal(err)
	}
	b := StepBox{Title: "P", Steps: []Step{{Key: "p", Title: "Path", Kind: StepPath, Default: dir + "/eng"}}}
	sm := stepKey(newStepModel(b), kTab)
	if got := sm.input.Value(); got != dir+"/engagements/" {
		t.Errorf("tab: %q", got)
	}
	if sm.summary {
		t.Error("tab must not advance")
	}
	if v := plainView(newStepModel(b)); !strings.Contains(v, "tab: complete path") {
		t.Errorf("no tab hint:\n%s", v)
	}
}

func TestStepBoxOptionalTextCanBeSkipped(t *testing.T) {
	b := StepBox{Title: "Note", Steps: []Step{
		{Key: "title", Title: "Title", Help: "optional, enter to skip", Kind: StepText},
		{Key: "text", Title: "Text", Kind: StepMultiline},
	}}
	sm := stepKey(newStepModel(b), kEnter, stepType("body"), kEnter)
	if !sm.summary || sm.answers.Text("title") != "" || sm.answers.Text("text") != "body" {
		t.Errorf("answers = %+v", sm.answers)
	}
}

func TestStepBoxAnswersOfStepsNeverAskedAreMissing(t *testing.T) {
	a := Answers{}
	if a.Text("nope") != "" || a.Choice("nope") != -1 {
		t.Errorf("Text=%q Choice=%d", a.Text("nope"), a.Choice("nope"))
	}
}

func TestRowsAlignsWithoutCountingColors(t *testing.T) {
	forceColors(t)
	rows := Rows([2]string{"Folder", "a"}, [2]string{"Recording", "b"})
	if len(rows) != 2 {
		t.Fatal(rows)
	}
	a, b := ansi.Strip(rows[0]), ansi.Strip(rows[1])
	if strings.Index(a, "a") != strings.Index(b, "b") {
		t.Errorf("values not aligned:\n%s\n%s", a, b)
	}
}

func TestStepBoxConfirmedLinesReplaceTheSummaryAfterConfirm(t *testing.T) {
	b := StepBox{
		Title:     "Add",
		Steps:     []Step{{Key: "t", Title: "T", Kind: StepText, Default: "x"}},
		Summary:   func(a Answers) []string { return []string{"Saves to my.md"} },
		Confirmed: func(a Answers) []string { return []string{"Saved to my.md"} },
	}
	sm := stepKey(newStepModel(b), kEnter)
	if v := plainView(sm); !strings.Contains(v, "Saves to my.md") || strings.Contains(v, "Saved to") {
		t.Errorf("before the confirmation:\n%s", v)
	}
	sm = stepKey(sm, kEnter)
	if v := plainView(sm); !strings.Contains(v, "Saved to my.md") || strings.Contains(v, "Saves to") || !strings.Contains(v, "- Done") || strings.Contains(v, "done ─") {
		t.Errorf("after the confirmation:\n%s", v)
	}
	sm = stepKey(newStepModel(b), kEnter, kCtrlC)
	if strings.Contains(plainView(sm), "Saved to") {
		t.Errorf("cancelled box:\n%s", plainView(sm))
	}
}
