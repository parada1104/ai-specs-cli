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
	"testing"
)

// --- fixture vocabulary (the Python module's _requirement/_canonical/_delta) ---

func TestEmptyDeltaIsANoOp(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	before := readText(t, canonical)
	writeDelta(t, root, "empty-delta", "capability", "# Delta for capability\n")

	report := mustPromote(t, root, "empty-delta")

	if report.Changed() {
		t.Error("Changed() = true, want false")
	}
	if got := readText(t, canonical); got != before {
		t.Errorf("canonical = %q, want unchanged %q", got, before)
	}
}

// --- refusal paths ---------------------------------------------------------

func TestRenamedOperationIsUnsupported(t *testing.T) {
	root := newRepo(t)
	canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock))
	before := readText(t, canonical)
	renamed := "## RENAMED Requirements\n\n- FROM: `### Requirement: Existing requirement`\n" +
		"  TO: `### Requirement: Renamed requirement`\n"
	writeDelta(t, root, "renamed", "capability", deltaDoc(renamed))

	report := PromoteChange(root, "renamed", false)

	wantOneBlocker(t, report,
		"specs/capability/spec.md: ## RENAMED Requirements is unsupported: the archive contract "+
			"defines ADDED/MODIFIED/REMOVED only — rewrite the delta as REMOVED plus ADDED before promoting")
	if got := readText(t, canonical); got != before {
		t.Errorf("blocked promotion wrote the canonical file: %q, want %q", got, before)
	}
}

func TestDeltaSectionRejections(t *testing.T) {
	cases := []struct {
		name  string
		delta string
		want  string
	}{
		{
			name:  "unsupported section label",
			delta: deltaDoc("## DELETED Requirements\n\n" + existingBlock),
			want:  "specs/capability/spec.md: ## DELETED Requirements is unsupported",
		},
		{
			name:  "duplicate operation for one requirement",
			delta: deltaDoc(addedSection(existingBlock), modifiedSection(existingBlock)),
			want: `specs/capability/spec.md: duplicate delta operation for requirement ` +
				`"Existing requirement" (ADDED and MODIFIED)`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newRepo(t)
			canonical := writeCanonical(t, root, "capability", canonicalDoc(existingBlock, untouchedBlock))
			before := readText(t, canonical)
			writeDelta(t, root, "rejected", "capability", tc.delta)

			report := PromoteChange(root, "rejected", false)

			wantOneBlocker(t, report, tc.want)
			if got := readText(t, canonical); got != before {
				t.Errorf("blocked promotion wrote the canonical file: %q, want %q", got, before)
			}
		})
	}
}
