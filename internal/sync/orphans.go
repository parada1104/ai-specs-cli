// orphans.go — native orphan cleanup for recipe materialization (GO-07 S10).
//
// Python's clean_orphans (lib/_internal/recipe-materialize.py:3564) computes
// the cache roots and the lock, asks the gate for the orphan plan, delegates
// the destructive deletes to --apply-orphans, then owns the lock pruning
// (remove_recipe_lock_entries + write_lock) and every printed success line.
// S14 will connect materialize orchestration; this file is the in-process
// core for the deletion + lock-prune + output contract. It reuses
// shared.PlanOrphans and shared.ApplyOrphans, so no deletion decision is
// copied here.
//
// Python stays the differential oracle until S16.
//
// Error contract: the returned error is the lock channel. A lock READ failure
// stops before any deletion (status pre_apply_failed, error non-nil), so the
// caller keeps its fallback. A lock WRITE failure happens after a successful
// deletion (status applied, error non-nil); the caller MUST check the error,
// because only the error marks the unpruned lock. Neither failure is retried.

package sync

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"ai-specs.dev/ai-specs/internal/lock"
	"ai-specs.dev/worktree-gate/shared"
)

// OrphanCleanupInput is the in-process contract for CleanOrphans. The orphan
// plan is an explicit argument: this core never acquires cache roots or reads
// the manifest (S14 owns that orchestration). Output receives the success
// lines; a nil Output discards them.
type OrphanCleanupInput struct {
	PlanInput shared.OrphanPlanInput
	Roots     shared.OrphanApplyRoots
	LockPath  string
	Output    io.Writer
	// ProjectRoot and CLIHome are the containment roots for the orphan roots:
	// the in-project tree follows the project rule, the CLI cache the strict
	// home rule (T6e).
	ProjectRoot string
	CLIHome     string
}

// containOrphanRoots refuses BEFORE any delete when an orphan root is
// redirected through a symlink (T6e): deleting `<root>/<name>` through a link
// would remove an outside tree. The in-project root stays inside the project;
// the CLI-cache roots refuse any symlink below the home.
func containOrphanRoots(in OrphanCleanupInput) error {
	if in.Roots.InprojectDeps != "" && in.ProjectRoot != "" {
		if err := projectWriteGuard(in.ProjectRoot, in.Roots.InprojectDeps, true); err != nil {
			return err
		}
		for _, name := range in.PlanInput.InprojectDeps {
			if err := projectWriteGuard(in.ProjectRoot, filepath.Join(in.Roots.InprojectDeps, name), true); err != nil {
				return err
			}
		}
	}
	if in.CLIHome == "" {
		return nil
	}
	for _, scope := range []struct {
		root  string
		names []string
	}{
		{in.Roots.RecipeSkills, in.PlanInput.RecipeSkills},
		{in.Roots.DepsSkills, in.PlanInput.DepsSkills},
	} {
		if scope.root == "" {
			continue
		}
		if err := homeWriteGuard(in.CLIHome, scope.root, true); err != nil {
			return err
		}
		for _, name := range scope.names {
			if err := homeWriteGuard(in.CLIHome, filepath.Join(scope.root, name), true); err != nil {
				return err
			}
		}
	}
	return nil
}

// planRecipeArtifactPrune uses managed ownership, not the skill-hash recipes table.
func planRecipeArtifactPrune(root string, enabledIDs []string, hooks []any) (func() error, error) {
	lockPath := filepath.Join(root, "ai-specs", ".ai-specs.lock")
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return nil, err
	}
	enabled := materializeStringSet(enabledIDs)
	launchers := map[string]bool{}
	for _, raw := range hooks {
		h, _ := raw.(*mcpObj)
		if path, ok := hooksGet(h, "script_path").(string); ok {
			launchers[path] = true
		}
	}
	recipeDir := func(rel string, record map[string]any) string {
		id, _ := record["recipe"].(string)
		if !fs.ValidPath(rel) || strings.Contains(rel, `\`) || id == "" || strings.Contains(id, "/") || id == "." || id == ".." {
			return ""
		}
		dir := "ai-specs/recipes/" + id
		suffix, ok := strings.CutPrefix(rel, dir+"/")
		if !ok || suffix == "overrides" || strings.HasPrefix(suffix, "overrides/") {
			return ""
		}
		if enabled[id] && (record["kind"] != "gate" || !strings.HasPrefix(suffix, "hooks/") || launchers[rel]) {
			return ""
		}
		return dir
	}
	var doomed, stale []string
	for _, rel := range slices.Sorted(maps.Keys(lk.Managed)) {
		record := lk.Managed[rel]
		if recipeDir(rel, record) != "" {
			if _, err := pruneFile(root, filepath.Join(root, rel)); err != nil {
				return nil, err
			}
			doomed = append(doomed, rel)
			continue
		}
		// A retired recipe's template/doc target is gone: drop the record, but
		// never delete a file this unit cannot prove ai-specs wrote.
		if recipeRetired(record, enabled) && managedTargetAbsent(root, rel) {
			stale = append(stale, rel)
		}
	}
	return func() error { return applyRecipeArtifactPrune(root, lockPath, doomed, stale, recipeDir, enabled) }, nil
}

// recipeRetired reports whether the record names a recipe the manifest no
// longer enables (records without an owning recipe always survive).
func recipeRetired(record map[string]any, enabled map[string]bool) bool {
	id, _ := record["recipe"].(string)
	return id != "" && !enabled[id]
}

// managedTargetAbsent proves the artifact is already gone without traversing
// a path the record cannot own.
func managedTargetAbsent(root, rel string) bool {
	if !fs.ValidPath(rel) || strings.Contains(rel, `\`) {
		return false
	}
	_, err := os.Lstat(filepath.Join(root, rel))
	return os.IsNotExist(err)
}

func applyRecipeArtifactPrune(root, lockPath string, doomed, stale []string, recipeDir func(string, map[string]any) string, enabled map[string]bool) error {
	if len(doomed) == 0 && len(stale) == 0 {
		return nil
	}
	// Keep records materialization added or repurposed since preflight.
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return err
	}
	changed := false
	for _, rel := range doomed {
		dir := recipeDir(rel, lk.Managed[rel])
		if dir == "" {
			continue
		}
		path := filepath.Join(root, rel)
		// Deleting a managed artifact must stay inside the project (T6e): a
		// symlinked ancestor below ai-specs/ must not redirect the removal.
		if err := projectWriteGuard(root, path, false); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		// Sweep only emptied parents. Overrides and other untracked contents
		// keep their directories, even when the recipe has been disabled.
		for parent, base := filepath.Dir(path), filepath.Join(root, dir); shared.TemplatePathContained(base, parent); parent = filepath.Dir(parent) {
			err := os.Remove(parent)
			if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
				break
			}
			if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		delete(lk.Managed, rel)
		changed = true
	}
	for _, rel := range stale {
		record := lk.Managed[rel]
		if !recipeRetired(record, enabled) || !managedTargetAbsent(root, rel) {
			continue
		}
		delete(lk.Managed, rel)
		changed = true
	}
	if changed {
		return lock.WriteLock(lockPath, lk)
	}
	return nil
}

// orphanScopePrefix mirrors Python _APPLY_SCOPE_MESSAGE_PREFIX. The in-project
// scope deliberately has no "cache" word, matching the historical print.
func orphanScopePrefix(scope string) string {
	switch scope {
	case "recipe_skills":
		return "cache .recipe/"
	case "deps_skills":
		return "cache .deps/"
	case "inproject_deps":
		return "ai-specs/.deps/"
	}
	return ""
}

// CleanOrphans applies the orphan plan, prints each removed line, then prunes
// the stale lock exactly as Python does. A fully applied deletion is the only
// path that prints success or touches the lock; a partial, uncertain, or
// pre-apply failure returns the shared apply outcome untouched.
func CleanOrphans(in OrphanCleanupInput, remove func(path string) error) (shared.OrphanApplyOutcome, error) {
	out := in.Output
	if out == nil {
		out = io.Discard
	}

	// Python reads the lock BEFORE any destructive work
	// (`lock = load_lock(lock_path) if lock_path.is_file() else {}`) and prunes
	// that same in-memory lock afterwards. Reading it first keeps a lock parse
	// failure a clean pre-apply stop: nothing is deleted, so the caller keeps
	// its fallback. LoadLock is Python-is_file tolerant for a missing or
	// non-regular path; only a real parse failure returns an error.
	lk, err := lock.LoadLock(in.LockPath)
	if err != nil {
		return shared.OrphanApplyOutcome{
			Status:    shared.StatusPreApplyFailed,
			Removed:   []shared.OrphanApplyEntry{},
			Remaining: []shared.OrphanApplyEntry{},
			Error:     fmt.Sprintf("load lock %s: %v", in.LockPath, err),
		}, err
	}

	// Nothing is deleted until every root (and candidate) is contained.
	if err := containOrphanRoots(in); err != nil {
		return shared.OrphanApplyOutcome{
			Status:    shared.StatusPreApplyFailed,
			Removed:   []shared.OrphanApplyEntry{},
			Remaining: []shared.OrphanApplyEntry{},
			Error:     err.Error(),
		}, err
	}

	plan := shared.PlanOrphans(in.PlanInput)
	outcome := shared.ApplyOrphans(shared.ApplyOrphansInput{
		OrphanPlanInput: in.PlanInput,
		Roots:           in.Roots,
	}, remove)

	// Only a fully applied deletion may print success or prune the lock. A
	// partial or uncertain outcome leaves the filesystem unknown, so it never
	// gets a success line and never gets a lock rewrite.
	if outcome.Status != shared.StatusApplied {
		return outcome, nil
	}

	for _, entry := range outcome.Removed {
		fmt.Fprintf(out, "  ✓ removed orphaned %s%s\n", orphanScopePrefix(entry.Scope), entry.Name)
	}

	// _prune_stale_lock parity: rewrite only when the lock is still a regular
	// file and at least one stale recipe entry was actually removed.
	if info, statErr := os.Stat(in.LockPath); statErr == nil && info.Mode().IsRegular() {
		removedAny := false
		for _, rid := range plan.StaleLockRecipes {
			if lock.RemoveRecipeLockEntries(lk, rid) {
				removedAny = true
				fmt.Fprintf(out, "  ✓ removed stale lock entries for recipe '%s'\n", rid)
			}
		}
		// A dep the manifest no longer declares loses its recorded hashes. The
		// expected set, not the orphan plan, is authoritative: a dep whose trees
		// are already gone has no orphan entry left to derive the ID from.
		expected := materializeStringSet(in.PlanInput.ExpectedDepIDs)
		for _, depID := range slices.Sorted(maps.Keys(lk.Deps)) {
			if expected[depID] {
				continue
			}
			if lock.RemoveDepLockEntries(lk, depID) {
				removedAny = true
				fmt.Fprintf(out, "  ✓ removed stale lock entries for dep '%s'\n", depID)
			}
		}
		if removedAny {
			if err := lock.WriteLock(in.LockPath, lk); err != nil {
				return outcome, fmt.Errorf("write lock %s: %w", in.LockPath, err)
			}
		}
	}

	return outcome, nil
}
