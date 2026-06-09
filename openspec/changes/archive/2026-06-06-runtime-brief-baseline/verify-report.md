# Verification Report — runtime-brief-baseline

**Change**: runtime-brief-baseline
**Spec**: runtime-brief-rendering (delta)
**Mode**: Strict TDD
**Verdict**: **PASS WITH WARNINGS**

---

## Completeness

| Metric | Value |
|--------|-------|
| Tasks total | 18 |
| Tasks complete | 18 (all `[x]`) |
| Tasks incomplete | 0 |

Every task is realized in the tree and proven by an independent re-run. No checked task whose work is absent.

---

## Build & Tests Execution (independent re-run)

- **`./tests/run.sh`** (`python3 -m unittest discover`): PASS — `Ran 519 tests in 96.8s` / `OK` / exit 0
- **`./tests/validate.sh`**: PASS — py_compile + bash -n + full suite clean / `Ran 519 tests` / `OK` / exit 0
- **`tests/test_runtime_brief_baseline.py`** (focused): PASS — `Ran 7 tests in 2.7s` / `OK` (2 unit + 5 E2E)
- `bash -n lib/init.sh` → OK
- `py_compile` recipe-materialize.py, agents-render.py, test file → OK

Note: project uses `unittest` (no pytest installed); ran via `python3 -m unittest`.

---

## Spec Compliance Matrix

| Requirement | Scenario | Test / evidence | Result |
|-------------|----------|-----------------|--------|
| R1: template pre-enables session-context | Fresh template parse yields session-context enabled | `TemplateDefaultTests.test_template_default_enables_session_context` (asserts `"session-context" in build_resolved_config(root)["enabled"]`) | ✅ COMPLIANT |
| R1 | Template default has no project-specific values | `TemplateDefaultTests.test_template_default_no_project_specific_tokens` (asserts board id / vault scope / project name absent from serialized resolved config) | ✅ COMPLIANT |
| R2: init renders non-empty brief | Fresh init produces non-empty behavioral brief | `InitBriefE2ETests.test_fresh_init_produces_behavioral_brief` (asserts `## Workflow Rules` + 1 fragment substring + `## Conflict Policy` with ≥2 bullets) | ✅ COMPLIANT |
| R2 | Init render failure falls back to placeholder | `InitBriefE2ETests.test_init_render_failure_falls_back_to_placeholder` (selective fake python3 forces materialize/render to exit 1; asserts exit 0, AGENTS.md non-empty, stderr `render skipped`) | ✅ COMPLIANT |
| R2 | Baseline brief contains no project-specific tokens | `InitBriefE2ETests.test_no_project_specific_tokens_in_baseline_agents_md` (asserts board id / `nnodes/proyectos` / `ai-specs-cli` absent) | ✅ COMPLIANT (partial on `{config.KEY}` clause — see W3) |
| R3: init→sync idempotency | Second render after init is byte-stable | `InitBriefE2ETests.test_init_then_sync_is_byte_stable` (`read_bytes()` snapshot, real `init` then `sync`, `assertEqual` on bytes) | ✅ COMPLIANT |
| R3 | User marker prevents re-render | `InitBriefE2ETests.test_force_init_preserves_runtime_brief_marker` (covers `init --force` only; sync path verified manually, not asserted in suite) | ⚠️ PARTIAL (W2) |
| R4: fragment dedupe on additional recipe enable | No duplication when second recipe provides same key | `test_agents_render_brief_fragments.py::test_key_dedup_first_wins` + `test_exact_string_dedup_across_recipes` (recipe-agnostic dedup, from PR #75) — NOT exercised with session-context specifically | ⚠️ PARTIAL (W1) |

**Compliance summary**: 6/8 scenarios fully COMPLIANT; 2 PARTIAL (both behaviorally satisfied by shared code paths but not directly asserted for this change's specific scenario wording).

---

## Adversarial Validation (manual re-derivation)

I executed the scenarios by hand against `bin/ai-specs`, not trusting the apply report:

1. **Real init** into `/tmp/.../real`: produced a non-empty, generic brief — `## Project`, `## Conflict Policy` (2 bullets), `## Workflow Rules` (1 bullet). Bullets match `catalog/recipes/session-context/recipe.toml [provides.brief]` verbatim (1 workflow_rule + 2 conflict_policy keyed bullets). No board id / `nnodes/proyectos` / `ai-specs-cli` / `{config.}` / `{{` present.
2. **Fallback** (fake python3 matching `*recipe-materialize*|*agents-render*` → exit 1, pass-through for others): init printed `! render skipped — fallback placeholder written` to stderr, **exited 0**, and wrote the one-line `# AGENTS.md - Runtime context` placeholder. `set -e` did NOT abort — the `if`-guard genuinely consumes the non-zero exit. The test passes for the RIGHT reason, not by accident: the fake forces exactly the render pipeline to fail while letting gitignore-render / refresh-bundled succeed via real python3.
3. **Byte-stability**: `diff` of init-rendered vs sync-rendered AGENTS.md → identical. Confirmed structurally too: both init (`lib/init.sh:187-190`) and sync (`lib/sync.sh:113-116`) call `materialize_recipes(...)` (full path, NOT `--resolved-config-only`) and `agents-render.py --preserve-if-runtime-brief --resolved-config`. The resolved-config write (`recipe-materialize.py:688-703`, `sort_keys=True`) is independent of sync's extra `--recipe-mcp-out` / `--resolved-hooks-out` flags — those feed MCP/hooks renderers, not AGENTS.md. So byte-parity is sound, not coincidental.
4. **Marker on sync**: manually wrote `<!-- ai-specs:runtime-brief -->` into AGENTS.md, ran `sync` → exit 0, file untouched. Same renderer marker code path as init.

---

## Correctness (Static — Structural Evidence)

| Requirement | Status | Notes |
|-------------|--------|-------|
| R1 template pre-enables session-context | ✅ Implemented | `templates/ai-specs.toml.tmpl:42-44` adds active `[recipes.session-context] enabled=true version="2.0.0"`, comment block retained |
| R2 init renders non-empty brief + fallback | ✅ Implemented | `lib/init.sh:181-197` step 3b mirrors sync; `if`-guard catches non-zero so `set -e` cannot abort; fallback placeholder in `else`; stderr message present |
| R3 idempotency / marker | ✅ Implemented | Identical flags + pipeline as sync; `--preserve-if-runtime-brief` honored in shared renderer (`agents-render.py:505-537`) |
| R4 fragment dedupe | ✅ Implemented (reused) | `collect_recipe_brief_fragments` is recipe-agnostic (PR #75); no new dedup code, none needed |

---

## Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| Pre-enable session-context in template | ✅ Yes | Active block added |
| Inline materialize→render in init (mirror sync) | ✅ Yes | Same two commands, same flags |
| Keep `--preserve-if-runtime-brief` on init | ✅ Yes | Present at `init.sh:190` |
| Best-effort guard, fallback on non-zero | ✅ Yes | `if/else` consumes exit code; proven exit 0 on failure |
| Byte-stable with later sync | ✅ Yes | `diff` identical; full `--resolved-config-out` path used |
| Version pin `2.0.0` | ✅ Yes | Matches catalog `recipe.toml` version |
| Open question (full materialize vs `--resolved-config-only`) | ✅ Resolved | Full `--resolved-config-out` chosen for byte-parity with sync; documented in tasks B5 + `init.sh` comment |

No deviations. Implementation matches design.md.

---

## Issues Found

**CRITICAL** (must fix before archive):
None.

**WARNING** (should fix):
~~1. R4 scenario not directly tested for session-context.~~ **CLOSED** — Added `SessionContextDedupTests` in `test_runtime_brief_baseline.py`: two tests exercise session-context + a second recipe sharing `key="conflict-policy-source-authority"`, asserting the bullet appears exactly once (unit: `test_session_context_key_wins_over_second_recipe`; e2e render: `test_session_context_key_dedup_appears_exactly_once_in_full_render`).
~~2. R3 marker scenario only half-asserted in the suite.~~ **CLOSED** — Added `SyncMarkerPreservationTests` in `test_runtime_brief_baseline.py`: `test_sync_preserves_user_edited_agents_md_with_runtime_brief_marker` exercises the full init → user adds marker → sync path and asserts byte-identity; `test_sync_without_marker_regenerates_agents_md` is the counterpart proving the guard is marker-gated.

**SUGGESTION** (nice to have):
~~1. Regex assertion for unrendered `{config.KEY}` / `{{` placeholders.~~ **CLOSED** — Added `BaselineBriefNoPlaceholderTests.test_no_unrendered_config_placeholders_in_baseline_agents_md`.
~~2. Add explicit R4 test.~~ **CLOSED** — see W1 above.
~~3. Add sync-side marker assertion.~~ **CLOSED** — see W2 above.

---

## Addendum (post-verify hardening pass)

**Tests added** (all in `tests/test_runtime_brief_baseline.py`):

| Class | Test | Closes |
|-------|------|--------|
| `SessionContextDedupTests` | `test_session_context_key_wins_over_second_recipe` | W1 (unit) |
| `SessionContextDedupTests` | `test_session_context_key_dedup_appears_exactly_once_in_full_render` | W1 (e2e render) |
| `SyncMarkerPreservationTests` | `test_sync_preserves_user_edited_agents_md_with_runtime_brief_marker` | W2 (sync path) |
| `SyncMarkerPreservationTests` | `test_sync_without_marker_regenerates_agents_md` | W2 counterpart |
| `BaselineBriefNoPlaceholderTests` | `test_no_unrendered_config_placeholders_in_baseline_agents_md` | SUGGESTION 1 |

**Results**: 519 → 524 tests. `./tests/run.sh` and `./tests/validate.sh` both exit 0. No implementation changes — tests only. No defects found.

---

## Verdict

**PASS** — all 18 tasks complete; 524 tests green (up from 519); build/validate exit 0; all spec scenarios now directly asserted including W1 (session-context dedup with second recipe) and W2 (sync-side marker preservation). Ready to archive.
