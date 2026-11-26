// Session and global variables, and how they combine.

package session

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/fsutil"
)

const VarsFileName = "vars"

var keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

type Vars map[string]string

func ValidateKey(key string) error {
	if !keyRe.MatchString(key) {
		return fmt.Errorf("invalid variable name %q: use letters, digits and _, not starting with a digit", key)
	}
	return nil
}

func ParsePair(s string) (key, value string, err error) {
	key, value, ok := strings.Cut(s, "=")
	if !ok {
		return "", "", fmt.Errorf("%q is not key=value: write it like target=10.10.10.5", s)
	}
	if err := ValidateKey(key); err != nil {
		return "", "", err
	}
	if value == "" {
		return "", "", fmt.Errorf("variable %q has an empty value: give it a value, or remove it with ars unset", key)
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", "", fmt.Errorf("variable %q: value must be a single line", key)
	}
	return key, value, nil
}

func loadFile(p string) (Vars, error) {
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return Vars{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cannot read variables: %w", err)
	}
	defer f.Close()

	vars := Vars{}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSuffix(sc.Text(), "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok || ValidateKey(k) != nil {
			return nil, fmt.Errorf("%s line %d is not key=value (%q): fix or delete the line", p, n, line)
		}
		vars[k] = v
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("cannot read variables: %w", err)
	}
	return vars, nil
}

func saveFile(p string, vars Vars) error {
	var b strings.Builder
	for _, k := range vars.Keys() {
		b.WriteString(k + "=" + vars[k] + "\n")
	}
	if err := fsutil.WriteFileAtomic(p, []byte(b.String()), config.FileMode); err != nil {
		return fmt.Errorf("cannot save variables: %w", err)
	}
	return nil
}

func setIn(p string, set Vars) error {
	vars, err := loadFile(p)
	if err != nil {
		return err
	}
	for k, v := range set {
		vars[k] = v
	}
	return saveFile(p, vars)
}

func unsetIn(p string, keys []string) (missing []string, err error) {
	vars, err := loadFile(p)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if _, ok := vars[k]; ok {
			delete(vars, k)
		} else {
			missing = append(missing, k)
		}
	}
	if len(missing) < len(keys) {
		if err := saveFile(p, vars); err != nil {
			return nil, err
		}
	}
	return missing, nil
}

func LoadVars(dir string) (Vars, error) {
	m, err := readMarker(dir)
	if err != nil {
		return nil, err
	}
	v := Vars{}
	for k, val := range m.Vars {
		v[k] = val
	}
	return v, nil
}

func SaveVars(dir string, vars Vars) error {
	m, err := readMarker(dir)
	if err != nil {
		return err
	}
	m.Vars = map[string]string(vars)
	if len(vars) == 0 {
		m.Vars = nil
	}
	return writeMarker(dir, m)
}

func SetVars(dir string, set Vars) error {
	v, err := LoadVars(dir)
	if err != nil {
		return err
	}
	for k, val := range set {
		v[k] = val
	}
	return SaveVars(dir, v)
}

func UnsetVars(dir string, keys []string) (missing []string, err error) {
	v, err := LoadVars(dir)
	if err != nil {
		return nil, err
	}
	removed := false
	for _, k := range keys {
		if _, ok := v[k]; ok {
			delete(v, k)
			removed = true
		} else {
			missing = append(missing, k)
		}
	}
	if removed {
		if err := SaveVars(dir, v); err != nil {
			return nil, err
		}
	}
	return missing, nil
}

func globalPath() (string, error) {
	p, err := config.GlobalVarsPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), config.DirMode); err != nil {
		return "", fmt.Errorf("cannot create the config folder: %w", err)
	}
	return p, nil
}

func LoadGlobalVars() (Vars, error) {
	p, err := config.GlobalVarsPath()
	if err != nil {
		return nil, err
	}
	return loadFile(p)
}

func SetGlobalVars(set Vars) error {
	p, err := globalPath()
	if err != nil {
		return err
	}
	return setIn(p, set)
}

func UnsetGlobalVars(keys []string) (missing []string, err error) {
	p, err := globalPath()
	if err != nil {
		return nil, err
	}
	return unsetIn(p, keys)
}

func ClearVars(dir string) error { return SaveVars(dir, Vars{}) }

func ClearGlobalVars() error {
	p, err := globalPath()
	if err != nil {
		return err
	}
	return saveFile(p, Vars{})
}

type Source string

const (
	SourceGlobal  Source = "global"
	SourceSession Source = "session"
)

type Resolved struct {
	Vars   Vars
	Source map[string]Source
}

func Resolve(dir string) (Resolved, error) {
	global, err := LoadGlobalVars()
	if err != nil {
		return Resolved{}, err
	}
	r := Resolved{Vars: Vars{}, Source: map[string]Source{}}
	for k, v := range global {
		r.Vars[k] = v
		r.Source[k] = SourceGlobal
	}
	if dir == "" {
		return r, nil
	}
	local, err := LoadVars(dir)
	if err != nil {
		return Resolved{}, err
	}
	for k, v := range local {
		r.Vars[k] = v
		r.Source[k] = SourceSession
	}
	return r, nil
}

func (v Vars) Keys() []string {
	keys := make([]string, 0, len(v))
	for k := range v {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
