# recipe-manifest-contract Specification

## Purpose

Define how recipes are declared in a project's `ai-specs.toml` manifest.

## ADDED Requirements

### Requirement: Recipe instance declaration
A project MAY declare an installed recipe using a top-level `[recipes.<id>]` table. The table SHALL contain `enabled` (boolean, required) and `version` (string, required). The `id` MUST match a recipe in the CLI recipe catalog.

#### Scenario: Recipe enabled and pinned
- **WHEN** `[recipes.runtime-memory-openmemory]` declares `enabled = true` and `version = "1.0.0"`
- **THEN** sync SHALL validate the recipe exists in the catalog
- **AND** sync SHALL validate the version matches `recipe.toml`
- **AND** sync SHALL materialize the recipe

#### Scenario: Recipe disabled
- **WHEN** `[recipes.runtime-memory-openmemory]` declares `enabled = false`
- **THEN** sync SHALL skip materialization for this recipe
- **AND** sync SHALL NOT fail

#### Scenario: Version mismatch
- **WHEN** manifest pins `version = "1.0.0"` but catalog has `version = "1.1.0"`
- **THEN** sync SHALL fail with an explicit version-mismatch error

#### Scenario: Unknown recipe ID
- **WHEN** manifest declares `[recipes.unknown-id]`
- **THEN** sync SHALL fail with an explicit "recipe not found" error

### Requirement: Backward compatibility
The absence of any `[recipes.*]` section SHALL NOT cause validation or sync to fail.

#### Scenario: Manifest without recipes
- **WHEN** `ai-specs.toml` contains no `[recipes.*]` tables
- **THEN** sync SHALL proceed normally
- **AND** no recipe-related behavior SHALL be triggered

### Requirement: Durable recipe init output

Recipe initialization output that is intended to survive sync SHALL be stored in the existing manifest section responsible for that data. Per-recipe values SHALL be stored under `[recipes.<id>.config]` unless another manifest section is explicitly responsible for the value.

#### Scenario: Init stores per-recipe config

- **GIVEN** recipe `tracker` declares config field `board_id`
- **WHEN** init proposes durable storage for the selected board
- **THEN** the proposed manifest delta SHALL target `[recipes.tracker.config]`
- **AND** the proposed key SHALL be `board_id`

#### Scenario: Init stores existing manifest responsibility elsewhere

- **GIVEN** init discovers an MCP server that belongs in `[mcp.trello]`
- **WHEN** init proposes durable storage for MCP declaration data
- **THEN** the proposal MAY target `[mcp.trello]`
- **AND** it SHALL NOT duplicate that MCP declaration under `[recipes.tracker.config]` unless the recipe config schema explicitly requires a separate reference value

### Requirement: Init manifest deltas avoid duplicate declarations

When init proposes updates to `ai-specs/ai-specs.toml`, it SHALL update existing `[recipes.<id>]` and `[recipes.<id>.config]` declarations instead of appending duplicate tables or duplicate keys.

#### Scenario: Existing recipe table updated

- **GIVEN** the manifest already contains `[recipes.tracker]`
- **WHEN** init proposes changing `enabled` or `version`
- **THEN** the proposed manifest delta SHALL update the existing `[recipes.tracker]` table
- **AND** it SHALL NOT add a second `[recipes.tracker]` table

#### Scenario: Existing config key updated

- **GIVEN** the manifest already contains `[recipes.tracker.config]` with `board_id = "old"`
- **WHEN** init proposes `board_id = "new"`
- **THEN** the proposed manifest delta SHALL update the existing `board_id` key
- **AND** it SHALL NOT append a duplicate `board_id` key

#### Scenario: Missing config table added once

- **GIVEN** the manifest contains `[recipes.tracker]`
- **AND** it does not contain `[recipes.tracker.config]`
- **WHEN** init proposes per-recipe config values
- **THEN** the proposed manifest delta SHALL add exactly one `[recipes.tracker.config]` table

---

### Requirement: [brief].render controls managed AGENTS.md generation

The manifest `[brief]` table MAY include an optional `render` key of type boolean.
When absent, the default SHALL be `true`. When `render = false`, the project
opts out of managed `AGENTS.md` generation: neither manifest `[brief]` prose nor
enabled recipe `[provides.brief]` fragments SHALL be written to `AGENTS.md` during
sync or init.

The `render` key is independent of other `[brief]` keys: prose fields and
`<section>_mode` keys MAY remain in the manifest for documentation or for use if
rendering is re-enabled later, but they MUST NOT affect `AGENTS.md` on disk while
`render = false`.

#### Scenario: render omitted defaults to enabled

- **GIVEN** a manifest `[brief]` table without a `render` key
- **WHEN** the manifest is validated and sync runs
- **THEN** validation SHALL pass
- **AND** managed AGENTS.md generation SHALL proceed as when `render = true`

#### Scenario: render false disables managed output

- **GIVEN** a manifest declaring:
  ```toml
  [brief]
  render = false
  intro = "Manual project voice."
  workflow_rules = ["This rule must not appear in AGENTS.md while render is false."]
  ```
- **AND** enabled recipes contribute `[provides.brief]` fragments
- **WHEN** `ai-specs sync` runs
- **THEN** `AGENTS.md` MUST NOT be updated with `intro`, `workflow_rules`, or recipe fragments
- **AND** sync MUST NOT fail solely because `[brief]` contains prose keys

#### Scenario: render true with prose and recipes behaves as today

- **GIVEN** a manifest declaring `[brief] render = true` (or omitting `render`)
- **AND** enabled recipes contribute fragments
- **WHEN** `ai-specs sync` runs
- **THEN** `AGENTS.md` MUST be generated with merged recipe fragments and manifest prose
- **AND** behavior MUST match the pre-change contract

---

### Requirement: [brief].render validation

The value of `[brief].render` MUST be a TOML boolean (`true` or `false`).
Non-boolean values (including capitalized `True`/`False` or string `"false"`)
SHALL be rejected during validation with an explicit error naming `[brief].render`
and listing the accepted boolean forms.

#### Scenario: Lowercase boolean accepted

- **GIVEN** a manifest declaring `[brief] render = false`
- **WHEN** the manifest is validated
- **THEN** validation SHALL pass

#### Scenario: Invalid boolean rejected

- **GIVEN** a manifest declaring `[brief] render = "false"` (string)
- **WHEN** validation runs (doctor or sync preflight)
- **THEN** validation SHALL fail with an error referencing `[brief].render`
- **AND** the error MUST indicate that a boolean is required

#### Scenario: Capitalized True rejected at parse time

- **GIVEN** a manifest file containing `render = True` (invalid TOML boolean)
- **WHEN** the manifest is parsed
- **THEN** parsing SHALL fail with a TOML decode error
- **OR** if caught by doctor, report an explicit boolean-format guidance message

---

### Requirement: render false propagates to subrepo sync targets

When the root manifest declares `[brief] render = false`, subrepo targets
resolved from `[project].subrepos` MUST inherit the same hands-off policy for
`AGENTS.md`. Subrepo sync MUST NOT invoke managed rendering using the root
manifest's `[brief]` or recipe fragments.

Per-subrepo override of `[brief].render` is out of scope for V1 (subrepos do not
carry their own manifest).

#### Scenario: Root render false applies to subrepo fan-out

- **GIVEN** the root manifest declares `[brief] render = false`
- **AND** `[project].subrepos` includes a wired subrepo path
- **AND** the subrepo has an existing `AGENTS.md`
- **WHEN** root `ai-specs sync` fans out to the subrepo
- **THEN** the subrepo's `AGENTS.md` MUST NOT be regenerated
- **AND** other subrepo artifacts (skills, commands) MUST still sync

---

### Requirement: Doctor guidance for render disabled configurations

`ai-specs doctor` MUST surface configuration guidance when `[brief].render = false`:

- INFO when render is disabled (sync will not update AGENTS.md)
- ERROR when render is disabled and `AGENTS.md` is missing
- WARN when render is disabled and any enabled recipe contributes non-empty
  `[provides.brief]` fragments (dead configuration weight)

#### Scenario: Doctor ERROR when render false and AGENTS.md missing

- **GIVEN** a project with `[brief] render = false`
- **AND** no `AGENTS.md` at the project root
- **WHEN** `ai-specs doctor` runs
- **THEN** doctor MUST report an ERROR for the missing AGENTS.md
- **AND** guidance MUST suggest creating a manual brief or enabling render

#### Scenario: Doctor WARN when recipe fragments unused

- **GIVEN** a project with `[brief] render = false`
- **AND** an enabled recipe declares `[provides.brief]` fragments
- **WHEN** `ai-specs doctor` runs
- **THEN** doctor MUST report a WARN indicating recipe brief fragments will not be applied
- **AND** doctor MUST NOT fail with a non-zero exit solely for this WARN

#### Scenario: Doctor INFO when render disabled with AGENTS.md present

- **GIVEN** a project with `[brief] render = false` and an existing `AGENTS.md`
- **WHEN** `ai-specs doctor` runs
- **THEN** doctor MUST report INFO that managed rendering is disabled
- **AND** doctor exit code MUST remain 0 unless other checks fail
