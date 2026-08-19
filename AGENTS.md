# ai-specs-cli Runtime Brief

> This is the project's director de orquesta: canonical runtime context for agents.
> It covers project identity, MCPs, context sources, safety rules, and workflow conventions.
> It does NOT track day-to-day work state — that lives in Trello and Engram.
>
> **YOU ARE ON THE GO SINGLE-BINARY MIGRATION BRANCH, NOT development.**
> This branch and every branch cut from it operate under the epic contract below.
> Rules you may know from `development` do not all apply here: the integration
> target is `epic/go-single-binary`, and nothing from this epic reaches
> `development` until the whole migration is verified. Epic:
> https://trello.com/c/qwlHQ7Xa

## Project

- **Project**: `ai-specs-cli`
- **Purpose**: per-project AI harness for agent configuration, MCPs, recipes, memory, and tracker integration.
- **Enabled runtimes**: `claude`, `cursor`, `opencode`, `pi`, `omp`
- **Integration branch**: `epic/go-single-binary`
- **Repo topology**: `standalone` (via auto)
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
- VCS/PR provider: GitHub (`gh` CLI); base branch: `epic/go-single-binary`

## Trello Tracking

- **Board**: `69ec097f13e2d38ecd89a557`

## Context Sources

- Trello is the source of truth for work state and dependencies.
- Vault is the canonical note-taker for decisions, handoffs, and structured project context.
- Specs and changes are tracked in the project's designated spec store (configurable per project).
- Engram is the operational memory layer for session facts, patterns, and short-lived continuity.
- Skills are executable guidance, not the primary contents of this runtime brief. Load specific skills by id when relevant (local under `ai-specs/skills/`, or CLI-bundled / recipe / dep via agent fan-out).
- Check Engram for the current active card and next recommended focus.

## Conflict Policy

- Current explicit human instruction controls the immediate scope unless it conflicts with safety, secrets, or a higher-authority project rule.
- Tracker controls work state; vault controls canonical decisions and handoffs; repo docs and manifests control versioned project contracts. Agent plans are lowest authority until accepted and recorded.
- Skills define reusable procedures. Engram provides searchable operational context, not final authority.
- Proposed agent plans are lowest authority until accepted and recorded in Trello, Vault, docs, or code.

## Workflow Rules

- Create a dedicated worktree for changes that write artifacts or modify code. Pure exploration can happen before a worktree if it writes no files.
- Do not merge or push to `epic/go-single-binary` without a PR and explicit human instruction.
- Preserve unrelated worktree changes; never revert changes you did not make.
- Before dispatching a write-capable subagent or task, verify which git repository, worktree, and branch yourself (`git rev-parse --show-toplevel`, `git branch --show-current`, `git worktree list`). Under monorepo-submodules, confirming which-repo via show-toplevel is mandatory. Do not rely solely on runtime pre-tool-use hooks — they may not fire for delegated/subprocess tool calls on opencode/pi/omp.
- If a structured Edit/Write/MultiEdit call is blocked or errors for any reason while on a protected branch, that is never grounds to retry the write via bash/shell (heredoc, `python3 -c`, `cat >`, `tee`, `sed -i`). Create a worktree first (e.g. `/worktree-new`) and write there instead.
- Use a PR-based merge workflow; all changes to `epic/go-single-binary` go through a pull request.
- VCS/PR provider: GitHub (gh CLI). Use gh for all PR operations.
- Do not push directly to `epic/go-single-binary`; always open a PR from a feature branch.
- After a merged PR, remove the feature worktree and delete the local branch (`git branch -D` after squash); delete the remote branch if it still exists.
- After a merged PR, sync `epic/go-single-binary` in the main worktree before further work: `git checkout epic/go-single-binary` then `git pull --ff-only`.
- A session works on one explicit user request or tracker card; resolve focus from memory and tracker before starting.
- Follow red-green-refactor discipline: write a failing test first, then implement, then clean up.
- Run the full test suite before committing; do not leave the suite in a failing state.
- Classify each substantial change (full planning chain, spec+tasks, or tasks-only) before writing production code; compute the signal depth, compare any explicit requested depth, ask on conflicts, and annotate requested/signal/decided depth in tasks.md before authorization.
- Direct implementation requests without a change folder still require planning at the classified depth; approval verbs do not skip the plan step.
- Do not open a PR until the change folder on the branch contains the tier minimum planning files (Light: proposal.md + tasks.md; Standard: proposal.md + tasks.md + specs/**/*.md; Full: tasks.md plus proposal.md or design.md plus specs/**/*.md), committed.
- After authorization, implement and validate in the change worktree when isolated worktrees are enabled.
- Before merge, run verify evidence before archive-tail (Standard/Full block without a conforming verify-report.md; Light is advisory), archive the change folder on the review branch at openspec/changes/archive/YYYY-MM-DD-<slug>/ using a valid ISO calendar date, and run the pre-merge guardian again; exact undated archive/<slug>/ is legacy fallback only, ambiguity and malformed or near-match candidates block, and archive is never deferred until after merge.
- Default artifact store for this project's planning artifacts: `openspec`. When a session asks where planning artifacts should live, answer with this value unless the user overrides it. The store is a persistence preference only: plan-build readiness is always proven by the file-backed canonical change-folder tree, never by a memory-only store.
- For recognized submodule worktrees, use the topology-derived central planning tree in the superproject; standalone repositories keep their own planning tree.
- Inspect the active Trello card before resuming work and keep card state in sync with actual progress.
- Before apply/production work on a structured change, create or link a Trello card and record it in the ## Tracker section of the change's proposal.md (or tasks.md) — card_id + url. openspec/** writes are never gated — write the link section there first.
- On SDD phase transitions, move the card and update its phase label; post a progress comment at milestones.
- If the tracker gate warns or blocks, create/link the card and write the ## Tracker section — never bypass via shell writes, and never claim 'Trello unavailable' when the real gap is a missing link section. A missing card is an availability failure only when the MCP/network is genuinely down.
- Only omit a card by writing openspec/changes/<slug>/tracker.none with a one-line reason; this is logged and rare.
- Follow the project's designated workflow for structured changes.
- Direct `skill-sync` runs are allowed only for metadata validation.
- EPIC CONTRACT — Go single-binary migration (https://trello.com/c/qwlHQ7Xa). This branch is the integration target for that epic. Card branches are cut FROM `epic/go-single-binary` and their PRs target it. No PR from this epic ever targets `development` or `main`.
- EPIC CONTRACT — Nothing from this epic reaches `development` while the epic is open. After all 16 cards are verified, ONE promotion PR is opened from card 16 (Cutover). Unrelated feature work continues on `development` in parallel and must not be disturbed.
- EPIC CONTRACT — TWO config values are deliberately scoped to this branch: `[recipes.git-pr-flow.config].base_branch` and `[recipes.worktree-flow.config].integration_branch`. BOTH MUST be reverted to `development` before the promotion PR, or every PR and worktree flow in the project starts targeting a deleted branch. Tracked as a blocking step on card 16.
- EPIC CONTRACT — New development moves toward the migration: no new Python modules under `lib/_internal/`, no new Bash logic in `lib/`, no new vendored Python under `lib/_vendor/`. New behavior in an already-ported area is written in Go; in a not-yet-ported area it must not deepen the Bash/Python surface.
- EPIC CONTRACT — Ported behavior is verified against `docs/go-migration-parity-contract.md`, which classifies every CLI surface FROZEN / TOLERANT / FREE. Do not 'fix' behavior recorded there as FROZEN, even when it looks wrong; recorded defects (D1-D35) are separate cards.
- DELEGATION — Orca workers own change content inside their assigned worktree only. A worker never stages, commits, pushes, merges, or manages worktree lifecycle; the canonical orchestrator owns those. See the `orca-aware-delegation` skill.
- DELEGATION — Never launch a worker headless (`pi -p`, `claude -p`, `opencode run`, provider API calls). Orca workers launch as visible interactive TUI sessions via `worker-start`.
- DELEGATION — `worker_done` never implies terminal closure by itself. Standing human decision for this epic: release once a worker completes its change cycle, since the PR lifecycle belongs to the orchestrator. Release only after an accepted `worker_done` — never on a timeout, idle TUI, heartbeat, question, escalation, or stale report — and retain anything that escalated, asked, or failed pending inspection.

## Useful Commands

- Full validation: `./tests/validate.sh`
- Focused tests (unit-only): `./tests/run.sh`
- Inspect the active Trello card before resuming work.
- For ai-specs harness operations (init, sync, recipes, skills/deps, doctor), load the `harness-lifecycle`, `harness-recipes`, or `harness-skills-deps` skills.
