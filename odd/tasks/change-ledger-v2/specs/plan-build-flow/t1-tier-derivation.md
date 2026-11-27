# Delta for plan-build-flow

The first unit of the change-ledger-v2 redesign makes the tier one derivation that
both execution modes share. The write-gate half of the redesign (inference,
refutations, the inert no-plan declaration) arrives with its own unit and its own
delta.

## ADDED Requirements

### Requirement: One tier derivation, both modes

The merge-time guardian SHALL compute a change's tier with one shared derivation in
both execution modes: `none` when the change carries neither a plan nor a delta,
`light` when it carries exactly one of them, and `standard` when it carries both.
ODD mode MUST NOT report a tier value of its own, so that a change is graded the
same way and a refutation escalates identically in both modes. An artifact that
cannot be observed SHALL never be reported as `light` or `standard`.

#### Scenario: An ODD plan-only change is graded light

- **WHEN** ODD mode is active and the branch-paired plan exists with no delta
- **THEN** the guardian reports the tier `light` and keeps its verdict, blockers and
  notes otherwise unchanged

#### Scenario: An ODD change with a plan and a promoted delta is graded standard

- **WHEN** ODD mode is active, the plan exists and a delta has been promoted into
  `odd/specs/`
- **THEN** the guardian reports the tier `standard`

#### Scenario: A detached HEAD never claims a tier

- **WHEN** ODD mode is active and HEAD is detached, so no plan can be paired
- **THEN** the guardian still blocks with its pairing blocker and reports the tier
  `none`, because no artifact was observed
