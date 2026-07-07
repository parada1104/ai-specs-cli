---
name: plan-build-flow
description: >
  Two-verb change workflow. Map `/plan` to producing reviewable planning
  artifacts and `/build` to implementing, validating, and closing an authorized
  plan. Degrade gracefully when no external orchestrator or memory backend is
  present.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Planning a change with /plan before implementation"
    - "Building an authorized plan with /build"
    - "Deciding how to run plan/build when no orchestrator or memory is available"
---

# Plan / Build Flow

Two verbs cover the whole change lifecycle: `/plan` produces a reviewable plan
and stops; `/build` implements an authorized plan, validates it, and closes
the change automatically. There is no third visible command.

## 1. What plan and build mean

| Verb | Contract |
|---|---|
| `/plan` | Turn an intent into reviewable planning artifacts. Stop and wait for human review/authorization. Never implement. |
| `/build` | Implement an already-authorized plan, validate the result, and close the change out — all in one invocation. |

## 2. Phase mapping (internal detail)

This mapping is private to this skill. It describes the underlying phases an
agent runs to fulfill each verb, but the phase names below MUST NOT leak into
the generated brief (`[provides.brief]`) or `README.md`, which speak only
"plan" and "build".

- `/plan` runs: explore → proposal → spec → design → tasks.
- `/build` runs: apply → verify → archive-tail.
  - The archive-tail is the automatic closing step at the *tail* of `/build`
    (change-folder close + optional vault summary + optional tracker comment).
    It is not a separate, visible third verb — see Section 8.

## 3. Orchestrator degradation policy

- **Orchestrator present** (e.g. a gentle-ai-style multi-agent orchestrator) —
  let it drive the phases as it normally would, delegating to sub-agents.
- **Orchestrator absent** — run the equivalent phases inline, in order, as ONE
  continuous conversation, without pausing for sub-agent handoffs. Never fail
  and never silently skip a phase because no orchestrator is available; produce
  the same artifacts inline that the orchestrated path would have produced.

## 4. Memory degradation policy

- **Persistent memory present** (e.g. an Engram-style memory backend) — may be
  used to persist artifacts for cross-session recovery.
- **Persistent memory absent** — fall back to file artifacts on disk (see
  Section 5). If the user explicitly wants no files at all, run inline-only
  (`none`) but say so explicitly before proceeding.

## 5. Artifact-store default policy

- If an orchestrator preflight already resolved an artifact store for this
  session, honor that choice.
- Otherwise, default to file artifacts written under
  `openspec/changes/<slug>/`, with `proposal.md`, `specs/<capability>/spec.md`,
  `design.md`, and `tasks.md` as the concrete on-disk layout. Files are the
  reviewable deliverable this workflow centers on; do not silently default to
  memory-only persistence. This ceremony vocabulary stays internal to this
  skill body — the agent still speaks only "plan" and "build" to the user.

## 6. Change-slug derivation

- On `/plan`, derive a short kebab-case slug from the user's intent (e.g. "add
  rate limiting to the login endpoint" → `rate-limit-login`).
- Persist or record the slug alongside the resolved artifact store so that a
  later `/build [change]` resolves the exact same slug and store `/plan` used.
- If `/build` is invoked with no argument:
  - If no plan is outstanding, stop and direct the user to run `/plan` first
    instead of guessing a target.
  - If exactly one plan is outstanding (planned but not yet built), resolve to
    it automatically.
  - If more than one plan is outstanding, ask the user which one to build
    rather than guessing.
- Degraded-mode persistence: when neither an orchestrator nor a persistent
  memory backend is available, do not rely on unstated persisted state for the
  slug↔store mapping. Instead, the mapping must be inferable directly from the
  file artifacts themselves — `/build` scans `openspec/changes/*/` for
  outstanding (not-yet-built) plans to resolve the change-slug and store.

## 7. Worktree deference

- `/plan` does not require a dedicated worktree — planning artifacts are
  review-first and typically small. Where an isolated-worktree workflow is
  enabled in the project, its own gate still governs any writes `/plan` makes.
- `/build` writes production code and MUST run inside a dedicated worktree
  when an isolated-worktree workflow is enabled. Defer to that workflow's own
  conventions for creating and naming the worktree rather than re-implementing
  isolation here. This is a deference, not a hard dependency: `/build` still
  runs standalone (in the current working tree) when no worktree workflow is
  enabled.

## 8. Archive-tail graceful no-op

The automatic closing step at the tail of `/build` never fails solely because
an optional output channel is unavailable:

- **Change-folder close** always completes. This is the one step that must
  succeed for `/build` to be considered done.
- **Vault/canonical-store summary** — write a durable summary note if a
  canonical-store integration is enabled; otherwise no-op with an informative
  note that this output was skipped (do not fail `/build`).
- **Tracker comment** — post a progress comment if a tracker integration is
  enabled; otherwise no-op with an informative note that this output was
  skipped (do not fail `/build`).
- Report to the user what was built, validated, and closed, and which optional
  outputs (if any) were skipped and why.
