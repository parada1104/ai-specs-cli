package sync

// CLI-served worktree cleanup (cli-served-recipes T3, S1/S4).
//
// Three worktree-flow template targets stop being materialized as script bytes:
//
//   - the retired launcher ai-specs/recipes/worktree-flow/overrides/bin/
//     worktree-cleanup.sh, an override (condition not_exists) that the user
//     could edit. The CLI verb `ai-specs worktree-cleanup` replaces it, so the
//     migration removes every copy, managed or user-modified (S1/S4).
//   - `.git/hooks/post-merge`, a one-line CLI hook that sync ALWAYS regenerates.
//     It is not an override and holds no policy: the verb reads the sync
//     snapshot at run time, so the stamping the old wrapper needed is gone.
//   - `.git/hooks/post-checkout`, the same hook in integration-only mode (S12):
//     a provider-side merge lands locally as `git checkout <integration>` after a
//     fetch, which post-merge never sees.
//
// The hook targets keep their git resolution (shared.ResolveTemplateDest), because
// in a linked worktree `.git` is a gitfile and hooks live in the shared hooks
// directory of the main repository.

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"ai-specs.dev/ai-specs/internal/lock"
	shared "ai-specs.dev/worktree-gate/shared"
)

// worktreePostMergeTarget is the recipe template target the CLI generates.
const worktreePostMergeTarget = ".git/hooks/post-merge"

// worktreePostCheckoutTarget is the S12 trigger: the checkout that lands a
// provider-side merged integration branch, which post-merge never sees.
const worktreePostCheckoutTarget = ".git/hooks/post-checkout"

// worktreeCleanupVerb is the CLI verb both the hook and the docs invoke.
const worktreeCleanupVerb = "worktree-cleanup"

// worktreeFlowRecipe ID is the recipe that owns the cleanup verb and the hook.
const worktreeFlowRecipeID = "worktree-flow"

// retiredWorktreeCleanupOverride is the user-editable launcher this migration
// removes. It is an override, so no lock record licenses the deletion.
const retiredWorktreeCleanupOverride = "ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh"

// worktreeCleanupHook renders one generated hook: a shebang, the CLI resolution,
// a verb-capability probe and the exec line. The CLI resolution order matches the
// CLI-served gate command (AI_SPECS_HOME/dist first, PATH second), and a missing
// CLI exits 0 so a missing binary never breaks an already-merged merge.
//
// The probe exists because `command -v` proves a binary exists, not that it serves
// the verb (S10): an installed CLI older than the verb would otherwise run
// `unknown command` into the void, clean nothing, and say nothing.
//
// The hook holds no policy. post-checkout fires on every checkout, so it asks the
// verb for `--if-integration` and the branch comparison stays in Go (S12).
func worktreeCleanupHook(hookName, verbArgs string) []byte {
	return []byte(`#!/bin/sh
cli="${AI_SPECS_HOME:-$HOME/.ai-specs}/dist/ai-specs"
[ -x "$cli" ] || cli=ai-specs
command -v "$cli" >/dev/null 2>&1 || exit 0
"$cli" ` + worktreeCleanupVerb + ` --help >/dev/null 2>&1 || {
	echo "` + hookName + `: $cli does not serve '` + worktreeCleanupVerb + `'; reinstall a current ai-specs, then run: ai-specs sync" >&2
	exit 0
}
exec "$cli" ` + worktreeCleanupVerb + verbArgs + `
`)
}

// worktreePostMergeHook is the plain cleanup trigger: git runs it after a merge
// or a fast-forward pull.
func worktreePostMergeHook() []byte {
	return worktreeCleanupHook("post-merge", "")
}

// worktreePostCheckoutHook is the integration-only trigger (S12).
func worktreePostCheckoutHook() []byte {
	return worktreeCleanupHook("post-checkout", " --if-integration")
}

// worktreeTemplateServesCLI reports the worktree-flow template targets the CLI
// writes itself instead of the governed-template actuator.
func worktreeTemplateServesCLI(recipeID, target string) bool {
	if recipeID != worktreeFlowRecipeID {
		return false
	}
	normalized := posixAsPosix(target)
	return normalized == worktreePostMergeTarget ||
		normalized == worktreePostCheckoutTarget ||
		normalized == retiredWorktreeCleanupOverride
}

// materializeWorktreePostMergeHook writes the generated hooks unconditionally:
// they are not overrides, so a user edit is simply overwritten on the next sync.
// The recipe declares both targets together, and the caller sets its flag from
// the post-merge entry, so both are written from here.
func materializeWorktreePostMergeHook(projectRoot string) error {
	for _, hook := range []struct {
		target string
		body   []byte
	}{
		{worktreePostMergeTarget, worktreePostMergeHook()},
		{worktreePostCheckoutTarget, worktreePostCheckoutHook()},
	} {
		if err := materializeWorktreeCLIHook(projectRoot, hook.target, hook.body); err != nil {
			return err
		}
	}
	return nil
}

// materializeWorktreeCLIHook writes one generated hook at its git-resolved path.
func materializeWorktreeCLIHook(projectRoot, target string, body []byte) error {
	dest, gitResolved := shared.ResolveTemplateDest(projectRoot, target)
	if !gitResolved && !shared.TemplatePathContained(projectRoot, dest) {
		return shared.Refuse(shared.TemplateEscapingTargetRefusal(target))
	}
	if err := shared.EnsureTemplateAncestorsReal(projectRoot, dest); err != nil {
		return shared.Refuse(fmt.Sprintf("cannot write %s: %v", target, err))
	}
	return shared.WriteTemplateContent(dest, body, 0o755)
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
	if err := removeEmptiedParents(base, path); err != nil {
		return err
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
