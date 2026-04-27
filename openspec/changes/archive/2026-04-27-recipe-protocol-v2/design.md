# Design: recipe-protocol-v2

## Overview

This document details the technical design for adding capabilities, hooks, and per-recipe configuration to the recipe protocol (V2). The design preserves full V1 backward compatibility while introducing new sync-time semantics.

## Architecture Decisions

1. **Additive Schema Only**  
   V2 tables (`[[capabilities]]`, `[[hooks]]`, `[config]`) are strictly optional. `recipe_schema.py` short-circuits to empty collections when tables are absent, ensuring V1 recipes require zero changes.

2. **Binding Resolution as a Pre-Materialization Phase**  
   Before any file writes, `recipe-materialize.py` resolves the capability-to-recipe binding map. This allows conflict detection and config validation to fail fast before primitives are copied.

3. **Auto-Bind Ambiguity = Warning (Non-Fatal)**  
   The `auto-binding` spec states that ambiguity must not cause sync failure. The `capability-conflicts` spec originally contained a scenario requiring failure for the same condition. **Reconciled resolution:** treat ambiguity as a logged warning and do not auto-bind. This aligns with the principle of least surprise and keeps V2 opt-in. The `capability-conflicts` spec has been updated to match (scenario renamed from "Auto-bind ambiguity causes conflict" to "Auto-bind ambiguity prevents binding with warning").

4. **Config Lives in Manifest, Schema in Recipe**  
   `recipe.toml` declares defaults and requirements; `ai-specs.toml` provides per-project overrides. This keeps recipes reusable and prevents secret or environment-specific values from leaking into the catalog.

5. **Hooks Execute Post-Materialization**  
   Hooks run after all primitives (skills, commands, templates, docs) are on disk. This lets `validate-config` actions inspect the materialized filesystem if needed.

6. **Reuse `Conflict` Dataclass for Capabilities**  
   `recipe-conflicts.py` already models collisions with `(type, id, recipes)`. Capability ambiguity fits this shape naturally. A `severity` field (`"fatal" | "warning"`) is added so callers can decide whether to fail or warn.

---

## Schema Extensions

### `recipe.toml` V2 Additions

```toml
[[capabilities]]
id = "tracker"

[[capabilities]]
id = "canonical-memory"

[config.board_id]
required = true
type = "string"

[config.timeout]
required = false
type = "integer"
default = 30

[[hooks]]
event = "on-sync"
action = "validate-config"
```

### `ai-specs.toml` V2 Additions

```toml
[[bindings]]
capability = "tracker"
recipe = "trello-pm"

[recipes.trello-pm]
enabled = true
version = "2.0.0"

[recipes.trello-pm.config]
board_id = "69ec0a2099ea20956e371d62"
timeout = 60
```

---

## Module Changes

### `lib/_internal/recipe_schema.py`

**New dataclasses:**
- `Capability(id: str)`
- `Hook(event: str, action: str)`
- `ConfigField(name: str, required: bool, type: str, default: Any)`
- `ConfigSchema(fields: dict[str, ConfigField])`

**Extend `Recipe`:**
```python
@dataclass
class Recipe:
    ...existing fields...
    capabilities: list[Capability] = field(default_factory=list)
    hooks: list[Hook] = field(default_factory=list)
    config_schema: ConfigSchema = field(default_factory=ConfigSchema)
```

**New parser helpers:**
- `_parse_capabilities(raw, context) -> list[Capability]`
  - Validates `id` is non-empty kebab-case string.
  - Fails on duplicate ids within the same recipe.
- `_parse_hooks(raw, context) -> list[Hook]`
  - Validates both `event` and `action` are non-empty strings.
- `_parse_config(raw, context) -> ConfigSchema`
  - Each key becomes a `ConfigField`.
  - `required` (bool) is mandatory per field.
  - `type` (str) and `default` (any) are optional.

**Map to specs:**
- `capability-declaration` → `_parse_capabilities`
- `sync-hooks` → `_parse_hooks`
- `recipe-config` → `_parse_config`

---

### `lib/_internal/toml-read.py`

**Extend `read_recipes`:**
Include `config` dict per recipe:
```python
out[recipe_id] = {
    "enabled": bool(enabled),
    "version": str(version),
    "config": dict(value.get("config", {})),
}
```

**New function `read_bindings`:**
```python
def read_bindings(data: dict[str, Any]) -> list[dict[str, str]]:
    raw = data.get("bindings", [])
    if not isinstance(raw, list):
        return []
    return [
        {"capability": str(b.get("capability", "")), "recipe": str(b.get("recipe", ""))}
        for b in raw if isinstance(b, dict)
    ]
```

**Update `read_section`:**
Add `"bindings"` branch.

**Map to specs:**
- `capability-binding` → `read_bindings`
- `recipe-config` → extended `read_recipes`

---

### `lib/_internal/recipe-conflicts.py`

**Extend `Conflict` dataclass:**
```python
@dataclass
class Conflict:
    primitive_type: str
    primitive_id: str
    recipes: set[str] = field(default_factory=set)
    severity: str = "fatal"  # "fatal" | "warning"
```

**New function `check_capability_conflicts`:**
```python
def check_capability_conflicts(
    catalog_dir: Path,
    recipe_ids: list[str],
    explicit_bindings: list[dict[str, str]],
) -> list[Conflict]:
    """
    Load enabled recipes, group by capability.
    For each capability with >1 provider:
      - If no explicit binding exists -> Conflict(severity="warning")
    """
```

**Keep existing `check_recipe_conflicts` unchanged** for primitive (skill/command/mcp) collisions. Those remain `severity="fatal"`.

**Map to specs:**
- `capability-conflicts` → `check_capability_conflicts`
- `backward-compatibility` (V1 conflicts) → existing `check_recipe_conflicts`

---

### `lib/_internal/recipe-materialize.py`

**New internal helpers:**

1. `resolve_bindings(enabled_recipes, manifest_bindings) -> dict[str, str]`
   - **Step 1 – Explicit:** Validate each binding (recipe enabled, recipe declares capability). Fail fast on invalid binding.
   - **Step 2 – Auto-bind:** For each capability declared by exactly one enabled recipe and not already explicitly bound, create implicit binding.
   - **Step 3 – Return:** Map `capability_id -> recipe_id`.

2. `merge_config(recipe: Recipe, manifest_config: dict[str, Any]) -> dict[str, Any]`
   - Start with defaults from `recipe.config_schema`.
   - Overlay `manifest_config`.
   - Fail if any `required=True` field is missing.
   - Warn if manifest provides keys not in schema.

3. `execute_hooks(recipe: Recipe, merged_config: dict[str, Any]) -> None`
   - Iterate `recipe.hooks` in declaration order.
   - Dispatch on `action`:
     - `"validate-config"`: verify required config fields are present and non-empty.
     - Unknown actions: emit warning and skip.
   - Any raised exception causes sync to fail with recipe name and action.

**Modify `materialize_recipes` pipeline:**

```text
load recipes from manifest
load Recipe objects for enabled recipes

--- NEW PHASE: Binding Resolution ---
resolved = resolve_bindings(enabled_recipes, manifest_bindings)

--- NEW PHASE: Capability Conflict Check ---
cap_conflicts = check_capability_conflicts(catalog_dir, enabled_ids, manifest_bindings)
for c in cap_conflicts:
    if c.severity == "fatal":
        fail(...)
    else:
        warn(...)

--- EXISTING PHASE: Primitive Conflict Check ---
primitive_conflicts = check_recipe_conflicts(catalog_dir, enabled_ids)
if primitive_conflicts: fail(...)

for each enabled recipe:
    validate_version_pin(...)
    merged_cfg = merge_config(recipe, manifest_config)
    materialize primitives (skills, commands, templates, docs)
    execute_hooks(recipe, merged_cfg)   # NEW

write .recipe-mcp.json
```

**Map to specs:**
- `capability-binding` → `resolve_bindings` (explicit validation)
- `auto-binding` → `resolve_bindings` (implicit step)
- `recipe-config` → `merge_config`
- `sync-hooks` → `execute_hooks`
- `backward-compatibility` → existing materialize primitives path is untouched when V2 tables are absent

---

### `lib/sync.sh`

No changes required. The existing call to `recipe-materialize.py` remains:
```bash
python3 "$RECIPE_MATERIALIZE_PY" "$ROOT_PATH" "$AI_SPECS_HOME"
```
All new logic is encapsulated inside the Python orchestrator.

---

### `docs/recipe-schema.md`

Add new sections after the V1 documentation:
- `[[capabilities]]` table syntax and examples
- `[config]` schema declaration
- `[[hooks]]` lifecycle events
- `[[bindings]]` in manifest
- `[recipes.<id>.config]` override syntax
- Backward compatibility note

---

## Sequence Diagrams

### 1. Binding Resolution (Pre-Materialization)

```
Manifest (ai-specs.toml)          recipe-materialize.py              Catalog Recipes
        |                                  |                                  |
        |  [recipes.*] + [[bindings]]      |                                  |
        +--------------------------------->|                                  |
        |                                  |  read recipe.toml for each enabled|
        |                                  +--------------------------------->|
        |                                  |  capabilities + hooks + config   |
        |                                  |<---------------------------------+
        |                                  |                                  |
        |                                  |  resolve_bindings()              |
        |                                  |  ------------------------------- |
        |                                  |  1. Validate explicit bindings   |
        |                                  |     (recipe enabled? declares    |
        |                                  |      capability?)                |
        |                                  |  2. Auto-bind unambiguous caps   |
        |                                  |     (exactly 1 provider)         |
        |                                  |                                  |
        |                                  |  check_capability_conflicts()    |
        |                                  |  ------------------------------- |
        |                                  |  Ambiguity? -> WARN              |
        |                                  |  Duplicate explicit? -> FAIL     |
        |                                  |                                  |
        |                                  v                                  |
```

### 2. Per-Recipe Materialization + Hooks

```
recipe-materialize.py
        |
        v
+-----------------+
| merge_config()  |
|  - defaults     |
|  - overrides    |
|  - validate req |
+--------+--------+
         |
         v
+-----------------+
| Materialize     |
|  - skills       |
|  - commands     |
|  - templates    |
|  - docs         |
+--------+--------+
         |
         v
+-----------------+
| execute_hooks() |
|  - validate-cfg |
|  - unknown ->   |
|    warn & skip  |
+--------+--------+
         |
         v
   Next Recipe
```

### 3. Full Sync Pipeline (V2 Insertions)

```
sync.sh
   |
   v
recipe-materialize.py
   |
   +--> toml-read.py          (manifest)
   +--> recipe_schema.py      (catalog recipes)
   |
   v
Binding Resolution
   |
   v
Capability Conflict Check    (NEW)
   |
   v
Primitive Conflict Check     (EXISTING)
   |
   v
+---------------------------+
| For each enabled recipe:  |
|  - version pin            |
|  - merge config           |  (NEW)
|  - materialize primitives |
|  - execute hooks          |  (NEW)
+---------------------------+
   |
   v
Write .recipe-mcp.json
```

---

## Data Flow: Config from Manifest to Hooks

```
recipe.toml
[config.timeout]
required = false
type = "integer"
default = 30

        |
        v
   ConfigSchema
   fields["timeout"] = ConfigField(default=30, required=False, type="integer")
        |
        +------------------+
                           |
ai-specs.toml              |
[recipes.my-recipe.config] |
timeout = 60               |
                           |
        +------------------+
        |
        v
   merge_config()
   result = {"timeout": 60}
        |
        v
   execute_hooks()
   hook context receives {"timeout": 60}
```

---

## Error Handling

| Failure Mode | Source Module | Exception / Behavior |
|---|---|---|
| Missing/empty capability `id` | `recipe_schema.py` | `RecipeValidationError` |
| Duplicate capability in recipe | `recipe_schema.py` | `RecipeValidationError` |
| Missing hook `event` or `action` | `recipe_schema.py` | `RecipeValidationError` |
| Invalid config field structure | `recipe_schema.py` | `RecipeValidationError` |
| Binding references disabled recipe | `recipe-materialize.py` | `RuntimeError` |
| Binding references unknown recipe | `recipe-materialize.py` | `RuntimeError` |
| Binding references recipe without capability | `recipe-materialize.py` | `RuntimeError` |
| Duplicate explicit bindings | `recipe-materialize.py` | `RuntimeError` |
| Missing required config value | `recipe-materialize.py` | `RuntimeError` |
| Hook action failure | `recipe-materialize.py` | `RuntimeError` |
| Primitive conflict (skill/cmd/mcp) | `recipe-conflicts.py` | `ConflictError` (fatal) |
| Capability ambiguity | `recipe-conflicts.py` | `Conflict` (warning, logged) |

---

## Testing Strategy

- **Unit tests** for `recipe_schema.py` covering every scenario in `capability-declaration`, `sync-hooks`, and `recipe-config` specs.
- **Unit tests** for `toml-read.py` ensuring `read_bindings` and extended `read_recipes` parse correctly.
- **Unit tests** for `recipe-conflicts.py` capability grouping and ambiguity detection.
- **Integration tests** for `recipe-materialize.py` using temporary directories to simulate full sync runs with V2 recipes.
- **Regression tests** using V1-only recipes and manifests to guarantee `backward-compatibility` spec compliance.

---

## Risks & Open Items

1. **Spec Reconciliation (Auto-Bind Ambiguity)** ✅ **RESOLVED**  
   `capability-conflicts` originally required failure on ambiguity; `auto-binding` requires non-failure. **Reconciled resolution:** ambiguity prevents auto-bind and emits a warning; sync does not fail. The `capability-conflicts` spec has been updated (scenario renamed and behavior changed from fatal to warning). Both specs are now consistent.

2. **Hook Action Extensibility**  
   Only `validate-config` is in scope. Future actions will require a formal dispatch registry. For now, a simple `if/elif` chain in `recipe-materialize.py` is sufficient.

3. **Config Type Enforcement**  
   The `type` field in config schema is recorded but not strictly validated at runtime (e.g., no integer parsing). This matches MVP scope; strict type checking can be added later without breaking the schema.
