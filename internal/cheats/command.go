// Fills the <variables> of a command with saved values and defaults, and builds the final line.

package cheats

import (
	"fmt"
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`<([^ <>]+)>`)

type ValueSource int

const (
	SourceNone ValueSource = iota
	SourceDefault
	SourceFile
	SourceSaved
)

type Arg struct {
	Name   string
	Value  string
	Source ValueSource
}

type Command struct {
	Cheat    *Cheat
	Template string
	Args     []Arg
}

func NewCommand(c *Cheat, saved map[string]string) *Command {
	cmd := &Command{Cheat: c}
	index := map[string]int{}
	defaults := map[string]string{}

	cmd.Template = placeholderRe.ReplaceAllStringFunc(c.Command, func(m string) string {
		inner := m[1 : len(m)-1]
		name, def, hasDef := strings.Cut(inner, "|")
		if name == "" {
			return m
		}
		if hasDef {
			def, _, _ = strings.Cut(def, "|")
			if _, seen := defaults[name]; !seen && def != "" {
				defaults[name] = def
			}
		}
		if _, ok := index[name]; !ok {
			index[name] = len(cmd.Args)
			cmd.Args = append(cmd.Args, Arg{Name: name})
		}
		return "<" + name + ">"
	})

	for i := range cmd.Args {
		a := &cmd.Args[i]
		switch {
		case saved[a.Name] != "":
			a.Value, a.Source = saved[a.Name], SourceSaved
		case defaults[a.Name] != "":
			a.Value, a.Source = defaults[a.Name], SourceDefault
		case c.Variables[a.Name] != "":
			a.Value, a.Source = c.Variables[a.Name], SourceFile
		}
	}
	return cmd
}

func (c *Command) NeedsInput() []int {
	var idx []int
	for i, a := range c.Args {
		if a.Source != SourceSaved {
			idx = append(idx, i)
		}
	}
	return idx
}

func (c *Command) Set(name, value string) {
	for i := range c.Args {
		if c.Args[i].Name == name {
			c.Args[i].Value = value
			return
		}
	}
}

func (c *Command) Missing() []string {
	var m []string
	for _, a := range c.Args {
		if a.Value == "" {
			m = append(m, a.Name)
		}
	}
	return m
}

func (c *Command) render(empty func(name string) string) string {
	values := map[string]string{}
	for _, a := range c.Args {
		values[a.Name] = a.Value
	}
	return placeholderRe.ReplaceAllStringFunc(c.Template, func(m string) string {
		name := m[1 : len(m)-1]
		if v, ok := values[name]; ok {
			if v == "" {
				return empty(name)
			}
			return v
		}
		return m
	})
}

func (c *Command) Build() (string, error) {
	if m := c.Missing(); len(m) > 0 {
		return "", fmt.Errorf("missing value for %s", strings.Join(m, ", "))
	}
	return c.render(func(string) string { return "" }), nil
}

func (c *Command) Preview() string {
	return c.render(func(name string) string { return "<" + name + ">" })
}

type Segment struct {
	Text string
	Arg  int
}

func (c *Command) Segments() []Segment {
	index := map[string]int{}
	for i, a := range c.Args {
		index[a.Name] = i
	}
	var segs []Segment
	last := 0
	for _, loc := range placeholderRe.FindAllStringSubmatchIndex(c.Template, -1) {
		name := c.Template[loc[2]:loc[3]]
		i, ok := index[name]
		if !ok {
			continue
		}
		if loc[0] > last {
			segs = append(segs, Segment{Text: c.Template[last:loc[0]], Arg: -1})
		}
		segs = append(segs, Segment{Text: name, Arg: i})
		last = loc[1]
	}
	if last < len(c.Template) {
		segs = append(segs, Segment{Text: c.Template[last:], Arg: -1})
	}
	return segs
}

func Prefixed(cmdline, prefix string) string {
	if prefix == "" {
		return cmdline
	}
	return prefix + " " + cmdline
}

const PrefixVar = "arsenal_prefix_cmd"
