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
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"ai-specs.dev/ai-specs/internal/config"
	"ai-specs.dev/ai-specs/internal/planbuild"
	"ai-specs.dev/ai-specs/internal/toml"
)

// GitPRRecipeID is the recipe whose config declares the project base branch.
const GitPRRecipeID = "git-pr-flow"

// ProductDirs is the write gate's graded production scope; ProductDirsEnv overrides it.
const (
	ProductDirs    = "src lib catalog"
	ProductDirsEnv = "PLAN_BUILD_GATE_PATHS"
)

// HighRiskPaths reads the project's explicit high-risk list from
// [recipes.plan-build-flow.config] high_risk_paths in ai-specs/ai-specs.toml at
// the trusted base revision the diff is graded against, never the working tree,
// so a candidate cannot delete a pattern to stop being graded by it. The key is
// not declared in the recipe's recipe.toml: that schema is frozen by a test and
// an array of path patterns has no shape the wizard can offer. Any other shape
// than an array of strings yields no list, so no declaration means no R-b
// refusal.
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
	blockers = append(blockers, canonicalSpecRefutation(repoRoot, mode, change, diff, "R-a")...)
	blockers = append(blockers, highRiskRefutation(repoRoot, mode, change, baseRef, diff, "R-b", "")...)
	return blockers, nil
}

// WriteRefutations returns the write gate's objective refutations of a `none`
// claim: R1–R5, each named with the fact that proves it and the exits that
// resolve it. They reuse the guardian's rules (R1/R2 are its R-a/R-b) and add
// the three the diff itself proves (R3 size, R4 contract removal, R5 plan
// evidence orphanhood). No refutation fires without a fact, so an empty
// worktree is never blocked.
func WriteRefutations(repoRoot, mode, change, target string) []string {
	base := BaseRef(repoRoot, nil)
	var diff []planbuild.DiffEntry
	diffText := ""
	if base != "" {
		if entries, ok := planbuild.DiffEntries(repoRoot, base); ok {
			diff = entries
		}
		if text, ok := planbuild.DiffText(repoRoot, base); ok {
			diffText = text
		}
	}
	var blockers []string
	// R1 also reads the diff: a canonical spec the branch already changed with
	// no delta this change contributes is a promotion bypass even when the
	// pending write targets something else.
	if change != "" && !planbuild.HasDelta(repoRoot, mode, change) {
		blockers = append(blockers, canonicalSpecRefutation(repoRoot, mode, change, diff, "R1")...)
	}
	blockers = append(blockers, highRiskRefutation(repoRoot, mode, change, base, diff, "R2", target)...)
	// S16: a plan-less, delta-less change with no product path is a workstream.
	if nothingToGrade(repoRoot, mode, change, diff) {
		return blockers
	}
	blockers = append(blockers, sizeRefutation(repoRoot, mode, change, base, diff, diffText)...)
	blockers = append(blockers, contractRefutation(repoRoot, mode, change, diffText)...)
	blockers = append(blockers, planEvidenceRefutation(repoRoot, mode, change)...)
	return blockers
}

// HighRiskRefutation is R2 alone, for the canonical-spec caller that already
// answered R1 with its delta.
func HighRiskRefutation(repoRoot, mode, change, target string) []string {
	base := BaseRef(repoRoot, nil)
	diff, _ := planbuild.DiffEntries(repoRoot, base)
	return highRiskRefutation(repoRoot, mode, change, base, diff, "R2", target)
}

// CanonicalSpecRefusal is R1's message body for a hand write to a canonical
// spec, naming the missing delta and the promotion exit. change is empty when
// no branch can be paired.
func CanonicalSpecRefusal(mode, change, specPath string) string {
	changes := "openspec/changes/"
	if mode == planbuild.ModeOdd {
		changes = "odd/changes/"
	}
	if change == "" {
		return fmt.Sprintf("R1: `%s` is written by promotion, not by hand, and the current branch "+
			"cannot be resolved (detached HEAD), so no delta can back it. Run the write from the "+
			"change's branch, or promote the delta instead of editing the canonical tree.", specPath)
	}
	return fmt.Sprintf("R1: `%s` is written by promotion, not by hand, and the change `%s` carries "+
		"no delta under `%s%s/specs/`. Write the delta there and run `ai-specs specs promote`, or "+
		"promote from the change folder that owns the rule. Writing under `%s` is never blocked.",
		specPath, change, changes, change, changes)
}

var (
	longFlagRe = regexp.MustCompile(`--[a-z][a-z0-9-]*`)
	tomlKeyRe  = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_-]*)[ \t]*=`)
)

// diffSides splits a unified diff into the content lines it removes and adds,
// skipping headers, hunk markers and the no-newline notice.
func diffSides(text string) (removed, added []string) {
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"),
			strings.HasPrefix(line, "@@"), strings.HasPrefix(line, "diff "),
			strings.HasPrefix(line, "index "), strings.HasPrefix(line, "\\"):
			continue
		case strings.HasPrefix(line, "-"):
			removed = append(removed, line[1:])
		case strings.HasPrefix(line, "+"):
			added = append(added, line[1:])
		}
	}
	return removed, added
}

// nothingToGrade reports the S16 workstream: no plan, no delta and no product
// path in the diff is nothing to grade, so only R1 and R2 still apply.
func nothingToGrade(repoRoot, mode, change string, diff []planbuild.DiffEntry) bool {
	if change != "" && (planbuild.PlanIsValidForMode(repoRoot, mode, change) ||
		planbuild.HasDelta(repoRoot, mode, change)) {
		return false
	}
	for _, entry := range diff {
		if productPath(entry.Path) {
			return false
		}
	}
	return true
}

// productPath reports whether a repository-relative path is in the production scope.
func productPath(rel string) bool {
	dirs := os.Getenv(ProductDirsEnv)
	// `${prod_dirs// /}`: only spaces make the value blank.
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

// sizeRefutation is R3: the diff exceeds a size or module threshold the project
// declares. An unconfigured threshold proves nothing, so it never refuses.
func sizeRefutation(repoRoot, mode, change, base string, diff []planbuild.DiffEntry, diffText string) []string {
	maxLines, hasLines := projectInt(repoRoot, base, "max_diff_lines")
	maxModules, hasModules := projectInt(repoRoot, base, "max_changed_modules")
	if !hasLines && !hasModules {
		return nil
	}
	removed, added := diffSides(diffText)
	if hasLines && len(removed)+len(added) > maxLines &&
		!strings.Contains(changeRecord(repoRoot, mode, change), "R3") {
		return []string{fmt.Sprintf("R3: the branch changes %d diff lines, over the project's "+
			"max_diff_lines = %d declared in ai-specs/ai-specs.toml. Split the change. %s",
			len(removed)+len(added), maxLines, changeExit(change, "R3"))}
	}
	if hasModules {
		modules := map[string]bool{}
		for _, entry := range diff {
			if module, _, found := strings.Cut(entry.Path, "/"); found && module != "" {
				modules[module] = true
			}
		}
		if len(modules) > maxModules && !strings.Contains(changeRecord(repoRoot, mode, change), "R3") {
			return []string{fmt.Sprintf("R3: the branch touches %d modules, over the project's "+
				"max_changed_modules = %d declared in ai-specs/ai-specs.toml. Split the change. %s",
				len(modules), maxModules, changeExit(change, "R3"))}
		}
	}
	return nil
}

// projectInt reads one integer from [recipes.plan-build-flow.config] at the
// trusted base revision, so a candidate cannot relax the threshold it is graded by.
func projectInt(root, base, key string) (int, bool) {
	text, ok := planbuild.FileAt(root, base, planbuild.ManifestRelPath)
	if !ok {
		return 0, false
	}
	manifest, err := toml.Parse([]byte(text))
	if err != nil {
		return 0, false
	}
	_, _, cfg := config.RecipeConfigState(manifest, planbuild.RecipeID)
	value, ok := config.LookupNestedConfig(cfg, key)
	if !ok {
		return 0, false
	}
	switch typed := value.(type) {
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	}
	return 0, false
}

// contractRefutation is R4: the diff removes a public long flag or a config key
// that no tracked file of the working tree still declares, which is a contract
// others consume. It tracks the file each hunk belongs to so a prose assignment
// outside a .toml is never read as a config key.
func contractRefutation(repoRoot, mode, change, diffText string) []string {
	onToml := false
	added := ""
	var flags, keys []string
	for _, line := range strings.Split(diffText, "\n") {
		switch {
		case strings.HasPrefix(line, "+++ "):
			onToml = onToml || strings.HasSuffix(line, ".toml")
			continue
		case strings.HasPrefix(line, "--- "):
			// A whole-file deletion carries `+++ /dev/null`, so both sides count.
			onToml = strings.HasSuffix(line, ".toml")
			continue
		case strings.HasPrefix(line, "@@"), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "\\"):
			continue
		case strings.HasPrefix(line, "+"):
			added += line[1:] + "\n"
		case strings.HasPrefix(line, "-"):
			content := line[1:]
			flags = append(flags, declaredFlags(content)...)
			if onToml {
				if match := tomlKeyRe.FindStringSubmatch(content); match != nil {
					keys = append(keys, match[1])
				}
			}
		}
	}
	for _, flag := range flags {
		if strings.Contains(added, flag) || planbuild.WorktreeContains(repoRoot, flag) ||
			strings.Contains(changeRecord(repoRoot, mode, change), flag) {
			continue
		}
		return []string{fmt.Sprintf("R4: the branch removes the public flag `%s`, a contract others "+
			"already consume. %s", flag, changeExit(change, "R4", flag))}
	}
	for _, key := range keys {
		if strings.Contains(added, key) || planbuild.WorktreeContains(repoRoot, key) ||
			strings.Contains(changeRecord(repoRoot, mode, change), key) {
			continue
		}
		return []string{fmt.Sprintf("R4: the branch removes the config key `%s`, a contract others "+
			"already consume. %s", key, changeExit(change, "R4", key))}
	}
	return nil
}

// changeExit names the exits R3 and R4 actually honour: the plan or a delta of
// the paired change naming the rule or the removed token, or no removal at all.
func changeExit(change string, tokens ...string) string {
	if change == "" {
		return "The branch pairs no change, so no plan or delta can declare it: restore the previous state."
	}
	return fmt.Sprintf("Write the plan or a delta of `%s` naming `%s`, or restore the previous state.",
		change, strings.Join(tokens, "` or `"))
}

// changeRecord is the text of the paired change's plan and its deltas: the one
// record whose naming of a rule or token satisfies a refusal.
func changeRecord(repoRoot, mode, change string) string {
	if change == "" {
		return ""
	}
	record := readText(planbuild.TasksPath(repoRoot, mode, change))
	specs := filepath.Join(planbuild.ChangeFolder(repoRoot, mode, change), "specs")
	_ = filepath.WalkDir(specs, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
			record += readText(path)
		}
		return nil
	})
	return record
}

// flagRegistrationRe matches the constructs that declare a CLI flag, so a removed
// registration counts as a contract change while a comment or a doc line does not.
var flagRegistrationRe = regexp.MustCompile(`\b(?:p?flag\.\w+|Flags\(\)\.\w+|Flag\s*\{)`)

// declaredFlags returns the long flags a removed line declares: a CLI switch case,
// a help line, or a flag registration, never a comment that merely names the flag.
func declaredFlags(line string) []string {
	trimmed := strings.TrimSpace(line)
	// A commented-out registration is not a declaration.
	if trimmed != "" && strings.ContainsAny(trimmed[:1], "/#*") {
		return nil
	}
	declared := strings.HasPrefix(trimmed, "case ") || strings.HasPrefix(trimmed, "--") ||
		flagRegistrationRe.MatchString(trimmed)
	if !declared {
		return nil
	}
	return longFlagRe.FindAllString(trimmed, -1)
}

// planEvidenceRefutation is R5: the plan declares S# rows but carries no
// `## Verification` section at all. Rows inside an existing section are graded
// by the merge guardian, so a plan still being authored is not blocked before
// the code exists to verify.
func planEvidenceRefutation(repoRoot, mode, change string) []string {
	if change == "" || !planbuild.PlanIsValidForMode(repoRoot, mode, change) {
		return nil
	}
	planPath := planbuild.TasksPath(repoRoot, mode, change)
	plan, _ := ParsePlan(readText(planPath))
	if len(plan.Specs) == 0 || plan.HasVerification {
		return nil
	}
	var declared []string
	for _, spec := range plan.Specs {
		declared = append(declared, spec.ID)
	}
	return []string{fmt.Sprintf("R5: the plan `%s` declares %s but carries no `## Verification` section — "+
		"a declared specification without its rows is an evidence orphan. Add one row per declared S# "+
		"under `## Verification`, record an exemption decision, or escalate.",
		repoRelative(repoRoot, planPath), strings.Join(declared, ", "))}
}

// unresolvedBaseNote names a base the caller supplied that does not resolve.
func unresolvedBaseNote(base string) string {
	return fmt.Sprintf("R-a and R-b were not applied: the explicit `--base %s` does not resolve to a "+
		"commit in this root, so there is no diff to grade. Pass a ref that exists "+
		"(`git rev-parse --verify %s`) or omit `--base` to use the project's base branch.", base, base)
}

// canonicalSpecRefutation is R-a/R1: a changed canonical spec with no delta this
// diff contributes, so an archive left by an earlier change proves nothing.
func canonicalSpecRefutation(repoRoot, mode, change string, diff []planbuild.DiffEntry, rule string) []string {
	first, count := firstMatch(diff, func(entry planbuild.DiffEntry) bool {
		return isCanonicalSpec(entry.Path)
	})
	if first == "" || deltaInDiff(mode, change, diff) {
		return nil
	}
	delta := repoRelative(repoRoot, filepath.Join(planbuild.ChangeFolder(repoRoot, mode, change), "specs"))
	return []string{fmt.Sprintf("%s: the branch modifies the canonical spec `%s`%s but the change `%s` "+
		"carries no delta — changing a canonical spec without a delta is a promotion bypass. "+
		"Write the delta under `%s/` and run `ai-specs specs promote`.",
		rule, first, andMore(count), change, delta)}
}

// highRiskRefutation is R-b/R2: a changed path the project declares as
// high-risk, answered per mode — a plan this diff contributes in openspec, a
// Risk trailer in odd.
func highRiskRefutation(repoRoot, mode, change, base string, diff []planbuild.DiffEntry, rule, pending string) []string {
	patterns := HighRiskPaths(repoRoot, base)
	if len(patterns) == 0 {
		return nil
	}
	matches := func(rel string) bool {
		for _, pattern := range patterns {
			if matchesHighRisk(pattern, rel) {
				return true
			}
		}
		return false
	}
	first, count := firstMatch(diff, func(entry planbuild.DiffEntry) bool { return matches(entry.Path) })
	// The pending write is graded before it happens, so a clean branch cannot
	// write a declared high-risk path without its evidence.
	if first == "" && pending != "" && matches(pending) {
		first, count = pending, 1
	}
	if first == "" {
		return nil
	}
	found := fmt.Sprintf("%s: the branch modifies the high-risk path `%s`%s, declared by the project in "+
		"ai-specs/ai-specs.toml", rule, first, andMore(count))
	if mode == planbuild.ModeOdd {
		if planbuild.CommitsWithRiskTrailer(repoRoot, base) {
			return nil
		}
		return []string{found + fmt.Sprintf(", but no commit of `%s..HEAD` carries a `Risk: item N (<reason>)` "+
			"trailer — the trailer is the verification evidence an ODD high-risk change must carry, and a "+
			"bare `Risk: none` does not satisfy it. N is one of: 1 data or irreversible effects that "+
			"cannot be rebuilt, 2 security, 3 contracts others already consume, 4 concurrency, "+
			"5 delivery or environment, 6 no test would catch a regression. "+
			"Add the trailer to one commit of the range and retry.",
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
	archived := parts[0] == "archive"
	if archived {
		parts = parts[1:]
	}
	if len(parts) < 2 {
		return nil
	}
	if parts[0] != change {
		// A date-prefixed name is the archive convention only: beneath archive/ it
		// pairs with the change by suffix, while anywhere else it names a different
		// change and must never answer for this one (R1-001).
		if !archived {
			return nil
		}
		dated := datedArchiveRe.FindStringSubmatch(parts[0])
		if dated == nil || dated[2] != change {
			return nil
		}
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
