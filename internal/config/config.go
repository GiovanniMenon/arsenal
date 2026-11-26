// Package config finds the Arsenal folders and reads config.yml.
// This file resolves the config folder and PENTEST_PATH.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	EnvPentestPath    = "PENTEST_PATH"
	DefaultPentestDir = "Documents/pentest"

	appDirName = "arsenal"
)

const DirMode os.FileMode = 0o700

const FileMode os.FileMode = 0o600

func ConfigDir() (string, error) {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, appDirName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find your home folder: %w", err)
	}
	return filepath.Join(home, ".config", appDirName), nil
}

func PentestPath() (string, error) {
	if p := os.Getenv(EnvPentestPath); p != "" {
		return expandHome(p)
	}
	if f, err := LoadFile(); err == nil && f.PentestPath != "" {
		return expandHome(f.PentestPath)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot find your home folder: %w", err)
	}
	return filepath.Join(home, filepath.FromSlash(DefaultPentestDir)), nil
}

func ExpandHome(p string) (string, error) { return expandHome(p) }

func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return filepath.Clean(p), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot expand %q: %w", p, err)
	}
	return filepath.Join(home, strings.TrimPrefix(p, "~")), nil
}

func EnsurePentestPath() (string, error) {
	p, err := PentestPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(p, DirMode); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", p, err)
	}
	return p, nil
}

func GlobalVarsPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "vars"), nil
}
