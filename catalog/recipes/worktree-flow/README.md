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
version = "1.0.0"

[recipes.worktree-flow.config]
worktrees_dir = ".worktrees"
integration_branch = "main"
auto_remove_merged = true
```

Then run `ai-specs sync`. The cleanup script materializes to
`ai-specs/recipes/worktree-flow/bin/worktree-cleanup.sh`.

## Worktree-gate modes

`worktree-flow` also gates writes to the main worktree on protected branches via
`gate_mode`:

| Mode | Behavior |
|---|---|
| `always` | Current strict behavior: block writes to the main worktree on protected branches. |
| `ask` | Block, but surface a bypass hint: rerun with `WORKTREE_GATE_MODE=off` for that one invocation. |
| `off` | Disable the gate entirely; writes are allowed even on protected branches. |

Default: `always`.

## Config

| Key | Default | Meaning |
|---|---|---|
| `worktrees_dir` | `.worktrees` | Directory that holds per-change worktrees. |
| `integration_branch` | `main` | Branch worktrees are created from and merged into. |
| `auto_remove_merged` | `true` | Whether merged worktrees are eligible for cleanup. |
| `gate_mode` | `always` | Main-worktree gate mode: `always`, `ask`, or `off`. |
| `WORKTREE_GATE_PROTECTED` | `main development` | Space-separated branch names where the `worktree-gate` hook blocks Edit/Write in the main worktree. Passed to the rendered hook as the `WORKTREE_GATE_PROTECTED` env var. |

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
