package specs

// ODD-mode promotion (plan-build-odd-companion T5, S4-S7). The composer, its
// blockers, its collision rule and its idempotence are mode-independent; only
// the source and canonical trees move, so these cases pin the odd trees and the
// fact that the OpenSpec trees stay byte-identical.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/planbuild"
)

// newOddRepo builds an odd-mode planning root plus a populated OpenSpec tree, so
// every case can prove the legacy tree is untouched.
func newOddRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, rel := range []string{"odd/changes", "odd/tasks", "openspec/specs/legacy"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	legacy := filepath.Join(root, "openspec", "specs", "legacy", "spec.md")
	if err := os.WriteFile(legacy, []byte(canonicalDoc(existingBlock)), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// writeOddDelta creates odd/changes/<slug>/specs/<domain>/spec.md.
func writeOddDelta(t *testing.T, root, slug, domain, text string) string {
	t.Helper()
	dir := filepath.Join(root, "odd", "changes", slug, "specs", domain)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "spec.md"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(root, "odd", "changes", slug)
}

func oddCanonical(root, domain string) string {
	return filepath.Join(root, "odd", "specs", domain, "spec.md")
}

func openSpecCanonical(root, domain string) string {
	return filepath.Join(root, "openspec", "specs", domain, "spec.md")
}

// One delta under odd/changes/ composes into odd/specs/, and the OpenSpec
// canonical tree is never touched.
func TestPromoteOddModeComposesIntoOddTree(t *testing.T) {
	root := newOddRepo(t)
	writeOddDelta(t, root, "odd-change", "capability", deltaDoc(addedSection(addedBlock)))
	legacyPath := openSpecCanonical(root, "legacy")
	legacyBefore := readText(t, legacyPath)

	report := PromoteChangeForMode(root, "odd-change", planbuild.ModeOdd, false)
	if len(report.Blockers) != 0 {
		t.Fatalf("Blockers = %q, want none", report.Blockers)
	}
	composed := readText(t, oddCanonical(root, "capability"))
	if !strings.Contains(composed, "### Requirement: Added requirement") {
		t.Fatalf("odd canonical = %q, want the added requirement composed in", composed)
	}
	if got := readText(t, legacyPath); got != legacyBefore {
		t.Fatalf("openspec/specs/legacy/spec.md changed in odd mode: %q", got)
	}
	if _, err := os.Stat(openSpecCanonical(root, "capability")); !os.IsNotExist(err) {
		t.Fatalf("openspec/specs/capability/spec.md was written in odd mode (err = %v)", err)
	}
}

// A missing change folder is reported against the odd tree the mode reads.
func TestPromoteOddModeNamesTheOddChangeTree(t *testing.T) {
	root := newOddRepo(t)
	report := PromoteChangeForMode(root, "ghost", planbuild.ModeOdd, false)
	wantOneBlocker(t, report, "active change folder missing at odd/changes/ghost/ — the promoter "+
		"runs before archive-tail while the change folder is still active")
}

// A root without odd/ is not an odd planning root.
func TestPromoteOddModeRequiresTheOddWorkspace(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes"), 0o755); err != nil {
		t.Fatal(err)
	}
	report := PromoteChangeForMode(root, "any-slug", planbuild.ModeOdd, false)
	wantOneBlocker(t, report, "--root "+root+" is not a planning root: "+filepath.Join(root, "odd")+
		" is missing; pass the resolved planning root, never the process cwd")
}

// A change with no delta composes nothing and raises no blocker.
func TestPromoteOddModeWithoutDeltaIsANoOp(t *testing.T) {
	root := newOddRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "odd", "changes", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	report := PromoteChangeForMode(root, "empty", planbuild.ModeOdd, false)
	if len(report.Blockers) != 0 || len(report.Plans) != 0 {
		t.Fatalf("Blockers = %q, Plans = %d, want none and 0", report.Blockers, len(report.Plans))
	}
}

// Promotion in odd mode is idempotent and reports the rerun as already promoted.
func TestPromoteOddModeIsIdempotent(t *testing.T) {
	root := newOddRepo(t)
	writeOddDelta(t, root, "rerun", "capability", deltaDoc(addedSection(addedBlock)))
	if report := PromoteChangeForMode(root, "rerun", planbuild.ModeOdd, false); len(report.Blockers) != 0 {
		t.Fatalf("first promote Blockers = %q", report.Blockers)
	}
	first := readText(t, oddCanonical(root, "capability"))
	second := PromoteChangeForMode(root, "rerun", planbuild.ModeOdd, false)
	if len(second.Blockers) != 0 {
		t.Fatalf("second promote Blockers = %q", second.Blockers)
	}
	if pending := second.Pending(); len(pending) != 0 {
		t.Fatalf("second promote Pending = %q, want none", pending)
	}
	if got := readText(t, oddCanonical(root, "capability")); got != first {
		t.Fatalf("rerun changed the odd canonical spec")
	}
}

// The promoter and the guardian share one mode decision: in odd mode the parity
// check reads odd/specs, so an odd delta matches its own canonical tree and an
// OpenSpec canonical file can never satisfy it.
func TestCheckFolderParityFollowsTheMode(t *testing.T) {
	root := newOddRepo(t)
	folder := writeOddDelta(t, root, "parity", "capability", deltaDoc(addedSection(addedBlock)))
	if err := os.MkdirAll(filepath.Join(root, "ai-specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ai-specs", "ai-specs.toml"),
		[]byte("[recipes.plan-build-flow]\nenabled = true\n\n[recipes.plan-build-flow.config]\nmode = \"odd\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := CheckFolderParity(root, folder); len(got) != 1 ||
		!strings.Contains(got[0], "is not promoted into odd/specs/capability/spec.md") {
		t.Fatalf("CheckFolderParity before promotion = %q, want the odd-tree blocker", got)
	}
	if report := PromoteChangeForMode(root, "parity", planbuild.ModeOdd, false); len(report.Blockers) != 0 {
		t.Fatalf("promote Blockers = %q", report.Blockers)
	}
	if got := CheckFolderParity(root, folder); len(got) != 0 {
		t.Fatalf("CheckFolderParity after promotion = %q, want none", got)
	}
}

// A task-only ODD change keeps no change folder: promotion is a no-op and never
// a blocker, and it writes neither a change folder nor a canonical tree.
func TestPromoteOddTaskOnlyWithoutChangeFolder(t *testing.T) {
	root := newOddRepo(t)
	plan := []byte("# Task-only work\n")
	path := filepath.Join(root, "odd", "tasks", "task-only.md")
	if err := os.WriteFile(path, plan, 0o644); err != nil {
		t.Fatal(err)
	}
	report := PromoteChangeForMode(root, "task-only", planbuild.ModeOdd, false)
	if len(report.Blockers) != 0 || len(report.Plans) != 0 {
		t.Fatalf("task-only promotion: Blockers = %q, Plans = %d, want none and 0",
			report.Blockers, len(report.Plans))
	}
	if _, err := os.Stat(filepath.Join(root, "odd", "changes", "task-only")); !os.IsNotExist(err) {
		t.Fatalf("odd/changes/task-only was created (err = %v)", err)
	}
}

// A symlinked odd/changes must not feed the composer from the OpenSpec tree.
func TestPromoteOddModeRefusesASymlinkedChangeTree(t *testing.T) {
	root := newOddRepo(t)
	writeDelta(t, root, "leak", "capability", deltaDoc(addedSection(addedBlock)))
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "leak", "specs", "capability"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "openspec", "changes", "leak", "specs", "capability", "spec.md"),
		[]byte(deltaDoc(addedSection(addedBlock))), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, "odd", "changes")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "openspec", "changes"), filepath.Join(root, "odd", "changes")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	report := PromoteChangeForMode(root, "leak", planbuild.ModeOdd, false)
	if len(report.Blockers) != 1 || !strings.Contains(report.Blockers[0], "resolves outside") {
		t.Fatalf("Blockers = %q, want the containment refusal", report.Blockers)
	}
	if _, err := os.Stat(oddCanonical(root, "capability")); !os.IsNotExist(err) {
		t.Fatalf("odd/specs was written through a symlinked change tree (err = %v)", err)
	}
}

// The OpenSpec entry point keeps the OpenSpec roots: the odd delta is invisible
// to it, which is the S9 isolation the mode switch promises.
func TestPromoteOpenSpecModeIgnoresOddDelta(t *testing.T) {
	root := newOddRepo(t)
	writeOddDelta(t, root, "odd-change", "capability", deltaDoc(addedSection(addedBlock)))
	if err := os.MkdirAll(filepath.Join(root, "openspec", "changes", "odd-change"), 0o755); err != nil {
		t.Fatal(err)
	}
	report := PromoteChange(root, "odd-change", false)
	if len(report.Blockers) != 0 || len(report.Plans) != 0 {
		t.Fatalf("Blockers = %q, Plans = %d, want none and 0", report.Blockers, len(report.Plans))
	}
	if _, err := os.Stat(oddCanonical(root, "capability")); !os.IsNotExist(err) {
		t.Fatalf("odd/specs was written by the OpenSpec entry point (err = %v)", err)
	}
}
