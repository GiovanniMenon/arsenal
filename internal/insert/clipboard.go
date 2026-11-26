// Copies a command to the clipboard with whatever tool the system has.

package insert

import (
	"errors"
	"os"
	"os/exec"
	"strings"
)

var (
	lookPath = exec.LookPath
	getenv   = os.Getenv
)

var ErrNoClipboard = errors.New("no clipboard tool found: install pbcopy, wl-copy, xclip or xsel")

func clipboardTools() [][]string {
	tools := [][]string{{"pbcopy"}}
	if getenv("WAYLAND_DISPLAY") != "" {
		tools = append(tools, []string{"wl-copy"})
	}
	tools = append(tools,
		[]string{"xclip", "-selection", "clipboard"},
		[]string{"xsel", "--clipboard", "--input"},
		[]string{"wl-copy"},
	)
	return tools
}

func Copy(text string) error {
	var lastErr error
	for _, t := range clipboardTools() {
		path, err := lookPath(t[0])
		if err != nil {
			continue
		}
		cmd := exec.Command(path, t[1:]...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return ErrNoClipboard
}
