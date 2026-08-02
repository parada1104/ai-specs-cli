# Verification Report: tracker-card-gate

## Verdict

**needs_fixes**

The implementation at HEAD `2119713f509ad7bec22af86585ac8146bd42da73` is present and the focused hermetic suites are green. Two contract issues prevent a clean verification: the implementation accepts a `## Tracker` section with `card_id` but no `url`, although the `trello-card-linking` delta requires both fields; and the strict-TDD evidence table is only a prose summary, without the per-task/file `RED`/`GREEN` evidence required by the verification guidance. A shell archive edge case also bypasses the design's required any-deficient fallback.

## Scope and structured status

- **Change selection:** exact, unambiguous `tracker-card-gate`.
- **Authoritative worktree:** `/Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate`.
- **Branch / HEAD:** `feat/tracker-card-gate` / `2119713f509ad7bec22af86585ac8146bd42da73`.
- **Worktree state:** clean at verification start.
- **Artifact store:** `openspec`.
- **Planning root:** `openspec/changes/tracker-card-gate/`.
- **Artifacts read in full:** `explore.md`, `proposal.md`, `design.md`, all six delta `spec.md` files, `tasks.md`, `apply-progress.md`, and `openspec/config.yaml`.
- **Artifact status:** proposal/design/specs/tasks/apply-progress present; this report is now present; no sync report is required by the task.
- **Task progress:** 48/51 checkboxes checked; three closure items remain open (listed below).
- **Apply state:** implementation work complete; ready for independent verification.
- **Action context:** `repo-local`; authoritative workspace root is the worktree above. The only file written by this phase is `openspec/changes/tracker-card-gate/verify-report.md`.
- **Next recommended action:** reconcile the URL-validity contract and shell archive fallback, then refresh strict-TDD evidence before archive/PR.

## Evidence by spec area

| Delta area | Result | Evidence |
|---|---|---|
| `trello-card-linking` | **Partial / blocker** | `lib/_internal/trello_link.py` implements tolerant section parsing, first-artifact precedence, key normalization, comments/backticks, duplicate-first semantics, fenced-sample exclusion, and a shared validity predicate. The skill, command, README, proposal, and config use the `## Tracker` contract; `tracker.none` is documented and honored. However, the delta says a valid section MUST contain non-empty `card_id` **and** `url` (`specs/trello-card-linking/spec.md:18-23`, scenarios at `:37-44` and project-doctor's invalid-link wording). The implementation's validity predicate is `bool(data.get("card_id"))` only (`trello_link.py:131-134`), the gate twin does the same (`tracker-card-gate.sh:155-157`), and the tests explicitly treat card-only/no-URL as valid (`tests/test_trello_link.py:114-125`). Doctor emits an INFO for missing URL instead of treating it as deficient (`doctor.py:616-631`). This conflicts with the delta; proposal/design text instead describes URL as expected/recommended and INFO-only, so the planning chain needs one authoritative resolution. |
| `tracker-card-gate` | **Partial / warning** | Recipe `1.3.0` declares both blocking `pre-tool-use` hooks with the required matchers and common script. The script covers mode resolution (`off|warn|always`), env override, marker activation, production path override, `openspec/**` exemption, `tracker.none`, parser/lookup fail-open behavior, path mode, shell `gh pr create`, archive helpers, and Cursor-native shell input. Focused gate tests cover these paths and passed. Design Decision 4c requires an archive whose source slug cannot be resolved to fall back to any-deficient evaluation (`design.md:392-399`). The implementation passes an unrecognized parsed slug as a focus to `_eval_deficient` (`tracker-card-gate.sh:597-605`), which returns no deficiency and allows the action. A direct hermetic probe with one deficient active change and `openspec archive unknown` returned `rc 0` with no warning under `always`; this should fall back to the any-deficient rule. |
| `project-doctor` | **Pass subject to URL contract** | `_check_tracker_card_link` is registered in the doctor run sequence, checks enabled recipe plus cache/local marker, scans only active change directories, honors `tracker.none`, uses sibling-loaded `trello_link.py`, emits WARN without changing ERROR-only exit behavior, and remains read-only (`doctor.py:561-652`). Dedicated tests cover missing, valid, exemption, disabled, absent marker, archive, empty ID, and read-only scenarios; they passed. Missing URL is currently INFO, which is consistent with design but not the literal `trello-card-linking` delta. |
| `trello-state-sync` | **Pass (guidance surface)** | Recipe brief rule and skill/command phase map require phase list/label/comment synchronization and resolve identity from `## Tracker`; availability failures remain documented as warnings/degrade, while missing artifact is not an availability excuse. The runtime sync hooks remain deferred as explicitly out of scope. |
| `trello-progress-comment` | **Pass (guidance surface)** | Recipe brief rule and skill require milestone progress comments, including apply/verify, using the linked section; `tracker.none` is the documented exception; MCP/network/API unavailability is documented as warning/continue. Deferred sync-time MCP execution remains out of scope. |
| `session-bootstrap` | **Pass** | `catalog/recipes/session-context/skills/session-bootstrap/SKILL.md` makes tracker consultation mandatory for new/ambiguous focus when bound, keeps memory-first ordering, and degrades when the provider is unavailable. Recipe tests assert mandatory wording and absence of the old `only if needed` rule. |
| Dogfood/config/docs/evals | **Pass** | `ai-specs/ai-specs.toml` sets `gate_mode = "warn"`; `openspec/config.yaml` contains the requested tracking block and aligned decision matrix; recipe README, runtime-hook docs, catalog, skill, and command document residual Cursor/OpenCode/pi-omp gaps and explicitly avoid MCP interception. Four live golden scenario directories, the live runner, and harness loading are present. Live execution was not attempted because it is opt-in/manual and no supported runtime was selected. |

## Design and proposal decisions checked

- Canonical section-first contract, no new per-change artifact, tolerant parser, proposal/tasks fallback, and grandfathered archives are implemented/documented.
- Activation is marker + non-off mode; recipe enablement is implied by materialized hooks, with a project-local marker fallback for hermetic tests.
- `GATE_MODE_PLACEHOLDERS` stamps worktree and tracker modes independently; `__TRACKER_CLI_HOME__` is stamped and the materializer call site passes `cli_home`.
- The two-hook Cursor separation is implemented and exercised by recipe/render tests.
- Production defaults are `lib catalog bin src`, with `TRACKER_CARD_GATE_PATHS` opt-in extension for `ai-specs`.
- The gate does not call Trello MCP; it uses artifact presence as proof.
- Doctor is WARN-only and read-only; no archive migration or default FAIL was introduced.
- The explicit apply-progress deviations are visible: on-disk `tracker.none`, recipe-scoped live eval filename, and authorized single-branch application despite the high/conditional chain forecast.

## Task-state accounting

`tasks.md` has **48 checked / 3 unchecked / 51 total** checkbox items. The exact remaining lines are:

```text
- [ ] 12.6 FINAL GATE: `./tests/validate.sh` green from the change worktree
- [ ] 12.7 Independent verify against proposal/design/specs/tasks; write
- [ ] 12.8 Archive the change folder on the review branch after verify PASS
```

Accounting for this verification handoff:

- **12.6:** passed externally by the orchestrator from this worktree: `./tests/validate.sh`, **1172 tests OK, exit 0**. It was not rerun here per assignment constraint.
- **12.7:** this report fulfills the verification artifact portion, but the checkbox is intentionally not edited by this executor.
- **12.8:** archive + PR remains orchestrator-owned and pending; it must not be treated as complete.
- No unchecked implementation phase (parser, gate, materializer, doctor, recipe/docs, dogfood, hermetic, or live-eval implementation work) remains. The three unchecked items are close/ownership tasks, not a reason to claim archive readiness before this report's blockers are resolved.

## Validation commands and results

Commands run from the change worktree:

1. `python3 -m unittest tests.test_trello_link tests.test_tracker_card_gate_hook tests.test_doctor_tracker_card tests.test_trello_mcp_workflow_recipe` — **52 tests, OK**.
2. `python3 -m unittest tests.test_recipe_materialize tests.test_hooks_render tests.test_trello_mcp_workflow_recipe tests.test_session_context_recipe tests.test_recipes_catalog tests.test_doctor` — **195 tests, OK**.
3. `python3 -m unittest tests.evals.eval_harness_smoke tests.test_eval_hook_wiring` — **29 tests, OK**.
4. `env -u EVALS_LIVE -u EVALS_RUNTIMES -u EVALS_RUNTIME python3 -m unittest tests.evals.eval_trello_mcp_workflow_live -v` — **1 test skipped** as expected because live mode/runtime were unavailable; no failure.
5. External orchestrator result (not rerun): `./tests/validate.sh` — **1172 tests OK, exit 0**.
6. Additional direct probes: a card-only/no-URL `## Tracker` section allowed a production edit under `always` (`rc 0`); an archive command with an unresolved slug and another deficient active change also allowed under `always` (`rc 0`). These probes demonstrate the two contract gaps described above.

The apply-progress artifact also records `./tests/run.sh` as 1224 OK on 2026-08-02. That historical count differs from the orchestrator's authoritative final-gate count (1172); this report relies on the explicitly supplied external `validate.sh` result and does not rerun a project-wide suite.

## Strict TDD compliance

Strict TDD is active in `openspec/config.yaml` and the parent assignment. `apply-progress.md` contains a `TDD Cycle Evidence` table and the commit sequence includes dedicated test/RED and implementation commits (for example `06b009e` for the Phase 2 RED harness, followed by `3838ac4` for the path/shell implementation; parser tests and implementation are in `6db316f`). All reported test files exist and the focused suites above remain green.

The evidence is **incomplete for strict verification**: the table contains prose such as `failing test_trello_link` and `parser module`, but does not provide the required per-task test-file rows, explicit `RED: Written` / `GREEN: Passed` results, triangulation counts, or safety-net fields. The table also summarizes phases rather than all TDD tasks. This is a CRITICAL evidence blocker even though the implementation tests pass; apply-progress should be refreshed with verifiable per-task RED/GREEN evidence before archive.

### TDD Compliance

| Check | Result | Details |
|---|---|---|
| TDD evidence table present | PASS | `apply-progress.md:74-85` contains the table. |
| Test files cross-referenced | PASS | Parser, gate, doctor, recipe, harness, and live-eval files exist. |
| RED evidence format complete | FAIL | No explicit `Written` markers or per-task/file mapping. |
| GREEN evidence format complete | FAIL | No explicit `Passed` markers or per-task execution mapping. |
| Focused GREEN execution now | PASS | 52 + 195 + 29 affected tests passed. |
| Triangulation/safety-net evidence | INCOMPLETE | Not recorded in the apply-progress table. |

**TDD compliance:** implementation tests are green, but the required evidence artifact is incomplete; do not call strict-TDD verification clean.

### Test layer distribution

- **Unit:** parser and doctor logic in `tests/test_trello_link.py` and `tests/test_doctor_tracker_card.py`.
- **Integration/contract:** shell gate, recipe materialization, hook rendering, session recipe, and catalog tests.
- **Live/E2E-style:** four manual/nightly golden scenarios exist; not executed because live mode/runtime was unavailable. Config reports integration/e2e tools unavailable.

### Assertion quality

**PASS — no assertion-quality blocker found.** The changed tests call production code and assert concrete return codes, parsed values, emitted diagnostics, generated files, hook registrations, and read-only behavior. No tautological assertions, CSS/implementation-detail assertions, empty-loop ghost tests, or smoke-only tests were found. Loops iterate over explicit non-empty fixture matrices/command lists; type/existence assertions are paired with behavioral assertions where applicable.

## Quality signals unavailable by configuration

As required by `openspec/config.yaml:27-39` and `:69-72`:

- **Coverage:** unavailable; no coverage command/tool is configured. Coverage analysis was skipped.
- **Linter:** unavailable; no linter is configured.
- **Type checker:** unavailable; no type checker is configured.
- **Formatter:** unavailable; no formatter is configured.

These are unavailable quality signals, not failures. The configured validation command was supplied externally by the orchestrator and passed.

## Review workload / PR boundary

`tasks.md` forecasts High review workload, conditionally recommends a three-PR chain (core gate/parser/materializer/doctor; recipe/docs/dogfood; live evals), and names `feature-branch-chain` as the strategy. `apply-progress.md` explicitly records parent authorization for a single full-branch apply and recommends preserving the PR split as an archive/delivery choice. The observed implementation stays within the proposal's affected areas; no unrelated scope creep or `size:exception` was found. Archive/PR work should honor the recorded boundary decision and should not be treated as complete until the blockers above are resolved.

## Exact blockers and risks

### Critical blockers

1. **URL validity contract is unresolved and currently violated.** The delta specification requires both non-empty `card_id` and `url`, while proposal/design/tests/implementation use card-id-only validity with URL as INFO/recommended. Reconcile the planning chain and then align parser, gate twin, doctor, docs, and tests. Until then, a card-only artifact can pass `always` enforcement contrary to the literal delta.
2. **Strict-TDD evidence is not auditable at the required granularity.** Add per-task test file, RED, GREEN, triangulation, safety-net, and execution evidence to `apply-progress.md` (without rewriting history or claiming unobserved results). This phase does not modify that artifact.

### Warning

3. **Unresolved archive slug bypasses the design fallback.** `openspec archive unknown` (or an archive move whose source slug cannot be resolved) should evaluate any deficient active change per `design.md:397-399`; the current focused evaluation returns no deficiency and allows it under `always`. Add a regression test and fix the gate in a subsequent implementation phase.
4. **Live Trello/MCP execution is unavailable/not run.** Four notes-file golden scenarios are present and harness discovery is green, but no live runtime or MCP board mutation evidence exists in this verification. This is expected because live evals are manual/nightly and the optional MCP-live scenario is not CI-gating.

## Archive recommendation

**Do not archive yet.** Resolve the two critical blockers, add the archive fallback regression coverage, rerun the affected focused suites, and have the orchestrator complete task 12.8 only after a new verification pass is clean.
 
---

## Re-verify after remediation (2026-08-02)

### Re-verify verdict

**PASS.** The three first-run findings are resolved at remediated HEAD `90ed152fe001bf7e942164dbaff6314b71ca185f` on `feat/tracker-card-gate`. No implementation or contract blocker remains. This is a verification pass; archive and final checkbox accounting remain orchestrator-owned.

### Structured status and action context

```yaml
schemaName: spec-driven
changeName: tracker-card-gate
artifactStore: openspec
planningHome:
  root: /Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate/openspec
  changesDir: /Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate/openspec/changes
changeRoot: /Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate/openspec/changes/tracker-card-gate
artifacts:
  proposal: done
  specs: done
  design: done
  tasks: done
  applyProgress: done
  verifyReport: done
  syncReport: not_applicable
taskProgress:
  total: 51
  complete: 48
  remaining: 3
  unchecked:
    - "- [ ] 12.6 FINAL GATE: `./tests/validate.sh` green from the change worktree"
    - "- [ ] 12.7 Independent verify against proposal/design/specs/tasks; write"
    - "- [ ] 12.8 Archive the change folder on the review branch after verify PASS"
applyState: all_done
dependencies:
  apply: all_done
  verify: all_done
  sync: not_applicable
  archive: blocked
actionContext:
  mode: repo-local
  workspaceRoot: /Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate
  allowedEditRoots:
    - /Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate/openspec/changes/tracker-card-gate/verify-report.md
  warnings: []
nextRecommended: orchestrator completes closure accounting and archives only after this PASS is accepted
```

The three unchecked lines are close/ownership items, not unfinished implementation tasks. They are intentionally preserved because this executor must not edit `tasks.md`: 12.6 passed externally, 12.7 is this report, and 12.8 remains orchestrator-owned.

### Finding 1: URL validity contract drift — resolved

The reconciled contract is now consistent across the planning chain and implementation:

- Proposal Locked Decision #2 defines `card_id` as the validity requirement and `url` as recommended.
- `design.md` Decision 1 states the shared predicate is a `## Tracker` section yielding a non-empty `card_id`; missing `url` is an `INFO` nudge and never a block.
- `trello-card-linking/spec.md` requires non-empty `card_id`, says `url` SHOULD be recorded when available, and explicitly says its absence does not invalidate the link.
- `project-doctor/spec.md` uses the same predicate and explicitly limits missing `url` to an informational nudge.
- `tracker-card-gate/spec.md` now describes a valid section as non-empty `card_id` with `url` recommended when available.
- `openspec/config.yaml` documents the same validity interpretation next to `required_fields: [card_id, url]`.
- `trello_link.py:is_valid_link` and the embedded gate parser both return validity from `bool(card_id)`; doctor emits `INFO` for a missing URL without changing validity.

The parser test `test_empty_card_id_invalid_nonempty_valid_without_url` exercises both sides of the boundary. The focused suite passed 53 tests, including parser/gate parity and doctor coverage. The former blocker is resolved.

### Finding 2: Strict-TDD evidence granularity — resolved

Strict TDD remains enabled by `openspec/config.yaml`. `apply-progress.md` now contains a `TDD Cycle Evidence` table with one row per phase/task, test-file paths, RED and GREEN evidence, triangulation notes, safety-net references, commit hashes, and explicit caveats where no separate RED commit or live execution was observed. The caveat that RED markers do not infer an unobserved failing transcript is honest and auditable rather than a claimed result.

All listed focused test files exist. Current execution confirms GREEN:

- Parser, gate, doctor, and recipe contract suites: 53 tests, OK.
- Materializer and general doctor contract suites: 143 tests, OK.
- The new archive regression is included in the 53-test run and passes.

The evidence table's 52-test historical safety-net count predates the archive regression; it explicitly says the regression was rerun separately during remediation. The current 53-test execution is the authoritative re-verification result. No strict-TDD evidence blocker remains.

### Finding 3: unresolved archive-slug fallback — resolved

Commit `6dd3309` adds `test_archive_unresolved_slug_falls_back_to_any_deficient`, covering both `always` (exit 2) and `warn` (exit 0) modes while asserting that the other deficient active slug is named in stderr. Commit `0704b13` changes the gate so an archive focus is used only when the source slug resolves to an existing active-change directory; an unresolved slug now calls `_eval_deficient` without a focus and therefore applies the design 4c any-deficient fallback. The focused suite passes the regression.

### Spec and implementation coverage

The full artifact set was reread: `explore.md`, `proposal.md`, `design.md`, all six delta specs (`trello-card-linking`, `trello-state-sync`, `trello-progress-comment`, `session-bootstrap`, `project-doctor`, and `tracker-card-gate`), `tasks.md`, `apply-progress.md`, and `openspec/config.yaml`. The remediation is limited to the three reported findings; canonical parsing, activation, `tracker.none`, production-path enforcement, shell coverage, doctor WARN/read-only behavior, recipe rendering/materialization, session-bootstrap guidance, state-sync/progress-comment brief rules, dogfood `warn`, and archive grandfathering remain covered by the prior passing evidence.

### Validation commands and results

Commands run from `/Users/robert/proyectos/nnodes/ai-specs-cli-tracker-card-gate`:

1. `python3 -m unittest tests.test_trello_link tests.test_tracker_card_gate_hook tests.test_doctor_tracker_card tests.test_trello_mcp_workflow_recipe` — **53 tests, OK**.
2. `python3 -m unittest tests.test_recipe_materialize tests.test_doctor` — **143 tests, OK**.
3. `./tests/validate.sh` — **not rerun** by this executor, per assignment constraint; orchestrator already reported **1172 tests OK, exit 0**.

An initial invocation of command 1 from the parent repository (rather than the authoritative worktree) produced four module-import errors; it was not a worktree test run. The exact command was rerun from the authoritative worktree and produced the 53-test result above.

Strict-TDD configuration reports no coverage, linter, type-checker, or formatter tool. Coverage and quality metrics are therefore unavailable and are informational, not blockers. Assertion-quality review of the affected tests found no tautologies, ghost loops, type-only assertions standing alone, smoke-only assertions, or implementation-detail/CSS assertions. The new regression calls the real hook and checks return codes and stderr content.

### Review workload and PR boundary

The change remains High review workload with the conditional three-PR forecast and `feature-branch-chain` strategy in `tasks.md`. The parent-authorized single-branch implementation is within the recorded boundary; remediation commits touch only the gate regression/fix and planning evidence/spec reconciliation. No unrelated scope creep or `size:exception` was observed. Archive/PR delivery remains orchestrator-owned and should preserve the recorded boundary choice.

### Updated blockers and risks

**Critical blockers:** none for verification. The URL validity contract, strict-TDD evidence granularity, and unresolved archive fallback findings are all resolved and independently rechecked.

**Remaining non-blocking risks:**

1. Live Trello/MCP scenarios remain manual/optional and were not run; this is the documented non-CI-gating limitation, not a remediation failure.
2. `tasks.md` still intentionally contains the three close/ownership checkboxes quoted above. Archive is not complete until the orchestrator records 12.6/12.7 and performs 12.8.
3. Coverage, linter, type-checker, and formatter signals are unavailable by configuration.

## Final verdict

**PASS — re-verification is clean at `90ed152`.** The implementation satisfies the reconciled proposal/design/spec contract, the refreshed TDD evidence is auditable and honest, and the archive fallback regression is fixed and green. No further code, test, or planning-artifact remediation is required before the orchestrator performs the remaining closure accounting and archive step.
