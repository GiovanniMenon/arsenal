// The huh theme, built from the shared palette.

package tui

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

func Theme() *huh.Theme {
	t := huh.ThemeBase()
	bar := lipgloss.Border{Left: "|"}
	t.Focused.Base = lipgloss.NewStyle().PaddingLeft(1).BorderStyle(bar).BorderLeft(true).BorderForeground(ColorPrimary)
	t.Blurred.Base = lipgloss.NewStyle().PaddingLeft(1).BorderStyle(lipgloss.Border{Left: " "}).BorderLeft(true)

	t.Focused.Title = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	t.Focused.NoteTitle = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary).MarginBottom(1)
	t.Focused.Description = MutedStyle
	t.Focused.ErrorIndicator = ErrorStyle.SetString(" *")
	t.Focused.ErrorMessage = ErrorStyle
	t.Focused.TextInput.Prompt = KeyStyle
	t.Focused.TextInput.Placeholder = MutedStyle
	t.Focused.TextInput.Cursor = KeyStyle
	t.Focused.FocusedButton = SelectedRowStyle.Padding(0, 2).MarginRight(1)
	t.Focused.BlurredButton = MutedStyle.Padding(0, 2).MarginRight(1)

	t.Blurred.Title = MutedStyle
	t.Blurred.NoteTitle = MutedStyle
	t.Blurred.Description = MutedStyle
	t.Blurred.TextInput.Prompt = MutedStyle
	t.Blurred.TextInput.Placeholder = MutedStyle
	return t
}
