---
name: session-bootstrap
description: >
  Concise session bootstrap for ai-specs shaped projects. Trigger: at the start
  of a new session when the active user request, card, or change is unclear.
  Reads the runtime brief first, then checks configured memory/tracker sources
  only as needed to resolve the session focus.
license: MIT
metadata:
  author: parada1104
  version: "2.0"
  scope: [root]
  auto_invoke:
    - "Starting a new session or conversation"
---

# session-bootstrap

Start every session from the project runtime brief, not from skill lore.

## Protocol

1. Read `AGENTS.md` first. Extract the project id, integration branch, configured MCPs, current blockers, workflow rules, and conflict policy.
2. If the user gave an explicit task, use that as the session focus. Do not run a full consensus ritual unless the task conflicts with `AGENTS.md`, Trello state, an OpenSpec artifact, or a safety rule.
3. If the focus is unclear, check only the configured sources:
- Query OpenMemory for recent project-scoped active card/change/session facts.
- Read the latest Vault handoff when a vault MCP is configured.
- Check Trello when a tracker MCP is configured and a card/state decision is needed.
4. If sources converge, proceed and state the focus briefly.
5. If sources diverge, ask one concrete question that names the conflicting sources.
6. If a configured MCP is unavailable, continue with available sources and state the gap.

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
- Treat Trello as work-state truth, OpenSpec as SDD artifact truth, Vault as canonical handoff/decision truth, and OpenMemory as operational context.
- Prefer the current explicit user request over stale memory unless it conflicts with a higher-authority project rule.
- Stop after resolving focus; implementation planning belongs to the relevant workflow skill.
