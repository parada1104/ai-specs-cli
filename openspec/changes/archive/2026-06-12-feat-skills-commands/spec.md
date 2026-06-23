# Spec: `ai-specs skills` commands

## Terminology

- **Vendored skill**: a `[[deps]]` entry in `ai-specs.toml`, cloned into
  `ai-specs/skills/<id>/` during `ai-specs sync`.
- **Local skill**: a directory with `SKILL.md` under `ai-specs/skills/` that is
  *not* a vendored dep (authored in-repo).
- **Catalog skill**: a skill under `catalog/skills/` shipped with the CLI.

## Given/When/Then

### Skills add

- **GIVEN** a project with `ai-specs/ai-specs.toml`
  **WHEN** the user runs `ai-specs skills add <git-url> [--flags]`
  **THEN** a `[[deps]]` block SHALL be appended to the manifest
  **AND** `ai-specs sync` SHALL run immediately after (unless `--no-sync`)

- **GIVEN** a skill with `id` already registered in `[[deps]]`
  **WHEN** the user runs `ai-specs skills add` with any URL resolving to that id
  **THEN** the command SHALL fail with exit code 1
  **AND** SHALL print an error message

- **GIVEN** a missing `ai-specs/ai-specs.toml`
  **WHEN** the user runs `ai-specs skills add`
  **THEN** the command SHALL fail with exit code 1

- **GIVEN** a git URL and no `--id` flag
  **WHEN** the user runs `ai-specs skills add`
  **THEN** the id SHALL be derived from the last URL path component (minus `.git`)

### Skills list

- **GIVEN** a project with `ai-specs/ai-specs.toml`
  **WHEN** the user runs `ai-specs skills list`
  **THEN** the output SHALL show three sections:
    - registered `[[deps]]` with sync status
    - local skills in `ai-specs/skills/`
    - available catalog skills in `catalog/skills/`

- **GIVEN** no `[[deps]]` entries
  **WHEN** the user runs `ai-specs skills list`
  **THEN** the deps section SHALL show "(none)"

### Skills remove

- **GIVEN** a project with a `[[deps]]` entry for `id="my-skill"`
  **WHEN** the user runs `ai-specs skills remove my-skill`
  **THEN** the `[[deps]]` block SHALL be removed from the manifest
  **AND** the on-disk `ai-specs/skills/my-skill/` SHALL be preserved

- **GIVEN** no `[[deps]]` entry for `id="my-skill"`
  **WHEN** the user runs `ai-specs skills remove my-skill`
  **THEN** the command SHALL fail with exit code 1

- **GIVEN** a project without `ai-specs/ai-specs.toml`
  **WHEN** the user runs `ai-specs skills remove`
  **THEN** the command SHALL fail with exit code 1

### Backward compatibility

- **GIVEN** the `add-dep` subcommand
  **WHEN** the user runs `ai-specs add-dep <url> [flags]`
  **THEN** it SHALL delegate to `skills-add.sh` with identical behavior
  **AND** `ai-specs help` SHALL document it as an alias

### Error handling

- All commands SHALL exit with code 2 on unknown flags
- All commands SHALL exit with code 2 when missing required positional arguments
- All commands SHALL print usage on `--help` and exit 0
