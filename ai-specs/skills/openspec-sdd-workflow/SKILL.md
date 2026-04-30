---
name: openspec-sdd-workflow
description: >
  Config-driven SDD + worktree workflow for ai-specs shaped projects. Covers
  card/change linkage, worktree creation before artifact-writing phases,
  phase-specialized subagents, verification, and archive/merge handoff.
license: MIT
metadata:
  author: parada1104
  version: "3.0"
  scope: [root]
  auto_invoke:
    - "Starting a new OpenSpec change"
    - "Orchestrating an OpenSpec change phase by phase"
    - "Creating a branch or worktree"
    - "Starting work from development"
    - "Deciding where implementation should begin"
    - "Running a specific SDD phase in isolation"
    - "Editing openspec/config.yaml for spec-driven workflow"
---

# OpenSpec SDD Workflow

Use this skill when a user request or tracker card becomes a durable SDD cycle.
Read `AGENTS.md` first; it defines the project-specific provider choices, base
branch, MCPs, safety rules, and test commands.

For this project, the runtime brief currently says: tracker = Trello, SDD provider = OpenSpec, integration branch = `development`, PR/VCS provider = GitHub via `gh`.

## Operating Model

- One explicit session focus maps to one user request, tracker card, or SDD change.
- One implementation/design card maps to one SDD cycle.
- SDD artifacts are authoritative in OpenSpec.
- Trello owns work state and dependencies.
- Vault owns canonical handoffs and decisions.
- OpenMemory owns operational continuity.

## Worktree Boundary

- Pure `explore` may run without a worktree when it only produces conversation-level thinking.
- Before `openspec-new-change` or any artifact-writing phase, create or enter a dedicated worktree from the integration branch.
- Artifact phases (`proposal`, `specs`, `design`, `tasks`) run inside that worktree.
- Implementation phases (`apply`, `verify`, `archive`) also run inside that worktree unless the user explicitly directs otherwise.
- Do not write OpenSpec artifacts on the integration branch.
- One change should have one branch/worktree. Reuse an existing matching worktree when present; do not create a second one silently.

## Standard Start

1. Resolve the session focus from the user request, tracker card, or existing change.
2. If only exploring, explore in place and stop with findings unless the user asks to create the change.
3. **Before creating the change, invoke `openspec-sdd-decision`** to classify the
   change and determine the ceremony level. Confirm the classification with the
   user before proceeding.
4. When creating the change, update the integration branch and create the worktree:

```bash
git checkout <integration-branch>
git pull --ff-only origin <integration-branch>
mkdir -p .worktrees
git worktree add .worktrees/<change-name> -b <change-name> <integration-branch>
cd .worktrees/<change-name>
```

5. Verify the worktree baseline when feasible with the focused command from `AGENTS.md`.
6. Run SDD phases through `openspec-phase-orchestrator` when subagents are available.

## Plan Mode Contract

When the user is in plan mode, asks for a plan, or is validating approach before
implementation, describe the workflow that will be executed rather than a
premature implementation plan.

- If the session focus is a formal tracker card or durable design/implementation
  request, perform explore/classification first, then present the SDD flow.
- For SDD-required classifications, the plan MUST emphasize ceremony level,
  cycle mode, subagent delegation, artifact phases, stop point, and review
  output. Likely code areas may be listed only as impact/risk context.
- For changes below the SDD threshold, the plan MAY describe direct code/test
  changes, but it MUST still include whether spec updates are needed and why.
- In build mode, execute the same flow that would have been described in plan
  mode.

Every plan response after classification MUST include:

- Classification and reasoning.
- SDD mode (`interactive`, `auto-artifacts`, or `auto`).
- Subagent execution model for required phases.
- Required phases and artifacts, according to `openspec/config.yaml`.
- Explicit stop point, especially "stop before `apply`" for `auto-artifacts`.
- Spec update statement: which specs/delta specs are expected, or why no spec
  update is required.
- Final handoff/review contents and options for next action.

## Ceremony-Level Plan Mapping

- `domain_change`: formal OpenSpec cycle. Create/enter a dedicated worktree,
  run artifact phases through phase-specialized subagents, normally
  `proposal`, `specs`, `design`, and `tasks`, then stop before `apply` in
  `auto-artifacts`. The handoff must include technical analysis, risks,
  recommendations, open questions, and apply options.
- `behavior_change`: lightweight SDD cycle. Create/enter a dedicated worktree
  and create the artifacts required by `openspec/config.yaml` (normally
  `tasks.md`). Create delta specs only when the decision matrix requires them
  or the observable contract/spec source of truth must change. Stop before
  `apply` unless the user explicitly requested implementation.
- `local_fix`: no formal OpenSpec artifacts by default. Describe or execute the
  localized fix plus focused tests. If the fix reveals contract/spec drift,
  escalate classification before proceeding.
- `trivial`: no SDD artifacts. Apply directly when in build mode, with minimal
  validation appropriate to the edit.

## SDD Cycle Modes

- `interactive`: run one phase, present handoff, ask before continuing.
- `auto-artifacts`: run artifact phases through `tasks.md`, then stop before `apply`.
- `auto`: run artifacts, apply, and verify; stop on blocker or failed verification.

Default for this project: `auto-artifacts`.

Default for this project: `auto-artifacts` unless the user explicitly asks for implementation.

When a user says “execute the SDD cycle” for a card or change without specifying
a mode, use `auto-artifacts`: create/link the change, run artifact phases, then
stop before `apply`.

When the harness supports subagents, every SDD phase MUST be executed by a
phase-specialized subagent through `openspec-phase-orchestrator`.

## Phase Map

- `explore`: thinking and scope discovery; no artifact required.
- `proposal`: create `openspec/changes/<change>/proposal.md`.
- `specs`: create/update delta specs under `openspec/changes/<change>/specs/`
  when required by the decision matrix or by a contract/source-of-truth change.
- `design`: create/update `design.md`.
- `tasks`: create/update `tasks.md`.
- `apply`: implement tasks and update checkboxes.
- `verify`: validate against specs and write `verify-report.md`.
- `archive`: archive after merge/approval according to project rules.

> **Adaptive note**: if the ceremony level is `trivial` or `local_fix`, omit
> artifact phases that the decision matrix declares unnecessary. For `trivial`,
> no SDD artifacts are required. For `local_fix`, only code changes and tests
> are required.

## Guardrails

- Use phase-specialized subagents whenever the runtime supports them.
- Do not enter `apply`, push, merge, archive, or clean up worktrees without explicit human instruction.
- Preserve unrelated changes in any workspace or worktree.
- If tracker state, OpenSpec state, and handoff disagree, present the conflict and ask.
- Keep commits aligned with coherent task groups when implementation begins.
- Before final verification, run the focused tests plus full validation when feasible.
- **Respect the `sdd.threshold` of the active recipe**. If the classification of
  the change falls below that threshold, emit a warning and consult the user
  before proceeding.

## Artifact-Cycle Handoff

After `tasks.md` in `auto-artifacts`, stop and report:

After the final artifact (`tasks.md`) in `auto-artifacts`, provide an exhaustive
artifact-cycle review with:

- Card/change linkage and intended outcome.
- Artifacts created or updated.
- Risks and open questions.
- Recommended implementation strategy.
- Clear options: `interactive apply`, `auto apply`, or `apply only`.

Else if tasks exist with pending checkboxes → `apply` only when mode is `auto` or the user explicitly requested apply; otherwise stop with the artifact-cycle review.

## Related Skills

- `openspec-phase-orchestrator` for subagent phase execution.
- `openspec-new-change` and `openspec-continue-change` for artifact creation.
- `openspec-apply-change`, `openspec-verify-change`, and `openspec-archive-change` for later phases.
- `git-merge-workflow` for PR, merge, cleanup, and sync.
