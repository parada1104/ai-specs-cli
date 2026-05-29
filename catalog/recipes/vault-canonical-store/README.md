# Vault Canonical Store recipe

Provides the **`canonical-store`** capability: durable decisions and handoffs in
a configured vault MCP (e.g. Obsidian over a filesystem MCP server). This is the
concrete (tier-2) provider that foundational recipes such as `session-context`
consume when they need a canonical store.

## What it provides

- **Skill `vault-context`** — when and how to read/write canonical decisions and
  handoffs, note shapes, and the rule of "Vault for durable record, operational
  memory for searchable continuity".
- **Capability `canonical-store`** — so a project can bind it wherever a
  canonical store is needed.

## Enable

```toml
[recipes.vault-canonical-store]
enabled = true
version = "1.0.0"

[recipes.vault-canonical-store.config]
decisions_folder = "decisiones"
sessions_folder = "sessions"
```

Configure the vault MCP itself under `[mcp.<name>]` in the manifest; this recipe
does not declare the server (env and paths belong to the MCP config).

## Config

| Key | Default | Meaning |
|---|---|---|
| `vault_scope` | — | Optional hint for the vault scope/path the agent should stay within. |
| `decisions_folder` | `decisiones` | Folder for decisions and conventions. |
| `sessions_folder` | `sessions` | Folder for session summaries and handoffs. |

## Capability

| Capability | Meaning |
|---|---|
| `canonical-store` | Durable decisions/handoffs store. |

Other providers (Notion, a git-tracked docs folder, etc.) can be future sibling
recipes that also provide `canonical-store`; a project picks one via
`[[bindings]]`.
