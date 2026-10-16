//go:build linux

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

// enterRawMode turns off canonical mode, echo and signal generation, so prompts can
// read a keystroke at a time. It returns the function that puts the terminal back
// exactly as it was. ISIG is cleared with ICANON because the line discipline would
// otherwise CONSUME Ctrl-C as SIGINT and the prompt layer would never see the abort
// (see the darwin twin for the measured PTY failure behind this).
func enterRawMode(f *os.File) (func(), error) {
	var saved syscall.Termios
	if err := ioctl(f.Fd(), syscall.TCGETS, unsafe.Pointer(&saved)); err != nil {
		return nil, err
	}
	raw := saved
	raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG
	raw.Iflag &^= syscall.ICRNL
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := ioctl(f.Fd(), syscall.TCSETS, unsafe.Pointer(&raw)); err != nil {
		return nil, err
	}
	return func() { _ = ioctl(f.Fd(), syscall.TCSETS, unsafe.Pointer(&saved)) }, nil
}

func ioctl(fd, req uintptr, arg unsafe.Pointer) error {
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, req, uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}
