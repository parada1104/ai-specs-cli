---
name: openmemory-proactive
description: >
  Proactive semantic memory capture for project context, patterns, and decisions.
  Trigger: After solving non-trivial problems, discovering reusable patterns, making technical decisions,
  or encountering project-specific context that future sessions should remember.
  Complements vault-context by storing searchable facts during the session, not just at close.
license: MIT
metadata:
  author: parada1104
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "After solving a non-trivial bug or error"
    - "Discovering a reusable pattern or convention"
    - "Making a technical decision or tradeoff"
    - "Completing a significant implementation task"
    - "When encountering project-specific context that future sessions should know"
    - "Before ending a session"
---

# openmemory-proactive — Proactive Semantic Memory

This skill governs **when and how** to persist knowledge to OpenMemory during active sessions, making the agent behave more like `opencode-mem` — capturing context as it happens, not only at session boundaries.

## Two systems, two speeds

| | vault-context | openmemory-proactive (this skill) |
|---|---|---|
| **Role** | Canonical notetaker | Real-time memory sensor |
| **What** | Structured markdown files in Obsidian vault | Semantic / factual entries in vector+graph store |
| **When** | Session start, architecture decisions, session close | During the session, immediately after value is created |
| **Format** | Markdown with frontmatter | Plain text memory + optional temporal facts |
| **Search** | Filesystem MCP (scoped) | OpenMemory query (semantic + factual) |
| **Who writes** | Skill (on close/decision) | This skill (proactively during work) |

**Rule of thumb:** If it's a formal decision or handoff artifact, use `vault-context`. If it's a discovered pattern, solved bug, or piece of context that future sessions will need to search for, use OpenMemory **immediately**.

---

## What to Store

Store memories that are **searchable, reusable, and non-obvious**:

1. **Bug + Fix pairs** — Non-trivial errors and their resolution
2. **Discovered patterns** — Code conventions, architectural patterns, or workflows specific to this project
3. **Technical decisions** — Tradeoffs made and why (lightweight; formal ADRs still go to vault)
4. **Project context** — Business rules, constraints, or environment quirks that affect implementation
5. **Tooling gotchas** — Build issues, test quirks, dependency behavior
6. **Completed milestones** — What was done and how, for continuity across sessions

**Do NOT store:**
- Transient or trivial observations ("I ran `ls`")
- Information already well-documented in READMEs or specs
- Raw code snippets without context (link to files instead)
- Duplicate of vault-context decisions (cross-reference instead)

---

## When to Store — Proactive Triggers

Capture **immediately** after any of these moments:

### 1. Bug Resolution
After fixing a non-trivial bug, store:
- Symptom description
- Root cause
- Fix applied
- Files involved

### 2. Pattern Discovery
After identifying a reusable pattern:
- What the pattern is
- Where it applies
- Example location in codebase

### 3. Technical Decision
After making a tradeoff or choosing an approach:
- Options considered
- Decision made
- Rationale (brief)

### 4. Project Context Encountered
When you learn something about the project that future you will need:
- Business rule
- Environment constraint
- Integration quirk

### 5. Significant Task Completion
After finishing a substantial task:
- What was done
- Key files changed
- Approach taken

### 6. Session End
Before closing, do a quick sweep — anything important not yet captured? This complements `vault-context` session summary.

---

## How to Store

Use the OpenMemory MCP tools. Choose the right storage type:

### Contextual Memory (HSG semantic search)
For knowledge, patterns, and context:

```
openmemory_openmemory_store
  content: "Plain text description of the knowledge"
  tags: ["pattern", "bugfix", "decision", "context", "tooling"]
  type: "contextual"
```

### Factual Memory (Temporal graph)
For facts with subject-predicate-object structure:

```
openmemory_openmemory_store
  content: "Description"
  type: "factual" | "both"
  facts:
    - subject: "Module X"
      predicate: "depends_on"
      object: "Module Y"
```

### Project-Scoped Memory
Always scope project-specific memories:

```
openmemory_openmemory_store_project
  content: "..."
  project_id: "ai-specs-cli"
  tags: ["..."]
```

### Sector Tags
Use semantic sectors when relevant:
- `episodic` — Session events, what happened
- `semantic` — Knowledge, facts, patterns
- `procedural` — How-to, steps, workflows
- `reflective` — Decisions, tradeoffs, lessons learned

---

## Querying Before Acting

**Always query OpenMemory first** when:
- Starting work on a new task
- Encountering an error that feels familiar
- Making a decision that might have precedent

```
openmemory_openmemory_query
  query: "How do we handle X in this project?"
  type: "contextual" | "factual" | "unified"
  project_id: "ai-specs-cli"
```

---

## Critical Rules

1. **Capture during the session, not after.** The value of proactive memory is immediacy.
2. **Use plain text, not markdown.** OpenMemory stores semantic text, not structured files.
3. **Tag consistently.** Use tags like `pattern`, `bugfix`, `decision`, `context`, `tooling`.
4. **Scope to project.** Always use `project_id` when storing project-specific knowledge.
5. **Don't duplicate vault-context.** If it's a formal architecture decision, write to vault AND store a lightweight reference in OpenMemory for searchability.
6. **Query before deciding.** Search OpenMemory before making decisions that might have precedent.
7. **Reinforce important memories.** If a memory proved valuable, boost its salience with `openmemory_openmemory_reinforce`.

---

## MCP Access

The OpenMemory MCP is configured in `ai-specs.toml` under `[mcp.openmemory]`:
- Type: HTTP
- URL: `http://localhost:8080/mcp`

Tools available:
- `openmemory_openmemory_query` — Search memories
- `openmemory_openmemory_store` — Store global knowledge
- `openmemory_openmemory_store_project` — Store project-scoped knowledge
- `openmemory_openmemory_reinforce` — Boost salience
- `openmemory_openmemory_list` — Recent memories
- `openmemory_openmemory_get` — Single memory by ID
- `openmemory_openmemory_delete` — Remove memory
