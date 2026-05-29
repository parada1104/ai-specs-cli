# Session Context

Foundational, **tool-agnostic** session-start discipline for any ai-specs
project. It bundles the bootstrap and conflict-resolution patterns and speaks in
terms of capabilities — never specific vendors. See
[`docs/capabilities.md`](../../../docs/capabilities.md).

## What it provides

### Skills (bundled)

- **session-bootstrap** — concise session bootstrap: resolve focus from the
  `memory` capability first, then the runtime brief, and only cross-check the
  `tracker` capability when gaps or contradictions remain.
- **context-precedence** — the canonical conflict-resolution policy used when
  project context sources (docs, skills, packs, handoffs, memory, proposed
  output) disagree.

### Capabilities provided

- `session-bootstrap` — start-of-session focus resolution.
- `conflict-policy` — auditable precedence rule for resolving source conflicts.

### Capabilities consumed (by convention)

This recipe refers to — but does not implement — these capabilities. Bind a
concrete provider for each in your manifest:

- `memory` — operational/session memory (e.g. the gentle-ai stack).
- `tracker` — work-state tracking (e.g. `trello-mcp-workflow`).
- `canonical-store` — durable decisions/handoffs (e.g. `vault-canonical-store`).

## How to enable

```toml
[recipes.session-context]
enabled = true
version = "2.0.0"
```

Then run `ai-specs sync`. The bundled skills materialize under
`ai-specs/.recipe/session-context/skills/` and this README is copied to
`ai-specs/recipes/session-context/README.md`.

To get a canonical store, enable a provider too, e.g.:

```toml
[recipes.vault-canonical-store]
enabled = true
version = "1.0.0"
```

## Maintenance note

The bundled `context-precedence` skill mirrors `catalog/skills/context-precedence/`
(same dogfood-duplication pattern noted in `catalog/README.md`); keep the two in
sync when editing the precedence policy.

> The `vault-context` skill moved to its own recipe **`vault-canonical-store`**
> (it provides `canonical-store`), so the canonical-store implementation can be
> swapped without touching this foundational recipe.
