---
name: vault-context
description: >
  Project-scoped canonical context via Obsidian vault — the structured record of what matters.
  Trigger: At session start (read context) and at session close (write decisions, summaries, handoffs).
  Acts as the canonical note-taker; runtime memory handles automatic capture during sessions.
license: MIT
metadata:
  author: parada1104
  version: "2.0"
  scope: [root]
  auto_invoke:
    - "Starting a new session or conversation"
    - "Making an architecture or design decision"
    - "Closing or handing off a session"
---

# vault-context — Canonical Project Notes via Obsidian Vault

This skill governs how AI agents write structured, canonical notes to the project's Obsidian vault, accessed through the `vault-*` filesystem MCP server configured in `ai-specs.toml`.

## Two systems, two jobs

| | runtime memory | vault (this skill) |
|---|---|---|
| **Role** | Automatic sensor | Canonical notetaker |
| **What** | Captures everything during the session | Stores what matters structurally after the session |
| **When** | Real-time | At session close or on decisions |
| **Scope** | Global / runtime-specific | This project's vault only |
| **Format** | Runtime-specific memory | Structured Markdown with frontmatter |
| **Search** | runtime memory search | Filesystem MCP (scoped) |
| **Who writes** | Automatic | This skill (on close/decision) |

**Runtime memory catches what happens. vault stores what matters.** Don't duplicate — if runtime memory already caught it as an observation, only write to vault if it's a decision, convention, or something structural.

---

## Vault Structure

The vault folder exposed via MCP contains:

```
{vault-scope}/
├── _context/
│   └── README.md            # Business context — READ FIRST every session
├── decisiones/              # Architecture decisions, observations, patterns
├── sessions/                # Session summaries and handoff packs
└── specs/                   # Specs and technical documentation (read-only)
```

---

## Auto-Invoke Rules

### 1. Session Start — Read Context

**When:** At the beginning of every new session, before writing any code.

**Action:**
1. Read `_context/README.md` via the vault filesystem MCP
2. If continuing previous work, read the latest file in `sessions/`

This ensures you never start blind. The context file contains business rules, constraints, and project-specific knowledge that overrides generic assumptions.

### 2. Architecture Decision — Write Decision

**When:** After an architecture decision, design choice, or convention is established during a session.

**Action:** Create a file in `decisiones/` using this format:

```markdown
---
tipo: decision
fecha: YYYY-MM-DD
estado: vigente | deprecada | reemplazada
reemplaza: <!-- ID of decision this replaces, if any -->
---

# {Short title}

## Contexto
{Why this decision was needed}

## Decisión
{What was decided}

## Razonamiento
{Why — tradeoffs considered, alternatives rejected}

## Consecuencias
{What changes as a result}
```

Filename: `decisiones/YYYY-MM-DD-{slug}.md`

### 3. Session Close — Write Summary

**When:** Before ending a session or when handing off work. This is the main write moment — treat it like closing a notebook.

**Action:** Create a file in `sessions/` using this format:

```markdown
---
tipo: session-summary | handoff
fecha: YYYY-MM-DD
sesion_id: <!-- optional, for linking -->
---

# Session: {Short description}

## Qué se hizo
- {Bullet list of completed work}

## Qué falta
- {Bullet list of remaining work}

## Archivos relevantes
- `path/to/file` — {what it does}

## Próximos pasos
1. {First thing next session should tackle}
2. {Second thing}

## Riesgos o bloqueos
- {Anything that might cause issues}
```

Filename: `sessions/YYYY-MM-DD-{slug}.md`

### 4. Handoff — Write Handoff Pack

**When:** When someone else (human or agent) needs to continue this work.

**Action:** Create a handoff file in `sessions/` using the session-close format with `tipo: handoff`, plus:

- **Estado actual**: What's done and what's in progress
- **Decisiones vigentes**: Link to files in `decisiones/`
- **Próximos pasos priorizados**: Ordered by urgency
- **Contexto mínimo**: What the next person/agent needs to know to be productive immediately

A handoff pack is the **checkpoint** — it's what lets someone start productive immediately without reading the full history.

---

## What Goes Where

| Type | runtime memory only | vault only | both |
|------|---------------------|------------|------|
| Bug I just found | ✅ | | |
| Quick observation during work | ✅ | | |
| Architecture decision | | ✅ | |
| Convention or pattern established | | ✅ | |
| Project business context | | ✅ | |
| Session summary at close | | ✅ | |
| Handoff pack | | ✅ | |
| Non-obvious gotcha that will matter again | | | ✅ |

Rule of thumb: **if it changes how future sessions should work, it goes in the vault.** If it's a transient observation, runtime memory handles it.

---

## Critical Rules

1. **NEVER write outside your vault scope.** You only see `{vault-scope}/` — stay inside it.
2. **NEVER edit `_context/README.md`** unless explicitly asked by the project owner. Context is source of truth written by humans.
3. **NEVER edit files in `specs/`** — that's the domain of the spec-driven development flow.
4. **ALWAYS use frontmatter** in every file you create. It's how we keep the vault queryable.
5. **ALWAYS use ISO dates** (YYYY-MM-DD) in filenames and frontmatter.
6. **ALWAYS use wikilinks** `[[like-this]]` to reference other vault notes when relevant.
7. **DON'T duplicate runtime memory.** If runtime memory already captured it, only write to vault if it's structural.
8. **Primary write moment is session close.** Don't write every observation during the session — memory handles that. Write at close to create the canonical record.

---

## MCP Access

The vault filesystem MCP is configured in `ai-specs.toml` under `[mcp.vault-*]`. You access it through the filesystem tools provided by the agent (e.g., `read_file`, `write_file`, `search_files`).

**For ai-specs specifically:**
- MCP name: `vault-ai-specs`
- Scope: `/Users/robert/Library/Mobile Documents/iCloud~md~obsidian/Documents/hermes-vault/nnodes/proyectos/ai-specs/`

---

## Resources

- **Project context**: Read `_context/README.md` first
- **Decision log**: `decisiones/` directory
- **Session history**: `sessions/` directory
- **Specs**: `specs/` directory (read-only unless using SDD flow)
