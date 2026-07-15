# project-recipe-cache Specification

## Purpose

NEW capability. Defines the per-project CLI cache for recipe origin staging: cache root and keying under `$AI_SPECS_HOME`, layout for `.recipe`, `.deps`, managed commands, and resolved-skills flatten output, shared resolver usage across materialize/vendor/skill-resolution/flatten/orphan paths, and migration cleanup of in-project origin trees.

## Requirements

### Requirement: Cache root and key

The system SHALL stage recipe origin under `$AI_SPECS_HOME/cache/projects/<key>/`. The cache key SHALL be derived from a short `sha256(realpath(project_root))` hash. A sidecar `meta.toml` SHALL record `project_root` for doctor/debug. The cache root MUST NOT use XDG paths.

#### Scenario: Cache path

- GIVEN project root `/p` and a configured `AI_SPECS_HOME`
- WHEN sync runs
- THEN origin materializes under `$AI_SPECS_HOME/cache/projects/<key>/`
- AND the sidecar records `project_root` as the realpath of `/p`

### Requirement: Cache layout and project surface split

The cache SHALL contain `.recipe`, `.deps`, managed command staging, and `resolved-skills`. The project SHALL retain `ai-specs.toml`, `ai-specs/skills/`, and `ai-specs/recipes/` (docs, hooks, bin, `not_exists` templates). Origin skills, deps, managed commands, and resolved-skills MUST NOT remain required under the project tree.

#### Scenario: Surface split

- GIVEN an enabled recipe
- WHEN sync completes
- THEN skills, deps, managed commands, and resolved-skills live under the cache
- AND docs and hooks remain under `ai-specs/recipes/`
- AND `ai-specs/skills/` contains only local user skills

### Requirement: Leftover origin cleanup

Sync SHALL delete leftover in-project `ai-specs/.recipe` and `ai-specs/.deps` when present. Init MUST NOT require or create those in-project origin directories.

#### Scenario: Leftover rm

- GIVEN leftover `ai-specs/.recipe` and/or `ai-specs/.deps` directories
- WHEN sync runs
- THEN both trees are deleted from the project

### Requirement: Shared cache resolver

Materialize, vendor, skill-resolution, flatten, and orphan cleanup MUST resolve cache paths through one shared resolver module. No component SHALL hardcode in-project `ai-specs/.recipe` or `ai-specs/.deps` for origin staging.

#### Scenario: Orphan via resolver

- GIVEN a recipe disabled after cache materialization
- WHEN sync runs
- THEN cache origin for that recipe is cleaned via the shared resolver
