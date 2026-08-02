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

## TDD Cycle Evidence

Evidence below is limited to committed test files, observable commit history,
and test results recorded or rerun in this worktree. A RED marker says whether
the test was separately committed before implementation; it does not infer an
unobserved failing-run transcript.

| Phase/task | Test file(s) | RED Written | GREEN Passed | Triangulation | Safety-net |
|---|---|---|---|---|---|
| 1.1 parser | `tests/test_trello_link.py` | Written in `6db316f` (no separate RED commit) | Passed: parser implementation and tests committed in `6db316f` | Python parser + gate parity fixture in `tests/test_tracker_card_gate_hook.py` | Focused suite below |
| 2.1 gate harness | `tests/test_tracker_card_gate_hook.py` | Written/RED commit `06b009e` | Passed: gate implementation in `3838ac4` | Path and shell event matrices | Focused suite below |
| 3.1 path gate | `tests/test_tracker_card_gate_hook.py` | Existing RED harness `06b009e` | Passed: `3838ac4` | Path-mode cases and fail-open cases | Focused suite below |
| 4.1 shell gate | `tests/test_tracker_card_gate_hook.py` | Existing RED harness `06b009e` | Passed: `3838ac4` | Shell `gh pr create` and archive cases | Focused suite below |
| 5.1 materialize/hooks | `tests/test_trello_mcp_workflow_recipe.py` | Written in `46fd59b` (no separate RED commit) | Passed: materializer and dual-hook implementation in `46fd59b` | Recipe render/materialize assertions | Focused suite below |
| 6.1 doctor | `tests/test_doctor_tracker_card.py` | Written in `0733f73` (no separate RED commit) | Passed: doctor implementation in `0733f73` | Doctor tracker tests + general doctor suite | Focused suite below |
| 7.1 skill/command | `tests/test_trello_mcp_workflow_recipe.py` | Written in `9874b3e` (no separate RED commit) | Passed: skill/command changes in `9874b3e` | Recipe contract assertions | Focused suite below |
| 8.1 docs/config | `tests/test_trello_mcp_workflow_recipe.py` | No dedicated RED test commit observed | Passed: docs/config commit `296f5a5`; contract assertions in recipe suite | Declarative config plus generated recipe surfaces | Focused suite below |
| 9.1 dogfood | `tests/test_trello_mcp_workflow_recipe.py` | No separate RED commit observed | Passed: dogfood config commit `9324028` | Warn-mode configuration and generated-file isolation | Focused suite below |
| 10.1 hermetic eval harness | `tests/evals/eval_harness_smoke.py` | Written in `b9bf5a8` (no separate RED commit) | Passed: harness tests committed in `b9bf5a8` | Scenario discovery and hook wiring | Focused suite below |
| 11.1 live eval client | `tests/evals/eval_trello_mcp_workflow_live.py` | Written in `b9bf5a8` (no separate RED commit) | Passed: client/scenario files committed in `b9bf5a8`; live execution not observed | Four scenario directories and smoke harness | Live run intentionally not executed |
| 12.1 fenced Tracker parsing | `tests/test_trello_link.py` | Written in `1769ca9` (no separate RED commit) | Passed: parser/gate fence fix in `1769ca9` | Python parser and shell twin | Focused suite below |

**Safety-net execution:** `python3 -m unittest tests.test_trello_link tests.test_tracker_card_gate_hook tests.test_doctor_tracker_card tests.test_trello_mcp_workflow_recipe` — **52 OK**, as recorded before the archive-fallback regression. The new regression is rerun separately during remediation; no unobserved historical count is claimed here.

`./tests/run.sh` — **1224 OK** (2026-08-02 recovery verify, historical apply
record). `./tests/validate.sh` was not run during apply (assignment constraint).

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
