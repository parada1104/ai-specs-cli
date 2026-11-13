package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGatesFamilyHelpUsageAndUnknownSubcommand(t *testing.T) {
	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "--help")
	if code != 0 || stderr != "" {
		t.Fatalf("exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	for _, want := range []string{
		"Usage: ai-specs gates premerge <slug> --root <planning-root>",
		"--root",
		"--tier",
		"--stage",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help output %q does not contain %q", stdout, want)
		}
	}

	code, stdout, stderr = runGatesCLI(t, "gates", "--help")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage: ai-specs gates <subcommand>") {
		t.Errorf("gates --help: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, stderr = runGatesCLI(t, "gates")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "Usage: ai-specs gates <subcommand>") {
		t.Errorf("bare gates: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}

	code, stdout, stderr = runGatesCLI(t, "gates", "bogus")
	if code != 2 || stdout != "" || !strings.Contains(stderr, "unknown subcommand 'bogus'") {
		t.Errorf("gates bogus: exit = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

func TestGatesFamilyRoutes(t *testing.T) {
	cases := []struct {
		args     []string
		wantKind routeKind
	}{
		{[]string{"gates", "premerge"}, routeGatesPremerge},
		{[]string{"gates", "premerge", "slug", "--root", "/tmp"}, routeGatesPremerge},
		{[]string{"gates"}, routeGates},
		{[]string{"gates", "--help"}, routeGates},
		{[]string{"gates", "help"}, routeGates},
		{[]string{"gates", "premergee"}, routeGates},
		{[]string{"premerge", "slug"}, routeUnknown},
	}
	for _, tc := range cases {
		if got := Route(tc.args).kind; got != tc.wantKind {
			t.Errorf("Route(%q) kind = %v, want %v", tc.args, got, tc.wantKind)
		}
	}
}

// TestRootHelpAnnouncesTheGatesFamily pins the one-line root announcement: the
// root dispatcher has no `premerge` verb, and the family is reachable through
// `gates premerge`.
func TestRootHelpAnnouncesTheGatesFamily(t *testing.T) {
	code, stdout, stderr := runGatesCLI(t, "help")
	if code != 0 || stderr != "" {
		t.Fatalf("help: exit = %d, stderr = %q; want 0/empty", code, stderr)
	}
	if !strings.Contains(stdout, "gates premerge <slug> --root <planning-root>") {
		t.Errorf("root help %q does not announce the gates family", stdout)
	}
}

// TestGatesVerdictIsStoreBlind ports the Python store-invariance cases
// (test_guardian_*_under_any_store): the preflight artifact store never changes
// a guardian verdict, so memory-only presence can never satisfy a file gate.
func TestGatesVerdictIsStoreBlind(t *testing.T) {
	root := gatesRepo(t)
	gatesConformingArchive(t, root, "ok-change")
	gatesWrite(t, filepath.Join(root, "openspec", "changes", "archive", "bad-change", "tasks.md"),
		"Depth: standard\n")

	baseline := map[string][3]string{}
	for _, slug := range []string{"ok-change", "bad-change"} {
		code, stdout, stderr := runGatesCLI(t, "gates", "premerge", slug, "--root", root)
		baseline[slug] = [3]string{stdout, stderr, string(rune('0' + code))}
	}
	for _, store := range []string{"openspec", "engram", "both"} {
		t.Run(store, func(t *testing.T) {
			t.Setenv("PLAN_BUILD_ARTIFACT_STORE", store)
			for _, slug := range []string{"ok-change", "bad-change"} {
				code, stdout, stderr := runGatesCLI(t, "gates", "premerge", slug, "--root", root)
				want := baseline[slug]
				if stdout != want[0] || stderr != want[1] || string(rune('0'+code)) != want[2] {
					t.Errorf("store=%s slug=%s verdict changed: %q/%q/%d vs %q/%q/%s",
						store, slug, stdout, stderr, code, want[0], want[1], want[2])
				}
			}
		})
	}
}

// TestGatesNeverInvokesTheTrackerLedger ports the Python W1 isolation case: the
// artifact guardian owns artifacts, the tracker host owns the ledger, so a
// blocking ledger stub must never be reached.
func TestGatesNeverInvokesTheTrackerLedger(t *testing.T) {
	root := gatesRepo(t)
	gatesConformingArchive(t, root, "done")
	stub := filepath.Join(t.TempDir(), "stub-worktree-gate")
	gatesWrite(t, stub, "#!/usr/bin/env bash\nprintf 'called\\n' >> \"$STUB_LOG\"\nexit 2\n")
	if err := os.Chmod(stub, 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "stub.log")
	t.Setenv("WORKTREE_GATE_BIN", stub)
	t.Setenv("STUB_LOG", log)
	t.Setenv("STUB_DECISION", "block")

	code, stdout, stderr := runGatesCLI(t, "gates", "premerge", "done", "--root", root)
	if code != 0 || stdout != "premerge-guardian: OK (standard)\n" {
		t.Fatalf("exit = %d, stdout = %q, stderr = %q; want the OK verdict", code, stdout, stderr)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatalf("the artifact guardian invoked the tracker ledger (log exists): %v", err)
	}
}
