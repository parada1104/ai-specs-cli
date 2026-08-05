# Delta: agent-assisted-recipe-config

## Purpose

Define the agent-assisted recipe configuration capability: a natural-language
entry path that produces grounded recommendations, applies canonical config
idempotently while preserving existing configuration and overrides, runs and
verifies sync, and reports assumptions, drift, and version/synchronization gaps.

## Non-Goals

- Override lock provenance, force-update of user-modified overrides, or
  per-artifact governance categories (sibling change / Trello #63).
- Replacing the human interactive `configure-recipes` wizard.
- MCP wrapping of the full CLI.
- Silent mutation during read-only `ai-specs recipe init`.

---

## ADDED Requirements

### Requirement: Natural-language entry to assisted configure

The harness SHALL provide an agent-facing assisted recipe configuration flow
reachable from natural-language intent (via always-on literacy skill guidance
and/or documented entry phrases). The flow SHALL identify target recipe id(s)
from user language, using `ai-specs recipe list` (or equivalent) when the id is
ambiguous.

#### Scenario: Clear recipe intent

- **GIVEN** a user request that names or uniquely implies a catalog recipe
  (e.g. configure worktree-flow for this repo)
- **WHEN** the assisted configure flow starts
- **THEN** the agent SHALL select that recipe id without requiring the user to
  edit TOML by hand

#### Scenario: Ambiguous recipe intent

- **GIVEN** a user request that could match multiple recipes
- **WHEN** the assisted configure flow starts
- **THEN** the agent SHALL disambiguate using catalog/list state before applying
  any manifest mutation

### Requirement: Grounded recommendation before apply

Before mutating the manifest, the assisted flow SHALL produce a reviewable
recommendation grounded in inspected repository and project state. Grounding
signals SHALL include, when available: existing `[recipes.<id>.config]` values,
the recipe config schema (required/optional/enum/defaults), and relevant
repository topology or MCP/dependency signals for that recipe. The
recommendation SHALL state assumptions explicitly.

#### Scenario: Recommendation cites existing config and schema

- **GIVEN** a project with an enabled recipe that already has some config keys
- **WHEN** the assisted flow recommends updates
- **THEN** the recommendation SHALL distinguish proposed changes from keys left
  unchanged
- **AND** SHALL NOT invent keys absent from the recipe config schema

#### Scenario: Topology-aware grounding when applicable

- **GIVEN** a recipe that declares topology-related config (e.g. `repo_topology`)
- **AND** the repository has inspectable topology signals (e.g. `.gitmodules` /
  submodule status)
- **WHEN** the assisted flow recommends configuration
- **THEN** the recommendation SHALL incorporate the resolved or detected topology
  signal (or state why it could not)
- **AND** SHALL NOT hardcode a single consumer repository's paths

#### Scenario: Stop before apply when unapproved

- **GIVEN** a recommendation has been produced
- **AND** the user has not approved apply
- **WHEN** the agent continues the flow
- **THEN** the agent SHALL NOT write `ai-specs/ai-specs.toml` until approval

### Requirement: Idempotent canonical config apply

When apply is approved, the assisted flow SHALL update canonical per-recipe
configuration under `[recipes.<id>.config]` idempotently. Re-applying an
identical approved recommendation SHALL NOT introduce spurious churn.
Unmentioned existing keys SHALL be preserved. Manifest comments SHALL be
preserved when using the surgical config write path.

#### Scenario: First apply writes recommended keys

- **GIVEN** an approved recommendation with schema-valid key/value pairs
- **WHEN** apply runs
- **THEN** those keys SHALL appear under `[recipes.<id>.config]`
- **AND** unrelated manifest sections SHALL remain intact

#### Scenario: Re-apply is idempotent

- **GIVEN** the manifest already matches the approved recommendation
- **WHEN** apply runs again with the same values
- **THEN** the effective config SHALL remain equivalent
- **AND** the flow SHALL NOT require destructive rewrite of the whole manifest

#### Scenario: Unmentioned keys preserved

- **GIVEN** existing `[recipes.<id>.config]` contains key `keep_me`
- **AND** the recommendation does not mention `keep_me`
- **WHEN** apply runs
- **THEN** `keep_me` SHALL still be present after apply

### Requirement: Preserve overrides

The assisted configure apply path SHALL NOT overwrite or delete project override
files under recipe override trees in order to "refresh" catalog content. Suspected
override drift MAY be reported; force-update policy is out of scope.

#### Scenario: Existing override file untouched

- **GIVEN** a consumer file under `ai-specs/recipes/<id>/overrides/` that differs
  from catalog content
- **WHEN** assisted configure apply + sync runs
- **THEN** that override file SHALL remain byte-identical unless an independent
  user-approved action outside this capability changes it

### Requirement: Sync and verify after apply

After a successful apply that changes project configuration, the assisted flow
SHALL run `ai-specs sync` for the project and verify the outcome (non-zero sync
exit is a failed flow). The flow SHALL also surface health/version verification
using `ai-specs doctor` and/or lock provenance (`cli_version`) consistent with
existing project conventions.

#### Scenario: Sync runs after approved apply

- **GIVEN** apply updated `[recipes.<id>.config]`
- **WHEN** the assisted flow continues
- **THEN** it SHALL invoke `ai-specs sync` on the project path
- **AND** SHALL treat sync failure as flow failure

#### Scenario: Verification surfaces version gap

- **GIVEN** `.ai-specs.lock` `[meta].cli_version` differs from the running CLI
  in a way doctor already reports
- **WHEN** the assisted flow verifies
- **THEN** the closing report SHALL include that version/synchronization gap

#### Scenario: Read-only recipe init remains non-mutating

- **GIVEN** a user runs only `ai-specs recipe init <id>`
- **WHEN** the command completes
- **THEN** the project manifest SHALL remain unmodified by that command
- **AND** that command alone SHALL NOT invoke sync
  (assisted configure apply is a distinct flow)

### Requirement: Closing report of assumptions and drift

The assisted flow SHALL end with a report that includes: summary of applied
changes (or none), unresolved assumptions, configuration drift signals observed
(if any), version/synchronization gaps observed (if any), and sync/verify
outcome.

#### Scenario: Report after successful configure

- **GIVEN** apply and sync succeeded
- **WHEN** the flow completes
- **THEN** the agent SHALL present the closing report fields above
- **AND** SHALL list any assumptions that remained unresolved

#### Scenario: Report on partial failure

- **GIVEN** apply succeeded but sync failed
- **WHEN** the flow stops
- **THEN** the report SHALL state the sync failure
- **AND** SHALL NOT claim the project is fully configured

### Requirement: Documentation and validation coverage

The assisted configure behavior SHALL be documented in agent-facing literacy
and/or project docs, and covered by the repository's existing validation
conventions (focused tests and/or suite runners used by this project).

#### Scenario: Literacy documents the flow

- **GIVEN** the shipped harness recipes/lifecycle literacy skills
- **WHEN** an agent loads the relevant skill for recipe configuration
- **THEN** the skill SHALL describe the inspect → recommend → apply →
  sync/verify → report sequence
- **AND** SHALL state preserve-config/overrides and no-secret-literal rules

#### Scenario: Validation conventions exercised

- **GIVEN** the change's tests for this capability
- **WHEN** `./tests/run.sh` and `./tests/validate.sh` run in apply/verify
- **THEN** they SHALL pass with the new coverage included
