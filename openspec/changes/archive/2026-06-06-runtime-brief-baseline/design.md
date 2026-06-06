# Design: Runtime Brief Baseline for Fresh Init

## Technical Approach

Two coordinated edits, no new modules. (1) The TOML template ships `[recipes.session-context]`
`enabled = true` (pinned `version`), so a bare project's `build_resolved_config` returns
`enabled = ["session-context"]`. (2) `init.sh`, after writing the manifest (step 3), runs the
SAME materialize→render pipeline `sync.sh:108-116` uses — `recipe-materialize.py` to a temp
resolved-config, then `agents-render.py --preserve-if-runtime-brief --resolved-config <tmp>` —
so the first `AGENTS.md` already carries `## Workflow Rules` + `## Conflict Policy` from the
recipe's universal `[provides.brief]` fragments. Render is best-effort: any non-zero exit falls
back to the existing placeholder write. No renderer/materialize logic changes (PR #75 reused).

## Architecture Decisions

| Decision | Choice | Alternatives → why not | Rationale |
|----------|--------|------------------------|-----------|
| Baseline source | Pre-enable `session-context` in template | New `foundation` recipe → duplicates fragments, adds catalog surface | Single versioned home for foundational prose; auto-updates on sync |
| Init render code path | Inline the same `materialize → render` calls in `init.sh` (mirror sync) | Call `sync.sh` from init → triggers full vendor/fan-out/network before manifest is configured; extract shared lib → larger refactor, out of scope | Two short commands; init stays self-contained and offline |
| `--preserve-if-runtime-brief` on init | Keep the flag | Drop it → fresh render always wins | Nothing to preserve on first init, but flag makes re-init/`--force` idempotent and honors a user marker if AGENTS.md pre-exists |
| Render failure handling | Best-effort guard; on non-zero exit, fall back to placeholder write | Let init abort | Init must succeed offline / on catalog hiccup; brief is regenerated on next sync |
| Idempotency with later sync | Init uses identical flags + resolved-config shape as sync | — | Byte-stable: init-rendered AGENTS.md == sync-rendered for the same manifest |
| Version pin | Pin `version = "2.0.0"` matching catalog `recipe.toml` | Omit version | Consistency with catalog; explicit pin matches existing recipe-block convention |

## Data Flow

    init.sh
      └─ step 3: write ai-specs.toml (from template, session-context enabled)
      └─ step 3b (NEW):
           recipe-materialize.py <root> <home> --resolved-config-out TMP
                                  │  reads catalog/recipes/session-context/recipe.toml (local, offline)
                                  ▼
           resolved-config.json { enabled:["session-context"], recipes:{...brief_fragments} }
                                  │
                                  ▼
           agents-render.py TOML AGENTS.md --preserve-if-runtime-brief --resolved-config TMP
                                  │  collect_recipe_brief_fragments → workflow_rules + conflict_policy
                                  ▼
           AGENTS.md (non-empty behavioral sections)   ── on any failure ──▶ placeholder fallback

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `templates/ai-specs.toml.tmpl` | Modify | Add active `[recipes.session-context]` block (`enabled = true`, `version = "2.0.0"`) in the recipes section (~line 40); keep the explanatory comment. |
| `lib/init.sh` | Modify | New step 3b after TOML write: define `RECIPE_MATERIALIZE_PY`/`AGENTS_RENDER_PY` (mirroring sync's vars), run materialize→render to a `mktemp` resolved-config inside a guarded block. Step 4 placeholder becomes the fallback path. |
| `tests/test_runtime_brief_baseline.py` (or extend `test_sync_pipeline.py`) | Create/Modify | e2e + unit coverage (below). |

## Interfaces / Contracts

No new interfaces. `init.sh` reuses the existing CLIs verbatim:
```bash
RESOLVED_CONFIG_TEMP="$(mktemp -t ai-specs-resolved-config-XXXXXX.json)"
trap 'rm -f "$RESOLVED_CONFIG_TEMP"' EXIT
if python3 "$RECIPE_MATERIALIZE_PY" "$TARGET_PATH" "$AI_SPECS_HOME" \
       --resolved-config-out "$RESOLVED_CONFIG_TEMP" \
   && python3 "$AGENTS_RENDER_PY" "$TOML_PATH" "$AGENTS_PATH" \
       --preserve-if-runtime-brief --resolved-config "$RESOLVED_CONFIG_TEMP"; then
    echo "  ✓ render AGENTS.md (baseline brief)"
else
    [[ -f "$AGENTS_PATH" ]] || echo "# AGENTS.md - Runtime context" > "$AGENTS_PATH"
    echo "  ! render skipped (fallback placeholder)"
fi
```
Place BEFORE the gitignore step so `set -e` cannot abort init — the `if` consumes the exit code.
Note: `recipe-materialize.py` runs the full enabled pipeline (vendor of recipe dep skills); for
`session-context` the provided skills are `source = "bundled"` (no network). Use
`--resolved-config-only` if a lighter path is preferred — sub-agents to confirm in tasks/apply.

## Testing Strategy (strict TDD — `./tests/run.sh`)

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Template default enables `session-context` | `load_module` `recipe-materialize.py`; render template to temp `ai-specs.toml`; assert `build_resolved_config(root)["enabled"]` contains `session-context` |
| E2E | Fresh init produces non-empty behavioral brief | `subprocess.run([CLI,"init",tmp])` (pattern from `test_sync_pipeline.py`); assert `AGENTS.md` contains the recipe's workflow_rules + conflict_policy fragment strings and `## Conflict Policy` header |
| E2E | Idempotent init→sync | init, snapshot `AGENTS.md`, run `sync`, assert byte-identical; separately, write a file bearing `<!-- ai-specs:runtime-brief -->` then `--force` init and assert untouched |
| E2E | No this-repo leakage | assert dogfood board id / vault scope / project-specific commands absent from baseline `AGENTS.md` |

All offline: catalog is read from `AI_SPECS_HOME`; `session-context` skills are `bundled`. No network, deterministic, runs under `python3 -m unittest`.

## Migration / Rollout

No data migration. Existing projects keep their manifests (already populated). Reversible: revert
the two edits (re-comment recipe block, restore placeholder-only step 4). AGENTS.md regenerates on
next sync regardless.

## Open Questions

- [ ] Tasks/apply to confirm `--resolved-config-out` (full materialize) vs `--resolved-config-only` for the init step — both yield the needed `brief_fragments`; full path matches sync exactly (preferred for byte-stability), lighter path avoids recipe dep-skill vendoring at init time.
