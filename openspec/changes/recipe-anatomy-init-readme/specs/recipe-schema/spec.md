## MODIFIED Requirements

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

#### Scenario: Minimal valid recipe

- **WHEN** a recipe directory contains only `recipe.toml`
- **THEN** the recipe SHALL be considered valid
- **AND** sync SHALL process it without error

#### Scenario: Recipe with bundled assets

- **WHEN** a recipe directory contains `recipe.toml`, `skills/`, `commands/`, `templates/`, and `docs/`
- **THEN** sync SHALL materialize all declared primitives

#### Scenario: Recipe with canonical root files

- **WHEN** a recipe directory contains `recipe.toml`, `README.md`, and `init.md` at the root
- **AND** `recipe.toml` declares `[init].prompt = "init.md"`
- **THEN** validation SHALL pass
- **AND** the recipe SHALL be considered well-formed under the canonical layout

#### Scenario: Multi-skill recipe

- **WHEN** a recipe declares two skills in `provides.skills`
- **THEN** each skill SHALL live at `skills/<skill-id>/SKILL.md`
- **AND** no `SKILL.md` SHALL exist at the recipe root

### Requirement: Init prompt path validation

The init `prompt` value SHALL be a relative path inside the recipe directory. The parser SHALL reject absolute paths, parent-directory traversal, empty paths, directory paths, and paths that do not exist.

The parser SHALL accept any relative path inside the recipe directory; the canonical layout recommends `init.md` at the root, but `docs/init.md` and other relative paths SHALL remain valid for backward compatibility.

#### Scenario: Prompt path inside recipe directory

- **GIVEN** `[init]` declares `prompt = "docs/init.md"`
- **AND** `catalog/recipes/example/docs/init.md` exists as a file
- **WHEN** the recipe is parsed
- **THEN** validation SHALL pass

#### Scenario: Prompt path at recipe root

- **GIVEN** `[init]` declares `prompt = "init.md"`
- **AND** `catalog/recipes/example/init.md` exists as a file
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
