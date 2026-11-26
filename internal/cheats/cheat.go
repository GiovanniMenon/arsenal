// Package cheats loads Arsenal cheatsheets and turns an entry into a ready command.
// This file has the Cheat type and its tags.
package cheats

import (
	"strings"
)

type Tag struct{ Key, Value string }

type CmdTags []Tag

func (t CmdTags) Get(key string) (string, bool) {
	for _, x := range t {
		if x.Key == key {
			return x.Value, true
		}
	}
	return "", false
}

func (t *CmdTags) Set(key, value string) {
	for i := range *t {
		if (*t)[i].Key == key {
			(*t)[i].Value = value
			return
		}
	}
	*t = append(*t, Tag{key, value})
}

func (t CmdTags) Clone() CmdTags { return append(CmdTags(nil), t...) }

func (t CmdTags) Values() string {
	var b strings.Builder
	for _, x := range t {
		b.WriteString(x.Value)
	}
	return b.String()
}

type Cheat struct {
	Name        string
	Title       string
	Tags        string
	CommandTags CmdTags
	Command     string
	Description string
	Variables   map[string]string
	Filename    string
	Line, Level int
}

func (c *Cheat) Key() string { return c.Title + c.Name }

func (c *Cheat) PrintableCommand() string { return strings.ReplaceAll(c.Command, "\\\n", "") }

const (
	SourceKey     = "src"
	SourceArsenal = "arsenal"
	SourceTag     = SourceKey + "/" + SourceArsenal
)

func (c *Cheat) FromArsenal() bool {
	v, ok := c.CommandTags.Get(SourceKey)
	return ok && v == SourceArsenal
}

func (c *Cheat) Tool() string { return strings.TrimSpace(c.Title) }

func (c *Cheat) TagItems() []string {
	var out []string
	seen := map[string]bool{}
	add := func(t string) {
		t = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(t), "#"))
		if k := strings.ToLower(t); t != "" && !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
	}
	for _, t := range strings.Split(c.Tags, ",") {
		add(t)
	}
	for _, t := range c.CommandTags {
		add(t.Key + "/" + t.Value)
	}
	return out
}

func (c *Cheat) TagList() []string {
	items := c.TagItems()
	for i, t := range items {
		items[i] = strings.Join(strings.Fields(t), "-")
	}
	return items
}

func finish(list []*Cheat, filename string, vars map[string]string) []*Cheat {
	for _, c := range list {
		c.Filename = filename
		c.Variables = map[string]string{}
		for k, v := range vars {
			c.Variables[k] = v
		}
	}
	return list
}
