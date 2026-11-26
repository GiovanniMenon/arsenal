package session

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ynn3m/arsenal/internal/config"
)

func isolate(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "pentest")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PENTEST_PATH", root)
	return root
}

func mk(t *testing.T, name string) Info {
	t.Helper()
	dir, err := Create(name)
	if err != nil {
		t.Fatal(err)
	}
	return Info{Name: name, Dir: dir}
}

func TestValidateName(t *testing.T) {
	good := []string{"acme", "a", "Acme-2026", "box.01", "a_b", strings.Repeat("a", 64)}
	bad := []string{"", ".", "..", ".hidden", "-flag", "_x", "a/b", "../x", "a b", "a\nb", "é", strings.Repeat("a", 65)}
	for _, n := range good {
		if err := ValidateName(n); err != nil {
			t.Errorf("ValidateName(%q) = %v, want nil", n, err)
		}
	}
	for _, n := range bad {
		if err := ValidateName(n); err == nil {
			t.Errorf("ValidateName(%q) = nil, want error", n)
		}
	}
}

func TestCreateMakesPentestPathAndSession0700(t *testing.T) {
	root := isolate(t)
	dir, err := Create("acme")
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(root, "acme") {
		t.Fatalf("dir = %q", dir)
	}
	for _, p := range []string{root, dir} {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o700 {
			t.Errorf("%s mode = %o, want 700", p, fi.Mode().Perm())
		}
	}
}

func TestCreateExistingIsNeverOverwritten(t *testing.T) {
	isolate(t)
	dir, err := Create("acme")
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "keep.txt")
	if err := os.WriteFile(marker, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("acme"); !errors.Is(err, ErrExists) {
		t.Fatalf("err = %v, want ErrExists", err)
	}
	if b, err := os.ReadFile(marker); err != nil || string(b) != "data" {
		t.Fatalf("existing session was modified: %q, %v", b, err)
	}
}

func TestCreateRejectsTraversal(t *testing.T) {
	root := isolate(t)
	if _, err := Create("../escape"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape")); err == nil {
		t.Fatal("folder created outside PENTEST_PATH")
	}
}

func TestCreateWritesTheMarker(t *testing.T) {
	isolate(t)
	s := mk(t, "acme")
	b, err := os.ReadFile(MarkerPath(s.Dir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "name: acme\n") || !strings.Contains(string(b), "created: "+time.Now().Format("2006-01-02")) {
		t.Errorf("marker:\n%s", b)
	}
	if fi, _ := os.Stat(MarkerPath(s.Dir)); fi.Mode().Perm() != 0o600 {
		t.Errorf("session.yml mode = %o, want 600", fi.Mode().Perm())
	}
	if fi, err := os.Stat(filepath.Join(s.Dir, ".arsenal")); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Errorf(".arsenal must be a 0700 directory: %v %v", fi, err)
	}
	if MarkerPath(s.Dir) != filepath.Join(s.Dir, ".arsenal", "session.yml") {
		t.Errorf("MarkerPath = %s", MarkerPath(s.Dir))
	}
	entries, _ := os.ReadDir(s.Dir)
	if len(entries) != 1 || entries[0].Name() != ".arsenal" {
		t.Errorf("session folder holds %v, want only .arsenal", entries)
	}
}

func TestExistsAndList(t *testing.T) {
	root := isolate(t)

	list, err := List()
	if err != nil || len(list) != 0 {
		t.Fatalf("missing root: %v, %v", list, err)
	}

	for _, n := range []string{"beta", "alpha"} {
		mk(t, n)
	}
	if err := os.WriteFile(filepath.Join(root, "file.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".hidden"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "plain"), 0o700); err != nil {
		t.Fatal(err)
	}

	list, err = List()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range list {
		names = append(names, s.Name)
	}
	if want := []string{"alpha", "beta"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("List = %v, want %v", names, want)
	}

	if ok, _ := Exists("alpha"); !ok {
		t.Error("alpha should exist")
	}
	for _, n := range []string{"nope", "file.txt", "plain"} {
		if ok, _ := Exists(n); ok {
			t.Errorf("%s is not a session", n)
		}
	}
	if exists, isSession, _ := FolderExists("plain"); !exists || isSession {
		t.Errorf("plain: exists %v isSession %v", exists, isSession)
	}
}

func TestFindWalksUpAndTheNearestMarkerWins(t *testing.T) {
	isolate(t)
	outer := mk(t, "outer")
	deep := filepath.Join(outer.Dir, "scans", "nmap")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, start := range []string{outer.Dir, filepath.Join(outer.Dir, "scans"), deep} {
		in, ok, err := Find(start)
		if err != nil || !ok || in != outer {
			t.Errorf("Find(%s) = %+v %v %v", start, in, ok, err)
		}
	}

	inner := filepath.Join(outer.Dir, "sub")
	if err := os.MkdirAll(inner, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(MetaDir(inner), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(MarkerPath(inner), []byte("name: inner\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if in, ok, _ := Find(inner); !ok || in.Name != "inner" || in.Dir != inner {
		t.Errorf("nested: %+v", in)
	}

	if _, ok, err := Find(t.TempDir()); ok || err != nil {
		t.Errorf("outside: ok %v err %v", ok, err)
	}
	bare := filepath.Join(t.TempDir(), "box01")
	os.MkdirAll(MetaDir(bare), 0o700)
	os.WriteFile(MarkerPath(bare), nil, 0o600)
	if in, ok, _ := Find(bare); !ok || in.Name != "box01" {
		t.Errorf("empty marker: %+v %v", in, ok)
	}
	os.WriteFile(MarkerPath(bare), []byte("name: [x\n"), 0o600)
	if _, ok, err := Find(bare); ok || err == nil {
		t.Errorf("broken marker: ok %v err %v", ok, err)
	}
}

func TestCurrentAndRequireUseTheWorkingDirectory(t *testing.T) {
	isolate(t)
	s := mk(t, "acme")
	sub := filepath.Join(s.Dir, "loot")
	os.MkdirAll(sub, 0o700)
	t.Chdir(sub)
	in, err := Require()
	if err != nil || in.Name != "acme" {
		t.Fatalf("Require in a subfolder: %+v %v", in, err)
	}
	t.Chdir(t.TempDir())
	if _, err := Require(); !errors.Is(err, ErrNoSession) {
		t.Fatalf("Require outside: %v", err)
	}
	if !strings.Contains(ErrNoSession.Error(), "not in a session folder: run ars init <name> or cd into one") {
		t.Errorf("message: %v", ErrNoSession)
	}
}

func TestAdoptOnlyAddsTheMarker(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "old")
	os.MkdirAll(dir, 0o700)
	os.WriteFile(filepath.Join(dir, "keep.txt"), []byte("data"), 0o600)
	if _, err := Adopt("old"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "keep.txt")); string(b) != "data" {
		t.Error("existing file touched")
	}
	if _, err := ByName("old"); err != nil {
		t.Errorf("not a session after Adopt: %v", err)
	}
}

func TestAFileNamedDotArsenalIsNotASession(t *testing.T) {
	isolate(t)
	d := t.TempDir()
	os.WriteFile(filepath.Join(d, ".arsenal"), []byte("name: x\n"), 0o600)
	if _, ok, err := Find(d); ok || err != nil {
		t.Errorf("an old marker file must not count until it is migrated: ok %v err %v", ok, err)
	}
}

func TestMigrateOldActiveSessionModel(t *testing.T) {
	root := isolate(t)
	cfg, _ := config.ConfigDir()
	os.MkdirAll(cfg, 0o700)
	os.WriteFile(filepath.Join(cfg, "current"), []byte("acme\n"), 0o600)
	os.MkdirAll(filepath.Join(root, "acme", "recordings"), 0o700)
	os.WriteFile(filepath.Join(root, "acme", "recordings", "r1.cast"), []byte("{}\n"), 0o600)
	os.WriteFile(filepath.Join(root, "acme", "vars"), []byte("# c\ntarget=10.10.10.5\nports=22,80\n"), 0o600)
	os.WriteFile(filepath.Join(root, "acme", "notes.md"), []byte("# Notes - acme\n"), 0o600)
	os.WriteFile(filepath.Join(root, "acme", "commands.jsonl"), []byte("{}\n"), 0o600)
	os.WriteFile(filepath.Join(root, "acme", "scan.txt"), []byte("user data"), 0o644)
	os.MkdirAll(filepath.Join(root, "empty"), 0o700)
	os.MkdirAll(filepath.Join(root, "broken"), 0o700)
	os.WriteFile(filepath.Join(root, "broken", "vars"), []byte("garbage line\n"), 0o600)
	os.MkdirAll(filepath.Join(root, ".hidden"), 0o700)
	os.MkdirAll(filepath.Join(root, "has space"), 0o700)

	lines, err := Migrate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	for _, want := range []string{"session acme: created .arsenal/session.yml, variables moved into it", "session acme: moved notes.md, commands.jsonl, recordings/ into .arsenal/", "session empty:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the output lacks %q:\n%s", want, joined)
		}
	}
	acme, err := ByName("acme")
	if err != nil {
		t.Fatal(err)
	}
	if v, _ := LoadVars(acme.Dir); !reflect.DeepEqual(v, Vars{"target": "10.10.10.5", "ports": "22,80"}) {
		t.Errorf("vars = %v", v)
	}
	for _, f := range []string{"notes.md", "commands.jsonl", "recordings/r1.cast", "session.yml"} {
		if _, err := os.Stat(filepath.Join(acme.Dir, ".arsenal", f)); err != nil {
			t.Errorf(".arsenal/%s missing: %v", f, err)
		}
	}
	for _, f := range []string{"notes.md", "commands.jsonl", "recordings", "vars"} {
		if _, err := os.Lstat(filepath.Join(acme.Dir, f)); err == nil {
			t.Errorf("%s was left in the session folder", f)
		}
	}
	if b, _ := os.ReadFile(filepath.Join(acme.Dir, "scan.txt")); string(b) != "user data" {
		t.Error("the user's own file was touched")
	}
	if fi, _ := os.Stat(filepath.Join(acme.Dir, ".arsenal", "recordings")); fi.Mode().Perm() != 0o700 {
		t.Errorf("recordings mode %o", fi.Mode().Perm())
	}
	if ok, _ := Exists("empty"); !ok {
		t.Error("an empty session folder is a session too")
	}
	if b, _ := os.ReadFile(filepath.Join(root, "broken", "vars")); string(b) != "garbage line\n" {
		t.Error("an unreadable vars file must stay")
	}
	if _, err := os.Stat(filepath.Join(cfg, "current")); err != nil {
		t.Error("current must stay when something could not be moved")
	}
	for _, n := range []string{".hidden", "has space"} {
		if _, err := os.Stat(filepath.Join(root, n, ".arsenal")); err == nil {
			t.Errorf("%s was made a session", n)
		}
	}

	os.Remove(filepath.Join(root, "broken", "vars"))
	if _, err := Migrate(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "current")); err == nil {
		t.Error("current not removed after a clean migration")
	}
	if lines, _ := Migrate(t.TempDir()); len(lines) != 0 {
		t.Errorf("second run: %v", lines)
	}
}

func TestMigrateOldMarkerFileToADirectory(t *testing.T) {
	root := isolate(t)
	dir := filepath.Join(root, "acme")
	os.MkdirAll(filepath.Join(dir, "recordings"), 0o700)
	os.WriteFile(filepath.Join(dir, ".arsenal"), []byte("name: acme\ncreated: 2026-11-01\nvars:\n    target: 10.10.10.5\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("# Notes - acme\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "commands.jsonl"), []byte("{}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "recordings", "r1.cast"), []byte("{}\n"), 0o600)
	os.MkdirAll(filepath.Join(dir, "scans"), 0o700)

	lines, err := Migrate(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "session acme: moved the old .arsenal file to .arsenal/session.yml") || !strings.Contains(joined, "moved notes.md, commands.jsonl, recordings/ into .arsenal/") {
		t.Errorf("the user was not told what moved:\n%s", joined)
	}
	if fi, err := os.Stat(filepath.Join(dir, ".arsenal")); err != nil || !fi.IsDir() || fi.Mode().Perm() != 0o700 {
		t.Fatalf(".arsenal: %v %v", fi, err)
	}
	in, err := ByName("acme")
	if err != nil || in.Name != "acme" {
		t.Fatalf("not a session after migration: %+v %v", in, err)
	}
	if v, _ := LoadVars(dir); v["target"] != "10.10.10.5" {
		t.Errorf("vars lost: %v", v)
	}
	for _, f := range []string{"notes.md", "commands.jsonl"} {
		if fi, err := os.Stat(filepath.Join(dir, ".arsenal", f)); err != nil || fi.Mode().Perm() != 0o600 {
			t.Errorf(".arsenal/%s: %v %v (want mode 600)", f, fi, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "scans")); err != nil {
		t.Error("the user's folder was touched")
	}
	if lines, _ := Migrate(t.TempDir()); len(lines) != 0 {
		t.Errorf("second run: %v", lines)
	}
}

func TestMigrateNeverOverwritesAndFindsASessionOutsidePentestPath(t *testing.T) {
	isolate(t)
	dir := filepath.Join(t.TempDir(), "box01")
	sub := filepath.Join(dir, "loot")
	os.MkdirAll(sub, 0o700)
	os.WriteFile(filepath.Join(dir, ".arsenal"), []byte("name: box01\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("old notes"), 0o600)
	lines, err := Migrate(sub)
	if err != nil || len(lines) == 0 {
		t.Fatalf("outside PENTEST_PATH: %v %v", lines, err)
	}
	if in, ok, _ := Find(sub); !ok || in.Name != "box01" {
		t.Errorf("not a session after migration: %+v %v", in, ok)
	}

}

func TestMoveManagedNeverOverwrites(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "box02")
	os.MkdirAll(MetaDir(dir), 0o700)
	os.WriteFile(filepath.Join(MetaDir(dir), "notes.md"), []byte("new notes"), 0o600)
	os.WriteFile(filepath.Join(dir, "notes.md"), []byte("stray notes"), 0o600)
	os.WriteFile(filepath.Join(dir, "commands.jsonl"), []byte("log"), 0o600)
	lines := moveManaged(dir, "box02")
	if b, _ := os.ReadFile(filepath.Join(MetaDir(dir), "notes.md")); string(b) != "new notes" {
		t.Errorf("destination overwritten: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "notes.md")); string(b) != "stray notes" {
		t.Errorf("source removed: %q", b)
	}
	if _, err := os.Stat(filepath.Join(MetaDir(dir), "commands.jsonl")); err != nil {
		t.Errorf("the free one was not moved: %v", err)
	}
	if joined := strings.Join(lines, "\n"); !strings.Contains(joined, "moved commands.jsonl") || !strings.Contains(joined, "notes.md left in place") {
		t.Errorf("lines:\n%s", joined)
	}
}

func TestMigratedNames(t *testing.T) {
	got := MigratedNames([]string{
		"session acme: moved the old .arsenal file to .arsenal/session.yml",
		"session other: moved notes.md into .arsenal/",
		"something else",
		"session : broken",
	})
	if len(got) != 2 || !got["acme"] || !got["other"] {
		t.Errorf("MigratedNames = %v", got)
	}
}
