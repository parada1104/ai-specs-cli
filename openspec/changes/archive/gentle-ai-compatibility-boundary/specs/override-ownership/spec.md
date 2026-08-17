# Delta for override-ownership

## ADDED Requirements

### Requirement: Gate hook provenance and refresh backup

For generated runtime hook scripts (for example worktree gate hooks), the CLI SHALL record a provenance baseline of the exact normalized bytes it last rendered, using the managed lock record mechanism or the per-project cache. Sync MUST classify each gate hook against that baseline:

- baseline matches current bytes → unmodified; sync MAY force-update the gate and update the baseline
- baseline present but current bytes differ → user-modified; sync MUST preserve the gate with a warning
- no baseline → unknown provenance; sync MUST preserve the gate with a warning, and a baseline SHALL be recorded only when the CLI itself renders the gate

An explicit refresh or replacement of a customized gate hook MUST first save the exact pre-refresh bytes to a cache-only immutable backup in the per-project CLI cache, keyed deterministically by project cache key and project-relative target, with a content-hash suffix or equivalent so repeated refreshes cannot overwrite the original snapshot. Backup and baseline/lock updates MUST be atomic (all-or-nothing) and collision-safe; on any failure the gate MUST remain unchanged. Gate provenance, refresh, and backup MUST NOT depend on any external provider and MUST behave identically when external orchestration is absent or disabled.

#### Scenario: Baseline match refreshes the generated gate

- GIVEN a gate whose recorded baseline matches its current bytes
- AND the current would-write catalog bytes differ
- WHEN ordinary sync runs
- THEN the gate is updated and the baseline is updated
- AND no user-modified warning is emitted

#### Scenario: Byte mismatch preserves the customized gate

- GIVEN a gate whose current bytes differ from its recorded baseline
- WHEN ordinary sync runs
- THEN the gate remains unchanged
- AND sync warns naming the path with refresh guidance

#### Scenario: Missing provenance preserves the gate

- GIVEN a materialized gate with no recorded baseline
- WHEN ordinary sync runs
- THEN the gate is preserved with a warning
- AND no baseline is seeded because the CLI did not render the gate

#### Scenario: Explicit refresh backs up pre-refresh bytes immutably

- GIVEN a customized gate and an explicit user refresh
- WHEN the refresh runs
- THEN the exact pre-refresh bytes are saved to the cache-only immutable backup
- AND the gate is replaced with current content and the baseline is updated

#### Scenario: Repeated refresh is collision-safe

- GIVEN an earlier explicit refresh created a backup
- AND the gate is customized again and explicitly refreshed
- WHEN a second explicit refresh runs
- THEN the original snapshot remains intact
- AND the new pre-refresh bytes are backed up under a distinct path without collision

#### Scenario: Failed backup or lock write leaves the gate unchanged

- GIVEN an explicit refresh whose backup or baseline write fails
- WHEN the refresh runs
- THEN the gate remains unchanged
- AND lock/cache state is not partially updated

#### Scenario: Absent or disabled external orchestration keeps behavior identical

- GIVEN external orchestration is absent or disabled
- WHEN ordinary sync or an explicit refresh runs
- THEN gate provenance and backup behavior is identical
- AND no external provider is invoked

## MODIFIED Requirements

### Requirement: Update policy by category

Governed targets SHALL honor `auto`, `confirm`, or `never-force`. Templates
default to `auto` unless `update_policy` is set on the recipe template entry.
Runtime hook scripts SHALL follow a gate provenance policy distinct from
template overrides: a recorded baseline matching current bytes is treated as
unmodified and MAY be force-updated by ordinary sync; a byte mismatch or missing
baseline is treated as user-modified or unknown and MUST be preserved with a
warning; an explicit refresh MAY replace a customized gate only after its exact
pre-refresh bytes are saved to the cache-only immutable backup (see *Gate hook
provenance and refresh backup*).
(Previously: runtime hook scripts were not governed overrides and were rewritten
unconditionally by sync.)

| Policy | On `managed_stale` |
|---|---|
| `auto` | overwrite with current catalog render and update lock |
| `confirm` | preserve and WARN with refresh instructions in v1 |
| `never-force` | preserve and WARN with refresh instructions |

#### Scenario: Auto policy force-updates managed stale

- **GIVEN** a managed-stale target with effective policy `auto`
- **WHEN** sync runs
- **THEN** the target is overwritten with current catalog content
- **AND** the lock hash is updated without a user-modified warning

#### Scenario: User-modified is never force-updated

- **GIVEN** a target whose bytes differ from its managed hash
- **WHEN** sync runs
- **THEN** the target remains unchanged
- **AND** sync emits a non-blocking warning naming the path and indicating
  user modification with `rm <target> && ai-specs sync` refresh guidance

#### Scenario: Customized gate is preserved instead of rewritten

- **GIVEN** a gate hook whose current bytes differ from its recorded baseline
- **WHEN** ordinary sync runs
- **THEN** the gate remains unchanged
- **AND** sync warns with `rm <target> && ai-specs sync` refresh guidance
- **AND** the gate is not treated as a template override

### Requirement: Doctor alignment

Doctor SHALL use the same classifier: warn for user-modified and diverged
untracked targets, and for managed-stale targets under `confirm` or
`never-force`; it SHALL remain quiet for managed-current and managed-stale
`auto` targets.

For gate hooks, doctor SHALL warn when the current bytes differ from the
recorded baseline or when no baseline exists, and SHALL remain quiet for gates
whose baseline matches.
(Previously: doctor classified template overrides only and did not cover gate
provenance.)

#### Scenario: Doctor warns on a customized gate

- **GIVEN** a gate hook whose current bytes differ from its recorded baseline
- **WHEN** doctor runs
- **THEN** it warns naming the path and indicating user modification
- **AND** it stays quiet for gates whose baseline matches current bytes

### Requirement: Policy documentation

Project or recipe documentation SHALL describe `auto`, `confirm`, and
`never-force` behavior, explicit `rm` plus sync refresh, and gate hook
provenance: baseline recording, preserve-on-mismatch or missing-provenance,
explicit refresh, and the cache-only immutable backup. Runtime hook scripts are
no longer rewritten unconditionally.
(Previously: docs stated that runtime hook scripts are always rewritten by the
CLI.)

#### Scenario: Docs describe gate provenance and refresh

- **GIVEN** the override-ownership and worktree-flow recipe documentation
- **WHEN** a reader consults hook update behavior
- **THEN** the docs describe baseline recording, preserve-on-mismatch or
  missing provenance, explicit refresh, and the cache-only immutable backup
- **AND** they do not claim hooks are always rewritten unconditionally
