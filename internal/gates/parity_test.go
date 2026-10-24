package gates

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ai-specs.dev/ai-specs/internal/specs"
)

// --- promotion parity (W3) ---

var (
	canonicalSpec = "# capability Specification\n\n## Purpose\n\nExisting purpose.\n\n" +
		"## Requirements\n\n" + requirementFixture("Existing requirement", "Existing body.")

	promotedDelta = "# Delta for capability\n\n## ADDED Requirements\n\n" +
		requirementFixture("Added requirement", "Added body.")

	unresolvedDelta = "# Delta for capability\n\n## MODIFIED Requirements\n\n" +
		requirementFixture("Absent requirement", "New body.")
)

func requirementFixture(name, body string) string {
	return "### Requirement: " + name + "\n\n" + body + "\n\n" +
		"#### Scenario: " + name + "\n\n- **WHEN** " + name + "\n- **THEN** " + name + "\n"
}

func parityRepo(t *testing.T) string {
	t.Helper()
	root := newRepo(t)
	mkdirAll(t, filepath.Join(root, "openspec", "specs"))
	return root
}

func parityActive(t *testing.T, root, slug, delta, tier string, evidence *string) string {
	t.Helper()
	folder := filepath.Join(root, "openspec", "changes", slug)
	mkdirAll(t, folder)
	writeFile(t, filepath.Join(folder, "tasks.md"), "Depth: "+tier+"\n")
	writeFile(t, filepath.Join(folder, "proposal.md"), "# proposal\n")
	writeFile(t, filepath.Join(folder, "specs", "capability", "spec.md"), delta)
	if evidence != nil {
		writeFile(t, filepath.Join(folder, "verify-report.md"), *evidence)
	}
	return folder
}

func parityArchive(t *testing.T, root, slug, delta, tier string, evidence *string) string {
	t.Helper()
	active := parityActive(t, root, slug, delta, tier, evidence)
	target := filepath.Join(root, "openspec", "changes", "archive", slug)
	mkdirAll(t, filepath.Dir(target))
	if err := os.Rename(active, target); err != nil {
		t.Fatalf("archive %s: %v", slug, err)
	}
	return target
}

func promote(t *testing.T, root, slug, delta string) {
	t.Helper()
	parityActive(t, root, slug, delta, "standard", nil)
	report := specs.PromoteChange(root, slug, false)
	if len(report.Blockers) != 0 {
		t.Fatalf("promoter blockers: %v", report.Blockers)
	}
}

func filterPromotionBlockers(blockers []string) []string {
	var found []string
	for _, blocker := range blockers {
		if strings.Contains(blocker, "spec_promotion") || strings.Contains(strings.ToLower(blocker), "canonical") {
			found = append(found, blocker)
		}
	}
	return found
}

func TestPrearchiveBlocksAnUnpromotedStandardDelta(t *testing.T) {
	root := parityRepo(t)
	parityActive(t, root, "unpromoted", promotedDelta, "standard", repoString(standardReport))

	result := CheckPrearchive(root, "unpromoted", tierOpt("standard"))
	if result.OK {
		t.Fatal("an unpromoted Standard delta must block")
	}
	if len(filterPromotionBlockers(result.Blockers)) == 0 {
		t.Fatalf("blockers %q are not promotion blockers", result.Blockers)
	}
	for _, blocker := range filterPromotionBlockers(result.Blockers) {
		if !strings.HasPrefix(blocker, "changes/unpromoted: ") {
			t.Fatalf("promotion blocker %q lacks the location prefix", blocker)
		}
	}
}

func TestPrearchivePassesWhenTheDeltaIsPromoted(t *testing.T) {
	root := parityRepo(t)
	promote(t, root, "promoted", promotedDelta)
	parityActive(t, root, "promoted", promotedDelta, "standard", repoString(standardReport))
	if result := CheckPrearchive(root, "promoted", tierOpt("standard")); !result.OK {
		t.Fatalf("promoted change = %+v, want OK", result)
	}
}

func TestPremergeBlocksAnUnpromotedArchivedDelta(t *testing.T) {
	root := parityRepo(t)
	parityArchive(t, root, "merged-unpromoted", promotedDelta, "standard", repoString(standardReport))
	result := CheckPremerge(root, "merged-unpromoted", tierOpt("standard"))
	if result.OK {
		t.Fatal("an unpromoted archived delta must block")
	}
	if len(filterPromotionBlockers(result.Blockers)) == 0 {
		t.Fatalf("blockers %q are not promotion blockers", result.Blockers)
	}
}

func TestPremergePassesForAPromotedStandardChange(t *testing.T) {
	root := parityRepo(t)
	promote(t, root, "merged-promoted", promotedDelta)
	parityArchive(t, root, "merged-promoted", promotedDelta, "standard", repoString(standardReport))
	if result := CheckPremerge(root, "merged-promoted", tierOpt("standard")); !result.OK {
		t.Fatalf("promoted archive = %+v, want OK", result)
	}
}

func TestPremergePassesForAPromotedFullChange(t *testing.T) {
	root := parityRepo(t)
	evidence := "## Verify evidence\n- Verdict: PASS\n- Command: ./tests/run.sh\n" +
		"- Exit: 0\n- Date: 2026-08-07\n- Commit: 1234567\n" +
		"- ready_for_archive: true\n\n" +
		"## Success-criteria mapping\n- Criterion 1: PASS — criterion one\n"
	promote(t, root, "merged-full", promotedDelta)
	archived := parityArchive(t, root, "merged-full", promotedDelta, "full", repoString(evidence))
	writeFile(t, filepath.Join(archived, "proposal.md"),
		"# proposal\n\n## Success Criteria\n\n- first criterion\n")
	if result := CheckPremerge(root, "merged-full", tierOpt("full")); !result.OK {
		t.Fatalf("promoted full archive = %+v, want OK", result)
	}
}

func TestUnresolvedDeltaBlocksEvenWithAConformingReport(t *testing.T) {
	root := parityRepo(t)
	parityActive(t, root, "unresolved", unresolvedDelta, "standard", repoString(standardReport))
	writeFile(t, filepath.Join(root, "openspec", "specs", "capability", "spec.md"), canonicalSpec)

	result := CheckPrearchive(root, "unresolved", tierOpt("standard"))
	if result.OK || len(filterPromotionBlockers(result.Blockers)) == 0 {
		t.Fatalf("unresolved delta = %+v, want promotion blockers", result)
	}
}

func TestRenamedDeltaIsReportedAsUnresolved(t *testing.T) {
	root := parityRepo(t)
	renamed := "# Delta for capability\n\n## RENAMED Requirements\n\n" +
		"- FROM: `### Requirement: Existing requirement`\n" +
		"  TO: `### Requirement: Renamed requirement`\n"
	parityActive(t, root, "renamed", renamed, "standard", repoString(standardReport))
	writeFile(t, filepath.Join(root, "openspec", "specs", "capability", "spec.md"), canonicalSpec)

	result := CheckPrearchive(root, "renamed", tierOpt("standard"))
	if result.OK {
		t.Fatal("a RENAMED delta must block")
	}
	found := false
	for _, blocker := range result.Blockers {
		if strings.Contains(blocker, "RENAMED") {
			found = true
		}
	}
	if !found {
		t.Fatalf("blockers %q do not name RENAMED", result.Blockers)
	}
}

func TestLightChangeWithoutSpecsIsUnaffected(t *testing.T) {
	root := parityRepo(t)
	folder := filepath.Join(root, "openspec", "changes", "light-change")
	mkdirAll(t, folder)
	writeFile(t, filepath.Join(folder, "tasks.md"), "Depth: light\n")
	writeFile(t, filepath.Join(folder, "proposal.md"), "# proposal\n")

	if result := CheckPrearchive(root, "light-change", tierOpt("light")); !result.OK {
		t.Fatalf("light pre-archive = %+v, want OK", result)
	}
	target := filepath.Join(root, "openspec", "changes", "archive", "light-change")
	if err := os.Rename(folder, target); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if result := CheckPremerge(root, "light-change", tierOpt("light")); !result.OK {
		t.Fatalf("light pre-merge = %+v, want OK", result)
	}
	entries, err := os.ReadDir(filepath.Join(root, "openspec", "specs"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("openspec/specs = %v (%v), want empty", entries, err)
	}
}

func TestGuardianNeverWritesCanonicalSpecsOrTempFiles(t *testing.T) {
	root := parityRepo(t)
	parityActive(t, root, "read-only", promotedDelta, "standard", repoString(standardReport))

	if result := CheckPrearchive(root, "read-only", tierOpt("standard")); result.OK {
		t.Fatal("expected the promotion blocker")
	}
	if _, err := os.Stat(filepath.Join(root, "openspec", "specs", "capability")); !os.IsNotExist(err) {
		t.Fatalf("guardian created a canonical domain: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "openspec", "specs"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("openspec/specs = %v (%v), want empty", entries, err)
	}
}

func TestGuardianEvaluatesOnlyRequestedSlug(t *testing.T) {
	root := newRepo(t)
	archiveWith(t, root, "target", "Depth: light\n", archiveOpts{proposal: true})
	archiveWith(t, root, "old-nonconforming", "Depth: standard\n", archiveOpts{})
	if result := CheckPremerge(root, "target", tierOpt("light")); !result.OK {
		t.Fatalf("target = %+v, want OK", result)
	}
}

func TestBlocksWhenActiveFolderAndArchiveBothReport(t *testing.T) {
	root := newRepo(t)
	slug := "both"
	archiveWith(t, root, slug, "Depth: light\n", archiveOpts{proposal: true})
	activeWith(t, root, slug, "Depth: light\n", archiveOpts{proposal: true})

	result := CheckPremerge(root, slug, tierOpt("light"))
	if result.OK {
		t.Fatal("expected blockers")
	}
	assertBlockerContains(t, result, "active change folder still present")
}
