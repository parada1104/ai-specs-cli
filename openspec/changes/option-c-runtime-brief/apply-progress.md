# Apply Progress: option-c-runtime-brief

## TDD Cycle Evidence

| Task | RED (test written) | GREEN (impl passes) | REFACTOR |
|------|-------------------|---------------------|----------|
| 1.1 test_sync_renders_rich_brief_from_manifest | DONE — fails: `[brief]` prose not rendered, board_id/test_cmd/vault_scope/integration_branch absent | DONE — commit `7a7d180` | N/A |
| 1.2 test_sync_rich_brief_identical_on_second_run | DONE — fails: `idempotency-board-xyz` needle absent from thin output | DONE — commit `7a7d180` | N/A |
| 1.3 test_agents_render_standalone_degradation | DONE — fails: `[brief]` prose absent from standalone render, no `--resolved-config` handling | DONE — commit `7a7d180` | N/A |
| 1.4 RED gate validation | DONE — confirmed: exactly 3 failures, 292 green | — | — |

## Completed Batches

### Batch 1 (RED) — DONE — commit `782a74b`

**Tests written** (`tests/test_sync_pipeline.py`):

1. `test_sync_renders_rich_brief_from_manifest`
   - Builds manifest with `[brief]` (intro, purpose, runtime_flow, context_sources, conflict_policy, workflow_rules, mcp_descriptions) plus recipe configs (board_id, integration_branch, test_command, vault_scope)
   - Runs `ai-specs sync`; asserts all prose needles and structured needles appear in `AGENTS.md`
   - Failure reason: `_render_lines()` ignores `[brief]`; sync does not pass `--resolved-config` to `agents-render.py`

2. `test_sync_rich_brief_identical_on_second_run`
   - Runs sync twice with `[brief]` + recipe configs; asserts byte-identical output PLUS `idempotency-board-xyz` needle
   - Tied to rich path via needle assertion — thin renderer passes byte-identity but fails on needle
   - Failure reason: `idempotency-board-xyz` (board_id) not in thin AGENTS.md

3. `test_agents_render_standalone_degradation`
   - Invokes `lib/_internal/agents-render.py` directly (no `--resolved-config`, no sync)
   - Asserts: exit 0, project name present, MCP section present, `[brief]` prose sections present
   - Failure reason: `_render_lines()` does not process `[brief]` table keys at all

**RED gate output summary**:
```
FAIL: test_agents_render_standalone_degradation — 'This is the degraded brief intro.' not found
FAIL: test_sync_renders_rich_brief_from_manifest — 'Canonical runtime context for agents.' not found
FAIL: test_sync_rich_brief_identical_on_second_run — 'idempotency-board-xyz' not found
Ran 295 tests in 52.216s
FAILED (failures=3)
```
292 pre-existing tests remain green.

### Batch 2 (resolved-config plumbing) — DONE — commit `7a7d180`

**Changes**:

- `lib/_internal/recipe-materialize.py`: Added `build_resolved_config()` function that reads raw manifest TOML (no catalog lookup required) to produce `{bindings, recipes, enabled}` JSON. Added `--resolved-config-out <path>` CLI arg to `main()` and wired it into `materialize_recipes()`. Also writes the JSON when no enabled recipes exist (prevents empty-file crash).
- `lib/sync.sh`: Added `RESOLVED_CONFIG_TEMP="$(mktemp ...)"` alongside `RECIPE_MCP_TEMP`; passes `--resolved-config-out "$RESOLVED_CONFIG_TEMP"` to `recipe-materialize.py`; passes `--resolved-config "$RESOLVED_CONFIG_TEMP"` to `agents-render.py`; cleans up both temps.
- `lib/sync-agent.sh`: Added `--resolved-config` arg parsing; passes it to `agents-render.py` in `ensure_target_workspace()` when present and file exists.

**Key discovery — TOML fixture format**: The RED test fixtures used `[[recipes]]` array-of-tables mixed with `[recipes.<id>]` sub-tables. Python's tomllib accepts this but produces `recipes` as a list, not the dict that `read_recipes()` expects. Fixed fixtures to use `[recipes.<id>]` flat keys + explicit `[[bindings]]` entries. This is the correct real-manifest format.

### Batch 3 (renderer enrichment) — DONE — commit `7a7d180`

**Changes**:

- `lib/_internal/agents-render.py`: Complete rewrite using 10-section helper architecture.
  - `--resolved-config <path>` arg added; loads JSON → `resolved` dict (default `{}`).
  - `_render_lines(manifest, resolved)` now orchestrates 10-section order.
  - Per-section helpers: `_section_intro`, `_section_project`, `_section_mcp`, `_section_runtime_flow`, `_section_trello`, `_section_context_sources`, `_section_conflict_policy`, `_section_workflow_rules`, `_section_useful_commands`.
  - All sections gracefully omit when data is absent (degradation path).
  - `_redact_env_value` preserved unchanged (backward compat R4).
  - `--preserve-if-runtime-brief` preserved unchanged (R5).
  - vault_scope rendered in `_section_project` via `bindings.canonical-store` → `vault_scope`.

**Test fixtures also fixed**:
- `test_sync_renders_rich_brief_from_manifest`: Changed from `[[recipes]]` array syntax to `[recipes.<id>]` dict syntax; added explicit `[[bindings]]` for `tracker`, `vcs-pr-flow`, `canonical-store`.
- `test_sync_rich_brief_identical_on_second_run`: Same fix.

**Duplicate test name fix**: Renamed first `test_sync_produces_identical_agents_md_on_second_run` (line 755) to `test_sync_produces_identical_agents_md_on_second_run_thin` so both variants now run.

### Batch 4 (GREEN gate) — DONE — commit `7a7d180`

**GREEN gate output**:
```
test_sync_renders_rich_brief_from_manifest ... ok
test_sync_rich_brief_identical_on_second_run ... ok
test_agents_render_standalone_degradation ... ok
Ran 3 tests in 2.243s OK

Full suite: Ran 296 tests in 56.0s OK
```
All 3 RED tests now GREEN. Total tests: 296 (was 295 in Batch 1 before duplicate rename added +1).

## Remaining Batches

- Batch 5: Docs + `[brief]` population in ai-specs/ai-specs.toml
- Batch 6: Migration atomic commit (remove runtime-brief marker)

## Relevant Files

- `tests/test_sync_pipeline.py` — 3 GREEN tests at ~line 823; duplicate rename at line 755
- `openspec/changes/option-c-runtime-brief/tasks.md` — Batches 1-4 marked [x]
- `lib/_internal/agents-render.py` — complete section-helper rewrite
- `lib/sync.sh` — RESOLVED_CONFIG_TEMP wiring
- `lib/sync-agent.sh` — `--resolved-config` passthrough
- `lib/_internal/recipe-materialize.py` — `build_resolved_config()` + `--resolved-config-out`
