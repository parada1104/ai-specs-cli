# Tasks: worktree-flow recipe modes (`always` / `ask` / `off`)

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~260–370 |
| 400-line budget risk | Medium |
| Chained PRs recommended | Yes (preflight force-chained) |
| Suggested split | PR 1 schema+stamp+hook dispatch → PR 2 docs/README → PR 3 test hardening only if over budget |
| Delivery strategy | auto-chain |
| Chain strategy | feature-branch-chain (tracker `feature/worktree-flow-modes` → `development`; PR #1 base=tracker; PR #2 base=PR #1) |

Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: feature-branch-chain
400-line budget risk: Medium

### Suggested Work Units

| Unit | Goal | Likely PR | Notes |
|------|------|-----------|-------|
| 1 | Schema enum + sync stamping + hook dispatch (AC1–AC8) | PR 1 | base=tracker; tests included with behavior |
| 2 | README + docs alignment + template frontmatter | PR 2 | base=PR 1; docs-only diff |
| 3 (optional) | Test breadth hardening | PR 3 | only if Unit 1+2 crosses 400 lines |

## Phase 1: Config / Schema (Foundation)

- [x] 1.1 RED: extend `tests/test_worktree_flow_recipe.py` with `test_sync_defaults_to_always` and `test_sync_rejects_invalid_gate_mode` (diagnostic names value + `always | ask | off`).
- [x] 1.2 Modify `lib/_internal/recipe_schema.py` to accept `enum` metadata on `[config.<key>]` and validate values against it; reject unknown metadata.
- [x] 1.3 Add `[config.gate_mode]` to `catalog/recipes/worktree-flow/recipe.toml`: required=false, type=string, default="always", enum=["always","ask","off"].
- [x] 1.4 GREEN: pass 1.1; record evidence via `./tests/run.sh`.

## Phase 2: Sync / Materialization (Stamping)

- [x] 2.1 RED: add `test_sync_materializes_gate_mode_into_hook` asserting the rendered `worktree-gate.sh` copy carries the resolved mode.
- [x] 2.2 Modify `lib/_internal/recipe-materialize.py`: validate enum fields in `merge_config()`; pass `merged_cfg` into `materialize_hook_script()` and replace placeholder `__WORKTREE_GATE_MODE__` in the project copy. Do NOT export `WORKTREE_GATE_MODE` in hook env (preserve override precedence).
- [x] 2.3 GREEN: pass 2.1.

## Phase 3: Hook / Runtime Dispatch

- [x] 3.1 RED: extend `tests/test_worktree_gate_hook.py` with `test_gate_off_self_disables`, `test_gate_ask_blocks_with_bypass_hint`, `test_env_override_beats_stamped`, `test_empty_env_keeps_stamped`, `test_linked_worktree_always_allowed_in_always`.
- [x] 3.2 Modify `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`: insert `stamped_gate_mode="__WORKTREE_GATE_MODE__"`; resolve `gate_mode="${WORKTREE_GATE_MODE:-$stamped_gate_mode}"`; warn+fallback on invalid env; `off` → early `exit 0`; `ask` → existing block plus stderr hint naming `WORKTREE_GATE_MODE=off`; `always` unchanged; keep `WORKTREE_GATE_PROTECTED` orthogonal; invalid/missing stamp falls back to `always`.
- [x] 3.3 GREEN: pass 3.1.
- [x] 3.4 REFACTOR: extract dispatch into labelled early-return sequence; preserve fail-open → `always` on parse error.

## Phase 4: Docs / Recipe README

- [ ] 4.1 Update `catalog/recipes/worktree-flow/README.md`: modes table, default, `ask` host-mediation caveat, one-shot `WORKTREE_GATE_MODE=off` override, recommended pick per team.
- [ ] 4.2 Update `docs/recipes-catalog.md`, `docs/runtime-hooks.md`, `docs/ai-specs-toml.md`, `docs/recipe-schema.md` to cover `gate_mode`, enum validation, and stamping flow.
- [ ] 4.3 If design file-change table lists it, add a `gate_mode` example to `templates/ai-specs.toml.tmpl`; otherwise skip.

## Phase 5: Tests / Verification

- [ ] 5.1 Run `./tests/run.sh`; ensure all `test_worktree_gate_hook.py` + `test_worktree_flow_recipe.py` scenarios pass (AC1–AC8 in spec).
- [ ] 5.2 Run `./tests/validate.sh` (py_compile + `bash -n` + tests); fix drift.
- [ ] 5.3 Cross-check every spec scenario against hook/sync behavior; state unavailable quality signals (coverage/lint/type/formatter: none configured).
- [ ] 5.4 Manual smoke: scratch project per mode; confirm `off` bypass path; record evidence in PR.