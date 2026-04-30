## ADDED Requirements

### Requirement: Command recipe init

The system SHALL provide `ai-specs recipe init <id> [path]` to produce an agent-readable initialization brief for a catalog recipe. The optional `path` argument SHALL select the project root using the same path semantics as existing recipe commands.

#### Scenario: Init command for installed recipe with init workflow

- **GIVEN** an initialized project containing `ai-specs/ai-specs.toml`
- **AND** the catalog contains recipe `tracker`
- **AND** the recipe declares a valid `[init]` workflow
- **AND** the manifest declares `[recipes.tracker]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the command SHALL exit 0
- **AND** it SHALL print an agent-readable initialization brief
- **AND** the brief SHALL include recipe identity, install state, existing recipe config state, init prompt content or path, relevant manifest context, and reviewable next actions

#### Scenario: Init command can inspect available recipe before add

- **GIVEN** an initialized project containing `ai-specs/ai-specs.toml`
- **AND** the catalog contains recipe `tracker`
- **AND** the recipe declares a valid `[init]` workflow
- **AND** the manifest does not declare `[recipes.tracker]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the command SHALL exit 0
- **AND** the initialization brief SHALL state that the recipe is not installed
- **AND** the brief MAY propose adding `[recipes.tracker]` as a reviewable manifest change

#### Scenario: Init command on uninitialized project

- **GIVEN** a directory without `ai-specs/ai-specs.toml`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the command SHALL fail with an explicit uninitialized-project error
- **AND** exit code SHALL be 1
- **AND** no files SHALL be mutated

#### Scenario: Init command for missing recipe

- **GIVEN** an initialized project containing `ai-specs/ai-specs.toml`
- **AND** `catalog/recipes/missing/` does not exist
- **WHEN** `ai-specs recipe init missing` runs
- **THEN** the command SHALL fail with `Recipe 'missing' no encontrada en catalog/recipes/` or an equivalent explicit recipe-not-found error
- **AND** exit code SHALL be 1
- **AND** no files SHALL be mutated

#### Scenario: Init command for recipe without init workflow

- **GIVEN** an initialized project containing `ai-specs/ai-specs.toml`
- **AND** the catalog contains recipe `basic`
- **AND** the recipe has no `[init]` declaration
- **WHEN** `ai-specs recipe init basic` runs
- **THEN** the command SHALL report that recipe `basic` has no init workflow
- **AND** exit code SHALL be 1
- **AND** no files SHALL be mutated

### Requirement: Init brief is reviewable and idempotent

`ai-specs recipe init` SHALL NOT silently apply behavioral changes. Any manifest edits, config updates, template overrides, or generated files proposed by init SHALL be shown as reviewable actions or patches before mutation. Re-running init SHALL detect existing recipe declarations, existing `[recipes.<id>.config]` keys, and existing target files so it can propose updates instead of duplicates.

#### Scenario: Existing recipe config detected

- **GIVEN** an initialized project whose manifest contains `[recipes.tracker.config]`
- **AND** the table already declares `board_id = "abc123"`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief SHALL report the existing `board_id` config key
- **AND** the brief SHALL NOT propose appending a duplicate `board_id` key
- **AND** any replacement SHALL be presented as an update to the existing key

#### Scenario: Existing recipe declaration detected

- **GIVEN** an initialized project whose manifest contains `[recipes.tracker]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief SHALL identify the existing recipe declaration
- **AND** the brief SHALL NOT propose appending a second `[recipes.tracker]` table

#### Scenario: Existing template target detected

- **GIVEN** a recipe init workflow that may propose a template override
- **AND** the target file already exists in the project
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief SHALL identify the existing target
- **AND** the brief SHALL propose reviewable update, skip, or diff guidance instead of silently overwriting the file
