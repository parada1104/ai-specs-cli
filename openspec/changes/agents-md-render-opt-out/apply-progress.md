# Apply Progress: agents-md-render-opt-out

**Change**: agents-md-render-opt-out  
**Tracker**: Trello #18  
**Branch / worktree**: `feat/agents-md-render-opt-out` at `.worktrees/agents-md-render-opt-out/`  
**Mode**: Strict TDD (batches B1–B7)  
**Apply commit**: `6faf783` — `feat(brief): opt out of managed AGENTS.md via [brief].render = false`  
**Date**: 2026-06-08

---

## Status

All implementation batches (B1–B7) complete. **44 tasks** marked `[x]` in `tasks.md`.  
New tests: 12 unit + 12 E2E/doctor. Full suite **563 tests OK**. `./tests/validate.sh` exit 0.

Post-apply (archive / PR): delta spec merge, Trello → Review — still open.

---

## TDD Cycle Evidence (highlights)

| Area | RED symptom | GREEN fix |
|------|-------------|-----------|
| B1 policy | Missing module / failing unit assertions | `brief-render-policy.py` + `test_brief_render_policy.py` |
| B2 sync skip | Manual `AGENTS.md` overwritten by runtime brief | `sync.sh` guard on `brief-render-policy.py`; test helper fixed (see Deviations) |
| B3 init | Placeholder / manual brief replaced on init | `init.sh` branch: skip render, write placeholder only when missing |
| B4 subrepo | `packages/b/AGENTS.md` missing blocked fan-out | Test limits `subrepos` to `packages/a`; `sync-agent.sh` skip + explicit error |
| B5 doctor INFO | `AttributeError: Severity.INFO` | Added `Severity.INFO`; summary line includes INFO count |
| B5 fragments WARN | `has_dead_recipe_fragments` always false in doctor | `attach_brief_fragments_to_resolved()` shared helper in `recipe-materialize.py` |

---

## Completed Batches

### B1 — `brief-render-policy.py`
- [x] Unit tests: default true, explicit false, invalid types, CLI stdout/`--validate`
- [x] `brief_render_enabled()`, `load_brief_render_enabled()`, `has_dead_recipe_fragments()`, CLI

### B2 — `sync.sh`
- [x] E2E: sync skips `AGENTS.md` when `render = false` (stdout + byte-stable)
- [x] Regression: default render still regenerates brief
- [x] `BRIEF_RENDER_POLICY_PY` guard before `agents-render.py`

### B3 — `init.sh`
- [x] Placeholder on fresh init (`# AGENTS.md - Runtime context`)
- [x] Preserves pre-existing manual `AGENTS.md`
- [x] Guarded block 3b mirrors sync behavior

### B4 — `sync-agent.sh`
- [x] Subrepo skip when root `render = false`
- [x] Clear error when subrepo `AGENTS.md` missing
- [x] `ensure_target_workspace()` policy guard

### B5 — `doctor.py`
- [x] INFO when render disabled + `AGENTS.md` present
- [x] ERROR when render disabled + `AGENTS.md` missing
- [x] WARN `brief-fragments-unused` when enabled recipes declare `[provides.brief]`
- [x] `_check_brief_render_policy()` after `_check_agents_md()`

### B6 — Docs
- [x] `docs/ai-specs-toml.md` — `[brief].render`, precedence, subrepo inheritance
- [x] `templates/ai-specs.toml.tmpl` — commented `render = false` example

### B7 — Regression
- [x] Marker + render=true preservation (`test_render_true_marker_still_preserves_file`)
- [x] Render false without marker untouched
- [x] Two syncs byte-stable with render false
- [x] `./tests/validate.sh` green

---

## Files Changed (apply commit)

| File | Action | Role |
|------|--------|------|
| `lib/_internal/brief-render-policy.py` | Created | Policy parser + CLI |
| `lib/sync.sh` | Modified | Root render guard |
| `lib/init.sh` | Modified | Init render guard + placeholder |
| `lib/sync-agent.sh` | Modified | Subrepo render guard |
| `lib/_internal/doctor.py` | Modified | INFO/WARN diagnostics; `Severity.INFO` |
| `lib/_internal/recipe-materialize.py` | Modified | `attach_brief_fragments_to_resolved()` |
| `docs/ai-specs-toml.md` | Modified | Manifest docs |
| `templates/ai-specs.toml.tmpl` | Modified | Commented example |
| `tests/test_brief_render_policy.py` | Created | B1 unit tests |
| `tests/test_agents_md_render_opt_out.py` | Created | B2–B4, B7 E2E |
| `tests/test_doctor.py` | Modified | B5 doctor tests |

---

## Deviations from Design

1. **Test helper `append_brief_render_false`**: Initial version treated template *comments* (`# [brief]`, `# render = false`) as real config and never wrote an active `[brief]` table. Fixed by always appending a real `[brief]\nrender = false` block at EOF (TOML table merge).
2. **`Severity.INFO`**: Design specified INFO-level doctor output; enum only had OK/WARN/ERROR — added INFO + summary count.
3. **Fragment detection in doctor**: `build_resolved_config()` alone omits catalog `brief_fragments`; extracted `attach_brief_fragments_to_resolved()` for doctor and materialize paths.

No behavioral deviation from `design.md` contract messages or precedence (`render=false` > marker > normal render).

---

## Risks / Notes

- Subrepo tests pin `subrepos = ["packages/a"]` to avoid multi-target fixture noise (`packages/b` without manual `AGENTS.md`).
- Dogfood `ai-specs/ai-specs.toml` **not** changed (`render` still default true) per spike scope.
- Archive step still required: merge delta specs into `openspec/specs/`.
