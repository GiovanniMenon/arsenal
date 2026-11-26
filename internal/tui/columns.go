// Lays out the columns of the search list.

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/ynn3m/arsenal/internal/cheats"
)

const (
	maxToolW    = 16
	maxTitleW   = 60
	maxTagsW    = 26
	minCmdW     = 24
	minLabelW   = 6
	colGap      = 2
	listTagsMax = 3
)

type columnStats struct {
	tool, title, tags, cmd int
}

func measure(list []*cheats.Cheat, showTags bool) columnStats {
	var s columnStats
	for _, c := range list {
		s.tool = max(s.tool, ansi.StringWidth(c.Tool()))
		s.title = max(s.title, ansi.StringWidth(strings.TrimSpace(c.Name)))
		if showTags {
			s.tags = max(s.tags, ansi.StringWidth(rowTags(c)))
		}
		s.cmd = max(s.cmd, ansi.StringWidth(rowCommand(c)))
	}
	return s
}

type rowLayout struct{ tool, title, tags, cmd int }

func (s columnStats) layout(avail int) rowLayout {
	l := s.fit(avail)
	cols := []struct {
		w      *int
		weight int
	}{{&l.tool, 2}, {&l.title, 3}, {&l.tags, 2}, {&l.cmd, 4}}
	total, shown, weights := 0, 0, 0
	for _, c := range cols {
		if *c.w > 0 {
			total += *c.w
			shown++
			weights += c.weight
		}
	}
	slack := avail - total - (shown-1)*colGap
	if slack <= 0 {
		return l
	}
	given, last := 0, (*int)(nil)
	for _, c := range cols {
		if *c.w > 0 {
			extra := slack * c.weight / weights
			*c.w += extra
			given += extra
			last = c.w
		}
	}
	*last += slack - given
	return l
}

func (s columnStats) fit(avail int) rowLayout {
	l := rowLayout{tool: min(s.tool, maxToolW), title: min(s.title, maxTitleW), tags: min(s.tags, maxTagsW)}
	if l.tags > 0 && avail-l.tool-l.title-l.tags-3*colGap < minCmdW {
		l.tags = 0
	}
	cols := 3
	if l.tags > 0 {
		cols = 4
	}
	rest := avail - l.tool - l.title - l.tags - (cols-1)*colGap
	if rest < minCmdW {
		deficit := minCmdW - rest
		cut := min(deficit, max(l.title-minLabelW, 0))
		l.title -= cut
		deficit -= cut
		l.tool -= min(deficit, max(l.tool-minLabelW, 0))
		rest = avail - l.tool - l.title - 2*colGap
	}
	l.cmd = max(min(s.cmd, rest), 1)
	return l
}

func rowTags(c *cheats.Cheat) string {
	var tags []string
	for _, t := range c.TagList() {
		if !strings.EqualFold(t, cheats.SourceTag) {
			tags = append(tags, t)
		}
	}
	return strings.Join(tags[:min(len(tags), listTagsMax)], ", ")
}

func rowCommand(c *cheats.Cheat) string {
	return strings.Join(strings.Fields(c.PrintableCommand()), " ")
}

func cell(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	if ansi.StringWidth(s) > width {
		s = ansi.Truncate(s, width, "...")
	}
	return s + strings.Repeat(" ", max(0, width-ansi.StringWidth(s)))
}

var (
	toolStyle  = lipgloss.NewStyle().Foreground(ColorAccent)
	titleStyle = lipgloss.NewStyle().Bold(true)
)

func renderRow(c *cheats.Cheat, selected bool, width int, st columnStats) string {
	l := st.layout(width - 3)
	type col struct {
		text  string
		w     int
		style lipgloss.Style
	}
	cols := []col{
		{c.Tool(), l.tool, toolStyle},
		{strings.TrimSpace(c.Name), l.title, titleStyle},
	}
	if l.tags > 0 {
		cols = append(cols, col{rowTags(c), l.tags, MutedStyle})
	}
	cols = append(cols, col{rowCommand(c), l.cmd, ValueStyle})

	gap := strings.Repeat(" ", colGap)
	var b strings.Builder
	for i, col := range cols {
		if i > 0 {
			b.WriteString(gap)
		}
		if selected {
			b.WriteString(cell(col.text, col.w))
		} else {
			b.WriteString(col.style.Render(cell(col.text, col.w)))
		}
	}
	line := ansi.Truncate(b.String(), max(width-3, 0), "")
	if selected {
		return SelectedRowStyle.Render("-> " + cell(line, width-3))
	}
	return "   " + line
}
