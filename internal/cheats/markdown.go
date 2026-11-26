// Parses Markdown cheatsheets in the upstream Arsenal format.

package cheats

import (
	"bufio"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
)

type ParseError struct {
	File string
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d: %s", e.File, e.Line, e.Msg)
	}
	return fmt.Sprintf("%s: %s", e.File, e.Msg)
}

var upstreamTagLeak = false

var cmdTagRe = regexp.MustCompile(`#([^\s#]+)`)

type mdParser struct {
	file string

	titles     []string
	firstTitle string
	level      int
	fileTags   string
	refTags    CmdTags
	vars       map[string]string

	cur       Cheat
	ownTags   CmdTags
	capturing bool
	inComment bool

	list []*Cheat
}

func (p *mdParser) isDone() bool {
	return p.cur.Name != "" && p.cur.Command != "" && !p.capturing
}

func (p *mdParser) newCheat() {
	p.cur = Cheat{}
	p.ownTags = nil
	if n := len(p.titles); n > 0 {
		p.cur.Name = p.titles[n-1]
	}
}

func (p *mdParser) endCheat() {
	c := p.cur
	c.Tags = p.fileTags
	if n := len(p.titles); n > 1 {
		c.Title = strings.Join(p.titles[:n-1], ", ")
	}
	if c.Title == "" {
		c.Title = p.firstTitle
	}
	tags := p.refTags.Clone()
	for _, t := range p.ownTags {
		tags.Set(t.Key, t.Value)
	}
	c.CommandTags = tags
	p.list = append(p.list, &c)
}

func (p *mdParser) descriptionAsCommand() bool {
	if p.cur.Command == "" && p.cur.Description != "" {
		p.cur.Command = strings.ReplaceAll(p.cur.Description, "\n", ";\\\n")
		p.cur.Description = ""
		p.endCheat()
		return true
	}
	return false
}

func ParseMarkdown(filename string, r io.Reader) ([]*Cheat, error) {
	p := &mdParser{file: filename, vars: map[string]string{}}
	p.newCheat()

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	n := 0
	for sc.Scan() {
		n++
		line := strings.TrimRight(sc.Text(), "\r")

		if p.isDone() {
			p.endCheat()
			p.newCheat()
		}

		if !p.capturing {
			if p.inComment {
				if strings.Contains(line, "-->") {
					p.inComment = false
				}
				continue
			}
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "<!--") {
				p.inComment = !strings.Contains(t[4:], "-->")
				continue
			}
		}

		if strings.HasPrefix(line, "```") {
			if p.cur.Name == "" {
				return nil, &ParseError{filename, n, "code block outside of a titled entry"}
			}
			p.capturing = !p.capturing
			continue
		}
		if p.capturing {
			if strings.TrimSpace(line) == "" {
				continue
			}
			line = strings.TrimRightFunc(line, unicode.IsSpace)
			switch {
			case p.cur.Command == "":
				p.cur.Command = line
			case strings.HasSuffix(p.cur.Command, ";"):
				p.cur.Command += "\\\n" + line
			default:
				p.cur.Command += ";\\\n" + line
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "%"):
			p.fileTags = strings.TrimSpace(line[1:])

		case strings.HasPrefix(line, "# ") || strings.HasPrefix(line, "##"):
			p.descriptionAsCommand()
			level := 0
			rest := line
			for strings.HasPrefix(rest, "#") {
				level++
				rest = rest[1:]
			}
			title := strings.TrimLeftFunc(rest, unicode.IsSpace)
			if p.firstTitle == "" {
				p.firstTitle = title
			}
			switch {
			case level == len(p.titles)+1:
				p.titles = append(p.titles, title)
			case level == len(p.titles):
				p.titles[len(p.titles)-1] = title
			case level < len(p.titles):
				p.titles = append(p.titles[:level-1], title)
			default:
				return nil, &ParseError{filename, n, fmt.Sprintf("title level skipped (level %d under level %d): add the missing level", level, len(p.titles))}
			}
			p.level = level
			p.newCheat()
			p.cur.Line, p.cur.Level = n, level

		case len(line) > 1 && line[0] == '#' && line[1] != '#' && line[1] != ' ' && line[1] != '\t':
			for _, m := range cmdTagRe.FindAllStringSubmatch(line, -1) {
				key, value, ok := strings.Cut(m[1], "/")
				if !ok {
					continue
				}
				if p.level <= 1 || upstreamTagLeak {
					p.refTags.Set(key, value)
				} else {
					p.ownTags.Set(key, value)
				}
			}

		case (strings.HasPrefix(line, "$") || strings.HasPrefix(line, "=")) && isVarLine(line):
			name, val, _ := strings.Cut(line[2:], ":")
			name, val = strings.TrimSpace(name), strings.TrimSpace(val)
			if line[0] == '$' {
				val = "$(" + val + ")"
			}
			p.vars[name] = val

		case strings.TrimSpace(line) != "":
			if p.cur.Description == "" {
				p.cur.Description = strings.TrimRightFunc(line, unicode.IsSpace)
			} else {
				p.cur.Description += "\n" + strings.TrimRightFunc(line, unicode.IsSpace)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return nil, &ParseError{filename, 0, err.Error()}
	}
	p.capturing = false
	if p.isDone() {
		p.endCheat()
	} else {
		p.descriptionAsCommand()
	}
	return finish(p.list, filename, p.vars), nil
}

func isVarLine(line string) bool {
	if len(line) < 3 {
		return false
	}
	name, _, ok := strings.Cut(line[2:], ":")
	return ok && strings.TrimSpace(name) != "" && !strings.ContainsAny(strings.TrimSpace(name), " \t")
}
