# ai-specs-cli Runtime Brief
<!-- ai-specs:runtime-brief -->

> This is the project's director de orquesta: canonical runtime context for agents. It covers project identity, MCPs, context sources, safety rules, and workflow conventions. It does NOT track day-to-day work state — that lives in Trello and OpenMemory. The auto-generated runtime brief from `ai-specs sync` is thinner than this manual version because `ai-specs.toml` does not yet support richer runtime context (board IDs, dependency tracking, workflow rules, useful commands). The north star is Option C: enrich `ai-specs.toml` so the generated brief matches this content without hand-editing. Until then, this file remains manual.

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

## Runtime Flow

- A session works on one explicit user request, Trello card, or OpenSpec change.
- A relevant Trello card maps to one SDD cycle when implementation or durable design is needed.
- This project uses OpenSpec as the SDD provider: proposal, specs, design, tasks, apply, verify, archive.
- **Subagent execution model**: The current runtime delegates each SDD phase to a `general` subagent via the `task` tool. Phase specialization is achieved through detailed prompts (e.g., a "specs specialist" prompt for the specs phase), not through native agent types. The orchestrator (primary agent) must respect the workflow flow — for example, `auto-artifacts` runs proposal through tasks, then pauses for human direction before apply.
- **Future**: Trello card #68 tracks professionalizing this with true phase-specialized subagents, enforcement hooks, and a dispatcher integrated with `openspec-phase-orchestrator`.
- `explore` can run without a worktree when it only produces thinking. Create the worktree before `openspec-new-change` or any artifact-writing phase.
- Artifact phases (`proposal`, `specs`, `design`, `tasks`) and implementation phases (`apply`, `verify`, `archive`) run inside the dedicated worktree.
- VCS/PR provider: GitHub through `gh` CLI.

## Trello Tracking

- Board: `69ec0a2099ea20956e371d62` (`ai-specs-cli Roadmap`).
- Trello is the source of truth for work state and dependencies. Check OpenMemory for the current active card and next recommended focus.

## Context Sources

- Trello is the source of truth for work state and dependencies.
- OpenSpec is the source of truth for specs, changes, tasks, apply evidence, verify reports, and archives.
- Vault is the canonical note-taker for decisions, handoffs, and structured project context.
- OpenMemory is the operational memory layer for session facts, patterns, and short-lived continuity.
- Skills are executable guidance, not the primary contents of this runtime brief. Load specific skills from `ai-specs/skills/<name>/SKILL.md` only when relevant.

## Conflict Policy

- Current explicit human instruction controls the immediate scope unless it conflicts with safety, secrets, or a higher-authority project rule.
- Trello controls work state; OpenSpec controls SDD artifacts; Vault controls canonical decisions and handoffs; repo docs and manifests control versioned project contracts.
- Skills define reusable procedures. OpenMemory provides searchable operational context, not final authority.
- Proposed agent plans are lowest authority until accepted and recorded in Trello, OpenSpec, Vault, docs, or code.

## Workflow Rules

- Do not merge or push to `development` without explicit human instruction.
- Start change artifact work from `development` in a dedicated worktree unless the user explicitly directs otherwise. Pure exploration can happen before a worktree if it writes no artifacts.
- Preserve unrelated worktree changes; never revert changes you did not make.
- For OpenSpec changes, use the project SDD workflow and subagent isolation when available.
- Before final verification, run the relevant focused tests plus `./tests/validate.sh` when feasible.
- Do not run `ai-specs sync` in this repo until the TOML schema supports richer runtime context (Option C).
- Direct `skill-sync` runs are allowed only for metadata validation; this file's runtime marker makes `skill-sync` skip Auto-invoke insertion.

## Current Transitional State

- `ai-specs/skills/skill-sync/assets/sync.sh` respects the `<!-- ai-specs:runtime-brief -->` marker and skips rewriting `AGENTS.md`.
- `lib/_internal/agents-md-render.py` generates a runtime brief from `ai-specs.toml`, but the output is thinner than this manual brief.
- This file remains intentionally manual and non-idempotent until the TOML schema supports richer runtime context (Option C: enrich `ai-specs.toml` so the generated brief matches this content without hand-editing).

## Useful Commands

- Focused tests: `./tests/run.sh`
- Full validation: `./tests/validate.sh`
- Inspect the active Trello card before resuming work.
