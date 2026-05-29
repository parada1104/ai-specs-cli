# worktree-clean

Reclaim git worktrees whose branch has been merged into the integration branch.

## When to use

After one or more change branches have been merged, to remove their worktrees
under `<worktrees_dir>/` (default `.worktrees/`).

## Steps

1. Preview what would be removed (safe, mutates nothing):

   ```bash
   bash ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh \
     --dir <worktrees_dir> --base <integration_branch> --dry-run
   ```

2. Review the output. Lines mean:
   - `would remove <name>` — merged + clean, eligible for removal.
   - `skipped <name> (dirty)` — has uncommitted changes; left untouched.
   - `skipped <name> (unmerged)` — branch not merged yet; left untouched.

3. Apply the cleanup once the preview looks right:

   ```bash
   bash ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh \
     --dir <worktrees_dir> --base <integration_branch>
   ```

Use the `worktrees_dir` and `integration_branch` values from
`[recipes.worktree-flow.config]` in `ai-specs/ai-specs.toml`.

## Safety

The script removes a worktree only when its branch is fully merged into the
base and the worktree is clean. It never removes dirty or unmerged worktrees,
and never touches the main or detached-HEAD worktrees.
