// Package cmdlog reads and writes commands.jsonl, the command log of a session.
// This file writes it, one JSON line per command start, end or insert.
package cmdlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/ynn3m/arsenal/internal/session"
	"os"
	"path/filepath"
	"unicode/utf8"

	"github.com/ynn3m/arsenal/internal/config"
)

const FileName = "commands.jsonl"

const Version = 1

const MaxCmdBytes = 3000

const (
	EvStart    = "start"
	EvEnd      = "end"
	EvInsert   = "insert"
	EvRecStart = "rec_start"
	EvRecEnd   = "rec_end"
)

const (
	ViaUser = "user"
	ViaArs  = "ars"
)

const (
	HowPrompt    = "prompt"
	HowShell     = "shell"
	HowClipboard = "clipboard"
	HowTmux      = "tmux"
	HowTmuxExec  = "tmux-exec"
	HowExec      = "exec"
	HowPrint     = "print"
	HowFile      = "file"
)

type Record struct {
	V       int    `json:"v"`
	Ev      string `json:"ev"`
	ID      string `json:"id,omitempty"`
	TS      int64  `json:"ts"`
	Session string `json:"session,omitempty"`
	CWD     string `json:"cwd,omitempty"`
	Cmd     string `json:"cmd,omitempty"`
	Sh      string `json:"sh,omitempty"`
	PID     int    `json:"pid,omitempty"`
	Rec     string `json:"rec,omitempty"`
	Via     string `json:"via,omitempty"`
	Exit    *int   `json:"exit,omitempty"`
	Dur     *int64 `json:"dur,omitempty"`
	How     string `json:"how,omitempty"`
	Cheat   string `json:"cheat,omitempty"`
	Trunc   int    `json:"trunc,omitempty"`
}

func Path(dir string) string { return filepath.Join(session.MetaDir(dir), FileName) }

func Ensure(dir string) (string, error) {
	if err := session.EnsureMetaDir(dir); err != nil {
		return "", err
	}
	p := Path(dir)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND|os.O_CREATE, config.FileMode)
	if err != nil {
		return "", fmt.Errorf("cannot create %s: %w", p, err)
	}
	defer f.Close()
	if err := f.Chmod(config.FileMode); err != nil {
		return "", fmt.Errorf("cannot set permissions on %s: %w", p, err)
	}
	return p, nil
}

func Marshal(r Record) ([]byte, error) {
	r.V = Version
	if r.Ev == "" {
		return nil, errors.New("cmdlog: record without ev")
	}
	if len(r.Cmd) > MaxCmdBytes {
		r.Cmd = cutUTF8(r.Cmd, MaxCmdBytes)
		r.Trunc = 1
	}
	return json.Marshal(r)
}

func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func Append(path string, r Record) error {
	line, err := Marshal(r)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, config.FileMode)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func AppendSession(name, dir string, r Record) error {
	if r.Session == "" {
		r.Session = name
	}
	return Append(Path(dir), r)
}
