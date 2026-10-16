//go:build linux

package tty

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal asks the kernel with the terminal ioctl. TCGETS is the Linux spelling
// of the same "get terminal attributes" request; it succeeds only for a real
// terminal.
func isTerminal(f *os.File) bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(syscall.TCGETS),
		uintptr(unsafe.Pointer(&termios)),
	)
	return errno == 0
}
