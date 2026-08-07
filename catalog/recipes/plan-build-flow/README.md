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

The classifier always computes a signal tier and separately checks for an
explicit requested depth. Illustrative requests such as “full planning”,
“acotado con spec”, or “solo tasks” are compared with that signal; this is
guidance, not an exhaustive parser. A mismatch is a depth conflict: the agent
must ask which tier wins in either direction before writing that planning chain.
When the conflict is resolved, `tasks.md` records standalone `Depth: <tier>` plus
the requested, signal, decided, and decision-source annotation lines. A same-turn
preference may resolve the conflict without a repeat ask; a deeper decision still
requires its complete planning chain.

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
version = "1.5.0"
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

## Cross-repository planning boundaries

When a recognized submodule code worktree is used, repository topology identifies
the containing **superproject** and its **central** planning tree. Central active
plan lookup and artifact writes stay within that planning tree; they never grant a
superproject-wide production-write bypass. A **standalone** repository continues
to use its own nearest planning root. If the topology cannot be proven, resolution
is **fail-safe** and retains nearest-repository behavior. The recipe has **no
duplication** and **no orchestration** boundary: it does not copy, synchronize, or
create per-repository plans or change worktrees and branches.
