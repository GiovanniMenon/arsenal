//go:build linux

// ioctl numbers for Linux.

package insert

import "golang.org/x/sys/unix"

const (
	getTermios      = unix.TCGETS
	setTermios      = unix.TCSETS
	setTermiosDrain = unix.TCSETSW
)
