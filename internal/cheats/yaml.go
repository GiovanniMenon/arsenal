// Parses YAML cheatsheets in the upstream Arsenal format.

package cheats

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

func ParseYAML(filename string, r io.Reader) ([]*Cheat, error) {
	var root yaml.Node
	if err := yaml.NewDecoder(r).Decode(&root); err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, &ParseError{filename, 0, err.Error()}
	}
	doc := &root
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		doc = doc.Content[0]
	}
	if doc.Kind != yaml.MappingNode {
		return nil, &ParseError{filename, doc.Line, "top level must be a mapping of tools"}
	}

	vars := map[string]string{}
	var list []*Cheat

	for i := 0; i+1 < len(doc.Content); i += 2 {
		tool := doc.Content[i+1]
		if tool.Kind != yaml.MappingNode {
			continue
		}
		fields := mapping(tool)
		nameNode, hasName := fields["name"]
		cmdsNode, hasCmds := fields["cmds"]
		if !hasName || !hasCmds {
			continue
		}
		var toolName string
		if err := nameNode.Decode(&toolName); err != nil {
			return nil, &ParseError{filename, nameNode.Line, "name must be text"}
		}

		var tags []string
		if n, ok := fields["tags"]; ok {
			if err := n.Decode(&tags); err != nil {
				return nil, &ParseError{filename, n.Line, "tags must be a list"}
			}
		}
		for _, key := range []string{"const_variables", "exec_variables"} {
			n, ok := fields[key]
			if !ok {
				continue
			}
			var items []map[string]any
			if err := n.Decode(&items); err != nil {
				return nil, &ParseError{filename, n.Line, key + " must be a list of {name: value}"}
			}
			for _, item := range items {
				for k, v := range item {
					val := fmt.Sprint(v)
					if key == "exec_variables" {
						val = "$(" + val + ")"
					}
					vars[k] = val
				}
			}
		}
		var refTags CmdTags
		if n, ok := fields["command_tags"]; ok {
			var raw []string
			if err := n.Decode(&raw); err != nil {
				return nil, &ParseError{filename, n.Line, "command_tags must be a list"}
			}
			refTags = tagsFromList(raw)
		}

		if cmdsNode.Kind != yaml.MappingNode {
			return nil, &ParseError{filename, cmdsNode.Line, "cmds must be a mapping"}
		}
		for j := 0; j+1 < len(cmdsNode.Content); j += 2 {
			cf := mapping(cmdsNode.Content[j+1])
			name, cmd := scalar(cf["name"]), scalar(cf["cmd"])
			if name == "" || cmd == "" {
				return nil, &ParseError{filename, cmdsNode.Content[j].Line, "command needs name and cmd"}
			}
			tags2 := refTags.Clone()
			if n, ok := cf["command_tags"]; ok {
				var raw []string
				if err := n.Decode(&raw); err != nil {
					return nil, &ParseError{filename, n.Line, "command_tags must be a list"}
				}
				for _, t := range tagsFromList(raw) {
					tags2.Set(t.Key, t.Value)
				}
			}
			list = append(list, &Cheat{
				Name:        name,
				Title:       toolName,
				Tags:        strings.Join(tags, ", "),
				CommandTags: tags2,
				Command:     cmd,
				Description: scalar(cf["description"]),
			})
		}
	}
	return finish(list, filename, vars), nil
}

func mapping(n *yaml.Node) map[string]*yaml.Node {
	m := map[string]*yaml.Node{}
	if n == nil || n.Kind != yaml.MappingNode {
		return m
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		m[n.Content[i].Value] = n.Content[i+1]
	}
	return m
}

func scalar(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
}

func tagsFromList(raw []string) CmdTags {
	var t CmdTags
	for _, s := range raw {
		if k, v, ok := strings.Cut(s, "/"); ok {
			t.Set(k, v)
		}
	}
	return t
}
