## MODIFIED Requirements

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
