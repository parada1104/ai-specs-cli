package tui

import (
	"bufio"
	"os"
)

// Terminal is the production KeySource. It takes the terminal out of canonical mode
// so each keystroke arrives without waiting for Enter, and restores the previous
// settings on Close — a hub that leaves a terminal in raw mode is a broken shell.
type Terminal struct {
	in      *os.File
	reader  *bufio.Reader
	restore func()
}

// NewTerminal takes over in, which must be a terminal. The caller owns Close.
func NewTerminal(in *os.File) (*Terminal, error) {
	restore, err := enterRawMode(in)
	if err != nil {
		return nil, err
	}
	return &Terminal{in: in, reader: bufio.NewReader(in), restore: restore}, nil
}

// Close restores the terminal settings, and is safe to call more than once.
func (t *Terminal) Close() {
	if t.restore != nil {
		t.restore()
		t.restore = nil
	}
}

// NextKey decodes one keystroke. Arrow keys arrive as escape sequences, which is why
// ESC needs the byte that follows to be told apart from an abort.
func (t *Terminal) NextKey() (Key, error) {
	b, err := t.reader.ReadByte()
	if err != nil {
		return 0, err
	}
	switch b {
	case 3:
		return KeyCtrlC, nil
	case '\r', '\n':
		return KeyEnter, nil
	case ' ':
		return KeySpace, nil
	case 27:
		return t.escapeSequence()
	case 'j':
		return KeyDown, nil
	case 'k':
		return KeyUp, nil
	case 'y', 'Y':
		return KeyYes, nil
	case 'n', 'N':
		return KeyNo, nil
	default:
		return KeyRune, nil
	}
}

// escapeSequence recognises the arrow forms and treats everything else as an abort,
// which is what ESC means to every prompt in this package. A bare ESC followed by a
// newline therefore aborts on the newline, which is the behaviour a user expects.
func (t *Terminal) escapeSequence() (Key, error) {
	b, err := t.reader.ReadByte()
	if err != nil || b != '[' {
		return KeyEscape, nil
	}
	c, err := t.reader.ReadByte()
	if err != nil {
		return KeyEscape, nil
	}
	switch c {
	case 'A':
		return KeyUp, nil
	case 'B':
		return KeyDown, nil
	default:
		return KeyRune, nil
	}
}
