---
name: plan-build-flow
description: >
  Ambient change workflow. On substantial requests, silently produce reviewable
  planning artifacts, stop for authorization, then implement, validate, and
  close without exposing slash commands. Degrade gracefully when no external
  orchestrator or memory backend is present.
license: MIT
metadata:
  author: ai-specs
  version: "2.0"
  scope: runtime
  auto_invoke:
    - "Starting a substantial feature, fix, or refactor that will modify code or artifacts"
    - "User describes a multi-step change that needs planning before implementation"
    - "Continuing implementation after the user authorizes a plan"
    - "Finishing a change: validate, archive artifacts pre-merge, and close out"
    - "Deciding how to run planning/build phases when no orchestrator or memory is available"
---

# Ambient Plan / Build Flow

This skill runs invisibly behind normal agent work. There are **no** `/plan` or
`/build` slash commands. When a request is substantial, plan first and stop for
authorization; when the user approves, implement, validate, and close in one
continuous flow.

## 1. What plan and build mean (internal)

| Phase | Contract |
|---|---|
| **Plan** | Turn an intent into reviewable planning artifacts. Stop and wait for human review/authorization. Never implement production code. |
| **Build** | Implement an authorized plan, validate the result, and close the change — including pre-merge archive per the bound VCS workflow. |

Speak to the user in plain language ("here is the plan", "implementing now") —
never expose internal phase names or slash verbs.

## 2. Phase mapping (private)

- **Plan** runs: explore → proposal → spec → design → tasks.
- **Build** runs: apply → verify → archive-tail (pre-merge on the review branch).

## 3. When to invoke

| Situation | Invoke plan? | Invoke build? |
|---|---|---|
| Quick question / read-only exploration | No | No |
| Substantial change request (new feature, refactor, multi-file fix) | Yes — plan and stop | No |
| User approves a pending plan ("go ahead", "implement it", "build it") | No | Yes |
| Trivial one-line fix explicitly scoped by user | Optional — use judgment | Maybe inline |

Prefer the project's native plan/review UX when available (e.g. plan mode) —
this skill supplies the artifact trail behind that surface.

## 4. Orchestrator degradation policy

- **Orchestrator present** — let it drive phases via sub-agents.
- **Orchestrator absent** — run equivalent phases inline in one conversation.

## 5. Memory degradation policy

- **Persistent memory present** — may persist cross-session facts.
- **Absent** — default to file artifacts under `openspec/changes/<slug>/`.

## 6. Change-slug derivation

- On plan, derive a kebab-case slug from intent.
- On build, resolve slug from outstanding change folders under
  `openspec/changes/` (excluding `archive/`).
- If multiple outstanding plans exist, ask which to build.

## 7. Worktree deference

- Plan artifacts may be written before a worktree exists when small.
- Build **must** use a dedicated worktree when `worktree-flow` is enabled.

## 8. Archive-tail graceful no-op

Archive-tail runs at the end of build, **before merge** when a VCS PR/MR flow is
in play:

- **Change-folder close** — move `openspec/changes/<slug>/` →
  `openspec/changes/archive/<slug>/` (required).
- **Vault summary** — no-op with note if canonical store absent.
- **Tracker comment** — no-op with note if tracker absent.
