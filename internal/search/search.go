// Package search filters the command list as you type, like the upstream search box.
package search

import (
	"strings"
	"unicode"

	"github.com/sahilm/fuzzy"

	"github.com/ynn3m/arsenal/internal/cheats"
)

const minFuzzyLen = 3

type haystack struct {
	fields []string
	words  []string
	tags   []string
}

func newHaystack(c *cheats.Cheat) haystack {
	h := haystackOf(c.Title, c.Name, c.Tags, c.CommandTags.Values(), c.Command, c.Description)
	for _, t := range c.TagList() {
		h.tags = append(h.tags, strings.ToLower(t))
	}
	return h
}

func haystackOf(fields ...string) haystack {
	h := haystack{fields: make([]string, len(fields))}
	for i, f := range fields {
		h.fields[i] = strings.ToLower(f)
	}
	seen := map[string]bool{}
	for _, f := range h.fields {
		for _, w := range strings.FieldsFunc(f, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if !seen[w] {
				seen[w] = true
				h.words = append(h.words, w)
			}
		}
	}
	return h
}

func (h haystack) contains(term string) bool {
	for _, f := range h.fields {
		if strings.Contains(f, term) {
			return true
		}
	}
	return false
}

func (h haystack) hasTag(prefix string) bool {
	for _, t := range h.tags {
		if strings.HasPrefix(t, prefix) {
			return true
		}
	}
	return false
}

func (h haystack) fuzzy(term string) bool {
	if len([]rune(term)) < minFuzzyLen {
		return false
	}
	limit := 2*len([]rune(term)) + 1
	var words []string
	for _, w := range h.words {
		if len([]rune(w)) <= limit {
			words = append(words, w)
		}
	}
	return len(fuzzy.Find(term, words)) > 0
}

type Index struct {
	cheats []*cheats.Cheat
	hay    []haystack
}

func NewIndex(list []*cheats.Cheat) *Index {
	ix := &Index{cheats: list, hay: make([]haystack, len(list))}
	for i, c := range list {
		ix.hay[i] = newHaystack(c)
	}
	return ix
}

func (ix *Index) Filter(input string) []*cheats.Cheat {
	input = strings.ToLower(input)
	terms := strings.Fields(input)
	if len(terms) == 0 {
		return ix.cheats
	}
	var out []*cheats.Cheat
	for _, i := range matchAll(terms, ix.hay, true) {
		out = append(out, ix.cheats[i])
	}
	return out
}

func isExclude(term string) bool { return strings.HasPrefix(term, "!") && len(term) > 1 }

func tagTerm(term string) (tag string, ok bool) {
	return strings.CutPrefix(term, "#")
}

func matchAll(terms []string, hay []haystack, tags bool) []int {
	var exact, loose []int
	for i, h := range hay {
		ok, usedFuzzy := true, false
		for _, term := range terms {
			if tags {
				neg := isExclude(term)
				t := term
				if neg {
					t = term[1:]
				}
				if tag, isTag := tagTerm(t); isTag {
					if tag == "" {
						continue
					}
					if h.hasTag(tag) == neg {
						ok = false
						break
					}
					continue
				}
			}
			if isExclude(term) {
				if h.contains(term[1:]) {
					ok = false
					break
				}
				continue
			}
			if h.contains(term) {
				continue
			}
			if h.fuzzy(term) {
				usedFuzzy = true
				continue
			}
			ok = false
			break
		}
		if !ok {
			continue
		}
		if usedFuzzy {
			loose = append(loose, i)
		} else {
			exact = append(exact, i)
		}
	}
	return append(exact, loose...)
}

type TextIndex struct{ hay []haystack }

func NewTextIndex(docs [][]string) *TextIndex {
	t := &TextIndex{hay: make([]haystack, len(docs))}
	for i, d := range docs {
		t.hay[i] = haystackOf(d...)
	}
	return t
}

func (t *TextIndex) Filter(input string) []int {
	terms := strings.Fields(strings.ToLower(input))
	if len(terms) == 0 {
		all := make([]int, len(t.hay))
		for i := range all {
			all[i] = i
		}
		return all
	}
	return matchAll(terms, t.hay, false)
}

func Terms(input string) []string {
	var out []string
	for _, t := range strings.Fields(strings.ToLower(input)) {
		if !isExclude(t) {
			out = append(out, t)
		}
	}
	return out
}

func (ix *Index) CommonPrefix(input string) string {
	if input == "" {
		return input
	}
	var prefix string
	found := false
	for _, c := range ix.cheats {
		if !strings.HasPrefix(c.Command, input) {
			continue
		}
		if !found {
			prefix, found = c.Command, true
			continue
		}
		prefix = commonPrefix(prefix, c.Command)
	}
	if !found {
		return input
	}
	return prefix
}

func commonPrefix(a, b string) string {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return string(ra[:n])
}
