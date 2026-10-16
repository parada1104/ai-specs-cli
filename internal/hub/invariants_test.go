package hub

// GO-20 U3b-2 (T4c): the two SOURCE invariants the Python suites used to
// enforce, re-expressed against the Go tree.
//
//   - tests/test_hub.py::TestNoOpenptyInHub asserted the hub never allocates a
//     pty. The native hub must keep that: it draws only through internal/tui, so
//     no pty/termios token may reappear.
//   - tests/test_hub_tui.py::TestPauseOnlySite asserted the pause prompt has one
//     call site, which is what keeps the loop's stop behaviour in one place. It
//     also guards that the hub never reads stdin itself; internal/tui owns the
//     terminal.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// nonTestGoFiles lists the package's production .go files, excluding _test.go.
func nonTestGoFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out
}

func sources(t *testing.T, files []string) map[string]string {
	t.Helper()
	out := make(map[string]string, len(files))
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		out[file] = string(data)
	}
	return out
}

func TestHubNeverAllocatesAPty(t *testing.T) {
	// Case-insensitive: it covers openpty/Openpty, ptmx and termios at once.
	forbidden := []string{"openpty", "ptmx", "termios"}
	for file, body := range sources(t, nonTestGoFiles(t, ".")) {
		lower := strings.ToLower(body)
		for _, token := range forbidden {
			if strings.Contains(lower, token) {
				t.Errorf("%s contains forbidden pty token %q; the hub must draw through internal/tui", file, token)
			}
		}
	}
}

func TestHubNeverReadsStdinDirectly(t *testing.T) {
	forbidden := []string{"os.stdin", "bufio", "readbyte"}
	for file, body := range sources(t, nonTestGoFiles(t, ".")) {
		lower := strings.ToLower(body)
		for _, token := range forbidden {
			if strings.Contains(lower, token) {
				t.Errorf("%s contains forbidden stdin token %q; internal/tui owns the terminal", file, token)
			}
		}
	}
}

func TestHubHasExactlyOnePauseCallSite(t *testing.T) {
	total := 0
	for file, body := range sources(t, nonTestGoFiles(t, ".")) {
		if n := strings.Count(body, "tui.Pause("); n > 0 {
			t.Logf("%s: %d tui.Pause( call site(s)", file, n)
		}
		total += strings.Count(body, "tui.Pause(")
	}
	if total != 1 {
		t.Errorf("tui.Pause( appears %d times across internal/hub, want exactly 1", total)
	}
}
