# Tasks: Option C — Generate the Rich Runtime Brief

## Batch 1: RED — Failing Tests (TDD first)

- [x] 1.1 [TEST-RED] `tests/test_sync_pipeline.py`: add `test_sync_renders_rich_brief_from_manifest` — build manifest with `[brief]` + recipe configs (`board_id`, `integration_branch`, `test_command`, `vault_scope`), run sync, assertIn each needle; must FAIL (renderer not yet enriched). Mirror `test_sync_redacts_literal_mcp_secrets_in_agents_md`.
- [x] 1.2 [TEST-RED] `tests/test_sync_pipeline.py`: add `test_sync_rich_brief_identical_on_second_run` — run sync twice with rich manifest, assertEqual bytes; must FAIL (no rich output yet).
- [x] 1.3 [TEST-RED] `tests/test_sync_pipeline.py`: add `test_agents_render_standalone_degradation` — invoke `agents-render.py` directly with no `--resolved-config`; assert prose + identity + MCP render, exit 0; must FAIL (arg not accepted yet).
- [x] 1.4 [TEST-GREEN gate] Run `./tests/run.sh`; confirm exactly the three new tests are RED (fail), all prior tests are GREEN. Record in apply-progress.

## Batch 2: Schema + resolved-config plumbing

- [x] 2.1 [IMPL] `lib/_internal/recipe-materialize.py`: add `--resolved-config-out <path>` CLI argument; after all recipes materialize, build the resolved-config JSON `{bindings, recipes, enabled}` from `resolved_bindings`, per-recipe `merged_cfg`, and enabled recipe ids; write to the path. Mirror `--recipe-mcp-out` pattern.
- [x] 2.2 [IMPL] `lib/sync.sh`: `mktemp` a `RESOLVED_CONFIG_TEMP` alongside `RECIPE_MCP_TEMP`; pass `--resolved-config-out "$RESOLVED_CONFIG_TEMP"` to `recipe-materialize.py`; pass `--resolved-config "$RESOLVED_CONFIG_TEMP"` to `agents-render.py`; `rm -f` at cleanup. No change to `sync-agent.sh` fan-out path (subrepos already receive resolved config via `--resolved-config` passed directly to `agents-render.py` in `ensure_target_workspace`).
  - Note: `sync-agent.sh` line 207 calls `agents-render.py` without `--preserve-if-runtime-brief` for subrepos — add `--resolved-config` passthrough here too.

## Batch 3: Renderer enrichment

- [x] 3.1 [IMPL] `lib/_internal/agents-render.py`: add `--resolved-config <path>` argument; load JSON when present; store as `resolved` dict (default `{}`).
- [x] 3.2 [IMPL] `lib/_internal/agents-render.py`: restructure `_render_lines` into per-section helper functions: `_section_project`, `_section_mcp`, `_section_runtime_flow`, `_section_trello`, `_section_context_sources`, `_section_conflict_policy`, `_section_workflow_rules`, `_section_useful_commands`. Fixed 10-section order as per design decision 3 / section-order spec.
- [x] 3.3 [IMPL] `lib/_internal/agents-render.py`: implement `_section_project` — name, manifest path, purpose, enabled runtimes, integration_branch (from `resolved.recipes["worktree-flow"].integration_branch` or omit).
- [x] 3.4 [IMPL] `lib/_internal/agents-render.py`: implement `_section_mcp` — existing MCP table + per-server description from `[brief.mcp_descriptions]` + "Never expose env-backed secrets" rule. Preserve `_redact_env_value` behavior (R4 backward compat).
- [x] 3.5 [IMPL] `lib/_internal/agents-render.py`: implement `_section_runtime_flow` — bullet list from `[brief].runtime_flow`; append VCS bullet from `resolved.bindings.vcs-pr-flow` → `resolved.recipes["git-pr-flow"].provider`; omit section if no content.
- [x] 3.6 [IMPL] `lib/_internal/agents-render.py`: implement `_section_trello` — board_id from `resolved.recipes[resolved.bindings.tracker].board_id`; omit section if no `tracker` binding (R3 no-tracker scenario).
- [x] 3.7 [IMPL] `lib/_internal/agents-render.py`: implement `_section_context_sources`, `_section_conflict_policy`, `_section_workflow_rules` — bullet lists from `[brief]` arrays; omit sections when key absent (R1 partial-brief scenario).
- [x] 3.8 [IMPL] `lib/_internal/agents-render.py`: implement `_section_useful_commands` — `test_command` from `resolved.recipes["tdd-flow"].test_command` + derived validate command; omit if absent.
- [x] 3.9 [IMPL] `lib/_internal/agents-render.py`: implement intro blockquote from `[brief].intro` (multi-line string → `> ` prefix); emit after H1, before `## Project`.

## Batch 4: GREEN — Make tests pass

- [x] 4.1 [TEST-GREEN] Run `./tests/run.sh`; confirm all three RED tests from Batch 1 now pass. Record GREEN evidence in apply-progress.
- [x] 4.2 [TEST-GREEN] Run `./tests/validate.sh`; confirm full suite passes including `test_sync_preserves_runtime_brief_marker_in_agents_md` and `test_sync_redacts_literal_mcp_secrets_in_agents_md` (backward compat R4, R5).

## Batch 5: Docs + [brief] population

- [ ] 5.1 [IMPL] `docs/ai-specs-toml.md`: add `[brief]` section documenting all keys (`intro`, `purpose`, `runtime_flow`, `context_sources`, `conflict_policy`, `workflow_rules`, `[brief.mcp_descriptions]`); add to field classification table; update "Canonical V1 surface" list.
- [ ] 5.2 [IMPL] `ai-specs/ai-specs.toml`: add `[brief]` table populated from current `AGENTS.md` content (intro, purpose, runtime_flow, context_sources, conflict_policy, workflow_rules, mcp_descriptions).

## Batch 6: Migration (atomic)

- [ ] 6.1 [VERIFY] Run `python3 lib/_internal/agents-render.py ai-specs/ai-specs.toml /tmp/AGENTS.scratch.md --resolved-config <generated-temp>`; diff `/tmp/AGENTS.scratch.md` vs current `AGENTS.md`; iterate `[brief]` in `ai-specs.toml` until output ≈ manual brief (modulo "Current Transitional State" section).
- [ ] 6.2 [ATOMIC-COMMIT] In ONE commit: (a) remove `<!-- ai-specs:runtime-brief -->` marker from `AGENTS.md` and `CLAUDE.md`; (b) remove "Current Transitional State" section from `AGENTS.md`; (c) update `CLAUDE.md` "Current Transitional State" / runtime-brief-marker notes to reflect completion; (d) confirm `ai-specs sync` now regenerates `AGENTS.md` without clobbering.
- [ ] 6.3 [TEST-SMOKE] Run `./tests/validate.sh` on the migrated state; confirm all tests green.
