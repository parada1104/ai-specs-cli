# Git PR Flow

**GitHub pull request (PR) and merge flow for feature branches** via the `gh` CLI.
The base branch is configurable per project.

## What it provides

- **Skill** `git-merge-workflow` (bundled) — push, open PR, approval-gated merge, and
  worktree/branch cleanup for GitHub.
- **Command** `/pr-create` — thin agent-facing command; stops after opening the PR.

## Capability

Declares the `vcs-pr-flow` capability. Bind this recipe when the project's host is GitHub:

```toml
[[bindings]]
capability = "vcs-pr-flow"
recipe = "git-pr-flow"
```

Sibling recipes cover GitLab ([`gitlab-mr-flow`](../gitlab-mr-flow/README.md)) and
Bitbucket ([`bitbucket-pr-flow`](../bitbucket-pr-flow/README.md)).

## Enable in `ai-specs.toml`

```toml
[recipes.git-pr-flow]
enabled = true
version = "1.3.0"

[recipes.git-pr-flow.config]
base_branch = "main"
expected_owner = ""
auto_switch_account = false
```

Run `ai-specs sync` to materialize the bundled skill, `/pr-create`, and this doc.

## Config

| Key                   | Required | Type    | Default | Description |
| --------------------- | -------- | ------- | ------- | ----------- |
| `base_branch`         | no       | string  | `main`  | Base branch the PR targets. |
| `expected_owner`      | no       | string  | `""`    | Account username this repo expects; activates auth preflight when set. |
| `auto_switch_account` | no       | boolean | `false` | gh only: auto-switch CLI account on mismatch (requires gh ≥ 2.50.0). |

## Prerequisites

- **`gh` CLI** installed and authenticated.

## Long-lived branches

Protected heads (`main`, `master`, `development`, `staging`, plus configured
`base_branch` / `integration_branch`) are **not** deleted after merge. The merge
skill warns when GitHub `delete_branch_on_merge` is enabled (repo-wide; no
per-branch exempt) and documents:

```bash
gh api -X PATCH repos/{owner}/{repo} -f delete_branch_on_merge=false
```

Feature heads (including `release/vX.Y.Z`) are deleted explicitly by the skill.
Prefer `release/vX.Y.Z` → `main` for releases, not `development` as the PR head.

## Safety note

Never push, create, or merge a PR without explicit user instruction. Feature work goes
through a PR — never a local `git merge`.
