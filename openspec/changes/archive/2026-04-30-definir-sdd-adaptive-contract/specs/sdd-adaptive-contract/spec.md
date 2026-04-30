# Spec: sdd-adaptive-contract

## ADDED Requirements

### Requirement: Ceremony Levels

The system SHALL define exactly four ceremony levels with the following names and classification criteria:

- `trivial`: changes limited to typographical errors, copy edits, minor CSS adjustments, internal renames without behavior change, or cleanup that does not alter observable system behavior.
- `local_fix`: localized bug fixes that restore intended behavior without changing system intent, user-facing contracts, or domain rules.
- `behavior_change`: changes that produce observable differences for users or downstream consumers, including but not limited to validation rules, permission checks, state transitions, API response shapes, billing calculations, notifications, or small domain rules.
- `domain_change`: changes that introduce new capabilities, modify significant business rules, span multiple modules, require data model migrations, affect security/auth/payment flows, or constitute architectural decisions.

#### Scenario: Typo correction classified as trivial

- **WHEN** a change consists solely of correcting a typographical error in a user-visible string or comment
- **THEN** the system SHALL permit classification as `trivial`

#### Scenario: Localized null-pointer fix classified as local_fix

- **WHEN** a change fixes a null-pointer or off-by-one error in a single function without altering input validation or output contracts
- **THEN** the system SHALL permit classification as `local_fix`

#### Scenario: Validation rule change classified as behavior_change

- **WHEN** a change adds, removes, or modifies an input validation rule that affects whether user requests are accepted or rejected
- **THEN** the system SHALL require classification as `behavior_change` or higher

#### Scenario: New capability classified as domain_change

- **WHEN** a change introduces a new capability module, cross-module integration, or modifies the data model persisted by the system
- **THEN** the system SHALL require classification as `domain_change`

---

### Requirement: Artifacts per Level

The system SHALL require the following SDD artifact sets for each ceremony level:

- `trivial`: no SDD artifacts are required.
- `local_fix`: code changes and corresponding automated tests; no spec artifacts are required unless existing specs are factually incorrect.
- `behavior_change`: existing specs MUST be updated or extended with new scenarios, plus code changes and corresponding automated tests; `proposal.md` and `design.md` are NOT required.
- `domain_change`: the full SDD artifact set MUST be produced: `proposal.md`, delta specs, `design.md` (if applicable), `tasks.md`, apply evidence, verification report, and archive report.

#### Scenario: Trivial change needs no artifacts

- **WHEN** a change is classified as `trivial`
- **THEN** the system SHALL NOT require the presence of any SDD artifact files

#### Scenario: Local fix requires tests but not specs

- **WHEN** a change is classified as `local_fix`
- **THEN** the system SHALL require code changes and automated tests, and SHALL NOT require new spec artifacts unless existing specs contain errors

#### Scenario: Behavior change requires updated specs and tests

- **WHEN** a change is classified as `behavior_change`
- **THEN** the system SHALL require updated or added scenarios in existing specs, plus code changes and automated tests, and SHALL NOT require `proposal.md` or `design.md`

#### Scenario: Domain change requires full SDD cycle

- **WHEN** a change is classified as `domain_change`
- **THEN** the system SHALL require `proposal.md`, delta specs, `tasks.md`, apply evidence, verification report, and archive report, and SHALL require `design.md` when the change involves architecture or multi-module interaction

---

### Requirement: Declarative Configuration

The system SHALL support a declarative `sdd.decision_matrix` section in `openspec/config.yaml`. Each level in the matrix MUST map to a configuration object containing:

- `artifacts`: a list of required artifact filenames or identifiers for that level; an empty list means no artifacts are required.
- `worktree_required`: a boolean indicating whether a dedicated git worktree is mandatory for that level.
- `proposal_required`: a boolean indicating whether `proposal.md` is mandatory for that level.
- `design_required`: a boolean indicating whether `design.md` is mandatory for that level.

#### Scenario: decision_matrix defines all four levels

- **WHEN** `openspec/config.yaml` contains a `sdd.decision_matrix` entry with keys `trivial`, `local_fix`, `behavior_change`, and `domain_change`
- **THEN** the system SHALL accept the configuration as structurally valid

#### Scenario: decision_matrix level maps to artifact list and flags

- **WHEN** a level entry in `decision_matrix` declares `artifacts`, `worktree_required`, `proposal_required`, and `design_required`
- **THEN** the system SHALL enforce those declarations when validating a change of that level

---

### Requirement: Recipe Threshold

The system SHALL allow a `recipe.toml` file to declare an optional `sdd.threshold` field. When present, the field sets the minimum ceremony level that any change processed through that recipe MUST satisfy. Valid values SHALL be exactly the four ceremony level names: `trivial`, `local_fix`, `behavior_change`, `domain_change`.

#### Scenario: recipe.toml declares threshold

- **WHEN** a `recipe.toml` contains `sdd.threshold = "behavior_change"`
- **THEN** the system SHALL enforce that any change processed by that recipe is classified at least as `behavior_change`

#### Scenario: invalid threshold value rejected

- **WHEN** a `recipe.toml` contains `sdd.threshold` with a value that is not one of the four defined ceremony levels
- **THEN** the system SHALL reject the recipe configuration and report an error indicating the invalid threshold

---

### Requirement: Configuration Validation

The system SHALL validate the following invariants:

- `sdd.decision_matrix` MUST contain exactly the four ceremony levels; absence of any level is an error.
- `sdd.threshold` in `recipe.toml`, when present, MUST be a known ceremony level.
- If a change is classified as `trivial` or `local_fix` but the effective threshold (from recipe or project default) forces a higher level of formality, the system SHALL emit a warning that classifies the discrepancy.

#### Scenario: Missing level in decision_matrix fails validation

- **WHEN** `openspec/config.yaml` defines `sdd.decision_matrix` without one of the four required levels
- **THEN** the system SHALL report a validation error identifying the missing level

#### Scenario: Unknown threshold in recipe fails validation

- **WHEN** `recipe.toml` declares `sdd.threshold` with a misspelled or unknown level name
- **THEN** the system SHALL report a validation error identifying the unknown level

#### Scenario: Low-impact change forced to high formality emits warning

- **WHEN** a change is classified as `trivial` or `local_fix` and the effective threshold is `behavior_change` or `domain_change`
- **THEN** the system SHALL emit a warning describing the mismatch between the change classification and the required formality

---

### Requirement: Decision Skill

The system SHALL provide an agent skill named `openspec-sdd-decision` with the following obligations:

- The skill MUST inspect existing specs before classifying a change.
- The skill MUST report: the selected classification, the reasoning for that classification, the list of existing specs touched or updated, the code modules touched, and the test modules touched.

#### Scenario: Skill inspects specs before classification

- **WHEN** the `openspec-sdd-decision` skill is invoked for a pending change
- **THEN** the skill SHALL read existing spec files under `openspec/specs/` before emitting a classification

#### Scenario: Skill reports classification and rationale

- **WHEN** the `openspec-sdd-decision` skill completes its analysis
- **THEN** the skill SHALL output the ceremony level classification and a concise reasoning paragraph justifying the level

#### Scenario: Skill reports touched artifacts

- **WHEN** the `openspec-sdd-decision` skill completes its analysis
- **THEN** the skill SHALL output a structured list of: specs touched, code files touched, and test files touched
