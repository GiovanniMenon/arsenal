// Opens the terminal to draw on, so stdout carries only the final command.

package tui

import (
	"io"
	"os"
)

func TTY() (in io.Reader, out io.Writer, closeFn func()) {
	if f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0); err == nil {
		return f, f, func() { f.Close() }
	}
	return os.Stdin, os.Stderr, func() {}
}
