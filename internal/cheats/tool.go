// Finds which tool a command runs, from its first word.

package cheats

import (
	"path"
	"regexp"
	"strings"
)

var (
	envAssignRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)
	toolNameRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._+-]*$`)
	scriptExts  = []string{".py", ".sh", ".rb", ".pl", ".php", ".ps1", ".exe", ".bat"}
)

func ToolOf(command string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(command), "\n")
	for _, w := range strings.Fields(first) {
		if envAssignRe.MatchString(w) || w == "sudo" {
			continue
		}
		w = strings.Trim(w, `"'`)
		if strings.Contains(w, "/") {
			w = path.Base(w)
		}
		lower := strings.ToLower(w)
		for _, ext := range scriptExts {
			if strings.HasSuffix(lower, ext) && len(w) > len(ext) {
				w = w[:len(w)-len(ext)]
				break
			}
		}
		if !toolNameRe.MatchString(w) {
			return ""
		}
		return w
	}
	return ""
}
