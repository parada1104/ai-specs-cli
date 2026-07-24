## ADDED Requirements

### Requirement: Conditional template targets MUST reside under the overrides boundary

`[[provides.templates]]` entries declaring `condition = "not_exists"` in a
recipe manifest MUST target a path under
`ai-specs/recipes/{recipe-id}/overrides/`. The project `.gitignore` only
re-includes the `overrides/` subtree; any conditional template targeting a
bare `ai-specs/recipes/{id}/templates/` or `ai-specs/recipes/{id}/bin/` path
is silently gitignored and therefore un-committable after first
materialization. Recipe authors MUST place conditional template targets under
`overrides/` so the materialized files are tracked project surface.

#### Scenario: Conditional template targets overrides path

- **GIVEN** a recipe `my-recipe` with a `[[provides.templates]]` entry
  declaring `condition = "not_exists"`
- **AND** `target = "ai-specs/recipes/my-recipe/overrides/templates/card-feature.md"`
- **WHEN** `ai-specs sync` materializes the template on a fresh project
- **THEN** the file is written at the declared target path
- **AND** `git check-ignore` confirms the path is NOT ignored (committable)

#### Scenario: Conditional template targets bare path (non-committable)

- **GIVEN** a recipe `my-recipe` with a `[[provides.templates]]` entry
  declaring `condition = "not_exists"`
- **AND** `target = "ai-specs/recipes/my-recipe/templates/card-feature.md"`
  (outside `overrides/`)
- **WHEN** `ai-specs sync` materializes the template
- **THEN** the file is written at the declared target path
- **AND** `git check-ignore` confirms the path IS ignored (un-committable)
- **AND** the file will not appear in `git status` for the project

#### Scenario: Bin script target under overrides

- **GIVEN** a recipe `my-recipe` with a `[[provides.templates]]` entry
  declaring `condition = "not_exists"`
- **AND** `target = "ai-specs/recipes/my-recipe/overrides/bin/cleanup.sh"`
- **WHEN** `ai-specs sync` materializes the script
- **THEN** the file is committable under the `overrides/` boundary
