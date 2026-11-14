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
	"strings"
	"testing"
)

// --- fixture vocabulary (the Python module's _requirement/_canonical/_delta) ---

func TestAddedRequirementIsAppendedIntoExistingCanonical(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	writeDelta(t, root, "add-spec", "capability", deltaDoc(addedSection(addedBlock)))

	report := mustPromote(t, root, "add-spec")

	want := canonicalDoc(existingBlock) + "\n" + addedBlock
	if got := readText(t, canonical); got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
	if !report.Changed() {
		t.Error("Changed() = false, want true")
	}
	if got := strings.Join(report.Pending(), "|"); got != "capability: ADDED Added requirement" {
		t.Errorf("Pending() = %q, want the ADDED item", got)
	}
	if got := CheckFolderParity(root, report.Folder); len(got) != 0 {
		t.Errorf("CheckFolderParity after promotion = %q, want none", got)
	}
}

func TestModifiedRequirementReplacesCanonicalBlock(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock, untouchedBlock))
	replacement := requirementScenario("Existing requirement", "Rewritten body.", "rewritten")
	writeDelta(t, root, "mod-spec", "capability", deltaDoc(modifiedSection(replacement)))

	mustPromote(t, root, "mod-spec")

	want := canonicalDoc(replacement, untouchedBlock)
	if got := readText(t, canonical); got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
}

func TestNewDomainCreatesCanonicalSpecFromTheDelta(t *testing.T) {
	root := newRepo(t)
	writeDelta(t, root, "new-domain", "fresh-domain", deltaDoc(addedSection(addedBlock)))

	mustPromote(t, root, "new-domain")

	got := readText(t, canonicalPath(root, "fresh-domain"))
	want := "# fresh-domain Specification\n\n## Requirements\n\n" + addedBlock
	if got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
	for _, forbidden := range []string{"## ADDED Requirements", "# Delta for"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("canonical contains delta scaffolding %q: %q", forbidden, got)
		}
	}
}

func TestNewDomainCarriesTheDeltaPurpose(t *testing.T) {
	root := newRepo(t)
	delta := "# Delta for fresh\n\n## Purpose\n\nDelta purpose.\n\n" + addedSection(addedBlock)
	writeDelta(t, root, "purpose-domain", "fresh", delta)

	mustPromote(t, root, "purpose-domain")

	want := "# fresh Specification\n\n## Purpose\n\nDelta purpose.\n\n## Requirements\n\n" + addedBlock
	if got := readText(t, canonicalPath(root, "fresh")); got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
}

func TestUnrelatedCanonicalRequirementsAndSectionsArePreserved(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability",
		canonicalDoc(existingBlock, untouchedBlock)+"\n\n## Notes\n\nCanonical note.\n")
	writeDelta(t, root, "preserve", "capability", deltaDoc(addedSection(addedBlock)))

	mustPromote(t, root, "preserve")

	want := canonicalDoc(existingBlock, untouchedBlock) + "\n" + addedBlock + "\n## Notes\n\nCanonical note.\n"
	if got := readText(t, canonical); got != want {
		t.Errorf("canonical = %q, want %q", got, want)
	}
}

func TestAddedCollisionWithDifferentContentIsRejected(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	before := readText(t, canonical)
	writeDelta(t, root, "collision", "capability",
		deltaDoc(addedSection(requirement("Existing requirement", "Conflicting body."))))

	report := PromoteChange(root, "collision", false)

	wantOneBlocker(t, report,
		`specs/capability/spec.md: ADDED requirement "Existing requirement" collides with an `+
			"existing canonical requirement with different content; use MODIFIED for an intentional "+
			"replacement, or rename the requirement")
	if got := readText(t, canonical); got != before {
		t.Errorf("blocked promotion wrote the canonical file: %q, want %q", got, before)
	}
}

func TestMissingModifiedTargetIsRejected(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	before := readText(t, canonical)
	writeDelta(t, root, "orphan-mod", "capability",
		deltaDoc(modifiedSection(requirement("Absent requirement", "New body."))))

	report := PromoteChange(root, "orphan-mod", false)

	wantOneBlocker(t, report,
		`specs/capability/spec.md: MODIFIED requirement "Absent requirement" does not exist in `+
			"the canonical spec; there is nothing safe to replace")
	if got := readText(t, canonical); got != before {
		t.Errorf("blocked promotion wrote the canonical file: %q, want %q", got, before)
	}
}

func TestModifiedWithoutCanonicalSpecIsRejected(t *testing.T) {
	root := newRepo(t)
	writeDelta(t, root, "no-canonical", "capability",
		deltaDoc(modifiedSection(requirement("Absent requirement", "body"))))

	report := PromoteChange(root, "no-canonical", false)

	wantOneBlocker(t, report,
		`specs/capability/spec.md: MODIFIED requirement "Absent requirement" has no canonical `+
			"spec to modify; MODIFIED requires an existing openspec/specs/<domain>/spec.md")
	if _, err := os.Stat(canonicalPath(root, "capability")); !os.IsNotExist(err) {
		t.Errorf("canonical spec was created despite the blocker (err = %v)", err)
	}
}

// TestCompositionKeepsCanonicalBytesForCRLFInput pins the universal-newline
// normalization both implementations share: a CRLF delta composes to the same
// canonical bytes as its LF twin.
func TestCompositionKeepsCanonicalBytesForCRLFInput(t *testing.T) {
	crlf := strings.ReplaceAll(deltaDoc(addedSection(addedBlock)), "\n", "\r\n")
	canonical := canonicalDoc(existingBlock)

	got := Compose(&canonical, crlf, "capability", false)

	if len(got.Blockers) != 0 {
		t.Fatalf("Blockers = %q, want none", got.Blockers)
	}
	want := canonicalDoc(existingBlock) + "\n" + addedBlock
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}

// TestAppendCreatesAMissingRequirementsSection covers the canonical spec that
// has no `## Requirements` heading at all: the delta's ADDED blocks are appended
// under a new heading instead of being dropped.
func TestAppendCreatesAMissingRequirementsSection(t *testing.T) {
	canonical := "# capability Specification\n\n## Purpose\n\nExisting purpose.\n"

	got := Compose(&canonical, deltaDoc(addedSection(addedBlock)), "capability", false)

	if len(got.Blockers) != 0 {
		t.Fatalf("Blockers = %q, want none", got.Blockers)
	}
	want := "# capability Specification\n\n## Purpose\n\nExisting purpose.\n\n## Requirements\n\n" + addedBlock
	if got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
}
