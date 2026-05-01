---
name: session-bootstrap
description: >
  Concise session bootstrap for ai-specs shaped projects. Trigger: at the start
  of a new session when the active user request, card, or change is unclear.
  Queries operational memory first, then reads the runtime brief, and only
  checks tracker sources when gaps or contradictions remain.
license: MIT
metadata:
  author: parada1104
  version: "2.1"
  scope: [root]
  auto_invoke:
    - "Starting a new session or conversation"
---

# session-bootstrap

Start every session from operational memory, then runtime brief, then tracker.

## Protocol

1. **If the user gave an explicit task:**
   a. **Check for referential ambiguity.** If the instruction uses language
      that refers to project state without naming it explicitly — e.g.,
      "siguiente card", "continuar", "la otra", "avanzar", "apply" without a
      change name, "verificar" or "archive" without context — treat the focus
      as unclear and proceed to step 2a to resolve the reference from
      OpenMemory before acting.
   b. **If the task is unambiguous** (names a specific card, change, file, or
      operation with no implicit state reference), use it as the session focus
      directly.

2. **If the focus is unclear (no explicit user task, or referential ambiguity
   detected in step 1a):**
   a. **Query OpenMemory first.** Search for:
      - `next_focus`, `next-card`, `active-card-id`, `active-change`,
        `next-session-focus`.
      - Recent handoffs or session-close entries.
      - Any project-scoped contextual memory from the last 48 h.
   b. **Read `AGENTS.md`** to extract runtime context: project id, integration
      branch, configured MCPs, current blockers, workflow rules, and conflict policy.
   c. **Cross-check Trello only if needed:**
      - OpenMemory has no recent entry for the active card.
      - OpenMemory names a card whose status is unknown or potentially stale.
      - A card/state decision is required (e.g., confirming a Ready → In Progress move).
   d. **Read the latest Vault handoff** when a vault MCP is configured and
      OpenMemory coverage is thin.

3. **If sources converge**, proceed and state the focus briefly.
4. **If sources diverge**, ask one concrete question that names the conflicting
   sources (e.g., *"OpenMemory says next card is #65, but Trello shows #65 in
   Backlog while #66 is in Ready. Which should I prioritize?"*).
5. **If a configured MCP is unavailable**, continue with available sources and
   state the gap.

## Referential Ambiguity Triggers

Treat an explicit user request as having unclear focus when it contains any of
these patterns (non-exhaustive):

- **Continuation:** "siguiente", "continuar", "seguuir", "la otra card",
  "avanzar", "proseguir", "vamos con la siguiente".
- **Implicit operations:** "apply", "verificar", "archive" without naming the
  target change.
- **Vague references:** "eso", "lo otro", "la que falta", "la pendiente",
  "la del backlog".
- **Status-relative:** "mover a Ready", "pasar a In Progress" without naming
  the card.

When any trigger is detected, **query OpenMemory for `next_focus` before
executing**. Do not assume the reference from Trello state or recent context
without confirming via operational memory first.

## Memory-First Rule

When no explicit user request exists, **OpenMemory is the first source to
consult**. Do not query Trello, Git, or Vault for session focus before checking
operational memory. This prevents redundant API calls and context bloat.

## Output Shape

Keep bootstrap output short:

```text
Focus: <card/change/request>
Evidence: <1-2 source references>
Next: <first action>
```

## Rules

- Do not assume the next card when no current user request exists.
- Do not load extra skills before the session focus is known.
- Scope OpenMemory queries by `project_id` when project-specific.
- Treat Trello as work-state truth, OpenSpec as SDD artifact truth, Vault as
  canonical handoff/decision truth, and OpenMemory as operational context.
- Prefer the current explicit user request over stale memory unless it conflicts
  with a higher-authority project rule.
- Stop after resolving focus; implementation planning belongs to the relevant
  workflow skill.
