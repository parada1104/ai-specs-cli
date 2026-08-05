# override-ownership Specification

## Purpose

Define ownership classification and update governance for CLI-seeded override
targets, primarily `[[provides.templates]]` entries with
`condition = "not_exists"`, using scoped `[managed.*]` lock records so sync can
refresh stale managed files without overwriting user modifications.

## Requirements

### Requirement: Managed lock record on CLI write

When the CLI writes a governed override target, it SHALL upsert
`[managed."<project-relative-path>"]` in `ai-specs/.ai-specs.lock` with `sha256`
equal to the normalized bytes written after placeholder substitution. It SHOULD
also record `recipe`, `source`, `kind`, and effective `policy` when known.

#### Scenario: Fresh seed records managed hash

- **GIVEN** no file at a governed template target
- **WHEN** `ai-specs sync` materializes the template
- **THEN** the target exists with catalog-derived content
- **AND** the lock contains a matching managed `sha256`

### Requirement: Ownership classifier

For each governed target, sync and doctor SHALL classify using on-disk bytes,
lock `sha256` (if any), and the bytes the CLI would write from the current
catalog after placeholder rendering:

| Classification | Rule |
|---|---|
| `missing` | destination absent |
| `untracked` | destination present, no managed entry |
| `managed_current` | lock sha equals disk and would-write equals disk |
| `managed_stale` | lock sha equals disk and would-write differs |
| `user_modified` | lock entry present and lock sha differs from disk |

### Requirement: Update policy by category

Governed targets SHALL honor `auto`, `confirm`, or `never-force`. Templates
default to `auto` unless `update_policy` is set on the recipe template entry.
Runtime hook scripts are not governed overrides and SHALL continue to be
rewritten unconditionally by sync.

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

### Requirement: Safe migration without managed metadata

When classification is `untracked`, sync SHALL never force-update on first
encounter. If bytes equal the would-write catalog bytes, sync SHALL seed a
managed record without rewriting or warning. If they differ, sync SHALL
preserve the file, emit a missing-metadata warning, and introduce no managed
hash until the CLI successfully writes the file.

### Requirement: Explicit refresh

Removing a governed target and rerunning sync SHALL reseed it from catalog and
record a fresh managed lock entry. Sync SHALL NOT discard `user_modified`
content without an explicit user delete.

### Requirement: Doctor alignment

Doctor SHALL use the same classifier: warn for user-modified and diverged
untracked targets, and for managed-stale targets under `confirm` or
`never-force`; it SHALL remain quiet for managed-current and managed-stale
`auto` targets.

### Requirement: Policy documentation

Project or recipe documentation SHALL describe `auto`, `confirm`, and
`never-force` behavior, explicit `rm` plus sync refresh, and that runtime hook
scripts are always rewritten by the CLI.
