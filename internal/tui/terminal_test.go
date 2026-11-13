package tui

// GO-20 U3b-2 (T4d): the raw-mode lifecycle. The hub keeps ONE Terminal for the
// whole interactive session and suspends raw mode around every delegated child,
// for two reasons the tests below pin separately: a child must inherit a normal
// terminal, and the keystroke reader must NOT be recreated, because bufio reads
// ahead and a fresh reader would strand bytes the user already typed (two PTY
// acceptance tests write their whole key script in a single write).

import (
	"io"
	"os"
	"testing"
)

// fakeTerminal builds a Terminal over a pipe whose raw-mode entry is scripted, so
// the lifecycle can be observed without a real terminal.
func fakeTerminal(t *testing.T, script string) (*Terminal, *int, *int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	if _, err := w.WriteString(script); err != nil {
		t.Fatal(err)
	}
	enters, restores := 0, 0
	term, err := newTerminal(r, func(*os.File) (func(), error) {
		enters++
		return func() { restores++ }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return term, &enters, &restores
}

func TestTerminalSuspendResume(t *testing.T) {
	term, enters, restores := fakeTerminal(t, "\x1b[B\n")

	before := term.reader
	if *enters != 1 || *restores != 0 {
		t.Fatalf("after construction: enters=%d restores=%d, want 1 and 0", *enters, *restores)
	}

	// A key before the suspension, so the reader has buffered the rest of the script.
	if key, err := term.NextKey(); err != nil || key != KeyDown {
		t.Fatalf("NextKey = (%v, %v), want (KeyDown, nil)", key, err)
	}

	term.Suspend()
	if *restores != 1 {
		t.Errorf("Suspend: restores=%d, want 1 (the child needs a canonical terminal)", *restores)
	}
	term.Suspend() // idempotent: nothing left to restore
	if *restores != 1 {
		t.Errorf("second Suspend: restores=%d, want 1", *restores)
	}

	if err := term.Resume(); err != nil {
		t.Fatalf("Resume returned %v", err)
	}
	if *enters != 2 {
		t.Errorf("Resume: enters=%d, want 2", *enters)
	}
	// The identity check guards the read below: if Resume had built a new reader
	// the buffered Enter would be gone and NextKey would block on the open pipe.
	if term.reader != before {
		t.Fatal("Resume replaced the keystroke reader: type-ahead would be stranded")
	}
	if key, err := term.NextKey(); err != nil || key != KeyEnter {
		t.Fatalf("NextKey after Resume = (%v, %v), want (KeyEnter, nil)", key, err)
	}

	term.Close()
	term.Close() // safe twice, like the existing contract says
	if *restores != 2 {
		t.Errorf("Close: restores=%d, want 2 (Suspend's restore plus Close's)", *restores)
	}
}

// TestTerminalCtrlDIsEOF pins the raw-mode meaning of Ctrl-D. In a canonical
// terminal the kernel turns it into end-of-file; in raw mode the byte arrives as
// 0x04, so the prompt layer has to honour the same meaning itself. Without this the
// hub's pause waits forever for an Enter that a user who typed Ctrl-D will never
// press — which is exactly what tests/test_hub_tui.py::test_aborted_pause_returns_zero
// drives over a real pty, and what prompt_toolkit did for the Python hub.
func TestTerminalCtrlDIsEOF(t *testing.T) {
	term, _, _ := fakeTerminal(t, "\x04")
	if _, err := term.NextKey(); err != io.EOF {
		t.Fatalf("NextKey(ctrl-D) error = %v, want io.EOF", err)
	}
}

// TestTerminalResumeDoesNotReenterTwice pins the idempotence of Resume: a second
// call while already raw must not stack a second restore closure, or Close would
// restore settings that were never saved.
func TestTerminalResumeDoesNotReenterTwice(t *testing.T) {
	term, enters, _ := fakeTerminal(t, "")
	if err := term.Resume(); err != nil {
		t.Fatalf("Resume on a live terminal returned %v", err)
	}
	if *enters != 1 {
		t.Errorf("Resume on a live terminal: enters=%d, want 1 (no re-entry)", *enters)
	}
}
