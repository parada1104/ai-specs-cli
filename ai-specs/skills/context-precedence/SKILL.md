---
name: context-precedence
description: >
  Optional conflict-resolution reference for ai-specs projects. Trigger only
  when runtime brief, tracker, SDD artifacts, vault, memory, or skill guidance
  disagree and the active AGENTS.md conflict policy is insufficient.
license: MIT
metadata:
  author: parada1104
  version: "2.0"
  scope: [root]
  auto_invoke:
    - "Explicitly resolving a context conflict after AGENTS.md policy is insufficient"
---

# Context Precedence

Use this skill only for unresolved conflicts. Normal sessions should rely on the
project runtime brief in `AGENTS.md`.

## Default Order

1. Safety, secrets, and explicit system/developer constraints.
2. Current explicit human instruction for immediate scope.
3. Project runtime brief (`AGENTS.md`) and versioned repo contracts.
4. Tracker state for work status and dependencies.
5. SDD artifacts for proposal/spec/design/tasks/apply/verify truth.
6. Vault decisions and handoffs for canonical project memory.
7. Project-local skills for reusable procedures.
8. OpenMemory for operational context and searchable session facts.
9. Agent plans or proposed context not yet accepted.

## Resolution Rules

- Prefer the narrowest source that owns the disputed fact.
- Do not let stale memory override a current card, artifact, or human instruction.
- If two high-authority sources disagree, present the conflict and ask.
- Record durable resolutions in the owning canonical place: Trello for state, OpenSpec for SDD artifacts, Vault for decisions/handoffs, docs/code for versioned contracts.

## Boundary

This is a decision aid, not a runtime merge engine and not an auto-invoked startup skill.
