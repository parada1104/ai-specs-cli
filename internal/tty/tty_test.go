package tty

import (
	"bytes"
	"os"
	"testing"
)

// TestIsTerminalRejectsAnythingThatIsNotAFile pins the shape contract: only an
// *os.File can be a terminal, so a buffer or a nil stdin answers false without
// touching the kernel. This is what keeps `ai-specs` with a closed stdin from
// being mistaken for an interactive session.
func TestIsTerminalRejectsAnythingThatIsNotAFile(t *testing.T) {
	var buf bytes.Buffer
	if IsTerminal(nil) {
		t.Error("IsTerminal(nil) = true, want false")
	}
	if IsTerminal(&buf) {
		t.Error("IsTerminal(buffer) = true, want false")
	}
	if IsTTY(nil, &buf) || IsTTY(&buf, nil) {
		t.Error("IsTTY with a nil stream = true, want false")
	}
}

// TestIsTerminalRejectsNonTerminalCharDevices is the regression this package
// exists for: /dev/null and /dev/zero are character devices and NOT terminals, so
// the character-device stat that used to answer this question made the hub's
// anti-hang guard launch Python instead of printing its message.
func TestIsTerminalRejectsNonTerminalCharDevices(t *testing.T) {
	for _, path := range []string{"/dev/null", "/dev/zero"} {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Skipf("%s is unavailable here: %v", path, err)
		}
		if IsTerminal(f) {
			t.Errorf("IsTerminal(%s) = true, want false: it is a character device, not a terminal", path)
		}
		if IsTTY(f, f) {
			t.Errorf("IsTTY(%s, %s) = true, want false", path, path)
		}
		f.Close()
	}
}

// TestIsTerminalRejectsPipes pins the other redirection every shell user has:
// piping the output must not look interactive.
func TestIsTerminalRejectsPipes(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsTerminal(r) || IsTerminal(w) {
		t.Error("a pipe is not a terminal")
	}
	if IsTTY(r, w) {
		t.Error("IsTTY(pipe, pipe) = true, want false")
	}
}

// TestIsTerminalAcceptsARealTerminal is the positive case, and it skips when the
// test binary has no controlling terminal (CI runs without one). The positive path
// is covered end to end by the PTY suites, which drive the whole binary.
func TestIsTerminalAcceptsARealTerminal(t *testing.T) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("no controlling terminal here: %v", err)
	}
	defer f.Close()
	if !IsTerminal(f) {
		t.Error("IsTerminal(/dev/tty) = false, want true")
	}
	if !IsTTY(f, f) {
		t.Error("IsTTY(/dev/tty, /dev/tty) = false, want true")
	}
}
