# Apply Progress: option-c-runtime-brief

## TDD Cycle Evidence

| Task | RED (test written) | GREEN (impl passes) | REFACTOR |
|------|-------------------|---------------------|----------|
| 1.1 test_sync_renders_rich_brief_from_manifest | DONE — fails: `[brief]` prose not rendered, board_id/test_cmd/vault_scope/integration_branch absent | PENDING (Batch 4) | PENDING |
| 1.2 test_sync_rich_brief_identical_on_second_run | DONE — fails: `idempotency-board-xyz` needle absent from thin output | PENDING (Batch 4) | PENDING |
| 1.3 test_agents_render_standalone_degradation | DONE — fails: `[brief]` prose absent from standalone render, no `--resolved-config` handling | PENDING (Batch 4) | PENDING |
| 1.4 RED gate validation | DONE — confirmed: exactly 3 failures, 292 green | — | — |

## Completed Batches

### Batch 1 (RED) — DONE — commit `782a74b`

**Tests written** (`tests/test_sync_pipeline.py`):

1. `test_sync_renders_rich_brief_from_manifest`
   - Builds manifest with `[brief]` (intro, purpose, runtime_flow, context_sources, conflict_policy, workflow_rules, mcp_descriptions) plus recipe configs (board_id, integration_branch, test_command, vault_scope)
   - Runs `ai-specs sync`; asserts all prose needles and structured needles appear in `AGENTS.md`
   - **Failure reason**: `_render_lines()` ignores `[brief]`; sync does not pass `--resolved-config` to `agents-render.py`

2. `test_sync_rich_brief_identical_on_second_run`
   - Runs sync twice with `[brief]` + recipe configs; asserts byte-identical output PLUS `idempotency-board-xyz` needle
   - Tied to rich path via needle assertion — thin renderer passes byte-identity but fails on needle
   - **Failure reason**: `idempotency-board-xyz` (board_id) not in thin AGENTS.md

3. `test_agents_render_standalone_degradation`
   - Invokes `lib/_internal/agents-render.py` directly (no `--resolved-config`, no sync)
   - Asserts: exit 0, project name present, MCP section present, `[brief]` prose sections present
   - **Failure reason**: `_render_lines()` does not process `[brief]` table keys at all

**RED gate output summary**:
```
FAIL: test_agents_render_standalone_degradation — 'This is the degraded brief intro.' not found
FAIL: test_sync_renders_rich_brief_from_manifest — 'Canonical runtime context for agents.' not found
FAIL: test_sync_rich_brief_identical_on_second_run — 'idempotency-board-xyz' not found
Ran 295 tests in 52.216s
FAILED (failures=3)
```
292 pre-existing tests remain green.

## What Batch 2 Must Implement (to wire resolved-config)

- `lib/_internal/recipe-materialize.py`: add `--resolved-config-out <path>` argument; after materialization, build JSON `{bindings, recipes, enabled}` and write to path.
- `lib/sync.sh`: `mktemp` a `RESOLVED_CONFIG_TEMP`; pass `--resolved-config-out "$RESOLVED_CONFIG_TEMP"` to `recipe-materialize.py`; pass `--resolved-config "$RESOLVED_CONFIG_TEMP"` to `agents-render.py`; cleanup on exit.

## What Batch 3 Must Implement (to turn tests GREEN)

- `lib/_internal/agents-render.py`: add `--resolved-config <path>` argument; load JSON → `resolved` dict (default `{}`).
- Restructure `_render_lines` into per-section helpers per design.md decision 3 and section-order (10 sections).
- `_section_project`: name, manifest, purpose, enabled runtimes, integration_branch.
- `_section_mcp`: existing table + per-server description from `[brief.mcp_descriptions]` + secrets rule.
- `_section_runtime_flow`: bullets from `[brief].runtime_flow` + VCS provider bullet.
- `_section_trello`: board_id from `resolved.recipes[resolved.bindings.tracker].board_id`; omit if no tracker.
- `_section_context_sources`, `_section_conflict_policy`, `_section_workflow_rules`: bullet lists from `[brief]` arrays.
- `_section_useful_commands`: test_command from `resolved.recipes["tdd-flow"].test_command`.
- Intro blockquote from `[brief].intro` (multi-line → `> ` prefix); emit after H1, before `## Project`.

## Remaining Batches

- Batch 2: Schema + resolved-config plumbing (recipe-materialize.py + sync.sh)
- Batch 3: Renderer enrichment (agents-render.py section helpers)
- Batch 4: GREEN gate — confirm 3 RED tests pass, full validate.sh green
- Batch 5: Docs + `[brief]` population in ai-specs/ai-specs.toml
- Batch 6: Migration atomic commit (remove runtime-brief marker)

## Relevant Files

- `tests/test_sync_pipeline.py` — new RED tests appended starting at line ~820
- `openspec/changes/option-c-runtime-brief/tasks.md` — Batch 1 items marked [x]
- `lib/_internal/agents-render.py` — thin renderer; no `[brief]` or `--resolved-config` (to modify in Batch 3)
- `lib/sync.sh` — no resolved-config wiring (to modify in Batch 2)
- `lib/_internal/recipe-materialize.py` — no `--resolved-config-out` (to add in Batch 2)
