# Design: worktree-flow recipe modes

## Technical Approach

Add `gate_mode` as a `worktree-flow` recipe config field resolved during `ai-specs sync`, then stamp the resolved value into the materialized `worktree-gate.sh` copy. Runtime behavior stays script-local: the hook reads the stamped value, allows a valid `WORKTREE_GATE_MODE` process override, and never reads `ai-specs.toml` at runtime.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Resolution point | Resolve in `lib/_internal/recipe-materialize.py` through `merge_config()` after catalog defaults and manifest overrides are merged. | Runtime manifest lookup from the hook. | Sync already owns recipe config precedence; runtime lookup would add failure modes and violate the spec. |
| Stamping mechanism | Pass `merged_cfg` to `materialize_hook_script()` and replace a catalog-script placeholder with the resolved `gate_mode` in the project copy. | Export `WORKTREE_GATE_MODE` via resolved hook env. | Harness renderers merge `{...process.env, ...ENV}`; exporting the mode would override the user’s one-shot env bypass. A stamped constant preserves override precedence. |
| Validation | Add `[config.gate_mode]` with default `always` and enum validation in recipe schema/sync validation. | Regex-only validation. | The spec requires a diagnostic listing `always | ask | off`; enum validation makes that contract explicit and reusable. |
| `ask` semantics | Block like `always`, but append a bypass hint naming `WORKTREE_GATE_MODE=off`. | TTY prompt in Bash. | Hooks are non-interactive across harnesses; confirmation belongs to the orchestrator, not stdin. |

## Data Flow

```text
ai-specs.toml [recipes.worktree-flow.config.gate_mode]
        │
        ▼
recipe-materialize.merge_config(defaults → manifest override)
        │
        ├─ validate-config enum check
        ▼
materialize_hook_script(..., merged_cfg)
        │ stamps catalog hook placeholder
        ▼
ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh
        │ runtime: WORKTREE_GATE_MODE if valid, else stamped mode
        ▼
always / ask / off dispatch
```

## File Changes

| File | Action | Description |
|---|---|---|
| `catalog/recipes/worktree-flow/recipe.toml` | Modify | Add `gate_mode` default `always` and allowed values. |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Modify | Add stamped mode constant, override resolution, mode dispatch, and ask hint. |
| `lib/_internal/recipe_schema.py` | Modify | Allow enum validation metadata on config fields. |
| `lib/_internal/recipe-materialize.py` | Modify | Validate enum fields and stamp `gate_mode` into runtime hook copies. Do not export `WORKTREE_GATE_MODE` in hook env. |
| `catalog/recipes/worktree-flow/README.md` | Modify | Document modes, default, `ask` caveat, and one-shot override. |
| `docs/recipes-catalog.md`, `docs/runtime-hooks.md`, `docs/ai-specs-toml.md`, `docs/recipe-schema.md` | Modify | Keep manifest, recipe catalog, hook config flow, and schema docs aligned. |
| `tests/test_worktree_gate_hook.py`, `tests/test_worktree_flow_recipe.py` | Modify | Cover runtime modes, stamping, defaults, and invalid config. |

## Interfaces / Contracts

`recipes.worktree-flow.config.gate_mode`: `always | ask | off`, default `always`.

Hook resolution contract:

```bash
stamped_gate_mode="always" # sync replaces this in the materialized copy
gate_mode="${WORKTREE_GATE_MODE:-$stamped_gate_mode}"
```

Only valid env values override. Empty/unset env falls back to the stamped value. Invalid env values warn and fall back to stamped; invalid/missing stamp falls back to `always`.

## Testing Strategy

| Layer | What to Test | Approach |
|---|---|---|
| Unit | enum schema and sync diagnostics | `test_sync_rejects_invalid_gate_mode`, `test_sync_defaults_to_always` in `tests/test_worktree_flow_recipe.py`. |
| Integration | hook behavior per mode | Extend `tests/test_worktree_gate_hook.py` for `always`, `off`, `ask`, env override, empty env fallback, and linked worktree allow. |
| Docs/generated | docs match schema and rendered hook | Assert materialized hook contains stamped mode and docs list all modes. Final verification remains `./tests/validate.sh`. |

## Migration / Rollout

No migration required. Existing manifests omit `gate_mode` and continue as `always`. Rollback is safe: revert the config field and hook dispatch; existing projects fall back to the current strict gate. If stamping fails during sync, fail before writing hook wiring; if a bad stamped hook somehow runs, fallback to `always` preserves trunk protection.

## Open Questions

None.
