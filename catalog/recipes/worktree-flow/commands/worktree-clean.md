# worktree-clean

Reclaim git worktrees whose branch has been merged into the integration branch.

## When to use

After one or more change branches have been merged, to remove their worktrees
under `<worktrees_dir>/` (default `.worktrees/`).

## Topology

Under resolved `monorepo-submodules`, cleanup enumerates **per initialized
submodule** (`git -C <module> worktree list` / `git submodule foreach`). Never
trust the superproject `git worktree list` alone — it does not show submodule
linked worktrees. `standalone` / `monorepo-apps` keep a single-repo pass.

Optional scope flags (forwarded to the script):

- `--topology <value>` — `auto` (default / stamped), `standalone`,
  `monorepo-apps`, or `monorepo-submodules`. Pass the project's configured
  `repo_topology` so an explicit `standalone`/`monorepo-apps` does **not**
  enumerate submodules even when `.gitmodules` exists (vendored-submodule
  mitigation). Sync stamps `__WORKTREE_REPO_TOPOLOGY__` like gate_mode.
- `--submodule <path>` / `--subrepo <path>` (repeatable) — limit to one or more
  initialized modules. Default = all initialized submodules.
- On a standalone repo these flags are inert (single pass), not an error.

## Steps

0. Ensure your shell is **outside** the worktree you might remove (`cd` to the
   main / superproject repository root first).

1. Preview what would be removed (safe, mutates nothing):

   ```bash
   bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
     --dir <worktrees_dir> --base <integration_branch> --dry-run
   ```

   Scope example:

   ```bash
   bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
     --dir <worktrees_dir> --base <integration_branch> \
     --submodule apps/api --dry-run
   ```

2. Review the output. Lines mean:
   - `would remove <name>` — merged + clean, eligible for removal.
   - `skipped <name> (dirty)` — has uncommitted changes; left untouched.
   - `skipped <name> (unmerged)` — branch not merged yet; left untouched.

3. Apply the cleanup once the preview looks right:

   ```bash
   bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
     --dir <worktrees_dir> --base <integration_branch>
   ```

Use the `worktrees_dir` and `integration_branch` values from
`[recipes.worktree-flow.config]` in `ai-specs/ai-specs.toml`. Shared layout:
worktrees live under the **superproject** `<worktrees_dir>/` as
`<subrepo>-<slug>` when topology is `monorepo-submodules`.

## Safety

The script removes a worktree only when its branch is fully merged into the
base and the worktree is clean. It never removes dirty or unmerged worktrees,
and never touches the main or detached-HEAD worktrees. Prefer this script over
ad-hoc `git worktree remove` after merge. Uninitialized submodules (`-` status)
are skipped and never `git -C`'d.
