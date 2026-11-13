# GO-20 U2e T5 PR4a S7b — parity and evals harnesses stop using `lib/`

Base: epic/go-single-binary 3346755 (after #524 merged). Branch:
change/go-20-u2e-t5-pr4a-s7b-parity.

Scope: the last two harnesses that still reached into the repository `lib/`:
`tests/parity/` (fixture home + normalizations + capture script) and
`tests/evals/lib/harness.py` (Python materializer + `hooks-render.py`). Parity
keeps comparing **the same frozen goldens** against the live Go binary; the
goldens are **not** recaptured, and no expected value is silently revised.

## Specs

- S1: Standing decision (Robert): finish PR4a ("ok,vos dale debemos ir
  terminando esto"); same slice discipline ("excelente, continuemos así muchas
  gracias").
- S2: Every `lib/` touch point in the four files is inventoried and classified:
  **copied**, **executed**, **dead reference**, **comment only**, or
  **tolerant removal** — each with a disposition.
- S3: Comparison strength is unchanged: `gate-absent failing=0` and
  `gate-present failing=0` before and after, with `git status` on
  `tests/parity/goldens/` empty (no golden byte touched).
- S4: Golden integrity: no recapture. `capture_goldens.py` cannot reproduce a
  legacy capture any more, so it refuses to write (exit 2) instead of freezing
  native output over the frozen corpus.
- S5: A normalization stays only if the frozen bytes still require it TODAY, and
  the decision is measured (remove it → parity must fail); otherwise it is
  removed with the parity run proving 0/0.
- S6: FALSE legacy labels are corrected to reality — the live side is Go, the
  frozen side is the legacy golden. Mode-id strings that `tests/test_parity_harness.py`
  pins (`legacy-vs-go`, `legacy-vs-legacy`) are kept as historical ids.
- S7: `tests/evals/lib/harness.py` replaces the legacy materializer and
  `hooks-render.py` calls with native CLI calls, keeping every eval scenario's
  setup behavior; `tests/evals/run.sh` keeps its `python3` runner (evals are
  test infrastructure, not production Python).
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh`
  rc=0, Python discover count unchanged (1804). No Go file changes, so
  `go vet`/`go test` need no separate run beyond `validate.sh`'s legs.

## Baseline (before any change)

`go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/build-baseline.log`).

- Parity, both modes, 35 fixtures: `gate-absent failing=0, gate-present
  failing=0 — PASS`, rc=0, 52.7s (`logs/parity-baseline.log`).
- Parity `--self-test`: rc=0, `failing=0` both modes, 9.6s
  (`logs/parity-selftest-baseline.log`) — see finding F4: this mode is vacuous.
- `./tests/evals/run.sh`: rc=1, `Ran 46 tests … FAILED (errors=2, skipped=18)`
  (`logs/evals-baseline.log`). The two errors are pre-existing and unrelated to
  this slice: `eval_harness_smoke.py` imports `tests._cache_paths`, a module that
  no longer exists (`ModuleNotFoundError`), and that file is outside the allowed
  surfaces.

## Inventory and disposition

| # | file:line (before) | `lib/` touch point | class | disposition |
|---|---|---|---|---|
| 1 | `tests/parity/parity.py:1526-1537` (`make_home`) | `shutil.copytree(ROOT/"lib", …)` + **unconditional** `(home/"lib").unlink()` | copied (the 8th intolerant caller named in #524) | copy removed, `unlink(missing_ok=True)`: the home carries no `lib/` and tolerates its absence after PR4b |
| 2 | `parity.py:167-193` (`N7`, `N8` + two regexes) | stripped legacy-Python `GO_*_BRIDGE_FALLBACK` / hook-bridge reconcile notices | dead normalization | **removed**, measured: parity 0/0 after removal (`logs/parity-phase2-noN7N8.log`) |
| 3 | `parity.py:192,196` `_norm_*bridge*` helpers | same as #2 | dead normalization | removed with `N7`/`N8` |
| 4 | `parity.py:222-225` (`N10` comment) | claimed N10 expires with `lib/` | comment only (false) | corrected: the frozen goldens still carry the legacy python3 deny line |
| 5 | `parity.py:365-368`, `:417-423` (`N10`/`N11`) | both claimed "expires with `lib/_internal` (T5)" | live normalization | **KEPT**, evidence F3: dropping them fails 3 fixtures in both modes |
| 6 | `parity.py:840` | comment naming the **deleted** `tests/test_materialize_bridge.py` | dead reference | removed |
| 7 | `parity.py:1-13, 28-31, 68-80, 1525-1536, 1672-1680, 1749-1766, 1806-1808` | prose describing the live leg as legacy | false labels/comments | corrected to "frozen golden vs live Go"; the self-test note now says `bin/ai-specs` is the native launcher and `legacy-vs-legacy` is a historical mode id |
| 8 | `parity.py:1644` (`load_golden_leg` error) | told the reader to recapture with `capture_goldens.py` | dead reference | repointed: the corpus is frozen, restore from git or use `GOLDEN_EXCLUDED` |
| 9 | `capture_goldens.py:2-4,46` | ran `bin/ai-specs` while calling it the legacy launcher | executed (false label) | docstring corrected from reality; the capture body is gone — `main()` refuses with exit 2 (S4) |
| 10 | `tests/evals/lib/harness.py:21,45-51` | `RECIPE_MATERIALIZE` + `importlib` load of `lib/_internal/recipe-materialize.py` | executed Python | removed; `materialize_project` now runs the native `sync` spine with no agent enabled (materialize half only) |
| 11 | `harness.py:109-132` | `materialize_recipes(…, resolved_hooks_out=…)` + `lib/_internal/hooks-render.py` | executed Python | removed; `wire_runtime_hooks` narrows `[agents].enabled` to the runtime's agent and re-syncs (native renderer wires only that channel) |
| 12 | `harness.py:16,19` (`sys`, `tempfile`) | only used by the materializer plumbing | dead imports | removed |
| 13 | `tests/evals/run.sh:14` | `python3 -m unittest` runner | — | **unchanged** (S7) |

## Findings

- **F1 — the parity fixture was the eighth intolerant caller.** `make_home`
  copied repository `lib/` and then unlinked the inherited symlink
  unconditionally, so it both staged Python the native binary never reads and
  would crash the moment PR4b deletes `lib/`. It now uses
  `bb.isolated_home(...)` + `(home/"lib").unlink(missing_ok=True)`: same fixture
  shape as the de-libbed black-box suites, tolerant by construction. Parity
  stayed 0/0 (`logs/parity-phase1.log`), i.e. the Go leg never read that copy.
- **F2 — `N7`/`N8` were already dead.** The frozen goldens contain neither
  `*_BRIDGE_FALLBACK` nor `reconciled the lock` lines (they were captured with
  those rules applied), and the Go leg prints neither. Removing both rules keeps
  `gate-absent failing=0, gate-present failing=0` (`logs/parity-phase2-noN7N8.log`).
  The registry keeps the `N9`/`N10`/`N11` ids so external references
  (`tests/test_parity_harness.py`, the odd/tasks records) stay valid, and a
  comment there marks the `N7`/`N8` gap.
- **F3 — `N10`/`N11` still stand; they expire with the corpus, not with `lib/`.**
  Measured by dropping both rules in-process and re-running the corpus
  (`logs/parity-n10n11-experiment.log`): `cache-layout` and `hooks-five-runtimes`
  diverge on the `.cursor/hooks/*.sh` tree hash (the golden hash was computed over
  the canonicalized wrapper) and `vendor-add-clone-failure` diverges on stderr
  (`  ✗ exit status N` is native-only; the golden was stored with the wrapper
  removed) — rc=1 in **both** gate modes. Their justifications and descriptions
  now say that, and keep the `T5` marker that
  `tests/test_parity_harness.py::test_n11_records_its_expiry` pins.
- **F4 — the `--self-test` mode is vacuous after the flip (pre-existing).** In an
  isolated home `bin/ai-specs` looks for `$AI_SPECS_HOME/dist/ai-specs`, which a
  capture/self-test home does not have, so every step of both legs prints
  `ai-specs: native binary not found …` and compares equal (probed directly:
  `fresh-init` step rc=1, that stderr). Recorded already in
  `odd/tasks/go-20-u2e-flip.md:354`. This slice corrects the label it prints
  (both legs are the native launcher, not legacy) and leaves the mode's fate to
  PR4b, which deletes the corpus; making it meaningful would need a built binary
  pin, i.e. a behavior change no spec asked for.
- **F5 — `tests/_go_cli.py` and `tests/parity/run.py` still call `bin/ai-specs`
  "the legacy Bash launcher" in their docstrings.** Both are outside the allowed
  surfaces; the statements were already false before this slice. Doc-only fix,
  PR4b.
- **F6 — remaining `lib/` Python executions under `tests/` (PR4b blockers).**
  `tests/test_sync_pipeline.py:2049` "directly invokes `recipe-materialize.py`",
  plus S7a's F2 (`tests/test_env_scaffold.py` imports `env_scaffold`/
  `dep_install` from a copied `home/lib/_internal`). Both are out of this slice.
- **F7 — `tests/test_eval_hook_wiring.py` pins the split the native spine does
  not offer in one call, and it shaped the design.** It asserts that after
  `materialize_project` + `wire_runtime_hooks(root, "cursor-agent")` there is
  **no** `.claude/settings.json`, i.e. hook wiring is per-runtime. A single
  `sync` cannot express that: its spine always passes the resolved-hooks
  document to the fan-out, so every enabled agent's channel is wired. Measured
  first attempt (`materialize_project` = one plain `sync`): the file is written
  for the manifest's default `claude`, so the cursor test fails
  (`AssertionError: True is not false`) and `./tests/validate.sh` rc=1. Fix, in
  the harness only, mirroring the legacy split exactly: `materialize_project`
  runs the spine with `[agents].enabled` temporarily emptied (recipes, lock,
  brief, gitignore — all agent-independent — and **no** agent channel; the
  manifest is restored in a `finally`), and `wire_runtime_hooks` narrows
  `[agents].enabled` to the runtime's agent and re-syncs, wiring only that
  channel (the legacy `hooks-render.py <agent>` argument did the same). Verified:
  `tests.test_eval_hook_wiring` + `tests.test_parity_harness` 41 tests OK, evals
  identical to baseline, and the out-of-tree self-check above.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S5 | inline | one work unit |
| T2 | S3, S4, S6 | inline | |
| T3 | S7 | inline | |
| T4 | S8 | inline | |

## Log

- L1 (Robert): "ok,vos dale debemos ir terminando esto" — finish PR4a.
- L2 (Robert): "excelente, continuemos así muchas gracias" — same slice
  discipline: inventory, classify, measure each removal.
- L3 — Baseline (S3/S8) captured: builds rc=0; parity 0/0 in both modes
  (52.7s); `--self-test` rc=0 (vacuous, F4); evals rc=1 with the two pre-existing
  `tests._cache_paths` errors.
- L4 — Phase 1 (fixture + false labels, #1/#4/#6/#7/#8): parity still
  `gate-absent failing=0, gate-present failing=0` (`logs/parity-phase1.log`).
  `python3 -m unittest tests.test_parity_harness` → 39 tests, OK
  (`logs/unittest-parity.log`); one wording fix was needed because that suite
  pins `"T5"` inside `N11.description`.
- L5 — Phase 2 (`N7`/`N8` removed, F2): 0/0 preserved.
- L6 — Phase 3 (capture refusal, S4): `python3 tests/parity/capture_goldens.py`
  → rc=2 with the refusal (`logs/capture-refusal.log`). No golden written;
  `git status tests/parity/goldens` empty.
- L7 — Phase 4 (evals harness, S7), first attempt: `materialize_project` = one
  plain `sync`. Evals unchanged (rc=1, the two pre-existing `tests._cache_paths`
  errors, `logs/evals-after.log`), but the full suite caught a pinned contract:
  `tests/test_eval_hook_wiring.py::test_no_file_write_hook_for_cursor` failed
  because a plain `sync` wires the default `claude` channel too (F7), and
  `./tests/validate.sh` returned rc=1 with exactly that failure
  (`logs/validate.log`).
- L8 — Phase 5 (F7 fix): materialize with no enabled agent, wire by narrowing to
  the runtime's agent. `tests.test_eval_hook_wiring tests.test_parity_harness`
  → 41 tests OK (`logs/unittest-hooks-parity.log`); `./tests/evals/run.sh` →
  rc=1, `Ran 46 tests … errors=2, skipped=18`, identical to baseline
  (`logs/evals-after2.log`). Out-of-tree self-check
  (`check-evals-harness.py`, not committed) proves: `materialize_project`
  produces recipes + lock + `AGENTS.md` + root gitignore, **no** agent channel
  and no `lib/`, and restores `[agents].enabled`; `wire_runtime_hooks(claude)`
  wires `PreToolUse`/`plan-build-gate`; `(cursor-agent)` writes no claude channel
  and skips the file-write hook; `(opencode)` renders
  `.opencode/plugin/plan-build-flow-plan-build-gate.ts`.
- L9 — (S8) `go build -o dist/ai-specs ./cmd/ai-specs` and
  `./tests/validate.sh` re-run after the F7 fix — see Result.

## Not this slice (out of allowed surfaces)

`tests/parity/run.py` and `tests/_go_cli.py` docstrings (F5),
`tests/test_sync_pipeline.py:2049` and `tests/test_env_scaffold.py` (F6),
`tests/test_parity_harness.py` (it pins the mode ids and `N11`'s `T5` marker; its
assertions were honoured, not edited), `tests/test_eval_hook_wiring.py` (it pins
the per-runtime hook-wiring contract; honoured by design, see F7), the goldens
themselves (never recaptured), and every production/`lib/` path.

## Result

- `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/build-final.rc`).
- `./tests/validate.sh` rc=0, 9m06s (`logs/validate-final.log`, `logs/validate-final.rc`):
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS` and the
  Python leg `Ran 1804 tests in 477.138s — OK (skipped=139, expected failures=3)`.
  Count unchanged (1804), so no assertion was lost. The 139 skips are
  environment-driven and pre-existing; neither unittest-discovered file this
  slice touches contributes one (`tests.test_eval_hook_wiring` +
  `tests.test_parity_harness` → 41 tests, OK, 0 skips).
- No Go file changed; `validate.sh`'s module-wide `go test` legs (both modules)
  cover it, all `ok`.
- No golden recaptured: `git status`/`git diff` on `tests/parity/goldens/` is
  empty, and `capture_goldens.py` refuses to write (exit 2).
- Final classified grep over `tests/` for repository `lib/` (`--include=*.py`):
  1. **`lib/` COPY: none.** No `shutil.copytree(…/lib…)` site is left anywhere
     under `tests/`; parity and evals are both gone.
  2. **Tolerant removals (fine):** 30 `(home / "lib").unlink(...)` lines, one of
     them this slice's `parity.py:1484` with `missing_ok=True`.
  3. **PR4b blocker, still intolerant:** 29 `(home / "lib").unlink()` calls in
     out-of-scope suites (e.g. `test_hub.py:66`, `test_sync_pipeline.py:40`,
     `test_env_scaffold.py:62`); they crash once `tests/_blackbox.py` stops
     creating the symlink (`_INSTALL_ENTRIES` still names `"lib"`). Parity is
     no longer one of them.
  4. **PR4b blocker, still executing Python:** `tests/test_sync_pipeline.py:2049`
     (invokes `recipe-materialize.py`) and `tests/test_env_scaffold.py` (imports
     `env_scaffold`/`dep_install` from `home/lib/_internal`), i.e. F6.
  5. **Comment/docstring history only:** every other `lib/` mention under
     `tests/` (`"no test may import lib/_internal"` headers, `test_upgrade_output.py`
     notes, `tests/evals/lib/project_fixture.py:296`, negative guards such as
     `test_git_pr_flow_recipe.py`'s absence assertion).
