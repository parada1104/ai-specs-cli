package gates

// The merge-time refutations and the posture (change-ledger-v2 T8, L5): R3 the size
// and module threshold, R4 the removed public contract, R5 the plan whose `S#` rows
// carry no evidence. They are NOT part of WriteFacts.Verdict(): the write gate (the
// hook path) keeps R1/R2 and pays no extra git call, and these three are graded where
// a merge is decided.
//
// Two declarations govern them, both under [recipes.plan-build-flow.config] of
// ai-specs/ai-specs.toml, and neither is declared in the recipe's own schema (that
// schema models scalars only):
//
//   - require: the checkpoints whose posture is `require` instead of `advise`.
//   - risk_thresholds: `lines` and `modules`, either may be omitted.
//
// A malformed value is a note and the CLI default stays, so a declaration that could
// not be read is never mistaken for one that was.

import (
	"fmt"
	"slices"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/toml"
)

const (
	// RequireKey is the manifest key listing the checkpoints whose posture is require.
	RequireKey = "require"
	// RiskThresholdsKey is the manifest key declaring the project's thresholds.
	RiskThresholdsKey = "risk_thresholds"
	// CheckpointMerge is the only checkpoint a posture governs: it is the transition
	// of a phase that is heading to a merge (open/planned). The other checkpoint
	// names (archive-close) are the tracker's, not a posture's.
	CheckpointMerge = "merge"
)

// AcceptedCheckpoints is the closed set of posture-governed checkpoints, in the order
// a note names them.
var AcceptedCheckpoints = []string{CheckpointMerge}

// Thresholds are the size limits R3 grades against.
type Thresholds struct {
	Lines   int
	Modules int
}

// DefaultThresholds are the CLI's: the master plan's per-PR cap and a module count an
// ordinary change does not exceed.
var DefaultThresholds = Thresholds{Lines: 1000, Modules: 5}

// Posture is the project's declaration: the checkpoints it requires, the thresholds
// it grades with, and the notes naming what could not be read.
type Posture struct {
	Require    []string
	Thresholds Thresholds
	Notes      []string
}

// Requires reports whether the posture governs this checkpoint. An empty checkpoint
// (a phase with no transition, or a workstream, S16) is never governed.
func (p Posture) Requires(checkpoint string) bool {
	return checkpoint != "" && slices.Contains(p.Require, checkpoint)
}

// LoadPosture reads the project's posture from the working tree. An absent manifest or
// key declares nothing, which is an answer: advise everywhere and the CLI thresholds.
func LoadPosture(root string) Posture {
	posture := Posture{Thresholds: DefaultThresholds}
	manifest, err := config.LoadManifest(planbuild.ManifestPath(root))
	if err != nil {
		return posture
	}
	_, _, cfg := config.RecipeConfigState(manifest, planbuild.RecipeID)
	posture.Require = loadRequire(cfg, &posture.Notes)
	posture.Thresholds = loadThresholds(cfg, &posture.Notes)
	return posture
}

// loadRequire reads `require`: an accepted checkpoint name is applied, anything else
// is a note naming the accepted list.
func loadRequire(cfg *toml.Table, notes *[]string) []string {
	value, ok := config.LookupNestedConfig(cfg, RequireKey)
	if !ok {
		return nil
	}
	raw, isArray := value.([]any)
	if !isArray {
		*notes = append(*notes, fmt.Sprintf(
			"`%s` in %s is not an array of checkpoint names (got %T), so no posture is required and "+
				"`advise` stays the one applied; the accepted names are: %s",
			RequireKey, planbuild.ManifestRelPath, value, strings.Join(AcceptedCheckpoints, ", ")))
		return nil
	}
	var require []string
	for index, item := range raw {
		name, isString := item.(string)
		if !isString || !slices.Contains(AcceptedCheckpoints, name) {
			*notes = append(*notes, fmt.Sprintf(
				"`%s` entry %d in %s (%v) is not an accepted checkpoint name, so it was not applied; "+
					"the accepted names are: %s", RequireKey, index, planbuild.ManifestRelPath, item,
				strings.Join(AcceptedCheckpoints, ", ")))
			continue
		}
		require = append(require, name)
	}
	return require
}

// loadThresholds reads `risk_thresholds`: a missing key keeps the default, a
// non-positive or non-integer value is a note and keeps it too.
func loadThresholds(cfg *toml.Table, notes *[]string) Thresholds {
	thresholds := DefaultThresholds
	value, ok := config.LookupNestedConfig(cfg, RiskThresholdsKey)
	if !ok {
		return thresholds
	}
	table, isTable := value.(*toml.Table)
	if !isTable {
		*notes = append(*notes, fmt.Sprintf(
			"`%s` in %s is not a table of integers (got %T), so the CLI defaults stay "+
				"(`lines = %d`, `modules = %d`)",
			RiskThresholdsKey, planbuild.ManifestRelPath, value, thresholds.Lines, thresholds.Modules))
		return thresholds
	}
	for _, key := range []string{"lines", "modules"} {
		raw, exists := table.Get(key)
		if !exists {
			continue
		}
		number, isInt := thresholdValue(raw)
		if !isInt || number <= 0 {
			*notes = append(*notes, fmt.Sprintf(
				"`%s.%s` in %s (%v) is not a positive integer, so the CLI default for `%s` stays",
				RiskThresholdsKey, key, planbuild.ManifestRelPath, raw, key))
			continue
		}
		if key == "modules" {
			thresholds.Modules = number
		} else {
			thresholds.Lines = number
		}
	}
	return thresholds
}

// thresholdValue reads one declared threshold: the parser yields int64 for a TOML
// integer, and int is accepted so a hand-built table grades the same.
func thresholdValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case int64:
		return int(value), true
	case int:
		return value, true
	}
	return 0, false
}

// MergeFacts is what the merge-time rules read: the size of the product diff, the
// declared high-risk paths the diff removes, and the change's own plan.
type MergeFacts struct {
	// Lines is added plus deleted lines over product paths, and Modules the distinct
	// first path segments of those paths (`.` for a root file). SizeKnown reports
	// whether git answered the numstat at all: an unanswered count is a note, never a
	// pass and never a block.
	Lines     int
	Modules   int
	SizeKnown bool
	// Specs is how many `S#` requirements the change's plan declares, and
	// Verification is the guardian's own verdict on its `## Verification` rows.
	Specs        int
	Verification []string
	// Removed names the deleted diff paths that match a declared high-risk pattern.
	Removed []string
}

// MergeFactsFor collects the merge-time facts once. It collects nothing for a
// worktree no rule governs — a workstream or an unpaired branch owes no posture
// (S16) — so the extra git call is paid only where a rule can fire.
func MergeFactsFor(root string, facts WriteFacts) MergeFacts {
	merge := MergeFacts{}
	if facts.Change == "" || facts.Workstream {
		return merge
	}
	if lines, ok := planbuild.DiffLines(root, facts.Base); ok {
		merge.SizeKnown = true
		modules := make(map[string]bool, len(lines))
		for rel, count := range lines {
			if !GradedPath(rel) || !facts.Scope.Path(rel) {
				continue
			}
			merge.Lines += count
			modules[firstSegment(rel)] = true
		}
		merge.Modules = len(modules)
	}
	for _, entry := range facts.Diff {
		if entry.Status != "D" || !highRiskPattern(facts.HighRisk, entry.Path) {
			continue
		}
		merge.Removed = append(merge.Removed, entry.Path)
	}
	if facts.HasPlan {
		planPath := planbuild.TasksPath(root, facts.Mode, facts.Change)
		plan, _ := ParsePlan(readText(planPath))
		merge.Specs = len(plan.Specs)
		merge.Verification = checkPlanVerification(planPath)
	}
	return merge
}

// MergeRefutations decides R3-R5 from the facts alone. A workstream or an unpaired
// branch is never graded (S16), and each rule keeps its own precondition: R3 and R4
// only where no artifact makes the tier above `none`, R5 only where a plan declares
// at least one `S#`.
func (f WriteFacts) MergeRefutations(merge MergeFacts, thresholds Thresholds) []Refutation {
	if f.Workstream || f.Change == "" {
		return nil
	}
	var refutations []Refutation
	if !f.HasPlan && !f.HasDelta {
		refutations = append(refutations, f.sizeRefutation(merge, thresholds)...)
		refutations = append(refutations, f.removedContractRefutation(merge)...)
	}
	return append(refutations, f.verificationRefutation(merge)...)
}

// MergeNotes names the facts the merge-time rules could not observe, so a rule that
// could not run is never indistinguishable from a rule that found nothing.
func (f WriteFacts) MergeNotes(merge MergeFacts) []string {
	if f.Workstream || f.Change == "" || merge.SizeKnown {
		return nil
	}
	return []string{"R3 was not applied: `git diff --numstat` against the base produced no usable " +
		"answer (a shallow clone, no base ref, or no such history in this root), so the changed line " +
		"and module counts of the product paths are unknown and the size threshold was not graded."}
}

// sizeRefutation is R3: a `none` change over the project's line or module threshold.
// It counts product paths only, so a documentation or planning worktree never trips
// it (T2 r1).
func (f WriteFacts) sizeRefutation(merge MergeFacts, thresholds Thresholds) []Refutation {
	if !merge.SizeKnown {
		return nil
	}
	var exceeded []string
	if merge.Lines > thresholds.Lines {
		exceeded = append(exceeded, fmt.Sprintf("%d changed line(s), over the project threshold "+
			"`lines = %d`", merge.Lines, thresholds.Lines))
	}
	if merge.Modules > thresholds.Modules {
		exceeded = append(exceeded, fmt.Sprintf("%d module(s), over the project threshold "+
			"`modules = %d`", merge.Modules, thresholds.Modules))
	}
	if len(exceeded) == 0 {
		return nil
	}
	return []Refutation{{Rule: "R3", Message: fmt.Sprintf(
		"the change `%s` carries no plan and no delta but changes %s. Write the change's plan `%s` "+
			"(tier light, which makes this threshold stop applying), or raise `%s` in %s.",
		f.Change, strings.Join(exceeded, " and "), f.PlanPath, RiskThresholdsKey,
		planbuild.ManifestRelPath)}}
}

// removedContractRefutation is R4: a deleted path the project declares as high-risk,
// with no plan recording the removal.
func (f WriteFacts) removedContractRefutation(merge MergeFacts) []Refutation {
	if len(merge.Removed) == 0 {
		return nil
	}
	return []Refutation{{Rule: "R4", Message: fmt.Sprintf(
		"the change `%s` deletes the high-risk path `%s`%s, declared by the project in %s, and "+
			"carries no plan — removing a public contract is what a plan records. Write the change's "+
			"plan `%s` (tier light) naming the removal.",
		f.Change, merge.Removed[0], andMore(len(merge.Removed)), planbuild.ManifestRelPath, f.PlanPath)}}
}

// verificationRefutation is R5: a plan that declares `S#` requirements and whose
// `## Verification` rows do not grade them.
func (f WriteFacts) verificationRefutation(merge MergeFacts) []Refutation {
	if !f.HasPlan || merge.Specs == 0 || len(merge.Verification) == 0 {
		return nil
	}
	return []Refutation{{Rule: "R5", Message: fmt.Sprintf(
		"the plan `%s` declares %d `S#` requirement(s) and its `## Verification` does not grade them: "+
			"%s. Add one `## Verification` row per `S#` with the command it ran "+
			"(\"- **S1: PASS** — `go test ./...`, exit 0\"), or `N/A` with its reason.",
		f.PlanPath, merge.Specs, firstFew(merge.Verification, 3))}}
}

// highRiskPattern reports whether a repository-relative path matches one of the
// declared high-risk patterns.
func highRiskPattern(patterns []string, rel string) bool {
	for _, pattern := range patterns {
		if matchesHighRisk(pattern, rel) {
			return true
		}
	}
	return false
}

// firstSegment is the module of a repository-relative path: `.` for a root file.
func firstSegment(rel string) string {
	if at := strings.Index(rel, "/"); at >= 0 {
		return rel[:at]
	}
	return "."
}

// firstFew joins the first shown items so one plan cannot print a page.
func firstFew(items []string, shown int) string {
	if len(items) <= shown {
		return strings.Join(items, "; ")
	}
	return fmt.Sprintf("%s; +%d more", strings.Join(items[:shown], "; "), len(items)-shown)
}
