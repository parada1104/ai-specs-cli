package cliversion

// The native stamp's rules, pinned: the Python it replaces
// (lib/_internal/cli_version.py:206-223) is the authority for all three.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ai-specs.dev/ai-specs/internal/lock"
)

// newStampFixture builds a project root with ai-specs/ and a CLI home carrying
// VERSION, then returns both.
func newStampFixture(t *testing.T, version string) (root, home string) {
	t.Helper()
	home = t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte(version+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root = t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root, home
}

func readMeta(t *testing.T, root string) map[string]string {
	t.Helper()
	current, err := lock.LoadLock(LockMetaPath(root))
	if err != nil {
		t.Fatal(err)
	}
	return current.Meta
}

// TestStampLockMetaCreatesTheStamp pins the first write: a project with no lock
// gets one carrying the installed version and the clock it was stamped with.
func TestStampLockMetaCreatesTheStamp(t *testing.T) {
	t.Parallel()
	root, home := newStampFixture(t, "1.2.3")
	stamp := time.Date(2026, 10, 17, 21, 30, 45, 0, time.UTC)

	if err := StampLockMeta(root, home, stamp); err != nil {
		t.Fatal(err)
	}
	meta := readMeta(t, root)
	if meta["cli_version"] != "1.2.3" {
		t.Errorf("cli_version = %q, want 1.2.3", meta["cli_version"])
	}
	if meta["synced_at"] != "2026-10-17T21:30:45Z" {
		t.Errorf("synced_at = %q, want the UTC second stamp", meta["synced_at"])
	}
	body, err := os.ReadFile(LockMetaPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "[meta]") {
		t.Errorf("lock has no [meta] table:\n%s", body)
	}
}

// TestStampLockMetaKeepsTheStampForTheSameVersion pins the rule the Python
// states as `if existing.cli_version == installed and existing.synced_at`: a
// re-run against the same install is NOT a new sync.
func TestStampLockMetaKeepsTheStampForTheSameVersion(t *testing.T) {
	t.Parallel()
	root, home := newStampFixture(t, "1.2.3")
	first := time.Date(2026, 10, 17, 21, 30, 45, 0, time.UTC)
	later := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

	if err := StampLockMeta(root, home, first); err != nil {
		t.Fatal(err)
	}
	if err := StampLockMeta(root, home, later); err != nil {
		t.Fatal(err)
	}
	if got := readMeta(t, root)["synced_at"]; got != "2026-10-17T21:30:45Z" {
		t.Errorf("synced_at = %q, want the FIRST stamp kept", got)
	}
}

// TestStampLockMetaRenewsOnANewVersion pins the other half: a different
// installed version always takes the new clock, because it IS a new sync.
func TestStampLockMetaRenewsOnANewVersion(t *testing.T) {
	t.Parallel()
	root, home := newStampFixture(t, "1.2.3")
	first := time.Date(2026, 10, 17, 21, 30, 45, 0, time.UTC)
	later := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)

	if err := StampLockMeta(root, home, first); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "VERSION"), []byte("1.3.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StampLockMeta(root, home, later); err != nil {
		t.Fatal(err)
	}
	meta := readMeta(t, root)
	if meta["cli_version"] != "1.3.0" {
		t.Errorf("cli_version = %q, want 1.3.0", meta["cli_version"])
	}
	if meta["synced_at"] != "2026-10-18T09:00:00Z" {
		t.Errorf("synced_at = %q, want the NEW stamp", meta["synced_at"])
	}
}

// TestStampLockMetaPreservesTheRestOfTheLock pins that the stamp writes the lock
// it read: entries the sync recorded survive the [meta] rewrite.
func TestStampLockMetaPreservesTheRestOfTheLock(t *testing.T) {
	t.Parallel()
	root, home := newStampFixture(t, "1.2.3")
	lockPath := LockMetaPath(root)
	original := `[meta]
cli_version = "0.0.1"
synced_at = "2026-01-01T00:00:00Z"

[managed."AGENTS.md"]
sha256 = "abc"
`
	if err := os.WriteFile(lockPath, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := StampLockMeta(root, home, time.Date(2026, 10, 17, 21, 30, 45, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "[managed.\"AGENTS.md\"]") {
		t.Errorf("the stamp dropped an unrelated entry:\n%s", body)
	}
}
