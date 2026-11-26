// Package insert hands the chosen command to the user.
// This file puts it in the shell prompt, with the shell function or the clipboard as fallbacks.
package insert

import (
	"errors"
	"fmt"
	"os"
)

const EnvCmdFile = "ARSENAL_CMD_FILE"

type Method string

const (
	MethodTIOCSTI   Method = "prompt"
	MethodShell     Method = "shell function"
	MethodClipboard Method = "clipboard"
)

var (
	injectFn = func(text string) error {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInjectBlocked, err)
		}
		defer tty.Close()
		return Inject(tty, text)
	}
	copyFn = Copy
)

func ToPrompt(line string) (Method, string, error) {
	err := injectFn(line)
	if err == nil {
		return MethodTIOCSTI, "", nil
	}
	if !errors.Is(err, ErrInjectBlocked) {
		return "", "", err
	}

	if f := getenv(EnvCmdFile); f != "" {
		if werr := os.WriteFile(f, []byte(line), 0o600); werr != nil {
			return "", "", fmt.Errorf("cannot pass the command to your shell: %w", werr)
		}
		return MethodShell, "", nil
	}

	if cerr := copyFn(line); cerr != nil {
		return "", "Run ars setup to get commands in your prompt.",
			fmt.Errorf("cannot put the command in your prompt (%v) or the clipboard (%v)", err, cerr)
	}
	return MethodClipboard, "This system does not let programs type into the terminal, so the command was copied instead. " +
		"Run ars setup to get commands in your prompt.", nil
}
