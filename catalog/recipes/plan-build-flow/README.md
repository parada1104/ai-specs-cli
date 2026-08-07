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

| Tier | Typical use | Planning chain | Minimum artifacts before build |
|------|-------------|----------------|-------------------------------|
| **Full** | New capability, architecture, breaking or ambiguous scope | explore → proposal → spec → design → tasks | `tasks.md` plus `proposal.md` or `design.md`, and at least one `specs/**/*.md` |
| **Standard** | Scoped feature or bounded multi-file work | conditional explore → proposal → spec → tasks | `proposal.md`, `tasks.md`, and at least one `specs/**/*.md` |
| **Light** | Small fix with clear file/symbol target | proposal → tasks | `proposal.md` + `tasks.md` |

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

- **No PR** until the change folder contains its tier minimum files committed:
  Light requires `proposal.md` + `tasks.md`; Standard requires those plus a
  spec delta; Full requires `tasks.md`, `proposal.md` or `design.md`, and a
  spec delta.
- **Verify before archive:** Standard requires a dedicated `verify-report.md`
  with command, exit `0`, a valid calendar date, commit SHA, and a non-failing
  verdict. Full additionally requires strict `PASS`, `ready_for_archive: true`,
  and exactly one strict-PASS `Criterion N` mapping row for every top-level
  bullet under `## Success Criteria` in the authoritative source: `proposal.md`
  when present, otherwise `design.md`. A missing, empty, or duplicate heading
  in the authoritative source blocks Full; it never falls back to `design.md`.
  Light is advisory only.
- **Two enforcement points:** run the verify gate before archive-tail and run
  the pre-merge guardian again after archive. Missing `explore.md` is never a
  guardian blocker.
- **Archive before merge** on the review branch — never after merge lands on the
  base branch.

Plans already in flight when this contract ships add missing `proposal.md` or
verify evidence before their PR/archive; no replan or restart is needed.
Historical archives are never rewritten. A stale PR is handled by its owning
agent, which adds the missing artifacts when that change resumes.

## Capability

- `plan-build-flow` — ambient plan/build workflow (skill-only surface).

## Enable

```toml
[recipes.plan-build-flow]
enabled = true
version = "1.6.0"
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
