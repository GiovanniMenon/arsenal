// Package tui has the screens, prompts and styles of ars.
// This file defines the color palette and the text styles.
package tui

import "github.com/charmbracelet/lipgloss"

var (
	ColorPrimary = lipgloss.AdaptiveColor{Light: "#5B3FD9", Dark: "#9D8CFF"}
	ColorAccent  = lipgloss.AdaptiveColor{Light: "#0B7A75", Dark: "#4FD1C5"}
	ColorSuccess = lipgloss.AdaptiveColor{Light: "#1A7F37", Dark: "#56D364"}
	ColorWarn    = lipgloss.AdaptiveColor{Light: "#9A6700", Dark: "#E3B341"}
	ColorError   = lipgloss.AdaptiveColor{Light: "#CF222E", Dark: "#FF7B72"}
	ColorMuted   = lipgloss.AdaptiveColor{Light: "#6E7781", Dark: "#8B949E"}
	ColorInk     = lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#1B1B1F"}
)

var (
	TitleStyle   = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	HeadingStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	KeyStyle     = lipgloss.NewStyle().Foreground(ColorAccent)
	ValueStyle   = lipgloss.NewStyle()
	MutedStyle   = lipgloss.NewStyle().Foreground(ColorMuted)
	ActiveStyle  = lipgloss.NewStyle().Bold(true).Foreground(ColorSuccess)
	SuccessStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
	WarnStyle    = lipgloss.NewStyle().Foreground(ColorWarn)
	ErrorStyle   = lipgloss.NewStyle().Foreground(ColorError)
)

var (
	SelectedRowStyle = lipgloss.NewStyle().Bold(true).
				Foreground(ColorInk).
				Background(ColorPrimary)
	HelpKeyStyle = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
	ChipOnStyle  = lipgloss.NewStyle().Bold(true).
			Foreground(ColorInk).
			Background(ColorSuccess).Padding(0, 1)
	ChipWarnStyle = lipgloss.NewStyle().Bold(true).
			Foreground(ColorInk).
			Background(ColorWarn).Padding(0, 1)
	ChipOffStyle = lipgloss.NewStyle().Foreground(ColorMuted).Padding(0, 1)
)

func Title(s string) string { return TitleStyle.Render(s) }

func Heading(s string) string { return HeadingStyle.Render(s) }

func Muted(s string) string { return MutedStyle.Render(s) }

func Success(s string) string { return SuccessStyle.Render(s) }

func Warn(s string) string { return WarnStyle.Render(s) }

func Error(s string) string { return ErrorStyle.Render("error: " + s) }

func Info(s string) string { return MutedStyle.Render(s) }
