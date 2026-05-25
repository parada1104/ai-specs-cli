# ai-specs-cli Runtime Brief
<!-- ai-specs:runtime-brief -->

> This is the project's director de orquesta: canonical runtime context for agents. It covers project identity, MCPs, context sources, safety rules, and workflow conventions. It does NOT track day-to-day work state — that lives in Trello and Engram. The auto-generated runtime brief from `ai-specs sync` is thinner than this manual version because `ai-specs.toml` does not yet support richer runtime context (board IDs, dependency tracking, workflow rules, useful commands). The north star is Option C: enrich `ai-specs.toml` so the generated brief matches this content without hand-editing. Until then, this file remains manual.

## Project

- Project: `ai-specs-cli`
- Manifest: `ai-specs/ai-specs.toml`
- Purpose: per-project AI harness for agent configuration, MCPs, recipes, memory, and tracker integration.
- Enabled runtimes: `claude`, `cursor`, `opencode`, `pi`
- Integration branch: `development`

## Runtime MCPs

- `trello`: project tracking through the ai-specs-cli Roadmap board.
- `engram`: operational/session memory (global MCP).
- `vault-ai-specs`: canonical project notes in the Obsidian vault scoped to `nnodes/proyectos/ai-specs`.
- Never expose env-backed secrets from MCP config in generated docs or comments.

## Runtime Flow

- A session works on one explicit user request or Trello card.
- The orchestrator coordinates work inline using project skills and the runtime brief.
- `explore` can run without a worktree when it only produces thinking.
- Artifact phases and implementation phases run in a dedicated worktree when they write files.
- VCS/PR provider: GitHub through `gh` CLI.

## Trello Tracking

- Board: `69ec0a2099ea20956e371d62` (`ai-specs-cli Roadmap`).
- Trello is the source of truth for work state and dependencies. Check Engram for the current active card and next recommended focus.

## Context Sources

- Trello is the source of truth for work state and dependencies.
- Specs and changes are tracked in the project's designated spec store (configurable per project).
- Vault is the canonical note-taker for decisions, handoffs, and structured project context.
- Engram is the operational memory layer for session facts, patterns, and short-lived continuity.
- Skills are executable guidance, not the primary contents of this runtime brief. Load specific skills from `ai-specs/skills/<name>/SKILL.md` only when relevant.

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
- Do not run `ai-specs sync` in this repo until the TOML schema supports richer runtime context (Option C).
- Direct `skill-sync` runs are allowed only for metadata validation; this file's runtime marker makes `skill-sync` skip Auto-invoke insertion.

## Current Transitional State

- `ai-specs/skills/skill-sync/assets/sync.sh` respects the `<!-- ai-specs:runtime-brief -->` marker and skips rewriting `AGENTS.md`.
- This file remains intentionally manual and non-idempotent until the TOML schema supports richer runtime context (Option C: enrich `ai-specs.toml` so the generated brief matches this content without hand-editing).

## Useful Commands

- Focused tests: `./tests/run.sh`
- Full validation: `./tests/validate.sh`
- Inspect the active Trello card before resuming work.
