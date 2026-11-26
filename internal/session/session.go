// Package session manages session folders, the ones with a .arsenal directory inside.
// This file creates them and finds the session of the current folder.
package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/fsutil"
)

const MetaDirName = ".arsenal"

const MarkerFile = "session.yml"

func MetaDir(dir string) string { return filepath.Join(dir, MetaDirName) }

func RecordingsDir(dir string) string { return filepath.Join(MetaDir(dir), "recordings") }

var ErrExists = errors.New("session already exists")

var ErrNotFound = errors.New("session not found")

var ErrNoSession = errors.New("not in a session folder: run ars init <name> or cd into one")

var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid session name %q: use letters, digits, . _ and - (up to 64, starting with a letter or digit)", name)
	}
	return nil
}

type Info struct {
	Name string
	Dir  string
}

type Marker struct {
	Name    string            `yaml:"name"`
	Created Date              `yaml:"created"`
	Vars    map[string]string `yaml:"vars,omitempty"`

	Extra map[string]any `yaml:",inline"`
}

type Date string

func (d Date) MarshalYAML() (any, error) {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!timestamp", Value: string(d)}, nil
}

const markerHeader = "# Arsenal session (the folder that holds this .arsenal/ directory is the session, like .git).\n" +
	"# vars: are the session variables used to fill commands; `ars set` edits them.\n"

func MarkerPath(dir string) string { return filepath.Join(MetaDir(dir), MarkerFile) }

func readMarker(dir string) (Marker, error) {
	var m Marker
	if !isMarker(dir) {
		return m, fmt.Errorf("%w: %s has no %s directory", ErrNotFound, dir, MetaDirName)
	}
	p := MarkerPath(dir)
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return m, err
	}
	if err := yaml.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("cannot read %s: %w (fix or delete the file)", p, err)
	}
	return m, nil
}

func writeMarker(dir string, m Marker) error {
	b, err := yaml.Marshal(&m)
	if err != nil {
		return err
	}
	if err := ensureMetaDir(dir); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(MarkerPath(dir), append([]byte(markerHeader), b...), config.FileMode); err != nil {
		return fmt.Errorf("cannot save %s: %w", MarkerPath(dir), err)
	}
	return nil
}

func EnsureMetaDir(dir string) error { return ensureMetaDir(dir) }

func ensureMetaDir(dir string) error {
	m := MetaDir(dir)
	if err := os.MkdirAll(m, config.DirMode); err != nil {
		return fmt.Errorf("cannot create %s: %w", m, err)
	}
	if err := os.Chmod(m, config.DirMode); err != nil {
		return fmt.Errorf("cannot set permissions on %s: %w", m, err)
	}
	return nil
}

func isMarker(dir string) bool {
	fi, err := os.Stat(filepath.Join(dir, MetaDirName))
	return err == nil && fi.IsDir()
}

func infoOf(dir string) (Info, error) {
	m, err := readMarker(dir)
	if err != nil {
		return Info{}, err
	}
	name := m.Name
	if name == "" {
		name = filepath.Base(dir)
	}
	return Info{Name: name, Dir: dir}, nil
}

func Find(start string) (Info, bool, error) {
	d, err := filepath.Abs(start)
	if err != nil {
		return Info{}, false, err
	}
	for {
		if isMarker(d) {
			in, err := infoOf(d)
			return in, err == nil, err
		}
		parent := filepath.Dir(d)
		if parent == d {
			return Info{}, false, nil
		}
		d = parent
	}
}

func Current() (Info, bool, error) {
	wd, err := os.Getwd()
	if err != nil {
		return Info{}, false, err
	}
	return Find(wd)
}

func Require() (Info, error) {
	in, ok, err := Current()
	if err != nil {
		return Info{}, err
	}
	if !ok {
		return Info{}, ErrNoSession
	}
	return in, nil
}

func Dir(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	root, err := config.PentestPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, name), nil
}

func ByName(name string) (Info, error) {
	dir, err := Dir(name)
	if err != nil {
		return Info{}, err
	}
	if !isMarker(dir) {
		return Info{}, fmt.Errorf("%w: %s (ars sessions lists them)", ErrNotFound, name)
	}
	return infoOf(dir)
}

func Exists(name string) (bool, error) {
	_, err := ByName(name)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

func FolderExists(name string) (exists, isSession bool, err error) {
	dir, err := Dir(name)
	if err != nil {
		return false, false, err
	}
	fi, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	return fi.IsDir(), fi.IsDir() && isMarker(dir), nil
}

func newMarker(name string, when time.Time) Marker {
	return Marker{Name: name, Created: Date(when.Format("2006-01-02"))}
}

func Create(name string) (string, error) {
	dir, err := Dir(name)
	if err != nil {
		return "", err
	}
	if _, err := config.EnsurePentestPath(); err != nil {
		return "", err
	}
	if err := os.Mkdir(dir, config.DirMode); err != nil {
		if errors.Is(err, os.ErrExist) {
			return dir, ErrExists
		}
		return "", fmt.Errorf("cannot create session: %w", err)
	}
	if err := os.Chmod(dir, config.DirMode); err != nil {
		return "", fmt.Errorf("cannot set permissions on %s: %w", dir, err)
	}
	if err := writeMarker(dir, newMarker(name, time.Now())); err != nil {
		return "", err
	}
	return dir, nil
}

func Adopt(name string) (string, error) {
	dir, err := Dir(name)
	if err != nil {
		return "", err
	}
	if isMarker(dir) {
		return dir, nil
	}
	if err := writeMarker(dir, newMarker(name, time.Now())); err != nil {
		return "", err
	}
	return dir, nil
}

func List() ([]Info, error) {
	root, err := config.PentestPath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", root, err)
	}
	var out []Info
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if !e.IsDir() && e.Type()&os.ModeSymlink == 0 {
			continue
		}
		if !isMarker(dir) {
			continue
		}
		in, err := infoOf(dir)
		if err != nil {
			continue
		}
		out = append(out, in)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
