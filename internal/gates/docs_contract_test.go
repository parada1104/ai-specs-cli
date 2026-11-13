package gates

// Characterization port of CanonicalOwnershipContractTests from the retired
// Python premerge guardian suite. W6: the canonical specs pin the post-split
// ownership, not the fused one.
//
// The three canonical specs are the durable contract. These regressions keep
// Plan Build owning verify -> promotion -> read-only guardian -> archive, the
// Tracker domain host owning `pre-merge`/`archive-close` independently of
// Plan Build/OpenSpec archive, and VCS owning transport/review/merge/cleanup
// only -- while no stale text still says the artifact guardian hosts tracker
// checkpoints.
//
// Unlike the rest of the gates tests these read the real repository documents,
// so they are repository-text contracts rather than fixture-driven checks.

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// docsRepoRoot walks up from this source file to the directory holding go.mod.
func docsRepoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve docs_contract_test.go location")
	}
	dir, err := filepath.Abs(filepath.Dir(thisFile))
	if err != nil {
		t.Fatalf("abs %s: %v", thisFile, err)
	}
	for {
		if fi, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !fi.IsDir() {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", filepath.Dir(thisFile))
		}
		dir = parent
	}
}

func docsRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// docsNorm collapses wrapping so an assertion pins the clause, not the line breaks.
func docsNorm(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func docsMustContain(t *testing.T, text, want string) {
	t.Helper()
	if !strings.Contains(text, want) {
		t.Fatalf("missing %q", want)
	}
}

func docsMustNotContain(t *testing.T, text, unwanted string) {
	t.Helper()
	if strings.Contains(text, unwanted) {
		t.Fatalf("unexpected %q", unwanted)
	}
}

func docsPath(t *testing.T, root string, parts ...string) string {
	t.Helper()
	return filepath.Join(append([]string{root}, parts...)...)
}

func docsPlanBuildSpec(t *testing.T) string {
	t.Helper()
	return docsPath(t, docsRepoRoot(t), "openspec", "specs", "plan-build-flow", "spec.md")
}

func docsTrackerLedgerSpec(t *testing.T) string {
	t.Helper()
	return docsPath(t, docsRepoRoot(t), "openspec", "specs", "tracker-ledger", "spec.md")
}

func docsVCSPrSpec(t *testing.T) string {
	t.Helper()
	return docsPath(t, docsRepoRoot(t), "openspec", "specs", "vcs-pr-flow", "spec.md")
}

// --- plan-build-flow: verify -> promote -> guardian -> archive ---

func TestDocsContractPlanBuildOwnsTheOrderedReviewBranchTail(t *testing.T) {
	text := docsNorm(docsRead(t, docsPlanBuildSpec(t)))
	docsMustContain(t, text,
		"verify evidence \u2192 promote canonical delta specs \u2192 run the "+
			"read-only artifact guardian \u2192 archive the change folder")
}

func TestDocsContractPlanBuildGuardianValidatesPromotionParityReadOnly(t *testing.T) {
	text := docsNorm(docsRead(t, docsPlanBuildSpec(t)))
	docsMustContain(t, text, "promotion parity")
	docsMustContain(t, text, "read-only")
	docsMustContain(t, text, "MUST NOT write, rewrite, promote, or repair canonical specs")
	// The pre-merge guardian hard-blocker list carries the parity blocker.
	docsMustContain(t, text, "promotion parity check")
}

func TestDocsContractPlanBuildKeepsLightAndOddNoSpecBehaviorExplicit(t *testing.T) {
	text := docsNorm(docsRead(t, docsPlanBuildSpec(t)))
	docsMustContain(t, text, "Light")
	docsMustContain(t, text, "ODD task-only")
	docsMustContain(t, text, "no spec deltas")
}

func TestDocsContractPlanBuildGuardianIsNotOfferedToVCSMergeSkills(t *testing.T) {
	text := docsNorm(docsRead(t, docsPlanBuildSpec(t)))
	docsMustNotContain(t, text, "or a VCS merge skill")
}

// --- tracker-ledger: autonomous core + Tracker domain port ---

func TestDocsContractTrackerGateDirectModeHostsThePreMergeAndArchiveCloseCheckpoints(t *testing.T) {
	text := docsNorm(docsRead(t, docsTrackerLedgerSpec(t)))
	// The canonical contract names the live shell bridge in direct host mode.
	docsMustContain(t, text,
		"`pre-merge` and `archive-close` \u2192 the `tracker-card-gate.sh` shell "+
			"bridge in direct host mode "+
			"(`--root <root> --checkpoint pre-merge|archive-close`)")
	// ... and no longer names the retired Python host anywhere.
	docsMustNotContain(t, text, "tracker_ledger_host.py")
}

func TestDocsContractTrackerLedgerSpecNeverPinsCheckpointsToTheGuardian(t *testing.T) {
	text := docsRead(t, docsTrackerLedgerSpec(t))
	docsMustNotContain(t, text, "premerge_guardian")
	docsMustNotContain(t, text, "pre-merge guardian")
}

func TestDocsContractTrackerClosureIsIndependentOfTheOpenSpecArchive(t *testing.T) {
	text := docsNorm(docsRead(t, docsTrackerLedgerSpec(t)))
	docsMustContain(t, text, "independent of OpenSpec archive")
	docsMustContain(t, text, "no `openspec/` tree")
}

func TestDocsContractTrackerDomainPortAndDeclarativeAdaptersAreSpecified(t *testing.T) {
	text := docsNorm(docsRead(t, docsTrackerLedgerSpec(t)))
	docsMustContain(t, text, "### Requirement: Tracker domain port with declarative provider adapters")
	docsMustContain(t, text, "[config.reconcile]")
	docsMustContain(t, text, "MUST NOT enable, disable, or change ledger behavior")
	docsMustContain(t, text, "no provider may be guessed")
}

func TestDocsContractNoHostResolvesConfigFromAHardcodedLiteral(t *testing.T) {
	text := docsNorm(docsRead(t, docsTrackerLedgerSpec(t)))
	docsMustContain(t, text, "No host MAY resolve its config section from a hardcoded")
	docsMustContain(t, text,
		"the plan-build work-start gate, the tracker gate (including its direct "+
			"`--root <root> --checkpoint pre-merge|archive-close` host mode), and doctor")
}

// --- vcs-pr-flow: transport/review/merge/cleanup only ---

func TestDocsContractVCSOwnsTransportWithoutSDDCeremony(t *testing.T) {
	text := docsNorm(docsRead(t, docsVCSPrSpec(t)))
	docsMustContain(t, text, "### Requirement: VCS transport owns PR/MR, review, and cleanup only")
	docsMustContain(t, text, "MUST NOT require, produce, schedule, or validate SDD/OpenSpec planning or archive artifacts")
	docsMustContain(t, text, "MUST NOT call the artifact guardian itself")
}

func TestDocsContractVCSSpecKeepsProviderNeutralSafety(t *testing.T) {
	text := docsNorm(docsRead(t, docsVCSPrSpec(t)))
	for _, phrase := range []string{"no auto-merge", "protected heads", "post-merge cleanup"} {
		docsMustContain(t, text, phrase)
	}
	docsMustNotContain(t, text, "premerge_guardian")
}

// --- no stale host reference anywhere in the shipped surface ---

func TestDocsContractTrackerGateCommentsNameTheDirectCheckpointHost(t *testing.T) {
	root := docsRepoRoot(t)
	text := docsRead(t, docsPath(t, root, "catalog", "recipes", "trello-mcp-workflow", "hooks", "tracker-card-gate.sh"))
	docsMustNotContain(t, text, "pre-merge guardian")
	docsMustContain(t, text, "--checkpoint pre-merge|archive-close")
	// The retired Python host may survive only as a historical removal note.
	if strings.Contains(text, "tracker_ledger_host.py") {
		docsMustContain(t, text, "retired")
	}
}

func TestDocsContractUnreleasedChangelogRecordsThePythonHostRemoval(t *testing.T) {
	changelog := docsRead(t, docsPath(t, docsRepoRoot(t), "CHANGELOG.md"))
	unreleased := strings.SplitN(changelog, "## [0.22.0]", 2)[0]
	// Historical removal note only: the entry names the retired module and the
	// shell bridge that now hosts the checkpoints, never a live Python host.
	docsMustContain(t, unreleased, "`lib/_internal/tracker_ledger_host.py` Python host is removed")
	docsMustContain(t, unreleased, "--checkpoint pre-merge|archive-close")
	docsMustNotContain(t, unreleased, "pre-merge guardian")
}

func TestDocsContractNoShippedRecipeClaimsTheGuardianHostsTrackerCheckpoints(t *testing.T) {
	root := docsRepoRoot(t)
	recipes := docsPath(t, root, "catalog", "recipes")
	var offenders []string
	err := filepath.WalkDir(recipes, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".sh") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(data)
		if strings.Contains(text, "pre-merge guardian") || strings.Contains(text, "premerge_guardian") {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				return relErr
			}
			offenders = append(offenders, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", recipes, err)
	}
	if len(offenders) != 0 {
		t.Fatalf("offenders = %q, want none: no tracker host may claim the Plan Build artifact guardian grades it", offenders)
	}
}
