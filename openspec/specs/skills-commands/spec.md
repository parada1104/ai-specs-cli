# skills-commands Specification

## Purpose

Define the `ai-specs skills` command surface (`add`, `list`, `remove`), the
`add-dep` backward-compatibility alias, and the shared flag-and-usage error
handling. The commands operate on the `[[deps]]` manifest entries and the
project's `ai-specs/skills/` directory.

Implemented by `lib/skills.sh`, `lib/skills-add.sh`, `lib/skills-list.sh`, and
`lib/skills-remove.sh`, dispatched from `bin/ai-specs`.

## Terminology

- **Vendored skill**: a `[[deps]]` entry in `ai-specs.toml`, cloned into
  `ai-specs/skills/<id>/` during `ai-specs sync`.
- **Local skill**: a directory with `SKILL.md` under `ai-specs/skills/` that is
  *not* a vendored dep (authored in-repo).
- **Catalog skill**: a skill under `catalog/skills/` shipped with the CLI.

## Requirements

### Requirement: `skills add` registers a vendored dependency

The `ai-specs skills add <git-url> [--flags]` command SHALL append a `[[deps]]`
block to `ai-specs/ai-specs.toml` and SHALL run `ai-specs sync` immediately
after, unless `--no-sync` is passed. When no `--id` flag is supplied, the id
SHALL be derived from the last URL path component with a trailing `.git`
removed. An id already registered in `[[deps]]` SHALL be rejected with exit code
1 and an error message, and a missing `ai-specs/ai-specs.toml` SHALL fail with
exit code 1.

#### Scenario: `skills add` appends a dep and syncs

- **GIVEN** a project with `ai-specs/ai-specs.toml`
- **WHEN** the user runs `ai-specs skills add <git-url> [--flags]`
- **THEN** a `[[deps]]` block SHALL be appended to the manifest
- **AND** `ai-specs sync` SHALL run immediately after (unless `--no-sync`)

#### Scenario: Duplicate skill id is rejected

- **GIVEN** a skill with `id` already registered in `[[deps]]`
- **WHEN** the user runs `ai-specs skills add` with any URL resolving to that id
- **THEN** the command SHALL fail with exit code 1
- **AND** SHALL print an error message

#### Scenario: Missing manifest fails

- **GIVEN** a missing `ai-specs/ai-specs.toml`
- **WHEN** the user runs `ai-specs skills add`
- **THEN** the command SHALL fail with exit code 1

#### Scenario: Id is derived from the URL

- **GIVEN** a git URL and no `--id` flag
- **WHEN** the user runs `ai-specs skills add`
- **THEN** the id SHALL be derived from the last URL path component (minus `.git`)

### Requirement: `skills list` reports registered, local, and catalog skills

The `ai-specs skills list` command SHALL print three sections: registered
`[[deps]]` entries with sync status, local skills in `ai-specs/skills/`, and
available catalog skills in `catalog/skills/`. When there are no `[[deps]]`
entries, the deps section SHALL show `(none)`.

#### Scenario: Output shows the three sections

- **GIVEN** a project with `ai-specs/ai-specs.toml`
- **WHEN** the user runs `ai-specs skills list`
- **THEN** the output SHALL show three sections:
  - registered `[[deps]]` with sync status
  - local skills in `ai-specs/skills/`
  - available catalog skills in `catalog/skills/`

#### Scenario: Empty deps section shows (none)

- **GIVEN** no `[[deps]]` entries
- **WHEN** the user runs `ai-specs skills list`
- **THEN** the deps section SHALL show `(none)`

### Requirement: `skills remove` drops a vendored dependency

The `ai-specs skills remove <id>` command SHALL remove the matching `[[deps]]`
block from the manifest and SHALL preserve the on-disk `ai-specs/skills/<id>/`
directory. An id with no `[[deps]]` entry SHALL fail with exit code 1, and a
project without `ai-specs/ai-specs.toml` SHALL fail with exit code 1.

#### Scenario: Remove drops the manifest entry but keeps the on-disk skill

- **GIVEN** a project with a `[[deps]]` entry for `id="my-skill"`
- **WHEN** the user runs `ai-specs skills remove my-skill`
- **THEN** the `[[deps]]` block SHALL be removed from the manifest
- **AND** the on-disk `ai-specs/skills/my-skill/` SHALL be preserved

#### Scenario: Removing an unknown id fails

- **GIVEN** no `[[deps]]` entry for `id="my-skill"`
- **WHEN** the user runs `ai-specs skills remove my-skill`
- **THEN** the command SHALL fail with exit code 1

#### Scenario: Missing manifest fails

- **GIVEN** a project without `ai-specs/ai-specs.toml`
- **WHEN** the user runs `ai-specs skills remove`
- **THEN** the command SHALL fail with exit code 1

### Requirement: Backward compatibility — `add-dep` alias

When the user runs `ai-specs add-dep <url> [flags]`, the command SHALL delegate
to `skills-add.sh` with identical behavior, and `ai-specs help` SHALL document it
as an alias.

#### Scenario: `add-dep` delegates to `skills add`

- **GIVEN** the `add-dep` subcommand
- **WHEN** the user runs `ai-specs add-dep <url> [flags]`
- **THEN** it SHALL delegate to `skills-add.sh` with identical behavior
- **AND** `ai-specs help` SHALL document it as an alias

### Requirement: Error handling for flags, arguments, and help

All `ai-specs skills` commands SHALL exit with code 2 on unknown flags and SHALL
exit with code 2 when missing required positional arguments. All commands SHALL
print usage on `--help` and exit 0.

#### Scenario: Unknown flags exit 2

- **WHEN** any `ai-specs skills` command is invoked with an unknown flag
- **THEN** the command SHALL exit with code 2

#### Scenario: Missing positional arguments exit 2

- **WHEN** any `ai-specs skills` command is invoked while missing a required
  positional argument
- **THEN** the command SHALL exit with code 2

#### Scenario: `--help` prints usage and exits 0

- **WHEN** any `ai-specs skills` command is invoked with `--help`
- **THEN** the command SHALL print usage and exit 0
