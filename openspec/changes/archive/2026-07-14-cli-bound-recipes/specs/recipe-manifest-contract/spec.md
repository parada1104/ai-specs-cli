# Spec delta: recipe-manifest-contract

## MODIFIED Requirements

### Requirement: Recipe instance declaration

`[recipes.<id>]` SHALL require `enabled` only. `version` is not required. Sync SHALL materialize the CLI catalog version with no pin fail-close. Legacy `version` keys SHALL be ignored with a WARN and MUST NOT block sync. Floating or `min_version` pins are not supported.

(Previously: required exact pin; fail-closed on catalog mismatch.)

#### Scenario: No version

- GIVEN an enabled recipe with no `version` key
- WHEN sync runs
- THEN catalog content is materialized successfully

#### Scenario: Legacy WARN

- GIVEN an enabled recipe with a stale `version` key
- WHEN sync runs
- THEN a WARN is emitted
- AND sync succeeds with current catalog content

#### Scenario: Disabled and unknown recipes

- GIVEN a disabled recipe
- WHEN sync runs
- THEN materialization is skipped
- GIVEN an unknown recipe id
- WHEN sync runs
- THEN sync fails with not-found behavior

### Requirement: Init deltas avoid duplicates

Init and config write paths SHALL update existing `[recipes.<id>]` and `.config` tables in place without creating duplicates. These paths MUST NOT write `version`.

(Previously: could change or require `version`.)

#### Scenario: In-place update

- GIVEN an existing `[recipes.<id>]` table
- WHEN init or config write runs
- THEN the table is updated in place
- AND no duplicate tables are created
- AND no `version` key is written

## ADDED Requirements

### Requirement: CLI catalog without pin ceremony

After a CLI upgrade, enabled recipes SHALL sync to the new catalog without requiring a toml edit.

#### Scenario: Post-upgrade

- GIVEN enabled recipes and an upgraded CLI
- WHEN sync runs without toml changes
- THEN new catalog content is materialized
