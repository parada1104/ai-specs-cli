// Package gates is the pre-merge guardian for planning artifacts and staged
// verification evidence. It is the port of lib/_internal/premerge_guardian.py
// (GO-20 U2e, unit U-B) and keeps its blockers byte for byte.
//
// The guardian is read-only: it validates tier minima, staged verification
// evidence and canonical/delta spec promotion parity. It never writes canonical
// specs and never touches tracker state; promotion is the separate explicit
// writer in internal/specs.
package gates

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/specs"
)

// depthRe keeps the exact standalone-line contract: plan annotations must not
// suffix it.
var depthRe = regexp.MustCompile(`(?im)^\s*Depth:\s*(light|standard|full)\s*$`)

// Result is one guardian verdict.
type Result struct {
	OK          bool
	Blockers    []string
	Tier        string // resolved tier; the raw option value when the check short-circuits
	ArchivePath string // inspected folder, empty when no archive resolved
}

// inferTier reads the Depth: line, defaulting to standard.
func inferTier(tasksText string) string {
	if match := depthRe.FindStringSubmatch(tasksText); match != nil {
		return strings.ToLower(match[1])
	}
	return TierStandard
}

// declaredTier reports the tier the plan declares on its Depth: line, and
// whether it declares one at all. It exists so a legacy plan without the line
// keeps the narrower contract instead of inheriting the standard default.
func declaredTier(tasksText string) (string, bool) {
	match := depthRe.FindStringSubmatch(tasksText)
	if match == nil {
		return "", false
	}
	return strings.ToLower(match[1]), true
}

// resolveTier applies an explicit --tier when given, blocking an unknown value
// and then falling back to the inference.
func resolveTier(tasksText string, tier *string, blockers *[]string) string {
	if tier == nil {
		return inferTier(tasksText)
	}
	resolved := strings.ToLower(*tier)
	if resolved != TierLight && resolved != TierStandard && resolved != TierFull {
		*blockers = append(*blockers, fmt.Sprintf(
			"unknown tier '%s' (expected light|standard|full)", *tier))
		return inferTier(tasksText)
	}
	return resolved
}

// hasSpecDelta reports whether the change folder carries any *.md delta.
func hasSpecDelta(folder string) bool {
	specsDir := filepath.Join(folder, "specs")
	if info, err := os.Stat(specsDir); err != nil || !info.IsDir() {
		return false
	}
	found := false
	_ = filepath.WalkDir(specsDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || found {
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			found = true
		}
		return nil
	})
	return found
}

// checkMinimums applies the per-tier artifact minima (P6): the plan always, a
// delta from standard up. proposal.md, design.md and verify-report.md are still
// accepted and honoured when present; they are simply not required.
func checkMinimums(folder, tier, location string) []string {
	var blockers []string
	if !isFile(filepath.Join(folder, "tasks.md")) {
		blockers = append(blockers, location+"/tasks.md is required for all tiers")
	}
	if tier == TierLight {
		return blockers
	}
	if !hasSpecDelta(folder) {
		blockers = append(blockers, location+"/specs/ must contain at least one *.md for tier "+tier)
	}
	return blockers
}

// checkPlan requires the ODD-shaped plan (plan-build-openspec-odd-align P1) for
// every tier: every archive is graded by the grammar, whatever its folder name,
// and a `## Log` with L1 is required from standard up (P6). Light keeps it
// optional.
func checkPlan(folder, location, tier string) []string {
	plan, blockers := ParsePlan(readText(filepath.Join(folder, "tasks.md")))
	if tier != TierLight && !plan.HasLog {
		blockers = append(blockers, "`## Log` section is missing: add `- **L1** — <the human "+
			"request>` because tier "+tier+" requires it")
	}
	located := make([]string, 0, len(blockers))
	for _, blocker := range blockers {
		located = append(located, location+"/tasks.md: "+blocker)
	}
	return located
}

// inspectFolder runs the whole gate over one change folder.
func inspectFolder(folder string, tier *string, location, repoRoot string) Result {
	var blockers []string
	tasksText := readText(filepath.Join(folder, "tasks.md"))
	resolved := resolveTier(tasksText, tier, &blockers)
	blockers = append(blockers, checkMinimums(folder, resolved, location)...)
	blockers = append(blockers, checkPlan(folder, location, resolved)...)
	blockers = append(blockers, checkTaskCommits(folder, repoRoot, location)...)
	blockers = append(blockers, checkVerifyEvidence(folder, resolved)...)
	blockers = append(blockers, checkProjectConfig(repoRoot, filepath.Join(folder, "tasks.md"), resolved)...)
	blockers = append(blockers, promotionBlockers(repoRoot, folder, resolved, location)...)
	return Result{OK: len(blockers) == 0, Blockers: blockers, Tier: resolved, ArchivePath: folder}
}

// CheckPrearchive inspects the active openspec/changes/<slug>/ before
// archive-tail.
func CheckPrearchive(repoRoot, slug string, tier *string) Result {
	slug = planbuild.NormalizeChange(slug)
	active := filepath.Join(repoRoot, "openspec", "changes", slug)
	if !isDir(active) {
		return Result{
			Blockers:    []string{"active change folder missing at openspec/changes/" + slug + "/"},
			Tier:        rawTier(tier),
			ArchivePath: active,
		}
	}
	return inspectFolder(active, tier, "changes/"+slug, repoRoot)
}

// CheckPremerge inspects the archived artifacts and staged evidence before
// merge. A branch-shaped slug (CI passes GITHUB_HEAD_REF) keeps its last segment.
func CheckPremerge(repoRoot, slug string, tier *string) Result {
	slug = planbuild.NormalizeChange(slug)
	changes := filepath.Join(repoRoot, "openspec", "changes")
	active := filepath.Join(changes, slug)
	var blockers []string
	if isDir(active) {
		blockers = append(blockers, fmt.Sprintf(
			"active change folder still present at openspec/changes/%s/ — "+
				"run archive-tail on the review branch before merge", slug))
	}
	archive, archiveBlockers := resolveArchive(filepath.Join(changes, "archive"), slug, "openspec")
	blockers = append(blockers, archiveBlockers...)
	if archive == "" {
		return Result{Blockers: blockers, Tier: rawTier(tier)}
	}

	// Every archive is graded the same way: the folder name never decides which
	// rules apply, so an undated archive cannot bypass the plan grammar.
	result := inspectFolder(archive, tier, "archive/"+filepath.Base(archive), repoRoot)
	result.Blockers = append(blockers, result.Blockers...)
	result.OK = len(result.Blockers) == 0
	result.ArchivePath = archive
	return result
}

// TierOdd is the guardian's label in odd mode: there is no Plan Build depth
// there, because ODD owns planning.
const TierOdd = "odd"

// CheckPremergeOdd validates an ODD change before merge: the branch-paired plan
// must be a regular file of the repository, every delta under
// odd/changes/<change>/specs/ must already be promoted into odd/specs/ — read from
// the archived folder once the change is archived — and the active delta folder
// must be archived (plan-build-openspec-odd-align I5/P4). A change that never
// carried a delta folder is not required to be archived: there is nothing to
// archive, so the paired plan alone passes. There is no OpenSpec archive and no
// tier artifact minimum. An empty change means the branch could not be paired.
// The mode is resolved by the caller, which is the same resolver the pre-tool-use
// gate uses.
func CheckPremergeOdd(repoRoot, change string) Result {
	change = planbuild.NormalizeChange(change)
	if change == "" {
		return Result{Blockers: []string{"ODD mode is active but the current branch cannot be " +
			"resolved (detached HEAD), so no change can be paired"}, Tier: TierOdd}
	}
	plan := planbuild.PlanPath(repoRoot, change)
	var blockers []string
	if !planbuild.PlanIsValid(repoRoot, change) {
		blockers = append(blockers, fmt.Sprintf(
			"ODD plan 'odd/tasks/%s.md' is missing, is not a regular file, or resolves outside the "+
				"repository for the current branch", change))
	}
	changesBase := planbuild.ChangesBase(repoRoot, planbuild.ModeOdd)
	folder := filepath.Join(changesBase, change)
	if info, err := os.Stat(folder); err == nil && info.IsDir() {
		blockers = append(blockers, fmt.Sprintf(
			"active change folder still present at odd/changes/%s/ — run `ai-specs changes archive` "+
				"on the review branch before merge", change))
	}
	// Every delta the change carried must be promoted, whether it is still active
	// or already archived: after archiving, the archived folder is the only
	// remaining source of that delta (R3).
	blockers = append(blockers, oddParityBlockers(repoRoot, changesBase, folder)...)
	archive, archiveBlockers := resolveArchive(planbuild.ArchiveBase(repoRoot, planbuild.ModeOdd),
		change, "odd")
	if archive != "" {
		blockers = append(blockers, oddParityBlockers(repoRoot, changesBase, archive)...)
	}
	// A plan that declares its tier also requires the change to be archived with
	// something of the change in it. A plan with no Depth: line is legacy and keeps
	// the #589 contract (plan, parity and no archive duty), so existing ODD
	// repositories do not retro-block. There is no project config in odd mode.
	if declared, ok := declaredTier(readText(plan)); ok {
		blockers = append(blockers, archiveBlockers...)
		// The plan lives at odd/tasks/<change>.md, so it is graded by the same
		// plan-verification grader the OpenSpec path uses (C3).
		if declared != TierLight {
			blockers = append(blockers, checkPlanVerification(plan, declared)...)
		}
	}
	return Result{OK: len(blockers) == 0, Blockers: blockers, Tier: TierOdd, ArchivePath: plan}
}

// oddParityBlockers is the promotion parity of one odd change folder, refused when
// the folder is reached through a symlinked change tree.
func oddParityBlockers(repoRoot, changesBase, folder string) []string {
	if !specs.SourceContained(folder, changesBase) {
		return []string{fmt.Sprintf(
			"refusing to read the change folder %s: it is reached through a symlinked odd change tree, "+
				"so the OpenSpec tree would become the parity source", folder)}
	}
	return specs.CheckFolderParityForMode(repoRoot, folder, planbuild.ModeOdd)
}

// rawTier is the unresolved option value the short-circuit verdicts report.
func rawTier(tier *string) string {
	if tier == nil {
		return ""
	}
	return *tier
}
