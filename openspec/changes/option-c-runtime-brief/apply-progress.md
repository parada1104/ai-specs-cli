# Apply Progress: option-c-runtime-brief

## TDD Cycle Evidence

| Task | RED (test written) | GREEN (impl passes) | REFACTOR |
|------|-------------------|---------------------|----------|
| 1.1 test_sync_renders_rich_brief_from_manifest | DONE — fails: `[brief]` prose not rendered, board_id/test_cmd/vault_scope/integration_branch absent | DONE — commit `7a7d180` | N/A |
| 1.2 test_sync_rich_brief_identical_on_second_run | DONE — fails: `idempotency-board-xyz` needle absent from thin output | DONE — commit `7a7d180` | N/A |
| 1.3 test_agents_render_standalone_degradation | DONE — fails: `[brief]` prose absent from standalone render, no `--resolved-config` handling | DONE — commit `7a7d180` | N/A |
| 1.4 RED gate validation | DONE — confirmed: exactly 3 failures, 292 green | — | — |
| 5.x test_brief_useful_commands_renders_extra_items | DONE — fails: `brief.useful_commands` not rendered | DONE — commit `99c2c74` | N/A |
| FIX-1a test_auto_binding_without_explicit_bindings | DONE — fails: board_id/vault_scope absent without explicit [[bindings]] | DONE — commit `dde00d8` | N/A |
| FIX-1b test_resolved_config_bindings_non_empty_without_explicit_bindings | DONE — fails: `bindings: {}` for no explicit [[bindings]] | DONE — commit `dde00d8` | N/A |
| FIX-5a test_partial_brief_renders_present_keys_no_crash | DONE — GREEN at write (R1 scenario) | DONE — commit `dde00d8` | N/A |
| FIX-5b test_no_tracker_binding_omits_trello_section | DONE — GREEN at write (R3 scenario) | DONE — commit `dde00d8` | N/A |
| FIX-5c test_subrepo_sync_agent_forwards_resolved_config | DONE — GREEN at write (R7 scenario) | DONE — commit `dde00d8` | N/A |

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

### FIX PASS (sdd-verify findings) — DONE — commits `dde00d8` + `d7b9d4d`

Addressed all 5 findings from verify-report.md:

#### FIX 1 (CRITICAL): Auto-binding gap — commit `dde00d8`
- `lib/_internal/recipe-materialize.py` line ~596: replaced `build_resolved_config()` call with
  `resolved_bindings` override: `resolved["bindings"] = resolved_bindings` (catalog-aware auto-bind).
- `ai-specs/ai-specs.toml`: removed hand-added explicit `[[bindings]]`; replaced with commented template.
- 2 RED tests (TDD): `test_auto_binding_without_explicit_bindings` + `test_resolved_config_bindings_non_empty_without_explicit_bindings`.
- AUTOBIND PROOF: `bindings` JSON = `{'canonical-store': 'vault-canonical-store', 'tracker': 'trello-mcp-workflow', 'vcs-pr-flow': 'git-pr-flow', ...}` (12 entries, no explicit [[bindings]]).

#### FIX 2 (WARNING): TDD gate runner — commit `dde00d8`
- `ai-specs/ai-specs.toml`: restored `tdd-flow.test_command = "./tests/validate.sh"`.
- Added `"Focused tests (unit-only): \`./tests/run.sh\`"` to `[brief].useful_commands`.
- `lib/_internal/agents-render.py` `_section_useful_commands`: if `validate.sh` in test_command, label is
  `"Full validation:"` (was always `"Focused tests:"` — now semantically correct).

#### FIX 3 (WARNING): Leaked materialization artifacts — commit `dde00d8`
- `git rm --cached` 25 files: `ai-specs/.recipe/**` (8 files), `ai-specs/commands/*.md` (5 files,
  not skills-as-rules.md), `ai-specs/recipes/**` (13 files incl. templates).
- `ai-specs/commands/.gitignore` added: ignores `*` except `.gitignore` and `skills-as-rules.md`.
- `ai-specs/.ai-specs.lock`: KEPT (was on development; updated with recipe skill SHAs is expected).

#### FIX 4 (WARNING): Trailing newline / idempotency — commit `dde00d8`
- AGENTS.md regenerated via `./lib/sync.sh` (real sync, not hand-trimmed).
- Two consecutive syncs produce byte-identical AGENTS.md.

#### FIX 5 (WARNING): Missing scenario tests — commit `dde00d8`
- `TestMissingScenarios` class with 3 new behavioral tests:
  - `test_partial_brief_renders_present_keys_no_crash` (R1)
  - `test_no_tracker_binding_omits_trello_section` (R3)
  - `test_subrepo_sync_agent_forwards_resolved_config` (R7)

#### FIX gitignore-render (WARNING) — commit `d7b9d4d`
- `lib/_internal/gitignore-render.py`: added `.recipe/`, `.deps/`, `recipes/` to generated `.gitignore`.
- `ai-specs/.gitignore` regenerated with new patterns.

## All Batches + Fix Pass Complete

Full suite post-fix: `Ran 302 tests OK` (+5 new tests vs 297 baseline).

## Relevant Files

- `tests/test_sync_pipeline.py` — 4 original GREEN tests + 6 new tests (useful_commands + 2 FIX-1 TDD + 3 FIX-5 scenarios)
- `lib/_internal/agents-render.py` — section-helper rewrite + brief.useful_commands + validate.sh label fix
- `lib/sync.sh` — RESOLVED_CONFIG_TEMP wiring
- `lib/sync-agent.sh` — `--resolved-config` passthrough
- `lib/_internal/recipe-materialize.py` — `build_resolved_config()` + auto-bind fix (`resolved_bindings` override)
- `lib/_internal/gitignore-render.py` — added recipe materialization ignore patterns
- `ai-specs/ai-specs.toml` — `[brief]` table + fixed project name + test_command restored + explicit bindings removed
- `ai-specs/.gitignore` — extended with .recipe/, .deps/, recipes/ patterns
- `ai-specs/commands/.gitignore` — new: ignores recipe-generated commands
- `docs/ai-specs-toml.md` — [brief] documented
- `AGENTS.md` — now manifest-generated; canonical trailing newline; idempotent
- `openspec/changes/option-c-runtime-brief/tasks.md` — all batches marked [x]
