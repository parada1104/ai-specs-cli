# Apply Progress: tracker-card-gate

**Status**: Ready for verify (implementable Phases 1–12 complete)  
**Branch / worktree**: `feat/tracker-card-gate` @ `/Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate`  
**HEAD**: `1769ca9` (+ this apply-progress commit)  
**Strict TDD**: active (`openspec/config.yaml` apply.tdd + parent prompt)

## Structured status consumed

| Field | Value |
|-------|-------|
| change | `tracker-card-gate` |
| applyState | in_progress → ready_for_verify |
| actionContext.mode | apply (parent authorized FULL Phases 1–12) |
| allowedEditRoots | worktree repo root |
| Review Workload | High / Conditional chain; parent authorized single-branch full apply with per-phase commits |
| Delivery path | parent: execute ALL Phases 1–12; do not run `validate.sh` |

## Completed phases / tasks

Persisted checkboxes in `tasks.md`: **48 checked / 3 open** (51 total).

| Phase | Tasks | Evidence |
|-------|-------|----------|
| P0 Planning | 6/6 | Authorization + plan lock committed |
| 1 Parser | 2/2 | `lib/_internal/trello_link.py` + `tests/test_trello_link.py` |
| 2 Gate RED harness | 3/3 | `tests/test_tracker_card_gate_hook.py` |
| 3 Path-mode GREEN | 3/3 | `catalog/.../hooks/tracker-card-gate.sh` |
| 4 Shell-mode | 2/2 | same script; gh pr create + archive helpers |
| 5 Materialize + dual hooks | 4/4 | recipe.toml dual ids; GATE_MODE_PLACEHOLDERS map |
| 6 Doctor WARN | 2/2 | `lib/_internal/doctor.py` + `tests/test_doctor_tracker_card.py` |
| 7 Skill/command/bootstrap | 4/4 | SKILL.md, trello-workflow.md, session-bootstrap |
| 8 Docs + config.yaml | 5/5 | decision_matrix + `tracking:` section |
| 9 Dogfood warn | 2/2 | `ai-specs/ai-specs.toml` `gate_mode=warn`; generated state reverted |
| 10 Hermetic smoke | 2/2 | focused suites green |
| 11 Live evals | 8/8 | `eval_trello_mcp_workflow_live.py`, `run-live-trello.sh`, scenarios |
| 12 Close | 5/8 | CHANGELOG, ## Tracker in proposal, Trello progress; 12.6–12.8 open |

### Remaining unchecked lines

```text
- [ ] 12.6 FINAL GATE: `./tests/validate.sh` green from the change worktree
- [ ] 12.7 Independent verify against proposal/design/specs/tasks; write
- [ ] 12.8 Archive the change folder on the review branch after verify PASS
```

`12.6` intentionally skipped per apply assignment (orchestrator runs validate).  
`12.7` / `12.8` are post-apply verify/archive.

## Files changed (implementation)

- `lib/_internal/trello_link.py` — canonical `## Tracker` parser (fenced samples ignored)
- `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` — dual-hook gate
- `catalog/recipes/trello-mcp-workflow/recipe.toml` — v1.3.0, dual hooks, gate_mode
- `lib/_internal/recipe-materialize.py` — `__TRACKER_CARD_GATE_MODE__` + `__TRACKER_CLI_HOME__`
- `lib/_internal/doctor.py` — WARN for missing tracker link
- Recipe skill/command/docs + session-bootstrap updates
- `openspec/config.yaml` — decision_matrix alignment + declarative `tracking:`
- `ai-specs/ai-specs.toml` — dogfood `gate_mode=warn`
- `CHANGELOG.md`
- `openspec/changes/tracker-card-gate/proposal.md` — task 12.2 `## Tracker`
- Tests: `test_trello_link.py`, `test_tracker_card_gate_hook.py`, `test_doctor_tracker_card.py`, recipe materialize/hooks coverage
- Live evals: `tests/evals/eval_trello_mcp_workflow_live.py`, `run-live-trello.sh`, `scenarios/trello-mcp-workflow/**`

## Test commands run

| Command | Result |
|---------|--------|
| `python3 -m unittest tests.test_trello_link tests.test_tracker_card_gate_hook tests.test_doctor_tracker_card tests.test_trello_mcp_workflow_recipe` | **52 OK** |
| `./tests/run.sh` | **1224 OK** (2026-08-02 recovery verify) |
| `./tests/validate.sh` | **not run** (assignment constraint) |
| Live `EVALS_LIVE` scenarios | scaffolded; not executed in apply |

## TDD Cycle Evidence (summary)

| Phase | RED | GREEN | Notes |
|-------|-----|-------|-------|
| 1 | failing `test_trello_link` | parser module | parity twin for gate later |
| 2–4 | failing gate harness | path+shell modes | exit 0/2/fail-open; openspec never blocked |
| 5 | materialize/recipe tests | placeholder map + dual hooks | version 1.2.0→1.3.0 |
| 6 | doctor tracker tests | WARN-only check | recipe+marker activation |
| 7–8 | recipe/docs/config assertions | skill+config.yaml | tracking declarative only |
| 9 | dogfood warn/always smoke | toml commit; revert generated | isolation honored |
| 10–11 | hermetic + live client tests | scenarios + runner | no live network in apply |
| 12 | fenced `## Tracker` false positive | parser ignore fences | proposal section written |

## Deviations from design

1. Exemption on-disk name aligned to **`tracker.none`** (design prose `tracker:none` path literal reconciled at call sites).
2. Live eval module named `eval_trello_mcp_workflow_live.py` (recipe-scoped), not a separate `eval_tracker_card_gate_live.py`.
3. Single-branch full apply despite High review-budget forecast — parent explicitly authorized Phases 1–12 on one feature branch; PR split remains a review/delivery choice for archive.

## Workload / PR boundary

- Forecast: High / Conditional chain (PR1 core → PR2 prose/dogfood → PR3 live evals).
- Applied as one feature branch with 10 apply commits + planning commits.
- Recommend verify next; archive/PR may still split review if desired.

## ## Tracker written (task 12.2)

```markdown
## Tracker

- **card_id**: `6a6ebd5e2cd9a2fcd419e62c`
- **shortLink**: `WHZ3fLzD`
- **url**: https://trello.com/c/WHZ3fLzD/56-sdd-gate-de-tracker-garantizar-card-trello-por-cambio-evals
- **list**: In Progress
```

## config.yaml result

- `sdd.decision_matrix`: `trivial` / `local_fix` / `behavior_change` / `domain_change` (spec vocabulary); local_fix has code+tests; behavior_change includes specs; domain_change includes apply/verify/archive reports; stale skill ref removed.
- `tracking:` declarative block with `tracker`, `board_id`, `artifact_section`, `required_fields`, `gate_mode: warn` — not used for doctor/gate enforcement.

## Next recommended

1. Independent `sdd-verify` → write verify-report (task 12.7)
2. Orchestrator `./tests/validate.sh` (task 12.6)
3. Archive + PR + Trello close (task 12.8)
