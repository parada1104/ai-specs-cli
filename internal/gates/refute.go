package gates

// The objective refutations of a `none` change (plan-build-openspec-odd-align
// S16/S17). The CLI cannot prove an ODD small task, so the absence of a plan is
// a valid declaration: it is only refuted by contradictions of the diff itself.
// Two refutations exist, both keyed on something explicit and project-owned:
//
//   - R-a: the branch changes a canonical spec (odd/specs/** or openspec/specs/**)
//     while the change carries no delta. That is a promotion bypass.
//   - R-b: the branch changes a path the project declares as high-risk. In
//     openspec a plan is the only place plan-build keeps verification evidence,
//     so the change must carry one; in odd the evidence is the `Risk: item N`
//     trailer of a commit in the range.
//
// Nothing here counts files, commits or lines: ODD forbids those heuristics.

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/toml"
)

// GitPRRecipeID is the recipe whose config declares the project base branch.
const GitPRRecipeID = "git-pr-flow"

// HighRiskPaths reads the project's explicit high-risk list from
// [recipes.plan-build-flow.config] high_risk_paths in ai-specs/ai-specs.toml at
// the trusted base revision the diff is graded against, never the working tree,
// so a candidate cannot delete a pattern to stop being graded by it. The key is
// not declared in the recipe's recipe.toml: that schema is frozen by a test, and
// the mode is read the same undeclared way. Any other shape than an array of
// strings yields no list, so no declaration means no R-b refusal.
func HighRiskPaths(root, base string) []string {
	text, ok := planbuild.FileAt(root, base, planbuild.ManifestRelPath)
	if !ok {
		return nil
	}
	manifest, err := toml.Parse([]byte(text))
	if err != nil {
		return nil
	}
	_, _, cfg := config.RecipeConfigState(manifest, planbuild.RecipeID)
	value, ok := config.LookupNestedConfig(cfg, "high_risk_paths")
	if !ok {
		return nil
	}
	raw, isArray := value.([]any)
	if !isArray {
		return nil
	}
	paths := make([]string, 0, len(raw))
	for _, item := range raw {
		if pattern, isString := item.(string); isString && pattern != "" {
			paths = append(paths, pattern)
		}
	}
	return paths
}

// BaseRef is the ref the diff is graded against: --base when given, otherwise
// the manifest's [recipes.git-pr-flow.config] base_branch (default development)
// as it resolves here, trying origin/<base>, <base>, then the common integration
// branch names. Empty when nothing resolves.
func BaseRef(root string, flag *string) string {
	if flag != nil && *flag != "" {
		return *flag
	}
	base := "development"
	if manifest, err := config.LoadManifest(planbuild.ManifestPath(root)); err == nil {
		if _, _, cfg := config.RecipeConfigState(manifest, GitPRRecipeID); cfg != nil {
			if value, ok := config.LookupNestedConfig(cfg, "base_branch"); ok {
				if declared, isString := value.(string); isString && declared != "" {
					base = declared
				}
			}
		}
	}
	candidates := []string{"origin/" + base, base}
	for _, fallback := range []string{"main", "master"} {
		candidates = append(candidates, "origin/"+fallback, fallback)
	}
	for _, candidate := range candidates {
		if !validGitRef(candidate) {
			continue
		}
		if planbuild.RefExists(root, candidate) {
			return candidate
		}
	}
	return ""
}

// Refutations returns the R-a and R-b blockers of one change's diff, each
// prefixed with its rule id and carrying its exits, plus one note per refutation
// that could not be applied. A note is never a blocker: with no diff to grade a
// legitimate small change still passes, but the guardian says which rule it did
// not evaluate instead of staying silent (S16).
func Refutations(repoRoot, mode, change string, base *string) (blockers, notes []string) {
	explicit := base != nil && *base != ""
	baseRef := BaseRef(repoRoot, base)
	if baseRef == "" {
		if explicit {
			return nil, []string{unresolvedBaseNote(*base)}
		}
		return nil, []string{"R-a and R-b were not applied: no base ref resolves in this root (not a git " +
			"repository, or none of the project's base branch names exists), so there is no diff to grade. " +
			"Run the guardian from the repository root and fetch the base branch, or pass `--base <ref>` " +
			"naming a ref that resolves."}
	}
	diff, ok := planbuild.DiffEntries(repoRoot, baseRef)
	if !ok {
		if explicit {
			return nil, []string{unresolvedBaseNote(*base)}
		}
		return nil, []string{fmt.Sprintf("R-a and R-b were not applied: `git diff` against `%s` produced "+
			"no usable answer (a shallow clone, or no such history in this root), so there is no diff to "+
			"grade. Fetch the base branch, or pass `--base <ref>` naming a ref that resolves.", baseRef)}
	}
	blockers = append(blockers, canonicalSpecRefutation(repoRoot, mode, change, diff)...)
	blockers = append(blockers, highRiskRefutation(repoRoot, mode, change, baseRef, diff)...)
	return blockers, nil
}

// unresolvedBaseNote names a base the caller supplied that does not resolve.
func unresolvedBaseNote(base string) string {
	return fmt.Sprintf("R-a and R-b were not applied: the explicit `--base %s` does not resolve to a "+
		"commit in this root, so there is no diff to grade. Pass a ref that exists "+
		"(`git rev-parse --verify %s`) or omit `--base` to use the project's base branch.", base, base)
}

// canonicalSpecRefutation is R-a: a changed canonical spec with no delta this
// diff contributes, so an archive left by an earlier change proves nothing.
func canonicalSpecRefutation(repoRoot, mode, change string, diff []planbuild.DiffEntry) []string {
	first, count := firstMatch(diff, func(entry planbuild.DiffEntry) bool {
		return isCanonicalSpec(entry.Path)
	})
	if first == "" || deltaInDiff(mode, change, diff) {
		return nil
	}
	delta := repoRelative(repoRoot, filepath.Join(planbuild.ChangeFolder(repoRoot, mode, change), "specs"))
	return []string{fmt.Sprintf("R-a: the branch modifies the canonical spec `%s`%s but the change `%s` "+
		"carries no delta — changing a canonical spec without a delta is a promotion bypass. "+
		"Write the delta under `%s/` and run `ai-specs specs promote`.",
		first, andMore(count), change, delta)}
}

// highRiskRefutation is R-b: a changed path the project declares as high-risk,
// answered per mode — a plan this diff contributes in openspec, a Risk trailer in
// odd.
func highRiskRefutation(repoRoot, mode, change, base string, diff []planbuild.DiffEntry) []string {
	patterns := HighRiskPaths(repoRoot, base)
	if len(patterns) == 0 {
		return nil
	}
	first, count := firstMatch(diff, func(entry planbuild.DiffEntry) bool {
		for _, pattern := range patterns {
			if matchesHighRisk(pattern, entry.Path) {
				return true
			}
		}
		return false
	})
	if first == "" {
		return nil
	}
	found := fmt.Sprintf("R-b: the branch modifies the high-risk path `%s`%s, declared by the project in "+
		"ai-specs/ai-specs.toml", first, andMore(count))
	if mode == planbuild.ModeOdd {
		if planbuild.CommitsWithRiskTrailer(repoRoot, base) {
			return nil
		}
		return []string{found + fmt.Sprintf(", but no commit of `%s..HEAD` carries a `Risk: item N (<reason>)` "+
			"trailer — the trailer is the verification evidence an ODD high-risk change must carry, and a "+
			"bare `Risk: none` does not satisfy it. Add the trailer to one commit of the range and retry.",
			base)}
	}
	// A plan from an earlier change never counts: only a plan this diff adds or
	// modifies is evidence, or a committed archive would answer for every change.
	if planInDiff(change, diff) {
		return nil
	}
	plan := repoRelative(repoRoot, planbuild.TasksPath(repoRoot, planbuild.ModeOpenspec, change))
	return []string{found + fmt.Sprintf(", but the change `%s` carries no plan — in openspec the plan is "+
		"where the verification evidence of a high-risk change lives. Write `%s` (tier light) and retry.",
		change, plan)}
}

// isCanonicalSpec reports whether a repository-relative path is a canonical spec
// of either mode.
func isCanonicalSpec(rel string) bool {
	return strings.HasPrefix(rel, "odd/specs/") || strings.HasPrefix(rel, "openspec/specs/")
}

// deltaInDiff reports whether the diff contributes a delta of the change: a path
// under its specs/ folder, active or archived, that this diff adds or modifies.
// A deleted or renamed-away delta proves nothing, and neither does one that
// merely sits in the tree.
func deltaInDiff(mode, change string, diff []planbuild.DiffEntry) bool {
	for _, entry := range diff {
		if !entry.Adds() || !strings.HasSuffix(entry.Path, ".md") {
			continue
		}
		if rest := changeRelative(mode, change, entry.Path); len(rest) >= 2 && rest[0] == "specs" {
			return true
		}
	}
	return false
}

// planInDiff reports whether the diff contributes the plan of the change, active
// or archived. A committed plan of an earlier change never counts (S17).
func planInDiff(change string, diff []planbuild.DiffEntry) bool {
	for _, entry := range diff {
		if !entry.Adds() {
			continue
		}
		if rest := changeRelative(planbuild.ModeOpenspec, change, entry.Path); len(rest) == 1 && rest[0] == "tasks.md" {
			return true
		}
	}
	return false
}

// changeRelative splits a diff path of the mode's change tree into the segments
// after the change folder, accepting the active folder, an archive entry named
// after the change and a dated archive entry of it. nil when the path belongs to
// another change.
func changeRelative(mode, change, rel string) []string {
	base := "openspec/changes/"
	if mode == planbuild.ModeOdd {
		base = "odd/changes/"
	}
	if !strings.HasPrefix(rel, base) {
		return nil
	}
	parts := strings.Split(strings.TrimPrefix(rel, base), "/")
	if parts[0] == "archive" {
		parts = parts[1:]
	}
	if len(parts) < 2 {
		return nil
	}
	dated := datedArchiveRe.FindStringSubmatch(parts[0])
	if parts[0] != change && (dated == nil || dated[2] != change) {
		return nil
	}
	return parts[1:]
}

// matchesHighRisk matches one repository-relative path against one declared
// pattern: a trailing `/**` covers everything under that prefix, anything else
// is a shell-style match of the whole path.
func matchesHighRisk(pattern, rel string) bool {
	pattern = strings.TrimPrefix(filepath.ToSlash(pattern), "./")
	if prefix, ok := strings.CutSuffix(pattern, "/**"); ok {
		return strings.HasPrefix(rel, prefix+"/")
	}
	matched, err := path.Match(pattern, rel)
	return err == nil && matched
}

// firstMatch names the first matching path and how many matched in total.
func firstMatch(entries []planbuild.DiffEntry, match func(planbuild.DiffEntry) bool) (string, int) {
	first, count := "", 0
	for _, entry := range entries {
		if !match(entry) {
			continue
		}
		if count == 0 {
			first = entry.Path
		}
		count++
	}
	return first, count
}

// andMore names the paths a blocker does not list one by one.
func andMore(count int) string {
	if count < 2 {
		return ""
	}
	return fmt.Sprintf(" (and %d more)", count-1)
}

// validGitRef keeps a ref-shaped value from reaching git as an option.
func validGitRef(ref string) bool {
	if ref == "" || strings.HasPrefix(ref, "-") {
		return false
	}
	return !strings.ContainsAny(ref, " \t\r\n")
}
