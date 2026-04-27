# Proposal: Design Recipe Schema

## Why

The ai-specs manifest (`ai-specs.toml`) only supports granular `[[deps]]` entries—one git URL per skill. Installing a complete capability like "operational memory" requires multiple manual steps (skills, MCP presets, commands, templates). A **recipe** is a named, versioned composition of primitives that `ai-specs` can materialize in a single declaration, reducing boilerplate and enabling reusable capability bundles.

## What Changes

- Introduce `catalog/recipes/<id>/recipe.toml` as the canonical declaration format for a recipe.
- Add `[recipes.<id>]` section to `ai-specs.toml` manifest so projects can declare which recipes are installed and pinned.
- Extend `ai-specs sync` to read `[recipes.*]`, validate `recipe.toml`, materialize bundled skills/commands/templates/docs, vendor external deps, and detect conflicts.
- Add conflict-detection logic: sync fails explicitly when two recipes declare the same primitive ID (skill, command, MCP).
- Add version-pinning validation: sync fails when manifest pin does not match catalog `recipe.toml` version.
- **BREAKING**: None. Recipes are additive; existing manifests without `[recipes.*]` continue to work.

## Capabilities

### New Capabilities
- `recipe-schema`: Defines the `recipe.toml` file format, catalog layout (`catalog/recipes/<id>/`), and primitive types (skills, commands, mcp, templates, docs).
- `recipe-manifest-contract`: Defines how `[recipes.<id>]` is declared in `ai-specs.toml` (fields: `enabled`, `version`).
- `recipe-sync-materialization`: Defines how `ai-specs sync` resolves, validates, and materializes recipes into the project workspace.
- `recipe-conflict-resolution`: Defines explicit error behavior when recipe primitives collide.

### Modified Capabilities
- `manifest-contract`: Extend to recognize `[recipes.<id>]` as an optional top-level section (delta spec).

## Impact

- `catalog/` — new `catalog/recipes/` directory and layout.
- `lib/sync.sh` and Python helpers — extended to resolve and materialize recipes.
- `lib/_internal/` — new `recipe-*.py` modules for validation, conflict detection, and materialization.
- `templates/ai-specs.toml.tmpl` — updated to include commented `[recipes.<id>]` example.
- `ai-specs/ai-specs.toml` (root manifest) — updated to document the new section in its contract table.
- Tests — new test modules for recipe validation, conflict detection, and sync integration.
