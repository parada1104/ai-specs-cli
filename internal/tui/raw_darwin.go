//go:build darwin

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

// enterRawMode turns off canonical mode and echo, so prompts can read a keystroke at
// a time. It returns the function that puts the terminal back exactly as it was.
func enterRawMode(f *os.File) (func(), error) {
	var saved syscall.Termios
	if err := ioctl(f.Fd(), syscall.TIOCGETA, unsafe.Pointer(&saved)); err != nil {
		return nil, err
	}
	raw := saved
	raw.Lflag &^= syscall.ICANON | syscall.ECHO
	raw.Iflag &^= syscall.ICRNL
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(f.Fd(), syscall.TIOCSETA, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return func() { _ = ioctl(f.Fd(), syscall.TIOCSETA, unsafe.Pointer(&saved)) }, nil
}

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}
