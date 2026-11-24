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

// deriveTier reads the artifacts of one change folder (S15): the plan is the
// artifact that asks for ceremony, and a delta alone is a light change because
// S20 allows a delta with no plan.
func deriveTier(folder string) string {
	hasPlan := isFile(filepath.Join(folder, "tasks.md"))
	hasDelta := hasSpecDelta(folder)
	switch {
	case !hasPlan && !hasDelta:
		return TierNone
	case hasPlan && hasDelta:
		return TierStandard
	default:
		return TierLight
	}
}

// declaredTier collects the tier declarations a change may still carry: the
// plan's Depth: line and the --tier option. Both are coherence assertions now,
// never the source of the tier.
func declaredTier(folder string, tier *string) []string {
	var declared []string
	if match := depthRe.FindStringSubmatch(readText(filepath.Join(folder, "tasks.md"))); match != nil {
		declared = append(declared, strings.ToLower(match[1]))
	}
	if tier != nil {
		declared = append(declared, strings.ToLower(*tier))
	}
	return declared
}

// resolveTier derives the tier from the artifacts and blocks every declaration
// that contradicts them. The removed full tier blocks with its migration message
// rather than as an unknown value, because that names the way out.
func resolveTier(folder string, tier *string, location string, blockers *[]string) string {
	derived := deriveTier(folder)
	for _, declared := range declaredTier(folder, tier) {
		switch {
		case declared == derived:
		case declared == TierFull:
			*blockers = append(*blockers, removedTierBlocker(location))
		case declared != TierNone && declared != TierLight && declared != TierStandard:
			*blockers = append(*blockers, fmt.Sprintf(
				"unknown tier '%s' (expected none|light|standard)", declared))
		default:
			*blockers = append(*blockers, fmt.Sprintf(
				"%s: the tier is derived from the artifacts of the change, and they derive '%s', not "+
					"the declared '%s' — %s", location, derived, declared, tierExit(derived)))
		}
	}
	return derived
}

// tierExit names the two ways out of a tier mismatch: complete the artifacts up
// to the declared tier, or drop the stale declaration.
func tierExit(derived string) string {
	switch derived {
	case TierNone:
		return "write tasks.md (light) or tasks.md plus a specs/ delta (standard), or drop the declaration"
	case TierLight:
		return "add a specs/ delta to reach standard, or drop the declaration"
	default:
		return "remove the specs/ delta to reach light, or drop the declaration"
	}
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

// checkPlan requires the ODD-shaped plan (P1) from light up: every archive is
// graded by the grammar, whatever its folder name, and a `## Log` with L1 is
// required at standard. A change with no plan derives `none` and has nothing to
// grade here.
func checkPlan(folder, location, tier string) []string {
	planPath := filepath.Join(folder, "tasks.md")
	if !isFile(planPath) {
		return nil
	}
	plan, blockers := ParsePlan(readText(planPath))
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
	resolved := resolveTier(folder, tier, location, &blockers)
	blockers = append(blockers, checkPlan(folder, location, resolved)...)
	blockers = append(blockers, checkTaskCommits(folder, repoRoot, location)...)
	blockers = append(blockers, checkVerifyEvidence(folder, resolved)...)
	blockers = append(blockers, checkVerificationCommands(repoRoot, filepath.Join(folder, "tasks.md"), resolved)...)
	blockers = append(blockers, promotionBlockers(repoRoot, folder, location)...)
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
// the archived folder once the change is archived — and the change must be gone
// from the active tree behind a dated, content-provable archive
// (plan-build-openspec-odd-align I5/P4). The contract is uniform: a plan with no
// Depth: line pays the same archive duty, and there is no tier artifact minimum.
// An empty change means the branch could not be paired.
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
	blockers = append(blockers, archiveBlockers...)
	if archive != "" {
		blockers = append(blockers, oddParityBlockers(repoRoot, changesBase, archive)...)
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
