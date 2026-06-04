# Tasks: omp-agent-target (Oh My Pi agent target)

## Batch 1: Platform Registration (RED → GREEN)

- [x] 1.1 RED: Write failing test in `tests/test_doctor.py` — 8 individual `test_omp_*` field tests asserting `platform_get omp <field>` returns correct value. (Note: tests placed in test_doctor.py::PlatformGetTests alongside pi tests, not test_sync_pipeline.py)
- [x] 1.2 RED: Add `test_omp_invalid_field_exits_nonzero` asserting `platform_get omp nonexistent_field` exits non-zero.
- [x] 1.3 GREEN: Add `omp)` case to `lib/_internal/platform.sh` after `pi)` with all 8 fields; update agent-list comment to include `omp`.

## Batch 2: CLI Wiring (RED → GREEN)

- [x] 2.1 RED: Write `test_sync_agent_omp_flag_accepted` in `tests/test_sync_pipeline.py` — assert `sync-agent --omp` exits 0 and `.omp/skills` symlink is created.
- [x] 2.2 RED: Write `test_sync_agent_help_lists_omp` asserting `sync-agent --help` output includes `--omp`.
- [x] 2.3 GREEN: Extend flag alternation in `lib/sync-agent.sh` to include `--omp` (alongside `...|--pi|--omp)`).
- [x] 2.4 GREEN: Add `--omp  Oh My Pi (.omp/skills, .omp/mcp.json, .omp/commands)` line to `usage()` in `lib/sync-agent.sh`.

## Batch 3: Hooks Renderer (RED → GREEN, with open-question task)

- [x] 3.1 RESEARCH: Verified omp ExtensionAPI import path — it is `@oh-my-pi/pi-coding-agent` (NOT `@earendil-works/pi-coding-agent`). Confirmed from official omp examples at github.com/can1357/oh-my-pi/packages/coding-agent/examples/extensions/hello.ts.
- [x] 3.2 RED: Write `test_omp_extension_shim` in `tests/test_hooks_render.py` — assert `.omp/extensions/demo-shell-gate.ts` exists with correct content including `@oh-my-pi/pi-coding-agent`.
- [x] 3.3 GREEN: Add module-level constant `OMP_EXT_IMPORT = "@oh-my-pi/pi-coding-agent"` to `lib/_internal/hooks-render.py`.
- [x] 3.4 GREEN: Add `"omp"` column to each `EVENT_MAP` entry in `hooks-render.py`, inheriting pi's native event names (`tool_call`, `tool_result`, `session_start`, `agent_end`).
- [x] 3.5 GREEN: Add `render_omp()` function (copy of `render_pi()`, write to `.omp/extensions/<recipe>-<hook>.ts`, use `OMP_EXT_IMPORT`).
- [x] 3.6 GREEN: Add `elif agent == "omp": render_omp(...)` dispatch branch in `hooks-render.py` after the `pi` branch.

## Batch 4: Templates (GREEN — no behavior under test)

- [x] 4.1 Add `.omp/` after `.pi/` in `templates/gitignore-root.tmpl`.
- [x] 4.2 Add `omp` to the agents comment/example list in `templates/ai-specs.toml.tmpl`.

## Batch 5: Fan-out Behaviors (RED → GREEN)

- [x] 5.1 RED: Write `test_sync_agent_omp_flag_accepted` (skills symlink test, covered in Batch 2).
- [x] 5.2 RED: Write `test_omp_mcp_json_rendered_when_mcps_declared` — assert `.omp/mcp.json` exists with `mcpServers` key when `[mcp.*]` entries are declared.
- [x] 5.3 RED: Write `test_omp_mcp_json_absent_when_no_mcps` — assert `.omp/mcp.json` is NOT created when no MCPs declared.
- [x] 5.4 RED: Write `test_omp_commands_populated` — assert files in `ai-specs/commands/` are present under `.omp/commands/` after sync.
- [x] 5.5 (merged with 5.4 — commands test covers both cases via init defaults)
- [x] 5.6 RED: Write `test_omp_no_instruction_symlink` — assert no OMP.md or omp.md is created.
- [x] 5.7 GREEN: Fan-out confirmed via generic `platform_get` loop in sync-agent.sh — no additional code needed; omp registered fields drive all behavior.

## Batch 6: --all Integration (RED → GREEN)

- [x] 6.1 RED: Write `test_sync_agent_all_includes_omp_when_enabled` — asserts --all syncs omp when enabled.
- [x] 6.2 RED: Write `test_sync_agent_all_excludes_omp_when_not_enabled` — asserts --all does NOT sync omp when absent.
- [x] 6.3 GREEN: No code change needed; existing TARGETS loop handles omp. Tests pass.

## Batch 7: Backward Compatibility Assertion

- [x] 7.1 RED: Write `test_existing_agents_unchanged_after_omp_added` — asserts CLAUDE.md and .claude/skills are byte-identical before and after adding omp to enabled.
- [x] 7.2 GREEN: No shared logic modified; all changes purely additive. Tests pass without code changes.

## Batch 8: Final Verification

- [x] 8.1 Run focused tests: `./tests/run.sh` — 369 tests, all GREEN (was 349; +20 new omp tests).
- [x] 8.2 Run full validation: `./tests/validate.sh` — 369 tests, all pass.
- [x] 8.3 Structural check: `.omp/` in gitignore template ✓; `--omp` in `--help` ✓; `OMP_EXT_IMPORT` defined and referenced ✓.
