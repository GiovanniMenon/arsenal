// Loads every cheatsheet found in the cheats folders and collects the files that fail.

package cheats

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ExcludedFiles = map[string]bool{"README.md": true, "README.rst": true, "index.rst": true}

type Result struct {
	Cheats   []*Cheat
	Warnings []error
}

func Load(paths []string) Result {
	var res Result
	index := map[string]int{}

	add := func(list []*Cheat) {
		for _, c := range list {
			if i, ok := index[c.Key()]; ok {
				res.Cheats[i] = c
			} else {
				index[c.Key()] = len(res.Cheats)
				res.Cheats = append(res.Cheats, c)
			}
		}
	}

	for _, root := range paths {
		if r, err := filepath.EvalSymlinks(root); err == nil {
			root = r
		}
		var files []string
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if p != root && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				return nil
			}
			if ExcludedFiles[d.Name()] {
				return nil
			}
			switch strings.ToLower(filepath.Ext(p)) {
			case ".md", ".yml":
				files = append(files, p)
			}
			return nil
		})
		sort.Strings(files)
		for _, f := range files {
			list, err := parseFile(f)
			if err != nil {
				res.Warnings = append(res.Warnings, err)
				continue
			}
			add(list)
		}
	}
	return res
}

func parseFile(path string) ([]*Cheat, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if strings.EqualFold(filepath.Ext(path), ".yml") {
		return ParseYAML(path, f)
	}
	return ParseMarkdown(path, f)
}
