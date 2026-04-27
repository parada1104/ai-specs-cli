# Proposal: recipe-protocol-v2

## Change

- **Name:** `recipe-protocol-v2`
- **Card:** #54 "Diseñar protocolo de capabilities y hooks para recipes"
- **EPIC:** 3 — Recipe Protocol Foundation

---

## Why

Recipes in V1 are static bundles of primitives (skills, commands, MCP presets, templates, docs). They materialize during `ai-specs sync` but have no awareness of the project context, cannot declare semantic roles, and cannot react to lifecycle events. This limits recipes to passive asset delivery.

The project needs recipes to become dynamic actors in the development workflow:

1. **Semantic binding:** A project should declare "I need a tracker" and bind that capability to a concrete recipe (e.g., `trello-pm`), enabling swappable implementations.
2. **Sync-time hooks:** Recipes should react to materialization events (e.g., a tracker recipe validating its board ID during sync, or a canonical-memory recipe ensuring its directory structure exists).
3. **Recipe config:** Recipes need per-project configuration (e.g., `board_id`, `memory_path`) without hardcoding defaults in the recipe itself.

These capabilities are prerequisites for:
- Card #55: Refactor `trello-pm-workflow` skill → recipe `trello-pm` v2
- Card #59: Implement `canonical-memory-filesystem` recipe with protocol v2
- Card #57: Implement `recipe-orchestrator` skill

---

## What Changes

### 1. recipe.toml V2 schema additions

Backward-compatible additions to `recipe.toml`:

- **`[[capabilities]]`** — semantic roles this recipe fulfills.
  ```toml
  [[capabilities]]
  id = "tracker"
  ```

- **`[[hooks]]`** — sync-time lifecycle hooks.
  ```toml
  [[hooks]]
  event = "on-sync"
  action = "validate-config"
  ```

- **`[config]`** — recipe-level config schema declaration (defaults and validation hints).
  ```toml
  [config]
  board_id = { required = true, type = "string" }
  ```

V1 tables (`[recipe]`, `[provides]`) remain unchanged and required.

### 2. ai-specs.toml manifest additions

- **`[[bindings]]`** — capability → recipe mapping.
  ```toml
  [[bindings]]
  capability = "tracker"
  recipe = "trello-pm"
  ```

- **`[recipes.<id>.config]`** — per-recipe instance configuration.
  ```toml
  [recipes.trello-pm]
  enabled = true
  version = "2.0.0"

  [recipes.trello-pm.config]
  board_id = "69ec0a2099ea20956e371d62"
  ```

### 3. Sync pipeline changes

- `recipe_schema.py` updated to parse V2 tables (capabilities, hooks, config).
- `recipe-materialize.py` updated to:
  - Validate bindings: every bound capability must map to an enabled recipe that declares that capability.
  - Auto-bind: if exactly one enabled recipe declares a capability, bind implicitly (no explicit `[[bindings]]` required).
  - Execute sync-time hooks after materialization of primitives.
  - Merge recipe config with recipe-level defaults.
- `recipe-conflicts.py` updated to detect capability conflicts (two recipes claiming the same capability when both are enabled and explicitly/implicitly bound).

### 4. Documentation updates

- `docs/recipe-schema.md` updated to describe V2 additions.
- New spec files for each capability domain (created in later phases).

---

## Capabilities

The following kebab-case capability identifiers are introduced by this change. Each will require a dedicated spec file in the `specs/` phase.

| Capability | Description | Example Recipe |
|-----------|-------------|----------------|
| `tracker` | Project management integration (boards, cards, lists) | `trello-pm` |
| `canonical-memory` | Structured, persistent memory storage for decisions, handoffs, and context | `canonical-memory-filesystem` |
| `sdd-tracker` | SDD workflow state tracking (phases, changes, verification) | `trello-pm` |

Recipes MAY declare multiple capabilities. A capability MAY be implemented by multiple recipes in the catalog, but at most ONE recipe MAY be bound to a given capability in a single project manifest.

---

## Impact

### For users
- Projects can now declare intent via capabilities (`tracker`, `canonical-memory`) instead of hardcoding recipe IDs.
- Recipes are configurable per-project via `[recipes.<id>.config]`.
- `ai-specs sync` will fail fast with clear errors if a required capability has no binding or conflicting bindings.

### For recipe authors
- Recipes can declare semantic roles via `[[capabilities]]`, making them discoverable and swappable.
- Recipes can define config schemas in `[config]` and receive validated values from the manifest.
- Recipes can hook into sync-time events for validation and setup.

### For agent behavior
- The `recipe-orchestrator` skill (future card #57) can read bindings and capabilities to orchestrate recipe interactions.
- Agents can suggest recipes based on missing capabilities in the manifest.

---

## Rollback Plan

1. **Code rollback:** Revert changes to `recipe_schema.py`, `recipe-materialize.py`, `recipe-conflicts.py`, and `toml-read.py`.
2. **Schema rollback:** Remove `[[capabilities]]`, `[[hooks]]`, and `[config]` tables from recipe.toml; remove `[[bindings]]` and `[recipes.<id>.config]` from ai-specs.toml.
3. **Recipe rollback:** Recipes written for V2 (e.g., `trello-pm` v2) can degrade gracefully by omitting V2 tables — they become V1 bundles without dynamic behavior.
4. **Manifest rollback:** Remove `[[bindings]]` and `.config` sections; V1 `[recipes.<id>]` tables continue to function.

No destructive data operations are involved. The risk is low because all changes are additive and opt-in.

---

## Scope

### In Scope
- Capabilities declaration in `recipe.toml` (`[[capabilities]]`).
- Capability binding in `ai-specs.toml` (`[[bindings]]` + auto-bind).
- Per-recipe config in manifest (`[recipes.<id>.config]`).
- Sync-time hooks (`[[hooks]]` with `event` and `action`).
- Validation: binding consistency, capability conflicts, config presence.
- Backward compatibility with recipe.toml V1.
- Updates to `recipe_schema.py`, `recipe-materialize.py`, `recipe-conflicts.py`, `toml-read.py`.
- Updates to `docs/recipe-schema.md`.

### Out of Scope (deferred to follow-up changes)
- **Session-time hooks:** Runtime hooks fired during agent sessions (e.g., `on-sdd-phase-change` triggering Trello card moves). This requires the `recipe-orchestrator` skill (card #57) and session instrumentation.
- **Handlers:** Cross-recipe event subscription (`[[handlers]]` listening to hooks from other recipes). Depends on session-time hook infrastructure.
- **`extends` / recipe inheritance:** Allowing recipes to extend other recipes. Complex dependency graph resolution; defer until canonical-memory variants (filesystem vs vault vs obsidian) justify it.
- **Hook action implementations beyond `validate-config`:** Actual business logic for hooks lives in recipe-specific skills/commands, not in the protocol itself.
- **Recipe-orchestrator skill:** Card #57 is a separate change.

---

## Key Decisions

1. **MVP = capabilities + bindings + sync-time hooks.** Session-time orchestration is explicitly deferred to keep this change bounded and shippable.
2. **Auto-bind for unambiguous capabilities.** If only one enabled recipe provides `tracker`, no explicit `[[bindings]]` is needed. This reduces boilerplate.
3. **Config lives in manifest, not recipe.toml.** Recipe.toml declares the schema (defaults, required fields); per-project values live in `ai-specs.toml` under `[recipes.<id>.config]`. This keeps recipes reusable across projects.
4. **Hooks are sync-time only.** They execute during `ai-specs sync` as part of materialization. This aligns with the existing pipeline and avoids runtime complexity.
5. **Backward compatibility is mandatory.** V1 recipes without `[[capabilities]]`, `[[hooks]]`, or `[config]` must continue to materialize exactly as before.
