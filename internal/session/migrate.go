// Moves sessions written by older versions into the .arsenal layout.

package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ynn3m/arsenal/internal/config"
)

const legacyCurrentFile = "current"

func Migrate(cwd string) ([]string, error) {
	cfg, err := config.ConfigDir()
	if err != nil {
		return nil, err
	}
	cur := filepath.Join(cfg, legacyCurrentFile)
	_, curErr := os.Lstat(cur)
	hasCurrent := curErr == nil

	root, err := config.PentestPath()
	if err != nil {
		return nil, err
	}
	var cands []string
	seen := map[string]bool{}
	add := func(d string) {
		if !seen[d] {
			seen[d] = true
			cands = append(cands, d)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("cannot read %s: %w", root, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() && ValidateName(e.Name()) == nil {
			add(filepath.Join(root, e.Name()))
		}
	}
	if d, ok := legacyMarkerAbove(cwd); ok {
		add(d)
	}

	var lines []string
	clean := true
	for _, dir := range cands {
		name := filepath.Base(dir)
		fi, err := os.Lstat(filepath.Join(dir, MetaDirName))
		switch {
		case err == nil && fi.Mode().IsRegular():
			if err := migrateMarkerFile(dir); err != nil {
				return lines, err
			}
			lines = append(lines, fmt.Sprintf("session %s: moved the old %s file to %s", name, MetaDirName, filepath.Join(MetaDirName, MarkerFile)))
		case errors.Is(err, os.ErrNotExist) && hasCurrent && ValidateName(name) == nil:
			l, ok, err := migrateFromVars(dir, name)
			if err != nil {
				return lines, err
			}
			clean = clean && ok
			lines = append(lines, l...)
		default:
			continue
		}
		lines = append(lines, moveManaged(dir, name)...)
	}
	if hasCurrent && clean {
		if err := os.Remove(cur); err != nil && !errors.Is(err, os.ErrNotExist) {
			return lines, err
		}
		lines = append(lines, "removed "+cur+" (not used: the session is the folder you are in)")
	}
	return lines, nil
}

func legacyMarkerAbove(start string) (string, bool) {
	d, err := filepath.Abs(start)
	if err != nil {
		return "", false
	}
	for {
		if fi, err := os.Lstat(filepath.Join(d, MetaDirName)); err == nil {
			return d, fi.Mode().IsRegular()
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", false
		}
		d = parent
	}
}

func migrateMarkerFile(dir string) error {
	old := filepath.Join(dir, MetaDirName)
	tmp := filepath.Join(dir, fmt.Sprintf("%s.migrating-%d", MetaDirName, os.Getpid()))
	if err := os.Rename(old, tmp); err != nil {
		return fmt.Errorf("cannot move %s: %w", old, err)
	}
	if err := ensureMetaDir(dir); err != nil {
		_ = os.Rename(tmp, old)
		return err
	}
	if err := os.Rename(tmp, MarkerPath(dir)); err != nil {
		_ = os.Remove(MetaDir(dir))
		_ = os.Rename(tmp, old)
		return fmt.Errorf("cannot move %s into %s: %w", old, MetaDir(dir), err)
	}
	return os.Chmod(MarkerPath(dir), config.FileMode)
}

func migrateFromVars(dir, name string) (lines []string, ok bool, err error) {
	m := newMarker(name, timeOf(dir))
	line := "session " + name + ": created " + filepath.Join(MetaDirName, MarkerFile)
	ok = true
	varsFile := filepath.Join(dir, VarsFileName)
	moved := false
	if _, err := os.Stat(varsFile); err == nil {
		v, err := loadFile(varsFile)
		if err != nil {
			ok = false
			lines = append(lines, fmt.Sprintf("session %s: could not move its vars file (%v)", name, err))
		} else {
			if len(v) > 0 {
				m.Vars = map[string]string(v)
			}
			moved = true
		}
	}
	if err := writeMarker(dir, m); err != nil {
		return lines, false, err
	}
	if moved {
		if err := os.Remove(varsFile); err != nil {
			ok = false
		} else {
			line += ", variables moved into it"
		}
	}
	return append(lines, line), ok, nil
}

func moveManaged(dir, name string) []string {
	var moved, kept []string
	for _, f := range []string{NotesFileName, "commands.jsonl", "recordings"} {
		src, dst := filepath.Join(dir, f), filepath.Join(MetaDir(dir), f)
		fi, err := os.Lstat(src)
		if err != nil {
			continue
		}
		if _, err := os.Lstat(dst); err == nil {
			kept = append(kept, f)
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			kept = append(kept, f)
			continue
		}
		if fi.IsDir() {
			_ = os.Chmod(dst, config.DirMode)
			moved = append(moved, f+"/")
		} else {
			_ = os.Chmod(dst, config.FileMode)
			moved = append(moved, f)
		}
	}
	var lines []string
	if len(moved) > 0 {
		lines = append(lines, fmt.Sprintf("session %s: moved %s into %s/", name, strings.Join(moved, ", "), MetaDirName))
	}
	if len(kept) > 0 {
		lines = append(lines, fmt.Sprintf("session %s: %s left in place (already in %s/ or could not be moved)", name, strings.Join(kept, ", "), MetaDirName))
	}
	return lines
}

func timeOf(dir string) time.Time {
	if fi, err := os.Stat(dir); err == nil {
		return fi.ModTime()
	}
	return time.Now()
}

func MigratedNames(lines []string) map[string]bool {
	out := map[string]bool{}
	for _, l := range lines {
		rest, ok := strings.CutPrefix(l, "session ")
		if !ok {
			continue
		}
		if name, _, ok := strings.Cut(rest, ":"); ok && name != "" {
			out[name] = true
		}
	}
	return out
}
