package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Orphan cache deletion for recipe materialization (GO-06 WU1). Go owns the
// DELETION actuator behind --apply-orphans: it consumes the same stdin
// envelope as --plan-orphans plus the three already-resolved cache roots,
// reuses the authoritative planOrphans decision, and removes only validated
// direct child directories under those roots. Python keeps cache-root
// acquisition, the lock-file serialization (remove_recipe_lock_entries +
// write_lock) and its printed messages; this command never touches any lock.
//
// Failure contract (consumed by the Python bridge): the structured outcome
// distinguishes a PRE-APPLY failure (nothing attempted, the caller may still
// run any fallback) from a PARTIAL application (at least one completed
// removal, remaining targets reported, the failing one flagged uncertain) so
// a partially applied deletion is never replayed blindly.

// orphanApplyStatus values for orphanApplyOutcome.Status.
const (
	statusApplied        = "applied"
	statusPreApplyFailed = "pre_apply_failed"
	statusPartial        = "partial"
)

// orphanApplyRoots holds the three resolved cache-root paths. They must be
// absolute; Go never resolves, creates, or widens them.
type orphanApplyRoots struct {
	RecipeSkills  string `json:"recipe_skills"`
	DepsSkills    string `json:"deps_skills"`
	InprojectDeps string `json:"inproject_deps"`
}

// applyOrphansInput is the --apply-orphans stdin contract: the orphan plan
// input (whose decision is reused verbatim) plus the resolved roots. The
// stale-lock part of the plan is computed but ignored here — lock pruning
// stays in Python.
type applyOrphansInput struct {
	orphanPlanInput
	Roots orphanApplyRoots `json:"roots"`
}

// orphanApplyEntry is one planned child: which scope root it belongs to and
// its direct-child name. Uncertain is set only on the entry whose removal
// failed, because a failed RemoveAll leaves unknown filesystem state.
type orphanApplyEntry struct {
	Scope     string `json:"scope"`
	Name      string `json:"name"`
	Uncertain bool   `json:"uncertain,omitempty"`
}

// orphanApplyOutcome is the structured stdout contract. Removed/remaining are
// always present (empty lists, never null). Status applied means every
// planned directory was removed (missing or non-directory children were
// skipped without error); pre_apply_failed means nothing was attempted;
// partial means at least one completed removal before the first failure.
type orphanApplyOutcome struct {
	Status    string             `json:"status"`
	Removed   []orphanApplyEntry `json:"removed"`
	Remaining []orphanApplyEntry `json:"remaining"`
	Error     string             `json:"error"`
}

// applyScope pairs one plan scope with its resolved root for the apply loop,
// in the same order clean_orphans uses: recipes, deps, in-project deps.
type applyScope struct {
	scope string
	root  string
	names []string
}

// validateOrphanChild enforces the direct-child contract for one planned
// name: non-empty, not a dot path, and no separator component — so the
// constructed target can never escape its root or name a nested path.
func validateOrphanChild(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("invalid orphan child name %q", name)
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator) || filepath.Base(name) != name {
		return fmt.Errorf("orphan child name %q is not a direct child", name)
	}
	return nil
}

// applyOrphans is the destructive core. remove is injected so tests can force
// deterministic failures; the CLI passes os.RemoveAll. Validation of every
// scope happens before the first removal, so a malformed envelope can never
// half-apply. The first filesystem failure stops the run.
func applyOrphans(in applyOrphansInput, remove func(path string) error) orphanApplyOutcome {
	outcome := orphanApplyOutcome{Status: statusApplied, Removed: []orphanApplyEntry{}, Remaining: []orphanApplyEntry{}}
	plan := planOrphans(in.orphanPlanInput)
	scopes := []applyScope{
		{"recipe_skills", in.Roots.RecipeSkills, plan.OrphanedRecipes},
		{"deps_skills", in.Roots.DepsSkills, plan.OrphanedDeps},
		{"inproject_deps", in.Roots.InprojectDeps, plan.OrphanedInprojectDeps},
	}

	// Pre-apply validation: roots must be absolute and every planned name a
	// safe direct child. Nothing has been attempted yet on failure.
	var planned []orphanApplyEntry
	for _, sc := range scopes {
		if len(sc.names) == 0 {
			continue
		}
		root := filepath.Clean(sc.root)
		if !filepath.IsAbs(root) {
			outcome.Status = statusPreApplyFailed
			outcome.Error = fmt.Sprintf("scope %s: cache root %q is not absolute", sc.scope, sc.root)
			outcome.Remaining = allPlannedEntries(scopes)
			return outcome
		}
		for _, name := range sc.names {
			if err := validateOrphanChild(name); err != nil {
				outcome.Status = statusPreApplyFailed
				outcome.Error = fmt.Sprintf("scope %s: %v", sc.scope, err)
				outcome.Remaining = allPlannedEntries(scopes)
				return outcome
			}
			planned = append(planned, orphanApplyEntry{Scope: sc.scope, Name: name})
		}
	}

	// Apply loop in plan order; first failure stops the run.
	for _, entry := range planned {
		var root string
		for _, sc := range scopes {
			if sc.scope == entry.Scope {
				root = filepath.Clean(sc.root)
				break
			}
		}
		target := filepath.Join(root, entry.Name)
		// Direct-child invariant (defense in depth over the name check).
		if filepath.Dir(target) != root {
			outcome.Status = preApplyOrPartial(&outcome)
			outcome.Error = fmt.Sprintf("scope %s: target %q is not a direct child of the root", entry.Scope, target)
			outcome.Remaining = append(outcome.Remaining, orphanApplyEntry{Scope: entry.Scope, Name: entry.Name, Uncertain: true})
			outcome.Remaining = append(outcome.Remaining, restPlannedEntries(planned, entry)...)
			return outcome
		}
		// Lstat never follows the final component: a symlink child (even one
		// pointing at a directory) is skipped, never traversed or removed.
		info, err := os.Lstat(target)
		if err != nil {
			if os.IsNotExist(err) {
				continue // already absent: idempotent no-op
			}
			outcome.Status = preApplyOrPartial(&outcome)
			outcome.Error = fmt.Sprintf("scope %s: stat %s: %v", entry.Scope, target, err)
			outcome.Remaining = append(outcome.Remaining, orphanApplyEntry{Scope: entry.Scope, Name: entry.Name, Uncertain: true})
			outcome.Remaining = append(outcome.Remaining, restPlannedEntries(planned, entry)...)
			return outcome
		}
		if !info.IsDir() {
			continue // files and symlinks are preserved, matching the legacy guard
		}
		if err := remove(target); err != nil {
			// RemoveAll may have partially deleted the tree: uncertain state.
			outcome.Status = statusPartial
			outcome.Error = fmt.Sprintf("scope %s: remove %s: %v", entry.Scope, target, err)
			outcome.Remaining = append(outcome.Remaining, orphanApplyEntry{Scope: entry.Scope, Name: entry.Name, Uncertain: true})
			outcome.Remaining = append(outcome.Remaining, restPlannedEntries(planned, entry)...)
			return outcome
		}
		outcome.Removed = append(outcome.Removed, entry)
	}
	return outcome
}

// preApplyOrPartial returns the status for a failure with zero completed
// removals (pre-apply) versus one after completed removals (partial).
func preApplyOrPartial(outcome *orphanApplyOutcome) string {
	if len(outcome.Removed) > 0 {
		return statusPartial
	}
	return statusPreApplyFailed
}

// allPlannedEntries flattens every planned (scope, name) pair in apply order,
// for the remaining list of a pre-apply validation failure.
func allPlannedEntries(scopes []applyScope) []orphanApplyEntry {
	var entries []orphanApplyEntry
	for _, sc := range scopes {
		for _, name := range sc.names {
			entries = append(entries, orphanApplyEntry{Scope: sc.scope, Name: name})
		}
	}
	return entries
}

// restPlannedEntries returns the entries after the given one in apply order.
func restPlannedEntries(planned []orphanApplyEntry, done orphanApplyEntry) []orphanApplyEntry {
	for i, entry := range planned {
		if entry.Scope == done.Scope && entry.Name == done.Name {
			return planned[i+1:]
		}
	}
	return nil
}

// runApplyOrphans is the --apply-orphans command: decode the envelope from
// stdin, apply the orphan deletions, print one structured outcome JSON object
// on stdout. Exit 0 = applied; 2 = malformed input (consistent with
// --plan-orphans); 3 = pre-apply or partial failure (see the outcome JSON).
func runApplyOrphans(stdin io.Reader, stdout, stderr io.Writer) int {
	var in applyOrphansInput
	raw, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --apply-orphans: read stdin: %v\n", err)
		return 2
	}
	if text := strings.TrimSpace(string(raw)); text != "" {
		if err := json.Unmarshal([]byte(text), &in); err != nil {
			fmt.Fprintf(stderr, "worktree-gate: --apply-orphans: invalid input JSON: %v\n", err)
			return 2
		}
	}
	outcome := applyOrphans(in, os.RemoveAll)
	payload, err := json.Marshal(outcome)
	if err != nil {
		fmt.Fprintf(stderr, "worktree-gate: --apply-orphans: %v\n", err)
		return 2
	}
	fmt.Fprintln(stdout, string(payload))
	if outcome.Status == statusApplied {
		return 0
	}
	return 3
}
