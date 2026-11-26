// Writes, parses, edits and deletes the session notes in notes.md.

package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/ynn3m/arsenal/internal/config"
	"github.com/ynn3m/arsenal/internal/fsutil"
)

const NotesFileName = "notes.md"

const NoteTimeFormat = "2006-01-02 15:04"

var ErrNoNotes = errors.New("no notes yet")

var ErrNoteChanged = errors.New("the note changed in notes.md since it was opened: open ars notes again")

type Note struct {
	Index  int
	Time   time.Time
	Title  string
	Target string
	Body   string
}

func NotesPath(dir string) string { return filepath.Join(MetaDir(dir), NotesFileName) }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func cleanTitle(s string) string {
	s = strings.ReplaceAll(oneLine(s), " | ", " / ")
	if strings.HasPrefix(strings.ToLower(s), "target:") {
		s = "target -" + s[len("target:"):]
	}
	return s
}

func NoteHeader(t time.Time, title, target string) string {
	h := "## " + t.Format(NoteTimeFormat)
	if title = cleanTitle(title); title != "" {
		h += " | " + title
	}
	if target = oneLine(target); target != "" {
		h += " | target: " + target
	}
	return h
}

var noteHeaderRe = regexp.MustCompile(`^## (\d{4}-\d{2}-\d{2} \d{2}:\d{2})(?: \| (.*))?$`)

func FormatNote(t time.Time, title, target, body string) string {
	body = strings.TrimSpace(strings.ReplaceAll(body, "\r\n", "\n"))
	lines := strings.Split(body, "\n")
	for i, l := range lines {
		if noteHeaderRe.MatchString(strings.TrimRight(l, " \t")) {
			lines[i] = " " + l
		}
	}
	out := NoteHeader(t, title, target) + "\n\n"
	if body != "" {
		out += strings.Join(lines, "\n") + "\n\n"
	}
	return out
}

type noteChunk struct {
	note Note
	text string
}

func splitNotes(md string) (pre string, chunks []noteChunk) {
	var body []string
	flush := func() {
		if n := len(chunks); n > 0 {
			chunks[n-1].note.Body = strings.TrimSpace(strings.Join(body, "\n"))
		}
		body = nil
	}
	for _, raw := range strings.SplitAfter(strings.ReplaceAll(md, "\r\n", "\n"), "\n") {
		if raw == "" {
			continue
		}
		line := strings.TrimSuffix(raw, "\n")
		n, ok := parseNoteHeader(line)
		if !ok {
			if len(chunks) == 0 {
				pre += raw
				continue
			}
			body = append(body, line)
			chunks[len(chunks)-1].text += raw
			continue
		}
		flush()
		n.Index = len(chunks)
		chunks = append(chunks, noteChunk{note: n, text: raw})
	}
	flush()
	return pre, chunks
}

func parseNoteHeader(line string) (Note, bool) {
	m := noteHeaderRe.FindStringSubmatch(strings.TrimRight(line, " \t"))
	if m == nil {
		return Note{}, false
	}
	t, err := time.ParseInLocation(NoteTimeFormat, m[1], time.Local)
	if err != nil {
		return Note{}, false
	}
	n := Note{Time: t}
	if m[2] != "" {
		parts := strings.Split(m[2], " | ")
		if last := parts[len(parts)-1]; strings.HasPrefix(last, "target: ") {
			n.Target = strings.TrimSpace(strings.TrimPrefix(last, "target: "))
			parts = parts[:len(parts)-1]
		}
		n.Title = strings.Join(parts, " | ")
	}
	return n, true
}

func ParseNotes(md string) []Note {
	_, chunks := splitNotes(md)
	notes := make([]Note, len(chunks))
	for i, c := range chunks {
		notes[i] = c.note
	}
	return notes
}

func UpdateNote(dir string, old Note, title, body string) error {
	if strings.TrimSpace(title) == "" && strings.TrimSpace(body) == "" {
		return errors.New("the note is empty: write some text")
	}
	return rewriteNote(dir, old, FormatNote(old.Time, title, old.Target, body))
}

func DeleteNote(dir string, old Note) error {
	return rewriteNote(dir, old, "")
}

func rewriteNote(dir string, old Note, replacement string) error {
	path := NotesPath(dir)
	b, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", path, err)
	}
	pre, chunks := splitNotes(string(b))
	if old.Index < 0 || old.Index >= len(chunks) || chunks[old.Index].note != old {
		return ErrNoteChanged
	}
	chunks[old.Index].text = replacement
	var out strings.Builder
	out.WriteString(pre)
	for _, c := range chunks {
		out.WriteString(c.text)
	}
	if err := fsutil.WriteFileAtomic(path, []byte(out.String()), config.FileMode); err != nil {
		return fmt.Errorf("cannot write %s: %w", path, err)
	}
	return nil
}

func AddNote(s Info, title, body string, now time.Time) (header string, err error) {
	name := s.Name
	if strings.TrimSpace(title) == "" && strings.TrimSpace(body) == "" {
		return "", errors.New("the note is empty")
	}
	path := NotesPath(s.Dir)
	r, err := Resolve(s.Dir)
	if err != nil {
		return "", err
	}
	target := r.Vars["target"]

	entry := FormatNote(now, title, target, body)
	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	switch {
	case len(existing) == 0:
		entry = "# Notes - " + name + "\n\n" + entry
	case !strings.HasSuffix(string(existing), "\n"):
		entry = "\n\n" + entry
	case !strings.HasSuffix(string(existing), "\n\n"):
		entry = "\n" + entry
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, config.FileMode)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %w", path, err)
	}
	if _, err := f.WriteString(entry); err != nil {
		f.Close()
		return "", fmt.Errorf("cannot write %s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return NoteHeader(now, title, target), nil
}

func ReadNotes(dir string) (string, error) {
	path := NotesPath(dir)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && strings.TrimSpace(string(b)) == "") {
		return "", ErrNoNotes
	}
	if err != nil {
		return "", fmt.Errorf("cannot read %s: %w", path, err)
	}
	return string(b), nil
}
