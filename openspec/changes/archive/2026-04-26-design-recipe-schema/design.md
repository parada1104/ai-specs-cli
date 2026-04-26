# Design: Recipe Schema

## Context

The ai-specs CLI currently supports:
- `[[deps]]`: vendored skills (one git URL = one skill)
- `[mcp.<name>]`: MCP server declarations
- `ai-specs/commands/`: slash commands
- Bundled skills/commands shipped with the CLI

There is no mechanism to install a *bundle* of related primitives. Users must manually add multiple deps, MCP blocks, and commands. A **recipe** fills this gap by providing a named, versioned composition of primitives.

## Goals / Non-Goals

**Goals:**
- Define a declarative `recipe.toml` format that composes skills, commands, MCP presets, templates, and docs.
- Allow projects to declare installed recipes via `[recipes.<id>]` in `ai-specs.toml`.
- Extend `ai-specs sync` to materialize recipes: copy bundled assets, vendor external deps, apply templates with conditions.
- Detect and reject conflicts (same primitive ID declared by multiple recipes).
- Support version pinning so projects control when recipes upgrade.

**Non-Goals:**
- Recipe-to-recipe dependencies (nesting) — out of scope for V1.
- Remote recipe catalogs (URLs) — V1 uses built-in `catalog/recipes/` only.
- Conditional recipe installation based on project type/detectors.
- Recipe uninstallation (`ai-specs recipe remove`) — out of scope for V1.

## Decisions

### 1. Recipe as package in `catalog/recipes/<id>/`
- **Rationale**: Matches the existing `catalog/skills/` pattern. Built-in recipes ship with the CLI repo, making them testable and version-locked without extra network calls.
- **Alternative considered**: Git-URL recipes like `[[deps]]`. Rejected for V1 to reduce complexity and avoid lockfile proliferation.

### 2. Hybrid skill sourcing (`bundled` vs `dep`)
- **Rationale**: Author-owned skills ship directly with the recipe (fast, no network). Reusable external skills use standard `vendor-skills.py` (consistent with `[[deps]]`).
- **Implementation**: `source = "bundled"` copies from `catalog/recipes/<id>/skills/`. `source = "dep"` + `url` triggers vendoring.

### 3. Explicit conflict rejection (Option B)
- **Rationale**: Silent merging creates unpredictable state. Failing fast forces the user to resolve ambiguity explicitly in `ai-specs.toml`.
- **Implementation**: During sync, build a registry of claimed primitive IDs. If a duplicate is found, emit a clear error and exit non-zero.

### 4. Version pinning with strict match
- **Rationale**: Recipes are contracts. A project pins to a specific version to avoid surprise breaking changes when the catalog updates.
- **Implementation**: `[recipes.<id>].version` must exactly match `recipe.toml`'s `[recipe].version`. Mismatch = sync error.

### 5. Templates with `condition = "not_exists"`
- **Rationale**: Recipes should not clobber user-created files (e.g., `.env`). `not_exists` is the only condition in V1.
- **Future**: `always`, `merge`, `prompt` can be added later.

### 6. No `[recipes.*]` validation during `ai-specs init`
- **Rationale**: `init` bootstraps a minimal manifest. Recipes are opt-in additions, not defaults.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Catalog bloat: `catalog/recipes/` grows large | Acceptable for V1. Future: split catalog into separate repo. |
| Recipe conflicts with user-local skills | Conflict detector covers `ai-specs/skills/` IDs; if user manually created a skill with same ID, recipe sync fails with explicit message. |
| Version pinning becomes tedious | V1 requires exact match. Future: support semver ranges. |
| Template conditions are minimal | V1 has `not_exists`. Document limitation; expand in V2. |
| Sync time increases | Recipes are local disk reads + standard copies. Benchmark if needed; no network for bundled content. |

## Migration Plan

No migration needed. Recipes are additive. Existing projects without `[recipes.*]` continue to work unchanged.

## Open Questions

- Should `ai-specs recipe list` show built-in recipes only, or also scan `catalog/recipes/`? (To be resolved during implementation of card #21.)
- Should recipes be able to declare `project.subrepos` defaults? (Deferred to V2.)
