// Rounded boxes with their labels in the top border.

package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type panel struct {
	left, right string
	focused     bool
}

func (p panel) color() lipgloss.AdaptiveColor {
	if p.focused {
		return ColorPrimary
	}
	return ColorMuted
}

func (p panel) render(content string, width int) string {
	b := lipgloss.RoundedBorder()
	line := lipgloss.NewStyle().Foreground(p.color())

	inner := width - 2
	l, r := p.left, p.right
	if l != "" {
		l = " " + l + " "
	}
	if r != "" {
		r = " " + r + " "
	}
	room := inner - 2
	if ansi.StringWidth(l)+ansi.StringWidth(r) > room {
		r = ansi.Truncate(r, max(0, room-ansi.StringWidth(l)), "...")
	}
	if ansi.StringWidth(l) > room {
		l = ansi.Truncate(l, max(0, room), "...")
	}
	fill := max(0, room-ansi.StringWidth(l)-ansi.StringWidth(r))
	top := line.Render(b.TopLeft+b.Top) + l + line.Render(strings.Repeat(b.Top, fill)) + r + line.Render(b.Top+b.TopRight)

	body := lipgloss.NewStyle().
		Border(b, false, true, true, true).
		BorderForeground(p.color()).
		Padding(0, 1).
		Width(width - 2).
		Render(content)
	return top + "\n" + body
}

func Box(title, content string) string {
	w := ansi.StringWidth(title) + 8
	for _, l := range strings.Split(content, "\n") {
		w = max(w, ansi.StringWidth(l)+4)
	}
	return panel{left: Title(title), focused: true}.render(content, w)
}
