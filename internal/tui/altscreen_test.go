// Checks which screens use the alternate screen.

package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOnlySearchNotesAndHistoryUseTheAltScreen(t *testing.T) {
	allowed := map[string]bool{"search.go": true, "notes.go": true, "history.go": true}
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "tea.WithAltScreen()") && !allowed[f] {
			t.Errorf("%s uses the alt screen: only search, notes and history are full screen", f)
		}
	}
	for f := range allowed {
		b, _ := os.ReadFile(f)
		if !strings.Contains(string(b), "tea.WithAltScreen()") {
			t.Errorf("%s no longer uses the alt screen", f)
		}
	}
	b, _ := os.ReadFile("stepbox.go")
	if strings.Contains(string(b), "AltScreen") {
		t.Error("the step box must be inline")
	}
}
