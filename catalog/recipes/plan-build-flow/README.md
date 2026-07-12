# Plan / Build Flow (Ambient)

An invisible change workflow: the bundled skill auto-invokes on substantial
requests to produce reviewable planning artifacts, waits for authorization, then
implements, validates, and closes — **without** `/plan` or `/build` commands.

## What it provides

- **Skill `plan-build-flow`** (bundled) — ambient planning/build contract,
  internal phase mapping, and graceful degradation without an orchestrator.
- **Doc** — this README at `ai-specs/recipes/plan-build-flow/README.md`.

## Capability

- `plan-build-flow` — ambient plan/build workflow (skill-only surface).

## Enable

```toml
[recipes.plan-build-flow]
enabled = true
version = "2.0.0"
```

Then run `ai-specs sync`.

## Config

None. Change slug and artifact store resolve per session.

## Worktree coexistence

Implementation defers to `worktree-flow` when enabled. The recipe syncs
standalone without a hard dependency.
