```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:eeeaa16d36bd340deb1db2e405589ae2f1ecacd91f24992a55fef0f89a726eed
verdict: pass
blockers: 0
critical_findings: 0
requirements: 13/13
scenarios: 39/39
test_command: ./tests/validate.sh
test_exit_code: 0
test_output_hash: sha256:b9f6a48069104a81d584491880de1baeaa353e9fe24bde74a0e30c66a42497a8
build_command: ./scripts/build-gate.sh
build_exit_code: 0
build_output_hash: sha256:e4a6af9859ae9933b772bb0e27514e592dfba9de179234571a15932ad9720346
```

## Verify evidence

- Verdict: PASS
- Command: `./tests/validate.sh`
- Exit: 0
- Date: 2026-09-14
- Commit: 1f13c80
- ready_for_archive: true

## Success-criteria mapping

- Criterion 1: PASS — explicit ledger writer
- Criterion 2: PASS — production lifecycle fields
- Criterion 3: PASS — production conflict evidence
- Criterion 4: PASS — persistent exemption behavior
- Criterion 5: PASS — pure grading
- Criterion 6: PASS — failure postures
- Criterion 7: PASS — preserved semantics
- Criterion 8: PASS — provider lookup coverage
- Criterion 9: PASS — validation and trust root
- Criterion 10: PASS — honest documentation

# Verify Report: trello-ledger-integration

Verdict: **PASS**. The implementation satisfies the approved delta spec (13/13 requirements,
39/39 scenarios), all 24 task checkboxes are complete, strict-TDD evidence is present and
independently re-confirmed GREEN, assertion quality is sound, the changed-file scope stays inside
the allowed edit root, the trust root matches the rebuilt four-target binary, and the explicit
one-PR `size:exception` decision is recorded and honored.

This is the retry after attempt ordinal 3 (`sdd-verify`, evidence revision
`sha256:1f67890271841fc6533f882b53cecc318f129767db2f68baa2c5e3e3779cefd2`) failed with an opaque
assistant error before producing any report. That failed evidence revision is named as remediated by
this passing settlement, which uses fresh, distinct verification evidence (digest above) produced by
commands re-run in this session.

## Structured status and actionContext

Native `gentle-ai.sdd-status` (v2), read fresh: change `trello-ledger-integration`,
`artifactStore: openspec`, `nextRecommended: verify`, `verify: ready`, `applyState: all_done`,
`taskProgress 24/24 allComplete`, `blockedReasons: []`. `actionContext.mode: repo-local`,
`workspaceRoot` and `allowedEditRoots` limited to
`/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/trello-ledger-integration`. Verified: the
worktree is on branch `change/trello-ledger-integration` and every changed file in `git status` is
inside that root. No `actionContext` warning fired. Runtime attempt ledger: acquire re-entered the
active ordinal-4 attempt with state `proceed` (token
`sha256:6f5561c178f7a57b6dca34e2d970e2033199a235354393d08d9bd249489c5b30`), with the settle
obligation to pass `--remediates-evidence-revision sha256:1f67890271841fc6533f882b53cecc318f129767db2f68baa2c5e3e3779cefd2`
— honored in this settlement.

## Test and validation commands (exact, with exit codes)

| Command | Exit | Result |
|---|---|---|
| `go -C catalog/recipes/worktree-flow/gate test ./ledger/ -race -count=3` | 0 | `ok ai-specs.dev/worktree-gate/ledger 8.674s` |
| `go -C catalog/recipes/worktree-flow/gate test ./... -count=1` | 0 | `ok` for both packages |
| `./scripts/build-gate.sh` | 0 | 4/4 targets built (darwin/arm64+amd64, linux/amd64+arm64) |
| `./scripts/verify-gate-sums.sh /tmp/verify_generated_sums.txt catalog/recipes/worktree-flow/bin/SHA256SUMS` | 0 | `ok — 4 digest entries match the committed trust root` |
| `./dist/worktree-gate-current --selftest` | 0 | `ok` (offline, includes write-surface invariants) |
| `./tests/run.sh` | 0 | `Ran 1989 tests in 696.789s` / `OK (skipped=2)` |
| `./tests/validate.sh` | 0 | gofmt clean, Go tests `ok`, `Ran 1989 tests in 630.949s` / `OK (skipped=2)` |

Operator note recorded for honesty: one intermediate `verify-gate-sums.sh` invocation exited 1
because I generated the sums file from the repo root (paths prefixed `dist/`, which the script's
canonicalizer does not match). Regenerating from inside `dist/` as the committed header prescribes
produced the green result above; the trust root itself was never in question.

Unavailable quality signals, stated not implied: coverage, linter, type-checker, and formatter
tooling do not exist in this repo (`openspec/config.yaml`: all `available: false`). Only `gofmt`
(via `validate.sh`) and `py_compile`/`bash -n` syntax checks ran besides the test suites.

## Spec coverage (13/13 requirements, 39/39 scenarios)

- **Machine write surface** (4 scenarios) — `ledger/store.go` (`WriteRequest`, `ApplyWrite`),
  `ledger_cmd.go`, `main.go`; `go.mod`/`go.sum` unchanged (empty diff confirmed); same four-target
  trust root, verified green. All four kinds plus every core item field have production writers.
- **Explicit item opening** (2) — `TestLedgerGradeNeverWritesStore` (byte + mtime identical, store
  never created by a grade); hosts never auto-open from a parse (argv assertions).
- **Idempotent, collision-safe writes under a bounded lock** (6) — open-if-absent, same-second
  `uniqueItemID`, link idempotency on canonical provider bytes, explicit `already-closed`,
  `change-ambiguous` refusal, 5×`LOCK_NB`/20 ms bounded lock with grade-fail-open /
  write-fail-closed (`TestApplyWriteHeldLockTimesOutFailsClosed` with timing bounds).
- **Local/code/git evidence, remote deferred** (3) — `lib/_internal/ledger_bridge.py` +
  `tests/test_ledger_evidence.py`; hosts pass `--evidence` at apply-start/pr-review/pre-merge/
  archive-close; malformed artifacts fail open; source-level offline assertions.
- **Persistent tracker.none exemption** (2) — `Item.Exemption` honored at all five checkpoints via
  CLI test; exempt does not conflict; revoke only by explicit adjudicate (`TestAdjudicateClearsExemption`).
- **Witness-derived provider lookup** (2) — `bridge.recipe_id` at the three in-scope sites; the
  in-scope literal lookup is gone (grep verified: only a doc comment remains in the hook); legacy
  fallback tested for missing/corrupt/unknown-version/no-git.
- **Checkpoint ownership stays put** (1) — `plan-build-gate.sh` and all of
  `catalog/recipes/plan-build-flow/**` byte-untouched (empty diff); doctor INFO reports unhosted
  `work-start`; the plan-build-flow literal residue at `plan-build-gate.sh:104` is named in
  `docs/capabilities.md` and pinned by `test_the_out_of_scope_residue_is_named_in_the_docs`.
- **Write-path failure postures fail closed** (1) — byte-identical store + exit 2 across invalid
  payload, ambiguous identity, lock timeout, IO error (`TestApplyWriteFailuresLeaveStoreByteIdentical`,
  corpus 33); flag-parse on verdict calls still fails open.
- **Human adjudication / branch reuse (MODIFIED)** (4) — corpus rows 06/07/10/11/12/13; grade never
  reopens; new primary only by explicit open.
- **Modes always/ask/warn (MODIFIED)** (5) — corpus 02–04, 09, 21, 22; no-TTY `ask` stays exit 2;
  warn-first adoption unchanged.
- **Go-authoritative grader (MODIFIED)** (3) — parity suites at legacy seams; single binary + single
  trust root (`verify-gate-sums.sh` green); `--selftest` exercises ledger + write invariants offline.
- **Unevaluable/outage behavior (MODIFIED)** (4) — corpus 16/17/23/24; corrupt store → `unevaluable`
  + doctor ERROR; older-binary degradation preserved.
- **Synthetic fixture + parity corpus (MODIFIED)** (2) — corpus is hermetic; `DESIGN_ROWS` (33 keys)
  maps 1:1 to fixtures 01–24 and 26–34; the globbing corpus runner additionally drives fixture
  `25-empty-store-ask-opt-out.json`; `test_corpus_covers_every_design_row` green in the suite run.

## Task completion

`tasks.md` scan: **24/24 checkboxes `- [x]`; zero unchecked `- [ ]` implementation task lines**
(`grep '^\s*- \[ \]'` → none). No archive blocker. Every task's evidence in `apply-progress.md`
cross-references files that exist in the codebase (spot-verified: `write_test.go` 659 lines,
`ledger_bridge.py` 221, `test_ledger_evidence.py` 223, fixtures 26–34, `DESIGN_ROWS`, the two
residue-pinning tests in `tests/test_ledger_mode_config.py`).

## Strict TDD compliance

`apply-progress.md` contains a `TDD Cycle Evidence` table covering nine cycles (1.1–1.2, 1.3–1.5,
1.6–1.7, 2.1–2.2, 2.3–2.4, 2.5, 2.6–2.7, 3.1–3.4, 3.5) with observed RED failures (build failures on
undefined symbols, missing module, `flag provided but not defined: -write`, 7 failing corpus rows)
and GREEN passes, plus TRIANGULATE and REFACTOR notes. Cross-referenced against the codebase: all
named test files and fixtures exist and pass in this session's re-runs. RED→GREEN→TRIANGULATE→REFACTOR
evidence: complete; nothing downgraded.

## Assertion quality

Audited `write_test.go` (all 659 lines), `test_ledger_evidence.py` (all 223 lines), and the
parity/CLI test structure: byte-identical store comparisons before/after failure paths, exact
outcome-field assertions (`applied`/`reason`), explicit negative cases, timing bounds on the lock
timeout, `-race` concurrency with duplicate-id and residue checks, source-level offline/acquisition
assertions, and malformed-input fail-open cases. No tautologies, no ghost loops, no type-only or
smoke-only assertions, no implementation-detail styling assertions (N/A domain). Corpus rows assert
verdict, conflict, exit code, and store pins together.

## Review workload / PR boundary

`tasks.md` forecast: estimated 1,500–2,200 authored lines, 400-line risk High, "Chained PRs
recommended: Yes", "Chain strategy: size-exception", delivery `exception-ok` explicitly accepted by
the maintainer. Verified: `size:exception` is explicitly recorded (tasks.md forecast + session
preflight; never inferred); the returned boundary is a single PR matching the `size-exception`
chain strategy; the native ledger records 3,773 changed lines for the apply attempt with
`changed_line_budget_exceeded: true` accepted via the maintainer reset. No scope creep: no
`plan-build-flow` content change, `go.mod` untouched, non-goals (provider writes, `remote` producer,
second binary/module/trust root, `unexempt` kind) all held. The seven documented deviations in
apply-progress are deliberate, documented, and none widen scope.

## Blockers

None. Archive is unblocked from the verification side: verify-report resolves at
`openspec/changes/trello-ledger-integration/verify-report.md` and every task is complete.
