# Plan / Build Flow (Ambient)

An invisible change workflow: the bundled skill auto-invokes on substantial
requests, **classifies planning depth**, produces reviewable artifacts, waits
for authorization, then implements, validates, and closes — **without** `/plan`
or `/build` commands.

## What it provides

- **Skill `plan-build-flow`** (bundled) — depth classifier (full / standard /
  light), ambient planning/build contract, PR artifact gate, pre-merge archive
  gate, and graceful degradation without an orchestrator.
- **Doc** — this README at `ai-specs/recipes/plan-build-flow/README.md`.

## Planning depth (classifier)

| Tier | Typical use | Artifacts |
|------|-------------|-----------|
| **Full** | New capability, architecture, breaking or ambiguous scope | explore → proposal → spec → design → tasks |
| **Standard** | Scoped feature or bounded multi-file work | spec → tasks |
| **Light** | Small fix with clear file/symbol target | tasks only |

Direct "implement this" requests still run the classifier first when no change
folder exists yet.

## PR and merge gates

- **No PR** until the change folder on the branch has the tier minimum files
  (at least `tasks.md`) committed.
- **Archive before merge** on the review branch — never after merge lands on the
  base branch.

## Capability

- `plan-build-flow` — ambient plan/build workflow (skill-only surface).

## Enable

```toml
[recipes.plan-build-flow]
enabled = true
version = "1.3.0"
```

Then run `ai-specs sync`.

## Delivery contracts

`artifact_store_default` is an optional string configuration that declares where this
project's planning artifacts live by default. It accepts `openspec`, `engram`, or `both`,
and defaults to `openspec` when the project manifest does not override it.

```toml
[recipes.plan-build-flow.config]
artifact_store_default = "both"
```

During `ai-specs sync`, the resolved value is materialized into the generated brief as a
workflow rule. An external session runtime may consume that rule when asked where planning
artifacts should live; this recipe declares the repository default but does not control that
runtime's session behavior.

## Worktree coexistence

Implementation defers to `worktree-flow` when enabled. The recipe syncs
standalone without a hard dependency.
