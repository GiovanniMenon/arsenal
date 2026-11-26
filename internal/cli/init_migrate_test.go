// Tests for ars init on a session that is migrated in the same run.

package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitContinuesAfterMigratingThatSession(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "acme")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".arsenal"), []byte("name: acme\ncreated: 2026-11-01\nvars:\n    target: 10.10.10.5\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("## 2026-11-01 10:00 | first\n\nhello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	isInteractive = func() bool { return true }
	confirm = func(string, string) (bool, error) {
		t.Fatal("init asked whether to resume a session it has just migrated")
		return false, nil
	}

	out, code := run(t, "init", "acme")
	if code != 0 {
		t.Fatalf("code %d:\n%s", code, out)
	}
	if strings.Contains(out, "Nothing changed") {
		t.Errorf("init stopped after the migration:\n%s", out)
	}
	for _, want := range []string{"session acme:", "Resumed: acme", "10.10.10.5", "1 note"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if got, want := wd(t), realPath(t, dir); got != want {
		t.Errorf("did not cd into the session: %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, ".arsenal", "session.yml")); err != nil {
		t.Errorf("session not migrated: %v", err)
	}

	answer := false
	confirm = func(string, string) (bool, error) { answer = true; return false, nil }
	if out, _ := run(t, "init", "acme"); !answer || !strings.Contains(out, "Nothing changed") {
		t.Errorf("a plain resume must still ask:\n%s", out)
	}
}

func TestInitStillAsksWhenOnlyAnotherSessionWasMigrated(t *testing.T) {
	root := isolate(t)
	run(t, "init", "mine")
	old := filepath.Join(root, "old")
	if err := os.MkdirAll(old, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, ".arsenal"), []byte("name: old\ncreated: 2026-11-01\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	isInteractive = func() bool { return true }
	asked := false
	confirm = func(string, string) (bool, error) { asked = true; return true, nil }
	if out, code := run(t, "init", "mine"); code != 0 || !asked {
		t.Fatalf("code %d, asked=%v:\n%s", code, asked, out)
	}
}
