# Git PR Flow

Provider-oriented PR and merge flow for feature branches. Defaults to GitHub via the
`gh` CLI, with the provider and base branch configurable per project.

## What it provides

- **Skill** `git-merge-workflow` (bundled) — the full provider-oriented merge workflow
  for feature branches created in worktrees: push, open PR, approval-gated merge, and
  worktree/branch cleanup.
- **Command** `/pr-create` — a thin agent-facing command that pushes the branch, opens a
  PR against the configured base branch, and only merges after explicit user approval.

## Capability

Declares the `vcs-pr-flow` capability so it can be bound as the project's
VCS/PR flow provider.

## Providers

Today, `provider = "github"` (via the `gh` CLI) is the **only implemented provider**.
Other providers (e.g. `gitlab`) are intended as future **sibling recipes** that also
provide the `vcs-pr-flow` capability. A project selects exactly one provider recipe for
that capability through a manifest `[[bindings]]` entry, so swapping providers is a
binding change — not a rewrite of this recipe.

## Enable in `ai-specs.toml`

```toml
[recipes.git-pr-flow]
enabled = true
version = "1.1.0"

[recipes.git-pr-flow.config]
provider = "github"
base_branch = "main"
```

Run `ai-specs sync` to materialize the bundled skill, the `/pr-create` command, and
this doc into the project.

## Config

| Key           | Required | Type   | Default    | Description                                          |
| ------------- | -------- | ------ | ---------- | ---------------------------------------------------- |
| `provider`    | no       | string | `github`   | VCS/PR provider. `github` (via `gh` CLI) is the only implemented provider. |
| `base_branch` | no       | string | `main`     | Base branch the PR targets (e.g. `main`, `develop`). |

## Safety note

Never push, create, or merge a PR without explicit user instruction. The `/pr-create`
command stops after opening the PR and waits for explicit approval before any merge.
Feature work goes through a PR — never a local `git merge`.
