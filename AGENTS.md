# ai-specs-cli Runtime Brief
<!-- ai-specs:runtime-brief -->

> Temporary runtime brief. Do not run `ai-specs sync` in this repo until Trello card #65 is implemented; the current CLI renderer would regenerate the legacy skill registry.

## Project

- Project: `ai-specs-cli`
- Manifest: `ai-specs/ai-specs.toml`
- Purpose: per-project AI harness for agent configuration, MCPs, recipes, memory, tracker integration, and OpenSpec/SDD workflows.
- Enabled runtimes: `claude`, `cursor`, `opencode`
- Integration branch: `development`

## Runtime MCPs

- `trello`: project tracking through the ai-specs-cli Roadmap board.
- `openmemory`: operational/session memory at `http://localhost:8080/mcp`.
- `vault-ai-specs`: canonical project notes in the Obsidian vault scoped to `nnodes/proyectos/ai-specs`.
- Never expose env-backed secrets from MCP config in generated docs or comments.

## Trello Tracking

- Board: `69ec0a2099ea20956e371d62` (`ai-specs-cli Roadmap`).
- Active blocked feature: #62 `Formalizar recipe trello-mcp-workflow`.
- #62 must not enter apply until #63, #64, and #65 are complete and merged to `development`.
- #65 covers this runtime brief migration: `AGENTS.md` becomes project runtime context, while skill/capability registries move to separate generated artifacts.

## Context Sources

- Trello is the source of truth for work state and dependencies.
- OpenSpec is the source of truth for specs, changes, tasks, apply evidence, verify reports, and archives.
- Vault is the canonical note-taker for decisions, handoffs, and structured project context.
- OpenMemory is the operational memory layer for session facts, patterns, and short-lived continuity.
- Skills are executable guidance, not the primary contents of this runtime brief. Load specific skills from `ai-specs/skills/<name>/SKILL.md` only when relevant.

## Workflow Rules

- Do not merge or push to `development` without explicit human instruction.
- Start implementation work from `development` in a dedicated worktree unless the user explicitly directs otherwise.
- Preserve unrelated worktree changes; never revert changes you did not make.
- For OpenSpec changes, use the project SDD workflow and subagent isolation when available.
- Before final verification, run the relevant focused tests plus `./tests/validate.sh` when feasible.
- Do not run `ai-specs sync` in this repo until #65 replaces the legacy `AGENTS.md` renderer.
- Direct `skill-sync` runs are allowed only for metadata validation; this file's runtime marker makes `skill-sync` skip Auto-invoke insertion.

## Current Transitional State

- `ai-specs/skills/skill-sync/assets/sync.sh` respects the `<!-- ai-specs:runtime-brief -->` marker and skips rewriting `AGENTS.md` as an Auto-invoke registry.
- `lib/_internal/agents-md-render.py` still renders the old registry model until #65 is implemented.
- This file is intentionally manual and non-idempotent until #65 lands.

## Useful Commands

- Focused tests: `./tests/run.sh`
- Full validation: `./tests/validate.sh`
- Inspect Trello card #62 before resuming recipe work.
- Avoid `ai-specs sync` until #65 is complete.
