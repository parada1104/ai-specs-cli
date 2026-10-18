package tui

// The text prompt: the ONE new primitive the init wizard needs (GO-20
// U4/T4b-1, S13) — a questionary.text port with a default. It lives beside
// Select/MultiSelect/Confirm and obeys the same three rules: the question is
// drawn before the first key is read, an abort answers ("", false), and the
// prompt never touches the terminal itself. Since GO-20 U4d-2 the line
// editor behind it also answers TextRaw (questionary.text's raw buffer, the
// env offer's value prompt) and Password (questionary.password's masked
// echo), so this file owns the shared engine and each wrapper one contract.
//
// KeySource alone cannot carry this: KeyKind classifies keystrokes (arrow,
// space, enter), while a line of text needs the CHARACTER. TextSource is that
// second seam — the production Terminal decodes the same byte stream either
// way, and a test scripts the two independently.

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// TextKeyKind classifies one keystroke in a text prompt.
type TextKeyKind int

const (
	// TextRune is a printable character; TextKey.Rune carries it.
	TextRune TextKeyKind = iota
	// TextEnter submits the line.
	TextEnter
	// TextBackspace deletes before the cursor (DEL or Ctrl-H).
	TextBackspace
	// TextCursorLeft and TextCursorRight move inside the line.
	TextCursorLeft
	TextCursorRight
	// TextLineStart is Ctrl-A: the cursor to the start of the line.
	TextLineStart
	// TextKillToEnd is Ctrl-K: delete from the cursor to the end.
	TextKillToEnd
	// TextKillToStart is Ctrl-U: delete from the start to the cursor.
	TextKillToStart
	// TextCtrlD is Ctrl-D: prompt_toolkit's meaning — abort on an empty line,
	// delete forward otherwise. tests/test_init_tui.py clears the default with
	// Ctrl-A Ctrl-K before EOF, so the empty case is the one PTY drives.
	TextCtrlD
	// TextAbort is Ctrl-C or a bare ESC: the abort every prompt here shares.
	TextAbort
	// TextIgnore is an unrecognised byte or escape sequence: consume it and
	// keep waiting rather than throwing the line away.
	TextIgnore
)

// TextKey is one text-prompt keystroke.
type TextKey struct {
	Kind TextKeyKind
	Rune rune
}

// TextSource is the character stream Text edits from. Terminal implements it
// beside NextKey; a test implements it beside its key script.
type TextSource interface {
	NextTextKey() (TextKey, error)
}

// Text asks the user for a line of text with a default — init_tui.py:261's
// questionary.text, which is the wizard's project-name step.
//
// The line is drawn BEFORE the first key is read (the order Confirm pins: a
// prompt that only appears once answered deadlocks every PTY stage waiting
// for the question text) and redrawn in place per keystroke. Enter returns the
// answer strip()ped, keeping defaultValue when that leaves nothing —
// init_tui.py's `answer.strip() or default` (S14). Ctrl-C, ESC, Ctrl-D on an
// empty line, EOF and a read error all return ("", false).
func Text(out io.Writer, in KeySource, message, defaultValue string) (string, bool) {
	return textLine(out, in, textPrompt{message: message, prefill: defaultValue})
}

// TextRaw is the SAME editor answering questionary's own contract: the buffer
// verbatim, no strip() and no default substitution (GO-20 U4d-2, S30's third
// correction). questionary.text never trims — the default is RESET INTO the
// buffer before the prompt runs, so what comes back is exactly what is on the
// line, and only a prefill the user left alone survives as the answer. The env
// offer's value prompt is this one; S14's strip() stays with the name step.
func TextRaw(out io.Writer, in KeySource, message, defaultValue string) (string, bool) {
	return textLine(out, in, textPrompt{message: message, prefill: defaultValue, verbatim: true})
}

// textPrompt is the one shape shared by Text, TextRaw and Password: the same
// line editor over TextSource, differing only in how the line is echoed (mask)
// and how Enter turns it into an answer (verbatim vs S14's strip+default).
type textPrompt struct {
	message     string
	instruction string // drawn after the message, questionary's instruction token
	prefill     string
	mask        bool // echo one `*` per code point — PasswordProcessor's mask
	verbatim    bool // answer with the buffer as-is (questionary.text/password)
}

// textLine runs the shared prompt. The prefix keeps questionary's token
// layout: qmark, ` <message> `, and, when present, ` <instruction> `.
func textLine(out io.Writer, in KeySource, p textPrompt) (string, bool) {
	src, ok := in.(TextSource)
	if !ok {
		// Key kinds carry no characters: without the rune stream the answer
		// would be invisible, so the prompt refuses rather than guesses.
		return "", false
	}
	prefix := "? " + p.message + " "
	if p.instruction != "" {
		prefix += " " + p.instruction + " "
	}
	value := []rune(p.prefill)
	pos := len(value)
	echo := func() string {
		if p.mask {
			return strings.Repeat("*", len(value))
		}
		return string(value)
	}
	// The line is `? <message> <value>`; with the cursor left of the end the
	// redraw parks it back on the cursor's column.
	draw := func() {
		fmt.Fprintf(out, "\r\x1b[2K%s%s", prefix, echo())
		if n := len(value) - pos; n > 0 {
			fmt.Fprintf(out, "\x1b[%dD", n)
		}
	}
	draw()
	for {
		ev, err := src.NextTextKey()
		if err != nil {
			fmt.Fprintln(out)
			return "", false
		}
		switch ev.Kind {
		case TextEnter:
			fmt.Fprintf(out, "\r\x1b[2K%s%s\n", prefix, echo())
			if p.verbatim {
				return string(value), true
			}
			answer := strings.TrimSpace(string(value))
			if answer == "" {
				answer = p.prefill
			}
			return answer, true
		case TextRune:
			value = append(value[:pos], append([]rune{ev.Rune}, value[pos:]...)...)
			pos++
		case TextBackspace:
			if pos > 0 {
				value = append(value[:pos-1], value[pos:]...)
				pos--
			}
		case TextCursorLeft:
			if pos > 0 {
				pos--
			}
		case TextCursorRight:
			if pos < len(value) {
				pos++
			}
		case TextLineStart:
			pos = 0
		case TextKillToEnd:
			value = value[:pos]
		case TextKillToStart:
			value = value[pos:]
			pos = 0
		case TextCtrlD:
			if len(value) == 0 {
				fmt.Fprintln(out)
				return "", false
			}
			if pos < len(value) {
				value = append(value[:pos], value[pos+1:]...)
			}
		case TextAbort:
			fmt.Fprintln(out)
			return "", false
		case TextIgnore:
		}
		draw()
	}
}

// NextTextKey decodes one text-prompt keystroke from the same byte stream
// NextKey decodes. It sits in this file because Text is its only caller —
// the list prompts keep NextKey.
//
// NextKey folds every printable byte into KeyRune (no prompt there needed the
// character), which is exactly what a text prompt must NOT do: 'y', 'n', 'j',
// 'k' and space are ordinary characters here, and a multi-byte UTF-8 rune has
// to survive as one.
func (t *Terminal) NextTextKey() (TextKey, error) {
	b, err := t.reader.ReadByte()
	if err != nil {
		return TextKey{}, err
	}
	if b >= utf8.RuneSelf {
		return t.textRune(b)
	}
	switch b {
	case 3:
		return TextKey{Kind: TextAbort}, nil
	case 4:
		return TextKey{Kind: TextCtrlD}, nil
	case '\r', '\n':
		return TextKey{Kind: TextEnter}, nil
	case 127, 8:
		return TextKey{Kind: TextBackspace}, nil
	case 1:
		return TextKey{Kind: TextLineStart}, nil
	case 11:
		return TextKey{Kind: TextKillToEnd}, nil
	case 21:
		return TextKey{Kind: TextKillToStart}, nil
	case 27:
		return t.textEscapeSequence()
	default:
		if b < 32 {
			// Unassigned control bytes (NUL, Ctrl-B, …): swallow, keep typing.
			return TextKey{Kind: TextIgnore}, nil
		}
		return TextKey{Kind: TextRune, Rune: rune(b)}, nil
	}
}

// textEscapeSequence recognises the cursor forms (ESC [ D / ESC [ C). A bare
// ESC, or anything that is not a CSI, is this package's abort — the same
// answer escapeSequence gives the list prompts.
func (t *Terminal) textEscapeSequence() (TextKey, error) {
	b, err := t.reader.ReadByte()
	if err != nil || b != '[' {
		return TextKey{Kind: TextAbort}, nil
	}
	b, err = t.reader.ReadByte()
	if err != nil {
		return TextKey{Kind: TextAbort}, nil
	}
	switch b {
	case 'D':
		return TextKey{Kind: TextCursorLeft}, nil
	case 'C':
		return TextKey{Kind: TextCursorRight}, nil
	default:
		// Up/Down and friends do not edit a single line: ignore them.
		return TextKey{Kind: TextIgnore}, nil
	}
}

// textRune decodes a multi-byte UTF-8 rune whose first byte has already been
// consumed. An invalid sequence (a lone continuation, an overlong form, a
// truncated stream's tail) is ignored rather than turned into a replacement
// character in the user's answer; a truncated stream propagates the read
// error so the prompt aborts instead of hanging.
func (t *Terminal) textRune(first byte) (TextKey, error) {
	var extra int
	switch {
	case first >= 0xC2 && first <= 0xDF:
		extra = 1
	case first >= 0xE0 && first <= 0xEF:
		extra = 2
	case first >= 0xF0 && first <= 0xF4:
		extra = 3
	default:
		return TextKey{Kind: TextIgnore}, nil
	}
	buf := make([]byte, extra+1)
	buf[0] = first
	for i := 1; i <= extra; i++ {
		b, err := t.reader.ReadByte()
		if err != nil {
			return TextKey{}, err
		}
		buf[i] = b
	}
	r, size := utf8.DecodeRune(buf)
	if r == utf8.RuneError && size <= 1 {
		return TextKey{Kind: TextIgnore}, nil
	}
	return TextKey{Kind: TextRune, Rune: r}, nil
}
