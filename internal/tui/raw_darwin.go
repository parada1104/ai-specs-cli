//go:build darwin

package tui

import (
	"os"
	"syscall"
	"unsafe"
)

// enterRawMode turns off canonical mode, echo and signal generation, so prompts can
// read a keystroke at a time. It returns the function that puts the terminal back
// exactly as it was.
//
// ISIG is cleared along with ICANON: with ISIG on, the line discipline turns Ctrl-C
// into SIGINT and CONSUMES the byte, so the prompt layer never sees the abort — the
// key is simply swallowed and the prompt waits for a key that will never arrive. That
// is a measured defect, not a preference: GO-20 U3b-2's PTY acceptance suite
// (Ctrl-C must abort a prompt and return to the menu) hung on it. Clearing ISIG is
// also what makes Ctrl-C mean "abort this prompt" instead of "kill the hub", which is
// how questionary/prompt_toolkit behaved for the Python hub.
func enterRawMode(f *os.File) (func(), error) {
	var saved syscall.Termios
	if err := ioctl(f.Fd(), syscall.TIOCGETA, unsafe.Pointer(&saved)); err != nil {
		return nil, err
	}
	raw := saved
	// IEXTEN goes with ISIG (its literal-next and discard controls must not eat a
	// keystroke), and IXON|IXOFF with it so the terminal's own Ctrl-S cannot freeze
	// the prompt's output — the layer reads bytes itself, so it owns those keys
	// (odd/tasks/go-20-u3b-advisories.md S8).
	raw.Lflag &^= syscall.ICANON | syscall.ECHO | syscall.ISIG | syscall.IEXTEN
	raw.Iflag &^= syscall.ICRNL | syscall.IXON | syscall.IXOFF
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
