# recipe-manifest-contract Specification

## Purpose

Define how recipes are declared in a project's `ai-specs.toml` manifest.

## ADDED Requirements

### Requirement: Recipe instance declaration
A project MAY declare an installed recipe using a top-level `[recipes.<id>]` table. The table SHALL contain `enabled` (boolean, required). `version` is not required. Sync SHALL materialize the CLI catalog version with no pin fail-close. Legacy `version` keys SHALL be ignored with a WARN and MUST NOT block sync. Floating or `min_version` pins are not supported. The `id` MUST match a recipe in the CLI recipe catalog.

#### Scenario: No version
- **GIVEN** an enabled recipe with no `version` key
- **WHEN** sync runs
- **THEN** catalog content is materialized successfully

#### Scenario: Legacy WARN
- **GIVEN** an enabled recipe with a stale `version` key
- **WHEN** sync runs
- **THEN** a WARN is emitted
- **AND** sync succeeds with current catalog content

#### Scenario: Recipe disabled
- **WHEN** `[recipes.runtime-memory-openmemory]` declares `enabled = false`
- **THEN** sync SHALL skip materialization for this recipe
- **AND** sync SHALL NOT fail

#### Scenario: Unknown recipe ID
- **WHEN** manifest declares `[recipes.unknown-id]`
- **THEN** sync SHALL fail with an explicit "recipe not found" error

### Requirement: CLI catalog without pin ceremony
After a CLI upgrade, enabled recipes SHALL sync to the new catalog without requiring a toml edit.

#### Scenario: Post-upgrade
- **GIVEN** enabled recipes and an upgraded CLI
- **WHEN** sync runs without toml changes
- **THEN** new catalog content is materialized

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

When init proposes updates to `ai-specs/ai-specs.toml`, it SHALL update existing `[recipes.<id>]` and `[recipes.<id>.config]` declarations instead of appending duplicate tables or duplicate keys. These paths MUST NOT write `version`.

#### Scenario: Existing recipe table updated

- **GIVEN** the manifest already contains `[recipes.tracker]`
- **WHEN** init proposes changing `enabled`
- **THEN** the proposed manifest delta SHALL update the existing `[recipes.tracker]` table
- **AND** it SHALL NOT add a second `[recipes.tracker]` table
- **AND** no `version` key is written

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

---

## MODIFIED Requirements (from recipe-brief-fragments)

### Requirement: [brief] table scope reduced to project voice

The manifest `[brief]` table is the project's voice. After this change, `intro` and
`purpose` remain exclusively under `[brief]` and MUST NOT be contributed by recipes.
The contributable sections (`runtime_flow`, `context_sources`, `conflict_policy`,
`workflow_rules`, `useful_commands`, `mcp_descriptions`) become optional in `[brief]`:
they augment or override recipe fragments rather than being the sole source.

A manifest `[brief]` with only `intro` and `purpose` (no contributable sections) is
valid and sufficient. The rendered brief will be populated for contributable sections
by enabled recipe fragments.

#### Scenario: Manifest [brief] with only intro and purpose — contributable sections populated by recipes

- **GIVEN** a manifest containing:
  ```toml
  [brief]
  intro = "Demo service for orders."
  purpose = "Process and reconcile order events."
  ```
- **AND** the enabled recipe `worktree-flow` contributes `workflow_rules` fragments
- **WHEN** `agents-render.py` renders the manifest
- **THEN** the output MUST contain `## Workflow Rules` populated by recipe fragments
- **AND** `intro` and `purpose` MUST appear from the manifest (project voice)

#### Scenario: Manifest [brief] with no contributable section keys is valid

- **GIVEN** a manifest `[brief]` table containing only `intro` and `purpose`
- **WHEN** `ai-specs sync` runs
- **THEN** validation SHALL pass
- **AND** sync SHALL NOT fail with a missing-key error for any contributable section

#### Scenario: Manifest [brief] additions to contributable sections append after recipe fragments

- **GIVEN** a manifest `[brief].context_sources = ["Project-specific source."]`
- **AND** recipe `vault-canonical-store` contributes a `context_sources` fragment
- **WHEN** `agents-render.py` renders the manifest (APPEND default)
- **THEN** the recipe fragment MUST appear BEFORE `"Project-specific source."` in `## Context Sources`

---

### Requirement: <section>_mode = "replace" suppresses recipe fragments per section

A project author MAY set `<section>_mode = "replace"` as a sibling key in `[brief]` to
suppress all recipe contributions for that specific section. In REPLACE mode, only the
manifest-authored content for that section is rendered.

The `_mode` suffix follows the section name: `workflow_rules_mode`, `context_sources_mode`,
`runtime_flow_mode`, `conflict_policy_mode`, `useful_commands_mode`.

The key is optional for every section. When absent, APPEND mode applies by default.

#### Scenario: workflow_rules_mode = "replace" suppresses recipe contributions

- **GIVEN** the manifest declares:
  ```toml
  [brief]
  workflow_rules_mode = "replace"
  workflow_rules = ["Only this project-specific rule."]
  ```
- **AND** an enabled recipe contributes `workflow_rules` fragments
- **WHEN** `agents-render.py` renders the manifest
- **THEN** `## Workflow Rules` MUST contain ONLY `"Only this project-specific rule."`
- **AND** NO recipe-contributed `workflow_rules` fragments MUST appear

#### Scenario: Replace mode for one section does not affect others

- **GIVEN** `workflow_rules_mode = "replace"` is set in `[brief]`
- **AND** an enabled recipe also contributes `context_sources` fragments
- **WHEN** `agents-render.py` renders the manifest
- **THEN** `## Context Sources` MUST still include recipe fragments (APPEND default)

#### Scenario: APPEND is the default when no _mode key is present

- **GIVEN** the manifest `[brief]` contains `workflow_rules = ["Extra rule."]` but no `workflow_rules_mode` key
- **AND** a recipe contributes a `workflow_rules` fragment `"Recipe rule."`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** `## Workflow Rules` MUST contain both `"Recipe rule."` and `"Extra rule."`
- **AND** `"Recipe rule."` MUST appear BEFORE `"Extra rule."`

#### Scenario: Unknown _mode value causes validation failure

- **GIVEN** the manifest declares `workflow_rules_mode = "merge"`
- **WHEN** `ai-specs sync` runs
- **THEN** validation SHALL fail with an explicit error naming `workflow_rules_mode` and listing the valid values (`"append"`, `"replace"`)

---

### Requirement: intro and purpose remain project-only in [brief]

`intro` and `purpose` MUST remain exclusively under the project manifest `[brief]` table.
The renderer MUST NOT accept or apply any recipe contribution to these sections.
The manifest `[brief].intro` and `[brief].purpose` values MUST NOT undergo `{config.KEY}`
substitution (they are rendered verbatim).

#### Scenario: intro and purpose rendered verbatim from manifest

- **GIVEN** a manifest `[brief].intro = "This is the {project.name} service."` (contains braces)
- **WHEN** `agents-render.py` renders the manifest
- **THEN** the rendered intro MUST be `"This is the {project.name} service."` — no substitution performed

#### Scenario: intro absent from manifest — intro section omitted

- **GIVEN** a manifest `[brief]` that does not contain `intro`
- **WHEN** `agents-render.py` renders the manifest
- **THEN** no intro blockquote section MUST appear in the output

---

### Requirement: mcp_descriptions in [brief] are project overrides

The manifest `[brief].mcp_descriptions` entries function as project-level overrides for
MCP server descriptions. For each server named in `mcp_descriptions`, the manifest value
takes precedence over any recipe-declared default for that server.

#### Scenario: Manifest mcp_descriptions overrides recipe default

- **GIVEN** a recipe declares a default description for `trello` server
- **AND** the manifest `[brief].mcp_descriptions.trello = "Our custom Trello description."`
- **WHEN** `agents-render.py` renders the MCP section
- **THEN** the trello server entry MUST use `"Our custom Trello description."`

#### Scenario: Manifest mcp_descriptions for one server does not affect others

- **GIVEN** the manifest `[brief].mcp_descriptions` contains an entry for `trello` only
- **AND** a recipe declares a default description for `vault` server
- **WHEN** `agents-render.py` renders the MCP section
- **THEN** the vault server entry MUST use the recipe-declared default (gap-fill)

---

### Requirement: Scaffold template [brief] reduced to intro and purpose

The project manifest scaffold template (`ai-specs.toml.tmpl` or equivalent) SHALL reduce
`[brief]` to only `intro` and `purpose`, with an explanatory comment directing project
authors to recipes for contributable sections.

#### Scenario: New project scaffold does not pre-populate contributable sections

- **GIVEN** a new project initialized with `ai-specs init`
- **WHEN** the generated `ai-specs.toml` is inspected
- **THEN** the `[brief]` table MUST contain only `intro` and `purpose` fields (and explanatory comment)
- **AND** it MUST NOT pre-populate `workflow_rules`, `context_sources`, `conflict_policy`,
  `runtime_flow`, or `useful_commands`

#### Scenario: Scaffold comment explains recipe contributions

- **GIVEN** the generated scaffold `ai-specs.toml`
- **WHEN** a project author reads the `[brief]` section
- **THEN** a comment MUST direct them to enable recipes to populate contributable sections
- **AND** the comment MUST mention that `<section>_mode = "replace"` can override recipe fragments
