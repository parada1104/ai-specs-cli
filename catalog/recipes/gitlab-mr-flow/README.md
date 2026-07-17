# GitLab MR Flow

**GitLab merge request (MR) and merge flow for feature branches** via the `glab` CLI.
The base branch is configurable per project.

## What it provides

- **Skill** `gitlab-merge-workflow` (bundled) — push, open MR, approval-gated merge, and
  worktree/branch cleanup for GitLab.
- **Command** `/mr-create` — thin agent-facing command; stops after opening the MR.

## Capability

Declares the `vcs-pr-flow` capability. Bind this recipe when the project's host is GitLab:

```toml
[[bindings]]
capability = "vcs-pr-flow"
recipe = "gitlab-mr-flow"
```

Sibling recipes cover GitHub ([`git-pr-flow`](../git-pr-flow/README.md)) and
Bitbucket ([`bitbucket-pr-flow`](../bitbucket-pr-flow/README.md)).

## Prerequisites

- **`glab` CLI** installed and authenticated (`glab auth status`).
- **`jq`** installed (required for SHA pinning during merge).

## Enable in `ai-specs.toml`

```toml
[recipes.gitlab-mr-flow]
enabled = true
version = "1.2.0"

[recipes.gitlab-mr-flow.config]
base_branch = "development"
expected_owner = ""
auto_switch_account = false
```

Run `ai-specs sync` to materialize the bundled skill, `/mr-create`, and this doc.

## Config

| Key                   | Required | Type    | Default        | Description |
| --------------------- | -------- | ------- | -------------- | ----------- |
| `base_branch`         | no       | string  | `development`  | Base branch the MR targets. |
| `expected_owner`      | no       | string  | `""`           | Account username this repo expects; activates auth preflight when set. |
| `auto_switch_account` | no       | boolean | `false`        | Reserved for API parity; glab has no auth switch — mismatch blocks with guidance. |

## Long-lived branches

Protected heads (`main`, `master`, `development`, `staging`, plus configured
`base_branch` / `integration_branch`) are **not** deleted after merge. The skill
passes `--remove-source-branch` only for feature heads. Keep GitLab UI "Delete
source branch" off for protected heads. Prefer `release/vX.Y.Z` → `main` for
releases, not `development` as the MR source.

## Safety note

Never push, create, or merge an MR without explicit user instruction. Feature work goes
through an MR — never a local `git merge`.
