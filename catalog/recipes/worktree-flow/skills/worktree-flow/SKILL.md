---
name: worktree-flow
description: >
  Optional isolated git worktree workflow for ai-specs change work. Worktree
  creation is opt-in via recipes.worktree-flow.config.creation_mode (default
  ask): offer one when intent is unclear or the current branch conflicts, never
  by default, and clean up merged worktrees safely after topologies via
  recipes.worktree-flow.config.repo_topology and topology-aware gate scope via
  recipes.worktree-flow.config.gate_scope.
license: MIT
metadata:
  author: ai-specs
  version: "1.2"
  scope: runtime
  auto_invoke:
    - "Deciding whether a change needs its own worktree"
    - "The user asked for an isolated worktree"
    - "Cleaning up worktrees after a branch is merged"
---

# Worktree Flow

Isolate change work in a git worktree under `.worktrees/` **when isolation is
actually warranted**, and remove worktrees safely once their branch is merged.
Creating one is not a default step of the flow: writing files is not by itself a
reason to branch off. Ask when the user's intent is unclear or the current branch
conflicts with the work, and otherwise stay in the current checkout.

## When to create a worktree

A worktree is **offered, not imposed**. `creation_mode` decides how the flow
behaves (default `ask`):

| `creation_mode` | Behavior |
|---|---|
| `ask` (default) | Never create one silently. Work in the current checkout unless a trigger below fires, and when one does, ask and wait. |
| `always` | Create one for any change that writes files (the previous default). |
| `never` | Never create one; the user manages checkouts themselves. |

Under `ask`, these are the only triggers — a change writing files is **not** one
of them by itself:

| Trigger | What to do |
|---|---|
| The user's intent is unclear — you cannot tell whether this belongs on the current branch or on a new one | Ask: current branch, or a new worktree? Wait for the answer. |
| The current branch conflicts with the work — it is a protected branch, it already carries unrelated uncommitted changes, or the work belongs to a different change than the branch is named for | Explain the conflict, propose a worktree, and wait. |
| The user asked for one (`/worktree-new`, "hacelo en un worktree") | Create it. |
| None of the above — intent is clear and the current branch fits | Do **not** create one. Work where you are. |

Never create a worktree to satisfy a gate. A blocking write gate means the
branch is wrong for this work: surface that to the user and let them choose.

SDD artifact phases are no exception. Writing `proposal.md`, `spec.md`,
`design.md`, or `tasks.md` does not itself justify a new worktree — those phases
follow the same triggers as any other work.

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

## Request context

Resolve one ai-specs request context with `util.resolve_request_context` before
any create or artifact write: `owner_root` (the repository owning code/VCS for
the request) is distinct from `planning_root` (the canonical planning tree). A
subrepo request owns the submodule but plans under the proven superproject; a
superrepo request owns and plans under the superproject. Missing, ambiguous,
detached, or uninitialized topology fails safe — no owner inference beyond the
toplevel, no planning-root exception. `/worktree-new` is generated Markdown
executed by the agent; there is NO executable `/worktree-new` helper.

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
  using bash to write bypasses the worktree gate entirely. Stop, say the branch
  is protected, and let the user choose the destination: another branch, or a
  worktree they approve. Creating one unasked is not the fix.

## Creating a worktree

Prefer the `/worktree-new` command.

### standalone / monorepo-apps

```bash
git worktree add <worktrees_dir>/<slug> -b <branch> <integration_branch>
```

### monorepo-submodules (locked contract)

Destination MUST be absolute. Infer or require `<subrepo>`; validate path-then
unique name; reject uninitialized (`git submodule update --init <path>`).
`<subrepo>` selection and the owner/planning-root split are validated by
`util.resolve_request_context` / `util.resolve_subrepo`. A request whose
context is the **superrepo** MUST NOT infer a subrepo: an explicit, validated
`<subrepo>` is required, otherwise hard-error **before any `git worktree add`**.

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

Prefer the materialized verified-Go cleanup launcher (or `/worktree-clean`) over
ad-hoc remove sequences. Run it from the main repository worktree; it owns
post-merge remote deletion and independently verifies remote absence:


```bash
bash ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh \
  --dir <worktrees_dir> --base <integration_branch>
```

Optional `--submodule` / `--subrepo` scopes to one module; default = all
initialized. Run with `--dry-run` first to preview removals.

The launcher is conservative by design:

- **Removes** a worktree only when its branch is fully merged into the base —
  detecting both regular/fast-forward merges (ancestry) and squash/rebase merges
  (all of the branch's changes already present in base by patch-id).
- **Preserves** worktrees with uncommitted changes (reported as `dirty`).
- **Preserves** worktrees whose branch is not yet merged (`unmerged`).
- **Never touches** the main worktree or detached-HEAD worktrees.
- Refuses loudly before every worktree, local-branch, or remote-branch delete
  when the branch is protected (`main`, `master`, `development`, `staging`,
  or configured base/integration names).
- Deletes the remote branch only after local cleanup and verifies absence with
  `git ls-remote --heads`; failure is not reported as success.
- Under submodules, enumerates per-module lists; uninitialized modules are
  skipped.

### Manual fallback

Only if the script is unavailable: from the main repo root, `git worktree
remove <path>`, then `git branch -D <branch>` after squash/rebase merges.
Stop without deleting if the worktree is dirty.

This honors the project rule: never revert or discard changes you did not make.
