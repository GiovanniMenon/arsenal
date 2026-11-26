// Reads and updates the keys of config.yml.

package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/ynn3m/arsenal/internal/fsutil"
)

type File struct {
	PentestPath  string   `yaml:"pentest_path"`
	CheatsPaths  []string `yaml:"cheats_paths"`
	RecByDefault bool     `yaml:"rec_by_default"`
	Hints        *bool    `yaml:"hints"`
	MaxCastMB    int      `yaml:"max_cast_mb"`

	RedactPatterns []string `yaml:"redact_patterns"`
}

func FilePath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "config.yml"), nil
}

func LoadFile() (File, error) {
	var f File
	p, err := FilePath()
	if err != nil {
		return f, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, fmt.Errorf("cannot read %s: %w", p, err)
	}
	if err := yaml.Unmarshal(b, &f); err != nil {
		return f, fmt.Errorf("cannot read %s: %w (fix the file, or delete it and run ars setup)", p, err)
	}
	return f, nil
}

func CheatsPaths() ([]string, error) {
	f, err := LoadFile()
	if err != nil {
		return nil, err
	}
	own, err := CheatsDir()
	if err != nil {
		return nil, err
	}
	raw := f.CheatsPaths
	if len(raw) == 0 {
		raw = []string{own}
	}
	raw = append(append([]string{}, raw...), own)
	if legacy, err := LegacyCheatsDir(); err == nil && isDir(legacy) {
		raw = append(raw, legacy)
	}
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, p := range raw {
		e, err := expandHome(p)
		if err != nil {
			return nil, err
		}
		if seen[e] {
			continue
		}
		seen[e] = true
		out = append(out, e)
	}
	return out, nil
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func RecByDefault() (bool, error) {
	f, err := LoadFile()
	if err != nil {
		return false, err
	}
	return f.RecByDefault, nil
}

func Hints() (bool, error) {
	f, err := LoadFile()
	if err != nil {
		return true, err
	}
	if f.Hints == nil {
		return true, nil
	}
	return *f.Hints, nil
}

func MaxCastMB() (int, error) {
	f, err := LoadFile()
	if err != nil {
		return 0, err
	}
	if f.MaxCastMB <= 0 {
		return 256, nil
	}
	return f.MaxCastMB, nil
}

func RedactPatterns() ([]string, error) {
	f, err := LoadFile()
	if err != nil {
		return nil, err
	}
	return f.RedactPatterns, nil
}

func CheatsDir() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "cheats"), nil
}

func LegacyCheatsDir() (string, error) { return expandHome("~/.cheats") }

func SetFileKeys(values map[string]any) error {
	p, err := FilePath()
	if err != nil {
		return err
	}
	var doc yaml.Node
	if b, err := os.ReadFile(p); err == nil {
		if err := yaml.Unmarshal(b, &doc); err != nil {
			return fmt.Errorf("cannot read %s: %w (fix the file, or delete it and run ars setup)", p, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("cannot read %s: %w", p, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	m := doc.Content[0]
	if m.Kind != yaml.MappingNode {
		return fmt.Errorf("cannot use %s: it must contain key: value lines (fix the file, or delete it and run ars setup)", p)
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var v yaml.Node
		if err := v.Encode(values[k]); err != nil {
			return err
		}
		replaced := false
		for i := 0; i+1 < len(m.Content); i += 2 {
			if m.Content[i].Value == k {
				v.HeadComment, v.LineComment = m.Content[i+1].HeadComment, m.Content[i+1].LineComment
				m.Content[i+1] = &v
				replaced = true
				break
			}
		}
		if !replaced {
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: k}, &v)
		}
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), DirMode); err != nil {
		return fmt.Errorf("cannot create the config folder: %w", err)
	}
	return fsutil.WriteFileAtomic(p, buf.Bytes(), FileMode)
}

func HasFileKey(key string) (bool, error) {
	p, err := FilePath()
	if err != nil {
		return false, err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("cannot read %s: %w", p, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		return false, fmt.Errorf("cannot read %s: %w (fix the file, or delete it and run ars setup)", p, err)
	}
	_, ok := m[key]
	return ok, nil
}
