# recipe-overrides-runtime Specification

## Purpose

Define the runtime override loading contract for recipe-bundled skills, allowing projects to customize recipe-provided behavior without modifying vendored files.

## Requirements

### Requirement: Override config loading
For each recipe bundled in `.recipe/{recipe-id}/`, the system SHALL check for `.recipe/{recipe-id}/overrides/config.toml`. If present, the system SHALL load it at runtime and merge its values into the bundled skill's configuration with override values taking precedence.

#### Scenario: Override config exists
- **GIVEN** a recipe `my-recipe` is bundled at `.recipe/my-recipe/`
- **AND** `.recipe/my-recipe/overrides/config.toml` exists with custom values
- **WHEN** the bundled skill reads its configuration
- **THEN** the override values SHALL take precedence over the bundled defaults

#### Scenario: Override config missing
- **GIVEN** a recipe is bundled but `.recipe/{recipe-id}/overrides/config.toml` does not exist
- **WHEN** the bundled skill reads its configuration
- **THEN** the system SHALL use the bundled defaults without error
- **AND** no warning SHALL be emitted for the missing override file

### Requirement: Override template loading
For each recipe bundled in `.recipe/{recipe-id}/`, the system SHALL check for `.recipe/{recipe-id}/overrides/templates/`. If present, templates in this directory SHALL be loaded at runtime and override identically-named templates from the bundled skill.

#### Scenario: Override template exists
- **GIVEN** `.recipe/my-recipe/overrides/templates/custom.md` exists
- **AND** the bundled skill provides a template named `custom.md`
- **WHEN** the skill renders the template
- **THEN** the override version from `.recipe/my-recipe/overrides/templates/` SHALL be used

#### Scenario: Partial template overrides
- **GIVEN** `.recipe/my-recipe/overrides/templates/` contains only `A.md`
- **AND** the bundled skill provides both `A.md` and `B.md`
- **WHEN** the skill renders templates
- **THEN** `A.md` SHALL use the override version
- **AND** `B.md` SHALL use the bundled version

### Requirement: Overrides apply only to their parent recipe
Override files in `.recipe/{recipe-id}/overrides/` SHALL affect only skills bundled by that specific recipe. They SHALL NOT affect skills from other recipes or from `.deps/`.

#### Scenario: Override isolation between recipes
- **GIVEN** `.recipe/recipe-a/overrides/config.toml` modifies a setting
- **AND** `.recipe/recipe-b/skills/shared-skill/` also uses that setting
- **WHEN** `shared-skill` runs under `recipe-b`
- **THEN** the override from `recipe-a` SHALL NOT affect it

### Requirement: Overrides are not committed
The system SHALL add `.recipe/{recipe-id}/overrides/` to `.gitignore` during init, or SHALL document that overrides are user-local customizations that SHOULD NOT be committed.

#### Scenario: Overrides ignored by git
- **WHEN** `ai-specs init` runs
- **THEN** `.gitignore` SHALL contain a pattern that ignores `.recipe/*/overrides/`
- **AND** users SHALL be able to customize recipes without polluting git status
