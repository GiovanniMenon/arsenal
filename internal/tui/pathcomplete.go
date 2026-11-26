// Completes file paths on tab, like a shell.

package tui

import (
	"os"
	"path/filepath"
	"strings"
)

func CompletePath(v string) string {
	if v == "" {
		return ""
	}
	dir, base := filepath.Split(v)
	listDir := dir
	if listDir == "" {
		listDir = "."
	}
	if listDir == "~" || strings.HasPrefix(listDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		listDir = filepath.Join(home, strings.TrimPrefix(listDir, "~"))
	}
	entries, err := os.ReadDir(listDir)
	if err != nil {
		return ""
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		if strings.HasPrefix(e.Name(), base) {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return ""
	}
	prefix := names[0]
	for _, n := range names[1:] {
		for !strings.HasPrefix(n, prefix) {
			prefix = prefix[:len(prefix)-1]
		}
	}
	result := dir + prefix
	check := filepath.Join(listDir, prefix)
	if fi, err := os.Stat(check); err == nil && fi.IsDir() && !strings.HasSuffix(result, string(os.PathSeparator)) {
		result += string(os.PathSeparator)
	}
	return result
}
