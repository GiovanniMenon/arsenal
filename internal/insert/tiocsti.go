//go:build darwin || linux

// Types a command into the terminal input with the TIOCSTI ioctl.

package insert

import (
	"errors"
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

var ErrInjectBlocked = errors.New("the system does not allow typing into the terminal")

func Inject(tty *os.File, text string) error {
	fd := int(tty.Fd())
	old, err := unix.IoctlGetTermios(fd, getTermios)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInjectBlocked, err)
	}
	raw := *old
	raw.Lflag &^= unix.ECHO | unix.ICANON
	if err := unix.IoctlSetTermios(fd, setTermios, &raw); err != nil {
		return fmt.Errorf("%w: %v", ErrInjectBlocked, err)
	}
	defer func() { _ = unix.IoctlSetTermios(fd, setTermiosDrain, old) }()

	for i := 0; i < len(text); i++ {
		b := text[i]
		if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TIOCSTI, uintptr(unsafe.Pointer(&b))); errno != 0 {
			return fmt.Errorf("%w: %v", ErrInjectBlocked, errno)
		}
	}
	return nil
}
