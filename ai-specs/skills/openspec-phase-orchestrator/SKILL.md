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

When the harness exposes a `task`/subagent capability, every SDD phase MUST run
through a fresh phase-specialized subagent. Inline execution is only a fallback
for harnesses that do not support subagents.

Inline execution is only a fallback for harnesses that do not support subagents.

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

| Mode | Behavior | Stop Point |
|------|----------|------------|
| `interactive` | Execute one phase at a time and ask before advancing. | After every phase. |
| `auto-artifacts` | Automatically run artifact phases through `tasks.md`. | Stop after `tasks.md`; do not apply or verify. |
| `auto` | Automatically run artifacts, apply, and verify. | Stop after verification (or on error/blocker). |

Default for this project: `auto-artifacts`.

Default to the mode declared in `AGENTS.md`; for this project that is
`auto-artifacts` unless the user explicitly requests apply.

## Plan Mode Summary

When asked for a plan before execution, summarize the orchestration plan instead
of implementation details:

- State the classification and the selected cycle mode.
- State that each required SDD phase will run in a fresh phase-specialized
  subagent when the harness supports subagents.
- List the required phases and artifacts from `openspec/config.yaml`.
- State the stop point, especially that `auto-artifacts` stops after `tasks.md`
  and before `apply`.
- Include a spec update statement: delta specs to create/update, or why specs
  are not required for this ceremony level.
- State that the final `auto-artifacts` output is an artifact-cycle review with
  technical analysis, risks, recommendations, open questions, and apply options.

Do not make detailed code-edit promises in this plan. Implementation details
belong in `tasks.md` or the later `apply` phase.

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
- `proposal`: use `openspec-new-change`; creates `proposal.md` **only if**
  `proposal_required` is `true` for the classified ceremony level.
- `specs`: use `openspec-continue-change`; creates delta specs when required by
  `openspec/config.yaml` or when the analyzed change modifies a spec-owned
  contract. Do not force specs for `behavior_change` when the active decision
  matrix only requires `tasks.md` and no source-of-truth spec needs updating.
- `design`: use `openspec-continue-change`; creates `design.md` **only if**
  `design_required` is `true` for the classified ceremony level.
- `tasks`: use `openspec-continue-change`; creates `tasks.md` when required by
  the active decision matrix; keep it minimal for lightweight ceremonies.
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
- Never enter `apply`, `verify`, `archive`, `merge`, PR creation, or push/merge to `development` from `auto-artifacts` without explicit human instruction.
- Never enter cleanup without explicit permission unless the user selected `auto` and the runtime brief allows it.
- **If the ceremony level is `trivial`, the full SDD cycle may be omitted.**
  The agent applies the change directly and documents the classification in the
  commit message or an operational note.

## Artifact-Cycle Review (auto-artifacts)

After `tasks.md` is created or updated in `auto-artifacts` mode, stop and present an exhaustive artifact-cycle review. Do not proceed to `apply` without explicit user confirmation. The review MUST include the following sections:

### 1. Artifact Summary
- List every artifact created or updated (`proposal.md`, delta specs, `design.md`, `tasks.md`).
- State schema workflow and progress (`N/M` complete).
- Note any artifacts that were skipped or blocked.

### 2. Technical Analysis
- Summarize the architecture and design decisions documented in `design.md`.
- Highlight any new modules, files, or public APIs introduced.
- Call out dependencies on other cards, changes, or external systems.
- Identify performance, security, or scalability implications if relevant.

### 3. Recommendations
- Suggest the most appropriate apply strategy:
  - `interactive apply` — recommended when the change is large, risky, or touches critical paths.
  - `auto apply` — only when the change is small, well-understood, and tests are comprehensive.
  - `apply only` — when the user wants to execute tasks manually without further agent assistance.
- Note any tasks that should be grouped into coherent commits.
- Advise on whether additional exploration or design refinement is needed before apply.

### 4. Gaps, Warnings, and Open Questions
- List any spec scenarios that lack test coverage or are marked as out of scope.
- Flag any warnings from `verify-report.md` or from the design review (e.g., spec divergence, tech-debt, backward-compatibility concerns).
- Pose explicit open questions that the user or reviewer should answer before apply.
- Highlight any assumptions made during artifact creation that may need validation.

### 5. Next Steps
- Offer clear options: `interactive apply`, `auto apply`, or `apply only`.
- Additional option: `refine artifacts` if the user wants to revisit specs, design, or tasks before applying.
- If the user chooses to refine, return to the relevant artifact phase (`specs`, `design`, or `tasks`).
- Always announce the SDD cycle mode (`interactive`, `auto-artifacts`, or `auto`) before presenting options.

## User-Facing Result

Report phase, status, artifacts, key findings, blockers, and next suggested action. Keep routine phase chatter short; details belong in the artifact or handoff.
