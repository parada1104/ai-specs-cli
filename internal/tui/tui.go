// Package tui is the hub's prompt layer: the native replacement for the Python
// hub's questionary prompts and rich panel, with NO third-party dependency.
//
// Why it exists at all: the interactive hub is the default verb's face, and the
// alternative measured worse. Vendoring a prompt library (pterm v0.12.82, the newest
// version compatible with the canonical go1.24.13 toolchain) brought 15 modules, 549
// Go files, 334 049 lines and 15 MB, plus a go.sum — which would have made D8's
// "zero third-party dependencies" false and forced amendments in six records. Two
// hundred lines of our own, on the terminal primitive `internal/tty` already owns,
// keeps the module's identity AND the layer reviewable line by line.
//
// The testability seam is KeySource: a prompt never touches the terminal itself, it
// asks for the next key. Production reads a real terminal in raw mode; a test feeds
// the exact keystrokes it wants the user to have pressed.
package tui

import (
	"fmt"
	"io"
	"strings"
)

// KeyKind is one keystroke.
type KeyKind int

const (
	// KeyRune is a printable character no prompt here acts on.
	KeyRune KeyKind = iota
	// KeyUp and KeyDown move the cursor.
	KeyUp
	KeyDown
	// KeySpace toggles a checkbox.
	KeySpace
	// KeyEnter confirms.
	KeyEnter
	// KeyEscape aborts, like questionary's ESC.
	KeyEscape
	// KeyCtrlC aborts, like questionary's KeyboardInterrupt.
	KeyCtrlC
	// KeyYes and KeyNo answer a confirmation explicitly.
	KeyYes
	KeyNo
)

// Key aliases KeyKind so callers write `tui.KeyEnter`, and keeps a single type in
// the KeySource signature.
type Key = KeyKind

// KeySource yields keystrokes. A real terminal blocks until the user presses
// something; a test returns scripted keys and then io.EOF.
type KeySource interface {
	NextKey() (Key, error)
}

// Choice is one option: what the user reads and what the caller receives.
type Choice struct {
	Title string
	Value string
}

// Select asks the user to choose one option. It returns the chosen value, or
// ("", false) when the user aborts — ESC, Ctrl-C, EOF or a read error. Callers
// branch on that empty result exactly as they did on questionary's None.
func Select(out io.Writer, in KeySource, message string, choices []Choice) (string, bool) {
	if len(choices) == 0 {
		return "", false
	}
	cursor, ok := navigate(out, in, message, choices, cursorState{current: 0}, false)
	if !ok {
		return "", false
	}
	return choices[cursor].Value, true
}

// MultiSelect asks the user to toggle any number of options: space toggles, enter
// confirms. The result follows the CHOICE order, not the order in which the user
// toggled them. checked is the initial state.
func MultiSelect(out io.Writer, in KeySource, message string, choices []Choice, checked []bool) ([]string, bool) {
	if len(choices) == 0 {
		return nil, false
	}
	state := cursorState{current: 0, checked: make([]bool, len(choices))}
	copy(state.checked, checked)
	if _, ok := navigate(out, in, message, choices, state, true); !ok {
		return nil, false
	}
	var picked []string
	for i, choice := range choices {
		if state.checked[i] {
			picked = append(picked, choice.Value)
		}
	}
	return picked, true
}

// Confirm asks a yes/no question. The question is drawn BEFORE any key is read —
// the order questionary uses — so the prompt is visible while it waits, and the
// answer is appended to that same line when Enter submits it. A confirm that only
// printed once answered was invisible on a real terminal and deadlocked every PTY
// stage that waits for the question text before typing the answer (GO-20 U3b-2).
// Enter answers with def, y and n answer explicitly, everything else answers with def.
func Confirm(out io.Writer, in KeySource, message string, def bool) (bool, bool) {
	answer := def
	fmt.Fprintf(out, "? %s (%s) ", message, defaultHint(def))
	for {
		key, err := in.NextKey()
		if err != nil {
			fmt.Fprintln(out)
			return false, false
		}
		switch key {
		case KeyEnter:
			fmt.Fprintf(out, "%s\n", yesNo(answer))
			return answer, true
		case KeyYes:
			answer = true
		case KeyNo:
			answer = false
		case KeyEscape, KeyCtrlC:
			fmt.Fprintln(out)
			return false, false
		}
	}
}

// defaultHint is questionary's hint: the defaulted answer is the capitalised one.
func defaultHint(def bool) string {
	if def {
		return "Y/n"
	}
	return "y/N"
}

// Pause waits for Enter, which is hub.py's `input("Press Enter to return…")`. It
// returns false when stdin ends, so a caller stops instead of hanging.
func Pause(out io.Writer, in KeySource, message string) bool {
	if message == "" {
		message = "Press Enter to return…"
	}
	fmt.Fprint(out, message)
	for {
		key, err := in.NextKey()
		if err != nil {
			fmt.Fprintln(out)
			return false
		}
		if key == KeyEnter {
			fmt.Fprintln(out)
			return true
		}
		if key == KeyEscape || key == KeyCtrlC {
			fmt.Fprintln(out)
			return false
		}
	}
}

// cursorState is the moving part of both list prompts, so their loop is written once.
type cursorState struct {
	current  int
	checked  []bool
	rendered int
}

// navigate runs the list loop and returns the cursor it settled on. It is the only
// place that reads keys for a list, which is why Select and MultiSelect cannot drift
// apart in how they move, abort or confirm.
func navigate(out io.Writer, in KeySource, message string, choices []Choice, state cursorState, multiselect bool) (int, bool) {
	for {
		frame := renderList(message, choices, state, multiselect)
		fmt.Fprint(out, frame)
		// The next frame has to move back up over this one, so the height is remembered
		// after drawing rather than guessed before.
		state.rendered = 1 + len(choices)
		key, err := in.NextKey()
		if err != nil {
			return 0, false
		}
		switch key {
		case KeyUp:
			state.current = (state.current - 1 + len(choices)) % len(choices)
		case KeyDown:
			state.current = (state.current + 1) % len(choices)
		case KeySpace:
			if multiselect {
				state.checked[state.current] = !state.checked[state.current]
			}
		case KeyEnter:
			return state.current, true
		case KeyEscape, KeyCtrlC:
			return 0, false
		}
	}
}

// renderList draws one frame of a list prompt. The redraw moves the cursor back up
// over the frame it drew before, so a real terminal shows a single list; a buffer
// just accumulates frames, which keeps the tests able to read them. The message line
// keeps questionary's trailing space: `? <message> `, then one line per choice.
func renderList(message string, choices []Choice, state cursorState, multiselect bool) string {
	var b strings.Builder
	if state.rendered > 0 {
		fmt.Fprintf(&b, "\x1b[%dA", state.rendered)
	}
	b.WriteString("\x1b[2K")
	fmt.Fprintf(&b, "? %s \n", message)
	for i, choice := range choices {
		b.WriteString("\x1b[2K")
		marker := " "
		if i == state.current {
			marker = "»"
		}
		box := ""
		if multiselect {
			if state.checked[i] {
				box = "[x] "
			} else {
				box = "[ ] "
			}
		}
		fmt.Fprintf(&b, "%s %s%s\n", marker, box, choice.Title)
	}
	return b.String()
}

func yesNo(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}
