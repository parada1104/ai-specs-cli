# GO-20 U2e T5 PR4a S7a — stop copying `lib/` into test fixtures

Base: epic/go-single-binary e3386d3 (after #523 merged). Branch:
change/go-20-u2e-t5-pr4a-s7-fixturelibs.

Scope: the 29 Python test surfaces that still copied the repository `lib/` into
their fixture home (`shutil.copytree`) or listed `"lib"` in an install-entry
tuple, while actually grading the NATIVE binary (`bin/ai-specs` → `dist/ai-specs`
→ `internal/…`). After the launcher flip (#451) the native binary needs no
`lib/` in the home, so each fixture drops it and every native assertion stays.

## Specs

- S1: Standing decision (Robert): finish PR4a ("ok,vos dale debemos ir terminando
  esto"); same slice discipline ("excelente, continuemos así muchas gracias").
- S2: Every listed file is classified below: **copy removed**, **negative guard
  kept**, **install entry dropped**, **docstring only**, or **PR4b blocker**.
- S3: No native assertion is deleted or weakened. A de-libbed fixture still
  grades the same verbs, streams and exit codes.
- S4: A fixture that needs `lib/` CONTENT (not existence) keeps its copy and is
  reported as a PR4b blocker with the observed failure, never faked.
- S5: Baseline before any change (rc + per-file counts) and the post-slice count,
  so the delta is reconcilable.
- S6: Production code, catalog, goldens, `tests/parity/`, `tests/evals/` and
  every Python test outside this slice stay untouched.
- S7: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh`
  rc=0; the Python discover count is unchanged (1804).

## Baseline (before any change)

Builds: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `scripts/build-gate.sh`
rc=0 (`logs/01b-build-gate.log`).

All 28 listed files ran once, per-file rc logged (`logs/02-baseline-per-file.log`,
per-file stdout in `logs/baseline-<file>.log`). **703 tests, every file rc=0.**
Per file: gitlab_mr_flow 62, jinna_provider 66, tdd_flow 2, playwright_ui 13,
envrc_scaffold 10, trello_consolidation 4, worktree_flow 34, git_pr_flow 25,
dep_check 12, runtime_brief_ownership 33, release_materialization 2, hub 32,
hub_tui 15, harness_cli_literacy 13, plan_build_flow 39, rules_audit 28,
session_context 3, recipe_materialize 73 (+1 expected failure), hooks_render 25,
doctor 89, doctor_worktree_gate 9, worktree_root_propagation 10,
vault_canonical_store 9, trello_mcp_workflow 26, env_scaffold 53,
gitignore_root_refresh 4, jinna_consumer 1, recipe_entrypoint_parity 11.

## Mapping tables

Totals: 29 surfaces. Disposition: **24 copy removed**, **1 copy + dead `_vendor`
plumbing removed**, **1 own install tuple loses `lib`**, **1 shared install list
kept (with reason)**, **1 shared install tuple loses `lib`**, **1 docstring
only**, **1 PR4b blocker (copy kept)**. Assertions deleted: **0** — the slice
removes staging code only, so the discover count must not move.

The ten `_make_home` bodies in the "common shape" group were identical (isolated
home → unlink the `lib` symlink → `shutil.copytree` a real copy → return). Each
keeps the unlink (that is the S6 pattern: the fixture proves the native binary
needs nothing from `lib/`) and loses the copy plus the now-unused `import shutil`.

| # | file | what it copied / listed | class | disposition |
|---|---|---|---|---|
| 1 | `test_gitlab_mr_flow_recipe.py:67` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 2 | `test_jinna_provider_recipe.py:105` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 3 | `test_tdd_flow_recipe.py:39` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 4 | `test_playwright_ui_flow_recipes.py:52` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 5 | `test_envrc_scaffold.py:59` | — (docstring "real lib copy" only; `_make_home` already returns `isolated_home(catalog=False)`) | docstring | docstring corrected; no code change |
| 6 | `test_trello_consolidation.py:38` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 7 | `test_worktree_flow_recipe.py:58` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 8 | `test_git_pr_flow_recipe.py:66` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 8b | `test_git_pr_flow_recipe.py:218` | `self.home / "lib" / "_internal" / "premerge_guardian.py"` absence guard | negative guard | KEPT, repointed to `ROOT / "lib" / "_internal" / …`: with `lib/` no longer installed the home-leg assertion would be vacuous; the install tree the home mirrored still proves the Python guardian is gone |
| 9 | `test_dep_check.py:67` | real `lib/` copy, empty catalog | fixture copy | copy removed, `import shutil` dropped |
| 10 | `test_runtime_brief_ownership.py:33` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 11 | `test_release_materialization.py:103` | real `lib/` copy over the symlink farm | fixture copy | copy removed, `import shutil` dropped |
| 12 | `test_hub.py:68` | real `lib/` copy + `_vendor` symlink + `vendor=` kwarg | fixture copy | copy, `_vendor` symlink and the now-meaningless `vendor` kwarg removed; `test_imports_without_third_party_deps` keeps its native assertion (bare home, `Summary`/`Commands`, rc 0) with the docstring corrected |
| 13 | `test_hub_tui.py:80` | real `lib/` copy + `_vendor` symlink ("needed by the interactive deps gate") | fixture copy | copy and `_vendor` symlink removed: the interactive deps gate is native (`internal/cli/initwizard.go`); all 15 PTY flows stay green |
| 14 | `test_harness_cli_literacy.py:61,68` | own install-entry tuple naming `"lib"` + real copy | install-entry list | `"lib"` dropped from the tuple and the copy removed; the home never has a `lib` entry to unlink |
| 15 | `test_plan_build_flow_recipe.py:49` | real `lib/` copy | fixture copy | copy removed; its `assertNotIn("lib/_internal/premerge_guardian.py", …)` is a rendered-text guard and stays |
| 16 | `test_rules_audit.py:37` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 17 | `test_session_context_recipe.py:40` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 18 | `test_recipe_materialize.py:63` | real `lib/` copy | fixture copy | copy removed |
| 19 | `test_hooks_render.py:59` | real `lib/` copy | fixture copy | copy removed |
| 20 | `test_doctor.py:53` | real `lib/` copy | fixture copy | copy removed |
| 21 | `test_doctor_worktree_gate.py:64` | real `lib/` copy, empty catalog | fixture copy | copy removed, `import shutil` dropped |
| 22 | `test_worktree_root_propagation.py:127` | real `lib/` copy inside the gate-acquisition home | fixture copy | copy removed, `import shutil` dropped |
| 23 | `test_vault_canonical_store_recipe.py:56` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 24 | `test_trello_mcp_workflow_recipe.py:53` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 25 | `test_gitignore_root_refresh.py:48` | real `lib/` copy | fixture copy | copy removed, `import shutil` dropped |
| 26 | `tests/_blackbox.py:57,78-84` | `_INSTALL_ENTRIES` names `"lib"` (symlink-farm entry) | shared install list | **KEPT** — see finding F1 |
| 27 | `test_jinna_consumer_recipe.py:45` | `CLI_HOME_LINKS` names `"lib"` (positive install list, not a negative guard) | install-entry list | `"lib"` dropped from the tuple; the module docstring no longer advertises it |
| 28 | `test_recipe_entrypoint_parity.py:41,86` | imports `CLI_HOME_LINKS` | install-entry list | no edit needed: it inherits row 27's tuple, so its consumer home also stops installing `lib/` (verified: no own copy, 11 tests green) |
| 29 | `test_env_scaffold.py:59` | real `lib/` copy + `_vendor` symlink | **PR4b blocker** | **copy KEPT** — see finding F2 |

### Findings

- **F1 — `_INSTALL_ENTRIES` keeps `"lib"` (deliberate).** Removing the entry
  breaks eight call sites outside this slice that inherit the symlink and then
  **unconditionally** unlink it: `tests/test_sync_pipeline.py:40`,
  `test_init_tui.py:59`, `test_external_dirs.py:41`,
  `test_tracker_ledger_witness.py:67`, `test_agents_render_brief_fragments.py:53`,
  `test_bitbucket_pr_flow_recipe.py:40`, `test_doctor_tracker_card.py:70` (all
  S6, already de-libbed) and `tests/parity/parity.py:1536` (out of scope).
  `isolated_home` only creates the symlink `if src.exists()`, so dropping the
  entry turns each of those `unlink()` calls into `FileNotFoundError`. Those
  eight sites are PR4b work (they must become tolerant when `lib/` is deleted);
  until then this slice removes the COPY everywhere it can, which is the actual
  goal. The de-libbed files keep the S6 `(home / "lib").unlink()` line for the
  same reason they had it before: it proves the fixture install root carries no
  `lib/` at all.
- **F2 — `test_env_scaffold.py` needs `lib/` CONTENT, not existence (PR4b
  blocker).** Its `# TRIAGE:` methods drive `env_scaffold`/`dep_install` at the
  module process boundary: `_run_internal` puts `home/lib/_internal` and
  `home/lib/_vendor` on `sys.path` and the driver does `import env_scaffold` /
  `import dep_install`. Probed by removing the copy:
  `ModuleNotFoundError: No module named 'dep_install' …` and
  `No module named 'env_scaffold'`, rc=1
  (`logs/06-probe-env-scaffold-without-lib.log`); the copy was then restored
  verbatim and the file is green again (53 tests, rc=0). These methods cover
  surfaces with **no** non-interactive CLI verb (`write_env`, legacy migration,
  the TTY-gated configure-recipes offer, `dep_install` plans); retiring them is
  PR4b (it deletes `lib/_internal`, these modules and their probes together).
- **F3 — `test_git_pr_flow_recipe.py` guardian guard.** The absence assertion
  only stayed meaningful if it named the tree the home used to mirror, so it now
  reads `ROOT / "lib" / "_internal" / "premerge_guardian.py"`; it still passes
  (the Python guardian is retired — `internal/gates/gates.go` is the canary it
  already asserts).
- **F4 — `_vendor` had no consumer left.** Both `test_hub.py` and
  `test_hub_tui.py` carried `_vendor` symlinks for rich/questionary and the deps
  gate; after the flip the hub and the wizard are native, so all 47 tests in the
  pair stay green with `_vendor` gone. `test_hub.py`'s `vendor=` parameter became
  dead and was removed with its single call site.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S4 | inline | one work unit |
| T2 | S5, S6 | inline | |
| T3 | S7 | inline | |

## Log

- L1 (Robert): "ok,vos dale debemos ir terminando esto" — finish PR4a.
- L2 (Robert): "excelente, continuemos así muchas gracias" — same slice
  discipline: classify each surface, drop only the staging code, verify.
- L3 — Baseline (S5): 28 files, 703 tests, per-file rc=0
  (`logs/02-baseline-per-file.log`); builds rc=0 (`logs/01b-build-gate.log`).
- L4 — Dispositions (S2/S3) applied, all unstaged. Post-slice per file —
  703/703 tests, every rc=0, no SKIPs added: `logs/07-after-a.log`,
  `logs/07-after-b.log`, `logs/07-after-c.log` (per-file stdout in
  `logs/after-<file>.log`). Identical counts to baseline, so no assertion was
  lost and the reconcile delta is zero.
- L5 — PR4b blockers found: F1 (shared install list), F2 (`test_env_scaffold.py`
  needs `lib/` content; probe `logs/06-probe-env-scaffold-without-lib.log` rc=1,
  copy restored, file green).
- L6 — (S6) Final classified grep over `tests/` (`--include=*.py`, parity/evals
  excluded), three buckets:
  1. **`lib/` COPY still staged — exactly one file:** `test_env_scaffold.py`
     (F2, PR4b blocker, `ROOT / "lib"` inside `_make_home`). Every other
     `shutil.copytree…lib` site is gone.
  2. **`ROOT / "lib"` path reference without a copy:** `test_git_pr_flow_recipe.py:212`
     (negative guard, F3 — necessarily non-vacuous) and
     `test_tracker_ledger_host.py:37-38` (out of scope: retired-host absence guard).
  3. **install-entry tuple naming `"lib"`:** only `tests/_blackbox.py:57` (F1,
     deliberately kept). `test_jinna_consumer_recipe.py`'s `CLI_HOME_LINKS` (and
     therefore `test_recipe_entrypoint_parity.py`, its sole importer) no longer
     names it.
  The 29 `(home / "lib").unlink()` lines that remain are the S6 pattern: the
  fixture REMOVES the inherited symlink, it never copies or reads `lib/`.
- L7 — (S7) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, then
  `./tests/validate.sh` — see Result.

## Not this slice (unchanged, out of allowed surfaces)

`tests/parity/parity.py:1536` (parity harness, F1), `tests/test_env_scaffold.py`
(F2, blocker), and the other `lib` mentions that are fixture *project* paths or
static contracts rather than install copies: `test_tracker_card_gate_hook.py`
(grades events on a `lib/foo.py` path), `test_tracker_ledger_host.py`,
`test_ledger_mode_config.py`, `test_plan_build_gate_hook.py`,
`test_install_narrow_checkout.py` (`RUNTIME_DIRS` static contract),
`test_install_binary_entry.py` / `test_install_dist_copy.py` (synthetic install
trees), `test_upgrade_mode_dirt.py` / `test_docs_drift_fixes.py` /
`test_upgrade_output.py` (upgrade fixtures that seed a legacy `lib/`), plus the
docstring mentions in already-de-libbed S6 suites.

## Result

- `go build -o dist/ai-specs ./cmd/ai-specs` rc=0.
- `./tests/validate.sh` rc=0 (`logs/11-validate.log`): `py_compile` clean, both
  Go modules green (root module `ok` for all `./cmd/... ./internal/...`),
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`,
  Python leg `Ran 1804 tests in 556.010s — OK (skipped=1, expected failures=3)`.
  Count reconciliation: baseline was 1804 and the slice deleted **0**
  assertions, so 1804 is the unchanged, correct count; the 28 touched files
  contribute the same 703 tests before and after
  (`logs/02-baseline-per-file.log` vs `logs/07-after-*.log`). The single skip and
  the three expected failures are pre-existing and elsewhere.
- No Go file changed, so `go vet`/`go test` needed no separate run beyond the
  module-wide `go test` legs inside `validate.sh`.
