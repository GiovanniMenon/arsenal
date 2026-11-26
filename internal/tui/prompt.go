// Asks a single question, as a line of text, a yes or no, or a choice from a list.

package tui

import (
	"errors"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
)

var ErrCancelled = errors.New("cancelled")

func IsInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd())
}

func run(f *huh.Form) error {
	in, out, closeTTY := TTY()
	defer closeTTY()
	err := f.WithTheme(Theme()).WithShowHelp(false).WithInput(in).WithOutput(out).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return ErrCancelled
	}
	return err
}

func AskText(title, description, placeholder string, validate func(string) error) (string, error) {
	var value string
	in := huh.NewInput().
		Title(title).
		Description(description + " (enter confirms, ctrl+c cancels)").
		Placeholder(placeholder).
		Value(&value)
	if validate != nil {
		in = in.Validate(validate)
	}
	if err := run(huh.NewForm(huh.NewGroup(in))); err != nil {
		return "", err
	}
	return value, nil
}

func Confirm(title, description string) (bool, error) {
	yes := true
	c := huh.NewConfirm().
		Title(title).
		Description(description + " (left/right or y/n, enter confirms)").
		Affirmative("Yes").
		Negative("No").
		Value(&yes)
	if err := run(huh.NewForm(huh.NewGroup(c))); err != nil {
		return false, err
	}
	return yes, nil
}

func ConfirmNo(title, description string) (bool, error) {
	yes := false
	c := huh.NewConfirm().
		Title(title).
		Description(description + " (left/right or y/n, enter confirms)").
		Affirmative("Yes").
		Negative("No").
		Value(&yes)
	if err := run(huh.NewForm(huh.NewGroup(c))); err != nil {
		return false, err
	}
	return yes, nil
}

func AskChoice(title, description string, labels []string) (int, error) {
	opts := make([]huh.Option[int], len(labels))
	for i, l := range labels {
		opts[i] = huh.NewOption(l, i)
	}
	var chosen int
	sel := huh.NewSelect[int]().
		Title(title).
		Description(description + " (up/down, enter confirms, esc cancels)").
		Options(opts...).
		Value(&chosen)
	if err := run(huh.NewForm(huh.NewGroup(sel))); err != nil {
		return 0, err
	}
	return chosen, nil
}

func AskTextDefault(title, description, def string, validate func(string) error) (string, error) {
	value := def
	in := huh.NewInput().
		Title(title).
		Description(description + " (enter confirms, ctrl+c cancels)").
		Value(&value)
	if validate != nil {
		in = in.Validate(validate)
	}
	if err := run(huh.NewForm(huh.NewGroup(in))); err != nil {
		return "", err
	}
	return value, nil
}

func ConfirmDefault(title, description string, def bool) (bool, error) {
	yes := def
	c := huh.NewConfirm().
		Title(title).
		Description(description + " (left/right or y/n, enter confirms)").
		Affirmative("Yes").
		Negative("No").
		Value(&yes)
	if err := run(huh.NewForm(huh.NewGroup(c))); err != nil {
		return false, err
	}
	return yes, nil
}
