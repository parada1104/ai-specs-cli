# Apply Progress: trello-ledger-integration

Phase: apply · Mode: strict TDD (RED → GREEN → TRIANGULATE → REFACTOR) · Run: single PR under
maintainer-accepted `size:exception` (`exception-ok`, 1200-line session budget).
Tasks: **24/24 complete**. Persisted checkbox updates: every `- [ ]` for 1.1–4.3 is now `- [x]` in
`openspec/changes/trello-ledger-integration/tasks.md`.

## Structured status consumed

Native `gentle-ai.sdd-status` (v2) for `trello-ledger-integration`, read fresh in this worktree:
`nextRecommended: apply`, `applyState: ready`, `artifactStore: openspec`, proposal/specs/design/tasks
`done`, `blockedReasons: []`, `actionContext.mode: repo-local`, `allowedEditRoots:
["…/.worktrees/trello-ledger-integration"]`. **No `actionContext` warning fired**: every edited path is
inside that single root, on branch `change/trello-ledger-integration`
(`git rev-parse --show-toplevel` confirmed the worktree before writing).

Review Workload Gate read from `tasks.md`: `Decision needed before apply: No`,
`Chained PRs recommended: Yes`, `Chain strategy: size-exception`, `400-line budget risk: High`. The
maintainer explicitly accepted `exception-ok` / `size:exception` for one PR before apply, so the
approved scope was implemented whole: no task was split, and **no test, doc, comment, or blank line was
removed or compressed to fit a budget**.

## Completed tasks

| Task | Persisted checkbox | Evidence |
|---|---|---|
| 1.1 RED write_test.go | `- [x] 1.1` | 15 new Go cases, RED = build failure (undefined `WriteRequest`/`ApplyWrite`/`WriteOutcome`/`WriteKinds`) |
| 1.2 GREEN store.go | `- [x] 1.2` | bounded `withStoreLock`, `OpenIfAbsent`, `uniqueItemID`, `ApplyWrite` lifecycle |
| 1.3 GREEN CLI vehicle | `- [x] 1.3` | `--ledger --write`, exclusivity, `write` sidecar, stderr+exit 2 with no JSON |
| 1.4 GREEN decide.go | `- [x] 1.4` | `adjudicate` clears `Item.Exemption`; persist paths inherit the bounded lock |
| 1.5 TRIANGULATE | `- [x] 1.5` | flag-parse vs post-parse posture, byte-identical+mtime store on failure, cwd vs `--project-root`, residue, `-race -count=3` |
| 1.6 REFACTOR | `- [x] 1.6` | grade-purity test, reused `newLedgerOut` marshal/exit path, dropped speculative fields, `gofmt -l` clean |
| 1.7 GREEN ledgerSelftest | `- [x] 1.7` | `ledgerWriteSelftest()` in-process/offline; `--selftest` still prints `ok` |
| 1.8 trust root | `- [x] 1.8` | `scripts/build-gate.sh` (4 targets) + `scripts/verify-gate-sums.sh` green; `SHA256SUMS` + `dist/worktree-gate-current` regenerated in the same unit; `go.mod` unchanged, no `go.sum` |
| 2.1 RED bridge tests | `- [x] 2.1` | `tests/test_ledger_evidence.py`, RED = module absent |
| 2.2 GREEN ledger_bridge.py | `- [x] 2.2` | 14 bridge tests green |
| 2.3 RED hook wiring | `- [x] 2.3` | `test_tracker_card_gate_hook.py` RED: no `--evidence`, no exempt write |
| 2.4 GREEN hook + stamp | `- [x] 2.4` | `__TRACKER_LIB_INTERNAL__` stamp + substitution; stamp test added |
| 2.5 RED/GREEN guardian | `- [x] 2.5` | `--evidence` + exempt write + write-exit-2 + cold home + pr-review rows |
| 2.6/2.7 corpus rows | `- [x] 2.6`, `- [x] 2.7` | fixtures 26–34 + `DESIGN_ROWS` 1:1; 7 rows RED before `--write` step support, then green; `remote` absent in the conflict row; residue set unchanged; two-grade byte-stable row |
| 2.8 REFACTOR | `- [x] 2.8` | `grep -n "card_id"` shows only the parser and the bridge's read of the parsed field; no host re-parses |
| 3.1 RED mode lookup | `- [x] 3.1` | tracker gate + guardian witness-recipe mode tests; doctor recipe-id/unhosted tests |
| 3.2 GREEN 3 sites | `- [x] 3.2` | `premerge_guardian.py`, `tracker-card-gate.sh`, `doctor.py` resolve the witness recipe id |
| 3.3 GREEN doctor INFO | `- [x] 3.3` | `work-start is unhosted` INFO when the witness is bound and `plan-build-flow` is not enabled; doctor still writes nothing |
| 3.4 TRIANGULATE residue | `- [x] 3.4` | in-scope-site literal-lookup `grep` assertion + plan-build-flow residue named in `docs/capabilities.md` |
| 3.5 docs | `- [x] 3.5` | `docs/capabilities.md`, `docs/runtime-hooks.md`, recipe README, recipe SKILL `## Tracker`, `CHANGELOG.md`; stale `tracker-card-gate.sh` header (`archive` is not a shell kind) fixed in the same unit |
| 4.1 smoke | `- [x] 4.1` | `./tests/validate.sh` exit 0 |
| 4.2 spec-to-code walk | `- [x] 4.2` | table below |
| 4.3 review-size check | `- [x] 4.3` | honest count below; `size:exception` recorded, nothing shrunk |

## Files changed

Go + trust root (Phase 1)
- `catalog/recipes/worktree-flow/gate/ledger/store.go` (bounded lock, `OpenIfAbsent`, `uniqueItemID`, `WriteRequest`, `ApplyWrite`, `canonicalProvider`)
- `catalog/recipes/worktree-flow/gate/ledger/decide.go` (`clearExemption`)
- `catalog/recipes/worktree-flow/gate/ledger_cmd.go` (`--write` parse/apply, `write` sidecar, `ledgerIdentityWithStoredSlug`, `ledgerWriteSelftest`)
- `catalog/recipes/worktree-flow/gate/main.go` (`--write` flag)
- `catalog/recipes/worktree-flow/gate/ledger/write_test.go` (new)
- `catalog/recipes/worktree-flow/gate/ledger_cmd_test.go` (CLI write + purity + selftest cases)
- `catalog/recipes/worktree-flow/bin/SHA256SUMS`, `dist/worktree-gate-current` (regenerated trust root)

Python + hosts + corpus (Phase 2)
- `lib/_internal/ledger_bridge.py` (new)
- `lib/_internal/recipe-materialize.py` (`__TRACKER_LIB_INTERNAL__` stamp)
- `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` (bridge resolution, exempt write, `--evidence`)
- `lib/_internal/premerge_guardian.py` (sibling bridge, evidence args, exempt write, `slug` argument)
- `tests/test_ledger_evidence.py` (new), `tests/test_tracker_card_gate_hook.py`, `tests/test_premerge_guardian.py`, `tests/test_tracker_ledger_parity.py`
- `tests/fixtures/tracker-ledger-corpus/26-…34-…` (9 new rows)

Lookup + docs (Phase 3)
- `lib/_internal/doctor.py` (`_tracker_recipe_id`, `_tracker_recipe_enabled`, `_tracker_witness_state`, unhosted `work-start` INFO)
- `tests/test_ledger_mode_config.py`, `tests/test_doctor_tracker_card.py`
- `docs/capabilities.md`, `docs/runtime-hooks.md`, `catalog/recipes/trello-mcp-workflow/README.md`, `catalog/recipes/trello-mcp-workflow/skills/trello-mcp-workflow/SKILL.md`, `CHANGELOG.md`

Untouched as required: `catalog/recipes/plan-build-flow/**`, `go.mod`, the `## Tracker` schema,
`openspec/**` blocking behavior, the exit-code contract, this project's `warn` posture.

## Test commands run

| Command | Result |
|---|---|
| `go -C catalog/recipes/worktree-flow/gate test ./ledger/ -count=1` | RED then GREEN (`ok ai-specs.dev/worktree-gate/ledger`) |
| `go -C catalog/recipes/worktree-flow/gate test ./... -count=1` | `ok` for both packages |
| `go -C catalog/recipes/worktree-flow/gate test ./ledger/ -race -count=3` | `ok` |
| `gofmt -l catalog/recipes/worktree-flow/gate` | empty (clean) |
| `python3 -m unittest tests.test_ledger_evidence -v` | RED (missing module) → 14 tests OK |
| `python3 -m unittest tests.test_tracker_card_gate_hook` | RED (3 failures) → OK |
| `python3 -m unittest tests.test_premerge_guardian` | RED (4 failures, 1 error) → 55 tests OK |
| `python3 -m unittest tests.test_tracker_ledger_parity` (×4 runs) | RED (7 rows) → 6 tests OK, repeated for flake resistance |
| `python3 -m unittest tests.test_ledger_mode_config tests.test_doctor_tracker_card` | RED (5 failures/errors) → OK |
| `./tests/run.sh` | go tests `ok`; `Ran 1976 tests … OK (skipped=2)` |
| `./tests/validate.sh` | exit 0 — `gofmt -l` clean, Go tests `ok`, `Ran 1989 tests … OK (skipped=2)` |
| `scripts/build-gate.sh` then `scripts/verify-gate-sums.sh <generated> bin/SHA256SUMS` | `ok — 4 digest entries match the committed trust root` |
| `./dist/worktree-gate-current --selftest` | `ok` |
| `bash -n catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh` | clean |

Unavailable quality signals (stated, not implied): **coverage, linter, type-checker, and formatter
tooling do not exist in this repo** (`openspec/config.yaml` reports `coverage.available: false`,
`linter.available: false`, `type_checker.available: false`, `formatter.available: false`). Only
`gofmt` (via `validate.sh`) and `py_compile`/`bash -n` syntax checks ran besides the test suites.

## TDD Cycle Evidence

| Cycle | RED (observed failure) | GREEN (observed pass) | TRIANGULATE | REFACTOR |
|---|---|---|---|---|
| 1.1–1.2 write surface | `undefined: WriteRequest / WriteOutcome / ApplyWrite / ApplyWriteRequest / WriteOpen|Link|Close|Exempt / WriteKinds` (build failure) | `ok ai-specs.dev/worktree-gate/ledger` after `store.go`/`decide.go` landed | retried open, close-then-close, changed-link, invalid provider JSON, nil provider, unavailable identity, concurrent opens, held lock, byte-identical failures, temp residue | `.Normalize()`/`.Validate(checkpoint)` split, `canonicalProvider` reused for both sides of the link comparison, `hasRow`/`hasItemID` helpers instead of inline scans |
| 1.3–1.5 CLI vehicle | `flag provided but not defined: -write` (7 CLI cases failed, exit 0 instead of 2) | all CLI write cases exit 0 with the sidecar / exit 2 with no JSON | flag-parse fail-open vs post-parse fail-closed, mtime + bytes unchanged, cwd vs `--project-root`, no `state.json.tmp.*`, `-race -count=3` | `ledgerWritePrefix`/`ledgerWriteRun` test helpers; `ledgerIdentityWithStoredSlug` extracted so the post-write re-grade reuses one seam |
| 1.6–1.7 purity + selftest | `undefined: ledgerWriteSelftest` | `TestLedgerGradeNeverWritesStore` + `--selftest` → `ok` | grade in all five checkpoints leaves no store file; consistent grade does not touch mtime | selftest covers the closed four-verb set, every validation rejection, and in-memory idempotent open — still no store IO |
| 2.1–2.2 bridge | `AssertionError: bridge module missing` | 14 tests OK | missing/malformed section, directory-as-artifact, archived change, ambiguous slug, corrupt witness, non-Git root, offline/acquisition-only source assertions | one `_load_trello_link` sibling loader; `change_dir` shared by `tracker_none_reason` and `evidence_payload` |
| 2.3–2.4 host wiring | 3 hook test failures (no `--evidence`, no exempt write, write exit 2 not honored) | hook suite OK, incl. stamp test | unstamped stamp skips both, exemption before grading, `tracker.none` never mutated, materialize substitutes the stamp and `""` without a CLI home | one `_ledger_bridge_call` verb table instead of four inline python heredocs |
| 2.5 guardian | 4 failures + 1 error (no evidence, no exempt write, no `slug`, cold-home path) | 55 guardian tests OK | archived `tracker.none`, write-exit-2 blocks before grading, cold `AI_SPECS_HOME`, direct `pr-review` call | evidence temp file removed in the same function that builds it; `_ledger_evidence_args` returns `(argv, cleanup)` |
| 2.6–2.7 corpus | 7 new rows failed (`needs-item` instead of `allow`, no write sidecar, missing store pins) | parity suite OK, repeated 4× | store pins (item count, id uniqueness, open count, exemption text, stored change), conflict fires at `apply-start` **and** `pre-merge`, residue set still `{witness.json, state.json, state.json.lock}`, byte-stable re-grade after stamp normalization | runner gained `--write` steps, `store_pin`, list `change`, `tracker_none`, and item-id normalization |
| 3.1–3.4 lookup | 5 failures/errors (legacy literal read `gate_mode=off` → mode skipped; `_tracker_recipe_id` undefined; three literal lookups still present) | tracker-gate, guardian, and doctor suites OK | env override still wins; witness recipe's `gate_mode` maps forward; missing witness identical to today; in-scope `grep` assertion | bound recipe's `gate_mode` beats the stamped legacy hint while the hint still fills in when that config declares none |
| 3.5 docs | doc rows absent (asserted by `test_the_out_of_scope_residue_is_named_in_the_docs`, `test_no_hardcoded_recipe_lookup_remains_at_the_three_in_scope_sites`) | `./tests/validate.sh` exit 0 | stale header comment corrected; residue named; checkpoint→owner map restated | one section per doc concern |

## Spec-to-code walk (task 4.2)

| Spec requirement | Satisfied by |
|---|---|
| Machine write surface (open/link/close/exempt, no new binary/module/asset) | `ledger/store.go` (`WriteRequest`, `ApplyWrite`), `ledger_cmd.go`, `main.go`; `go.mod` unchanged; same four-target trust root (`scripts/verify-gate-sums.sh` green) |
| Every declared kind/field has a production writer | `DecisionOpen` (`OpenIfAbsent`), `DecisionClose` (`CloseItem` via `ApplyWrite`), `DecisionLink` (link branch), `Item.{ItemID,URL,NativeType,State,Provider,Exemption}` (link/exempt branches) |
| `link` records native fields + opaque provider; predicate never reads provider | `ApplyWrite` link branch; `verdict.go` untouched; `TestItemCoreFieldsAreProviderNeutral` still green |
| `exempt` persists a reason; visible in `state.json` | `WriteExempt` branch + `setExemption`; corpus 28 `store_pin.exemption` |
| Explicit item opening / "Grading never writes" | `TestLedgerGradeNeverWritesStore` (store file never created; bytes + mtime unchanged); corpus 02/04/07 block with no write |
| Parsing never opens | `Grade` untouched (pure, no IO); hosts never `--write open` from a parse (`test_the_three_in_scope_sites…` + hook/guardian argv assertions) |
| Idempotent retried open | `TestApplyWriteOpenIsOpenIfAbsent`, `TestApplyWriteConcurrentOpensKeepOnePrimary`, corpus 29 (`items: 1`) |
| Unique item ids in one second | `uniqueItemID` + `TestApplyWriteSameSecondIdsStayUnique`, corpus 31 (`items: 2`, `unique_ids: 2`) |
| Repeated link does not duplicate | `TestApplyWriteLinkIsIdempotentForIdenticalState` (`unchanged`, one `link` decision) |
| Close-after-close is explicit, no duplicate | `TestApplyWriteCloseThenCloseIsExplicit` (`already-closed`, row stays) |
| `change-ambiguous` refused without a slug | `TestApplyWriteChangeAmbiguousRefusesWithoutSlug`, `TestLedgerWriteChangeAmbiguousRefusesViaCLI`, corpus 32 |
| Bounded lock: grade fails open, write fails closed | `withStoreLock` (5 × `LOCK_NB`, 20 ms, `ErrLockTimeout`); grade → stderr + continue, write/`--decide` → exit 2; `TestApplyWriteHeldLockTimesOutFailsClosed` |
| Local/code/git evidence, `remote` deferred | `tests/test_ledger_evidence.py` (four keys, `local`/`remote` empty, `git` needs `pr:`); docs state the gap; corpus 27 uses `remote: ""` |
| Evidence passing at apply-start/pr-review/pre-merge/archive-close | hook argv test (`apply-start`), guardian argv tests (`pre-merge`, `archive-close`, direct `pr-review`); `work-start` untouched (`plan-build-gate.sh` unmodified) |
| Malformed artifact fails open | `test_missing_or_malformed_artifact_fails_open`; `loadLedgerEvidence` + empty sides |
| Evidence acquisition is offline / cold cache | source-level no-network assertions; `test_evidence_survives_a_cold_cli_home`; `test_bridge_is_acquisition_only_and_offline` |
| Persistent `tracker.none` exemption honored at every checkpoint, not a conflict, not auto-revoked | `WU-1` exemption branch (`ReasonExempt`), `TestAdjudicateClearsExemption`, `TestLedgerWriteExemptHonoredAtEveryCheckpointViaCLI`, corpus 28 across all five checkpoints |
| Witness-derived config lookup at the three sites | `bridge.recipe_id` in `premerge_guardian.resolve_ledger_mode`, `tracker-card-gate.sh::_ledger_recipe_id`, `doctor._tracker_recipe_id`; bound-recipe fixture tests |
| Missing witness falls back identically | guardian/tracker-gate mode tests with no witness; bridge unit tests for corrupt/empty/unknown-version/no-git |
| Checkpoint ownership stays put | `plan-build-gate.sh` bytes untouched; hook + guardian argv assertions; `test_the_out_of_scope_residue_is_named_in_the_docs` |
| Unhosted `work-start` visible | `doctor` INFO `work-start is unhosted` (+ absence when plan-build-flow is enabled or no bound witness); `docs/capabilities.md` names the residue |
| Write failure postures fail closed; grade paths fail open | CLI table (invalid payload, ambiguous, colliding identity, unwritable path) + mtime check; corpus 33 (`exit 2`, empty stdout, store pin); `TestLedgerFlagParseFailsOpen` |
| Preserved behaviors: exit codes, `openspec/**`, no-TTY `ask` = exit 2, corrupt store → unevaluable + doctor ERROR, `warn` posture | existing suites unchanged: `TestLedgerExitCodeContract`, `test_openspec_paths_never_grade`, corpus 24; `test_ask_without_tty_blocks_without_fabricating_a_decision` (hook + guardian); `test_infra_unevaluable_renders_error`, corpus 16/17; `ai-specs/ai-specs.toml` `gate_mode = "warn"` untouched |
| Selftest covers ledger + write invariants, offline | `ledgerWriteSelftest()` + `--selftest` → `ok` |
| Corpus pins every design row | `DESIGN_ROWS` now 33 entries, all with a 1:1 fixture; `test_corpus_covers_every_design_row` green |

## Deviations from design (all deliberate, none widening scope)

1. **`ApplyWriteRequest` carries the `Checkpoint`.** The design's `Decision` has a `Checkpoint` field and
   its `Validate(checkpoint)` implies the value is known at write time; recording `open`/`link`/`close`
   decisions with an empty checkpoint would have lost the audit trail, so the checkpoint is validated and
   stored on the appended decision.
2. **`--write` is attempted regardless of binding activity.** `--decide` skips when the witness is
   dormant; a silently skipped write would be exactly the "silent no-op" DW4 forbids. A dormant ledger's
   write is harmless (nothing grades it unless it becomes bound), and every outcome is reported in the
   sidecar or as exit 2.
3. **The bound recipe's `gate_mode` now beats the stamped legacy hint** in the hook and guardian. Task
   3.1/3.2 require the effective `gate_mode` to come from `recipes.<witness-id>.config`; the stamped hint
   (which is always the `trello-mcp-workflow` value) still fills in when that section declares none.
4. **The bridge exposes three extra helpers** (`change_slug`, `change_dir`, and the `recipe`/`exempt`/
   `evidence` verbs the hosts call) beyond `recipe_id`/`tracker_none_reason`/`evidence_payload`. The hook
   cannot know the change slug from a path event, and pre-merge grades an *archived* change, so
   archive-aware change resolution was required by 2.4/2.5. The bridge remains acquisition-only and
   `trello_link.parse_tracker_section` remains the only parser.
5. **`doctor` holds no literal recipe id.** `_tracker_recipe_id()` returns `""` when the bridge cannot be
   loaded, so the fallback literal lives only in `ledger_bridge.LEGACY_RECIPE_ID`. With a witness present
   the relevance gate behaves as before (the witness clause already covered that case), so the doctor
   change is a no-behavior-change refactor plus the new unhosted-`work-start` INFO.
6. **Parity runner extensions** (test infrastructure): `--write` steps, `unittest`-level `store_pin`
   assertions, `tracker_none` fixture support, list-valued `change`, and item-id normalization. The item
   id is `sha256(identity + opened-at)`, i.e. clock-derived like `recorded_at`, so two fresh runs of a
   write row cannot be byte-compared without masking it.
7. **No `unexempt` kind, no store schema bump, no `remote` producer, no provider or MCP write, no
   plan-build-flow content change** — non-goals held.

## Remaining tasks

None. All 24 checkboxes in `openspec/changes/trello-ledger-integration/tasks.md` are `- [x]`.

## Workload / PR boundary

Honest authored count (additions + deletions, generated digests excluded from the authored figure but
included in the tree):

| Unit | Authored changed lines |
|---|---|
| WU-1 Phase 1 — Go write surface + tests + trust root | ~1,549 (incl. 659 new `write_test.go`; generated `SHA256SUMS` recorded here per the "never split from Go" rule) |
| WU-2 Phase 2 — bridge + hosts + corpus + tests | ~1,406 (incl. 444 new Python test/bridge lines and 552 fixture lines) |
| WU-3 Phase 3 — witness lookup + docs | ~551 |
| **Total** | **~3,506** |

Budget status: far above the 400-line canonical review budget and above the 1200-line session budget.
The maintainer accepted `exception-ok` / `size:exception` for a single PR before apply, so the three
review units ship together as one PR boundary (trust root never split from the Go unit). No honest
slicing pass could shrink this below the budget as one cohesive change — nothing was deleted or
compressed to reach a number.

Rollback boundary (independent of commits): revert the Go files and regenerate
`bin/SHA256SUMS` + `dist/worktree-gate-current` in the same revert; revert `ledger_bridge.py`, the two
host edits, the stamp, and the fixtures — the grader then behaves verdict-only as before. No store
schema bump and no external side effect to compensate.

## Risks / notes for verify

- **Harness attempt accounting could not be executed.** `gentle-ai` is not on `PATH` in this
  environment (`command -v gentle-ai` → not found), so `sdd-attempt acquire/settle` could not be run and
  the pre-existing active attempt token
  `sha256:1b83fabee8494c4bceacc514544165b3fa945d230e811cbce1b4b42501c2df33` was neither continued nor
  settled. The work itself consumed the fresh native status the parent injected (apply/ready). A parent
  with the CLI available should settle that attempt.
- `dist/` is gitignored; `dist/worktree-gate-current` was rebuilt locally so the parity suite runs. The
  committed trust root (`bin/SHA256SUMS`) is the durable artifact and is verified by
  `scripts/verify-gate-sums.sh`.
- The hook now spends one extra `python3` invocation per production write to read the witness recipe id
  (part of `_ledger_mode`). Measured only indirectly: the hook suite (24 tests, each spawning the hook
  and stub binary) runs in ~20 s, unchanged in order of magnitude.
- `premerge_guardian.ledger_blockers` gained a third positional parameter (`slug`), passed from `main()`
  as `args.slug`. Callers outside this repo would need the argument; it defaults to `None`.
- `Item.Exemption` written before a revert stays inert in `state.json` and is cleared only by an explicit
  `--decide kind=adjudicate` (documented in `docs/capabilities.md` and the recipe README).
