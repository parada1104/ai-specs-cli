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

// hasSpecDelta reports whether the change folder carries any *.md delta, through
// the one predicate the gate shares.
func hasSpecDelta(folder string) bool {
	return planbuild.HasDeltaDir(folder)
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
// The optional base is the --base override the refutations of a `none` change are
// graded against. S16: a change with no folder, no archive entry and no plan is a
// valid `none` declaration, so it passes unless the refutations of its diff block it.
func CheckPremerge(repoRoot, slug string, tier *string,
	// Variadic so the call sites that predate --base keep compiling; omitting it
	// falls back to the resolved base ref, never to "no refutations".
	base ...*string) Result {
	slug = planbuild.NormalizeChange(slug)
	changes := filepath.Join(repoRoot, "openspec", "changes")
	active := filepath.Join(changes, slug)
	activePresent := isDir(active)
	var blockers []string
	if activePresent {
		blockers = append(blockers, fmt.Sprintf(
			"active change folder still present at openspec/changes/%s/ — "+
				"run archive-tail on the review branch before merge", slug))
	}
	archive, present, archiveBlockers := resolveArchive(filepath.Join(changes, "archive"), slug, "openspec")
	refutations := Refutations(repoRoot, planbuild.ModeOpenspec, slug, baseOption(base))
	// S16: no folder and no archive entry at all is the valid `none` declaration.
	// A symlinked entry is still an entry, so the resolver keeps naming it. The
	// declared depth still has to cohere with the derivation, or an explicit
	// `--tier standard` would pass silently as none.
	if !activePresent && !present && planbuild.ArchiveEntry(repoRoot, planbuild.ModeOpenspec, slug) == "" {
		resolved := resolveTier(active, tier, "changes/"+slug, &blockers)
		blockers = append(blockers, refutations...)
		return Result{OK: len(blockers) == 0, Blockers: blockers, Tier: resolved}
	}
	blockers = append(blockers, archiveBlockers...)
	if archive == "" {
		blockers = append(blockers, refutations...)
		return Result{Blockers: blockers, Tier: rawTier(tier)}
	}

	// Every archive is graded the same way: the folder name never decides which
	// rules apply, so an undated archive cannot bypass the plan grammar.
	result := inspectFolder(archive, tier, "archive/"+filepath.Base(archive), repoRoot)
	result.Blockers = append(blockers, result.Blockers...)
	result.Blockers = append(result.Blockers, refutations...)
	result.OK = len(result.Blockers) == 0
	result.ArchivePath = archive
	return result
}

// baseOption is the optional --base of the premerge verbs.
func baseOption(base []*string) *string {
	if len(base) == 0 {
		return nil
	}
	return base[0]
}

// TierOdd is the guardian's label in odd mode: there is no Plan Build depth
// there, because ODD owns planning.
const TierOdd = "odd"

// CheckPremergeOdd validates an ODD change before merge: the branch-paired plan
// must be a regular file of the repository, every delta under
// odd/changes/<change>/specs/ must already be promoted into odd/specs/ — read from
// the archived folder once the change is archived — and a change that still
// carries its folder must be gone from the active tree behind a dated,
// content-provable archive (plan-build-openspec-odd-align I5/P4, S21).
// S20 allows a change whose only artifact is a delta, and S21 makes the archive a
// duty only of a change that carries a folder: a plan-only change owes nothing
// there because the plan stays in odd/tasks/ as the permanent record, and a
// change with no artifact at all is `none` and passes.
// An empty change means the branch could not be paired.
// The mode is resolved by the caller, which is the same resolver the pre-tool-use
// gate uses.
func CheckPremergeOdd(repoRoot, change string,
	// Variadic so the call sites that predate --base keep compiling; omitting it
	// falls back to the resolved base ref, never to "no refutations".
	base ...*string) Result {
	change = planbuild.NormalizeChange(change)
	if change == "" {
		return Result{Blockers: []string{"ODD mode is active but the current branch cannot be " +
			"resolved (detached HEAD), so no change can be paired"}, Tier: TierOdd}
	}
	changesBase := planbuild.ChangesBase(repoRoot, planbuild.ModeOdd)
	folder := filepath.Join(changesBase, change)
	active := isDir(folder)
	validPlan := planbuild.PlanIsValid(repoRoot, change)

	var blockers []string
	if active {
		blockers = append(blockers, fmt.Sprintf(
			"active change folder still present at odd/changes/%s/ — run `ai-specs changes archive` "+
				"on the review branch before merge", change))
		blockers = append(blockers, oddParityBlockers(repoRoot, changesBase, folder)...)
	}
	archive, present, archiveBlockers := resolveArchive(planbuild.ArchiveBase(repoRoot, planbuild.ModeOdd),
		change, "odd")
	// An existing archive is still graded, so an empty archive cannot pass as
	// "no archive owed".
	if active || present {
		blockers = append(blockers, archiveBlockers...)
	}
	// After archiving, the archived folder is the only remaining source of the
	// delta, so its parity is checked there too (R3).
	if archive != "" {
		blockers = append(blockers, oddParityBlockers(repoRoot, changesBase, archive)...)
	}
	tier := TierNone
	if active || present || validPlan {
		tier = TierOdd
	}
	blockers = append(blockers, Refutations(repoRoot, planbuild.ModeOdd, change, baseOption(base))...)
	archivePath := archive
	if archivePath == "" && validPlan {
		archivePath = planbuild.PlanPath(repoRoot, change)
	}
	return Result{OK: len(blockers) == 0, Blockers: blockers, Tier: tier, ArchivePath: archivePath}
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
