---
name: vault-context
description: >
  Project-scoped canonical context via a configured vault MCP. Trigger when
  recording decisions, closing sessions, or creating handoffs. Runtime memory
  handles operational capture; Vault stores the deliberate record.
license: MIT
metadata:
  author: parada1104
  version: "3.0"
  scope: [root]
  auto_invoke:
    - "Making an architecture or design decision"
    - "Closing or handing off a session"
---

# vault-context

Vault is the canonical notetaker. Use the vault MCP declared in the project runtime brief or manifest. Do not hardcode a path when the runtime brief provides one.

## Responsibilities

- Store decisions, conventions, and handoffs that future humans or agents must trust.
- Keep notes structured, dated, and human-auditable.
- Avoid duplicating transient runtime observations already captured by OpenMemory.

## Read Moments

- At bootstrap only when `session-bootstrap` needs canonical context or the user asks to resume previous work.
- Before changing a durable workflow or architecture decision.
- Before writing a new handoff that should supersede an older one.

## Write Moments

- After an architecture, workflow, or product decision becomes durable.
- At session close when there is meaningful state to hand off.
- When a blocker, dependency order, or next-step plan must be canonical.

## Standard Folders

Use the folders exposed by the configured vault scope:

- `_context/` for human-authored project context. Do not edit unless explicitly asked.
- `decisiones/` for decisions and conventions.
- `sessions/` for summaries and handoffs.
- `specs/` for canonical specs only when the project workflow allows it.

## Decision Note Shape

```markdown
---
tipo: decision
fecha: YYYY-MM-DD
estado: vigente
---

# <title>

## Contexto
<why this decision exists>

## Decisión
<what was decided>

## Razonamiento
<tradeoffs and alternatives>

## Consecuencias
<what changes now>
```

## Handoff Shape

```markdown
---
tipo: handoff
fecha: YYYY-MM-DD
sesion_id: <short-id>
---

# Handoff: <title>

## Estado actual
- <current card/change/session state>

## Qué se hizo
- <completed work>

## Decisiones vigentes
- <decisions that should guide the next session>

## Próximos pasos priorizados
1. <next action>
2. <next action>

## Riesgos o bloqueos
- <risk or blocker>
```

## Rules

- Never write outside the configured vault scope.
- Never store secrets or env-backed values.
- Do not edit `_context/` or `specs/` unless the user explicitly asks or the active SDD workflow owns that path.
- Use ISO dates in filenames and frontmatter.
- If it changes how future sessions should work, put it in Vault; if it is only searchable operational continuity, put it in OpenMemory.
