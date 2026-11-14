package cli

import (
	"bytes"
	"os"
	"testing"
)

// TestIsTTYRejectsNonTerminalCharDevices is the RED that pins the confirmed U3a
// defect: /dev/null and /dev/zero ARE character devices, so a character-device
// stat cannot answer "is this a terminal?".
//
// The consequence is not cosmetic. With stdin and stdout both character devices
// over an uninitialized target, the hub used to take the interactive branch and
// launch Python, where the shell route prints the anti-hang guard and exits 2 —
// so the guard's whole premise was false.
func TestIsTTYRejectsNonTerminalCharDevices(t *testing.T) {
	for _, path := range []string{"/dev/null", "/dev/zero"} {
		f, err := os.OpenFile(path, os.O_RDWR, 0)
		if err != nil {
			t.Skipf("%s is unavailable here: %v", path, err)
		}
		if isTTY(f, f) {
			t.Errorf("isTTY(%s, %s) = true; %s is a character device but not a terminal", path, path, path)
		}
		f.Close()
	}
}

// TestIsTTYRejectsRedirectedStreams keeps the contract the dispatcher always had:
// a pipe, a buffer or a closed stdin is not a terminal, whatever the platform.
func TestIsTTYRejectsRedirectedStreams(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	var buf bytes.Buffer
	if isTTY(r, w) {
		t.Error("isTTY(pipe, pipe) = true, want false")
	}
	if isTTY(nil, &buf) {
		t.Error("isTTY(nil, buffer) = true, want false")
	}
	if isTTY(&buf, &buf) {
		t.Error("isTTY(buffer, buffer) = true, want false")
	}
	if isTTY(r, w) || isTTY(nil, w) || isTTY(r, nil) {
		t.Error("a single non-terminal stream must be enough to answer false")
	}
}

// TestIsTTYAcceptsATerminalOnBothStreams documents the positive case: the one
// thing a character-device stat got RIGHT. It needs a real terminal, which a test
// binary does not always have (CI runs without one), so it skips instead of
// failing — the positive path is covered end to end by the PTY suites, which drive
// the whole binary through a real pty.
func TestIsTTYAcceptsATerminalOnBothStreams(t *testing.T) {
	in, err := os.Open("/dev/tty")
	if err != nil {
		t.Skipf("no controlling terminal here: %v", err)
	}
	defer in.Close()
	out, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	if err != nil {
		t.Skipf("no controlling terminal here: %v", err)
	}
	defer out.Close()
	if !isTTY(in, out) {
		t.Error("isTTY(/dev/tty, /dev/tty) = false, want true")
	}
}
