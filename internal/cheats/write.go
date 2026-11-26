// Validates and writes new entries to the user's cheatsheets.

package cheats

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ynn3m/arsenal/internal/fsutil"
)

const defaultHeader = "My cheats"

var ErrDuplicate = errors.New("an entry with this title already exists: choose another title")

type Entry struct {
	Title       string
	Command     string
	Description string
	Tags        string
	Vars        []string
}

func ValidateTitle(s string) error {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return errors.New("a title is required")
	case strings.ContainsAny(s, "\r\n"):
		return errors.New("the title must be a single line")
	case strings.HasPrefix(s, "#"):
		return errors.New("the title must not start with #")
	}
	return nil
}

func ValidateCommand(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("a command is required")
	}
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			return errors.New("a command line must not start with ``` (it would end the code block)")
		}
	}
	return nil
}

func ValidateCheatFile(s string) error {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return errors.New("a file is required")
	case !strings.EqualFold(filepath.Ext(s), ".md"):
		return errors.New("a cheatsheet file must end in .md")
	}
	return nil
}

func (e Entry) Validate() error {
	if err := ValidateTitle(e.Title); err != nil {
		return err
	}
	return ValidateCommand(e.Command)
}

func NormalizeTags(s string) string {
	s = strings.NewReplacer("\r", ",", "\n", ",").Replace(s)
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, ", ")
}

func splitTags(s string) (plain, command []string) {
	for _, t := range strings.Split(NormalizeTags(s), ", ") {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		k, v, ok := strings.Cut(t, "/")
		switch {
		case t == "":
		case ok && k != "" && v != "" && !strings.ContainsAny(t, " \t"):
			command = append(command, t)
		default:
			plain = append(plain, t)
		}
	}
	return
}

func FormatEntry(e Entry) string { return strings.Join(entryLines(e), "\n") + "\n" }

func entryLines(e Entry) []string {
	var out []string
	out = append(out, "## "+strings.TrimSpace(e.Title))
	out = append(out, e.Vars...)
	plain, command := splitTags(e.Tags)
	if len(command) > 0 {
		out = append(out, "#"+strings.Join(command, " #"))
	}
	if len(plain) > 0 {
		out = append(out, "% "+strings.Join(plain, ", "))
	}
	for _, l := range strings.Split(strings.ReplaceAll(e.Description, "\r\n", "\n"), "\n") {
		l = strings.TrimRight(l, " \t\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		if strings.ContainsAny(l[:1], "#%=$`") {
			l = " " + l
		}
		out = append(out, l)
	}
	out = append(out, "", "```")
	for _, l := range strings.Split(strings.ReplaceAll(e.Command, "\r\n", "\n"), "\n") {
		if l = strings.TrimRight(l, " \t\r"); strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return append(out, "```")
}

func TitleExists(path, title string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	list, err := ParseMarkdown(path, strings.NewReader(string(b)))
	if err != nil {
		return false
	}
	want := strings.TrimSpace(title)
	for _, c := range list {
		if strings.TrimSpace(c.Name) == want {
			return true
		}
	}
	return false
}

func AppendEntry(path string, e Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := ValidateCheatFile(path); err != nil {
		return err
	}

	var existing []byte
	perm := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		if fi.IsDir() {
			return fmt.Errorf("%s is a folder: give a .md file", path)
		}
		perm = fi.Mode().Perm()
		if existing, err = os.ReadFile(path); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	text := string(existing)
	if text != "" {
		list, err := ParseMarkdown(path, strings.NewReader(text))
		if err != nil {
			return fmt.Errorf("cannot add to %s because it has errors (fix the file first): %w", path, err)
		}
		want := strings.TrimSpace(e.Title)
		for _, c := range list {
			if strings.TrimSpace(c.Name) == want {
				return fmt.Errorf("%w: %q in %s", ErrDuplicate, want, path)
			}
		}
	}

	d := loadDoc(text)
	plain, _ := splitTags(e.Tags)
	d.insertEntry(SectionOf(e.Command), entryLines(e), strings.TrimSpace(e.Title), strings.Join(plain, ", "))
	d.fixTags()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, []byte(d.text()), perm)
}

func SectionOf(command string) string {
	if t := ToolOf(command); t != "" {
		return t
	}
	return defaultHeader
}
