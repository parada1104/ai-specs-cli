# Worktree Flow recipe

Isolated git worktrees under `.worktrees/` for ai-specs change work, with safe
post-merge cleanup.

## What it provides

- **Skill `worktree-flow`** — when to create a worktree (file-writing work) vs.
  stay outside one (pure exploration), naming conventions, and cleanup rules.
- **Commands `/worktree-new`, `/worktree-clean`** — agent-facing flows to create
  a worktree for a change and to reclaim merged worktrees.
- **Script `bin/worktree-cleanup.sh`** — conservative cleanup: removes only
  merged + clean worktrees, preserves dirty and unmerged ones, never touches the
  main worktree.

## Enable

```toml
[recipes.worktree-flow]
enabled = true
version = "1.3.0"

[recipes.worktree-flow.config]
worktrees_dir = ".worktrees"
integration_branch = "main"
auto_remove_merged = true
repo_topology = "auto"
```

Then run `ai-specs sync`. The cleanup script materializes to
`ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh`.

## Worktree-gate modes

`worktree-flow` also gates writes to the main worktree on protected branches via
`gate_mode`:

| Mode | Behavior |
|---|---|
| `always` | Current strict behavior: block writes to the main worktree on protected branches. |
| `ask` | Block, but surface a bypass hint: rerun with `WORKTREE_GATE_MODE=off` for that one invocation. |
| `off` | Disable the gate entirely; writes are allowed even on protected branches. |

Default: `always`.

**Delegation caveat:** the gate is a `pre-tool-use` hook. On opencode/pi/omp it
may not see tool calls made inside a delegated subagent/task (separate process
or host gap — see `docs/runtime-hooks.md`). Before dispatching write-capable
subagents, verify worktree and branch yourself; do not rely on the hook alone.

**Shell-write coverage:** the same gate also best-effort blocks shell/bash
commands (`>`, `>>`, `tee`, `sed -i`/`perl -i`, `cp`/`mv`, interpreter
heredoc/`-c` write calls) that would write into the protected main worktree —
closing the gap where an agent falls back to bash after a blocked or errored
Edit/Write. This is a **heuristic, not a sandbox**: obfuscated or multi-stage
writers (`awk`, `dd`, base64-piped content, opaque `bash -c "$(...)"`) can
still evade it by design (fail-open on ambiguity), and coverage is uneven by
harness — see the coverage matrix in `docs/runtime-hooks.md`.

## Config

| Key | Default | Meaning |
|---|---|---|
| `worktrees_dir` | `.worktrees` | Directory that holds per-change worktrees. |
| `integration_branch` | `main` | Branch worktrees are created from and merged into. |
| `auto_remove_merged` | `true` | Whether merged worktrees are eligible for cleanup. |
| `gate_mode` | `always` | Main-worktree gate mode: `always`, `ask`, or `off`. |
| `repo_topology` | `auto` | Repository topology: `auto` (initialized `.gitmodules` → `monorepo-submodules`, else `standalone`), `standalone`, `monorepo-apps` (naming-only; same mechanics as standalone), or `monorepo-submodules`. |
| `WORKTREE_GATE_PROTECTED` | `main development` | Space-separated branch names where the `worktree-gate` hook blocks Edit/Write in the main worktree. Passed to the rendered hook as the `WORKTREE_GATE_PROTECTED` env var. |


## Repo topologies

| Resolved topology | Create | Clean |
|---|---|---|
| `standalone` | `git worktree add <worktrees_dir>/<slug> …` | Single-repo scan (unchanged) |
| `monorepo-apps` | Same as standalone (naming-only) | Same as standalone |
| `monorepo-submodules` | `git -C <subrepo> worktree add <absolute>/<worktrees_dir>/<subrepo>-<slug> …` | Enumerate each initialized submodule; never superproject `worktree list` alone |

Shared layout: worktrees always live under the **superproject**
`<worktrees_dir>/` (default `.worktrees/`). Under submodules the directory name
is `<subrepo>-<slug>`.

## Stale cleanup override

The cleanup script uses `condition = "not_exists"`, so sync will not overwrite a
hand-edited override. If your
`ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh` drifts from
the catalog template, `ai-specs sync` (and doctor) emit a non-blocking WARN.
Refresh with:

```bash
rm ai-specs/recipes/worktree-flow/overrides/bin/worktree-cleanup.sh
ai-specs sync
```

Then re-apply any local customizations.

## Cleanup contract

| Worktree state | Action |
|---|---|
| Branch merged into base (regular **or** squash/rebase), clean | removed |
| Uncommitted changes | preserved (`dirty`) |
| Branch not merged | preserved (`unmerged`) |
| Main / detached HEAD | never touched |

Squash/rebase merges are detected by patch-id (`git cherry`), since the squashed
commit is not an ancestor of the base branch.

Run with `--dry-run` to preview before removing anything.
