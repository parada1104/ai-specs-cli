---
name: openspec-phase-orchestrator
description: Orchestrate OpenSpec changes using phase-specialized subagents for cleaner context windows. Use when working through a change and you want each SDD phase executed in isolation.
license: MIT
compatibility: Requires openspec CLI.
metadata:
  author: ai-specs
  version: "1.0"
  generatedBy: "1.0.0"
  scope: [root]
  auto_invoke:
    - "Orchestrating an OpenSpec change phase by phase"
    - "Running a specific SDD phase in isolation"
---

Orchestrate OpenSpec changes using phase-specialized subagents (or inline fallback) to keep context windows focused and clean.

**Input**: Optionally specify a change name and/or target phase. If omitted, infer from context or auto-select.

**Modes of Execution**

This skill operates in two modes, auto-selected by harness capability:

| Mode | Trigger | Behavior | Context Window |
|---|---|---|---|
| `subagent` | `task` tool is available | Delegates phase to a fresh subagent | **Reset per phase** |
| `inline` | `task` tool is NOT available | Executes phase directly in current agent | Shared (no reset) |

The `subagent` mode is preferred because it isolates the chain-of-thought, tool calls, and intermediate scratch work of each phase. The `inline` mode preserves the exact same flow and handoff contract, but without context isolation.

**Phase Definitions (spec-driven schema)**

| Phase | Artifact(s) Produced | Specialist Skill Loaded |
|---|---|---|
| `explore` | Thinking captured in conversation or `proposal.md` if crystallized | `openspec-explore` |
| `proposal` | `openspec/changes/<name>/proposal.md` | `openspec-new-change` (artifact creation) |
| `specs` | `openspec/changes/<name>/specs/<capability>/spec.md` | `openspec-continue-change` |
| `design` | `openspec/changes/<name>/design.md` | `openspec-continue-change` |
| `tasks` | `openspec/changes/<name>/tasks.md` | `openspec-continue-change` |
| `apply` | Code changes in repo + updated `tasks.md` checkboxes | `openspec-apply-change` |
| `verify` | `openspec/changes/<name>/verify-report.md` | `openspec-verify-change` |
| `archive` | Archived change in `openspec/changes/archive/` | `openspec-archive-change` |

**Steps**

1. **Select the change**

   If a name is provided, use it. Otherwise:
   - Infer from conversation context
   - Auto-select if only one active change exists
   - If ambiguous, run `openspec list --json` and prompt the user

   Announce: "Using change: <name> (phase: <detected-or-requested>)"

2. **Detect current phase**

   ```bash
   openspec status --change "<name>" --json
   ```

   Parse the JSON to determine:
   - `schemaName` (e.g., "spec-driven")
   - `artifacts` array with statuses: `done`, `ready`, `blocked`
   - `isComplete` boolean

   Map artifact statuses to phase:
   - If user requested a specific phase → use that phase
   - Else if `isComplete: true` → suggest `verify` or `archive`
   - Else if no `proposal.md` → `explore` or `proposal`
   - Else if proposal exists but specs incomplete → `specs`
   - Else if specs done but design missing → `design`
   - Else if design done but tasks missing → `tasks`
   - Else if tasks exist with pending checkboxes → `apply`
   - Else → `verify`

3. **Check for blockers**

   If the target phase is `blocked` (missing dependencies):
   - Show which artifacts are blocking it
   - Ask the user whether to jump to the missing dependency phase first

4. **Assemble phase payload**

   Build a self-contained prompt for the phase. Include ONLY:
   - The phase-specific system prompt (see templates below)
   - Minimal project context: `AGENTS.md` summary, tech stack from `openspec/config.yaml` context
   - Paths to dependency artifacts (already completed) — the executor will read them
   - The target output path(s)

   **DO NOT include** the full conversation history, previous phase chain-of-thought, or unrelated files.

5. **Execute the phase**

   **If in `subagent` mode:**
   ```
   Launch task(subagent_type="general", prompt=assembled_payload)
   ```

   **If in `inline` mode:**
   Load the relevant skill for this phase and execute it directly.

6. **Receive and parse handoff**

   The phase executor MUST return output starting with a structured YAML handoff block:

   ```yaml
   ---
   handoff:
     phase: "<phase-name>"
     status: "complete"      # complete | blocked | partial | error
     artifacts:
       - path: "openspec/changes/<name>/proposal.md"
         status: "created"   # created | updated | unchanged
     findings:
       - "Key finding 1"
       - "Key finding 2"
     blockers: null          # null or list of strings
     decisions:
       - "Decision made during this phase"
     next_phase_suggested: "<next-phase>"
     notes: "Optional narrative summary"
   ---
   ```

   Parse this block. If missing or malformed, attempt to extract information from the narrative text and warn.

7. **Present results to user**

   Display:
   - Phase completed (or blocked/partial)
   - Artifacts created/updated
   - Key findings and decisions
   - Any blockers
   - Suggested next phase

   Ask: "Continue to <next-phase-suggested>?" or "Choose a different phase?"

**Phase Prompt Templates**

These are injected as the system context for the phase executor (subagent or inline).

### explore
```
You are an EXPLORATION specialist.
Goal: Understand the problem space deeply before formalizing anything.
Constraints:
- Do NOT write code or implementation.
- Do NOT create final artifacts unless the user explicitly asks to capture findings.
- Read relevant codebase areas to ground your thinking.
- Surface risks, tradeoffs, and hidden complexity.
- If insights crystallize into requirements, summarize them for a future proposal.
Output: Structured handoff with findings and suggested proposal scope.
```

### proposal
```
You are a PROPOSAL specialist.
Goal: Create a clear, scoped proposal artifact.
Constraints:
- Read the exploration context (if exists) and AGENTS.md.
- Include: Why, What Changes, Capabilities, Impact, Rollback plan.
- Capabilities must be kebab-case; each will need a spec file.
- Output path: openspec/changes/<name>/proposal.md
Output: Complete proposal.md + handoff.
```

### specs
```
You are a SPECIFICATION specialist.
Goal: Write Given/When/Then specs for each capability in the proposal.
Constraints:
- Read proposal.md for capabilities list.
- Use RFC 2119 keywords (MUST, SHALL, SHOULD, MAY).
- Create one spec per capability at openspec/changes/<name>/specs/<capability>/spec.md
- Include acceptance criteria as scenarios.
Output: All spec files + handoff.
```

### design
```
You are a DESIGN specialist.
Goal: Document technical decisions and architecture.
Constraints:
- Read proposal and specs for constraints.
- Include sequence diagrams for complex flows (ASCII is fine).
- Document architecture decisions with rationale.
- Output path: openspec/changes/<name>/design.md
Output: Complete design.md + handoff.
```

### tasks
```
You are a TASK-PLANNING specialist.
Goal: Break implementation into small, checkboxed tasks.
Constraints:
- Read design.md for implementation approach.
- Group by phase (infrastructure, implementation, testing).
- Use hierarchical numbering (1.1, 1.2, etc.).
- Keep tasks small enough for one session.
- Output path: openspec/changes/<name>/tasks.md
Output: Complete tasks.md + handoff.
```

### apply
```
You are an IMPLEMENTATION specialist.
Goal: Execute pending tasks from tasks.md.
Constraints:
- Read tasks.md, design.md, and relevant specs.
- Follow existing code patterns and conventions.
- Use ./tests/run.sh for focused TDD feedback.
- Update task checkboxes as you complete them.
- Prefer commits grouped by task phase.
- Keep changes minimal and focused per task.
- If a task is unclear, stop and ask.
Output: Code changes + updated tasks.md + handoff with RED/GREEN evidence if TDD applies.
```

### verify
```
You are a VERIFICATION specialist.
Goal: Validate implementation against specs and record evidence.
Constraints:
- Read all spec scenarios and compare against implementation.
- Run ./tests/validate.sh.
- State unavailable quality signals explicitly (coverage, linter, type-checker, formatter).
- Update or create verify-report.md.
- Output path: openspec/changes/<name>/verify-report.md
Output: verify-report.md + handoff with PASS/FAIL verdict.
```

### archive
```
You are an ARCHIVE specialist.
Goal: Close the change and move it to the archive.
Constraints:
- Verify implementation is merged or ready for merge.
- Run openspec-archive-change or equivalent steps.
- Ensure delta specs are synced to main specs if required.
- Output: Archived change folder + handoff.
```

**Output Format**

When orchestrating:

```
## Phase Orchestrator: <change-name>

Schema: <schema-name>
Mode: <subagent|inline>
Phase: <phase-name>

### Context Injected
- AGENTS.md (summary)
- Dependency artifacts: <list>
- Target outputs: <list>

### Executing...
[subagent runs or inline execution]

### Handoff Received
Status: <status>
Artifacts:
- <path> (<status>)
Findings:
- <finding 1>
Next suggested: <next-phase>

Continue to <next-phase>? Or choose another action?
```

**Guardrails**

- Always announce the mode (`subagent` vs `inline`) so the user knows if context is isolated.
- Never skip dependency detection. If a phase is blocked, do not blindly execute it.
- Keep phase payloads minimal. The executor should read dependency files itself; do not inline their contents into the prompt unless they are very small (<500 tokens).
- If a phase executor returns `status: error`, stop and present the error. Do not auto-advance.
- If `apply` phase is selected and there are no pending tasks, suggest `verify` instead.
- The orchestrator never writes code or artifacts directly in `subagent` mode — it only delegates and interprets handoffs.

**Cross-Harness Replicability**

This skill is designed to be harness-agnostic:

- The flow (detect → assemble → execute → handoff → transition) is identical in any harness.
- The `inline` mode works in **every** harness because it uses the same skills already distributed by `ai-specs sync-agent`.
- The `subagent` mode requires the harness to support subagent/task delegation. If unavailable, the skill degrades gracefully to `inline` with zero behavioral change in the flow.

| Harness | Subagent Support | Notes |
|---|---|---|
| OpenCode | Yes (`task` tool) | Full context isolation per phase |
| Claude Code | Partial (`//` subagent mode) | May need adapter; inline fallback always works |
| Cursor | No native subagent | Always uses `inline` mode |
| GitHub Copilot / Codex | No native subagent | Always uses `inline` mode |

To replicate this flow in a new harness:
1. Ensure `openspec` CLI is available.
2. Ensure the phase skills (`openspec-explore`, `openspec-apply-change`, etc.) are present (via `ai-specs sync-agent`).
3. Implement the orchestrator skill using the exact same handoff schema and prompt templates above.
4. If the harness supports subagents, map the `task` launch to the native equivalent; otherwise, use `inline`.

**Related**

- `openspec-explore` — exploration stance
- `openspec-new-change` — change creation
- `openspec-continue-change` — artifact creation
- `openspec-apply-change` — implementation
- `openspec-verify-change` — verification
- `openspec-archive-change` — archival
