package tui

import (
	"bufio"
	"io"
	"os"
)

// Terminal is the production KeySource. It takes the terminal out of canonical mode
// so each keystroke arrives without waiting for Enter, and restores the previous
// settings on Close — a hub that leaves a terminal in raw mode is a broken shell.
//
// Raw mode is SUSPENDED around every delegated child and resumed after it: a child
// inherits a normal terminal, while the keystroke reader below survives both calls,
// so keys the user typed ahead are not stranded.
type Terminal struct {
	in      *os.File
	reader  *bufio.Reader
	enter   func(*os.File) (func(), error)
	restore func()
}

// NewTerminal takes over in, which must be a terminal. The caller owns Close.
func NewTerminal(in *os.File) (*Terminal, error) {
	return newTerminal(in, enterRawMode)
}

// newTerminal is NewTerminal with an injectable raw-mode entry, so the
// suspend/resume lifecycle is testable without a real terminal.
func newTerminal(in *os.File, enter func(*os.File) (func(), error)) (*Terminal, error) {
	restore, err := enter(in)
	if err != nil {
		return nil, err
	}
	return &Terminal{in: in, reader: bufio.NewReader(in), enter: enter, restore: restore}, nil
}

// Suspend restores the terminal's previous settings, so a delegated child runs
// against a canonical terminal instead of a raw one. It is safe to call twice.
func (t *Terminal) Suspend() {
	if t.restore != nil {
		t.restore()
		t.restore = nil
	}
}

// Resume re-enters raw mode after a delegation. It is safe to call while already
// raw, and reports the error when the terminal cannot be taken over again, so the
// caller can stop instead of reading keys it cannot decode.
func (t *Terminal) Resume() error {
	if t.restore != nil {
		return nil
	}
	restore, err := t.enter(t.in)
	if err != nil {
		return err
	}
	t.restore = restore
	return nil
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
	case 4:
		// Ctrl-D. A canonical terminal turns it into end-of-file; raw mode hands
		// the byte over instead, so the same meaning is honoured here by hand.
		// prompt_toolkit did this for the Python hub, and the PTY acceptance test
		// (Ctrl-D at a pause must exit the hub cleanly) depends on it.
		return 0, io.EOF
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
