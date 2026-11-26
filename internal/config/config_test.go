package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func isolate(t *testing.T) (home, pentest string) {
	t.Helper()
	home = t.TempDir()
	pentest = filepath.Join(t.TempDir(), "pentest")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(EnvPentestPath, pentest)
	return home, pentest
}

func TestConfigDirDefaultAndXDG(t *testing.T) {
	home, _ := isolate(t)
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "arsenal"); got != want {
		t.Fatalf("ConfigDir = %q, want %q", got, want)
	}

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, _ = ConfigDir()
	if want := filepath.Join(xdg, "arsenal"); got != want {
		t.Fatalf("ConfigDir with XDG = %q, want %q", got, want)
	}
}

func TestPentestPath(t *testing.T) {
	home, pentest := isolate(t)

	got, _ := PentestPath()
	if got != pentest {
		t.Fatalf("PentestPath = %q, want %q", got, pentest)
	}

	t.Setenv(EnvPentestPath, "~/work/pt")
	got, _ = PentestPath()
	if want := filepath.Join(home, "work", "pt"); got != want {
		t.Fatalf("tilde expansion = %q, want %q", got, want)
	}

	t.Setenv(EnvPentestPath, "")
	got, _ = PentestPath()
	if want := filepath.Join(home, "Documents", "pentest"); got != want {
		t.Fatalf("default = %q, want %q", got, want)
	}
}

func TestEnsurePentestPathCreatesWithMode0700(t *testing.T) {
	_, pentest := isolate(t)
	got, err := EnsurePentestPath()
	if err != nil {
		t.Fatal(err)
	}
	if got != pentest {
		t.Fatalf("got %q want %q", got, pentest)
	}
	fi, err := os.Stat(pentest)
	if err != nil || !fi.IsDir() {
		t.Fatalf("pentest path not created: %v", err)
	}
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o, want 700", fi.Mode().Perm())
	}
	if _, err := EnsurePentestPath(); err != nil {
		t.Fatal(err)
	}
}

func TestCheatsPathsDefaultAndConfigFile(t *testing.T) {
	home, _ := isolate(t)
	own, _ := CheatsDir()
	if want := filepath.Join(home, ".config", "arsenal", "cheats"); own != want {
		t.Fatalf("CheatsDir = %q, want %q", own, want)
	}

	got, err := CheatsPaths()
	if err != nil || len(got) != 1 || got[0] != own {
		t.Fatalf("default = %v, %v", got, err)
	}

	legacy := filepath.Join(home, ".cheats")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	got, _ = CheatsPaths()
	if len(got) != 2 || got[0] != own || got[1] != legacy {
		t.Fatalf("with ~/.cheats = %v", got)
	}

	dir, _ := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	cfg := "cheats_paths:\n  - /srv/my-cheats\n  - ~/.cheats\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = CheatsPaths()
	want := []string{"/srv/my-cheats", legacy, own}
	if err != nil || len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("configured = %v, %v (want %v)", got, err, want)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("cheats_paths: [unclosed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CheatsPaths(); err == nil {
		t.Fatal("expected an error for invalid yaml")
	}
}

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOtherConfigNoticeNeverMovesAnything(t *testing.T) {
	home, _ := isolate(t)
	old := filepath.Join(home, ".config", "arsenal")
	write(t, filepath.Join(old, "config.yml"), "pentest_path: ~/pt\n")
	write(t, filepath.Join(old, "vars"), "target=10.10.10.5\n")
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)

	got := OtherConfigNotice()
	for _, want := range []string{old, xdg, filepath.Join(xdg, "arsenal")} {
		if !strings.Contains(got, want) {
			t.Errorf("%q lacks %q", got, want)
		}
	}
	if strings.Contains(got, "\n") {
		t.Errorf("%q", got)
	}
	for _, name := range []string{"config.yml", "vars"} {
		if _, err := os.Stat(filepath.Join(old, name)); err != nil {
			t.Errorf("%v", err)
		}
		if _, err := os.Stat(filepath.Join(xdg, "arsenal", name)); err == nil {
			t.Errorf("%s was moved", name)
		}
	}
}

func TestOtherConfigNoticeIsSilentWhenThereIsNothingToSay(t *testing.T) {
	home, _ := isolate(t)
	t.Setenv("XDG_CONFIG_HOME", "")
	write(t, filepath.Join(home, ".config", "arsenal", "config.yml"), "x: 1\n")
	if got := OtherConfigNotice(); got != "" {
		t.Errorf("%q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	if got := OtherConfigNotice(); got != "" {
		t.Errorf("%q", got)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := os.Remove(filepath.Join(home, ".config", "arsenal", "config.yml")); err != nil {
		t.Fatal(err)
	}
	if got := OtherConfigNotice(); got != "" {
		t.Errorf("%q", got)
	}
}

func TestFileExists(t *testing.T) {
	isolate(t)
	if ok, err := FileExists(); err != nil || ok {
		t.Fatalf("FileExists = %v, %v", ok, err)
	}
	if err := SetFileKeys(map[string]any{"rec_by_default": true}); err != nil {
		t.Fatal(err)
	}
	if ok, err := FileExists(); err != nil || !ok {
		t.Fatalf("FileExists = %v, %v", ok, err)
	}
}

func TestRecordingOptionsDefaultsAndConfigFile(t *testing.T) {
	isolate(t)
	if on, err := RecByDefault(); err != nil || on {
		t.Errorf("rec_by_default default = %v, %v; want false", on, err)
	}
	dir, _ := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("auto_attach: false\nrec_by_default: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if on, err := RecByDefault(); err != nil || !on {
		t.Errorf("rec_by_default = %v, %v; want true", on, err)
	}
}

func TestPentestPathFromConfigFile(t *testing.T) {
	home, _ := isolate(t)
	t.Setenv(EnvPentestPath, "")
	if err := SetFileKeys(map[string]any{"pentest_path": "~/ops/pt"}); err != nil {
		t.Fatal(err)
	}
	got, _ := PentestPath()
	if want := filepath.Join(home, "ops", "pt"); got != want {
		t.Fatalf("from config.yml: %q, want %q", got, want)
	}
	t.Setenv(EnvPentestPath, "/srv/env-pt")
	if got, _ := PentestPath(); got != "/srv/env-pt" {
		t.Fatalf("env must win: %q", got)
	}
}

func TestSetFileKeysKeepsCommentsAndOtherKeys(t *testing.T) {
	isolate(t)
	p, _ := FilePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	orig := "# my settings\ncheats_paths:\n  - ~/.cheats\n  - ~/work/cheats\nmax_cast_mb: 64 # keep it\nrec_by_default: false\n"
	if err := os.WriteFile(p, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SetFileKeys(map[string]any{"rec_by_default": true, "pentest_path": "~/pt"}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	text := string(b)
	for _, want := range []string{"# my settings", "# keep it", "~/work/cheats", "max_cast_mb: 64", "rec_by_default: true", "pentest_path: ~/pt"} {
		if !strings.Contains(text, want) {
			t.Errorf("config.yml lacks %q:\n%s", want, text)
		}
	}
	f, err := LoadFile()
	if err != nil || !f.RecByDefault || f.PentestPath != "~/pt" || len(f.CheatsPaths) != 2 || f.MaxCastMB != 64 {
		t.Fatalf("LoadFile = %+v, %v", f, err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode = %o, want 600", fi.Mode().Perm())
	}
}

func TestSetFileKeysCreatesMissingFileAndRejectsBadYAML(t *testing.T) {
	isolate(t)
	if err := SetFileKeys(map[string]any{"rec_by_default": false}); err != nil {
		t.Fatal(err)
	}
	if f, err := LoadFile(); err != nil || f.RecByDefault {
		t.Fatalf("LoadFile = %+v, %v", f, err)
	}
	p, _ := FilePath()
	if err := os.WriteFile(p, []byte("a: [unclosed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetFileKeys(map[string]any{"x": 1}); err == nil {
		t.Error("invalid config.yml was overwritten")
	}
	if b, _ := os.ReadFile(p); string(b) != "a: [unclosed\n" {
		t.Errorf("invalid file changed: %q", b)
	}
}
