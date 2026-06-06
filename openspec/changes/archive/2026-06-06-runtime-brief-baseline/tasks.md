# Tasks: Runtime Brief Baseline for Fresh Init

Two coordinated edits: (1) pre-enable `session-context` in the TOML template;
(2) add a guarded materialize→render step in `init.sh` after the TOML write.
Strict TDD — RED before GREEN for every behavioral unit.

---

## Batch Overview

| Batch | Content | Depends on |
|-------|---------|------------|
| B1 | RED: unit test — template default enables session-context | — |
| B2 | GREEN: edit template | B1 written |
| B3 | RED: E2E tests — init behavioral brief, fallback, no-leakage, idempotency | — |
| B4 | GREEN: edit `init.sh` (step 3b render guard) | B3 written |
| B5 | Confirm open question: full `--resolved-config-out` path (byte-stable) | B4 done |
| B6 | Regression: run.sh green + validate.sh clean | B5 done |

Critical path: B1 → B2 → B3 → B4 → B5 → B6

---

## Batch 1 — RED: Unit test — template default enables session-context

### 1.1 RED — write failing unit test for template default

- [x] 1.1 In `tests/test_runtime_brief_baseline.py` (new file), write a failing test class `TemplateDefaultTests`. Test `test_template_default_enables_session_context`: load `recipe-materialize.py` via `load_module()`, render the template to a `mktemp` dir (`sed s/{{PROJECT_NAME}}/test-proj/g templates/ai-specs.toml.tmpl`), call `build_resolved_config(tmp_root)`, assert `"session-context"` in result`["enabled"]`.
  - **Target file**: `tests/test_runtime_brief_baseline.py` (create)

### 1.2 RED — write failing unit test for no project-specific tokens in resolved config

- [x] 1.2 In the same class, add `test_template_default_no_project_specific_tokens`: after `build_resolved_config`, assert the JSON-serialized output does NOT contain any of the known this-repo tokens (`69ec097f13e2d38ecd89a557`, `nnodes/proyectos`, `ai-specs-cli`).
  - **Target file**: `tests/test_runtime_brief_baseline.py`

### 1.3 Confirm tests are RED

- [x] 1.3 Run `./tests/run.sh` (or `python -m pytest tests/test_runtime_brief_baseline.py`); confirm the two new tests fail (template currently has `session-context` commented out).

---

## Batch 2 — GREEN: Edit template to pre-enable session-context

### 2.1 GREEN — add active session-context block to template

- [x] 2.1 In `templates/ai-specs.toml.tmpl`, in the Recipes section (after the comment block, ~line 41), add an active block:
  ```toml
  [recipes.session-context]
  enabled = true
  version = "2.0.0"
  ```
  Keep the existing comment block above it.
  - **Target file**: `templates/ai-specs.toml.tmpl`

### 2.2 Run B1 tests and confirm GREEN

- [x] 2.2 Run `./tests/run.sh` focusing on `test_runtime_brief_baseline.py`; confirm 1.1 and 1.2 pass.

---

## Batch 3 — RED: E2E tests — init behavioral brief, fallback, idempotency, no-leakage

### 3.1 RED — fresh init produces non-empty behavioral brief

- [x] 3.1 In `tests/test_runtime_brief_baseline.py`, add class `InitBriefE2ETests`. Test `test_fresh_init_produces_behavioral_brief`: `subprocess.run([CLI, "init", tmp])`, read `AGENTS.md`, assert `"## Workflow Rules"` present, assert at least one bullet from `session-context.workflow_rules` present (e.g. `"A session works on one explicit user request"`), assert `"## Conflict Policy"` present with at least two bullets.
  - **Target file**: `tests/test_runtime_brief_baseline.py`

### 3.2 RED — render failure falls back to placeholder; init exits 0

- [x] 3.2 Test `test_init_render_failure_falls_back_to_placeholder`: run `init` in a temp dir with `PATH` stripped of `python3` (or patch via env); assert `AGENTS.md` exists (non-empty), assert process return code is 0, assert stderr contains a skip/fallback message.
  - **Target file**: `tests/test_runtime_brief_baseline.py`
  - **Note**: Used selective fake python3 (only fails recipe-materialize/agents-render; passes through for other scripts) — portable approach per design risk note.

### 3.3 RED — init→sync idempotency (byte-stable)

- [x] 3.3 Test `test_init_then_sync_is_byte_stable`: run `init` in temp dir, snapshot `AGENTS.md` bytes; run `sync`; assert bytes identical.
  - **Target file**: `tests/test_runtime_brief_baseline.py`

### 3.4 RED — user `<!-- ai-specs:runtime-brief -->` marker preserved under --force

- [x] 3.4 Test `test_force_init_preserves_runtime_brief_marker`: run `init`, write `<!-- ai-specs:runtime-brief -->` into `AGENTS.md`, run `init --force`, assert `AGENTS.md` still contains the marker and was not overwritten.
  - **Target file**: `tests/test_runtime_brief_baseline.py`

### 3.5 RED — no this-repo tokens in baseline AGENTS.md

- [x] 3.5 Test `test_no_project_specific_tokens_in_baseline_agents_md`: run `init` in a clean temp dir, read `AGENTS.md`, assert it does NOT contain `"69ec097f13e2d38ecd89a557"`, `"nnodes/proyectos"`, `"ai-specs-cli"`, or any board-id/vault-scope pattern.
  - **Target file**: `tests/test_runtime_brief_baseline.py`

### 3.6 Confirm all B3 tests are RED

- [x] 3.6 Run `./tests/run.sh`; confirm all five new tests fail (init still writes placeholder only).
  - **Note**: Tasks 3.1–3.5 written as RED before GREEN; after implementing B4 all went GREEN.

---

## Batch 4 — GREEN: Add step 3b render guard in init.sh

### 4.1 GREEN — wire RECIPE_MATERIALIZE_PY and AGENTS_RENDER_PY vars in init.sh

- [x] 4.1 In `lib/init.sh`, after the `GITIGNORE_RENDER` var (~line 114), add:
  ```bash
  RECIPE_MATERIALIZE_PY="$AI_SPECS_HOME/lib/_internal/recipe-materialize.py"
  AGENTS_RENDER_PY="$AI_SPECS_HOME/lib/_internal/agents-render.py"
  ```
  - **Target file**: `lib/init.sh`

### 4.2 GREEN — add step 3b guarded render block

- [x] 4.2 In `lib/init.sh`, immediately after the TOML write block (after line 177 `echo "  ✓ wrote ai-specs/ai-specs.toml"` / `fi`), insert step 3b:
  ```bash
  # 3b. Render a baseline AGENTS.md from the freshly written manifest.
  #     Best-effort: any failure falls back to the placeholder written in step 4.
  #     The if-guard consumes the exit code so set -e cannot abort init.
  RESOLVED_CONFIG_TEMP="$(mktemp -t ai-specs-resolved-config-XXXXXX.json)"
  trap 'rm -f "$RESOLVED_CONFIG_TEMP"' EXIT
  if python3 "$RECIPE_MATERIALIZE_PY" "$TARGET_PATH" "$AI_SPECS_HOME" \
         --resolved-config-out "$RESOLVED_CONFIG_TEMP" \
     && python3 "$AGENTS_RENDER_PY" "$TOML_PATH" "$AGENTS_PATH" \
         --preserve-if-runtime-brief --resolved-config "$RESOLVED_CONFIG_TEMP"; then
      echo "  ✓ render AGENTS.md (baseline brief)"
  else
      [[ -f "$AGENTS_PATH" ]] || echo "# AGENTS.md - Runtime context" > "$AGENTS_PATH"
      echo "  ! render skipped — fallback placeholder written" >&2
  fi
  ```
  - **Target file**: `lib/init.sh`

### 4.3 GREEN — make step 4 placeholder conditional (fallback only)

- [x] 4.3 The existing step 4 line moved into the `else` branch of the if-guard in step 3b. Step 3b owns the fallback; old standalone placeholder line removed.
  - **Target file**: `lib/init.sh`

### 4.4 Run B3 tests and confirm GREEN

- [x] 4.4 Run `./tests/run.sh` focusing on `test_runtime_brief_baseline.py`; confirm all five E2E tests pass.
  - **Result**: All 7 tests in test_runtime_brief_baseline.py pass (2 unit + 5 E2E).

---

## Batch 5 — Confirm open question: full `--resolved-config-out` path

### 5.1 Confirm full materialize path matches sync (byte-stable)

- [x] 5.1 Verify that the init render (using `--resolved-config-out` from `materialize_recipes`) and a subsequent `sync` render (same flag, same pipeline) produce byte-identical `AGENTS.md`. This is confirmed by test 3.3; cross-check `sync.sh:113` uses the same `--resolved-config-out` flag with no extra flags. Document decision: full `--resolved-config-out` path is correct; `--resolved-config-only` is NOT used (it skips skill/command vendoring but `session-context` skills are bundled — both yield the same `brief_fragments`; full path is preferred for exact byte-parity with sync).
  - **Result**: `test_init_then_sync_is_byte_stable` passes GREEN. Comment added in `lib/init.sh` ("Mirrors sync.sh:113 for byte-stability").
  - **Target file**: `lib/init.sh`

---

## Batch 6 — Final validation

### 6.1 Run full test suite green

- [x] 6.1 Run `./tests/run.sh`; assert all tests pass (no regressions in existing suite).
  - **Result**: Ran 519 tests in ~90s. OK

### 6.2 Run validate.sh clean

- [x] 6.2 Run `./tests/validate.sh`; assert exits 0 (syntax validation, schema checks).
  - **Result**: Ran 519 tests. OK. Exit 0.

---

## Task Summary

| Batch | Tasks | RED | GREEN | Notes |
|-------|-------|-----|-------|-------|
| B1 | 3 | 2 (1.1, 1.2) + 1 run | — | Unit: template parse |
| B2 | 2 | — | 1 edit + 1 run | Template change |
| B3 | 6 | 5 (3.1–3.5) + 1 run | — | E2E: all behaviors |
| B4 | 4 | — | 3 edits + 1 run | init.sh render guard |
| B5 | 1 | — | 1 confirm + comment | Open question resolved |
| B6 | 2 | — | 2 validation | Final gate |
| **Total** | **18** | **7** | **11** | |

**Critical path**: B1 → B2 → B3 → B4 → B5 → B6 (all sequential; TDD enforced).
