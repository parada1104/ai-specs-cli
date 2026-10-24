package specs

// Behaviour tests for the canonical spec promoter, the port of
// lib/_internal/spec_promotion.py (GO-20 U2e, unit U-A). Each case maps to one
// case of tests/test_spec_promotion.py; the expected bytes come from the Python
// implementation, so the port is compared against its oracle, not against a
// re-description of it.
//
// The promoter writes files, so every case runs on its own t.TempDir() tree:
// openspec/changes/<slug>/specs/<domain>/spec.md (the delta) is composed into
// openspec/specs/<domain>/spec.md (the canonical spec).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- fixture vocabulary (the Python module's _requirement/_canonical/_delta) ---

func TestRerunIsIdempotentAndResumeSafe(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	writeDelta(t, root, "rerun", "capability", deltaDoc(addedSection(addedBlock)))

	mustPromote(t, root, "rerun")
	afterFirst := readText(t, canonical)

	second := PromoteChange(root, "rerun", false)
	if len(second.Blockers) != 0 {
		t.Fatalf("second Blockers = %q, want none", second.Blockers)
	}
	if second.Changed() {
		t.Error("second Changed() = true, want false")
	}
	if got := readText(t, canonical); got != afterFirst {
		t.Errorf("second run rewrote the canonical file: %q, want %q", got, afterFirst)
	}
	if got := strings.Join(second.Applied(), "|"); got != "capability: ADDED Added requirement (already canonical)" {
		t.Errorf("Applied() = %q, want the already-canonical item", got)
	}
	if got := CheckFolderParity(root, second.Folder); len(got) != 0 {
		t.Errorf("CheckFolderParity after rerun = %q, want none", got)
	}
	entries, err := os.ReadDir(filepath.Dir(canonical))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Name() != "spec.md" {
			t.Errorf("leftover temp file in the canonical directory: %q", entry.Name())
		}
	}
}

func TestRemovedIsRefusedWithoutTheExplicitSafePath(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock, untouchedBlock))
	before := readText(t, canonical)
	writeDelta(t, root, "destructive", "capability", deltaDoc(removedSection(existingBlock)))

	report := PromoteChange(root, "destructive", false)

	wantOneBlocker(t, report,
		`specs/capability/spec.md: REMOVED requirement "Existing requirement" is destructive and `+
			"needs the explicit approval path; re-run the promoter with --allow-removed after the removal "+
			"was approved")
	if got := readText(t, canonical); got != before {
		t.Errorf("blocked promotion wrote the canonical file: %q, want %q", got, before)
	}

	allowed := PromoteChange(root, "destructive", true)
	if len(allowed.Blockers) != 0 {
		t.Fatalf("allow_removed Blockers = %q, want none", allowed.Blockers)
	}
	if got := strings.Join(allowed.Pending(), "|"); got != "capability: REMOVED Existing requirement" {
		t.Errorf("allow_removed Pending() = %q, want the REMOVED item", got)
	}
	if got, want := readText(t, canonical), canonicalDoc(untouchedBlock); got != want {
		t.Errorf("canonical after approved removal = %q, want %q", got, want)
	}
}

func TestOneBlockedDomainAbortsEveryWrite(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	before := readText(t, canonical)
	writeDelta(t, root, "atomic", "capability", deltaDoc(addedSection(addedBlock)))
	writeDelta(t, root, "atomic", "other-domain",
		deltaDoc(modifiedSection(requirement("Absent requirement", "body"))))

	report := PromoteChange(root, "atomic", false)

	if len(report.Blockers) == 0 {
		t.Fatal("Blockers = none, want the other-domain blocker")
	}
	if got := readText(t, canonical); got != before {
		t.Errorf("blocked promotion wrote the clean domain's canonical file: %q, want %q", got, before)
	}
	if _, err := os.Stat(canonicalPath(root, "other-domain")); !os.IsNotExist(err) {
		t.Errorf("other-domain canonical spec exists despite the blocker (err = %v)", err)
	}
}

// --- active same-domain collisions -----------------------------------------

func TestActiveSameDomainCollisionBlocksBeforeAnyWrite(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	before := readText(t, canonical)
	writeDelta(t, root, "selected", "capability", deltaDoc(addedSection(addedBlock)))
	writeDelta(t, root, "sibling", "capability", deltaDoc(addedSection(addedBlock)))

	report := PromoteChange(root, "selected", false)

	wantOneBlocker(t, report,
		`active change "sibling" also declares spec delta specs/capability/spec.md — the collision `+
			"is unresolved; choose an explicit sync/archive order or narrow one change before promoting")
	if got := readText(t, canonical); got != before {
		t.Errorf("blocked promotion wrote the canonical file: %q, want %q", got, before)
	}
}

func TestActiveCollisionOnOneDomainBlocksEveryDomain(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	before := readText(t, canonical)
	writeDelta(t, root, "selected", "capability", deltaDoc(addedSection(addedBlock)))
	writeDelta(t, root, "selected", "clean-domain", deltaDoc(addedSection(addedBlock)))
	writeDelta(t, root, "sibling", "capability", deltaDoc(addedSection(addedBlock)))

	report := PromoteChange(root, "selected", false)

	if len(report.Blockers) == 0 {
		t.Fatal("Blockers = none, want the sibling collision")
	}
	if got := readText(t, canonical); got != before {
		t.Errorf("blocked promotion wrote the canonical file: %q, want %q", got, before)
	}
	if _, err := os.Stat(canonicalPath(root, "clean-domain")); !os.IsNotExist(err) {
		t.Errorf("clean-domain canonical spec exists despite the blocker (err = %v)", err)
	}
}

func TestArchivedUnrelatedAndSpecLessChangesDoNotCollide(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	writeDelta(t, root, "selected", "capability", deltaDoc(addedSection(addedBlock)))
	// A closed change on the same domain is history, not a competing order.
	archived := filepath.Join(root, "openspec", "changes", "archive", "closed", "specs", "capability")
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatal(err)
	}
	archivedDelta := filepath.Join(archived, "spec.md")
	if err := os.WriteFile(archivedDelta, []byte(deltaDoc(addedSection(addedBlock))), 0o644); err != nil {
		t.Fatal(err)
	}
	// A sibling on another domain and a sibling with no deltas are unrelated.
	writeDelta(t, root, "other-domain", "elsewhere", deltaDoc(addedSection(addedBlock)))
	taskOnly := filepath.Join(root, "openspec", "changes", "task-only")
	if err := os.MkdirAll(taskOnly, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskOnly, "tasks.md"), []byte("Depth: light\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := mustPromote(t, root, "selected")

	if got := report.Pending(); len(got) != 1 {
		t.Errorf("Pending() = %q, want one item", got)
	}
	want := canonicalDoc(existingBlock) + "\n" + addedBlock
	if got := readText(t, canonical); got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
}

func TestSymlinkedActiveChangeIsNotFollowedByTheCollisionScan(t *testing.T) {
	root := newRepo(t)
	outsideDelta := filepath.Join(t.TempDir(), "outside-change", "specs", "capability", "spec.md")
	if err := os.MkdirAll(filepath.Dir(outsideDelta), 0o755); err != nil {
		t.Fatal(err)
	}
	content := deltaDoc(addedSection(addedBlock))
	if err := os.WriteFile(outsideDelta, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(filepath.Dir(outsideDelta)), filepath.Join(root, "openspec", "changes", "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	writeDelta(t, root, "selected", "capability", deltaDoc(addedSection(addedBlock)))

	mustPromote(t, root, "selected")

	if _, err := os.Stat(canonicalPath(root, "capability")); err != nil {
		t.Errorf("canonical spec not written: %v", err)
	}
	if got := readText(t, outsideDelta); got != content {
		t.Errorf("outside delta was modified: %q, want %q", got, content)
	}
}

// --- boundaries ------------------------------------------------------------

func TestMissingChangeFolderIsRejected(t *testing.T) {
	root := newRepo(t)

	report := PromoteChange(root, "ghost", false)

	wantOneBlocker(t, report, "active change folder missing at openspec/changes/ghost/ — the promoter "+
		"runs before archive-tail while the change folder is still active")
}

func TestMissingSpecsDirectoryIsANoOp(t *testing.T) {
	root := newRepo(t)
	folder := filepath.Join(root, "openspec", "changes", "odd-task-only")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "tasks.md"), []byte("Depth: light\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report := PromoteChange(root, "odd-task-only", false)

	if len(report.Blockers) != 0 {
		t.Fatalf("Blockers = %q, want none", report.Blockers)
	}
	if report.Changed() {
		t.Error("Changed() = true, want false")
	}
	if got := CheckFolderParity(root, folder); len(got) != 0 {
		t.Errorf("CheckFolderParity = %q, want none", got)
	}
	entries, err := os.ReadDir(filepath.Join(root, "openspec", "specs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("promotion wrote into openspec/specs: %v", entries)
	}
}

// TestCheckFolderParityReportsUnpromotedDelta is the read-only half the Plan
// Build guardian uses: the same composition, reported instead of written.
func TestCheckFolderParityReportsUnpromotedDelta(t *testing.T) {
	root := newRepo(t)
	writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	folder := writeDelta(t, root, "pending-change", "capability", deltaDoc(addedSection(addedBlock)))

	want := "specs/capability/spec.md is not promoted into openspec/specs/capability/spec.md " +
		"(ADDED Added requirement); run the canonical spec promoter " +
		"(lib/_internal/spec_promotion.py) before archive"
	got := CheckFolderParity(root, folder)
	if len(got) != 1 || got[0] != want {
		t.Fatalf("CheckFolderParity = %q, want [%q]", got, want)
	}
}
