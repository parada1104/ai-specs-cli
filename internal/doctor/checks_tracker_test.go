package doctor

import (
	"os"
	"path/filepath"
	"testing"
)

func writeOpenspecTrackingConfig(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, "openspec", "config.yaml"), "tracking:\n  provider: trello\n")
}

// writeExecutableFile writes an executable test script.
func writeExecutableFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestTrackerDeclared(t *testing.T) {
	root := t.TempDir()
	if trackerDeclared(root) {
		t.Fatal("no openspec config must not be declared")
	}
	writeOpenspecTrackingConfig(t, root)
	if !trackerDeclared(root) {
		t.Fatal("tracking: line must declare the ledger")
	}
}

func TestTrackerRecipeIDFallback(t *testing.T) {
	if got := trackerRecipeID(t.TempDir()); got != legacyTrackerRecipeID {
		t.Fatalf("non-git root recipe id = %q, want %q", got, legacyTrackerRecipeID)
	}
}

func TestTrackerLedgerNotInPlay(t *testing.T) {
	d := New(t.TempDir(), t.TempDir())
	d.checkTrackerLedger()
	assertChecks(t, d, []Check{})
}

func TestTrackerLedgerNoVerifiedBinary(t *testing.T) {
	root := t.TempDir()
	writeOpenspecTrackingConfig(t, root)
	t.Setenv("WORKTREE_GATE_BIN", "")

	d := New(root, t.TempDir())
	d.checkTrackerLedger()
	assertChecks(t, d, []Check{
		{ERROR, "tracker-ledger",
			"ai-specs CLI is missing or not executable; the tracker ledger is failing open",
			"run ai-specs sync"},
	})
}

func TestTrackerLedgerWarnFromBinary(t *testing.T) {
	root := t.TempDir()
	writeOpenspecTrackingConfig(t, root)
	home := t.TempDir()
	gate := filepath.Join(home, "dist", "ai-specs")
	if err := os.MkdirAll(filepath.Dir(gate), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutableFile(t, gate, "#!/bin/sh\n"+
		"[ \"$1\" = worktree-gate ] && [ \"$2\" = --ledger ] || exit 2\n"+
		"printf '%s' '{\"reason\":\"witness-missing\",\"doctor\":{\"severity\":\"WARN\",\"message\":\"witness missing; run ai-specs sync\"}}'\n")

	d := New(root, home)
	d.checkTrackerLedger()
	assertChecks(t, d, []Check{
		{WARN, "tracker-ledger", "witness missing; run ai-specs sync", "ai-specs sync"},
	})
}

func TestTrackerLedgerUnreadableVerdict(t *testing.T) {
	root := t.TempDir()
	writeOpenspecTrackingConfig(t, root)
	home := t.TempDir()
	gate := filepath.Join(home, "dist", "ai-specs")
	if err := os.MkdirAll(filepath.Dir(gate), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutableFile(t, gate, "#!/bin/sh\nprintf '%s' 'not json'\n")

	d := New(root, home)
	d.checkTrackerLedger()
	assertChecks(t, d, []Check{
		{ERROR, "tracker-ledger",
			"tracker ledger verdict was not machine-readable; failing open",
			"run ai-specs sync or ai-specs sync --refresh-gates"},
	})
}

func TestTrackerLedgerGuidance(t *testing.T) {
	cases := []struct {
		reason   string
		severity Severity
		want     string
	}{
		{"witness-missing", WARN, "ai-specs sync"},
		{"ambiguous", INFO, `add [[bindings]] capability="tracker"`},
		{"declared-not-bound", WARN, "enable the provider recipe or remove the tracking declaration"},
		{"conflict", WARN, "adjudicate at the next checkpoint"},
		{"unbound", INFO, "enable one tracker recipe or ignore"},
		{"something-else", WARN, ""},
		{"witness-missing", ERROR, "run ai-specs sync"},
	}
	for _, tc := range cases {
		if got := trackerLedgerGuidance(tc.reason, tc.severity); got != tc.want {
			t.Errorf("trackerLedgerGuidance(%q, %v) = %q, want %q", tc.reason, tc.severity, got, tc.want)
		}
	}
}

func TestLedgerSeverity(t *testing.T) {
	cases := []struct {
		value any
		want  Severity
	}{
		{nil, OK},
		{"", OK},
		{"WARN", WARN},
		{"INFO", INFO},
		{"ERROR", ERROR},
		{"bogus", ERROR},
	}
	for _, tc := range cases {
		if got := ledgerSeverity(tc.value); got != tc.want {
			t.Errorf("ledgerSeverity(%v) = %v, want %v", tc.value, got, tc.want)
		}
	}
}

func TestTrackerLedgerUsesInstalledCLI(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, "dist", "ai-specs")
	if got := installedCLIPath(home); got != want {
		t.Fatalf("installedCLIPath = %q, want %q", got, want)
	}
	if got := (&Doctor{Home: home}).trackerLedgerCLI(); got != "" {
		t.Fatalf("missing installed CLI = %q, want empty", got)
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	writeExecutableFile(t, want, "#!/bin/sh\nexit 0\n")
	if got := (&Doctor{Home: home}).trackerLedgerCLI(); got != want {
		t.Fatalf("installed CLI = %q, want %q", got, want)
	}
}
