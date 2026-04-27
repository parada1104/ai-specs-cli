# Tasks: recipe-protocol-v2

Change: `recipe-protocol-v2`  
Card: #54 "Diseñar protocolo de capabilities y hooks para recipes"  
Design reference: `openspec/changes/recipe-protocol-v2/design.md`

---

## Phase 1 — Infrastructure / Schema

> Maps to Design sections: **Schema Extensions**, **Module Changes: `lib/_internal/recipe_schema.py`** and **`lib/_internal/toml-read.py`**

### 1.1 Extend `Recipe` dataclass with V2 fields
- [ ] Add `Capability`, `Hook`, `ConfigField`, `ConfigSchema` dataclasses to `lib/_internal/recipe_schema.py`.
- [ ] Extend `Recipe` with `capabilities: list[Capability]`, `hooks: list[Hook]`, `config_schema: ConfigSchema`.
- [ ] All new fields default to empty collections so V1 recipes require zero changes.

### 1.2 Implement `_parse_capabilities()` parser
- [ ] Add `_parse_capabilities(raw, context) -> list[Capability]` to `lib/_internal/recipe_schema.py`.
- [ ] Validate `id` is non-empty kebab-case string; fail with `RecipeValidationError` otherwise.
- [ ] Detect duplicate capability ids within the same recipe and fail with explicit error.
- [ ] Return empty list when `raw` is not a list (backward compatibility).

### 1.3 Implement `_parse_hooks()` parser
- [ ] Add `_parse_hooks(raw, context) -> list[Hook]` to `lib/_internal/recipe_schema.py`.
- [ ] Validate both `event` and `action` are non-empty strings; fail with `RecipeValidationError` otherwise.
- [ ] Return empty list when `raw` is not a list.

### 1.4 Implement `_parse_config()` parser
- [ ] Add `_parse_config(raw, context) -> ConfigSchema` to `lib/_internal/recipe_schema.py`.
- [ ] Each key in `raw` becomes a `ConfigField` with `required` (bool, mandatory), `type` (str, optional), `default` (any, optional).
- [ ] Fail with `RecipeValidationError` for invalid field structure (missing `required`, wrong type).
- [ ] Return empty `ConfigSchema` when `raw` is not a dict.

### 1.5 Integrate V2 parsers into `validate_recipe_toml()`
- [ ] Call `_parse_capabilities`, `_parse_hooks`, `_parse_config` inside `validate_recipe_toml()`.
- [ ] Populate the new `Recipe` fields from the parsed results.
- [ ] Verify that existing V1 recipes still parse correctly (no V2 tables → empty collections).

### 1.6 Extend `toml-read.py` for manifest V2 additions
- [ ] Extend `read_recipes()` to include `config` dict per recipe (reads `[recipes.<id>.config]`).
- [ ] Add `read_bindings(data) -> list[dict[str, str]]` to parse top-level `[[bindings]]` tables.
- [ ] Add `"bindings"` branch to `read_section()`.
- [ ] Ensure absent `bindings` or `config` returns safe defaults (empty list / empty dict).

---

## Phase 2 — Implementation

> Maps to Design sections: **Module Changes: `lib/_internal/recipe-conflicts.py`** and **`lib/_internal/recipe-materialize.py`**

### 2.1 Extend `Conflict` dataclass with severity
- [ ] Add `severity: str = "fatal"` field to `Conflict` in `lib/_internal/recipe-conflicts.py`.
- [ ] Accept `"fatal" | "warning"` values.
- [ ] Existing primitive conflicts remain `"fatal"`.

### 2.2 Implement `check_capability_conflicts()`
- [ ] Add `check_capability_conflicts(catalog_dir, recipe_ids, explicit_bindings) -> list[Conflict]` to `lib/_internal/recipe-conflicts.py`.
- [ ] Load enabled recipes and group by declared capability.
- [ ] For each capability with >1 provider and no explicit binding, return `Conflict(severity="warning")`.
- [ ] For duplicate explicit bindings for same capability, return `Conflict(severity="fatal")`.
- [ ] Keep existing `check_recipe_conflicts()` unchanged.

### 2.3 Implement `resolve_bindings()`
- [ ] Add `resolve_bindings(enabled_recipes, manifest_bindings) -> dict[str, str]` helper to `lib/_internal/recipe-materialize.py`.
- [ ] **Step 1 – Explicit:** Validate each binding (recipe enabled, recipe declares capability). Fail fast (`RuntimeError`) on invalid binding.
- [ ] **Step 2 – Auto-bind:** For each capability declared by exactly one enabled recipe and not already explicitly bound, create implicit binding.
- [ ] Return map `capability_id -> recipe_id`.

### 2.4 Implement `merge_config()`
- [ ] Add `merge_config(recipe: Recipe, manifest_config: dict[str, Any]) -> dict[str, Any]` to `lib/_internal/recipe-materialize.py`.
- [ ] Start with defaults from `recipe.config_schema`.
- [ ] Overlay `manifest_config` values.
- [ ] Fail (`RuntimeError`) if any `required=True` field is missing in the final dict.
- [ ] Warn if manifest provides keys not in the schema.

### 2.5 Implement `execute_hooks()`
- [ ] Add `execute_hooks(recipe: Recipe, merged_config: dict[str, Any]) -> None` to `lib/_internal/recipe-materialize.py`.
- [ ] Iterate `recipe.hooks` in declaration order.
- [ ] Dispatch on `action`:
  - `"validate-config"`: verify required config fields are present and non-empty.
  - Unknown actions: emit warning and skip.
- [ ] Any raised exception causes sync to fail with recipe name and action in the error.

### 2.6 Integrate V2 phases into `materialize_recipes()` pipeline
- [ ] Insert **Binding Resolution** phase before primitive conflict check.
- [ ] Insert **Capability Conflict Check** phase (warn for ambiguity, fatal for duplicate explicit).
- [ ] For each enabled recipe:
  - Call `merge_config()` before materialization.
  - Pass merged config through materialization (no config fields are written; config is only for hook context).
  - Call `execute_hooks()` after all primitives are materialized.
- [ ] Ensure V1 recipes (no capabilities/hooks/config) continue to materialize exactly as before.

---

## Phase 3 — Testing

> Maps to Design section: **Testing Strategy**

### 3.1 Unit tests for `_parse_capabilities()`
- [ ] Valid single capability declaration.
- [ ] Valid multiple capability declarations.
- [ ] Missing capability id → `RecipeValidationError`.
- [ ] Empty capability id → `RecipeValidationError`.
- [ ] No capabilities → empty list.
- [ ] Duplicate capability ids within a recipe → `RecipeValidationError`.

### 3.2 Unit tests for `_parse_hooks()`
- [ ] Valid hook declaration (`event="on-sync"`, `action="validate-config"`).
- [ ] Missing hook event → `RecipeValidationError`.
- [ ] Missing hook action → `RecipeValidationError`.
- [ ] No hooks → empty list.

### 3.3 Unit tests for `_parse_config()`
- [ ] Valid config schema with required field.
- [ ] Valid config schema with default value.
- [ ] No config schema → empty `ConfigSchema`.
- [ ] Missing `required` in field → `RecipeValidationError`.
- [ ] Invalid `required` type → `RecipeValidationError`.

### 3.4 Unit tests for `toml-read.py` V2 additions
- [ ] `read_recipes()` returns `config` dict when `[recipes.<id>.config]` is present.
- [ ] `read_recipes()` returns empty `config` dict when absent.
- [ ] `read_bindings()` returns list of `{capability, recipe}` for valid `[[bindings]]`.
- [ ] `read_bindings()` returns empty list when `bindings` is absent.
- [ ] `read_bindings()` returns empty list when `bindings` is not a list.
- [ ] `read_section("bindings")` dispatches correctly.

### 3.5 Unit tests for `recipe-conflicts.py` capability conflicts
- [ ] `check_capability_conflicts()` returns warning-level conflict when two enabled recipes declare same capability with no explicit binding.
- [ ] Returns fatal-level conflict when duplicate explicit bindings exist for same capability.
- [ ] Returns empty list when only one recipe declares the capability (auto-bind allowed).
- [ ] Returns empty list when capability has multiple providers but explicit binding resolves ambiguity.
- [ ] Disabled recipes are excluded from conflict detection.

### 3.6 Integration tests for `resolve_bindings()`
- [ ] Explicit binding accepted when recipe enabled and declares capability.
- [ ] Explicit binding referencing disabled recipe → `RuntimeError`.
- [ ] Explicit binding referencing recipe without capability → `RuntimeError`.
- [ ] Explicit binding referencing unknown recipe → `RuntimeError`.
- [ ] Duplicate explicit bindings for same capability → `RuntimeError`.
- [ ] No bindings declared → auto-bind evaluated, no error.
- [ ] Auto-bind succeeds for single provider.
- [ ] Auto-bind skipped for multiple providers (no error).
- [ ] Auto-bind skipped for zero providers (no error).
- [ ] Explicit binding overrides auto-bind.
- [ ] Disabled recipe excluded from auto-bind count.

### 3.7 Integration tests for `merge_config()`
- [ ] Manifest overrides default value.
- [ ] Manifest provides required value → proceeds without error.
- [ ] Missing required config value → `RuntimeError` naming the field.
- [ ] Manifest config for recipe without schema → warning emitted, no failure.
- [ ] Unknown config keys in manifest → warning emitted, no failure.

### 3.8 Integration tests for `execute_hooks()`
- [ ] Successful `validate-config` hook when all required fields present.
- [ ] Hook validation failure when required config missing → `RuntimeError` with recipe name and action.
- [ ] Hooks execute in declaration order.
- [ ] Unknown action → warning emitted, sync continues.

### 3.9 Integration tests for full V2 materialize pipeline
- [ ] End-to-end sync with one V2 recipe (capabilities + config + hooks) succeeds.
- [ ] End-to-end sync with mixed V1 and V2 recipes succeeds, V1 behavior unchanged.
- [ ] V2 recipe without V2 tables materializes exactly like V1.
- [ ] Capability conflict warning is logged but sync proceeds (per reconciled spec).
- [ ] Duplicate explicit binding causes sync to fail before materialization.

### 3.10 Regression tests for V1 backward compatibility
- [ ] V1 recipe without V2 tables parses and materializes exactly as before.
- [ ] V1 manifest without bindings or recipe config parses correctly.
- [ ] Manifest with no recipes section proceeds normally.
- [ ] V1 skill/command/mcp conflict detection still works unchanged.
- [ ] Existing unit tests for `recipe_schema.py`, `toml-read.py`, `recipe-conflicts.py`, and `recipe-materialize.py` continue to pass.

---

## Phase 4 — Documentation

> Maps to Design section: **docs/recipe-schema.md**

### 4.1 Update `docs/recipe-schema.md` with recipe.toml V2 additions
- [ ] Document `[[capabilities]]` table syntax, `id` field, and kebab-case requirement.
- [ ] Document `[config]` schema declaration (`required`, `type`, `default`).
- [ ] Document `[[hooks]]` lifecycle events (`event`, `action`), with `on-sync` / `validate-config` example.
- [ ] Add backward compatibility note: V2 tables are optional, V1 recipes require no changes.

### 4.2 Update `docs/recipe-schema.md` with manifest V2 additions
- [ ] Document `[[bindings]]` table syntax (`capability`, `recipe`).
- [ ] Document `[recipes.<id>.config]` override syntax.
- [ ] Document auto-binding behavior (single provider, ambiguity warning).
- [ ] Document config merge rules (defaults + overrides, required validation).

---

## Phase 5 — Spec Reconciliation ✅ COMPLETED

> Maps to Design section: **Risks & Open Items #1** and **Architecture Decision #3**

### 5.1 Reconcile the auto-binding vs capability-conflicts contradiction
- [x] Document the contradiction in `tasks.md`:
  - `auto-binding` spec says: "multiple providers prevent auto-bind, sync SHALL NOT fail solely due to unbound capability."
  - `capability-conflicts` spec says: "auto-bind ambiguity causes conflict, sync SHALL fail."
- [x] Confirm design resolution (warning, non-fatal) aligns with project principle of least surprise and V2 opt-in behavior.

### 5.2 Update spec files to reflect reconciled behavior
- [x] Update `capability-conflicts/spec.md`: change the "auto-bind ambiguity causes conflict" scenario from **fail** to **warn + do not auto-bind**.
  - New expected behavior: ambiguity prevents auto-bind, emits a warning, and sync proceeds (capability remains unbound).
- [x] Ensure `auto-binding/spec.md` remains unchanged (already matches the reconciled behavior).
- [x] Verify all other `capability-conflicts` scenarios still hold:
  - Explicit binding resolves ambiguity → no conflict.
  - Only one recipe enabled → auto-bind, no conflict.
  - Unbound capability with ambiguity → warning, no fail.

### 5.3 Update design.md with final reconciliation decision
- [x] Add explicit note in `design.md` Architecture Decision #3 confirming the chosen resolution.
- [x] Reference the updated `capability-conflicts/spec.md` as the reconciled source of truth.

---

## Traceability Matrix

| Task | Design Section | Spec Domain |
|------|---------------|-------------|
| 1.1–1.5 | `lib/_internal/recipe_schema.py` | `capability-declaration`, `sync-hooks`, `recipe-config` |
| 1.6 | `lib/_internal/toml-read.py` | `capability-binding`, `recipe-config` |
| 2.1–2.2 | `lib/_internal/recipe-conflicts.py` | `capability-conflicts` |
| 2.3 | `resolve_bindings()` | `capability-binding`, `auto-binding` |
| 2.4 | `merge_config()` | `recipe-config` |
| 2.5 | `execute_hooks()` | `sync-hooks` |
| 2.6 | `materialize_recipes()` pipeline | all domains |
| 3.1 | — | `capability-declaration` |
| 3.2 | — | `sync-hooks` |
| 3.3 | — | `recipe-config` |
| 3.4 | — | `capability-binding`, `recipe-config` |
| 3.5 | — | `capability-conflicts` |
| 3.6 | — | `capability-binding`, `auto-binding` |
| 3.7 | — | `recipe-config` |
| 3.8 | — | `sync-hooks` |
| 3.9 | — | all domains |
| 3.10 | — | `backward-compatibility` |
| 4.1–4.2 | `docs/recipe-schema.md` | all domains |
| 5.1–5.3 | Spec Reconciliation | `auto-binding`, `capability-conflicts` |

---

## Task Execution Order (Suggested)

1. **Phase 1** first (schema infrastructure blocks everything else).
2. **Phase 2** second (implementation depends on schema).
3. **Phase 5** third (reconcile specs before writing tests that depend on the reconciled behavior).
4. **Phase 3** fourth (tests validate implementation against reconciled specs).
5. **Phase 4** last (docs reflect final behavior).
