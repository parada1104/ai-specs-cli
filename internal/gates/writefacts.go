package gates

// The write gate's decision, as data (change-ledger-v2 T2, L5/L12): one fact
// collection per pending write, then pure rules over those facts. The refusal and
// `--explain` are both read from the one Verdict this produces, so no path of the
// caller can print one and skip the other, and no rule infers its applicability
// from where a caller returned.
//
// Two facts decide the rules, and each is collected here rather than by a rule or
// a caller:
//
//   - Workstream: a worktree that is not a change at all owes no artifact (S16),
//     so the diff-based evidence rules are skipped for it. It never exempts a rule
//     whose fact is the pending write itself (R1 on the canonical tree, R2).
//   - HasPlan/HasDelta: the artifacts that make the tier, and the exits R1 and R2
//     advertise.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/tier"
)

// WriteFacts is everything one pending write is graded on. It is pure data: no
// field is re-derived by a rule, and none is supplied by the caller except the
// four observations of the event itself.
type WriteFacts struct {
	// Mode is the execution mode the write is graded in.
	Mode string
	// Change is the change the branch pairs, "" when the branch cannot be paired.
	Change string
	// Target is the repository-relative path the write would touch.
	Target string
	// DeltaPath is the repository-relative delta folder promotion would read.
	DeltaPath string
	// PlanPath is the repository-relative plan the change would carry.
	PlanPath string
	// Base is the ref the diff is graded against, "" when no ref resolves.
	Base string
	// Production reports whether Target is inside the graded production scope.
	Production bool
	// HasPlan and HasDelta are the change's artifacts: they make the tier and they
	// are the exits R1 and R2 name.
	HasPlan  bool
	HasDelta bool
	// HighRisk is the project's declared high-risk patterns, read at Base.
	HighRisk []string
	// Diff is the change's diff against Base, and DiffKnown reports whether git
	// answered it at all: an unknown diff is graded, never exempted.
	Diff      []planbuild.DiffEntry
	DiffKnown bool
	// RiskTrailer reports a `Risk: item N` trailer in Base..HEAD (odd mode only,
	// and only when a high-risk pattern is declared).
	RiskTrailer bool
	// Workstream reports that this worktree is not a change (S16).
	Workstream bool
}

// WriteFactsFor collects every fact of one pending write, once.
func WriteFactsFor(repoRoot, mode, change, target string) WriteFacts {
	facts := WriteFacts{
		Mode:       mode,
		Change:     change,
		Target:     target,
		Production: ProductPath(target),
		Base:       BaseRef(repoRoot, nil),
	}
	if change != "" {
		facts.HasPlan = planbuild.PlanIsValidForMode(repoRoot, mode, change)
		facts.HasDelta = planbuild.HasDelta(repoRoot, mode, change)
		facts.DeltaPath = repoRelative(repoRoot,
			filepath.Join(planbuild.ChangeFolder(repoRoot, mode, change), "specs"))
		facts.PlanPath = repoRelative(repoRoot, planbuild.TasksPath(repoRoot, mode, change))
	}
	if facts.Base != "" {
		facts.Diff, facts.DiffKnown = planbuild.DiffEntries(repoRoot, facts.Base)
	}
	facts.HighRisk = HighRiskPaths(repoRoot, facts.Base)
	// The trailer is evidence only for a declared high-risk path, so the git call
	// that reads it costs nothing for a project that declares none.
	if mode == planbuild.ModeOdd && len(facts.HighRisk) > 0 {
		facts.RiskTrailer = planbuild.CommitsWithRiskTrailer(repoRoot, facts.Base)
	}
	facts.Workstream = facts.isWorkstream()
	return facts
}

// Refutation is one refusal: the rule that fired and the fact that proves it,
// with the exits that resolve it.
type Refutation struct {
	Rule    string
	Message string
}

// String is the line the gate prints, so a refusal always names its rule.
func (r Refutation) String() string { return r.Rule + ": " + r.Message }

// Verdict is the one decision of a pending write.
type Verdict struct {
	// Tier is the tier the artifacts infer, never a declared one.
	Tier string
	// Blockers are the refutations the facts prove.
	Blockers []Refutation
}

// Allowed reports whether the write passes.
func (v Verdict) Allowed() bool { return len(v.Blockers) == 0 }

// Rules names the rules that fired, in order, for `--explain`.
func (v Verdict) Rules() string {
	if len(v.Blockers) == 0 {
		return "none"
	}
	rules := make([]string, 0, len(v.Blockers))
	for _, blocker := range v.Blockers {
		if !slicesContain(rules, blocker.Rule) {
			rules = append(rules, blocker.Rule)
		}
	}
	return strings.Join(rules, ", ")
}

// Verdict decides the pending write from its facts alone.
func (f WriteFacts) Verdict() Verdict {
	verdict := Verdict{Tier: tier.None}
	if f.Change != "" {
		verdict.Tier = tier.Derive(f.HasPlan, f.HasDelta)
	}
	verdict.Blockers = append(f.canonicalSpecBlockers(), f.highRiskBlockers()...)
	return verdict
}

// ProductPath reports whether a repository-relative path is inside the graded
// production scope, which PLAN_BUILD_GATE_PATHS overrides.
func ProductPath(rel string) bool {
	dirs := os.Getenv(ProductDirsEnv)
	if strings.ReplaceAll(dirs, " ", "") == "" {
		dirs = ProductDirs
	}
	first, _, _ := strings.Cut(rel, "/")
	for _, dir := range strings.Fields(dirs) {
		if first == dir {
			return true
		}
	}
	return false
}

// isWorkstream is the S16 fact: a worktree with no artifact and no product-path
// divergence is not a change at all. It is conservative — any divergence, an
// unpaired branch or a diff git could not answer keeps the worktree graded.
func (f WriteFacts) isWorkstream() bool {
	if f.Change == "" || f.HasPlan || f.HasDelta || !f.DiffKnown {
		return false
	}
	for _, entry := range f.Diff {
		if ProductPath(entry.Path) {
			return false
		}
	}
	return true
}

// canonicalSpecBlockers is R1: the canonical spec tree is written by promotion,
// so a write that changes it without a delta behind it is a bypass.
func (f WriteFacts) canonicalSpecBlockers() []Refutation {
	spec, pending, ok := f.canonicalSpecFact()
	if !ok {
		return nil
	}
	if pending {
		// The pending write is the fact; a delta in the tree is the exit, whether
		// or not this branch committed it.
		if f.HasDelta {
			return nil
		}
		return []Refutation{{Rule: "R1", Message: CanonicalSpecRefusal(f.Mode, f.Change, spec)}}
	}
	_, count := firstMatch(f.Diff, func(entry planbuild.DiffEntry) bool { return isCanonicalSpec(entry.Path) })
	return []Refutation{{Rule: "R1", Message: fmt.Sprintf(
		"the branch modifies the canonical spec `%s`%s but the change `%s` carries no delta — "+
			"changing a canonical spec without a delta is a promotion bypass. Write the delta under "+
			"`%s/` and run `ai-specs specs promote`.", spec, andMore(count), f.Change, f.DeltaPath)}}
}

// canonicalSpecFact names the canonical spec the write would change and whether it
// is the pending target (the fact of this write) rather than a committed path.
func (f WriteFacts) canonicalSpecFact() (spec string, pending, ok bool) {
	if isCanonicalSpec(f.Target) {
		return f.Target, true, true
	}
	// A committed canonical edit is graded for a change only: a workstream owes no
	// artifact (S16), so no delta could answer it.
	if f.Workstream || f.Change == "" {
		return "", false, false
	}
	changed, _ := firstMatch(f.Diff, func(entry planbuild.DiffEntry) bool { return isCanonicalSpec(entry.Path) })
	return changed, false, changed != ""
}

// highRiskBlockers is R2: a path the project declares as high-risk owes the
// evidence the mode requires. The pending target is graded wherever it lives, so
// a clean branch cannot write one before its evidence exists.
func (f WriteFacts) highRiskBlockers() []Refutation {
	if len(f.HighRisk) == 0 {
		return nil
	}
	matches := func(rel string) bool {
		for _, pattern := range f.HighRisk {
			if matchesHighRisk(pattern, rel) {
				return true
			}
		}
		return false
	}
	path, count := firstMatch(f.Diff, func(entry planbuild.DiffEntry) bool { return matches(entry.Path) })
	if path == "" && f.Target != "" && matches(f.Target) {
		path, count = f.Target, 1
	}
	if path == "" {
		return nil
	}
	found := fmt.Sprintf("the branch modifies the high-risk path `%s`%s, declared by the project in "+
		"ai-specs/ai-specs.toml", path, andMore(count))
	if f.Mode == planbuild.ModeOdd {
		if f.RiskTrailer {
			return nil
		}
		return []Refutation{{Rule: "R2", Message: found + highRiskOddExit(f.Base)}}
	}
	// A plan from an earlier change never counts: only a plan this diff contributes
	// is evidence, or a committed plan would answer for every change.
	if planInDiff(f.Change, f.Diff) {
		return nil
	}
	plan := f.PlanPath
	return []Refutation{{Rule: "R2", Message: found + fmt.Sprintf(", but the change `%s` carries no "+
		"plan — in openspec the plan is where the verification evidence of a high-risk change lives. "+
		"Write `%s` (tier light) and retry.", f.Change, plan)}}
}

// highRiskOddExit is the odd-mode exit R2 advertises: the trailer, and the six
// items it names.
func highRiskOddExit(base string) string {
	return fmt.Sprintf(", but no commit of `%s..HEAD` carries a `Risk: item N (<reason>)` "+
		"trailer — the trailer is the verification evidence an ODD high-risk change must carry, and a "+
		"bare `Risk: none` does not satisfy it. N is one of: 1 data or irreversible effects that "+
		"cannot be rebuilt, 2 security, 3 contracts others already consume, 4 concurrency, "+
		"5 delivery or environment, 6 no test would catch a regression. "+
		"Add the trailer to one commit of the range and retry.", base)
}

// slicesContain reports whether values holds one value.
func slicesContain(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
