# Apply Progress: option-c-runtime-brief

## TDD Cycle Evidence

| Task | RED (test written) | GREEN (impl passes) | REFACTOR |
|------|-------------------|---------------------|----------|
| 1.1 test_sync_renders_rich_brief_from_manifest | DONE — fails: `[brief]` prose not rendered, board_id/test_cmd/vault_scope/integration_branch absent | DONE — commit `7a7d180` | N/A |
| 1.2 test_sync_rich_brief_identical_on_second_run | DONE — fails: `idempotency-board-xyz` needle absent from thin output | DONE — commit `7a7d180` | N/A |
| 1.3 test_agents_render_standalone_degradation | DONE — fails: `[brief]` prose absent from standalone render, no `--resolved-config` handling | DONE — commit `7a7d180` | N/A |
| 1.4 RED gate validation | DONE — confirmed: exactly 3 failures, 292 green | — | — |
| 5.x test_brief_useful_commands_renders_extra_items | DONE — fails: `brief.useful_commands` not rendered | DONE — commit `99c2c74` | N/A |

## Completed Batches

### Batch 1 (RED) — DONE — commit `782a74b`

3 RED tests written. RED gate: exactly 3 failures, 292 green.

### Batch 2 (resolved-config plumbing) — DONE — commit `7a7d180`

- `lib/_internal/recipe-materialize.py`: Added `build_resolved_config()` + `--resolved-config-out` CLI arg.
- `lib/sync.sh`: Added `RESOLVED_CONFIG_TEMP` mktemp; passes to materialize and agents-render; cleans up.
- `lib/sync-agent.sh`: Added `--resolved-config` passthrough.

Key discovery: Test fixtures must use `[recipes.<id>]` flat keys + explicit `[[bindings]]` (not `[[recipes]]` array syntax).

### Batch 3 (renderer enrichment) — DONE — commit `7a7d180`

- `lib/_internal/agents-render.py`: Complete rewrite with 10-section helper architecture.
- All sections gracefully omit when data absent. `_redact_env_value` and `--preserve-if-runtime-brief` preserved.

### Batch 4 (GREEN gate) — DONE — commit `7a7d180`

GREEN gate: all 3 RED tests pass. Full suite: `Ran 296 tests OK`.

### Batch 5 (Docs + [brief] population) — DONE — commit `99c2c74`

- `docs/ai-specs-toml.md`: Added `[brief]` + `[brief.mcp_descriptions]` to Canonical V1 surface, field classification table, and new `### [brief]` / `### [brief.mcp_descriptions]` sections.
- `ai-specs/ai-specs.toml`: Added `[brief]` table (intro, purpose, runtime_flow, context_sources, conflict_policy, workflow_rules, useful_commands, mcp_descriptions); changed `tdd-flow.test_command` to `./tests/run.sh`; added explicit `[[bindings]]`; fixed `[project].name` to `ai-specs-cli`.
- `lib/_internal/agents-render.py`: Added `brief.useful_commands` support in `_section_useful_commands` (TDD: RED test first, then GREEN).
- New test: `test_brief_useful_commands_renders_extra_items`.

### Batch 6 (Migration atomic) — DONE — commit `99c2c74`

Scratch diff confirmed generated output ≈ equivalent to manual AGENTS.md. All substantive content preserved; acceptable minor formatting differences (see diff_summary in return envelope).

- Wrote generated content to `AGENTS.md` (marker removed, "Current Transitional State" section gone)
- Materialized recipe artifacts (.recipe/, commands/, recipes/) from updated TOML
- Full suite post-commit: `Ran 297 tests OK` (+1 for new test)

## All Batches Complete — Ready for sdd-verify

## Relevant Files

- `tests/test_sync_pipeline.py` — 4 GREEN tests + 1 new useful_commands test
- `lib/_internal/agents-render.py` — section-helper rewrite + brief.useful_commands support
- `lib/sync.sh` — RESOLVED_CONFIG_TEMP wiring
- `lib/sync-agent.sh` — `--resolved-config` passthrough
- `lib/_internal/recipe-materialize.py` — `build_resolved_config()` + `--resolved-config-out`
- `ai-specs/ai-specs.toml` — `[brief]` table + fixed project name + test_command + explicit bindings
- `docs/ai-specs-toml.md` — [brief] documented
- `AGENTS.md` — now manifest-generated; marker removed
- `openspec/changes/option-c-runtime-brief/tasks.md` — all batches marked [x]
