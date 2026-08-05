# worktree-flow (delta)

## MODIFIED Requirements

### Requirement: Stale Cleanup Override Detection

When a `[[provides.templates]]` entry with `condition = "not_exists"` already
has a materialized target, sync (and doctor) MUST classify the target using
override-ownership rules (lock-backed last-managed hash vs would-write catalog
bytes), not catalog-only comparison alone.

- **Managed current:** no stale warning; leave file untouched (lock may be
  backfilled).
- **Managed stale** with effective policy `auto`: overwrite with current
  catalog-derived content, update the managed lock record, and MUST NOT emit a
  user-modified warning.
- **User-modified** or **untracked diverged**: emit a non-blocking WARN naming
  the path with refresh instructions; MUST NOT overwrite.
- **Missing target:** normal fresh-copy path under `not_exists`; not a warning
  case.

#### Scenario: Unmodified managed override matching catalog produces no warning

- **GIVEN** a materialized `worktree-cleanup.sh` override whose bytes match the
  current would-write catalog content and the lock managed hash (or is seeded
  as matching untracked)
- **WHEN** `ai-specs sync` runs
- **THEN** it MUST NOT emit a user-modified or stale-overwrite WARN for that file
- **AND** it MUST leave the override content unchanged when already current

#### Scenario: Managed stale override is refreshed under auto policy

- **GIVEN** a materialized cleanup override whose bytes still match the lock
  managed hash
- **AND** the current catalog would-write content differs
- **AND** effective policy is `auto`
- **WHEN** `ai-specs sync` runs
- **THEN** it MUST overwrite the override with current catalog-derived content
- **AND** it MUST update the lock managed hash
- **AND** sync MUST exit successfully

#### Scenario: User-modified override warns and sync succeeds

- **GIVEN** a materialized cleanup override whose content differs from the lock
  managed hash
- **WHEN** `ai-specs sync` runs
- **THEN** it MUST emit a non-blocking WARN naming the override path and
  indicating user modification
- **AND** the WARN MUST include refresh instructions (`rm <target> && ai-specs sync`
  or equivalent)
- **AND** sync MUST exit successfully without overwriting the override

#### Scenario: Missing override gets a fresh copy

- **GIVEN** no materialized cleanup override exists at the `not_exists` target
- **WHEN** `ai-specs sync` runs
- **THEN** it MUST copy/render the catalog template to the target as the normal
  `not_exists` path
- **AND** it MUST record a managed lock entry
- **AND** it MUST NOT emit a stale-override WARN for that missing file
