---
name: session-bootstrap
description: >
  Session bootstrap protocol for ai-specs shaped projects.
  Trigger: At the start of every new session.
  Orchestrates a 3-level consensus check across OpenMemory (operational), Vault (canonical),
  and Trello (ground truth) to determine the current active card / change without assuming.
  Replicable: copy this skill to any ai-specs project and adjust the MCP names.
license: MIT
metadata:
  author: parada1104
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Starting a new session or conversation"
---

# session-bootstrap — 3-Level Session Start Protocol

This skill defines the **bootstrap ritual** for every new session in an ai-specs project.
It replaces blind assumptions with a structured 3-level consensus check.

## Design Philosophy

> **Never assume the next card.** Always contrast three sources.

| Level | Source | Question | Nature |
|-------|--------|----------|--------|
| **1** | **OpenMemory** | What did we do last session? What was agreed? | Operational, temporal, session-to-session |
| **2** | **Vault (handoff)** | What does the canonical close-out say? Is there an agreed execution order? | Canonical, deliberate, human-auditable |
| **3** | **Trello** | What cards are actually in what state? Are there inconsistencies? | Ground truth of work |

**Rule of thumb:** If all three converge → proceed with confidence. If they diverge → present analysis and ask.

---

## Protocol Steps

### Step 0 — Read Vault Context (Canonical Base)

Before any decision, invoke `vault-context` to read:
- `_context/README.md` (business context)
- Most recent `sessions/YYYY-MM-DD-handoff-*.md` (canonical close-out)

This gives the human-auditable baseline.

### Step 1 — Query OpenMemory (Operational Memory)

Query for the active state:

```
openmemory_openmemory_query
  query: "active epic next card last session agreed continuation"
  type: "unified"
  project_id: "<project-id>"
  k: 10
```

Look for facts like:
- `ai-specs-cli next-card-id = 54`
- `ai-specs-cli active-epic = "EPIC 3"`
- `session-YYYY-MM-DD next_phase = "apply"`

### Step 2 — Read Vault Handoff (Canonical Check)

Read the most recent handoff file in the vault `sessions/` directory.

Check for:
- **"Próximos pasos"** or **"Next steps"** section
- **"Orden de ejecución acordado"** or agreed execution order
- Any explicit note like *"se acordó continuar con card #35"*

If no handoff exists, note this as a gap — the previous session may not have closed properly.

### Step 3 — Verify with Trello (Ground Truth)

If Trello MCP is configured, verify the cards referenced by Level 1 and 2:

- Are the referenced cards actually in the expected list (Ready / In Progress / Done)?
- Are there cards marked as merged/archived that match the "last completed" work?
- Is there a card labeled or positioned as "current" / "next"?

**Note:** The agent does not need to know the board ID by heart. The board should be discoverable via `trello_get_active_board_info` or the user can provide it.

### Step 4 — Consensus Analysis

Compare the three sources:

| Scenario | Sources | Action |
|----------|---------|--------|
| **All converge** | Memory says #54, handoff says #54, Trello shows #54 in Ready | Proceed with #54 |
| **Memory vs Handoff diverge** | Memory says #54, handoff says #35 was agreed | **Ask user:** "Memory says #54, but handoff says we agreed on #35. Which one?" |
| **No handoff** | Memory exists, but no vault handoff | Proceed with memory but flag: *"No vault handoff found — proceeding based on OpenMemory only."* |
| **Trello diverges** | Memory/handoff say #54, but Trello shows #54 in Done | **Ask user:** "#54 appears done in Trello. Should we move to the next card?" |
| **No memory, no handoff** | Blank slate | **Ask user:** "No previous context found. What would you like to work on?" |

### Step 5 — Present and Confirm

Always present the conclusion as a question:

> *"Based on the 3-level check, the next card appears to be **#54 — Diseñar protocolo de capabilities y hooks** (EPIC 3). OpenMemory confirms this as the active card, the vault handoff lists it as the first step in Fase 1, and Trello shows it in Ready. Should we proceed with this card?"*

---

## Replicating This Skill

To use this in another ai-specs project:

1. **Copy** `ai-specs/skills/session-bootstrap/SKILL.md` to the new project's `ai-specs/skills/session-bootstrap/`
2. **Adjust** `project_id` references in the protocol text
3. **Ensure** the project has:
   - OpenMemory MCP configured in `ai-specs.toml`
   - Vault MCP configured in `ai-specs.toml`
   - Trello MCP (optional but recommended)
4. **Run** `ai-specs sync` to regenerate `AGENTS.md`
5. **Move** "Starting a new session or conversation" auto-invoke from `vault-context` to `session-bootstrap` (or keep both if desired)

---

## Relationship to Other Skills

| Skill | Role in Bootstrap |
|-------|-------------------|
| `vault-context` | Step 0 — provides canonical read at session start and write at close |
| `openmemory-proactive` | Captures context during sessions; this skill **reads** that context |
| `trello-pm-workflow` | Ground truth verification of card states |

---

## Critical Rules

1. **Never assume.** Even if OpenMemory is 100% clear, always check vault and Trello.
2. **Ask on divergence.** If sources disagree, present the conflict and let the user decide.
3. **Flag missing handoffs.** A missing vault handoff is a signal that the previous session didn't close properly.
4. **Be explicit.** State which source said what when presenting the conclusion.
5. **Scope queries.** Always use `project_id` when querying OpenMemory to avoid cross-project pollution.
