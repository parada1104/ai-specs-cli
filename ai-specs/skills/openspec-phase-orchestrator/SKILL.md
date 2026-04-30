---
name: openspec-phase-orchestrator
description: >
  Orchestrate SDD/OpenSpec changes using phase-specialized subagents, minimal
  context packets, structured handoffs, and phase events consumable by recipes.
license: MIT
compatibility: Requires openspec CLI when OpenSpec is the configured SDD provider.
metadata:
  author: ai-specs
  version: "2.0"
  generatedBy: "manual-runtime"
  scope: [root]
  auto_invoke:
    - "Orchestrating an OpenSpec change phase by phase"
    - "Running a specific SDD phase in isolation"
    - "Starting a new OpenSpec change"
    - "Continuing an OpenSpec change"
    - "Implementing tasks from an OpenSpec change"
    - "Verifying an OpenSpec change implementation"
---

# openspec-phase-orchestrator

This skill coordinates SDD phases. It delegates execution to phase-specialized
subagents when the harness supports them and falls back to inline execution only
when subagents are unavailable.

Read `AGENTS.md` first. It defines the SDD provider, integration branch, tracker,
memory providers, test commands, and safety rules for the current project.

## Core Rules

- Use a fresh subagent for each phase whenever available.
- Keep each phase payload minimal; include paths and decisions, not full history.
- The executor reads dependency artifacts itself.
- The orchestrator delegates, parses handoffs, emits events, and decides whether to advance.
- It does not write artifacts directly in subagent mode.

## Worktree Requirement

- `explore` may run outside a worktree if it produces no artifact.
- Before `proposal`/`openspec-new-change` or any later artifact-writing phase, create or enter the dedicated change worktree.
- Payloads for artifact-writing phases must include `cwd` and `worktree_path`.
- Subagents must verify `cwd` before writing.

## Cycle Modes

- `interactive`: run one phase and ask before the next.
- `auto-artifacts`: run `proposal`, `specs`, `design`, and `tasks`, then stop before `apply`.
- `auto`: run artifacts, `apply`, and `verify`; stop on blocker, error, or failed verification.

Default to the mode declared in `AGENTS.md`; for this project that is `auto-artifacts` unless the user explicitly requests apply.

## Runtime Context Pack

Send each phase executor only:

- `project`: project id/name from `AGENTS.md`.
- `card`: tracker card id/title/list when relevant.
- `change`: OpenSpec change name/path.
- `phase`: target phase and requested cycle mode.
- `providers`: SDD, tracker, VCS, memory providers from `AGENTS.md`.
- `cwd`: worktree path for artifact-writing phases.
- `constraints`: safety rules, no-push/no-merge rules, test commands.
- `inputs`: paths to prerequisite artifacts.
- `outputs`: paths the phase may create/update.

## Phase Map

- `explore`: use `openspec-explore`; no artifact required.
- `proposal`: use `openspec-new-change`; creates `proposal.md`.
- `specs`: use `openspec-continue-change`; creates delta specs.
- `design`: use `openspec-continue-change`; creates `design.md`.
- `tasks`: use `openspec-continue-change`; creates `tasks.md`.
- `apply`: use `openspec-apply-change`; updates code and task checkboxes.
- `verify`: use `openspec-verify-change`; creates/updates `verify-report.md`.
- `archive`: use `openspec-archive-change`; only after merge/approval.
- `merge`: use `git-merge-workflow`; only on explicit user request.

## Handoff Contract

Every phase executor returns this YAML block first:

```yaml
---
handoff:
  phase: "<phase>"
  status: "complete" # complete | blocked | partial | error
  artifacts:
    - path: "<path>"
      status: "created" # created | updated | unchanged
  findings:
    - "<finding>"
  blockers: null
  decisions:
    - "<decision>"
  next_phase_suggested: "<phase-or-null>"
  notes: "<brief notes>"
---
```

If the block is missing, extract what you can, warn once, and stop before auto-advancing.

## Phase Events

Emit an event after each attempted phase so recipes/hooks can observe progress:

```yaml
---
event:
  type: "sdd.phase.completed" # or sdd.phase.started | sdd.phase.blocked | sdd.phase.failed
  provider: "openspec"
  change: "<change-name>"
  phase: "<phase>"
  status: "<status>"
  artifacts:
    - "<path>"
---
```

The orchestrator emits the event into the conversation or handoff. It does not execute recipe handlers directly unless a separate recipe contract says to do so.

## Advancement Rules

- In `interactive`, stop after every phase.
- In `auto-artifacts`, continue through `tasks` only, then present an artifact-cycle handoff.
- In `auto`, continue through `verify` only if no blocker/error occurs.
- Never enter `apply`, `verify`, `archive`, `merge`, PR creation, push, or cleanup without explicit permission unless the user selected `auto` and the runtime brief allows it.

## User-Facing Result

Report phase, status, artifacts, key findings, blockers, and next suggested action. Keep routine phase chatter short; details belong in the artifact or handoff.
