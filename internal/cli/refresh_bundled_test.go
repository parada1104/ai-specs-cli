// Tests for the refresh-bundled compatibility stub (card [Go 06]): the verb
// becomes a native no-op reporting embedded provenance. The legacy cache
// machinery stays untouched (card 16 removes it); the frozen cross-cutting
// exit-code contract (0 ok/help, 2 unknown flag/unexpected positional) is
// preserved.
package cli

import (
	"strings"
	"testing"
)

func TestRefreshBundledRoutesNativeStub(t *testing.T) {
	r := Route([]string{"refresh-bundled"})
	if r.kind != routeRefreshBundled {
		t.Fatalf("kind = %v, want routeRefreshBundled", r.kind)
	}
}

func TestRunRefreshBundledStubReportsProvenance(t *testing.T) {
	var stdout, stderr strings.Builder
	code := Run([]string{"refresh-bundled"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit = %d, want 0; stderr=%q", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "embedded") {
		t.Errorf("stdout must report embedded provenance, got %q", out)
	}
	if !strings.Contains(out, "refresh-bundled") {
		t.Errorf("stdout must name the verb, got %q", out)
	}
	// The provenance line must come from the real embedded accessor: every
	// asset root listed with a nonzero file count (proves the embed is
	// linked into the binary, not just compiled into tests).
	for _, root := range []string{"catalog", "templates", "bundled-skills", "bundled-commands"} {
		if !strings.Contains(out, root) {
			t.Errorf("stdout must list embedded root %q, got %q", root, out)
		}
	}
	if strings.Contains(out, "0 files") {
		t.Errorf("embedded root resolved to zero files: %q", out)
	}
}

func TestRunRefreshBundledStubAcceptsPositionalAndInitFlag(t *testing.T) {
	for _, argv := range [][]string{
		{"refresh-bundled"},
		{"refresh-bundled", "/tmp/some-project"},
		{"refresh-bundled", "--init"},
		{"refresh-bundled", "/tmp/some-project", "--init"},
	} {
		var stdout, stderr strings.Builder
		code := Run(argv, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
		if code != 0 {
			t.Errorf("%v: exit = %d, want 0 (stderr=%q)", argv, code, stderr.String())
		}
	}
}

func TestRunRefreshBundledStubHelp(t *testing.T) {
	for _, flag := range []string{"-h", "--help"} {
		var stdout, stderr strings.Builder
		code := Run([]string{"refresh-bundled", flag}, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
		if code != 0 {
			t.Errorf("%s: exit = %d, want 0", flag, code)
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Errorf("%s: stdout must print usage, got %q", flag, stdout.String())
		}
	}
}

func TestRunRefreshBundledStubUnknownFlagExits2(t *testing.T) {
	var stdout, stderr strings.Builder
	code := Run([]string{"refresh-bundled", "--bogus"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (frozen unknown-flag contract)", code)
	}
	if !strings.Contains(stderr.String(), "--bogus") {
		t.Errorf("stderr must name the flag, got %q", stderr.String())
	}
}

func TestRunRefreshBundledStubExtraPositionalExits2(t *testing.T) {
	var stdout, stderr strings.Builder
	code := Run([]string{"refresh-bundled", "/a", "/b"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit = %d, want 2 (frozen unexpected-positional contract)", code)
	}
}
