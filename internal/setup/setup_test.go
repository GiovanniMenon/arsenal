package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/ynn3m/arsenal/internal/cheats"
)

func TestDetectShellAndRCFile(t *testing.T) {
	for in, want := range map[string]string{"/bin/zsh": "zsh", "/usr/local/bin/bash": "bash", "/usr/bin/fish": "", "": ""} {
		if got := DetectShell(in); got != want {
			t.Errorf("DetectShell(%q) = %q, want %q", in, got, want)
		}
	}
	if got := RCFile("zsh", "/h", ""); got != "/h/.zshrc" {
		t.Errorf("zsh rc = %s", got)
	}
	if got := RCFile("zsh", "/h", "/z"); got != "/z/.zshrc" {
		t.Errorf("ZDOTDIR rc = %s", got)
	}
	if got := RCFile("bash", "/h", "/z"); got != "/h/.bashrc" {
		t.Errorf("bash rc = %s", got)
	}
	if got := HookLine("zsh"); got != `command -v ars >/dev/null 2>&1 && eval "$(ars hook zsh)"` {
		t.Errorf("HookLine = %s", got)
	}
}

func TestAnalyzeRC(t *testing.T) {
	cur := HookLine("zsh")
	cases := []struct {
		name, in     string
		current, old int
	}{
		{"empty", "", 0, 0},
		{"current", "export A=1\n" + cur + "\n", 1, 0},
		{"old init", `eval "$(command ars init zsh)"`, 0, 1},
		{"old unguarded hook", `eval "$(ars hook zsh)"`, 0, 1},
		{"old arsenal spelling", "  eval \"$(arsenal init zsh)\"\n", 0, 1},
		{"current and old", cur + "\n" + `eval "$(command ars init zsh)"`, 1, 1},
		{"comments and look-alikes", "# " + `eval "$(command ars init zsh)"` + "\necho \"ars initial\"\nalias x=y\n", 0, 0},
	}
	for _, c := range cases {
		st := AnalyzeRC(c.in, "zsh")
		if st.Current != (c.current == 1) || len(st.Old) != c.old {
			t.Errorf("%s: %+v", c.name, st)
		}
		if want := c.current == 0 || c.old > 0; st.Needs() != want {
			t.Errorf("%s: Needs = %v", c.name, st.Needs())
		}
	}
}

func TestPlanRCAppendsOnceAndKeepsContent(t *testing.T) {
	cur := HookLine("zsh")
	got := PlanRC("export A=1", "zsh", "", "/h")
	if !strings.HasPrefix(got, "export A=1\n\n# Arsenal") || !strings.HasSuffix(got, cur+"\n") {
		t.Errorf("append:\n%q", got)
	}
	if again := PlanRC(got, "zsh", "", "/h"); again != got {
		t.Errorf("not idempotent:\n%q\n%q", got, again)
	}
	if strings.Count(got, "ars hook") != 1 {
		t.Errorf("hook line count in %q", got)
	}
	if got := PlanRC("", "bash", "", "/h"); got != "# Arsenal shell integration (added by `ars setup`)\n"+HookLine("bash")+"\n" {
		t.Errorf("empty file: %q", got)
	}
}

func TestPlanRCReplacesOldLinesInPlaceWithoutDuplicates(t *testing.T) {
	in := "export A=1\n" +
		"# Arsenal shell integration (added by `ars setup`)\n" +
		"eval \"$(command ars init zsh)\"\n" +
		"alias ll='ls -l'\n" +
		"eval \"$(ars hook zsh)\"\n" +
		"export B=2\n"
	got := PlanRC(in, "zsh", "", "/h")
	want := "export A=1\n" +
		"# Arsenal shell integration (added by `ars setup`)\n" +
		HookLine("zsh") + "\n" +
		"alias ll='ls -l'\n" +
		"export B=2\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "init zsh") {
		t.Error("an old line is left")
	}
}

func TestPlanRCPathExportComesBeforeTheLine(t *testing.T) {
	home := "/h"
	dir := "/h/.local/bin"
	got := PlanRC("export A=1\n", "zsh", dir, home)
	exp := PathExport(dir, home)
	if exp != `export PATH="$HOME/.local/bin:$PATH"` {
		t.Fatalf("PathExport = %s", exp)
	}
	i, j := strings.Index(got, exp), strings.Index(got, HookLine("zsh"))
	if i < 0 || j < 0 || i > j {
		t.Errorf("PATH line must be before the ars line:\n%s", got)
	}
	if again := PlanRC(got, "zsh", dir, home); again != got {
		t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
	}

	got = PlanRC(`eval "$(command ars init zsh)"`+"\n", "zsh", dir, home)
	if i, j := strings.Index(got, exp), strings.Index(got, HookLine("zsh")); i < 0 || i > j {
		t.Errorf("replace: PATH line must be before the ars line:\n%s", got)
	}

	mine := "export PATH=\"$HOME/.local/bin:$PATH\"\n"
	if got := PlanRC(mine, "zsh", dir, home); strings.Count(got, ".local/bin") != 1 {
		t.Errorf("duplicated PATH line:\n%s", got)
	}
	late := HookLine("zsh") + "\n" + mine
	if got := PlanRC(late, "zsh", dir, home); strings.Index(got, ".local/bin") > strings.Index(got, "ars hook") {
		t.Errorf("PATH line after the ars line:\n%s", got)
	}
}

func TestInstallRCBackupModeAndSymlink(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	old := "export A=1\neval \"$(command ars init zsh)\"\n"
	if err := os.WriteFile(rc, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := InstallRC(rc, "zsh", "", dir)
	if err != nil || !changed {
		t.Fatalf("%v %v", changed, err)
	}
	if b, _ := os.ReadFile(rc + ".arsenal-bak"); string(b) != old {
		t.Errorf("backup = %q", b)
	}
	if fi, _ := os.Stat(rc); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode changed to %o", fi.Mode().Perm())
	}
	if changed, err := InstallRC(rc, "zsh", "", dir); err != nil || changed {
		t.Errorf("second run: %v %v", changed, err)
	}

	real := filepath.Join(dir, "dotfiles-zshrc")
	if err := os.WriteFile(real, []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, ".link-zshrc")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("no symlinks")
	}
	if _, err := InstallRC(link, "zsh", "", dir); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Lstat(link); fi.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a regular file")
	}
	if b, _ := os.ReadFile(real); !strings.Contains(string(b), "ars hook zsh") {
		t.Error("line not written through the symlink")
	}
	if _, err := os.Stat(link + ".arsenal-bak"); err == nil {
		t.Error("a backup was made for a pure append")
	}
}

func TestFindInstall(t *testing.T) {
	home := t.TempDir()
	found := func(string) (string, error) { return "/usr/local/bin/ars", nil }
	missing := func(string) (string, error) { return "", os.ErrNotExist }

	if in, ok := FindInstall(found, home); !ok || !in.InPath || in.Dir != "/usr/local/bin" {
		t.Errorf("in PATH: %+v %v", in, ok)
	}
	if _, ok := FindInstall(missing, home); ok {
		t.Error("nothing installed must not be found (./bin/ars is not an install)")
	}
	bin := filepath.Join(home, ".local", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "ars"), []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if in, ok := FindInstall(missing, home); !ok || in.InPath || in.Dir != bin {
		t.Errorf("installed in ~/.local/bin but not in PATH: %+v %v", in, ok)
	}
}

const sampleCheat = "# Nmap\n\n## scan\n% nmap\n\n```\nnmap -sV <target>\n```\n"

func TestAddOfficialAddsTheCommandsTaggedAndLeavesNoTempFolder(t *testing.T) {
	pack := fstest.MapFS{
		"Scan/nmap.md": {Data: []byte(sampleCheat)},
		"README.md":    {Data: []byte("# readme\n")},
	}
	dir := filepath.Join(t.TempDir(), "cheats")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	res, err := AddOfficial(dir, pack)
	if err != nil || res.Added != 1 || res.Files != 1 {
		t.Fatalf("%+v, %v", res, err)
	}
	list := cheats.Load([]string{dir}).Cheats
	if len(list) != 1 || !list[0].FromArsenal() {
		t.Fatalf("%+v", list)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("%v", left)
	}
	if again, err := AddOfficial(dir, pack); err != nil || again.Added != 0 {
		t.Errorf("%+v, %v", again, err)
	}
}

func TestAddOfficialFailsWhenThePackIsEmpty(t *testing.T) {
	pack := fstest.MapFS{"README.md": {Data: []byte("# readme\n")}}
	if _, err := AddOfficial(filepath.Join(t.TempDir(), "cheats"), pack); err == nil {
		t.Error("no error")
	}
}
