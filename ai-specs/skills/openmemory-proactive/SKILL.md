---
name: openmemory-proactive
description: >
  Selective operational memory capture for project context, patterns, decisions,
  and handoff continuity. Trigger after non-trivial discoveries, decisions,
  resolved blockers, significant progress, or session close. Complements Vault;
  it does not replace canonical notes.
license: MIT
metadata:
  author: parada1104
  version: "2.0"
  scope: [root]
  auto_invoke:
    - "After solving a non-trivial bug or error"
    - "Discovering a reusable pattern or convention"
    - "Making a technical decision or tradeoff"
    - "Completing a significant implementation task"
    - "When encountering project-specific context that future sessions should know"
    - "Before ending a session"
---

# openmemory-proactive

OpenMemory is the operational memory layer: fast, searchable, and useful during future sessions. Vault remains the canonical record for decisions and handoffs.

## Store When It Will Help Future Work

Store a memory when it is reusable, non-obvious, and likely to be searched later:

- Card/change status that affects the next session.
- A resolved blocker with symptom, cause, and fix.
- A project-specific pattern or convention discovered in code or workflow.
- A technical tradeoff that influenced implementation or planning.
- A tooling gotcha, environment constraint, or dependency behavior.
- A compact session milestone when it improves continuity.

Do not store raw logs, command transcripts, trivial observations, secrets, obvious file listings, or content already captured canonically in OpenSpec/Vault/docs unless storing a short searchable pointer.

## Project vs Global

- Use project-scoped storage for project-specific knowledge.
- Use global storage only for general knowledge that applies across projects.
- If unsure whether a memory is project-specific or global, ask the user before storing globally.

## Relationship To Vault

Promote to Vault when the information is canonical:

- Architecture or workflow decision.
- Handoff or session close-out.
- Convention that future agents must follow.
- Structured project context that should be human-auditable.

When both are useful, write the canonical note to Vault and store a short OpenMemory pointer with tags like `decision`, `handoff`, or `workflow`.

## Query Before Acting

Query OpenMemory when a task might have precedent:

- Starting or resuming a card/change.
- Seeing a familiar error.
- Making a workflow or architecture decision.
- Reconstructing what happened last session.

Use `project_id` for project work and prefer `type: "unified"` when both semantic memories and factual state may matter.

## Storage Shape

Good memories are short and self-contained:

```text
<date/context>: <what changed or was learned>. <why it matters>. Relevant refs: <card/change/path>.
```

Recommended tags: `card`, `change`, `decision`, `handoff`, `pattern`, `bugfix`, `tooling`, `workflow`, `risk`, `tests`.

Use factual memory sparingly for durable relationships such as `card-62 depends_on card-65` or `project uses sdd_provider OpenSpec`.

## Rules

- Store after value is created, not at random checkpoints.
- Prefer one useful memory over many noisy memories.
- Never store secrets or env-backed values.
- Do not use OpenMemory to override Trello state, OpenSpec artifacts, Vault decisions, or the runtime brief.
- Reinforce memories that proved useful; avoid duplicating them.
