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
| Planning or SDD artifact phases that write files | Yes |
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

After a branch is merged into the integration branch, reclaim its worktree.

### Leave the worktree first (hard rule)

Never run `git worktree remove` (or the cleanup script) while `$PWD` is inside
the worktree being removed — that yields `fatal: Unable to read current working
directory`. Always:

```bash
cd <main-repo-root>
```

### Script-first (preferred)

Prefer the materialized cleanup script (or `/worktree-clean`) over ad-hoc
remove sequences:

```bash
bash ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh \
  --dir <worktrees_dir> --base <integration_branch>
```

Run with `--dry-run` first to preview removals.

The script is conservative by design:

- **Removes** a worktree only when its branch is fully merged into the base —
  detecting both regular/fast-forward merges (ancestry) and squash/rebase merges
  (all of the branch's changes already present in base by patch-id).
- **Preserves** worktrees with uncommitted changes (reported as `dirty`).
- **Preserves** worktrees whose branch is not yet merged (`unmerged`).
- **Never touches** the main worktree or detached-HEAD worktrees.

### Manual fallback

Only if the script is unavailable: from the main repo root, `git worktree
remove <path>`, then `git branch -D <branch>` after squash/rebase merges.
Stop without deleting if the worktree is dirty.

This honors the project rule: never revert or discard changes you did not make.
