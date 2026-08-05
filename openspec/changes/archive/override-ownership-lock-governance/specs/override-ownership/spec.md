# override-ownership Specification

## Purpose

Define ownership classification and update governance for CLI-seeded override
targets (primarily `[[provides.templates]]` with `condition = "not_exists"`),
using scoped `[managed.*]` lock records so sync can refresh stale managed files
without overwriting user modifications.

## ADDED Requirements

### Requirement: Managed lock record on CLI write

When the CLI writes a governed override target, it SHALL upsert
`[managed."<project-relative-path>"]` in `ai-specs/.ai-specs.lock` with
`sha256` equal to the bytes written (after any placeholder substitution). It
SHOULD also record `recipe`, `source`, `kind`, and effective `policy` when known.

#### Scenario: Fresh seed records managed hash

- **GIVEN** no file at a governed template target
- **WHEN** `ai-specs sync` materializes the template
- **THEN** the target file exists with catalog-derived content
- **AND** the lock contains `[managed."<target>"]` with matching `sha256`

### Requirement: Ownership classifier

For each governed target, sync (and doctor, when checking overrides) SHALL
classify using on-disk bytes, lock `sha256` (if any), and the bytes the CLI would
write from the current catalog (post-placeholder):

| Classification | Rule |
|---|---|
| `missing` | destination absent |
| `untracked` | destination present, no `[managed.*]` entry |
| `managed_current` | lock sha equals disk; would-write equals disk |
| `managed_stale` | lock sha equals disk; would-write differs |
| `user_modified` | lock entry present and lock sha differs from disk |

#### Scenario: Managed stale when catalog evolved and disk still matches lock

- **GIVEN** a governed target whose on-disk bytes equal the lock `sha256`
- **AND** the current catalog would-write bytes differ from on-disk
- **WHEN** classification runs
- **THEN** the result MUST be `managed_stale`

#### Scenario: User-modified when disk diverges from lock

- **GIVEN** a governed target with a lock entry
- **AND** on-disk bytes differ from the lock `sha256`
- **WHEN** classification runs
- **THEN** the result MUST be `user_modified`

### Requirement: Update policy by category

Governed targets SHALL honor an effective update policy of `auto`, `confirm`,
or `never-force`. Templates default to `auto` unless `update_policy` is set on
the recipe template entry. Runtime hook scripts are **not** governed overrides:
they SHALL continue to be rewritten unconditionally by sync (always-CLI).

| Policy | On `managed_stale` |
|---|---|
| `auto` | Overwrite with current catalog render; update lock |
| `confirm` | Do not overwrite; WARN with refresh instructions (v1) |
| `never-force` | Do not overwrite; WARN with refresh instructions |

#### Scenario: Auto policy force-updates managed stale

- **GIVEN** classification `managed_stale` and effective policy `auto`
- **WHEN** `ai-specs sync` runs
- **THEN** the target MUST be overwritten with current catalog-derived content
- **AND** the lock `sha256` MUST be updated
- **AND** sync MUST NOT emit a user-modified warning for that file

#### Scenario: User-modified is never force-updated

- **GIVEN** classification `user_modified`
- **WHEN** `ai-specs sync` runs
- **THEN** the target MUST be left unchanged
- **AND** sync MUST emit a non-blocking warning that names the path and
  indicates the file was user-modified
- **AND** the warning MUST include refresh guidance (`rm <target> && ai-specs sync`
  or equivalent)

### Requirement: Safe migration without managed metadata

When classification is `untracked`:

- If on-disk bytes equal would-write catalog bytes, sync SHALL seed a managed
  lock record without rewriting the file and SHALL NOT warn.
- If on-disk bytes differ from would-write, sync SHALL preserve the file, SHALL
  NOT seed a managed hash, SHALL NOT overwrite, and SHALL emit a non-blocking
  warning that metadata was missing / the file was not refreshed.

#### Scenario: Matching untracked seeds lock

- **GIVEN** a governed target with no lock entry whose bytes match would-write
- **WHEN** `ai-specs sync` runs
- **THEN** a `[managed.*]` entry MUST be written
- **AND** file bytes MUST remain unchanged

#### Scenario: Diverged untracked never force-updates

- **GIVEN** a governed target with no lock entry whose bytes differ from would-write
- **WHEN** `ai-specs sync` runs
- **THEN** the file MUST remain unchanged
- **AND** no managed `sha256` for that path MUST be introduced as if CLI-owned
- **AND** a non-blocking warning MUST be emitted

### Requirement: Explicit refresh

Removing a governed target and re-running sync SHALL re-seed from catalog and
record a fresh managed lock entry. Sync MUST NOT discard `user_modified`
content without an explicit user delete (or a future documented force path
outside v1 defaults).

#### Scenario: rm then sync refreshes

- **GIVEN** a user-modified governed target
- **WHEN** the user deletes the target and runs `ai-specs sync`
- **THEN** the catalog template MUST be copied/rendered to the target
- **AND** a managed lock entry MUST be recorded for the new bytes

### Requirement: Policy documentation

Project or recipe documentation touched by this change SHALL describe when
updates are automatic (`auto`), confirmation-deferred (`confirm`), or never
forced (`never-force`), and SHALL state that hook scripts are always rewritten
by the CLI.
