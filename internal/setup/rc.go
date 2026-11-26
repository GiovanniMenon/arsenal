// Package setup has the parts of ars setup that touch files.
// This file reads and updates the shell rc file.
package setup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func DetectShell(shellEnv string) string {
	switch filepath.Base(shellEnv) {
	case "zsh":
		return "zsh"
	case "bash":
		return "bash"
	}
	return ""
}

func RCFile(shell, home, zdotdir string) string {
	if shell == "zsh" {
		if zdotdir != "" {
			return filepath.Join(zdotdir, ".zshrc")
		}
		return filepath.Join(home, ".zshrc")
	}
	return filepath.Join(home, ".bashrc")
}

func HookLine(shell string) string {
	return fmt.Sprintf(`command -v ars >/dev/null 2>&1 && eval "$(ars hook %s)"`, shell)
}

const rcComment = "# Arsenal shell integration (added by `ars setup`)"

var hookRe = regexp.MustCompile(`\b(ars|arsenal) (init|hook) (zsh|bash)\b`)

func isComment(l string) bool { return strings.HasPrefix(strings.TrimSpace(l), "#") }

func isHookLine(l string) bool { return !isComment(l) && hookRe.MatchString(l) }

func isCurrentLine(l, shell string) bool { return strings.TrimSpace(l) == HookLine(shell) }

type RCState struct {
	Current bool
	Old     []string
}

func (s RCState) Needs() bool { return !s.Current || len(s.Old) > 0 }

func AnalyzeRC(content, shell string) RCState {
	var st RCState
	for _, l := range strings.Split(content, "\n") {
		switch {
		case isCurrentLine(l, shell):
			st.Current = true
		case isHookLine(l):
			st.Old = append(st.Old, strings.TrimSpace(l))
		}
	}
	return st
}

func ReadRC(path string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

func PathExport(dir, home string) string {
	if home != "" && (dir == home || strings.HasPrefix(dir, home+string(filepath.Separator))) {
		dir = "$HOME" + strings.TrimPrefix(dir, home)
	}
	return fmt.Sprintf(`export PATH="%s:$PATH"`, dir)
}

func mentionsDir(l, dir, home string) bool {
	if isComment(l) || !strings.Contains(l, "PATH") {
		return false
	}
	for _, d := range []string{dir, strings.Replace(dir, home, "$HOME", 1), strings.Replace(dir, home, "~", 1)} {
		if d != "" && strings.Contains(l, d) {
			return true
		}
	}
	return false
}

func PlanRC(content, shell, pathDir, home string) string {
	hook := HookLine(shell)
	lines := strings.Split(content, "\n")
	var out []string
	placed := false
	for _, l := range lines {
		switch {
		case isCurrentLine(l, shell), isHookLine(l):
			if placed {
				continue
			}
			placed = true
			out = append(out, hook)
		default:
			out = append(out, l)
		}
	}
	if !placed {
		for len(out) > 0 && out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, rcComment, hook, "")
	}
	if pathDir != "" {
		idx := -1
		for i, l := range out {
			if l == hook {
				idx = i
				break
			}
		}
		have := false
		for _, l := range out[:idx] {
			if mentionsDir(l, pathDir, home) {
				have = true
				break
			}
		}
		if !have {
			out = append(out[:idx], append([]string{PathExport(pathDir, home)}, out[idx:]...)...)
		}
	}
	return strings.Join(out, "\n")
}

func InstallRC(path, shell, pathDir, home string) (changed bool, err error) {
	old, err := ReadRC(path)
	if err != nil {
		return false, err
	}
	want := PlanRC(old, shell, pathDir, home)
	if want == old {
		return false, nil
	}
	if AnalyzeRC(old, shell).Old != nil {
		if err := os.WriteFile(path+".arsenal-bak", []byte(old), 0o600); err != nil {
			return false, fmt.Errorf("cannot back up %s: %w (nothing was changed)", path, err)
		}
	}
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return false, err
	}
	_, werr := f.WriteString(want)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	return werr == nil, werr
}

type Install struct {
	Path   string
	Dir    string
	InPath bool
}

func FindInstall(lookPath func(string) (string, error), home string) (in Install, ok bool) {
	if p, err := lookPath("ars"); err == nil {
		return Install{Path: p, Dir: filepath.Dir(p), InPath: true}, true
	}
	p := filepath.Join(home, ".local", "bin", "ars")
	if fi, err := os.Stat(p); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
		return Install{Path: p, Dir: filepath.Dir(p)}, true
	}
	return Install{}, false
}
