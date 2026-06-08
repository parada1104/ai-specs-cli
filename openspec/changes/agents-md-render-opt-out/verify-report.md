# Verify Report: agents-md-render-opt-out

**Verifier**: Independent (sdd-verify executor — NOT the implementer)  
**Date**: 2026-06-08  
**Verdict**: PASS-WITH-WARNINGS  
**Branch / worktree**: `feat/agents-md-render-opt-out` at `.worktrees/agents-md-render-opt-out/`

---

## Test Evidence (independently collected)

**Command**: `./tests/validate.sh` from worktree root  
**Result**: `Ran 563 tests in 114.788s — OK`  
**Exit code**: 0  
**Source**: independent run, not copied from apply-progress.md

Sub-suite confirmations (run independently):

| Suite | Command | Count | Result |
|-------|---------|-------|--------|
| unit policy | `python3 -m unittest tests.test_brief_render_policy -v` | 11 | OK |
| E2E opt-out | `python3 -m unittest tests.test_agents_md_render_opt_out -v` | 9 | OK |
| doctor render | `python3 -m unittest tests.test_doctor.BriefRenderPolicyDoctorTests -v` | 3 | OK |

---

## Summary Scorecard

| Dimension    | Status |
|--------------|--------|
| Completeness | 27/29 tasks done; 2 open are non-code (archive + Trello) |
| Correctness  | All 8 requirements verified; 1 scenario with no automated test (see WARNING W1) |
| Coherence    | Design contract followed; 1 minor deviation from CLI contract (see SUGGESTION S1) |

---

## Issues

### CRITICAL — 0

None.

---

### WARNING — 1

**W1 — Scenario "Capitalized True rejected at parse time" has no automated test**

Spec (`recipe-manifest-contract` §"Capitalized True rejected"): given `render = True` (uppercase T) the manifest MUST fail with a TOML decode error OR doctor must report an explicit boolean-format guidance message.

Evidence: manually confirmed `python3 lib/_internal/brief-render-policy.py <toml>` exits 1 with "Invalid value (at line 2, column 10)" when `render = True`. Behavior is correct at runtime.

Gap: no automated test in `tests/test_brief_render_policy.py` exercises the `render = True` (uppercase, invalid TOML) parse path. The scenario is covered by the TOML parser itself, but the test suite does not assert it.

Recommendation: add `test_render_uppercase_true_is_toml_error()` in `tests/test_brief_render_policy.py:~58` that writes `render = True` to a temp file and asserts `subprocess.run(...).returncode != 0`.

---

### SUGGESTION — 2

**S1 — CLI exits 1 on ValueError regardless of --validate flag (design minor deviation)**

Design contract states: `python3 brief-render-policy.py <toml_path>` (without `--validate`) should "print `true`/`false`, exit 0". The implementation exits 1 on `ValueError` (non-boolean render) even without `--validate`. This is a benign deviation — bash callers only check whether stdout equals `"true"` and the error case is silent-skip in that branch. No runtime impact.

Recommendation: if CLI contract needs to be exact, guard the non-validate path with `sys.exit(0)` after printing nothing; otherwise document the deviation in the module docstring.

**S2 — `test_render_disabled_with_recipe_fragments_reports_warn` does not verify INFO is absent**

The doctor test for fragment WARN uses the same project as the INFO test (init with session-context enabled). Both INFO and WARN are emitted simultaneously. The test only asserts WARN presence. This is fine for correctness, but could mask a future regression where INFO disappears while WARN remains.

Recommendation: add `assertIn("INFO", result.stdout)` to the fragment-warn test so both signals are verified together.

---

## Spec Scenario Coverage

### recipe-manifest-contract spec

| Scenario | Test | Status |
|----------|------|--------|
| render omitted defaults to enabled | `test_no_brief_table_defaults_true`, `test_brief_without_render_defaults_true` | COVERED |
| render false disables managed output | `test_sync_skips_agents_md_when_render_false` + unit `test_render_false` | COVERED |
| render true with prose and recipes behaves as today | `test_sync_default_render_true_regenerates` | COVERED (behavior, not fragment content) |
| Lowercase boolean accepted | `test_render_false` (unit) | COVERED |
| Invalid boolean rejected | `test_render_string_raises`, `test_cli_validate_rejects_string` | COVERED |
| Capitalized True rejected at parse time | no test; runtime behavior confirmed manually | **NOT AUTOMATED — WARNING W1** |
| Root render false applies to subrepo fan-out | `test_subrepo_skips_render_when_root_render_false` | COVERED |
| Doctor ERROR when render false and AGENTS.md missing | `test_render_disabled_missing_agents_md_reports_error` | COVERED |
| Doctor WARN when recipe fragments unused | `test_render_disabled_with_recipe_fragments_reports_warn` | COVERED |
| Doctor INFO when render disabled with AGENTS.md present | `test_render_disabled_with_agents_md_reports_info` | COVERED |

### runtime-brief-rendering spec

| Scenario | Test | Status |
|----------|------|--------|
| Fresh init produces non-empty behavioral brief | existing baseline tests (`test_runtime_brief_baseline`) | COVERED (pre-existing) |
| Init with render disabled creates placeholder only | `test_init_placeholder_when_render_false` | COVERED |
| Init with render disabled preserves existing AGENTS.md | `test_init_preserves_manual_agents_md_when_render_false` | COVERED |
| Init render failure falls back to placeholder | existing fallback path in `init.sh` + pre-existing tests | COVERED by code path |
| Baseline brief contains no project-specific tokens | existing baseline tests | COVERED (pre-existing) |
| Second render after init is byte-stable | existing idempotency tests | COVERED (pre-existing) |
| Sync with render disabled leaves AGENTS.md unchanged | `test_sync_skips_agents_md_when_render_false`, `test_two_syncs_with_render_false_are_byte_stable` | COVERED |
| User-authored marker prevents re-render | `test_render_true_marker_still_preserves_file` | COVERED |
| Subrepo AGENTS.md contains structured fields | existing sync-agent tests | COVERED (pre-existing) |
| Subrepo render skipped when root render disabled | `test_subrepo_skips_render_when_root_render_false` | COVERED |
| Subrepo missing AGENTS.md with render disabled fails clearly | `test_subrepo_missing_agents_md_errors_when_render_false` | COVERED |
| File with marker left untouched | `test_render_true_marker_still_preserves_file` | COVERED |
| File without marker is overwritten | `test_sync_default_render_true_regenerates` | COVERED |
| Sync skips render when render is false | `test_sync_skips_agents_md_when_render_false` | COVERED |
| Default render true preserves current behavior | `test_sync_default_render_true_regenerates` | COVERED |
| Render false does not block other sync artifacts | structural: guard only wraps agents-render block; sync exits 0 | COVERED (structural) |
| Render false skips even without marker | `test_render_false_without_marker_leaves_file_untouched` | COVERED |
| Render false with marker present is redundant but valid | (same as sync skip + byte-stable) | COVERED by implication |
| Render true with marker still preserves file | `test_render_true_marker_still_preserves_file` | COVERED |
| Sync stdout names skip reason | `assertIn("skipped AGENTS.md (brief.render = false)", result.stdout)` | COVERED |
| Init stderr guides manual brief authoring | `assertIn("placeholder", result.stderr.lower())` | COVERED |
| Second sync produces no diff when render enabled | existing idempotency tests | COVERED (pre-existing) |
| Two syncs with render disabled produce no diff | `test_two_syncs_with_render_false_are_byte_stable` | COVERED |

---

## Task Completion

**tasks.md state**: 27 of 29 tasks marked `[x]`.

| Task group | Status | Evidence |
|-----------|--------|---------|
| B1 — brief-render-policy.py (1.1–1.5) | DONE | file exists; 11 unit tests pass |
| B2 — sync.sh guard (2.1–2.4) | DONE | guard at `lib/sync.sh:117`; E2E tests pass |
| B3 — init.sh guard (3.1–3.4) | DONE | guard at `lib/init.sh:186`; placeholder/preserve tests pass |
| B4 — sync-agent.sh guard (4.1–4.4) | DONE | guard at `lib/sync-agent.sh:230`; subrepo tests pass |
| B5 — doctor.py (5.1–5.3) | DONE | `_check_brief_render_policy()` at line 254; `Severity.INFO` added; 3 doctor tests pass |
| B6 — Docs + template (6.1–6.2) | DONE | `docs/ai-specs-toml.md` has `[brief].render` row; template has commented example |
| B7 — Regression + validation (7.1–7.4) | DONE | `./tests/validate.sh` exits 0; 563 OK |
| Post-apply: Merge delta specs | OPEN | archive-time task — expected |
| Post-apply: Trello #18 → Review | OPEN | tracker task — expected |

---

## Design Contract Adherence

| Decision | Expected | Actual | Match |
|----------|----------|--------|-------|
| Enforcement point | Shell callers before agents-render.py | Guards in sync.sh, init.sh, sync-agent.sh | YES |
| Shared parser module | `lib/_internal/brief-render-policy.py` | Present | YES |
| Default when key absent | `true` | `brief.get("render", True) is not False` | YES |
| Only `False` disables | `render is False` check | YES | YES |
| Invalid render type | ValueError + error in doctor | ValueError raised; doctor ERROR check present | YES |
| Init placeholder | `# AGENTS.md - Runtime context` | Same string at `init.sh:203` | YES |
| Subrepo policy | Root TOML_PATH inherited | Root `TOML_PATH` passed to sync-agent.sh guard | YES |
| Messages (exact strings) | Per design table | sync: `· skipped AGENTS.md (brief.render = false)` matches; subrepo: `    · skipped AGENTS.md (brief.render = false)` (4-space indent) matches | YES |
| `attach_brief_fragments_to_resolved()` | Not in original design (deviation documented) | Added to recipe-materialize.py; used in doctor for fragment detection | DEVIATION — documented, accepted |

---

## Files Changed (confirmed)

All files claimed in apply-progress.md are present in `git diff development...HEAD --name-only`:

- `lib/_internal/brief-render-policy.py` — created
- `lib/sync.sh` — modified (guard at line 117)
- `lib/init.sh` — modified (guard at line 186)
- `lib/sync-agent.sh` — modified (guard at line 230)
- `lib/_internal/doctor.py` — modified (`_check_brief_render_policy`, `Severity.INFO`, adjusted `_check_agents_md`)
- `lib/_internal/recipe-materialize.py` — modified (`attach_brief_fragments_to_resolved`)
- `docs/ai-specs-toml.md` — modified
- `templates/ai-specs.toml.tmpl` — modified
- `tests/test_brief_render_policy.py` — created
- `tests/test_agents_md_render_opt_out.py` — created
- `tests/test_doctor.py` — modified

---

## Final Assessment

**Verdict: PASS-WITH-WARNINGS**

0 CRITICAL, 1 WARNING, 2 SUGGESTIONS.

The implementation is functionally complete and correct. All spec requirements are implemented. The full test suite (563 tests) passes. One scenario ("Capitalized True rejected") lacks an automated test but is satisfied at runtime by the TOML parser. The single WARNING (W1) is low risk and does not block archive, but should be addressed in a follow-up or as part of the PR review.

**Ready for archive**, with the recommendation to add the missing automated test for `render = True` (uppercase) parse rejection before merging to production.
