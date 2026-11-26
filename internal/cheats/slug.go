// Turns a group or tool name into the name of its cheatsheet file.

package cheats

import (
	"path/filepath"
	"strings"
)

func Slug(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		} else {
			dash = true
		}
	}
	if b.Len() == 0 {
		return "cheats"
	}
	return b.String()
}

func GroupFile(dir, group string) string { return filepath.Join(dir, Slug(group)+".md") }

func CommandFile(dir, command string) string { return GroupFile(dir, SectionOf(command)) }
