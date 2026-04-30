# external-dirs-layout Specification

## Purpose

Define the directory structure, gitignore rules, and `ai-specs init` behavior for isolating external skill sources from local project skills.

## Requirements

### Requirement: Init creates external directories
When `ai-specs init` runs, the system SHALL create `.recipe/` and `.deps/` directories at the project root if they do not already exist.

#### Scenario: Fresh init
- **WHEN** `ai-specs init` runs in a new project
- **THEN** the project root SHALL contain a `.recipe/` directory
- **AND** the project root SHALL contain a `.deps/` directory

#### Scenario: Directories already exist
- **WHEN** `ai-specs init` runs and `.recipe/` and `.deps/` already exist
- **THEN** init SHALL succeed without error
- **AND** existing contents SHALL NOT be modified

### Requirement: External directories are gitignored
The system SHALL add `.recipe/` and `.deps/` to the generated `.gitignore` during `ai-specs init`.

#### Scenario: Gitignore generation
- **WHEN** `ai-specs init` completes
- **THEN** `.gitignore` SHALL contain an entry for `.recipe/`
- **AND** `.gitignore` SHALL contain an entry for `.deps/`

#### Scenario: Gitignore idempotency
- **WHEN** `ai-specs init` runs multiple times
- **THEN** `.gitignore` SHALL NOT contain duplicate entries for `.recipe/` or `.deps/`

### Requirement: Recipe materialization directory layout
Recipe-bundled skills SHALL be materialized into `.recipe/{recipe-id}/skills/{skill-id}/`.

#### Scenario: Recipe skill materialization
- **WHEN** a recipe declares a bundled skill with `id = "my-skill"`
- **THEN** sync SHALL materialize it to `.recipe/{recipe-id}/skills/my-skill/`
- **AND** the skill files SHALL be identical to the recipe catalog source

### Requirement: Dependency vendor directory layout
Vendored dependency skills SHALL be cloned into `.deps/{dep-id}/skills/{skill-id}/`.

#### Scenario: Dependency skill vendoring
- **WHEN** a dependency declares a skill with `id = "vendor-skill"`
- **THEN** vendor-skills.py SHALL clone it to `.deps/{dep-id}/skills/vendor-skill/`
- **AND** the clone SHALL be a shallow clone if configured

### Requirement: Local skills directory exclusivity
`ai-specs/skills/` SHALL contain only local, project-owned skills. The system SHALL NOT place recipe-bundled or vendored dependency skills into `ai-specs/skills/`.

#### Scenario: Sync does not pollute local skills
- **WHEN** sync materializes a recipe skill and a dep skill
- **THEN** neither SHALL be written to `ai-specs/skills/`
- **AND** `ai-specs/skills/` SHALL be unchanged except by explicit local actions
