# ai-specs-cli Runtime Brief

> This is the project's director de orquesta: canonical runtime context for agents.
> It covers project identity, MCPs, context sources, safety rules, and workflow conventions.
> It does NOT track day-to-day work state — that lives in Trello and Engram.

## Project

- **Project**: `ai-specs-cli`
- **Purpose**: per-project AI harness for agent configuration, MCPs, recipes, memory, and tracker integration.
- **Enabled runtimes**: `claude`, `cursor`, `opencode`, `pi`, `omp`
- **Integration branch**: `development`
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
- VCS/PR provider: GitHub (`gh` CLI); base branch: `development`

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

- Protected-branch writes are governed by the effective worktree `gate_mode = ask`: `always` requires a dedicated worktree before writing; `ask` requires the orchestrator to ask the user to choose a destination (a dedicated worktree, a feature branch in the current checkout, or an explicit protected-branch override); `off` permits writing where the user directs. Pure exploration can proceed without a worktree when it writes no files.
- Do not merge or push to `development` without a PR and explicit human instruction.
- Preserve unrelated worktree changes; never revert changes you did not make.
- Before dispatching a write-capable subagent or task, verify which git repository, worktree, and branch yourself (`git rev-parse --show-toplevel`, `git branch --show-current`, `git worktree list`). Under monorepo-submodules, confirming which-repo via show-toplevel is mandatory. Do not rely solely on runtime pre-tool-use hooks — they may not fire for delegated/subprocess tool calls on opencode/pi/omp.
- If a structured Edit/Write/MultiEdit call is blocked or errors for any reason while on a protected branch, that is never grounds to retry the write via bash/shell (heredoc, `python3 -c`, `cat >`, `tee`, `sed -i`). With `gate_mode = always`, create a dedicated worktree first (e.g. `/worktree-new`) and write there instead; with `gate_mode = ask`, ask the user which destination to use — worktree (recommended), feature branch in place, or explicit protected-branch override — and wait; with `gate_mode = off`, the gate does not block.
- Use a PR-based merge workflow; all changes to `development` go through a pull request.
- VCS/PR provider: GitHub (gh CLI). Use gh for all PR operations.
- Do not push directly to `development`; always open a PR from a feature branch.
- After a merged PR, remove the feature worktree and delete the local branch (`git branch -D` after squash); delete the remote branch if it still exists.
- After a merged PR, sync `development` in the main worktree before further work: `git checkout development` then `git pull --ff-only`.
- A session works on one explicit user request or tracker card; resolve focus from memory and tracker before starting.
- Follow red-green-refactor discipline: write a failing test first, then implement, then clean up.
- Run the full test suite before committing; do not leave the suite in a failing state.
- Inspect the active Trello card before resuming work and keep card state in sync with actual progress.
- Before apply/production work on a structured change, create or link a Trello card and record it in the ## Tracker section of the change's proposal.md (or tasks.md) — card_id + url. openspec/** writes are never gated — write the link section there first.
- On SDD phase transitions, move the card and update its phase label; post a progress comment at milestones.
- The tracker gate is advisory: a warn/block/ask verdict is a prompt to create/link the card and write the ## Tracker section — never bypass via shell writes, and never claim 'Trello unavailable' when the real gap is a missing link section. A missing card is an availability failure only when the MCP/network is genuinely down.
- Only omit a card by writing openspec/changes/<slug>/tracker.none with a one-line reason; this is logged and rare.
- Tracker lifecycle checkpoints are Plan Build-independent: grade them with the tracker gate verb `ai-specs tracker-card-gate --root <root> --checkpoint pre-merge|archive-close [slug]`; no openspec/ tree is required, and `archive-close` is tracker item closure, never an OpenSpec archive.
- Provider tracker recipes only map native state through `[config.reconcile]`; they never enable, configure, or implement the ledger, and they never grade items.
- Classify each substantial change from the artifacts it carries instead of from a declared or requested depth: no plan is `none` (an ODD small task), a plan alone (a tasks-only change) is `light`, a plan plus a delta is `standard`, and a delta with no plan is allowed; nothing declares a depth, no tier has artifact minima, and `full` does not exist.
- Direct implementation requests without a change folder need no plan when the change stays `none` or carries only a delta; approval verbs never skip the plan a `light` or `standard` change needs.
- Do not open a PR until the change's plan parses (`## Specs` and `## Tasks`, plus `## Log` with `L1` from `standard` up), until the delivery evidence the guardian grades — the plan and the delta — is contributed by the change's own diff, and until that evidence is committed; `proposal.md`, `design.md` and `verify-report.md` are optional and still honoured when present.
- After authorization, implement and validate in the change worktree when isolated worktrees are enabled.
- Before merge on the review branch: run the evidence the plan's `## Verification` records as one row per declared `S#`, graded from `standard` up and advisory for `light` (`verify-report.md` is never required, and `proposal.md` and `design.md` stay optional), then promote canonical specs so every delta under specs/<domain>/spec.md is composed into openspec/specs/<domain>/spec.md (the promoter is the only writer and the guardian only validates), then pass the pre-archive guardian, then archive the change folder at openspec/changes/archive/YYYY-MM-DD-<slug>/ using a valid ISO calendar date, then run the pre-merge guardian again; an unpromoted or unresolved delta blocks a `standard` change, exact undated archive/<slug>/ is legacy fallback only, ambiguity and malformed or near-match candidates block, and archive is never deferred until after merge.
- Default artifact store for this project's planning artifacts: `openspec`. When a session asks where planning artifacts should live, answer with this value unless the user overrides it. The store is a persistence preference only: plan-build readiness is always proven by the file-backed canonical change-folder tree, never by a memory-only store.
- For recognized submodule worktrees, use the topology-derived central planning tree in the superproject; standalone repositories keep their own planning tree.
- Resolve the execution mode from the project manifest and let each mode gate its own way: in odd mode the pre-tool-use hook requires no plan and protects only the canonical spec tree `odd/specs/`, and an archive is owed only when the change still carries a change folder; in openspec mode the hook still requires the paired change's plan, with the local escape hatch `ai-specs changes start --none --root <root>`.
- Grade the change with `ai-specs gates premerge --root <root> [--change <c>] [--base <ref>] [--tier none|light|standard]`, or with `ai-specs plan-build-gate` for the pre-tool-use gate; `--tier` is a coherence assertion that blocks when it contradicts the derived tier, naming the derived tier and both exits, and never a choice.
- Consume one session-level preflight for execution mode, artifact store, review budget, delivery strategy, and chain strategy; never recollect or override those values.
- Present an artifact-derived plan with intent, scope, key decisions, affected areas, risks, open questions, and labeled recommendations/assumptions; ask accept, adjust, or stop and block unresolved product decisions.
- Follow the project's designated workflow for structured changes.
- Direct `skill-sync` runs are allowed only for metadata validation.
- New authoritative logic, state machines, predicates, and durable state belong in Go.
- When touching Python on the active path, migrate the behavior/dependency being touched to Go where practical; Python may remain only as a thin compatibility/acquisition/JSON bridge during the transition.
- Keep one authoritative grader per behavior and add parity/contract tests at each migrated seam.
- Do not perform unrelated drive-by rewrites; the policy is incremental.
- DELEGATION — Orca workers own change content inside their assigned worktree only. A worker never stages, commits, pushes, merges, or manages worktree lifecycle; the canonical orchestrator owns those. See the `orca-aware-delegation` skill.
- DELEGATION — Never launch a worker headless (`pi -p`, `claude -p`, `opencode run`, provider API calls). Orca workers launch as visible interactive TUI sessions via `worker-start`.
- DELEGATION — `worker_done` never implies terminal closure by itself. Standing human decision: release once a worker completes its change cycle, since the PR lifecycle belongs to the orchestrator. Release only after an accepted `worker_done` — never on a timeout, idle TUI, heartbeat, question, escalation, or stale report — and retain anything that escalated, asked, or failed pending inspection.

## Useful Commands

- Full validation: `./tests/validate.sh`
- Focused tests (unit-only): `./tests/run.sh`
- Inspect the active Trello card before resuming work.
- For ai-specs harness operations (init, sync, recipes, skills/deps, doctor), load the `harness-lifecycle`, `harness-recipes`, or `harness-skills-deps` skills.
