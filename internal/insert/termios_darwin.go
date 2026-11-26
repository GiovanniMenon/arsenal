//go:build darwin

// ioctl numbers for macOS.

package insert

import "golang.org/x/sys/unix"

const (
	getTermios      = unix.TIOCGETA
	setTermios      = unix.TIOCSETA
	setTermiosDrain = unix.TIOCSETAW
)
