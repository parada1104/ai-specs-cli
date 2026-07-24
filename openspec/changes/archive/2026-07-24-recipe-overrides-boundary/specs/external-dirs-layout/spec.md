## ADDED Requirements

### Requirement: Recipe-provided conditional templates resolve within the overrides boundary

The `.gitignore` contract for `ai-specs/recipes/` ignores everything under
`recipes/**` except the `recipes/*/overrides/` subtree. Any
`[[provides.templates]]` entry with `condition = "not_exists"` that targets a
path outside `ai-specs/recipes/{recipe-id}/overrides/` produces a file that
is silently gitignored and un-committable. Recipe manifests MUST declare
conditional template targets under the `overrides/` subtree to produce
committable project surface. This applies to all conditional sub-kinds:
card templates (`overrides/templates/`), scripts
(`overrides/bin/`), and any future conditional target kind.

#### Scenario: Gitignore allows overrides templates but not bare recipes paths

- **GIVEN** a project with `ai-specs/.gitignore` rendered by sync
- **WHEN** `git check-ignore` is run against
  `ai-specs/recipes/my-recipe/overrides/templates/card-feature.md`
- **THEN** the path is NOT ignored
- **AND** `git check-ignore` against
  `ai-specs/recipes/my-recipe/templates/card-feature.md` (bare, outside
  `overrides/`) reports the path IS ignored

#### Scenario: Override boundary covers all conditional template sub-paths

- **GIVEN** a recipe declaring multiple `[[provides.templates]]` entries with
  `condition = "not_exists"` targeting both
  `ai-specs/recipes/{id}/overrides/templates/` and
  `ai-specs/recipes/{id}/overrides/bin/`
- **WHEN** sync materializes all entries
- **THEN** every materialized file is committable
- **AND** no materialized file falls outside the `overrides/` subtree

#### Scenario: Non-conditional template targets are unaffected

- **GIVEN** a `[[provides.templates]]` entry WITHOUT `condition = "not_exists"`
  (unconditional template)
- **WHEN** sync materializes the template
- **THEN** the target path behavior is unchanged (this requirement applies
  only to conditional entries whose `not_exists` semantics imply
  project-owned, committable surface)
