// Inserts, updates and deletes single entries in a Markdown cheatsheet,
// leaving the rest of the file and the tags of the other entries as they are.

package cheats

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/ynn3m/arsenal/internal/fsutil"
)

type mdBlock struct {
	level int
	title string
	lines []string
	want  string
	check bool
}

type mdDoc struct{ blocks []mdBlock }

func headingOf(line string) (level int, title string, ok bool) {
	if !strings.HasPrefix(line, "# ") && !strings.HasPrefix(line, "##") {
		return 0, "", false
	}
	rest := line
	for strings.HasPrefix(rest, "#") {
		level++
		rest = rest[1:]
	}
	return level, strings.TrimSpace(strings.TrimLeftFunc(rest, unicode.IsSpace)), true
}

func walkLines(lines []string, f func(i int, line string, afterCode bool)) {
	fenced, inComment, after := false, false, false
	for i, l := range lines {
		if !fenced {
			if inComment {
				inComment = !strings.Contains(l, "-->")
				continue
			}
			if t := strings.TrimSpace(l); strings.HasPrefix(t, "<!--") {
				inComment = !strings.Contains(t[4:], "-->")
				continue
			}
		}
		if strings.HasPrefix(l, "```") {
			if fenced {
				after = true
			}
			fenced = !fenced
			continue
		}
		if !fenced {
			f(i, strings.TrimRight(l, "\r"), after)
		}
	}
}

func splitBlocks(text string) []mdBlock {
	var blocks []mdBlock
	cur := mdBlock{}
	fenced, inComment := false, false
	flush := func() {
		if len(cur.lines) > 0 {
			blocks = append(blocks, cur)
		}
	}
	for _, l := range strings.Split(text, "\n") {
		switch {
		case fenced:
			if strings.HasPrefix(l, "```") {
				fenced = false
			}
		case inComment:
			inComment = !strings.Contains(l, "-->")
		case strings.HasPrefix(l, "```"):
			fenced = true
		case strings.HasPrefix(strings.TrimSpace(l), "<!--"):
			inComment = !strings.Contains(strings.TrimSpace(l)[4:], "-->")
		default:
			if lv, title, ok := headingOf(strings.TrimRight(l, "\r")); ok {
				flush()
				cur = mdBlock{level: lv, title: title}
			}
		}
		cur.lines = append(cur.lines, l)
	}
	flush()
	if len(blocks) == 1 && blocks[0].level == 0 && strings.TrimSpace(strings.Join(blocks[0].lines, "")) == "" {
		return nil
	}
	return blocks
}

func (b mdBlock) hasCode() bool {
	for _, l := range b.lines {
		if strings.HasPrefix(l, "```") {
			return true
		}
	}
	return false
}

func (b mdBlock) isEntry() bool { return b.level >= 2 && b.hasCode() }

func contentLen(lines []string) int {
	n := len(lines)
	for n > 0 && strings.TrimSpace(lines[n-1]) == "" {
		n--
	}
	return n
}

func (b mdBlock) tagLines() (before string, hasBefore bool, after string, hasAfter bool) {
	walkLines(b.lines, func(_ int, l string, afterCode bool) {
		if !strings.HasPrefix(l, "%") {
			return
		}
		if afterCode {
			after, hasAfter = strings.TrimSpace(l[1:]), true
		} else {
			before, hasBefore = strings.TrimSpace(l[1:]), true
		}
	})
	return
}

func loadDoc(text string) *mdDoc {
	d := &mdDoc{blocks: splitBlocks(text)}
	state := ""
	for i := range d.blocks {
		b := &d.blocks[i]
		before, hb, after, ha := b.tagLines()
		b.want = state
		if hb {
			b.want, state = before, before
		}
		if ha {
			state = after
		}
		b.check = b.isEntry()
	}
	return d
}

func (d *mdDoc) fixTags() {
	state := ""
	for i := range d.blocks {
		b := &d.blocks[i]
		before, hb, after, ha := b.tagLines()
		if b.check && !hb && state != b.want {
			line := "%"
			if b.want != "" {
				line = "% " + b.want
			}
			b.lines = append(b.lines[:1], append([]string{line}, b.lines[1:]...)...)
			before, hb = b.want, true
		}
		if hb {
			state = before
		}
		if ha {
			state = after
		}
	}
}

func (d *mdDoc) text() string {
	var all []string
	for _, b := range d.blocks {
		all = append(all, b.lines...)
	}
	s := strings.Join(all, "\n")
	if s != "" && !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	return s
}

func (d *mdDoc) section(tool string) int {
	for i, b := range d.blocks {
		if b.level == 1 && strings.EqualFold(b.title, tool) {
			return i
		}
	}
	return -1
}

func (d *mdDoc) ensureBlank(i int) {
	b := &d.blocks[i]
	if len(b.lines) == contentLen(b.lines) {
		b.lines = append(b.lines, "")
	}
}

func (d *mdDoc) insertEntry(tool string, lines []string, title, want string) {
	nb := mdBlock{level: 2, title: title, lines: append(append([]string(nil), lines...), ""), want: want, check: true}
	if si := d.section(tool); si >= 0 {
		k := si
		for j := si + 1; j < len(d.blocks) && d.blocks[j].level != 1; j++ {
			k = j
		}
		d.ensureBlank(k)
		d.blocks = append(d.blocks[:k+1], append([]mdBlock{nb}, d.blocks[k+1:]...)...)
		return
	}
	if n := len(d.blocks); n > 0 {
		d.ensureBlank(n - 1)
	}
	d.blocks = append(d.blocks, mdBlock{level: 1, title: tool, lines: []string{"# " + tool, ""}}, nb)
}

func (d *mdDoc) removeBlock(i int) {
	d.blocks = append(d.blocks[:i], d.blocks[i+1:]...)
	if i > 0 {
		d.dropIfEmptySection(i - 1)
	}
}

func (d *mdDoc) dropIfEmptySection(i int) {
	h := d.blocks[i]
	if h.level != 1 || contentLen(h.lines) > 1 || (i+1 < len(d.blocks) && d.blocks[i+1].level != 1) {
		return
	}
	d.blocks = append(d.blocks[:i], d.blocks[i+1:]...)
}

func (d *mdDoc) entryTitles(except int) map[string]bool {
	out := map[string]bool{}
	for i, b := range d.blocks {
		if i != except && b.level >= 2 {
			out[strings.TrimSpace(b.title)] = true
		}
	}
	return out
}

var ErrChanged = errors.New("the entry changed on disk: run ars again")

func openEntry(c *Cheat) (d *mdDoc, idx int, mode os.FileMode, err error) {
	if err := ValidateCheatFile(c.Filename); err != nil {
		return nil, 0, 0, err
	}
	if c.Level != 2 || c.Line == 0 {
		return nil, 0, 0, fmt.Errorf("cannot change %q in place: it is not a ## entry", c.Name)
	}
	fi, err := os.Stat(c.Filename)
	if err != nil {
		return nil, 0, 0, err
	}
	raw, err := os.ReadFile(c.Filename)
	if err != nil {
		return nil, 0, 0, err
	}
	d = loadDoc(string(raw))
	idx, start := -1, 0
	for i, b := range d.blocks {
		if start == c.Line-1 {
			idx = i
			break
		}
		start += len(b.lines)
	}
	if idx < 0 || !d.blocks[idx].isEntry() || d.blocks[idx].level != 2 || d.blocks[idx].title != strings.TrimSpace(c.Name) {
		return nil, 0, 0, ErrChanged
	}
	return d, idx, fi.Mode().Perm(), nil
}

func DeleteEntry(c *Cheat) error {
	d, idx, mode, err := openEntry(c)
	if err != nil {
		return err
	}
	d.removeBlock(idx)
	d.fixTags()
	return fsutil.WriteFileAtomic(c.Filename, []byte(d.text()), mode)
}

func UpdateEntry(c *Cheat, e Entry) error {
	if err := e.Validate(); err != nil {
		return err
	}
	d, idx, mode, err := openEntry(c)
	if err != nil {
		return err
	}
	if d.entryTitles(idx)[strings.TrimSpace(e.Title)] {
		return fmt.Errorf("%w: %q in %s", ErrDuplicate, strings.TrimSpace(e.Title), c.Filename)
	}

	old := d.blocks[idx]
	lines := entryLines(e)
	var vars []string
	walkLines(old.lines, func(_ int, l string, _ bool) {
		if (strings.HasPrefix(l, "=") || strings.HasPrefix(l, "$")) && isVarLine(l) {
			vars = append(vars, l)
		}
	})
	lines = append(lines[:1], append(vars, lines[1:]...)...)

	plain, _ := splitTags(e.Tags)
	tb := old.lines[contentLen(old.lines):]
	if len(tb) == 0 {
		tb = []string{""}
	}
	d.blocks[idx] = mdBlock{level: 2, title: strings.TrimSpace(e.Title), lines: append(lines, tb...), want: strings.Join(plain, ", "), check: true}
	d.fixTags()
	return fsutil.WriteFileAtomic(c.Filename, []byte(d.text()), mode)
}
