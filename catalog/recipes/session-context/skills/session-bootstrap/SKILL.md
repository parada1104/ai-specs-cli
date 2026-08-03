---
name: session-bootstrap
description: >
  Concise session bootstrap. Trigger: at the start of a new session when the
  active user request, card, or change is unclear. Resolves focus from the
  memory capability first, then the runtime brief, and only checks the tracker
  capability when gaps or contradictions remain.
license: MIT
metadata:
  author: ai-specs
  version: "4.0"
  scope: runtime
  auto_invoke:
    - "Starting a new session or conversation"
---

# session-bootstrap

Start every session from the **memory** capability, then the runtime brief, then
the **tracker** capability. This skill is tool-agnostic: it refers to
capabilities, not specific products. Concrete providers are bound in the
project's manifest (see `docs/capabilities.md`).

## Protocol

1. **If the user gave an explicit task:**
   a. **Check for referential ambiguity.** If the instruction refers to project
      state without naming it — e.g. "siguiente card", "continuar", "la otra",
      "avanzar", "apply" without a change name, "verificar"/"archive" without
      context — treat the focus as unclear and go to step 2a to resolve the
      reference from the memory capability before acting.
   b. **If the task is unambiguous** (names a specific card, change, file, or
      operation), use it as the session focus directly.

2. **If the focus is unclear:**
   a. **Query the memory capability first.** Get recent session history, then
      search for: `next_focus`, active card/change references, recent handoffs
      or session-close entries, and project-scoped context from the last ~48 h.
   b. **Read `AGENTS.md`** for runtime context: project id, integration branch,
      configured MCPs/capabilities, blockers, workflow rules, conflict policy.
   c. **Consult the tracker capability** — when a tracker capability is
      bound in the manifest, this is **mandatory** for new or ambiguous changes:
      resolve/confirm the active card before proceeding, and ensure a new
      structured change has a linked card recorded in the `## Tracker` section of
      its proposal.md (or tasks.md). When no tracker capability is bound,
      cross-check only if memory is missing/stale. If the tracker is unavailable,
      degrade (warn + continue) without blocking.
   d. **Read the latest handoff from the canonical-store capability** when one is
      configured and memory coverage is thin.

3. **If sources converge**, proceed and state the focus briefly.
4. **If sources diverge**, ask one concrete question that names the conflicting
   sources (e.g. *"memory says next card is #65, but the tracker shows #65 in
   Backlog while #66 is in Ready. Which should I prioritize?"*).
5. **If a configured capability/MCP is unavailable**, continue with available
   sources and state the gap.

## Referential Ambiguity Triggers

Treat an explicit request as unclear focus when it contains (non-exhaustive):

- **Continuation:** "siguiente", "continuar", "la otra card", "avanzar",
  "vamos con la siguiente".
- **Implicit operations:** "apply", "verificar", "archive" without naming the
  target change.
- **Vague references:** "eso", "lo otro", "la que falta", "la pendiente".
- **Status-relative:** "mover a Ready", "pasar a In Progress" without naming the
  card.

When any trigger fires, **query the memory capability for `next_focus` before
executing**. Do not infer the reference from tracker state without confirming
against operational memory first.

## Memory-First Rule

When no explicit user request exists, **the memory capability is the first
source to consult**. Do not query the tracker, VCS, or canonical store for
session focus before checking operational memory. This avoids redundant calls
and context bloat.

## Output Shape

```text
Focus: <card/change/request>
Evidence: <1-2 source references>
Next: <first action>
```

## Rules

- Do not assume the next card when no current user request exists.
- Do not load extra skills before the session focus is known.
- Scope memory searches by project when project-specific.
- Treat the **tracker** as work-state truth, **SDD artifacts** as spec truth,
  the **canonical store** as durable handoff/decision truth, and **memory** as
  operational context.
- Prefer the current explicit user request over stale memory unless it conflicts
  with a higher-authority project rule.
- Stop after resolving focus; implementation planning belongs to the relevant
  workflow skill.
