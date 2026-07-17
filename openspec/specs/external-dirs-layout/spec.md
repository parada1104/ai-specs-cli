# external-dirs-layout Specification

## Purpose

Define the project-surface vs CLI-cache split for recipe origin staging.
Origin skills, deps, managed commands, and resolved-skills flatten live under
`$AI_SPECS_HOME/cache/projects/<key>/`. The project keeps `ai-specs.toml`,
`ai-specs/skills/`, and `ai-specs/recipes/` (docs, hooks, templates, overrides).

## Requirements

### Requirement: Init does not create in-project origin dirs

`ai-specs init` MUST NOT create in-project `.recipe/` or `.deps/` origin directories.
Leftover cleanup is handled during sync migration.

#### Scenario: Fresh init

- **WHEN** `ai-specs init` runs in a new project
- **THEN** the project MUST NOT require `ai-specs/.recipe/` or `ai-specs/.deps/`

### Requirement: In-project origin gitignore removed

Gitignore rules for in-project origin paths (`.recipe/`, `.deps/`) SHALL be
removed as part of migration. Origin trees are disposable under AI_SPECS_HOME.

#### Scenario: Gitignore generation

- **WHEN** `ai-specs init` completes
- **THEN** root `.gitignore` MUST NOT list `ai-specs/.recipe/` or `ai-specs/.deps/` as required origin paths

### Requirement: Recipe skill layout

Recipe bundled skill origin SHALL live at `{cache}/.recipe/{id}/skills/{skill}/`.

#### Scenario: Recipe skills in cache

- **WHEN** materialize runs for a recipe
- **THEN** bundled skills are staged under the cache `.recipe` path
- **AND** catalog content matches the installed CLI

### Requirement: Dependency skill layout

Dependency skill origin SHALL live at `{cache}/.deps/{dep}/skills/{skill}/`.

#### Scenario: Dep skills in cache

- **WHEN** vendor sync runs for a dependency
- **THEN** dependency skills are staged under the cache `.deps` path

### Requirement: Local skills exclusivity

`ai-specs/skills/` SHALL contain only local user skills after sync.

#### Scenario: No skills pollution

- **WHEN** sync completes
- **THEN** `ai-specs/skills/` contains only local skills

### Requirement: recipes/ user surface

`ai-specs/recipes/` SHALL hold docs, overrides, hooks, and `not_exists` templates.
It is user/project surface, not origin staging for skills or deps.

#### Scenario: Not origin

- **WHEN** sync completes
- **THEN** skills and deps are not required under `ai-specs/recipes/` as origin paths
