// Package tty answers the one question the hub's anti-hang guard depends on: is
// this stream a terminal?
//
// A character-device stat is NOT the answer. `/dev/null` and `/dev/zero` are
// character devices but not terminals, and using the stat as the test made the
// guard's premise false: with both streams redirected to those devices over an
// uninitialized target, the hub took the interactive branch and launched Python
// where the shell prints the guard and exits 2 (GO-20 U3a, advisories R1-001,
// R3-istty-chardevice-not-isatty and R4-istty-chardevice-defeats-antihang-guard).
// So the kernel is asked with the terminal ioctl, per shipped platform, and no
// dependency is added — D8 keeps go.mod free of third-party modules.
package tty

import (
	"io"
	"os"
)

// IsTTY reports whether BOTH streams are terminals: the shell's `[[ -t 0 && -t 1 ]]`
// and hub.py's `stdin.isatty() and stdout.isatty()`.
func IsTTY(stdin io.Reader, stdout io.Writer) bool {
	return IsTerminal(stdin) && IsTerminal(stdout)
}

// IsTerminal reports whether v is a terminal. Anything that is not an *os.File is
// not one: a buffer, a wrapper, or the nil stdin of a bare invocation.
func IsTerminal(v any) bool {
	f, ok := v.(*os.File)
	if !ok {
		return false
	}
	return isTerminal(f)
}
