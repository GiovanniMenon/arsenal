// Compares the parser with what upstream Arsenal reads from the same cheatsheets.

package cheats

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestUpstreamParity(t *testing.T) {
	dir, golden := os.Getenv("ARSENAL_UPSTREAM_CHEATS"), os.Getenv("ARSENAL_UPSTREAM_GOLDEN")
	if dir == "" || golden == "" {
		t.Skip("set ARSENAL_UPSTREAM_CHEATS and ARSENAL_UPSTREAM_GOLDEN to run")
	}
	raw, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	var want []struct {
		Key         string      `json:"key"`
		Name        string      `json:"name"`
		Title       string      `json:"title"`
		Tags        string      `json:"tags"`
		CommandTags [][2]string `json:"command_tags"`
		Command     string      `json:"command"`
		Description string      `json:"description"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}

	upstreamTagLeak = true
	defer func() { upstreamTagLeak = false }()
	res := Load([]string{dir})
	for _, w := range res.Warnings {
		t.Errorf("warning: %v", w)
	}
	got := map[string]*Cheat{}
	for _, c := range res.Cheats {
		got[c.Key()] = c
	}
	known := map[string]bool{}
	for _, w := range want {
		known[w.Key] = true
	}
	extra := 0
	for k := range got {
		if !known[k] {
			extra++
			t.Logf("extra cheat not loaded by upstream: %q", k)
		}
	}
	if extra > 1 {
		t.Errorf("%d cheats upstream did not load, want at most 1", extra)
	}
	diffs := 0
	for _, w := range want {
		c, ok := got[w.Key]
		if !ok {
			t.Errorf("missing cheat %q", w.Key)
			diffs++
			continue
		}
		var tags [][2]string
		for _, x := range c.CommandTags {
			tags = append(tags, [2]string{x.Key, x.Value})
		}
		bad := func(field string, g, u any) {
			diffs++
			if diffs <= 15 {
				t.Errorf("%q %s:\n  got      %#v\n  upstream %#v", w.Key, field, g, u)
			}
		}
		if c.Name != w.Name {
			bad("name", c.Name, w.Name)
		}
		if c.Title != w.Title {
			bad("title", c.Title, w.Title)
		}
		if c.Tags != w.Tags {
			bad("tags", c.Tags, w.Tags)
		}
		if c.Command != w.Command {
			bad("command", c.Command, w.Command)
		}
		if c.Description != w.Description {
			bad("description", c.Description, w.Description)
		}
		if !reflect.DeepEqual(tags, w.CommandTags) && !(len(tags) == 0 && len(w.CommandTags) == 0) {
			bad("command_tags", tags, w.CommandTags)
		}
	}
	t.Logf("%d upstream cheats compared, %d field differences", len(want), diffs)
}

func TestUpstreamArgumentsParity(t *testing.T) {
	dir, argsFile := os.Getenv("ARSENAL_UPSTREAM_CHEATS"), os.Getenv("ARSENAL_UPSTREAM_ARGS")
	if dir == "" || argsFile == "" {
		t.Skip("set ARSENAL_UPSTREAM_CHEATS and ARSENAL_UPSTREAM_ARGS to run")
	}
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]struct {
		Names   []string `json:"names"`
		Cmdline string   `json:"cmdline"`
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	upstreamTagLeak = true
	defer func() { upstreamTagLeak = false }()
	checked, bad := 0, 0
	for _, c := range Load([]string{dir}).Cheats {
		w, ok := want[c.Key()]
		if !ok {
			continue
		}
		checked++
		cmd := NewCommand(c, nil)
		var names []string
		for _, a := range cmd.Args {
			names = append(names, a.Name)
		}
		if !reflect.DeepEqual(names, w.Names) && !(len(names) == 0 && len(w.Names) == 0) {
			bad++
			if bad <= 10 {
				t.Errorf("%q args: got %v, upstream %v", c.Key(), names, w.Names)
			}
		}
		if cmd.Template != w.Cmdline {
			bad++
			if bad <= 10 {
				t.Errorf("%q template:\n  got      %q\n  upstream %q", c.Key(), cmd.Template, w.Cmdline)
			}
		}
	}
	t.Logf("%d cheats checked, %d differences", checked, bad)
}
