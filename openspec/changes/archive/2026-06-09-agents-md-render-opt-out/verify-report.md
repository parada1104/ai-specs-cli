# Verify Report: agents-md-render-opt-out

**Verifier**: Independent (orchestrator, inline) — NOT the implementer
**Date**: 2026-06-09
**Verdict**: PASS
**Branch / worktree**: `feat/agents-md-render-opt-out` at `.worktrees/agents-md-render-opt-out/`

> This report supersedes the earlier `PASS-WITH-WARNINGS` verdict (2026-06-08). The 1 WARNING and 2 SUGGESTIONS from that pass have been resolved — see **Follow-up Resolution** below.

---

## Test Evidence (independently collected)

**Command**: `./tests/run.sh` (unittest discovery) + `./tests/validate.sh` (py_compile + bash -n + run.sh), both from worktree root
**Result**: `Ran 565 tests in 116.742s — OK`
**Exit code**: 0
**Source**: independent run by the verifier, not copied from apply-progress.md

Baseline was 563 tests; 2 new tests were added by the follow-up work (W1 + S1), bringing the total to 565.

---

## Summary Scorecard

| Dimension    | Status |
|--------------|--------|
| Completeness | 27/29 tasks done; 2 open are non-code (archive + Trello) |
| Correctness  | All 8 requirements verified; every core scenario now has automated coverage |
| Coherence    | Design CLI contract now followed exactly (non-validate is fail-safe; `--validate` is the strict gate) |

---

## Follow-up Resolution (2026-06-09)

The three findings from the first independent verify were addressed under strict TDD. Full suite GREEN (565/565, exit 0) after the changes.

### W1 (was WARNING) — uppercase `True` rejection now tested — RESOLVED

Added `test_render_uppercase_true_is_toml_error()` in `tests/test_brief_render_policy.py`. It writes `[brief]\nrender = True` and asserts the CLI exits non-zero with a TOML parse error on stderr. Characterization test for behavior the TOML parser already guarantees; the coverage gap is now closed.

### S1 (was SUGGESTION) — CLI non-validate contract aligned with design — RESOLVED

Decision (user): non-validate mode must be fail-safe. `lib/_internal/brief-render-policy.py` `main()` was fixed:

- The tautology `return 1 if args.validate else 1` was removed.
- Non-validate mode: an invalid (non-boolean) render *value* is treated as the default (render ENABLED) → prints `true`, exit 0. A typo can no longer silently drop the managed brief for bash callers.
- `--validate` mode: invalid render value still prints the error and exits 1 (strict gate preserved; doctor reports it as ERROR/INFO independently).
- Exception handlers were reordered so `tomllib.TOMLDecodeError` is caught **before** `ValueError` — required because `TOMLDecodeError` subclasses `ValueError`; without the reorder the lenient non-validate branch would have swallowed genuine TOML parse errors (including the W1 case).

Tests: `test_cli_non_validate_invalid_render_defaults_to_true` (new — asserts stdout `true` / exit 0 for `render = "false"` without `--validate`) and `test_cli_validate_rejects_string` (preserved — `--validate` still exits 1). Module docstring updated to describe the fail-safe contract.

### S2 (was SUGGESTION) — doctor test strengthened — RESOLVED

`test_render_disabled_with_recipe_fragments_reports_warn` (`tests/test_doctor.py`) now also asserts `INFO` and `brief-render` are present alongside the WARN, so a future regression that drops the INFO signal while keeping WARN is caught. Existing WARN assertions untouched.

---

## Issues (current pass)

### CRITICAL — 0
None.

### WARNING — 0
None.

### SUGGESTION — 0
None.

---

## Spec Scenario Coverage

### recipe-manifest-contract spec

| Scenario | Test | Status |
|----------|------|--------|
| render omitted defaults to enabled | `test_no_brief_table_defaults_true`, `test_brief_without_render_defaults_true` | COVERED |
| render false disables managed output | `test_sync_skips_agents_md_when_render_false` + unit `test_render_false` | COVERED |
| render true with prose and recipes behaves as today | `test_sync_default_render_true_regenerates` | COVERED |
| Lowercase boolean accepted | `test_render_false` (unit) | COVERED |
| Invalid boolean rejected (validate) | `test_render_string_raises`, `test_cli_validate_rejects_string` | COVERED |
| Invalid boolean lenient (non-validate, fail-safe) | `test_cli_non_validate_invalid_render_defaults_to_true` | COVERED (NEW) |
| Capitalized True rejected at parse time | `test_render_uppercase_true_is_toml_error` | COVERED (NEW) |
| Root render false applies to subrepo fan-out | `test_subrepo_skips_render_when_root_render_false` | COVERED |
| Doctor ERROR when render false and AGENTS.md missing | `test_render_disabled_missing_agents_md_reports_error` | COVERED |
| Doctor WARN + INFO when recipe fragments unused | `test_render_disabled_with_recipe_fragments_reports_warn` | COVERED (strengthened) |
| Doctor INFO when render disabled with AGENTS.md present | `test_render_disabled_with_agents_md_reports_info` | COVERED |

### runtime-brief-rendering spec

| Scenario | Test | Status |
|----------|------|--------|
| Fresh init produces non-empty behavioral brief | `test_runtime_brief_baseline` | COVERED (pre-existing) |
| Init with render disabled creates placeholder only | `test_init_placeholder_when_render_false` | COVERED |
| Init with render disabled preserves existing AGENTS.md | `test_init_preserves_manual_agents_md_when_render_false` | COVERED |
| Init render failure falls back to placeholder | `init.sh` fallback + pre-existing tests | COVERED |
| Baseline brief contains no project-specific tokens | baseline tests | COVERED (pre-existing) |
| Second render after init is byte-stable | idempotency tests | COVERED (pre-existing) |
| Sync with render disabled leaves AGENTS.md unchanged | `test_sync_skips_agents_md_when_render_false`, `test_two_syncs_with_render_false_are_byte_stable` | COVERED |
| User-authored marker prevents re-render | `test_render_true_marker_still_preserves_file` | COVERED |
| Subrepo AGENTS.md contains structured fields | sync-agent tests | COVERED (pre-existing) |
| Subrepo render skipped when root render disabled | `test_subrepo_skips_render_when_root_render_false` | COVERED |
| Subrepo missing AGENTS.md with render disabled fails clearly | `test_subrepo_missing_agents_md_errors_when_render_false` | COVERED |
| File without marker is overwritten | `test_sync_default_render_true_regenerates` | COVERED |
| Render false skips even without marker | `test_render_false_without_marker_leaves_file_untouched` | COVERED |
| Sync stdout names skip reason | `assertIn("skipped AGENTS.md (brief.render = false)", ...)` | COVERED |
| Init stderr guides manual brief authoring | `assertIn("placeholder", result.stderr.lower())` | COVERED |
| Two syncs with render disabled produce no diff | `test_two_syncs_with_render_false_are_byte_stable` | COVERED |

---

## Task Completion

**tasks.md state**: 27 of 29 tasks marked `[x]`. The 2 open tasks are non-code and expected:

- Merge delta specs into `openspec/specs/` — archive-time task.
- Trello #18 → Review after PR — tracker hygiene.

All B1–B7 implementation task groups are realized in committed code and backed by tests.

---

## Files Changed Since First Verify (uncommitted, this follow-up)

- `lib/_internal/brief-render-policy.py` — non-validate fail-safe + exception-handler reorder + docstring (S1)
- `tests/test_brief_render_policy.py` — `test_render_uppercase_true_is_toml_error` (W1), `test_cli_non_validate_invalid_render_defaults_to_true` (S1)
- `tests/test_doctor.py` — INFO + brief-render assertions added to the fragment-warn test (S2)

---

## Final Assessment

**Verdict: PASS** — 0 CRITICAL, 0 WARNING, 0 SUGGESTION.

The implementation is functionally complete and correct. Every core spec scenario now has explicit automated coverage. The CLI honors its documented contract (fail-safe in non-validate mode, strict in `--validate`). Full suite GREEN at 565/565, exit 0.

**Ready for archive** once the two non-code tasks (delta-spec merge at archive time; Trello #18 → Review) are handled as part of the PR/archive flow.
