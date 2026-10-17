package hub

// GO-20 U3b-2 (T4c) data-safety fix: once terminal.Resume fails after a
// delegation, the terminal is back in canonical mode and the hub must stop
// instead of prompting keys it can no longer decode.

import (
	"bytes"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/tui"
)

// TestLoopStopsWhenResumeFails pins FIX 9 (loop half): a session that already
// lost raw mode must return the loop's exit code (0) without drawing the panel
// or prompting the menu.
func TestLoopStopsWhenResumeFails(t *testing.T) {
	var out, errb bytes.Buffer
	s := &interactive{
		target:       initializedHubTarget(t),
		home:         hubHome(t),
		keys:         &scriptedKeys{keys: selectIndex(menuQuit)},
		stdout:       &out,
		stderr:       &errb,
		resumeFailed: true,
	}

	if rc := s.runLoop(); rc != 0 {
		t.Fatalf("rc = %d, want the loop's exit code 0", rc)
	}
	if strings.Contains(out.String(), "What do you want to do?") {
		t.Errorf("loop prompted after a failed resume:\n%s", out.String())
	}
}

// TestPauseStopsWhenResumeFails pins FIX 9 (pause half): the pause must not draw
// its message or read a key once raw mode could not be restored.
func TestPauseStopsWhenResumeFails(t *testing.T) {
	var out, errb bytes.Buffer
	s := &interactive{
		keys:         &scriptedKeys{keys: []tui.Key{tui.KeyEnter}},
		stdout:       &out,
		stderr:       &errb,
		resumeFailed: true,
	}

	if s.pause() {
		t.Error("pause returned true after a failed resume, want false so the loop stops")
	}
	if strings.Contains(out.String(), "Press Enter") {
		t.Errorf("pause drew its prompt after a failed resume:\n%s", out.String())
	}
}
