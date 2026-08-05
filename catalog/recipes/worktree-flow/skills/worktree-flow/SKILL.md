---
name: worktree-flow
description: >
  Isolated git worktree workflow for ai-specs change work. Create a dedicated
  worktree under .worktrees/ for any change that writes files, keep pure
  exploration outside a worktree, and clean up merged worktrees safely after
  topologies via recipes.worktree-flow.config.repo_topology and topology-aware
  gate scope via recipes.worktree-flow.config.gate_scope.
license: MIT
metadata:
  author: ai-specs
  version: "1.2"
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

## Topology matrix

| Resolved topology | Create | Clean |
|---|---|---|
| `standalone` | `git worktree add <worktrees_dir>/<slug> -b <branch> <integration_branch>` | Single-repo `worktree list` + flush (unchanged) |
| `monorepo-apps` | Same as standalone (naming-only) | Same as standalone |
| `monorepo-submodules` | `git -C <subrepo_path> worktree add <absolute-super>/<worktrees_dir>/<subrepo>-<slug> -b <branch> <integration_branch>` | Enumerate each initialized submodule (`git -C` / `submodule foreach`); never superproject `worktree list` alone |

`repo_topology = "auto"` (default) detects initialized `.gitmodules` entries →
`monorepo-submodules`, else `standalone`. It never auto-selects `monorepo-apps`.

## Gate scope and repository ownership

`gate_scope` is separate from `gate_mode` and `repo_topology`. Classification
requires effective `repo_topology=monorepo-submodules`; explicit
`standalone`/`monorepo-apps` never create a topology bypass:

| Value | Protected owner enforced | Runtime behavior |
|---|---|---|
| `auto` | Proven superrepo and subrepo | Default topology-derived protection; only canonical superrepo planning is excepted. |
| `superrepo` | Proven superrepo only | Subrepo writes are outside this selected enforcement scope. |
| `subrepo` | Proven initialized subrepo only | Superrepo writes are outside this explicit Melón scope. |

Before any write-capable delegation in a cross-repository layout, identify the
owning repository and branch, not just the current directory:

```bash
git rev-parse --show-toplevel
git rev-parse --absolute-git-dir
git rev-parse --git-common-dir
git branch --show-current
```

Central planning artifacts belong to the proven superrepo canonical subtree
`<superrepo>/openspec/changes/**`; subrepo code remains subject to its own
protected branch and the separate `plan-build-flow` authorization. If sync or
doctor reports a stale materialized hook without the scope stamp, refresh it
explicitly with `rm <hook-path> && ai-specs sync`; customized hook bytes are
never silently overwritten. The runtime hook is a defense in depth, not the
sole guard for delegated or subprocess writes.

## Conventions

- Shared layout: worktrees live under the **superproject**
  `<worktrees_dir>/<…>` (default `.worktrees/`). Under submodules the directory
  is `<subrepo>-<slug>`.
- Branch off the integration branch declared in recipe config
  (`integration_branch`, default `main`).
- Branch name and directory slug match so cleanup can map them 1:1.
- Add `<worktrees_dir>/` to the superproject `.gitignore`.
- Before dispatching a write-capable subagent or task, verify **which git
  repository**, worktree, and branch yourself (`git rev-parse --show-toplevel`,
  `git branch --show-current`, `git worktree list`). Under
  `monorepo-submodules`, confirming the toplevel (which submodule / linked wt)
  is mandatory — not only branch + worktree list. Runtime pre-tool-use hooks
  may not fire for delegated/subprocess tool calls on opencode/pi/omp — do not
  treat the gate as the sole guard for delegated writes.
- If a structured Edit/Write/MultiEdit call is blocked or errors for any
  reason while on a protected branch, that is never grounds to retry the
  write via bash/shell (heredoc, `python3 -c`, `cat >`, `tee`, `sed -i`) —
  using bash to write bypasses the worktree gate entirely. Create a worktree
  first (`/worktree-new`) and write there instead.

## Creating a worktree

Prefer the `/worktree-new` command.

### standalone / monorepo-apps

```bash
git worktree add <worktrees_dir>/<slug> -b <branch> <integration_branch>
```

### monorepo-submodules (locked contract)

Destination MUST be absolute. Infer or require `<subrepo>`; validate path-then
unique name; reject uninitialized (`git submodule update --init <path>`).
`<subrepo>` selection is validated by `util.resolve_subrepo`.

```bash
super_abs="$(git -C "$super_root" rev-parse --show-toplevel)"
git -C "$super_abs/<subrepo_path>" worktree add \
  "$super_abs/<worktrees_dir>/<subrepo>-<slug>" \
  -b <branch> <integration_branch>
```

cwd inference uses `git -C`/`rev-parse --show-toplevel` and, for linked
worktrees named `<path>-<slug>`, the **longest** matching submodule path
prefix (see `util.resolve_subrepo`).

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
bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
  --dir <worktrees_dir> --base <integration_branch>
```

Optional `--submodule` / `--subrepo` scopes to one module; default = all
initialized. Run with `--dry-run` first to preview removals.

The script is conservative by design:

- **Removes** a worktree only when its branch is fully merged into the base —
  detecting both regular/fast-forward merges (ancestry) and squash/rebase merges
  (all of the branch's changes already present in base by patch-id).
- **Preserves** worktrees with uncommitted changes (reported as `dirty`).
- **Preserves** worktrees whose branch is not yet merged (`unmerged`).
- **Never touches** the main worktree or detached-HEAD worktrees.
- Under submodules, enumerates per-module lists; uninitialized modules are
  skipped.

### Manual fallback

Only if the script is unavailable: from the main repo root, `git worktree
remove <path>`, then `git branch -D <branch>` after squash/rebase merges.
Stop without deleting if the worktree is dirty.

This honors the project rule: never revert or discard changes you did not make.
