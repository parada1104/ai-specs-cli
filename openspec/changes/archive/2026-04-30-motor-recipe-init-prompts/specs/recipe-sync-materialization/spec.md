## ADDED Requirements

### Requirement: Init remains separate from sync materialization

Recipe initialization MAY preview or propose templates, overrides, and manifest config needed before sync, but init SHALL NOT run `ai-specs sync` and SHALL NOT silently materialize recipe primitives. Sync SHALL remain responsible for materializing enabled recipe primitives.

#### Scenario: Init does not run sync

- **GIVEN** a recipe declares bundled skills, commands, MCP presets, templates, and docs
- **WHEN** `ai-specs recipe init <id>` runs
- **THEN** init SHALL NOT materialize those primitives
- **AND** init SHALL NOT update derived agent configs or registries through sync

#### Scenario: Init previews template target

- **GIVEN** a recipe declares a template with `target = "ai-specs/example.md"`
- **WHEN** `ai-specs recipe init <id>` runs
- **THEN** init MAY report that the template target is relevant to setup
- **AND** init MAY propose a reviewable create or update action
- **AND** sync SHALL remain the command that materializes the declared template primitive

#### Scenario: Human-reviewed init mutation remains idempotent

- **GIVEN** an init workflow has already created or updated a project override file after review
- **WHEN** `ai-specs recipe init <id>` runs again
- **THEN** init SHALL detect the existing file
- **AND** init SHALL propose skip, update, or diff guidance instead of creating a duplicate file

#### Scenario: Sync still materializes after init

- **GIVEN** init has proposed or applied reviewed manifest config for a recipe
- **AND** the recipe is enabled in `ai-specs/ai-specs.toml`
- **WHEN** `ai-specs sync` runs
- **THEN** sync SHALL validate and materialize the enabled recipe according to the sync materialization contract
- **AND** sync SHALL NOT assume init previously ran
