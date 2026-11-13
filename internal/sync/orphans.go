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
	"fmt"
	"io"
	"os"

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
		if removedAny {
			if err := lock.WriteLock(in.LockPath, lk); err != nil {
				return outcome, fmt.Errorf("write lock %s: %w", in.LockPath, err)
			}
		}
	}

	return outcome, nil
}
