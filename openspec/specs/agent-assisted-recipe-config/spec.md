# Delta: agent-assisted-recipe-config

## Purpose

Define the agent-assisted recipe configuration capability: a natural-language
entry path that produces grounded recommendations, applies canonical config
idempotently while preserving existing configuration and overrides, runs and
verifies sync with explicit partial-failure semantics, and reports assumptions,
drift, and version/synchronization gaps through a stable structured report.

The deterministic parts of the flow are owned by a minimum non-interactive
helper so they can be asserted without an LLM; the conversational parts stay in
agent-facing literacy. Behavior is evidenced in two tiers: deterministic unit
tests and real-runtime agent evals.

## Non-Goals

- Override lock provenance, force-update of user-modified overrides, or
  per-artifact governance categories (sibling change / Trello #63).
- Replacing or altering the human interactive `configure-recipes` wizard.
- Adding a JSON output mode to `ai-specs doctor`.
- MCP wrapping of the full CLI.
- Silent mutation during read-only `ai-specs recipe init`.
- Authoring `init.md` for recipes that lack one.
- Browser- or UI-driven evaluation, or a new eval platform.

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

### Requirement: Deterministic non-interactive configure helper

The harness SHALL provide a non-interactive helper that performs inspection and
apply for assisted configuration without requiring a TTY. The helper SHALL
support a machine-readable output mode whose output is a pure function of
project state and arguments.

The helper's machine-readable output SHALL NOT contain wall-clock timestamps,
durations, process identifiers, hostnames, or absolute host paths; paths SHALL
be expressed relative to the project root. Collections SHALL be emitted in a
documented deterministic order, and the document SHALL carry an integer schema
version.

The helper SHALL use distinct exit codes for: success, apply/sync failure,
usage error, request rejected before any write, and preflight refusal before
any write. Rejection and refusal SHALL guarantee that no file was modified.

The existing interactive `ai-specs configure-recipes` wizard SHALL retain its
current behavior.

#### Scenario: Repeated inspection is byte-identical

- **GIVEN** an unchanged project
- **WHEN** the helper's inspection is invoked twice in machine-readable mode
- **THEN** the two outputs SHALL be byte-identical

#### Scenario: Rejected request writes nothing

- **GIVEN** a requested key that is absent from the recipe config schema, or a
  value that violates the schema type or enum
- **WHEN** apply is invoked
- **THEN** the helper SHALL exit with the rejection code
- **AND** the project manifest SHALL remain byte-identical

#### Scenario: Interactive wizard behavior preserved

- **GIVEN** the interactive `ai-specs configure-recipes` flow
- **WHEN** it is used after this capability ships
- **THEN** its prompts and resulting configuration behavior SHALL be unchanged
  apart from inheriting comment and no-op preservation from the shared write
  path

### Requirement: Grounded recommendation before apply

Before mutating the manifest, the assisted flow SHALL produce a reviewable
recommendation grounded in inspected repository and project state. Grounding
signals SHALL include, when available: existing `[recipes.<id>.config]` values,
the recipe config schema (required/optional/enum/defaults/help text), and
relevant repository topology or MCP/dependency signals for that recipe. The
recommendation SHALL state assumptions explicitly.

#### Scenario: Recommendation cites existing config and schema

- **GIVEN** a project with an enabled recipe that already has some config keys
- **WHEN** the assisted flow recommends updates
- **THEN** the recommendation SHALL distinguish proposed changes from keys left
  unchanged
- **AND** SHALL NOT invent keys absent from the recipe config schema

#### Scenario: Stop before apply when unapproved

- **GIVEN** a recommendation has been produced
- **AND** the user has not approved apply
- **WHEN** the agent continues the flow
- **THEN** the agent SHALL NOT write `ai-specs/ai-specs.toml` until approval

### Requirement: Topology grounding without a recipe init contract

Topology grounding SHALL be derived from the recipe config schema and from
repository topology detection. A per-recipe `init.md` contract SHALL NOT be a
precondition for topology-grounded recommendations; when an `init.md` exists it
MAY be used as additional material.

When topology detection cannot distinguish a topology value that the schema
offers, the flow SHALL surface that value as an explicit question to the user
rather than asserting the detected default. When detection degrades (for
example a git failure resolving to the standalone default), the flow SHALL
report the degradation as an assumption rather than as an observed fact.

#### Scenario: Topology-aware grounding for a recipe with no init.md

- **GIVEN** a recipe that declares a topology config field and ships no
  `init.md`
- **AND** the repository has inspectable topology signals (`.gitmodules` /
  initialized submodules)
- **WHEN** the assisted flow recommends configuration
- **THEN** the recommendation SHALL incorporate the resolved or detected
  topology signal and the evidence it came from
- **AND** SHALL NOT hardcode a single consumer repository's paths

#### Scenario: Undetectable topology value is asked, not assumed

- **GIVEN** a recipe whose topology enum includes a value that detection never
  resolves to on its own
- **AND** the repository shows no submodule signals
- **WHEN** the assisted flow recommends configuration
- **THEN** the recommendation SHALL present that value as an explicit user
  question
- **AND** SHALL NOT silently assert the detected default as the user's intent

### Requirement: Idempotent canonical config apply

When apply is approved, the assisted flow SHALL update canonical per-recipe
configuration under `[recipes.<id>.config]` idempotently. Unmentioned existing
keys SHALL be preserved.

Applying a recommendation whose values already equal the effective
configuration SHALL leave the manifest **byte-identical**, SHALL report a no-op
status, and SHALL report an empty set of changed keys. Value equality SHALL be
evaluated on parsed TOML values, not on their textual form.

#### Scenario: First apply writes recommended keys

- **GIVEN** an approved recommendation with schema-valid key/value pairs
- **WHEN** apply runs
- **THEN** those keys SHALL appear under `[recipes.<id>.config]`
- **AND** unrelated manifest sections SHALL remain intact

#### Scenario: No-op apply leaves the manifest byte-identical

- **GIVEN** `[recipes.<id>.config]` already contains the approved values,
  written with different but equivalent formatting (for example single-quoted
  strings or no spaces around `=`)
- **WHEN** apply runs
- **THEN** the manifest bytes SHALL be unchanged
- **AND** the reported status SHALL be the no-op status with no changed keys

#### Scenario: Unmentioned keys preserved

- **GIVEN** existing `[recipes.<id>.config]` contains key `keep_me`
- **AND** the recommendation does not mention `keep_me`
- **WHEN** apply runs
- **THEN** `keep_me` SHALL still be present after apply with its original bytes

### Requirement: Comment preservation on config write

The surgical config write path SHALL preserve manifest comments, including a
trailing inline comment on a line whose key value is replaced. Comment
detection SHALL be TOML-string aware: a `#` occurring inside a basic or literal
string SHALL NOT be treated as the start of a comment. A comment SHALL NOT be
invented for a key that had none.

When a key's existing value spans multiple lines, the write path SHALL reject
the request rather than rewrite the value, so that no comment or value text is
silently lost.

#### Scenario: Inline comment survives value replacement

- **GIVEN** `[recipes.<id>.config]` contains
  `integration_branch = "main"  # team decision`
- **WHEN** apply changes `integration_branch` to `development`
- **THEN** the resulting line SHALL contain the new value
- **AND** SHALL still contain `# team decision`

#### Scenario: Hash inside a string is not a comment

- **GIVEN** a config value whose string content contains a `#` character
- **WHEN** another key in the same block is replaced
- **THEN** the value containing `#` SHALL remain intact and unmodified

#### Scenario: Own-line comments survive

- **GIVEN** `[recipes.<id>.config]` contains an own-line comment
- **WHEN** apply replaces a key in that block
- **THEN** the own-line comment SHALL remain present and unmodified

### Requirement: Preserve overrides

The assisted configure apply path SHALL NOT overwrite or delete project override
files under recipe override trees in order to "refresh" catalog content.
Suspected override drift MAY be reported; force-update policy is out of scope.

#### Scenario: Existing override file untouched

- **GIVEN** a consumer file under `ai-specs/recipes/<id>/overrides/` that differs
  from catalog content
- **WHEN** assisted configure apply + sync runs
- **THEN** that override file SHALL remain byte-identical unless an independent
  user-approved action outside this capability changes it

### Requirement: Preflight gate on CLI version policy before apply

Before writing the manifest, the assisted flow SHALL evaluate the project's CLI
version policy (the manifest `[tool]` pin, including a malformed policy).

When the installed CLI does not satisfy the policy — the condition under which
`ai-specs sync` aborts before performing any work — the flow SHALL refuse to
apply, SHALL exit with the preflight-refusal code, SHALL report a blocked
status with the reason, and SHALL leave the manifest byte-identical.

An explicit ignore-policy option MAY be provided; when used, the flow SHALL
record the bypass in the report, forward it to sync, and state in the closing
report that the version policy was bypassed.

This gate is ordered before apply so the flow can never leave an edited
manifest that sync will refuse to process.

#### Scenario: Pin violation blocks before any write

- **GIVEN** a manifest whose `[tool]` policy the installed CLI does not satisfy
- **WHEN** the assisted flow is asked to apply an approved recommendation
- **THEN** no file SHALL be modified
- **AND** the flow SHALL report a blocked status naming the policy violation
- **AND** SHALL NOT invoke sync

#### Scenario: Bypass is recorded, not hidden

- **GIVEN** the same project and an explicit ignore-policy option
- **WHEN** apply runs
- **THEN** the report SHALL record that the version policy was bypassed

### Requirement: Sync and verify after apply

The assisted flow SHALL run `ai-specs sync` for the project and verify the
outcome after a successful apply that changes project configuration. The flow
SHALL also surface health verification using `ai-specs doctor`.

Because sync stops at the first failing step and does not roll back earlier
writes, a failed sync following a successful manifest write SHALL be reported as
a **partial** outcome — distinct from both success and from a failure that
changed nothing. A partial outcome SHALL record the failing step, the sync exit
code, that no rollback occurred, and that the lock CLI version was not stamped.

The flow SHALL NOT describe a partial outcome as configured, synced, or
complete.

#### Scenario: Sync runs after approved apply

- **GIVEN** apply updated `[recipes.<id>.config]`
- **WHEN** the assisted flow continues with sync requested
- **THEN** it SHALL invoke `ai-specs sync` on the project path
- **AND** SHALL treat a non-zero sync exit as a failed flow

#### Scenario: Partial outcome after failed sync

- **GIVEN** apply wrote the manifest successfully
- **AND** a subsequent sync step fails
- **WHEN** the flow stops
- **THEN** the reported status SHALL be the partial status
- **AND** the report SHALL state that earlier sync writes were not rolled back
- **AND** SHALL state that the lock CLI version was not stamped
- **AND** SHALL NOT claim the project is fully configured

#### Scenario: Read-only recipe init remains non-mutating

- **GIVEN** a user runs only `ai-specs recipe init <id>`
- **WHEN** the command completes
- **THEN** the project manifest SHALL remain unmodified by that command
- **AND** that command alone SHALL NOT invoke sync
  (assisted configure apply is a distinct flow)

### Requirement: Structured closing report

The assisted flow SHALL end with a versioned, structured report containing at
least: report schema version, outcome status, applied changes (changed,
unchanged, and preserved keys), preflight version state, sync outcome, doctor
verification outcome, unresolved assumptions, configuration drift signals, and
version/synchronization gaps.

Doctor's health outcome SHALL be recorded from its exit status and its summary
counts. When those counts cannot be parsed, the report SHALL mark them as
unparsed rather than reporting zero findings.

Lock CLI version staleness (`.ai-specs.lock` `[meta].cli_version` differing from
the installed CLI) SHALL be reported as an informational gap, distinct from a
CLI version policy violation, because sync proceeds and restamps it.

#### Scenario: Report after successful configure

- **GIVEN** apply and sync succeeded
- **WHEN** the flow completes
- **THEN** the report SHALL contain every field named above
- **AND** SHALL list any assumptions that remained unresolved
- **AND** SHALL list preserved keys that were present and untouched

#### Scenario: Unparsed doctor summary is not silent success

- **GIVEN** doctor output whose summary line cannot be parsed
- **WHEN** the report is produced
- **THEN** the doctor counts SHALL be marked unparsed
- **AND** the report SHALL NOT state zero warnings or zero errors

#### Scenario: Lock staleness reported as a gap, not a block

- **GIVEN** `.ai-specs.lock` `[meta].cli_version` differs from the installed CLI
- **AND** the manifest `[tool]` policy is satisfied
- **WHEN** the assisted flow runs
- **THEN** the flow SHALL proceed
- **AND** the report SHALL include the staleness as an informational gap

### Requirement: Real-runtime evaluation of the assisted flow

The assisted configure behavior SHALL be evidenced by real-runtime agent evals
added as a new client on the repository's existing eval system, using any
supported agent CLI runtime. Evals SHALL be distinct from the deterministic
unit tests: unit tests gate merge, evals provide runtime evidence and SHALL NOT
be required to pass in the per-change test suite run.

The existing eval system SHALL remain unchanged: the scenario contract,
fixture model, assertions, pass criteria, isolation guarantees, and runner
contract SHALL NOT be altered by this capability. Additions to shared eval
support code SHALL be additive and SHALL NOT change the behavior of existing
callers or scenarios.

No specific runtime SHALL be mandated. Evals SHALL be runnable from a plain
shell using the existing runner and runtime-selection environment contract.

Evals SHALL execute against a disposable project fixture outside the repository
working tree and SHALL NOT mutate the repository or worktree they are launched
from. Recorded evidence SHALL identify at least the scenario, runtime, model,
trial, CLI version, and outcome so a run can be reproduced. When more than one
runtime is exercised, evidence SHALL be attributed per runtime rather than
aggregated into a single verdict.

Evaluation SHALL NOT introduce a new eval platform, runner, or scoring service,
and SHALL NOT depend on browser or UI automation.

#### Scenario: Evals excluded from the unit suite

- **GIVEN** the repository's standard test runner
- **WHEN** it discovers tests
- **THEN** the assisted-configure eval module SHALL NOT be collected
- **AND** the deterministic helper tests SHALL be collected

#### Scenario: Eval run does not mutate the source worktree

- **GIVEN** a live eval run of the assisted configure scenarios
- **WHEN** the run completes
- **THEN** all agent and CLI writes SHALL be confined to the disposable fixture
- **AND** the repository worktree SHALL contain no changes attributable to the
  run

#### Scenario: Approval gate holds at runtime

- **GIVEN** a natural-language configure request with no approval given
- **WHEN** the agent runs the assisted flow in a real runtime
- **THEN** the fixture's `ai-specs/ai-specs.toml` SHALL remain byte-identical

#### Scenario: End-to-end apply, sync, verify, report at runtime

- **GIVEN** a natural-language configure request with approval given
- **WHEN** the agent runs the assisted flow in a real runtime
- **THEN** the recipe configuration SHALL be updated
- **AND** sync SHALL have been run
- **AND** the closing report SHALL contain the structured report fields

#### Scenario: Existing eval system semantics untouched

- **GIVEN** the eval scenarios, fixtures, assertions, and runners that existed
  before this capability
- **WHEN** this capability's eval client is added
- **THEN** their definitions and outcomes SHALL be unchanged
- **AND** any shared eval support code added SHALL be additive only

### Requirement: Optional cross-runtime eval orchestration

Cross-runtime eval orchestration SHALL be optional. An orchestration layer MAY
be used to execute the existing eval runners across several real agent runtimes
and to aggregate the results for comparison; with no such layer present, the
same evals SHALL remain runnable directly and SHALL produce the same scenarios,
assertions, and verdicts.

The orchestration layer SHALL be limited to invoking the existing runners and
collecting their results. It SHALL NOT modify scenarios, prompts, fixtures,
assertions, isolation, trial rules, or pass criteria, and SHALL NOT re-judge or
override a runner's verdict. It SHALL NOT be a runtime, an eval runner, or a
prerequisite of the capability.

When multiple runtimes are exercised, results SHALL be reported per runtime so
that a runtime-specific behavioral difference is attributable.

#### Scenario: Evals run without the orchestration layer

- **GIVEN** an environment with no orchestration layer installed
- **WHEN** the operator invokes the eval runner directly
- **THEN** the scenarios, assertions, and verdicts SHALL be the same as when
  orchestration is used

#### Scenario: Cross-runtime divergence is attributable

- **GIVEN** the same scenario executed on more than one runtime
- **AND** the scenario passes on one runtime and fails on another
- **WHEN** results are reported
- **THEN** the outcome SHALL be recorded separately per runtime
- **AND** the failure SHALL NOT be masked by an aggregate verdict

#### Scenario: Orchestration does not alter eval semantics

- **GIVEN** an orchestrated multi-runtime eval run
- **WHEN** results are collected
- **THEN** the scenario definitions and pass criteria SHALL be unchanged
- **AND** the orchestration layer SHALL NOT re-judge a runner's verdict

### Requirement: Documentation and validation coverage

The assisted configure behavior SHALL be documented in agent-facing literacy
and/or project docs, including the two evidence tiers and how to run each, and
SHALL be covered by the repository's existing validation conventions.

#### Scenario: Literacy documents the flow

- **GIVEN** the shipped harness recipes/lifecycle literacy skills
- **WHEN** an agent loads the relevant skill for recipe configuration
- **THEN** the skill SHALL describe the inspect → recommend → apply →
  sync/verify → report sequence
- **AND** SHALL state the approval gate, preserve-config/overrides, and
  no-secret-literal rules

#### Scenario: Validation conventions exercised

- **GIVEN** the change's tests for this capability
- **WHEN** `./tests/run.sh` and `./tests/validate.sh` run in apply/verify
- **THEN** they SHALL pass with the new coverage included
