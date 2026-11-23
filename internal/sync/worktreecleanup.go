package sync

// CLI-served worktree cleanup (cli-served-recipes T3, S1/S4).
//
// Two worktree-flow template targets stop being materialized as script bytes:
//
//   - the retired launcher ai-specs/recipes/worktree-flow/overrides/bin/
//     worktree-cleanup.sh, an override (condition not_exists) that the user
//     could edit. The CLI verb `ai-specs worktree-cleanup` replaces it, so the
//     migration removes every copy, managed or user-modified (S1/S4).
//   - `.git/hooks/post-merge`, a one-line CLI hook that sync ALWAYS regenerates.
//     It is not an override and holds no policy: the verb reads the sync
//     snapshot at run time, so the stamping the old wrapper needed is gone.
//
// The hook target keeps its git resolution (shared.ResolveTemplateDest), because
// in a linked worktree `.git` is a gitfile and hooks live in the shared hooks
// directory of the main repository.

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"ai-specs.dev/ai-specs/internal/lock"
	shared "ai-specs.dev/worktree-gate/shared"
)

// worktreePostMergeTarget is the recipe template target the CLI generates.
const worktreePostMergeTarget = ".git/hooks/post-merge"

// worktreeCleanupVerb is the CLI verb both the hook and the docs invoke.
const worktreeCleanupVerb = "worktree-cleanup"

// worktreeFlowRecipe ID is the recipe that owns the cleanup verb and the hook.
const worktreeFlowRecipeID = "worktree-flow"

// retiredWorktreeCleanupOverride is the user-editable launcher this migration
// removes. It is an override, so no lock record licenses the deletion.
const retiredWorktreeCleanupOverride = "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"

// worktreePostMergeHook renders the managed hook: one shebang plus the exec
// line. The CLI resolution order matches the CLI-served gate command (AI_SPECS
// _HOME/dist first, PATH second), and a missing CLI exits 0 so a missing binary
// never breaks an already-merged merge.
func worktreePostMergeHook() []byte {
	return []byte("#!/bin/sh\n" +
		`cli="${AI_SPECS_HOME:-$HOME/.ai-specs}/dist/ai-specs"; [ -x "$cli" ] || cli=ai-specs; ` +
		`command -v "$cli" >/dev/null 2>&1 || exit 0; exec "$cli" ` + worktreeCleanupVerb + "\n")
}

// worktreeTemplateServesCLI reports the worktree-flow template targets the CLI
// writes itself instead of the governed-template actuator.
func worktreeTemplateServesCLI(recipeID, target string) bool {
	if recipeID != worktreeFlowRecipeID {
		return false
	}
	normalized := posixAsPosix(target)
	return normalized == worktreePostMergeTarget || normalized == retiredWorktreeCleanupOverride
}

// materializeWorktreePostMergeHook writes the generated hook unconditionally:
// it is not an override, so a user edit is simply overwritten on the next sync.
func materializeWorktreePostMergeHook(projectRoot string) error {
	dest, gitResolved := shared.ResolveTemplateDest(projectRoot, worktreePostMergeTarget)
	if !gitResolved && !shared.TemplatePathContained(projectRoot, dest) {
		return shared.Refuse(shared.TemplateEscapingTargetRefusal(worktreePostMergeTarget))
	}
	if err := shared.EnsureTemplateAncestorsReal(projectRoot, dest); err != nil {
		return shared.Refuse(fmt.Sprintf("cannot write %s: %v", worktreePostMergeTarget, err))
	}
	return shared.WriteTemplateContent(dest, worktreePostMergeHook(), 0o755)
}

// retireWorktreeCleanupOverride removes the launcher the CLI now serves. It is
// explicit because the asset is an override: `condition = not_exists` never
// prunes it, and a user-edited copy is untracked in the lock, so no record
// proves ownership. S1 forbids the user modifying it, so even an edited copy is
// removed; the notice says so once.
func retireWorktreeCleanupOverride(root, lockPath string, out io.Writer) error {
	path := filepath.Join(root, filepath.FromSlash(retiredWorktreeCleanupOverride))
	if _, err := os.Lstat(path); err != nil {
		return nil
	}
	if err := projectWriteGuard(root, path, false); err != nil {
		return err
	}
	lk, err := lock.LoadLock(lockPath)
	if err != nil {
		return err
	}
	if retiredCleanupOverrideModified(lk, path) {
		fmt.Fprintf(out, "  ✓ removed user-modified worktree-cleanup.sh override %s (ai-specs now serves cleanup from the CLI)\n",
			retiredWorktreeCleanupOverride)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	// Sweep only emptied parents, so a user override elsewhere in the recipe
	// (the trello card templates) keeps its directories.
	base := filepath.Join(root, "ai-specs", "recipes", worktreeFlowRecipeID, "overrides")
	for parent := filepath.Dir(path); shared.TemplatePathContained(base, parent); parent = filepath.Dir(parent) {
		err := os.Remove(parent)
		if errors.Is(err, syscall.ENOTEMPTY) || errors.Is(err, syscall.EEXIST) {
			break
		}
		if err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if _, recorded := lk.Managed[retiredWorktreeCleanupOverride]; !recorded {
		return nil
	}
	delete(lk.Managed, retiredWorktreeCleanupOverride)
	return lock.WriteLock(lockPath, lk)
}

// retiredCleanupOverrideModified reports whether the copy differs from the bytes
// sync last recorded. An untracked copy counts as modified: sync cannot prove it
// wrote them, and the old not_exists policy left user edits untracked on purpose.
func retiredCleanupOverrideModified(lk *lock.Lock, path string) bool {
	recorded, _ := lk.Managed[retiredWorktreeCleanupOverride]["sha256"].(string)
	if strings.TrimSpace(recorded) == "" {
		return true
	}
	current, err := lock.Sha256OfFile(path)
	return err != nil || current != recorded
}
