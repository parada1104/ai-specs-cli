## ADDED Requirements

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
