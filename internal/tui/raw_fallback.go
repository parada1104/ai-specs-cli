//go:build !darwin && !linux

package tui

import (
	"errors"
	"os"
)

// errRawUnsupported is the answer on platforms this program does not ship: raw mode
// needs the platform's terminal ioctl, and guessing one would be worse than saying so.
var errRawUnsupported = errors.New("tui: interactive prompts need a terminal on darwin or linux")

func enterRawMode(*os.File) (func(), error) { return nil, errRawUnsupported }
