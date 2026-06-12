# recipe-schema Specification

## Purpose

Define the canonical file format and directory layout for a recipe package.
## Requirements
### Requirement: Recipe package layout
A recipe SHALL be declared in a directory `catalog/recipes/<id>/` containing at minimum a `recipe.toml` file. The directory MAY contain `skills/`, `commands/`, `templates/`, and `docs/` subdirectories.

#### Scenario: Minimal valid recipe
- **WHEN** a recipe directory contains only `recipe.toml`
- **THEN** the recipe SHALL be considered valid
- **AND** sync SHALL process it without error

#### Scenario: Recipe with bundled assets
- **WHEN** a recipe directory contains `recipe.toml`, `skills/`, `commands/`, `templates/`, and `docs/`
- **THEN** sync SHALL materialize all declared primitives

### Requirement: recipe.toml schema
The file `recipe.toml` SHALL contain a `[recipe]` table with fields: `id` (string, required), `name` (string, required), `description` (string, required), `version` (string, required), `author` (string, optional), `license` (string, optional). It SHALL contain a `[provides]` table declaring primitives.

#### Scenario: Valid recipe.toml
- **WHEN** `recipe.toml` contains all required `[recipe]` fields and a valid `[provides]` table
- **THEN** validation SHALL pass

#### Scenario: Missing required field
- **WHEN** `recipe.toml` omits `id`, `name`, `description`, or `version`
- **THEN** validation SHALL fail with an explicit error

### Requirement: Primitive declarations in [provides]
The `[provides]` table SHALL support: `skills` (array of objects with `id` and `source`), `commands` (array of objects with `id` and `path`), `mcp` (array of tables with `id` and MCP fields), `templates` (array of tables with `source`, `target`, and `condition`), `docs` (array of tables with `source` and `target`), and `hooks` (array of tables declaring agent-runtime lifecycle hooks).

Each `[[provides.hooks]]` entry SHALL contain `id` (string, required, unique within the recipe), `event` (string, required, one of the known abstract events), and `script` (string, required, a path inside the recipe directory). It MAY contain `matcher` (string), `blocking` (boolean, default `false`), and `description` (string). Runtime hooks are declared ONLY in `recipe.toml`; they are never declared in the project manifest.

#### Scenario: Skills with bundled source
- **WHEN** a skill declares `source = "bundled"`
- **THEN** sync SHALL copy the skill from `catalog/recipes/<id>/skills/<skill-id>/`

#### Scenario: Skills with dep source
- **WHEN** a skill declares `source = "dep"`, `url` (string, required), and optionally `path` (string)
- **THEN** sync SHALL vendor the skill using the standard `[[deps]]` flow
- **AND** the skill SHALL be installed into `ai-specs/skills/<skill-id>/`

#### Scenario: Commands declaration
- **WHEN** a command is declared with `id` and `path`
- **THEN** sync SHALL copy the file at `path` into `ai-specs/commands/<id>.md`
- **AND** `path` SHALL be relative to the recipe directory

#### Scenario: MCP preset declaration
- **WHEN** an MCP preset is declared under `[[provides.mcp]]`
- **THEN** sync SHALL merge it into derived MCP configs

#### Scenario: Template with not_exists condition
- **WHEN** a template declares `condition = "not_exists"`, `source` (relative to recipe directory), and `target` (relative to project root)
- **THEN** sync SHALL copy it only if `target` does not already exist

#### Scenario: Docs declaration
- **WHEN** a doc is declared with `source` (relative to recipe directory) and `target` (relative to project root)
- **THEN** sync SHALL copy `source` to `target`

#### Scenario: Hooks declaration is valid
- **WHEN** `[[provides.hooks]]` contains `id`, a known `event`, and a `script` path inside the recipe directory
- **THEN** validation SHALL pass and the hook SHALL be registered for runtime-hook distribution

#### Scenario: Hook with missing required field
- **GIVEN** a `[[provides.hooks]]` entry that omits `id`, `event`, or `script`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing field

#### Scenario: Hook with unknown event
- **GIVEN** a `[[provides.hooks]]` entry whose `event` is not a known abstract event
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an error listing the known events

#### Scenario: Hook script path escapes the recipe directory
- **GIVEN** a `[[provides.hooks]]` entry whose `script` resolves outside the recipe directory (absolute path or `../` traversal)
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit path-escape error

### Requirement: Optional recipe init declaration

A recipe MAY declare an initialization workflow in `recipe.toml` using a top-level `[init]` table. The `[init]` table SHALL be optional, and recipes without `[init]` SHALL remain valid and SHALL preserve their existing parse, add, and sync behavior.

The `[init]` table SHALL support `prompt` (string, required when `[init]` exists), `description` (string, optional), `needs_manifest` (boolean, optional), and `needs_mcp` (array of strings, optional). Additional init fields SHALL be rejected unless a later spec explicitly defines them.

#### Scenario: Recipe without init declaration

- **GIVEN** a valid recipe `recipe.toml` without `[init]`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the recipe SHALL be treated as having no init workflow
- **AND** existing recipe add and sync behavior SHALL be unchanged

#### Scenario: Recipe with valid init declaration

- **GIVEN** a valid recipe `recipe.toml` with `[init]`
- **AND** `[init]` declares `prompt = "docs/init.md"`
- **AND** `docs/init.md` exists under the recipe directory
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
- **AND** the parsed recipe metadata SHALL include the init prompt path and optional init fields

#### Scenario: Init declaration without prompt

- **GIVEN** a valid recipe `recipe.toml` with `[init]`
- **AND** `[init]` does not declare `prompt`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming `[init].prompt`

#### Scenario: Unknown init field

- **GIVEN** a valid recipe `recipe.toml` with `[init]`
- **AND** `[init]` declares an unsupported field
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the unsupported init field

### Requirement: Init prompt path validation

The init `prompt` value SHALL be a relative path inside the recipe directory. The parser SHALL reject absolute paths, parent-directory traversal, empty paths, directory paths, and paths that do not exist.

#### Scenario: Prompt path inside recipe directory

- **GIVEN** `[init]` declares `prompt = "docs/init.md"`
- **AND** `catalog/recipes/example/docs/init.md` exists as a file
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass

#### Scenario: Absolute prompt path

- **GIVEN** `[init]` declares `prompt = "/tmp/init.md"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error that init prompt paths MUST be relative to the recipe directory

#### Scenario: Prompt path escapes recipe directory

- **GIVEN** `[init]` declares `prompt = "../init.md"`
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error that init prompt paths MUST stay inside the recipe directory

#### Scenario: Missing prompt file

- **GIVEN** `[init]` declares `prompt = "docs/missing.md"`
- **AND** the file does not exist under the recipe directory
- **WHEN** the recipe is parsed
- **THEN** validation SHALL fail with an explicit error naming the missing init prompt file

### Requirement: ConfigField supports optional validation attribute
The `ConfigField` dataclass SHALL support an optional `validation` attribute. When present, it SHALL contain a `regex` string representing a format constraint.

#### Scenario: ConfigField parsed with validation
- **GIVEN** a `recipe.toml` with `[config.board_id]` where `required = true`, `type = "string"`, and `validation.regex = "^[0-9a-fA-F]{24}$"`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** the resulting `ConfigField` SHALL have `validation` set to a dict containing `{"regex": "^[0-9a-fA-F]{24}$"}`

#### Scenario: ConfigField parsed without validation
- **GIVEN** a `recipe.toml` with `[config.timeout]` where `required = false`, `type = "integer"`, and no `validation` table
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** the resulting `ConfigField` SHALL have `validation` set to `None`

#### Scenario: ConfigField rejects malformed validation table
- **GIVEN** a `recipe.toml` with `[config.board_id]` where `validation` is a table but contains an unknown key (e.g., `validation.min = 5`)
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error naming the unsupported validation key


---

## MODIFIED Requirements (from recipe-brief-fragments)

### Requirement: [provides.brief] optional slot in recipe.toml

The `[provides]` table in `recipe.toml` SHALL support an optional `[provides.brief]` table.
When present, it declares per-section brief fragments that `agents-render.py` merges into
the runtime brief. When absent, the recipe contributes no fragments and all existing parse,
add, and sync behavior is unchanged.

#### Scenario: [provides.brief] absent — recipe parses and syncs unchanged

- **GIVEN** a valid `recipe.toml` with no `[provides.brief]` table
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL pass
- **AND** the parsed `Recipe` object SHALL have `brief_fragments = None` (or equivalent empty value)
- **AND** sync SHALL process the recipe without error

#### Scenario: [provides.brief] present with simple array form

- **GIVEN** a `recipe.toml` containing:
  ```toml
  [provides.brief]
  workflow_rules = [
    "Create a dedicated worktree for changes that modify code.",
    "Do not push to `{config.integration_branch}` without a PR.",
  ]
  ```
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL pass
- **AND** the `BriefFragments.workflow_rules` list SHALL contain two `BriefFragment` entries
- **AND** each entry SHALL have `key = None` and `text` equal to the corresponding string

#### Scenario: [provides.brief] present with inline-table form

- **GIVEN** a `recipe.toml` containing:
  ```toml
  [[provides.brief.context_sources]]
  key = "trello-source-of-truth"
  text = "Trello is the source of truth for work state and dependencies."
  ```
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL pass
- **AND** `BriefFragments.context_sources` SHALL contain one entry with `key = "trello-source-of-truth"` and the specified `text`

#### Scenario: [provides.brief] with both contributable sections

- **GIVEN** a `recipe.toml` with `[provides.brief]` declaring both `workflow_rules` (simple array) and `context_sources` (inline-tables)
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL pass
- **AND** both section lists SHALL be populated in the `BriefFragments` object

---

### Requirement: BriefFragment and BriefFragments dataclasses

`recipe_schema.py` SHALL define:

- `BriefFragment`: a dataclass with fields `key: Optional[str]` and `text: str`.
- `BriefFragments`: a dataclass (or equivalent typed container) with one optional field per
  contributable section (`runtime_flow`, `context_sources`, `conflict_policy`,
  `workflow_rules`, `useful_commands`, `mcp_descriptions`), each typed as
  `Optional[List[BriefFragment]]`.

The `Recipe` dataclass SHALL include a field `brief_fragments: Optional[BriefFragments]`.

#### Scenario: BriefFragment normalized correctly for simple string

- **GIVEN** a fragment text parsed from a simple string array element `"Do X."`
- **WHEN** the parser normalizes it
- **THEN** the resulting `BriefFragment` SHALL have `key = None` and `text = "Do X."`

#### Scenario: BriefFragment normalized correctly for inline-table

- **GIVEN** a fragment inline-table `{key = "foo", text = "Do Y."}`
- **WHEN** the parser normalizes it
- **THEN** the resulting `BriefFragment` SHALL have `key = "foo"` and `text = "Do Y."`

---

### Requirement: Validation of [provides.brief] content

`recipe_schema.py` SHALL validate `[provides.brief]` entries and fail with explicit errors for:

1. Fragment declared for `intro` or `purpose` (project-only sections).
2. Fragment declared for an unknown section name.
3. Inline-table entry missing `text` or `key`.
4. Mixed forms in a single section (TOML array value AND inline-table entries simultaneously).

#### Scenario: intro section in [provides.brief] causes validation failure

- **GIVEN** a `recipe.toml` with `[provides.brief].intro = ["My intro."]`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error stating `intro` is a project-only section

#### Scenario: purpose section in [provides.brief] causes validation failure

- **GIVEN** a `recipe.toml` with `[provides.brief].purpose = ["My purpose."]`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error stating `purpose` is a project-only section

#### Scenario: Unknown section name causes validation failure

- **GIVEN** a `recipe.toml` with `[provides.brief].custom_rules = ["A rule."]`
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error naming `custom_rules` as unknown and listing the valid section names

#### Scenario: Inline-table with missing text field fails validation

- **GIVEN** a `[[provides.brief.workflow_rules]]` entry containing only `key = "foo"` (no `text`)
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error naming the missing `text` field

#### Scenario: Inline-table with missing key field fails validation

- **GIVEN** a `[[provides.brief.workflow_rules]]` entry containing only `text = "A rule."` (no `key`)
- **WHEN** `recipe_schema.py` parses the recipe
- **THEN** validation SHALL fail with an explicit error naming the missing `key` field

---

## MODIFIED Requirements (from recipe-anatomy-init-readme)

### Requirement: Recipe package layout

A recipe SHALL be declared in a directory `catalog/recipes/<id>/` containing at minimum a `recipe.toml` file. The directory MAY contain `skills/`, `commands/`, `templates/`, and `docs/` subdirectories.

A recipe MAY include canonical root files for human and agent audiences:
- `README.md` (human audience): describes what the recipe does, installation, and configuration. MAY be referenced by `provides.docs[].source` to materialize as installed documentation in consumer projects; the file at the recipe root is not implicitly installed.
- `init.md` (agent audience): executable initialization contract referenced by `[init].prompt`. When `[init]` is declared, the file SHOULD live at the recipe root (`init.md`) rather than under `docs/`.

The audience separation is canonical:
- Recipe root holds files describing the recipe to its consumers (human via `README.md`, agent via `init.md`).
- `skills/<skill-id>/SKILL.md` holds bundled skill definitions (one subdirectory per declared skill in `provides.skills`).
- `docs/` is reserved for documentation assets that will be materialized into consumer projects via `provides.docs[]`.
- `commands/` and `templates/` hold their respective primitive sources.

#### Scenario: Recipe with canonical root files

- **WHEN** a recipe directory contains `recipe.toml`, `README.md`, and `init.md` at the root
- **AND** `recipe.toml` declares `[init].prompt = "init.md"`
- **THEN** validation SHALL pass
- **AND** the recipe SHALL be considered well-formed under the canonical layout

#### Scenario: Multi-skill recipe

- **WHEN** a recipe declares two skills in `provides.skills`
- **THEN** each skill SHALL live at `skills/<skill-id>/SKILL.md`
- **AND** no `SKILL.md` SHALL exist at the recipe root

---

### Requirement: Init prompt path validation (extended)

The init `prompt` value SHALL be a relative path inside the recipe directory. The parser SHALL
reject absolute paths, parent-directory traversal, empty paths, directory paths, and paths that
do not exist.

The parser SHALL accept any relative path inside the recipe directory; the canonical layout
recommends `init.md` at the root, but `docs/init.md` and other relative paths SHALL remain
valid for backward compatibility.

#### Scenario: Prompt path at recipe root

- **GIVEN** `[init]` declares `prompt = "init.md"`
- **AND** `catalog/recipes/example/init.md` exists as a file
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass
