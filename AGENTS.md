# ai-specs-cli Runtime Brief

> This is the project's director de orquesta: canonical runtime context for agents.
> It covers project identity, MCPs, context sources, safety rules, and workflow conventions.
> It does NOT track day-to-day work state — that lives in Trello and Engram.

## Project

- **Project**: `ai-specs-cli`
- **Purpose**: per-project AI harness for agent configuration, MCPs, recipes, memory, and tracker integration.
- **Enabled runtimes**: `claude`, `cursor`, `opencode`, `pi`, `omp`
- **Integration branch**: `development`
- **Vault scope**: `nnodes/proyectos/ai-specs`

## Runtime MCPs

**trello** *(global)*
- description: project tracking through the ai-specs-cli Roadmap board.

**vault-canonical** *(global)*
- description: canonical project notes in the vault scoped to `nnodes/proyectos/ai-specs` (path from $CANONICAL_VAULT_PATH).

**engram** *(global)*
- description: operational/session memory (global MCP).

Never expose env-backed secrets from MCP config in generated docs or comments.

## Runtime Flow

- A session works on one explicit user request or Trello card.
- The orchestrator coordinates work inline using project skills and the runtime brief.
- `explore` can run without a worktree when it only produces thinking.
- Artifact phases and implementation phases run in a dedicated worktree when they write files.
- VCS/PR provider: github (`gh` CLI); base branch: `development`

## Trello Tracking

- **Board**: `69ec097f13e2d38ecd89a557`

## Context Sources

- Trello is the source of truth for work state and dependencies.
- Vault is the canonical note-taker for decisions, handoffs, and structured project context.
- Specs and changes are tracked in the project's designated spec store (configurable per project).
- Engram is the operational memory layer for session facts, patterns, and short-lived continuity.
- Skills are executable guidance, not the primary contents of this runtime brief. Load specific skills from `ai-specs/skills/<name>/SKILL.md` only when relevant.
- Check Engram for the current active card and next recommended focus.

## Conflict Policy

- Current explicit human instruction controls the immediate scope unless it conflicts with safety, secrets, or a higher-authority project rule.
- Tracker controls work state; vault controls canonical decisions and handoffs; repo docs and manifests control versioned project contracts. Agent plans are lowest authority until accepted and recorded.
- Skills define reusable procedures. Engram provides searchable operational context, not final authority.
- Proposed agent plans are lowest authority until accepted and recorded in Trello, Vault, docs, or code.

## Workflow Rules

- Create a dedicated worktree for changes that write artifacts or modify code. Pure exploration can happen before a worktree if it writes no files.
- Do not merge or push to `development` without a PR and explicit human instruction.
- Preserve unrelated worktree changes; never revert changes you did not make.
- Use a PR-based merge workflow; all changes to `development` go through a pull request.
- VCS/PR provider: `github`. Use the provider CLI for all PR operations.
- Do not push directly to `development`; always open a PR from a feature branch.
- A session works on one explicit user request or tracker card; resolve focus from memory and tracker before starting.
- Follow red-green-refactor discipline: write a failing test first, then implement, then clean up.
- Run the full test suite before committing; do not leave the suite in a failing state.
- Inspect the active Trello card before resuming work and keep card state in sync with actual progress.
- Follow the project's designated workflow for structured changes.
- Direct `skill-sync` runs are allowed only for metadata validation.

## Useful Commands

- Full validation: `./tests/validate.sh`
- Focused tests (unit-only): `./tests/run.sh`
- Inspect the active Trello card before resuming work.
