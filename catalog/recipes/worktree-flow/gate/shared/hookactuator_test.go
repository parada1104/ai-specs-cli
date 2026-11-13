package shared

import (
	"path/filepath"
	"testing"
)

// The tracker bridge tokens, after GO-20 U2e T5 PR4b removed the legacy
// `__TRACKER_LIB_INTERNAL__` stamp: absolute resolved CLI home when one is
// given, EMPTY when it is not (empty makes the host skip evidence acquisition
// and the tracker.none write — fail open).
//
// Moved here from the gate main package's hookgateactuator_test.go when PR4b
// deleted the --materialize-hook bridge flag: the renderer is the shared core
// the root module uses in production (internal/sync.MaterializeHook), so the
// pin follows the core.
func TestRenderHookGateContentTrackerHomes(t *testing.T) {
	home := t.TempDir()
	src := []byte("home=__TRACKER_CLI_HOME__\n")
	got, err := RenderHookGateContent(src, nil, home, "dev")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	want := "home=" + filepath.Clean(home) + "\n"
	if got != want {
		t.Errorf("render = %q, want %q", got, want)
	}
	got, err = RenderHookGateContent(src, nil, "", "dev")
	if err != nil {
		t.Fatalf("render (no home): %v", err)
	}
	if got != "home=\n" {
		t.Errorf("render without home = %q, want the empty substitution", got)
	}
}

// TestRenderHookGateContentIgnoresTheRetiredLibInternalStamp proves the deleted
// placeholder is no longer substituted: a stray token in a stale stamped hook
// copy is left untouched, and nothing fills a lib/_internal path into a
// generated hook.
func TestRenderHookGateContentIgnoresTheRetiredLibInternalStamp(t *testing.T) {
	home := t.TempDir()
	got, err := RenderHookGateContent([]byte("# internal=__TRACKER_LIB_INTERNAL__\n"), nil, home, "dev")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if got != "# internal=__TRACKER_LIB_INTERNAL__\n" {
		t.Errorf("render = %q, want the token left untouched", got)
	}
}
