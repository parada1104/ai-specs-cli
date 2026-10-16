//go:build darwin

package tty

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal asks the kernel with the terminal ioctl. TIOCGETA is Darwin's "get
// terminal attributes" request: it succeeds only for a descriptor that is a real
// terminal, which is exactly the question the anti-hang guard needs answered.
func isTerminal(f *os.File) bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(
		syscall.SYS_IOCTL,
		f.Fd(),
		uintptr(syscall.TIOCGETA),
		uintptr(unsafe.Pointer(&termios)),
	)
	return errno == 0
}
