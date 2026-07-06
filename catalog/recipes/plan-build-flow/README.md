# Plan / Build Flow

A two-verb workflow for change work: `/plan` turns an intent into a reviewable
plan, and `/build` implements an authorized plan, validates it, and closes the
change out — all without exposing a third command.

## What it provides

- **Skill `plan-build-flow`** (bundled) — the two-verb contract, how `/plan`
  and `/build` map to the underlying phases, and how to degrade gracefully
  when no external orchestrator or memory backend is available.
- **Command `/plan`** — capture an intent, derive a change-slug, resolve the
  artifact store, run the planning phases, and stop for human review and
  authorization before any implementation happens.
- **Command `/build`** — resolve an authorized plan, implement it, validate
  the result, and automatically close the change (change-folder close, plus
  an optional vault summary and tracker comment when those integrations are
  enabled).
- **Doc** — this README, materialized to
  `ai-specs/recipes/plan-build-flow/README.md`.

## Capability

- `plan-build-flow` — the two-verb plan/build workflow.

## Enable

In your project's `ai-specs.toml`:

```toml
[recipes.plan-build-flow]
enabled = true
version = "1.0.0"
```

Then run `ai-specs sync` to materialize the skill, commands, and docs.

## Config

None. The change-slug and artifact store are resolved per invocation, not
frozen at sync time — there is no per-project knob to set.

## Worktree coexistence

`/build` writes production code and defers to an isolated-worktree workflow
when one is enabled in the project, running inside that change's dedicated
worktree rather than re-implementing isolation. `plan-build-flow` still works
standalone when no worktree workflow is enabled — the deference is a soft
cross-reference, not a hard dependency.
