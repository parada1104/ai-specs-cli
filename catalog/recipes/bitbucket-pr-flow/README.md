# Bitbucket PR Flow

**Bitbucket pull request (PR) and merge flow for feature branches** via the `bb` CLI.
The base branch is configurable per project.

## What it provides

- **Skill** `bitbucket-merge-workflow` (bundled) — push, open PR, approval-gated merge, and
  worktree/branch cleanup for Bitbucket.
- **Command** `/bb-pr-create` — thin agent-facing command; stops after opening the PR.

## Capability

Declares the `vcs-pr-flow` capability. Bind this recipe when the project's host is Bitbucket:

```toml
[[bindings]]
capability = "vcs-pr-flow"
recipe = "bitbucket-pr-flow"
```

Sibling recipes cover GitHub ([`git-pr-flow`](../git-pr-flow/README.md)) and
GitLab ([`gitlab-mr-flow`](../gitlab-mr-flow/README.md)).

## Prerequisites

- **`bb` CLI** installed and authenticated (`bb auth status`). Install from
  https://bitbucket-cli.paulvanderlei.com/getting-started/installation/

## Enable in `ai-specs.toml`

```toml
[recipes.bitbucket-pr-flow]
enabled = true
version = "1.0.0"

[recipes.bitbucket-pr-flow.config]
base_branch = "development"
```

Run `ai-specs sync` to materialize the bundled skill, `/bb-pr-create`, and this doc.

## Config

| Key           | Required | Type   | Default        | Description |
| ------------- | -------- | ------ | -------------- | ----------- |
| `base_branch` | no       | string | `development`  | Base branch the PR targets. |

## Safety note

Never push, create, or merge a PR without explicit user instruction. Feature work goes
through a PR — never a local `git merge`.
