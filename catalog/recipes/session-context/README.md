# Session Context

Session-start context discipline for any ai-specs project. This recipe bundles three
skills so a session reliably starts from memory, resolves conflicts between context
sources, and records durable decisions in the canonical store.

## What it provides

### Skills (all bundled)

- **session-bootstrap** — concise session bootstrap: query Engram first, then read the
  runtime brief, and only cross-check tracker sources when gaps or contradictions remain.
- **vault-context** — project-scoped canonical context via a configured vault MCP for
  decisions, conventions, and handoffs.
- **context-precedence** — the canonical conflict-resolution policy used when project
  context sources (docs, skills, packs, handoffs, memory, proposed output) disagree.

### Capabilities

- `session-bootstrap` — start-of-session focus resolution.
- `canonical-memory` — deliberate, auditable record of decisions and handoffs.
- `conflict-policy` — auditable precedence rule for resolving source conflicts.

## How to enable

Add the recipe to your project manifest at `ai-specs/ai-specs.toml`:

```toml
[recipes.session-context]
enabled = true
version = "1.0.0"

# optional
[recipes.session-context.config]
vault_scope = "nnodes/proyectos/my-project"
```

Then run `ai-specs sync`. The three bundled skills materialize under
`ai-specs/.recipe/session-context/skills/` and this README is copied to
`ai-specs/recipes/session-context/README.md`.

## Config

| Key           | Required | Type   | Default | Description                                                  |
|---------------|----------|--------|---------|--------------------------------------------------------------|
| `vault_scope` | no       | string | (none)  | Optional vault scope hint for the bundled `vault-context` skill. |

## Maintenance note

The bundled `context-precedence` skill under
`catalog/recipes/session-context/skills/context-precedence/` is a **copy** that mirrors
`catalog/skills/context-precedence/`. This mirrors the existing dogfood-duplication note
in `catalog/README.md`: keep the two copies in sync when editing the precedence policy so
the recipe ships the same rule the repository dogfoods.
