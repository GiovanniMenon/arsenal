// Package history reads the last command from the shell history, to prefill ars add.
package history

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var ErrNone = errors.New("no command found in your shell history: run one first (see Troubleshooting in the README)")

const tailBytes = 256 * 1024

func File() string {
	if h := os.Getenv("HISTFILE"); h != "" {
		return h
	}
	home, _ := os.UserHomeDir()
	zsh := filepath.Join(home, ".zsh_history")
	if z := os.Getenv("ZDOTDIR"); z != "" {
		zsh = filepath.Join(z, ".zsh_history")
	}
	bash := filepath.Join(home, ".bash_history")
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	fish := filepath.Join(data, "fish", "fish_history")

	var order []string
	switch filepath.Base(os.Getenv("SHELL")) {
	case "zsh":
		order = []string{zsh, bash, fish}
	case "fish":
		order = []string{fish, zsh, bash}
	default:
		order = []string{bash, zsh, fish}
	}
	for _, p := range order {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return order[0]
}

func Last(path string, skip func(cmd string) bool) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", ErrNone
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return "", ErrNone
	}
	partial := false
	if fi.Size() > tailBytes {
		if _, err := f.Seek(fi.Size()-tailBytes, io.SeekStart); err != nil {
			return "", ErrNone
		}
		partial = true
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return "", ErrNone
	}
	text := string(raw)
	if partial {
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			text = text[i+1:]
		}
	}
	cmds := Parse(text)
	for i := len(cmds) - 1; i >= 0; i-- {
		if c := strings.TrimSpace(cmds[i]); c != "" && (skip == nil || !skip(c)) {
			return c, nil
		}
	}
	return "", ErrNone
}

var (
	zshExtended = regexp.MustCompile(`^: \d+:\d+;`)
	bashStamp   = regexp.MustCompile(`^#\d+$`)
)

func Parse(text string) []string {
	var out []string
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")

	if strings.HasPrefix(strings.TrimSpace(text), "- cmd:") || strings.Contains(text, "\n- cmd: ") {
		for _, l := range lines {
			if c, ok := strings.CutPrefix(l, "- cmd: "); ok {
				out = append(out, strings.ReplaceAll(c, `\n`, "\n"))
			}
		}
		return out
	}

	var cont *string
	for _, l := range lines {
		if cont != nil {
			*cont += "\n" + l
			if !strings.HasSuffix(l, `\`) {
				out = append(out, *cont)
				cont = nil
			}
			continue
		}
		if bashStamp.MatchString(l) {
			continue
		}
		if loc := zshExtended.FindStringIndex(l); loc != nil {
			l = l[loc[1]:]
		}
		if strings.HasSuffix(l, `\`) {
			c := l
			cont = &c
			continue
		}
		if l != "" {
			out = append(out, l)
		}
	}
	if cont != nil {
		out = append(out, *cont)
	}
	return out
}

func IsArsenalCmd(cmd string) bool {
	f := strings.Fields(cmd)
	for len(f) > 0 && (f[0] == "command" || f[0] == "sudo" || f[0] == "exec") {
		f = f[1:]
	}
	if len(f) == 0 {
		return false
	}
	switch filepath.Base(f[0]) {
	case "ars", "arsenal":
		return true
	}
	return false
}
