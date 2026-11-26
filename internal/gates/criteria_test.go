package gates

// The legacy criteria source survives the removal of the full tier: when a plan
// declares no `## Specs`, requirements still fall back to the `## Success
// Criteria` block of proposal.md, then design.md. The full-tier mapping rules
// are gone with the tier.

import (
	"path/filepath"
	"testing"
)

func TestRequirementsFallBackToProposalSuccessCriteria(t *testing.T) {
	folder := t.TempDir()
	writeFile(t, filepath.Join(folder, "proposal.md"),
		"# proposal\n\n## Success Criteria\n- [ ] First criterion\n- [ ] Second criterion\n")

	declared, issue := parseRequirementsFor(folder, "## Tasks\n")
	if issue != "" {
		t.Fatalf("issue = %q, want none", issue)
	}
	if len(declared.ids) != 2 || declared.ids[0] != "Criterion 1" ||
		declared.ids[1] != "Criterion 2" || declared.source != "proposal.md" {
		t.Fatalf("declared = %+v, want Criterion 1-2 from proposal.md", declared)
	}
}

func TestRequirementsUseDesignOnlyWhenProposalIsAbsent(t *testing.T) {
	folder := t.TempDir()
	writeFile(t, filepath.Join(folder, "design.md"),
		"# design\n\n## Success Criteria\n- [ ] Design criterion\n")

	declared, issue := parseRequirementsFor(folder, "## Tasks\n")
	if issue != "" || len(declared.ids) != 1 || declared.source != "design.md" {
		t.Fatalf("declared = %+v (issue %q), want one criterion from design.md", declared, issue)
	}
}

func TestRequirementsNameTheMissingSource(t *testing.T) {
	folder := t.TempDir()
	_, issue := parseRequirementsFor(folder, "## Tasks\n")
	want := "requirements are missing: add at least one `S#` under `## Specs` in tasks.md, " +
		"or a non-empty ## Success Criteria section in proposal.md or design.md"
	if issue != want {
		t.Fatalf("issue = %q, want %q", issue, want)
	}
}

func TestRequirementsRejectDuplicateSuccessCriteriaHeadings(t *testing.T) {
	folder := t.TempDir()
	writeFile(t, filepath.Join(folder, "proposal.md"),
		"# proposal\n\n## Success Criteria\n- [ ] First\n\n## Success Criteria\n- [ ] Second\n")

	_, issue := parseRequirementsFor(folder, "## Tasks\n")
	if issue != "verify-report.md has duplicate ## Success Criteria headings in proposal.md" {
		t.Fatalf("issue = %q, want the duplicate-heading blocker", issue)
	}
}
