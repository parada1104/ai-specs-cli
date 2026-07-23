# external-dirs-layout Specification

## Purpose

Define the project-surface vs CLI-cache split for materialization.
Recipe origin, recipe-deps, managed commands, resolved-skills flatten, and
CLI-bundled skills live under `$AI_SPECS_HOME/cache/projects/<key>/`. The
committed project surface keeps `ai-specs.toml`, local `ai-specs/skills/`,
declared recipe overrides under `ai-specs/recipes/*/overrides/`, and (when
present) regenerable toml-deps under gitignored `ai-specs/.deps/`.

## Requirements

### Requirement: Init does not create in-project origin dirs

`ai-specs init` MUST NOT create in-project `.recipe/` directories. It MUST NOT
pre-create empty `ai-specs/.deps/` — that tree appears only when a toml-dep is
vendored. Leftover `.recipe/` cleanup is handled during sync migration.

#### Scenario: Fresh init

- **WHEN** `ai-specs init` runs in a new project
- **THEN** the project MUST NOT require `ai-specs/.recipe/`
- **AND** `ai-specs/.deps/` MUST NOT exist until a toml-dep is vendored

### Requirement: CLI-bundled skills never materialize in-project

CLI-bundled skills (recipe-independent skills shipped under
`$AI_SPECS_HOME/bundled-skills/`) SHALL be flattened into the cache at
`{cache}/.bundled/skills/{id}/` and MUST NOT be written into `ai-specs/skills/`
or committed to the project. Sync SHALL delete leftover in-project copies of
CLI-bundled skills, except where a locally-authored skill of the same id is
declared by the project.

#### Scenario: Leftover bundled skills removed on sync

- **GIVEN** `ai-specs/skills/{harness-lifecycle,harness-recipes,harness-skills-deps}`
  materialized by a prior CLI version
- **AND** none of those ids is a locally-authored project skill
- **WHEN** sync runs
- **THEN** those directories are deleted from the project
- **AND** the skills resolve from `{cache}/.bundled/skills/`

#### Scenario: Locally-authored copy is preserved

- **GIVEN** `ai-specs/skills/skill-creator/` is declared as a local project skill
- **WHEN** sync runs
- **THEN** the local copy is NOT deleted

### Requirement: Recipe skill layout

Recipe bundled skill origin SHALL live at `{cache}/.recipe/{id}/skills/{skill}/`.

#### Scenario: Recipe skills in cache

- **WHEN** materialize runs for a recipe
- **THEN** bundled skills are staged under the cache `.recipe` path
- **AND** catalog content matches the installed CLI

### Requirement: Dependency skill layout

Recipe-dependency skill origin SHALL live at `{cache}/.deps/{dep}/skills/{skill}/`.
Toml-deps declared in the project `ai-specs.toml` (`[[deps]]`) SHALL materialize
in-project instead (see toml-deps requirement).

#### Scenario: Recipe-dep skills in cache

- **WHEN** vendor sync runs for a recipe dependency
- **THEN** dependency skills are staged under the cache `.deps` path

### Requirement: toml-deps materialize in-project (gitignored)

Dependencies declared in the project `ai-specs.toml` (`[[deps]]`, added via
`add-dep`) are project governance. Their skills SHALL materialize in-project at
`ai-specs/.deps/{dep}/skills/{skill}/` and SHALL be gitignored (regenerable from
the declared git source). Dependencies pulled in transitively by a recipe
(recipe-deps) are unaffected and SHALL remain under `{cache}/.deps/`.

#### Scenario: toml-dep in-project and ignored

- **GIVEN** a dependency declared in `ai-specs.toml`
- **WHEN** vendor sync runs
- **THEN** its skills materialize under `ai-specs/.deps/`
- **AND** `ai-specs/.gitignore` lists `.deps/`

#### Scenario: recipe-dep stays in cache

- **GIVEN** a dependency vendored by an enabled recipe (not declared in
  `ai-specs.toml`)
- **WHEN** vendor sync runs
- **THEN** its skills materialize under `{cache}/.deps/`, not in-project

### Requirement: Local skills exclusivity

`ai-specs/skills/` SHALL contain only local user skills after sync.

#### Scenario: No skills pollution

- **WHEN** sync completes
- **THEN** `ai-specs/skills/` contains only local skills

### Requirement: recipes/ user surface

`ai-specs/recipes/` SHALL hold only the project-owned override surface. The
project-owned override surface (`ai-specs/recipes/{id}/overrides/`) SHALL be
committed. Bundled recipe docs, hooks, and default templates SHALL resolve from
the cache and SHALL NOT be required as committed content under
`ai-specs/recipes/`. Sync SHALL gitignore `ai-specs/recipes/` with negations
that re-include only the declared override surface.

#### Scenario: Only overrides committed

- **WHEN** sync completes
- **THEN** `ai-specs/.gitignore` ignores `recipes/**` except
  `recipes/*/overrides/`
- **AND** bundled recipe docs/hooks/templates are not required as committed
  content

#### Scenario: Not origin

- **WHEN** sync completes
- **THEN** skills and deps are not required under `ai-specs/recipes/` as origin paths

### Requirement: In-project surface gitignore

`ai-specs/.gitignore` SHALL ignore `.deps/` (toml-dep materialization) and
`recipes/**` (except the declared override surface). Root `.gitignore` SHALL
NOT list `ai-specs/.recipe/` or `ai-specs/.deps/` as agent-surface entries
(recipe/dep origin for recipes lives under `$AI_SPECS_HOME` cache; toml-deps
are ignored via the nested `ai-specs/.gitignore`).

#### Scenario: Gitignore generation

- **WHEN** `ai-specs init` or `sync` completes
- **THEN** `ai-specs/.gitignore` ignores `.deps/` and `recipes/**` except
  `recipes/*/overrides/`
- **AND** root `.gitignore` does NOT require `ai-specs/.recipe/`
