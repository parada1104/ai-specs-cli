# Tasks: `agents-md-render-opt-out`

Manifest-level `[brief].render = false` skips managed `AGENTS.md` generation on
sync/init/subrepos while preserving the HTML marker escape hatch when render is enabled.

---

## Batch Overview

| Batch | Content | Can run after | Parallelizable within? |
|-------|---------|---------------|------------------------|
| B1 | `brief-render-policy.py` — parser + CLI (tests first) | — | No (TDD sequential) |
| B2 | `sync.sh` guard + E2E sync skip tests | B1 done | No |
| B3 | `init.sh` guard + E2E init tests | B1 done | No |
| B4 | `sync-agent.sh` subrepo guard + E2E tests | B1 done | Yes — parallel with B2/B3 after B1 |
| B5 | `doctor.py` checks + tests | B1 done | Yes — parallel with B2-B4 after B1 |
| B6 | Docs + template comment | B2-B5 done | Yes — two files independent |
| B7 | Regression + `./tests/validate.sh` | B1-B6 done | No |

**Critical path**: B1 → B2 → B3 → B7  
**Parallel opportunity**: B4 and B5 after B1; B6 after shell/doctor land.

Estimated batch count: **7**

---

## Batch 1 — `brief-render-policy.py`: Parser + CLI

> **TDD**: write failing tests (RED) → implement (GREEN).

### 1.1 RED — unit tests for `brief_render_enabled`

- [x] 1.1 Write failing tests in `tests/test_brief_render_policy.py`:
  - No `[brief]` table → `True`
  - `[brief]` without `render` key → `True`
  - `[brief] render = true` → `True`
  - `[brief] render = false` → `False`
  - `[brief] render = "false"` (string) → raises `ValueError` naming `[brief].render`
  - `[brief] render = 1` (int) → raises `ValueError`
  - **Spec scenarios**: recipe-manifest-contract §"render omitted defaults to enabled", §"render false disables managed output", §"Invalid boolean rejected"
  - **Target file**: `tests/test_brief_render_policy.py`

### 1.2 GREEN — implement `brief_render_enabled` + `load_brief_render_enabled`

- [x] 1.2 Create `lib/_internal/brief-render-policy.py` with `PLACEHOLDER_LINE`, `brief_render_enabled()`, `load_brief_render_enabled()`, and `has_dead_recipe_fragments()`.
  - **Target file**: `lib/_internal/brief-render-policy.py`

### 1.3 RED — CLI stdout tests

- [x] 1.3 Write failing tests for CLI:
  - `python3 brief-render-policy.py <toml>` prints `true` or `false` to stdout, exit 0
  - Manifest with `render = false` → stdout `false`
  - `--validate` exits 1 on non-boolean `render` with stderr mentioning boolean
  - **Target file**: `tests/test_brief_render_policy.py`

### 1.4 GREEN — implement CLI `main()`

- [x] 1.4 Add `argparse` CLI: positional `toml_path`, optional `--validate`; print `true`/`false`; propagate `ValueError` as exit 1 on validate.
  - **Target file**: `lib/_internal/brief-render-policy.py`

### 1.5 Run B1 tests

- [x] 1.5 Run `./tests/run.sh tests/test_brief_render_policy.py` — all GREEN.

---

## Batch 2 — `sync.sh`: Root render guard

> **TDD**: E2E test first, then wire shell.

### 2.1 RED — sync skips AGENTS.md when render false

- [x] 2.1 Write failing E2E test in `tests/test_agents_md_render_opt_out.py`:
  - Fixture project with `[brief] render = false` and existing manual `AGENTS.md`
  - Run sync harness
  - Assert `AGENTS.md` byte-identical pre/post
  - Assert stdout contains `skipped AGENTS.md (brief.render = false)`
  - Assert `agents-render.py` not invoked (spy via env or output absence of render sections change)
  - **Spec scenarios**: runtime-brief-rendering §"Sync skips render when render is false", §"Render false does not block other sync artifacts"
  - **Target file**: `tests/test_agents_md_render_opt_out.py`

### 2.2 GREEN — wire `sync.sh`

- [x] 2.2 Add `BRIEF_RENDER_POLICY_PY` variable; wrap `agents-render` block in `brief-render-policy.py` guard with skip message.
  - **Target file**: `lib/sync.sh`

### 2.3 RED — default render true regression

- [x] 2.3 Write failing regression test: manifest without `render` key still regenerates `AGENTS.md` on sync (behavior unchanged).
  - **Spec scenario**: runtime-brief-rendering §"Default render true preserves current behavior"
  - **Target file**: `tests/test_agents_md_render_opt_out.py`

### 2.4 Run B2 tests

- [x] 2.4 Run `./tests/run.sh tests/test_agents_md_render_opt_out.py -k sync` — GREEN.

---

## Batch 3 — `init.sh`: Init guard + placeholder

### 3.1 RED — init placeholder when render false

- [x] 3.1 Write failing test:
  - Fresh init with manifest `[brief] render = false`
  - `AGENTS.md` exists with exactly `# AGENTS.md - Runtime context`
  - stderr mentions placeholder guidance
  - `agents-render.py` not invoked
  - **Spec scenario**: runtime-brief-rendering §"Init with render disabled creates placeholder only"
  - **Target file**: `tests/test_agents_md_render_opt_out.py`

### 3.2 RED — init preserves existing manual brief

- [x] 3.2 Write failing test:
  - Pre-existing `AGENTS.md` with custom content
  - `init` with `[brief] render = false` (and `--force` if needed for other artifacts)
  - `AGENTS.md` byte-identical
  - **Spec scenario**: runtime-brief-rendering §"Init with render disabled preserves existing AGENTS.md"
  - **Target file**: `tests/test_agents_md_render_opt_out.py`

### 3.3 GREEN — wire `init.sh`

- [x] 3.3 Branch block 3b on `brief-render-policy.py`; placeholder path uses `PLACEHOLDER_LINE` via python constant or duplicated exact string; skip materialize+render when disabled.
  - **Target file**: `lib/init.sh`

### 3.4 Run B3 tests

- [x] 3.4 Run `./tests/run.sh tests/test_agents_md_render_opt_out.py -k init` — GREEN.

---

## Batch 4 — `sync-agent.sh`: Subrepo guard

### 4.1 RED — subrepo skip when root render false

- [x] 4.1 Write failing E2E test:
  - Root manifest `render = false`
  - Subrepo target with existing `AGENTS.md`
  - Fan-out sync leaves subrepo `AGENTS.md` unchanged
  - Skills still mirrored (directory exists post-sync)
  - **Spec scenarios**: runtime-brief-rendering §"Subrepo render skipped when root render disabled"; recipe-manifest-contract §"Root render false applies to subrepo fan-out"
  - **Target file**: `tests/test_agents_md_render_opt_out.py`

### 4.2 RED — subrepo missing AGENTS.md errors

- [x] 4.2 Write failing test:
  - Root `render = false`, subrepo without `AGENTS.md`
  - sync-agent exits non-zero with message to create manually or enable render
  - **Spec scenarios**: runtime-brief-rendering §"Subrepo missing AGENTS.md with render disabled fails clearly"
  - **Target file**: `tests/test_agents_md_render_opt_out.py`

### 4.3 GREEN — wire `sync-agent.sh`

- [x] 4.3 Add `BRIEF_RENDER_POLICY_PY`; guard `ensure_target_workspace()` render call; error on missing file when disabled.
  - **Target file**: `lib/sync-agent.sh`

### 4.4 Run B4 tests

- [x] 4.4 Run `./tests/run.sh tests/test_agents_md_render_opt_out.py -k subrepo` — GREEN.

---

## Batch 5 — `doctor.py`: Diagnostics

### 5.1 RED — doctor checks for render disabled

- [x] 5.1 Write failing tests in `tests/test_doctor.py`:
  - `render = false` + `AGENTS.md` present → INFO `brief-render`
  - `render = false` + no `AGENTS.md` → ERROR `brief-render`
  - `render = false` + enabled recipe with `[provides.brief]` → WARN `brief-fragments-unused`
  - `render = false` + marker in AGENTS.md → INFO `brief-render-marker` (optional combined check)
  - **Spec scenarios**: recipe-manifest-contract §"Doctor ERROR/WARN/INFO when render disabled"
  - **Target file**: `tests/test_doctor.py`

### 5.2 GREEN — implement `_check_brief_render_policy`

- [x] 5.2 Add method to `doctor.py`; call from `run()` after `_check_agents_md()`; reuse `brief_render_enabled` and resolved-config for fragment detection; adjust agents-md guidance when render off.
  - **Target files**: `lib/_internal/doctor.py`

### 5.3 Run B5 tests

- [x] 5.3 Run `./tests/run.sh tests/test_doctor.py -k brief_render` — GREEN.

---

## Batch 6 — Documentation

### 6.1 Update `docs/ai-specs-toml.md`

- [x] 6.1 Add `[brief].render` to manifest table; document default `true`, precedence (flag > marker > normal render), subrepo inheritance, migration note from marker-only opt-out.
  - **Target file**: `docs/ai-specs-toml.md`

### 6.2 Update scaffold template

- [x] 6.2 Add commented `[brief] render = false` example to `templates/ai-specs.toml.tmpl`.
  - **Target file**: `templates/ai-specs.toml.tmpl`

---

## Batch 7 — Regression + validation

### 7.1 Marker regression when render enabled

- [x] 7.1 Confirm existing tests still pass:
  - `tests/test_sync_pipeline.py` marker preservation
  - `tests/test_runtime_brief_baseline.py` sync/init idempotency with render enabled
  - Add explicit test if gap: `render=true` (or absent) + marker → file untouched
  - **Spec scenario**: runtime-brief-rendering §"Render true with marker still preserves file"
  - **Target files**: `tests/test_agents_md_render_opt_out.py` or existing files

### 7.2 Precedence tests

- [x] 7.2 Add test: `render=false` without marker → file untouched (no renderer call).
  - **Spec scenario**: runtime-brief-rendering §"Render false skips even without marker"
  - **Target file**: `tests/test_agents_md_render_opt_out.py`

### 7.3 Idempotency with render disabled

- [x] 7.3 Add test: two consecutive syncs with `render=false` → byte-identical `AGENTS.md`.
  - **Spec scenario**: runtime-brief-rendering §"Two syncs with render disabled produce no diff"
  - **Target file**: `tests/test_agents_md_render_opt_out.py`

### 7.4 Full validation

- [x] 7.4 Run `./tests/validate.sh` — all green.

---

## Apply checklist (post-tasks)

- [x] Merge delta specs into `openspec/specs/` at archive time
- [ ] Trello #18 → In Progress during apply, Review after PR
- [x] No dogfood change to ai-specs-cli `[brief]` unless explicitly requested
