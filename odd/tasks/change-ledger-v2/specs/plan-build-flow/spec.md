# Delta for plan-build-flow

Supersedes the plan-before-build precondition recorded in the archived
`2026-11-24-plan-build-openspec-odd-align` delta for `openspec` mode. The write
gate stops authorizing and starts inferring; `none` becomes a claim that is only
refuted by objective facts.

## ADDED Requirements

### Requirement: Tier is inferred at the write gate

The write gate (`plan-build-gate`, `work-start`) SHALL derive the change tier from
the artifacts present, in both execution modes, as `none` (no plan and no delta),
`light` (plan only, or delta only) and `standard` (plan and delta). It MUST NOT
require a plan file to allow a production write in any mode, and it MUST NOT
require a declaration, ceremony or explicit authorization to allow one. An absent
or unreadable artifact SHALL resolve to `none`, never to an error.

The merge-time guardian SHALL compute the same three values in ODD mode as in
OpenSpec mode, so that a refutation escalates identically in both.

#### Scenario: An OpenSpec production write with no plan is allowed

- **WHEN** OpenSpec mode is active, no change folder or no `tasks.md` exists, and
  the agent writes a file under a production directory
- **THEN** the gate exits `0` with no blocker, and the inferred tier is `none`

#### Scenario: An ODD production write with no plan stays allowed

- **WHEN** ODD mode is active and a production file is written without a plan
- **THEN** the gate exits `0` with no blocker, exactly as before this change

#### Scenario: The inferred tier is reported, not enforced

- **WHEN** any production write is graded
- **THEN** the gate reports the inferred tier and every refutation it evaluated,
  and never treats the tier itself as a reason to refuse

### Requirement: Only objective refutations refuse, naming the rule and the exits

A write SHALL be refused only when one of these refutations is proven from facts:

- **R1** a canonical spec is written by hand without the delta that should carry
  the rule (today's `odd/specs/` guard, preserved);
- **R2** a project-declared high-risk path changes without the evidence the project
  requires;
- **R3** the diff exceeds the configured size or module threshold;
- **R4** a contract others consume changes (public command, flag, config format,
  export);
- **R5** a plan declares `S#` rows without the verification rows that back them.

Every refusal MUST name the rule, the fact that proves it, and the exits that
resolve it: write the missing plan or delta, record an exemption decision, or
escalate. A refusal MUST NOT tell the user to request authorization, and MUST NOT
claim that the change is small or large on its own authority.

#### Scenario: A hand write to a canonical spec names R1

- **WHEN** ODD mode is active and a file under `odd/specs/` is written without a
  paired delta
- **THEN** the gate exits `2` and the message names R1, the missing delta path and
  the promotion command as the exit

#### Scenario: A contract change names R4

- **WHEN** a change removes or renames a public flag or a config key
- **THEN** the gate exits `2` and the message names R4 and the exit that records
  the decision, even when a plan exists

### Requirement: The no-plan declaration is inert and then removed

`ai-specs changes start --none` SHALL stop being a gate precondition. During one
version it SHALL remain callable, print a deprecation notice and record nothing
that the gate reads; after that version it SHALL be removed. The existing
per-branch value (`branch.<branch>.aiSpecsCeremony`) SHALL be ignored by the gate
and MUST NOT unlock or relock any transition.

Removal SHALL NOT leave any worktree without a legal path. The two uses the verb
serves today SHALL be served explicitly: a change that owes no plan needs no
declaration at all (the tier is inferred), and a worktree that is not a change is
classified as a workstream from facts, with `changes workstream` available for the
deliberate case. No refusal and no deprecation message SHALL suggest that a
worktree must declare something before it may be written.

#### Scenario: A leftover declaration changes nothing

- **WHEN** a branch still carries `aiSpecsCeremony = none` from an earlier release
- **THEN** the write gate behaves exactly as on a branch without it, and no
  deprecation notice is emitted for a pre-existing value

#### Scenario: An audit worktree is never asked to declare

- **WHEN** a worktree that is not a change is written to
- **THEN** no message mentions a plan, a declaration or `changes start --none`, and
  the write is allowed

#### Scenario: A new declaration warns

- **WHEN** a user runs `ai-specs changes start --none`
- **THEN** the command explains that the tier is inferred now and points at
  `changes workstream` for a worktree that is not a change, and exits successfully
  without affecting the gate
