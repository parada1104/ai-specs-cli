//go:build !darwin && !linux

package tty

import "os"

// isTerminal is the documented FALLBACK for the platforms this program does not
// ship. It repeats the character-device stat, which is strictly weaker than the
// ioctl the shipped platforms use: `/dev/null` and `/dev/zero` satisfy it without
// being terminals. It exists so the build stays portable, not because it is a good
// answer — see the package comment.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
