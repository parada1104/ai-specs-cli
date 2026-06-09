# GitLab MR Flow

**Provider-oriented merge request (MR) and merge flow for feature branches.** Defaults
to GitLab via the `glab` CLI, with the provider and base branch configurable per project.

## What it provides

- **Skill** `gitlab-merge-workflow` (bundled) — the full provider-oriented merge
  workflow for feature branches created in worktrees: push, open MR, approval-gated
  merge, and worktree/branch cleanup.
- **Command** `/mr-create` — a thin agent-facing command that pushes the branch, opens
  an MR against the configured base branch, and only merges after explicit user approval.

## Capability

Declares the `vcs-pr-flow` capability so it can be bound as the project's
VCS/MR flow provider.

## Prerequisites

- **`glab` CLI** installed and authenticated (`glab auth status` to verify).
- **`jq`** installed (required for SHA pinning during merge). Install from https://jqlang.github.io/jq/download/.
- A GitLab remote configured on the repository (`git remote -v`).

## Providers

Today, `provider = "gitlab"` (via the `glab` CLI) is the **only implemented provider**
for this recipe. The sibling recipe [`git-pr-flow`](../git-pr-flow/README.md) covers
GitHub via the `gh` CLI. Both recipes provide the `vcs-pr-flow` capability — a project
selects exactly one through a manifest `[[bindings]]` entry, so swapping providers is a
binding change, not a rewrite.

## Enable in `ai-specs.toml`

```toml
[recipes.gitlab-mr-flow]
enabled = true
version = "1.0.0"

[recipes.gitlab-mr-flow.config]
provider = "gitlab"
base_branch = "development"
```

Run `ai-specs sync` to materialize the bundled skill, the `/mr-create` command, and
this doc into the project.

## Config

| Key           | Required | Type   | Default      | Description                                          |
| ------------- | -------- | ------ | ------------ | ---------------------------------------------------- |
| `provider`    | no       | string | `gitlab`     | VCS/MR provider. `gitlab` (via `glab` CLI) is the only implemented provider. |
| `base_branch` | no       | string | `development`| Base branch the MR targets (e.g. `main`, `development`). |

## Safety note

Never push, create, or merge an MR without explicit user instruction. The `/mr-create`
command stops after opening the MR and waits for explicit approval before any merge.
Feature work goes through an MR — never a local `git merge`.
