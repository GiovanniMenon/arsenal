// Adds the official Arsenal commands to the user's collection and removes them again.

package cheats

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/ynn3m/arsenal/internal/fsutil"
)

type Addition struct {
	Group string
	Entry Entry
}

type AddResult struct {
	Added     int
	Files     int
	Conflicts []string
}

func parseVarLine(l string) (name, value string) {
	l = strings.TrimSpace(l)
	if len(l) < 2 {
		return "", ""
	}
	n, v, _ := strings.Cut(l[1:], ":")
	name, value = strings.TrimSpace(n), strings.TrimSpace(v)
	if l[0] == '$' {
		value = "$(" + value + ")"
	}
	return
}

func inlineSafe(value string) bool {
	return value != "" && !strings.ContainsAny(value, " \t<>|")
}

func printable(command string) string {
	var parts []string
	for _, l := range strings.Split(strings.ReplaceAll(command, "\r\n", "\n"), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			parts = append(parts, l)
		}
	}
	var b strings.Builder
	for i, l := range parts {
		b.WriteString(l)
		if i < len(parts)-1 && !strings.HasSuffix(l, ";") {
			b.WriteString(";")
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func commandLines(c *Cheat) string { return strings.ReplaceAll(c.Command, "\\\n", "\n") }

func AddEntries(dir string, items []Addition) (AddResult, error) {
	type pending struct {
		path    string
		perm    os.FileMode
		doc     *mdDoc
		titles  map[string]string
		vars    map[string]string
		changed bool
	}
	files := map[string]*pending{}
	var order []string
	var res AddResult

	for _, it := range items {
		e := it.Entry
		if err := e.Validate(); err != nil {
			continue
		}
		path := GroupFile(dir, it.Group)
		f := files[path]
		if f == nil {
			f = &pending{path: path, perm: 0o644, titles: map[string]string{}, vars: map[string]string{}}
			text := ""
			if b, err := os.ReadFile(path); err == nil {
				text = string(b)
				if fi, err := os.Stat(path); err == nil {
					f.perm = fi.Mode().Perm()
				}
				list, err := ParseMarkdown(path, strings.NewReader(text))
				if err != nil {
					return AddResult{}, fmt.Errorf("cannot add to %s because it has errors (fix the file first): %w", path, err)
				}
				for _, c := range list {
					f.titles[strings.TrimSpace(c.Name)] = printable(commandLines(c))
					for name, v := range c.Variables {
						f.vars[name] = v
					}
				}
			} else if !os.IsNotExist(err) {
				return AddResult{}, err
			}
			f.doc = loadDoc(text)
			files[path] = f
			order = append(order, path)
		}

		var newVars, clashes []string
		for _, v := range e.Vars {
			name, val := parseVarLine(v)
			cur, defined := f.vars[name]
			switch {
			case !defined:
				newVars = append(newVars, v)
			case cur == val:
			case inlineSafe(val):
				e.Command = strings.ReplaceAll(e.Command, "<"+name+">", "<"+name+"|"+val+">")
			default:
				clashes = append(clashes, filepath.Base(path)+": "+name)
			}
		}
		title, cmd := strings.TrimSpace(e.Title), printable(e.Command)
		if have, ok := f.titles[title]; ok {
			if have == cmd {
				continue
			}
			base, free, same := title, "", false
			for n := 2; free == "" && !same; n++ {
				cand := fmt.Sprintf("%s (%d)", base, n)
				if have, taken := f.titles[cand]; !taken {
					free = cand
				} else {
					same = have == cmd
				}
			}
			if same {
				continue
			}
			title, e.Title = free, free
		}
		for _, v := range newVars {
			name, val := parseVarLine(v)
			f.vars[name] = val
		}
		for _, c := range clashes {
			if !slices.Contains(res.Conflicts, c) {
				res.Conflicts = append(res.Conflicts, c)
			}
		}
		e.Vars = newVars
		plain, _ := splitTags(e.Tags)
		f.doc.insertEntry(it.Group, entryLines(e), title, strings.Join(plain, ", "))
		f.titles[title] = cmd
		f.changed = true
		res.Added++
	}

	for _, p := range order {
		if f := files[p]; f.changed {
			f.doc.fixTags()
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return AddResult{}, err
	}
	for _, p := range order {
		f := files[p]
		if !f.changed {
			continue
		}
		if err := fsutil.WriteFileAtomic(f.path, []byte(f.doc.text()), f.perm); err != nil {
			return AddResult{}, err
		}
		res.Files++
	}
	return res, nil
}

func varLines(c *Cheat) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range placeholderRe.FindAllStringSubmatch(c.Command, -1) {
		name, _, _ := strings.Cut(m[1], "|")
		v := c.Variables[name]
		if name == "" || v == "" || seen[name] {
			continue
		}
		seen[name] = true
		if strings.HasPrefix(v, "$(") && strings.HasSuffix(v, ")") {
			out = append(out, "$ "+name+": "+v[2:len(v)-1])
		} else {
			out = append(out, "= "+name+": "+v)
		}
	}
	return out
}

func Additions(list []*Cheat, arsenal bool) []Addition {
	out := make([]Addition, 0, len(list))
	for _, c := range list {
		tags := c.TagItems()
		if arsenal {
			tags = append(tags, SourceTag)
		}
		group := c.Tool()
		if group == "" {
			group = SectionOf(c.Command)
		}
		out = append(out, Addition{Group: group, Entry: Entry{
			Title:       strings.TrimSpace(c.Name),
			Command:     commandLines(c),
			Description: c.Description,
			Tags:        strings.Join(tags, ", "),
			Vars:        varLines(c),
		}})
	}
	return out
}

func markdownFiles(dir string) []string {
	var files []string
	_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != dir && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !ExcludedFiles[d.Name()] && strings.EqualFold(filepath.Ext(p), ".md") {
			files = append(files, p)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func Count(dir string) (arsenal, own int) {
	for _, f := range markdownFiles(dir) {
		list, err := parseFile(f)
		if err != nil {
			continue
		}
		for _, c := range list {
			if c.FromArsenal() {
				arsenal++
			} else {
				own++
			}
		}
	}
	return
}

type RemoveResult struct {
	Removed int
	Kept    int
}

func RemoveArsenal(dir string) (RemoveResult, error) {
	var res RemoveResult
	for _, f := range markdownFiles(dir) {
		raw, err := os.ReadFile(f)
		if err != nil {
			return res, err
		}
		list, err := ParseMarkdown(f, strings.NewReader(string(raw)))
		if err != nil {
			continue
		}
		drop := map[int]bool{}
		for _, c := range list {
			if c.FromArsenal() && c.Level == 2 && c.Line > 0 {
				drop[c.Line-1] = true
			}
		}
		res.Kept += len(list) - len(drop)
		if len(drop) == 0 {
			continue
		}

		d := loadDoc(string(raw))
		starts, start := make([]int, len(d.blocks)), 0
		for i, b := range d.blocks {
			starts[i] = start
			start += len(b.lines)
		}
		var kept []mdBlock
		for i, b := range d.blocks {
			if drop[starts[i]] && b.isEntry() {
				res.Removed++
				continue
			}
			kept = append(kept, b)
		}
		d.blocks = kept
		for i := len(d.blocks) - 1; i >= 0; i-- {
			d.dropIfEmptySection(i)
		}
		d.fixTags()
		text := d.text()
		if onlyTitles(text) {
			if err := os.Remove(f); err != nil {
				return res, err
			}
			continue
		}
		fi, err := os.Stat(f)
		if err != nil {
			return res, err
		}
		if err := fsutil.WriteFileAtomic(f, []byte(text), fi.Mode().Perm()); err != nil {
			return res, err
		}
	}
	return res, nil
}

func onlyTitles(text string) bool {
	empty := true
	walkLines(strings.Split(text, "\n"), func(_ int, l string, _ bool) {
		if strings.TrimSpace(l) != "" && !strings.HasPrefix(l, "# ") {
			empty = false
		}
	})
	return empty && !strings.Contains(text, "```")
}
