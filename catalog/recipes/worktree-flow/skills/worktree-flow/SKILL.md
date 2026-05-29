---
name: worktree-flow
description: >
  Isolated git worktree workflow for ai-specs change work. Create a dedicated
  worktree under .worktrees/ for any change that writes files, keep pure
  exploration outside a worktree, and clean up merged worktrees safely after
  integration.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Starting a change that will write files or modify code"
    - "Cleaning up worktrees after a branch is merged"
---

# Worktree Flow

Run file-writing change work in an isolated git worktree under `.worktrees/`,
and remove worktrees safely once their branch is merged. Pure exploration that
writes nothing does not need a worktree.

## When to create a worktree

| Situation | Worktree? |
|---|---|
| Exploration / reading / answering a question (no files written) | No |
| Implementing a change, writing artifacts, editing code | Yes |
| Any phase that produces committed output | Yes |

## Conventions

- One worktree per change, located at `<worktrees_dir>/<branch-slug>` (default
  `worktrees_dir` is `.worktrees`).
- Branch off the integration branch declared in recipe config
  (`integration_branch`, default `main`).
- Branch name and directory slug match so cleanup can map them 1:1.
- Add `<worktrees_dir>/` to `.gitignore` so worktree checkouts are never
  committed into the parent tree.

## Creating a worktree

Prefer the `/worktree-new` command. Equivalent manual form:

```bash
git worktree add .worktrees/<slug> -b <branch> <integration_branch>
```

## Post-merge cleanup

After a branch is merged into the integration branch, reclaim its worktree with
the materialized cleanup script (or the `/worktree-clean` command):

```bash
bash ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh \
  --dir <worktrees_dir> --base <integration_branch>
```

The script is conservative by design:

- **Removes** a worktree only when its branch is fully merged into the base.
- **Preserves** worktrees with uncommitted changes (reported as `dirty`).
- **Preserves** worktrees whose branch is not yet merged (`unmerged`).
- **Never touches** the main worktree or detached-HEAD worktrees.

Run with `--dry-run` first to preview removals. This honors the project rule:
never revert or discard changes you did not make.
