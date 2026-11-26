package tui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompletePathUnderTheHomeFolder(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, "Documents", "pentest"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got := CompletePath("~/Docu"); got != "~/Documents/" {
		t.Errorf("CompletePath(~/Docu) = %q", got)
	}
	if got := CompletePath("~/Documents/pen"); got != "~/Documents/pentest/" {
		t.Errorf("CompletePath = %q", got)
	}
	if got := CompletePath(""); got != "" {
		t.Errorf("empty = %q", got)
	}
}
