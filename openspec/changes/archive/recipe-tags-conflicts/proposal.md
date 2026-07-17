# Proposal: Recipe tags/categories + conflicts_with (Trello #27)

## Why

Recipes in `catalog/recipes/` have no way to declare their domain (VCS, tracker,
infra, quality, storage) or to signal that they are alternatives to one another.
Today a project can enable two competing recipes (for example two VCS PR flows)
and nothing surfaces the overlap to the developer. The capability-binding system
already disambiguates *which provider serves a capability*, but it does not
communicate the broader "these two recipes occupy the same category" signal, nor
does it let a recipe author declare a genuine incompatibility.

## What Changes

- Extend the `[recipe]` table with two optional fields:
  - `tags` (array of strings): domain/category labels.
  - `conflicts_with` (array of strings): recipe IDs this recipe is incompatible
    with. A recipe MUST NOT list itself.
- Add tag-based conflict detection in `recipe-conflicts.py`:
  - Recipes sharing a tag emit a **warning** (same category).
  - Recipes sharing a tag where one lists the other in `conflicts_with` emit a
    **fatal**-severity conflict.
- Surface tag conflicts during `ai-specs sync` as **advisory warnings only**.
  Tags are metadata; they MUST NOT block materialization (see Design for the
  decision and its rationale).
- Add `tags` to the catalog recipes that have a clear domain.
- Document the new fields in `docs/recipe-schema.md`.
- **BREAKING**: None. Both fields default to empty; existing recipes and
  manifests are unaffected.

## Capabilities

### Modified Capabilities
- `recipe-schema`: add `tags` and `conflicts_with` to the `[recipe]` table
  contract (delta spec).
- `recipe-conflict-resolution`: add tag-based conflict detection and its
  advisory (non-blocking) surfacing during sync (delta spec).

## Affected Modules

- `lib/_internal/recipe_schema.py` — `Recipe` dataclass + parsing/validation.
- `lib/_internal/recipe-conflicts.py` — `TagConflict` + `check_tag_conflicts`.
- `lib/_internal/recipe-materialize.py` — advisory surfacing during sync.
- `catalog/recipes/*/recipe.toml` — `tags` metadata.
- `docs/recipe-schema.md` — documentation.

## Key Constraint Discovered (drives Design)

`git-pr-flow` and `bitbucket-pr-flow` are **designed to coexist**: a project may
enable both and resolve the active provider via `[[bindings]]` for the
`vcs-pr-flow` capability (`tests/test_bitbucket_pr_flow_recipe.py::BitbucketPrFlowDualProviderTests`).
Therefore:
1. The catalog VCS recipes MUST NOT declare `conflicts_with` against each other.
   They share the `vcs` tag (warning), which is the correct signal.
2. Sync MUST NOT hard-fail on tag conflicts, or it would break the supported
   dual-provider scenario and violate "tags are advisory metadata."

## Rollback Plan

The change is additive and isolated to the recipe schema/conflict layer.
Rollback = revert the branch; both new fields default empty, so removing them
leaves existing recipes and the sync flow exactly as before.
