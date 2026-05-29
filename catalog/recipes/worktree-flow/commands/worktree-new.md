# worktree-new

Create an isolated git worktree for a change that will write files.

## When to use

Before implementing a change, writing artifacts, or editing code. Skip it for
pure exploration that writes nothing.

## Steps

1. Resolve a branch slug from the change name (kebab-case, e.g. `feat/x` →
   directory `feat-x`).
2. Ensure `<worktrees_dir>/` (default `.worktrees/`) is listed in `.gitignore`.
3. Create the worktree off the integration branch:

   ```bash
   git worktree add <worktrees_dir>/<slug> -b <branch> <integration_branch>
   ```

   Use the `worktrees_dir` and `integration_branch` values from
   `[recipes.worktree-flow.config]` in `ai-specs/ai-specs.toml` (defaults
   `.worktrees` and `main`).
4. Do all file-writing work for the change inside that worktree.

## Notes

- One worktree per change; keep the branch name and directory slug identical.
- Preserve unrelated worktrees — never remove a worktree you did not create.
