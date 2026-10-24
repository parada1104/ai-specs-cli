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

// checkMinimums applies the per-tier artifact minima.
func checkMinimums(folder, tier, location string) []string {
	var blockers []string
	if !isFile(filepath.Join(folder, "tasks.md")) {
		blockers = append(blockers, location+"/tasks.md is required for all tiers")
	}
	hasProposal := isFile(filepath.Join(folder, "proposal.md"))
	switch tier {
	case TierLight:
		if !hasProposal {
			blockers = append(blockers, location+"/proposal.md is required for tier light")
		}
	case TierStandard:
		if !hasProposal {
			blockers = append(blockers, location+"/proposal.md is required for tier standard")
		}
		if !hasSpecDelta(folder) {
			blockers = append(blockers, location+"/specs/ must contain at least one *.md for tier standard")
		}
	default:
		if !hasProposal && !isFile(filepath.Join(folder, "design.md")) {
			blockers = append(blockers, location+"/proposal.md or design.md is required for tier full")
		}
		if !hasSpecDelta(folder) {
			blockers = append(blockers, location+"/specs/ must contain at least one *.md for tier full")
		}
	}
	return blockers
}

// inspectFolder runs the whole gate over one change folder.
func inspectFolder(folder string, tier *string, location, repoRoot string) Result {
	var blockers []string
	tasksText := readText(filepath.Join(folder, "tasks.md"))
	resolved := resolveTier(tasksText, tier, &blockers)
	blockers = append(blockers, checkMinimums(folder, resolved, location)...)
	blockers = append(blockers, checkVerifyEvidence(folder, resolved)...)
	blockers = append(blockers, promotionBlockers(repoRoot, folder, resolved, location)...)
	return Result{OK: len(blockers) == 0, Blockers: blockers, Tier: resolved, ArchivePath: folder}
}

// CheckPrearchive inspects the active openspec/changes/<slug>/ before
// archive-tail.
func CheckPrearchive(repoRoot, slug string, tier *string) Result {
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
// merge.
func CheckPremerge(repoRoot, slug string, tier *string) Result {
	changes := filepath.Join(repoRoot, "openspec", "changes")
	active := filepath.Join(changes, slug)
	var blockers []string
	if isDir(active) {
		blockers = append(blockers, fmt.Sprintf(
			"active change folder still present at openspec/changes/%s/ — "+
				"run archive-tail on the review branch before merge", slug))
	}
	archive, archiveBlockers := resolveArchive(filepath.Join(changes, "archive"), slug)
	blockers = append(blockers, archiveBlockers...)
	if archive == "" {
		return Result{Blockers: blockers, Tier: rawTier(tier)}
	}

	result := inspectFolder(archive, tier, "archive/"+filepath.Base(archive), repoRoot)
	result.Blockers = append(blockers, result.Blockers...)
	result.OK = len(result.Blockers) == 0
	result.ArchivePath = archive
	return result
}

// rawTier is the unresolved option value the short-circuit verdicts report.
func rawTier(tier *string) string {
	if tier == nil {
		return ""
	}
	return *tier
}
