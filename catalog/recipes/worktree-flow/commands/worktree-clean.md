# worktree-clean

Reclaim git worktrees whose branch has been merged into the integration branch,
and delete their remote branches from the main worktree after independent
verification.

## When to use

After one or more change branches have been merged, to remove their worktrees
under `<worktrees_dir>/` (default `.worktrees/`).

## Topology

Under resolved `monorepo-submodules`, cleanup enumerates **per initialized
submodule** (`git -C <module> worktree list` / `git submodule foreach`). Never
trust the superproject `git worktree list` alone — it does not show submodule
linked worktrees. `standalone` / `monorepo-apps` keep a single-repo pass.

Optional scope flags:

- `--submodule <path>` / `--subrepo <path>` (repeatable) — limit to one or more
  initialized modules. Default = all initialized submodules.
- On a standalone repo these flags are inert (single pass), not an error.

The worktrees dir, integration branch, topology and protected branches come from
the project's last `ai-specs sync` snapshot, never from flags or `ai-specs.toml`.
Without a snapshot the verb removes nothing and exits 2; run `ai-specs sync` first.

## Steps

Cleanup MUST be run from the main repository worktree. Do not invoke it from a
linked feature worktree: that worktree may be the deletion target, and GitHub's
`gh pr merge --delete-branch` cannot handle this layout when the base is already
checked out elsewhere. Cleanup fails closed: without the installed CLI, or without a
sync snapshot, it performs no destructive action.

0. Ensure your shell is **outside** the worktree you might remove (`cd` to the
   main / superproject repository root first).

1. Preview what would be removed (safe, mutates nothing):

   ```bash
   ai-specs worktree-cleanup --dry-run
   ```

   Scope example:

   ```bash
   ai-specs worktree-cleanup --submodule apps/api --dry-run
   ```

2. Review the output. Lines mean:
   - `would remove <name>` — merged + clean, eligible for removal.
   - `skipped <name> (dirty)` — has uncommitted changes; left untouched.
   - `skipped <name> (unmerged)` — branch not merged yet; left untouched.

3. Apply the cleanup once the preview looks right:

   ```bash
   ai-specs worktree-cleanup
   ```

Cleanup uses the `worktrees_dir` and `integration_branch` values recorded by the
last `ai-specs sync` (from `[recipes.worktree-flow.config]`). Shared layout:
worktrees live under the **superproject** `<worktrees_dir>/` as
`<subrepo>-<slug>` when topology is `monorepo-submodules`.

## Safety

Cleanup removes a worktree only when its branch is fully merged into the
base and the worktree is clean. It never removes dirty or unmerged worktrees,
and never touches the main or detached-HEAD worktrees. Prefer this verb over
ad-hoc `git worktree remove` after merge. Uninitialized submodules (`-` status)
are skipped and never `git -C`'d.
