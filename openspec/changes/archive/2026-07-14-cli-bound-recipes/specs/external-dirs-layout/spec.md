# Spec delta: external-dirs-layout

## REMOVED Requirements

### Requirement: Init creates external dirs

Init SHALL NOT create in-project external origin directories. Leftover cleanup is handled during sync migration.

### Requirement: External dirs gitignored

In-project origin directories (`.recipe`, `.deps`) are no longer part of the required layout. Gitignore rules for those in-project origin paths SHALL be removed as part of migration.

## MODIFIED Requirements

### Requirement: Recipe skill layout

Recipe bundled skill origin SHALL live at `{cache}/.recipe/{id}/skills/{skill}/` instead of in-project `.recipe`.

#### Scenario: Recipe skills in cache

- WHEN materialize runs for a recipe
- THEN bundled skills are staged under the cache `.recipe` path
- AND catalog content matches the installed CLI

### Requirement: Dependency skill layout

Dependency skill origin SHALL live at `{cache}/.deps/{dep}/skills/{skill}/` instead of in-project `.deps`.

#### Scenario: Dep skills in cache

- WHEN vendor sync runs for a dependency
- THEN dependency skills are staged under the cache `.deps` path

### Requirement: Local skills exclusivity

`ai-specs/skills/` SHALL contain only local user skills after sync.

#### Scenario: No skills pollution

- WHEN sync completes
- THEN `ai-specs/skills/` contains only local skills

## ADDED Requirements

### Requirement: recipes/ user surface

`ai-specs/recipes/` SHALL hold docs, overrides, hooks, and `not_exists` templates. It is user/project surface, not origin staging for skills or deps.

#### Scenario: Not origin

- WHEN sync completes
- THEN skills and deps are not required under `ai-specs/recipes/` as origin paths
