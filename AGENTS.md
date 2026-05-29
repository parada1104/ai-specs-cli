# ai-specs-cli Runtime Brief

> This is the project's director de orquesta: canonical runtime context for agents.
> It covers project identity, MCPs, context sources, safety rules, and workflow conventions.
> It does NOT track day-to-day work state — that lives in Trello and Engram.

## Project

- **Project**: `ai-specs-cli`
- **Purpose**: per-project AI harness for agent configuration, MCPs, recipes, memory, and tracker integration.
- **Enabled runtimes**: `claude`, `cursor`, `opencode`, `pi`
- **Integration branch**: `development`
- **Vault scope**: `nnodes/proyectos/ai-specs`

## Runtime MCPs

**trello**
- command: npx
- args: -y @delorenj/mcp-server-trello
- env:
  - TRELLO_API_KEY: ${TRELLO_API_KEY}
  - TRELLO_TOKEN: ${TRELLO_TOKEN}
- description: project tracking through the ai-specs-cli Roadmap board.

**vault-ai-specs**
- command: npx
- args: -y @modelcontextprotocol/server-filesystem $OBSIDIAN_VAULT_PATH/nnodes/proyectos/ai-specs
- description: canonical project notes in the Obsidian vault scoped to `nnodes/proyectos/ai-specs`.

Never expose env-backed secrets from MCP config in generated docs or comments.

## Runtime Flow

- A session works on one explicit user request or Trello card.
- The orchestrator coordinates work inline using project skills and the runtime brief.
- `explore` can run without a worktree when it only produces thinking.
- Artifact phases and implementation phases run in a dedicated worktree when they write files.
- VCS/PR provider: github (`gh` CLI); base branch: `development`

## Trello Tracking

- **Board**: `69ec097f13e2d38ecd89a557` (`trello-mcp-workflow`).

## Context Sources

- Trello is the source of truth for work state and dependencies.
- Specs and changes are tracked in the project's designated spec store (configurable per project).
- Vault is the canonical note-taker for decisions, handoffs, and structured project context.
- Engram is the operational memory layer for session facts, patterns, and short-lived continuity.
- Skills are executable guidance, not the primary contents of this runtime brief. Load specific skills from `ai-specs/skills/<name>/SKILL.md` only when relevant.
- Check Engram for the current active card and next recommended focus.

## Conflict Policy

- Current explicit human instruction controls the immediate scope unless it conflicts with safety, secrets, or a higher-authority project rule.
- Trello controls work state; Vault controls canonical decisions and handoffs; repo docs and manifests control versioned project contracts.
- Skills define reusable procedures. Engram provides searchable operational context, not final authority.
- Proposed agent plans are lowest authority until accepted and recorded in Trello, Vault, docs, or code.

## Workflow Rules

- Do not merge or push to `development` without explicit human instruction.
- Create a dedicated worktree for changes that write artifacts or modify code. Pure exploration can happen before a worktree if it writes no files.
- Preserve unrelated worktree changes; never revert changes you did not make.
- Follow the project's designated workflow for structured changes.
- Before final verification, run the relevant focused tests plus `./tests/validate.sh` when feasible.
- Direct `skill-sync` runs are allowed only for metadata validation.

## Useful Commands

- Focused tests: `./tests/run.sh`
- Full validation: `./tests/validate.sh`
- Inspect the active Trello card before resuming work.