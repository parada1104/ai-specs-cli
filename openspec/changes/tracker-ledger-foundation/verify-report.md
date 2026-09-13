```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:289721dec642d555ee6d8743ce22035ccd1e481adaeab195a5e753e5af59912f
verdict: pass_with_warnings
blockers: 0
critical_findings: 0
requirements: 11/11
scenarios: 32/32
test_command: ./tests/validate.sh
test_exit_code: 0
test_output_hash: sha256:b314c54bbe4d095d49f35fb0e9de251accd0fb51f6630653ec5baabbd4dd3576
build_command: ./scripts/build-gate.sh
build_exit_code: 0
build_output_hash: sha256:42da1a2230b24d7f8321ddfa4150469cb814c69c05f8c1a3185b109453015258
```

# Verification Report: Tracker-only Go Ledger foundation

## Verify evidence

- Verdict: PASS WITH WARNINGS
- Command: ./tests/validate.sh
- Exit: 0
- Date: 2026-09-13
- Commit: f5de19842bd3d81c990f2410dfb96ce9ff3cb394
- Branch: change/tracker-ledger-foundation @ .worktrees/tracker-ledger-foundation
- ready_for_archive: false

Canonical verify-executor update (2026-09-13) — every prior apply-phase row below is preserved
unchanged. The Commit field now carries the current full HEAD SHA because the canonical
pre-archive guardian requires a bare 7-40 hex revision. The apply phase had recorded `dd677b2`
for units 1–6 with the Phase 7 slice uncommitted; that slice is now commit `f5de198` and the
worktree is clean at HEAD. Tasks 7.6 (archive) and 7.7 (PR) remain parent-owned closeout
prerequisites, so `ready_for_archive` stays false until closeout completes.

This executor implemented **only** Phase 7 tasks 7.1–7.5. Tasks 7.6 (`pre-archive`/`pre-merge`
guardian + archive move) and 7.7 (single `gh` PR) were deliberately not run; the exact handoff
is in **Remaining close-out tasks** below.

## Independent verification (canonical verify executor, 2026-09-13)

Independent Full SDD verification of implementation units 1–7.5, run inside the worktree at
HEAD `f5de19842bd3d81c990f2410dfb96ce9ff3cb394` (clean tree). Verification edited no code and
no production file; only this report was updated. All authoritative artifacts (proposal, spec,
design, tasks, apply-progress) were read from the worktree change folder, not the main checkout.

### Exact commands and results

| Command | Result |
|---|---|
| `./tests/validate.sh` | exit 0 — py_compile + `bash -n` + gofmt clean + Go `test ./...` (both packages ok) + `Ran 1947 tests in 745.947s` → `OK (skipped=2)` |
| `./scripts/build-gate.sh` | exit 0 — four targets built with canonical `go1.24.13`; digests of `dist/worktree-gate-*` (excluding `current`) match the committed trust root |
| `./scripts/verify-gate-sums.sh <generated> catalog/recipes/worktree-flow/bin/SHA256SUMS` | exit 0 — `ok — 4 digest entries match the committed trust root` |
| `go -C catalog/recipes/worktree-flow/gate test -count=1 ./...` | `ok ai-specs.dev/worktree-gate 15.534s` + `ok ai-specs.dev/worktree-gate/ledger 2.934s` |
| `gofmt -l catalog/recipes/worktree-flow/gate` and `go vet ./...` | gofmt empty; vet clean |
| `./dist/worktree-gate-current --selftest` | prints `ok`, exit 0 |
| `./dist/worktree-gate-current --ledger --checkpoint work-start --ledger-mode warn --project-root $PWD` | exit 0 — JSON `decision=dormant reason=witness-missing active=false` carrying the full design key set (capability, active, checkpoint, mode, decision, reason, identity{common_dir, branch, change, key}, item, conflict, prompt, doctor) |
| focused suites: `tests.test_tracker_ledger_parity`, `tests.test_doctor_tracker_card`, `tests.test_tracker_ledger_witness`, `tests.test_ledger_mode_config`, `tests.test_tracker_card_gate_hook`, `tests.test_plan_build_gate_hook`, `tests.test_premerge_guardian`, `tests.test_worktree_gate_parity`, `tests.test_trello_link`, `tests.test_trello_mcp_workflow_recipe`, `tests.test_doctor` | `Ran 269 tests in 108.547s` → OK, exit 0 |
| `python3 lib/_internal/premerge_guardian.py --root . --stage pre-archive tracker-ledger-foundation` | exit 1 — BLOCKED only on `verify-report.md is missing Commit/SHA/Revision (7-40 hex)` (this report's commit-field format; fixed by this update); the ledger checkpoint itself was non-blocking (dormant) and tier minima passed |
| `python3 lib/_internal/premerge_guardian.py --root . --stage pre-merge tracker-ledger-foundation` | exit 1 — BLOCKED on the missing dated archive (expected pre-closeout state; the archive move is parent-owned task 7.6) |

### Unit evidence cross-check (units 1–7.5)

| Unit | Runtime evidence confirmed by this verification |
|---|---|
| 1 — identity + witness (Go) | `ledger/identity.go` + `witness.go` and tests present; live probe on this repo resolved `change=tracker-ledger-foundation` through the archive-aware lookup and reported dormant on the missing witness |
| 2 — store (Go) | `ledger/store.go` + tests; D17 branch-reuse, D19 checkpoint-scoped opt-out, two-open collision and provider-neutral key-set tests all green in the Go run |
| 3 — verdict + CLI | `--ledger` contract verified live (exact design JSON keys, exit 0 dormant); `--selftest` still prints `ok` with the ledger invariants |
| 4 — witness producer | atomic writer in `lib/_internal/recipe-materialize.py`; `tests.test_tracker_ledger_witness` (14 tests) green; synthetic fixture recipes present and internal-gated |
| 5 — checkpoint hosts | `plan-build-gate.sh` (work-start), `tracker-card-gate.sh` (apply-start / pr-review), `premerge_guardian.py` (archive-close / pre-merge) invoke the verified binary; heredoc grader gone (grep for `is_valid_link`, `_eval_deficient`, `marker_present`, `_emit_and_exit` → no match); `[config.ledger_mode]` enum in `recipe.toml`; `pr-create.md` points at the `pr-review` verdict |
| 6 — one grader + parity | `doctor.py` renders `_check_tracker_ledger`; legacy `_check_tracker_card_link` / `_load_trello_link` gone (grep clean); `trello_link.py` parser-only; 24 corpus fixtures + parity runner green |
| 7 — docs + trust | sections present in `docs/capabilities.md`, `docs/runtime-hooks.md`, `README.md`, `CHANGELOG.md`, tracker recipe README; trust root regenerated with canonical `go1.24.13` and byte-reproducible across two builds |

### Spec/design compliance matrix

11 requirements and 32 scenarios in `specs/tracker-ledger/spec.md`, all verified at runtime
(full suite + focused suites + parity corpus + live binary probes):

| Requirement | Scenarios | Verified by |
|---|---|---|
| Durable binding witness | 3 | witness suite (14 tests), corpus rows 01/18–20, live dormant probe |
| Binding-only activation | 2 | corpus rows 02–04/19, 5×3×8 verdict matrix |
| Doctor-only dormancy visibility (D15) | 2 | `tests.test_doctor_tracker_card` (16 tests, includes the no-brief-line assertion), corpus rows 18–20 |
| One primary item per work identity | 4 | identity/store Go suites, corpus rows 10–15 |
| Human adjudication + branch reuse (D16/D17) | 3 | decide suite, corpus rows 06–13 |
| Five-checkpoint lifecycle verdicts | 3 | `test_all_five_hosts_block_on_the_same_verdict` + allow twin, corpus rows 21/22/24, work-start no-folder test |
| Modes always/ask/warn + D18/D19 | 4 | verdict matrix, A9 mode-mapping suite, corpus rows 04/09 |
| Go-authoritative grader, Python bridge only | 3 | one-grader greps, parity runner, `--selftest ok`, single four-arch trust root |
| Unevaluable/outage behavior | 3 | corpus rows 16/17/23, guardian cold-home test, corrupt-store Go tests |
| Scope boundaries (D1/D2/D11/D13/D14) | 3 | provider-neutral key-set test, single module (`go list -deps ./ledger` = stdlib + ledger), no archive edits in the diff |
| Synthetic fixture + parity corpus | 2 | 24 pinned fixtures, `test_corpus_covers_every_design_row`, hermetic no-network runs |

### Findings

CRITICAL — none.

WARNING

1. Tasks 7.6 (guardian run + archive move to `openspec/changes/archive/2026-09-13-tracker-ledger-foundation/` + guardian re-run) and 7.7 (ONE `gh` PR, base `development`, no merge, line counts recorded) are unchecked — parent-owned closeout prerequisites per the delivery decision, not production implementation. `pre-merge` blocking on the missing archive is the expected pre-closeout state, and `ready_for_archive` stays false until both run; archive is not ready from this report alone.
2. Guardian tier inference falls back to `standard` because `tasks.md` has no canonical `Depth: full` line (the parser wants `^Depth: (light|standard|full)$`; the current prose "Planning depth: Full …" does not match). If closeout ever runs the guardian with an explicit `--tier full`, this report would additionally need `ready_for_archive: true` and canonical `- Criterion N: PASS — …` mapping rows (the existing mapping uses table rows the strict parser does not read). Harness-mechanics note for closeout, not an implementation defect.
3. Status-engine discrepancy (carried from every apply unit): the native SDD status is computed against the main checkout, where this change folder does not exist, so it reports "No active SDD changes found." Non-authoritative for the worktree-resident change; all inputs were read from the worktree change folder on disk.

SUGGESTION

1. `tracker-card-gate.sh`'s header comment still lists `archive` among graded shell actions although archive-close moved to the guardian in unit 5. Cosmetic, already recorded by the apply phase; fix opportunistically during closeout.

### Strict TDD compliance

`strict_tdd: true` (`openspec/config.yaml`). `apply-progress.md` carries a TDD Cycle Evidence
table for every unit (1–7) with RED → GREEN → TRIANGULATE → REFACTOR rows and recorded mutation
checks proving the assertions are not vacuous. Every referenced test file exists in the tree
(spot-checked by this verification) and every suite is GREEN at HEAD under this verification's
own runs. Assertion quality — no tautological, type-only, or smoke-only assertions found in
the changed/created tests; the corpus pins exact decisions, reasons, and exit codes and carries
its own divergence-rejection test (`test_verdict_corpus_rejects_a_divergent_pin`).

### Review workload / PR boundary

`tasks.md` records the accepted `size:exception` explicitly (authorization, rationale,
conditions), `Chained PRs recommended: No`, `Chain strategy: size-exception`, `Decision needed
before apply: No`. Whole-change diff vs `development` at HEAD: 67 files changed, 10,244
insertions(+), 1,009 deletions(-) (the prior row below counted 9,796/1,001 across 61 files
before the Phase 7 commit landed). The branch carries exactly one commit per unit in unit
order — f1bde2a (1), bc54ecf (2), 6ddb631 (3), 6e3667b (4), 6311e88 (5), dd677b2 (6), f5de198
(7) — matching the recorded single-PR, ordered-slices strategy; no merge.

### Closeout prerequisites (parent-owned; not implementation defects)

1. **7.6** — re-run `python3 lib/_internal/premerge_guardian.py --root . --stage pre-archive tracker-ledger-foundation` (expected to pass after this report's commit-field fix at the inferred standard tier), archive the folder to `openspec/changes/archive/2026-09-13-tracker-ledger-foundation/` on the review branch, re-run the guardian after the move, then tick 7.6.
2. **7.7** — open ONE `gh` PR (base `development`, accepted `size:exception`) keeping the seven unit-order commits, record the PR URL + total and per-commit changed-line counts in the (archived) verify report, then tick 7.7. No merge.

## Results

- **Python unittest discovery**: `Ran 1947 tests in 718.846s` → `OK (skipped=2)`, 0 failures,
  0 errors (full `./tests/validate.sh`, exit 0).
- **Go**: `go test -count=1 ./...` → `ok ai-specs.dev/worktree-gate 15.090s`,
  `ok ai-specs.dev/worktree-gate/ledger 2.986s`; `go vet ./...` clean;
  `gofmt -l catalog/recipes/worktree-flow/gate` empty.
- **Syntax checks**: `python3 -m py_compile lib/_internal/*.py tests/*.py` and
  `bash -n lib/*.sh bin/ai-specs tests/*.sh` pass inside `validate.sh`.
- **Trust root**: rebuilt with the canonical `go1.24.13` toolchain (`go version go1.24.13
  darwin/arm64`), reproducible across two builds into different directories; the four new
  digests are committed to `catalog/recipes/worktree-flow/bin/SHA256SUMS`, and
  `scripts/verify-gate-sums.sh` reports `ok — 4 digest entries match the committed trust root`.
- **Focused ledger/host suites**: `python3 -m unittest tests.test_tracker_ledger_parity
  tests.test_doctor_tracker_card tests.test_tracker_ledger_witness tests.test_ledger_mode_config
  tests.test_tracker_card_gate_hook tests.test_plan_build_gate_hook tests.test_premerge_guardian
  tests.test_worktree_gate_parity tests.test_trello_link` → `Ran 173 tests ... OK`.
- **Pre-existing digest failures resolved, not hidden**: before 7.2, the three gate-asset
  modules failed with exactly 11 failures (`test_worktree_gate_release_phase4` ×9,
  `test_release_materialization` ×1, `test_worktree_root_propagation` ×1) because the committed
  `SHA256SUMS` predated the ledger source. The regeneration is the fix task 7.2 owns; after it,
  the same three modules are `Ran 22 tests ... OK`, and the full suite is green. No test was
  deleted, skipped, or weakened to reach green.

## Success-criteria mapping

One row per proposal `## Success Criteria`; the criterion text is condensed, the evidence is exact.

| Criterion | Verdict | Evidence |
|---|---|---|
| 1 — decisions come from a mode of the single existing Go product; no second module/asset/dependency; existing four-target `SHA256SUMS` remains the only verified acquisition path | **Criterion 1: PASS** | `catalog/recipes/worktree-flow/gate/ledger/` (identity, witness, store, verdict, decide) + `ledger_cmd.go` on the one module; `go.mod` unchanged; `go list -deps ./ledger` lists only stdlib plus `ai-spec…/ledger`; `SHA256SUMS` still exactly four `worktree-gate-*` entries; `scripts/verify-gate-sums.sh` ok |
| 2 — activation from a durable bound-`tracker` witness; declaration-only/ambiguous/unbound dormant **and** visible via `doctor` (D15); no brief dormancy line; never guesses/synthesizes a provider | **Criterion 2: PASS** | Witness writer in `lib/_internal/recipe-materialize.py` (`bound`/`ambiguous`/`unbound`/`declared-not-bound`) + `tests/test_tracker_ledger_witness.py` (14); `ReadBinding` dormancy `witness-missing` (8 unusable shapes); `_check_tracker_ledger` A10 rendering + D15 static assertion in `tests/test_doctor_tracker_card.py`; corpus rows 01, 18–20 |
| 3 — at most one primary item per work identity; detached HEAD / unborn / collision resolve to explicit human reconciliation; branch reuse after close opens a new item (D17) | **Criterion 3: PASS** | `ledger/identity_test.go` (A2 table, slug enrichment, archived slug, rename, two worktrees one identity), `ledger/store_test.go` (`TestClosedItemIsNeverSelectedOrReopened`, `TestReusedBranchOpensNewItemD17`, `TestTwoOpenItemsAreConflictNotPick`); corpus rows 10–15 |
| 4 — all five checkpoints reach the same Go predicate through their existing hosts and return the same verdict for one fixture; work-start precedes proposal/first write; no change folder required | **Criterion 4: PASS** | `tests/test_ledger_mode_config.py::test_all_five_hosts_block_on_the_same_verdict` + `test_all_five_hosts_allow_on_the_same_verdict` (one stub binary, five hosts, five distinct checkpoints logged once each); `tests/test_plan_build_gate_hook.py` (`work-start` before the artifact gate, no change folder); corpus rows 02–04, 21, 22; spec scenario "All five checkpoints reach one predicate" |
| 5 — modes match D10/D18/D19: `always` asks/links/blocks missing-or-conflicted at pre-merge; `ask` prompts per checkpoint with checkpoint-scoped opt-out; `warn` reports only; warn-first adoption; promotion is a human config change; no inferred decision | **Criterion 5: PASS** | `ledger/verdict_test.go` (5 checkpoints × 3 modes × 8 scenarios), `TestGradeAskPromptAndExitZero`, `TestGradeWarnNeverBlocksAnyCheckpoint`, `TestGradeCheckpointScopedOptOutD19`; `tests/test_ledger_mode_config.py` (A9 mapping incl. warn default + legacy `gate_mode` forward-map + worktree mode never read); recipe `[config.ledger_mode]` enum/default |
| 6 — four-side evidence reconciliation; every disagreement shown to a human with the evidence; decision persisted in the ledger (D16); no MCP/API create or update in this slice | **Criterion 6: PASS** | `ledger/decide_test.go` (`TestEvidenceConflictFourSidesNoDefaultWinner`, `TestPersistDecisionThenRegradeAllows`, `TestPersistDecisionClearsCurrentConflictSnapshot`, fail-closed paths); corpus rows 05–08; `lib/_internal/doctor.py`/hooks perform no provider call (no MCP tool touched anywhere in the diff) |
| 7 — `## Tracker` validity has one authoritative grader; each legacy copy delegates to Go or is held by a parity test that fails on divergence | **Criterion 7: PASS** | `_check_tracker_card_link`/`_load_trello_link` deleted from `doctor.py` (16 tests render the Go `doctor` finding); `trello_link.py` parser-only comments, no new predicate; heredoc grader deleted from `tracker-card-gate.sh`; `tests/test_tracker_ledger_parity.py` drives the built binary over 24 pinned fixtures and fails on divergence (`test_verdict_corpus_rejects_a_divergent_pin`, `test_every_verdict_case_matches_its_pin`) |
| 8 — synthetic tracker provider recipe under `tests/fixtures/recipes/` + pinned `(identity + evidence) → (verdict, conflict)` corpus, no MCP/network/live board | **Criterion 8: PASS** | `tests/fixtures/recipes/test-tracker-ledger/` + `...-conflict/` (gated by `AI_SPECS_ALLOW_INTERNAL_TEST_RECIPES`); 24 fixtures in `tests/fixtures/tracker-ledger-corpus/`; `test_tracker_ledger_parity.py` covers every design test-table row (`test_corpus_covers_every_design_row`) with no network |
| 9 — every architecture question answered or explicitly deferred; D1–D19 neither widened nor reopened | **Criterion 9: PASS** | `design.md` A1–A11 answers A1–A11 and carries an explicit `### Deferred (do not implement)` list; `proposal.md` D1–D19 unchanged; the spec restates the same closed decisions without adding capability scope |
| 10 — build/format/test/digest/release plumbing updated in the same change; `./tests/validate.sh` passes; Go vet/test runs in CI for the ledger surface; `verify-gate-sums.sh` green | **Criterion 10: PASS** | `tests/run.sh` already runs `go -C … test ./...` (now including `ledger`); `tests/validate.sh` exit 0; `go vet ./...` clean; `SHA256SUMS` regenerated with canonical `go1.24.13`; `scripts/verify-gate-sums.sh` ok |
| 11 — existing behavior preserved: worktree-gate verdicts, exit-code contract, fail-open blast radius, `openspec/**` non-blocking, warn dogfood posture, archived changes | **Criterion 11: PASS** | `tests/test_worktree_gate_parity.py` 8/8 unchanged; old-vs-new `--explain`/`--tokenize` byte-identical (Unit 3 evidence); `TestWorktreeExplainUnchangedWithoutLedger` + `TestLedgerFlagParseFailsOpen`; corpus row 24 `openspec-path-never-blocked`; repo dogfood `ledger_mode` default `warn`; no archive migrated |

All 11 criteria are **PASS**.

## RED → GREEN → TRIANGULATE → REFACTOR evidence per unit

Strict TDD is active (`openspec/config.yaml` `strict_tdd: true`, runner `unittest`).

| Unit | RED | GREEN | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| 1 — identity + witness (Go) | `go test ./ledger/...` → build failed (`undefined: DeriveIdentity`, `IdentityOptions`, `ReasonIdentityUnavailable`, `ReadBinding`, …) | identity + witness suites pass (39 cases) | archive-aware slug order with a mutation check (`dated[0]` vs `dated[-1]`); Python `tests/_change_paths.py::change_dir` parity over 6 trees; stored-slug-after-archive; rename = new identity | `realPath` resolves a missing `.git` through its parent; `dormant()` helper; `TestResolveChangeDirMatchesPythonHelper` skips without `python3` |
| 2 — atomic store + item rules (Go) | same RED build run (`undefined: ItemIdentity`, `StorePath`, `LoadStore`, `StoreVersion`, `StoreCorruptError`, …) | 8 store cases pass (missing = empty, corrupt = unevaluable, atomic rename, identity key) | `-race -count=3` concurrency (no lost decision, no `state.json.tmp.*` residue); D17 reuse; provider-neutral key-set guard; both ceiling branches; 3 mutations produced exactly 3 failures | `writeAtomic`, `withStoreLock`, typed `StoreCorruptError`, shared `decisionKinds` |
| 3 — verdict + `--ledger` CLI (Go) | `undefined: Evidence, Verdict, Checkpoints, Grade, DecisionRequest, PersistDecision`; CLI: `--ledger` unknown / no JSON | 5×3×8 posture matrix + 15 CLI contract tests pass; `go build` ok | old-vs-new `--explain` byte-identical for 4 inputs and `--tokenize` identical; worktree parity 8/8; 2 mutations (conflict-side comparison; `ModeAsk`→block) failed the exact tests | `rfc3339Stamp` dedupe; `ledgerSelftest()` isolated; `go list -deps ./ledger` shows no gate types |
| 4 — durable witness (Python bridge) | `unittest tests.test_tracker_ledger_witness` → `AttributeError: … has no attribute 'git_common_dir'` (9 errors, 2 failures) | 14 witness tests pass (four states, trap survival, atomic write, no guess, non-repo) | linked-worktree case: witness written from main checkout is read back via the linked `.git` file, binary returns `active=true`; disabling the bound recipe overwrites to `unbound`; ambiguous candidates carry both ids with `recipe_id == ""` | `tracker_witness_payload` split from `write_tracker_witness`; shared `LEDGER_*`/`WITNESS_*` constants; mutation (`ambiguous`→`unbound`) failed only the ambiguous test |
| 5 — checkpoint hosts | `unittest tests.test_ledger_mode_config tests.test_tracker_card_gate_hook tests.test_plan_build_gate_hook tests.test_premerge_guardian` → 15 failures (`ledger_mode` absent; hosts never spawned the binary, `_logged_checkpoints() == []`) | 115 focused tests pass (A9 mapping, heredoc grader removed, ask + `--decide`, guardian both stages, tier math unchanged) | one stub binary across five hosts logs `apply-start, archive-close, pr-review, pre-merge, work-start` once each and blocks consistently; `test_hosts_add_no_tracker_predicate` grep-asserts no new predicate | ledger merged into guardian `main`; 3 mutations (checkpoint swap, empty blockers, work-start after artifact gate) each failed the matching test |
| 6 — one grader + parity corpus | `unittest tests.test_doctor_tracker_card` → 14 errors (`Doctor` has no `_check_tracker_ledger`) + 1 failure (legacy grader still present); parity runner initially `FAILED (failures=32)` against mis-pinned rows | 16 doctor tests pass; 24-case corpus; `test_tracker_ledger_parity` `Ran 6 tests ... OK` | two fresh runs byte-identical (RFC3339 stamps masked); residue check allows only `witness.json`/`state.json`/`state.json.lock`; 3 mutation checks; worktree parity 8/8 | deleted the legacy grader + loader; `_tracker_ledger_in_play`/`_tracker_ledger_binary`/`_tracker_ledger_guidance` extracted |
| 7 — docs + trust + close-out | `unittest tests.test_worktree_gate_release_phase4 tests.test_release_materialization tests.test_worktree_root_propagation` → `FAILED (failures=11)` against the stale committed digests (real behaviour failure, not a missing symbol) | `Ran 22 tests ... OK` after the canonical rebuild; `verify-gate-sums.sh` ok; full `./tests/validate.sh` exit 0, `Ran 1947 tests ... OK (skipped=2)` | second build into `/tmp/dist2` produced byte-identical digests (reproducibility); doc claims cross-checked against the shipped hooks/doctor/recipe | `SHA256SUMS` header updated to name the ledger regeneration; docs kept provider-neutral outside the tracker recipe README |

## Judgment Day input set

Authorized rounds: up to 3. This executor recorded the input set and re-ran the verifying
commands. **No fix round was triggered by this phase** — the Phase 7 slice is documentation,
trust-root regeneration, and close-out evidence, and every mapped test/scenario was already
green (full suite `Ran 1947 tests ... OK`). If a reviewer requests a round, the mapping below is
the exact input set: spec scenario → corpus fixture → test (or host/Go test where a scenario has
no single corpus row).

| # | Spec scenario | Corpus fixture | Test / command |
|---|---|---|---|
| 1 | Witness survives sync | — (witness producer) | `tests/test_tracker_ledger_witness.py::test_real_sync_leaves_witness_after_trap`, `test_witness_survives_resolved_config_temp_deletion` |
| 2 | Ambiguous binding recorded as ambiguous | `18-ambiguous-witness-warn.json` | `test_tracker_ledger_parity.py::test_every_verdict_case_matches_its_pin`; `test_tracker_ledger_witness.py::test_ambiguous_witness_records_candidates_without_guessing` |
| 3 | Missing witness is dormant, never bound | `01-dormant-no-witness.json` | `ledger/witness_test.go::TestReadBindingMissingIsDormant`; `ledger_cmd_test.go::TestLedgerDormantWhenWitnessMissing` |
| 4 | Bound witness activates the ledger | `02-empty-store-warn-work-start.json` | `ledger/verdict_test.go::TestGradePostureMatrixFiveCheckpointsThreeModes`; parity `test_every_verdict_case_matches_its_pin` |
| 5 | Declared but not bound stays inactive | `19-declared-not-bound-warn.json` | parity `test_every_verdict_case_matches_its_pin`; `ledger/verdict_test.go::TestGradeAmbiguousAndDeclaredCarryTheirOwnDoctorSeverity` |
| 6 | Dormant project renders in doctor only | `18-ambiguous-witness-warn.json` | `tests/test_doctor_tracker_card.py` (A10 WARN + D15 no-brief-line assertion) |
| 7 | Bound and healthy renders OK | `05-consistent-evidence-allow.json` | `tests/test_doctor_tracker_card.py::test_bound_healthy_renders_ok` |
| 8 | Identity derived from common dir and branch | — | `ledger/identity_test.go::TestDeriveIdentityA2` |
| 9 | Detached HEAD is unavailable, not guessed | `14-detached-head-always-pre-merge.json`, `15-detached-head-warn.json` | `ledger_cmd_test.go::TestLedgerIdentityUnavailableModes`; `ledger/verdict_test.go::TestGradeAlwaysBlocksPreMergeIdentityUnavailable` |
| 10 | Several active changes omit the slug | — | `ledger/identity_test.go::TestActiveChangeSlugs` (0/1/several) |
| 11 | Archived change keeps its slug | — | `ledger/identity_test.go::TestDeriveIdentityKeepsStoredSlugWhenArchived`; `TestResolveChangeDirArchiveOrder` |
| 12 | Disagreement presents evidence and persists the human choice | `06-four-side-conflict-ask.json`, `08-persisted-decide-then-regrade.json` | `ledger/decide_test.go::TestEvidenceConflictFourSidesNoDefaultWinner`, `TestPersistDecisionThenRegradeAllows` |
| 13 | Branch reuse after closed work opens a new item | `10-closed-item-not-reopened.json`, `11-closed-plus-new-open-allows.json` | `ledger/store_test.go::TestReusedBranchOpensNewItemD17`, `TestClosedItemIsNeverSelectedOrReopened` |
| 14 | Two open items are a conflict, not a pick | `12-two-open-items-conflict-warn.json`, `13-two-open-items-conflict-always.json` | `ledger/store_test.go::TestTwoOpenItemsAreConflictNotPick`; `ledger_cmd_test.go::TestLedgerTwoOpenItemsConflict` |
| 15 | All five checkpoints reach one predicate | rows 02–04, 21, 22 | `tests/test_ledger_mode_config.py::test_all_five_hosts_block_on_the_same_verdict`, `test_all_five_hosts_allow_on_the_same_verdict` |
| 16 | Work-start without a change folder | — | `tests/test_plan_build_gate_hook.py::test_work_start_grades_without_a_change_folder` |
| 17 | openspec writes are never blocked | `24-openspec-path-never-blocked.json` | parity `test_bridge_rows_fail_open_and_never_block_openspec` |
| 18 | Always blocks missing state at pre-merge | `04-empty-store-always-apply-start.json` (apply), pre-merge via verdict matrix | `ledger/verdict_test.go::TestGradePostureMatrixFiveCheckpointsThreeModes`; `tests/test_premerge_guardian.py::test_premerge_grades_pre_merge` |
| 19 | Ask opt-out is checkpoint-scoped | `09-opt-out-checkpoint-scoped.json` | `ledger/verdict_test.go::TestGradeCheckpointScopedOptOutD19`; `ledger/decide_test.go::TestPersistOptOutIsCheckpointScopedD19` |
| 20 | Warn never blocks | `02-empty-store-warn-work-start.json` | `ledger/verdict_test.go::TestGradeWarnNeverBlocksAnyCheckpoint`; `ledger_cmd_test.go::TestLedgerWarnNeverBlocksViaCLI` |
| 21 | Warn-first adoption | — | `tests/test_ledger_mode_config.py` (A9 mapping: unset + `gate_mode=warn` → `warn`) |
| 22 | One grader, parity at legacy seams | all verdict rows | `tests/test_tracker_ledger_parity.py::test_verdict_corpus_rejects_a_divergent_pin`; `tests/test_trello_link.py` (13 parser tests) |
| 23 | Single verified binary serves both modes | — | `scripts/build-gate.sh` + `scripts/verify-gate-sums.sh` (4 entries, one trust root); `go build` of one module |
| 24 | Selftest covers ledger invariants | — | `dist/worktree-gate-current --selftest` prints `ok`; `ledger_cmd_test.go` covers `ledgerSelftest` invariants |
| 25 | Cold cache at pre-merge in always mode | `23-missing-binary-fails-open.json` | manifest-binary fail-open test in `tests/test_tracker_ledger_parity.py`; guardian cold-home test |
| 26 | Corrupt store never synthesizes | `16-store-corrupt-warn.json`, `17-store-corrupt-always.json` | `ledger/store_test.go::TestLoadStoreCorruptIsUnevaluable`; `ledger/verdict_test.go::TestGradeDormantBeatsCorruptStore` |
| 27 | Older binary without ledger support | — | `tests/test_ledger_mode_config.py` host fail-open assertions (missing/unverified binary → exit 0) |
| 28 | No generic capability ledger | — | `go list -deps ./ledger` (stdlib + ledger only); design `### Deferred`; `Capabilities` unchanged |
| 29 | Provider vocabulary stays out of core | — | `ledger/store_test.go::TestItemCoreFieldsAreProviderNeutral` (neutral key set + opaque round-trip) |
| 30 | History is untouched | — | `git diff` shows no `openspec/changes/archive/**` edit; design D13 |
| 31 | Corpus pins verdict behavior | all 24 fixtures | `tests/test_tracker_ledger_parity.py` (`Ran 6 tests ... OK`, every row asserted) |
| 32 | Hermetic fixture provider | — | `tests/fixtures/recipes/test-tracker-ledger/`; witness tests run with no MCP/network |

### Rounds run

- **Round 1 input set**: recorded above; verifying re-run `./tests/validate.sh` → exit 0,
  `Ran 1947 tests ... OK (skipped=2)`; focused ledger/host suites `Ran 173 tests ... OK`.
- **Fixes applied**: none (no divergence found by this phase's own evidence). No round 2/3 fix
  re-run exists because none was needed; a reviewer-triggered round would reuse the table above.

## Scope and deviations carried into this report

- The native SDD status engine was computed against the main checkout, where the change folder
  exists only on this branch inside the worktree; it reported the `openspec` artifacts `missing`
  with `applyState: blocked`. Per the artifact-store rules, the authoritative inputs were read
  from `.worktrees/tracker-ledger-foundation/openspec/changes/tracker-ledger-foundation/` on
  disk. No real blocker existed; the forecast gate was already resolved
  (`Decision needed before apply: No`, `Chained PRs recommended: No`, accepted
  `size:exception`).
- `tracker-card-gate.sh` still carries a stale header comment saying shell actions are
  `{pr_create, archive}` while only `pr_create` is produced (archive-close moved to the
  guardian). It is a comment outside this phase's allowed edit surface; recorded here rather
  than silently edited.
- Review workload: this phase's slice is 163 added / 12 deleted lines across 7 files
  (`git diff --stat`), plus the regenerated trust root. Whole-change total is 9,796 insertions /
  1,001 deletions across 61 files versus `development` — far over the 400-line budget, delivered
  under the maintainer-accepted `size:exception` recorded in `tasks.md`. Nothing was compressed
  or restyled to fit.

## Remaining close-out tasks (parent-owned)

7.6 and 7.7 remain unchecked in `openspec/changes/tracker-ledger-foundation/tasks.md`:

- **7.6** — `python3 lib/_internal/premerge_guardian.py --root . --stage pre-archive` then
  `--stage pre-merge` must pass; archive this folder to
  `openspec/changes/archive/2026-09-13-tracker-ledger-foundation/` on the review branch and
  re-run the guardian after the move. Exact blocker: it is an ordered after-7.5 step whose
  archive move changes the folder path this report lives at, so this executor left it to the
  parent (running it here would move the artifact before the parent's PR step).
- **7.7** — open ONE `gh` PR (base `development`, no merge) with the 7 work units as separate
  commits in unit order, and record the PR URL + total changed-line count + per-commit line
  counts. Exact blocker: this executor is explicitly forbidden to commit, push, or open a PR;
  the parent owns the single PR.
