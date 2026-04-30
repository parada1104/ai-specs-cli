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
3. When creating the change, update the integration branch and create the worktree:

```bash
git checkout <integration-branch>
git pull --ff-only origin <integration-branch>
mkdir -p .worktrees
git worktree add .worktrees/<change-name> -b <change-name> <integration-branch>
cd .worktrees/<change-name>
```

4. Verify the worktree baseline when feasible with the focused command from `AGENTS.md`.
5. Run SDD phases through `openspec-phase-orchestrator` when subagents are available.

## SDD Cycle Modes

- `interactive`: run one phase, present handoff, ask before continuing.
- `auto-artifacts`: run artifact phases through `tasks.md`, then stop before `apply`.
- `auto`: run artifacts, apply, and verify; stop on blocker or failed verification.

Default for this project: `auto-artifacts` unless the user explicitly asks for implementation.

## Phase Map

- `explore`: thinking and scope discovery; no artifact required.
- `proposal`: create `openspec/changes/<change>/proposal.md`.
- `specs`: create/update delta specs under `openspec/changes/<change>/specs/`.
- `design`: create/update `design.md`.
- `tasks`: create/update `tasks.md`.
- `apply`: implement tasks and update checkboxes.
- `verify`: validate against specs and write `verify-report.md`.
- `archive`: archive after merge/approval according to project rules.

## Guardrails

- Use phase-specialized subagents whenever the runtime supports them.
- Do not enter `apply`, push, merge, archive, or clean up worktrees without explicit human instruction.
- Preserve unrelated changes in any workspace or worktree.
- If tracker state, OpenSpec state, and handoff disagree, present the conflict and ask.
- Keep commits aligned with coherent task groups when implementation begins.
- Before final verification, run the focused tests plus full validation when feasible.

## Artifact-Cycle Handoff

After `tasks.md` in `auto-artifacts`, stop and report:

- Card/change linkage and intended outcome.
- Artifacts created or updated.
- Risks and open questions.
- Recommended implementation strategy.
- Clear options: `interactive apply`, `auto apply`, or `apply only`.

## Related Skills

- `openspec-phase-orchestrator` for subagent phase execution.
- `openspec-new-change` and `openspec-continue-change` for artifact creation.
- `openspec-apply-change`, `openspec-verify-change`, and `openspec-archive-change` for later phases.
- `git-merge-workflow` for PR, merge, cleanup, and sync.
