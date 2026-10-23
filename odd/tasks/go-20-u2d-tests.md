# Go 20 / U2d — tests: retarget launcher-driven modules to the Go binary

Unit U2d of `odd/tasks/go-20-u2-launcher-flip.md`. Branch `change/go-20-u2d-tests` cut from `epic/go-single-binary` @ `84e151b` (U2c, PR #421). PR target: `epic/go-single-binary`, never `development`.
Tracker: epic card https://trello.com/c/qwlHQ7Xa; U2d card #189 https://trello.com/c/1COfwsgG (board `69ec097f13e2d38ecd89a557`, In Progress). Predecessor card #187 (U2c, Done).

## Specs

- S1 — Parent request, verbatim (2026-10-23): "migra los módulos de tests que aún usan launcher/scripts legacy hacia binario Go, preservando cada comportamiento y cobertura; no borres aserciones sin equivalente ni adelantes flip/deleción U2e."
- S2 — Plan U2d, verbatim from `go-20-u2-launcher-flip.md`: "retarget the 20 launcher-driven modules to `_go_cli.cli()` and rewrite the 5 legacy-only modules to drive the binary. Adjust `test_upgrade.py` and `test_harness_cli_literacy.py`, which read the launcher as text. No test is deleted without a replacement that asserts the same behaviour."
- S3 — Measured inventory (epic `84e151b`): `tests/_blackbox.py` `CLI` points at `bin/ai-specs`; every module that imports `_blackbox.invoke` (about 50 modules, not 20 as S2 counted) therefore drives the bash launcher. The launcher dispatches all verbs to `lib/*.sh`, except `hub`, `init`, `sync`, `sync-agent`, `refresh-bundled` and `add-dep` (still bash in `bin/ai-specs:38-50`); the Go binary routes every verb natively (`internal/cli/cli.go` `native` map and `Route`).
- S4 — Legacy-only modules (run `bash lib/*.sh` with `AI_SPECS_HOME=ROOT`): `test_recipe_remove.py`, `test_skills_add.py`, `test_skills_list.py`, `test_skills_remove.py`, `test_upgrade.py`. Their replacement drives the binary through `_go_cli.cli()`, asserting the same exit codes, stdout/stderr and file effects.
- S5 — Text-reading tests: `test_upgrade.py:25-28` and `test_harness_cli_literacy.py:90-103` read `bin/ai-specs` as text. They assert the native route for the verb (the Go binary), not the launcher's bash arms.
- S6 — Scope boundary, recorded, not declared settled: modules that still run `lib/*.sh` outside the list above (`test_docs_drift_fixes.py`, `test_upgrade_mode_dirt.py`, `test_upgrade_output.py`, others found by re-running the grep after T2) are classified in T5. Modules that load `lib/_internal/*.py` in-process or as subprocess oracles are U2e's inventory (deletion of `lib/_internal`); U2d does not delete them and does not rewrite them unless a native route exists and the module is a launcher or `lib/*.sh` driver.
- S7 — Interpreter: Python legs run with `PATH=/Users/robert/.hermes/profiles/cody/cache/scratch/u2c-pybin:$PATH` (CPython 3.14.7, `tomllib` present; `python3` on the default PATH is 3.9). Scratch and `TMPDIR` are `/Users/robert/.hermes/profiles/cody/cache/scratch`. No network, no global or external temporary state.
- S8 — Permissions: commits and PRs to `epic/go-single-binary` under the standing permissions. Native assess runs before each publish with `baseRef` = the epic tip the slice was cut from, `committedOnly` = true, `workspaceRoot` = this worktree. No auto-merge is inferred from this document.
- S9 — Out of scope: U2e (flip, `lib/_internal` deletion, parity corpus snapshot, contract rewrite); tracker ledger hook (not materialized); U2a L11/L12 (separate audit); routine `ai-specs sync` or upgrade.
- S10 — Gates per slice: `go build ./...`, `gofmt -l` on touched Go, focused `unittest` modules on the Python leg, and `./tests/validate.sh` once by the parent before the final publish.

## Tasks

- **T1** — RED: `tests/_blackbox.py` must drive the native binary. A test asserts that `invoke` runs the Go route (`cli()`), not `bin/ai-specs`, and that a verb the launcher delegates to bash (`version`) prints the same bytes through the binary. Fails today (`CLI` is the launcher). Covers: S2, S3.
- **T2** — GREEN: retarget `_blackbox.CLI` to `_go_cli.cli()`; fix the launcher-driven modules that break on the binary, asserting the same behaviour (no assertion removed without an equivalent). Covers: S2, S3.
- **T3** — Legacy-only rewrite: `test_recipe_remove.py`, `test_skills_add.py`, `test_skills_list.py`, `test_skills_remove.py` drive `cli()` with the verb arguments; `test_upgrade.py` (lib/upgrade.sh) likewise. Each module keeps its case list and assertion count or documents the equivalent. Covers: S4.
- **T4** — Text-reading tests: `test_upgrade.py:25-28` and `test_harness_cli_literacy.py:90-103` assert the native route. Covers: S5.
- **T5** — Re-run the grep for `lib/*.sh` drivers outside the list, classify each (migrate, or defer to U2e with a named row), and migrate the in-scope ones. Covers: S6.
- **T6** — Gates and publish: per-slice commits of about 400 changed lines, native assess with `baseRef`, PR to the epic, `validate.sh` once. Covers: S8, S10.

## Log

- L1 — Parent request verbatim (2026-10-23): "Continúa Go 20/U2d según odd/tasks/go-20-u2-launcher-flip.md." followed by "migra los módulos de tests que aún usan launcher/scripts legacy hacia binario Go, preservando cada comportamiento y cobertura; no borres aserciones sin equivalente ni adelantes flip/deleción U2e." The plan file named in the request is `odd/tasks/go-20-u2-launcher-flip.md` (U2d section).
- L2 — Baseline before any write, epic `84e151b`, scratch Python 3.14.7: `test_recipe_remove`, `test_skills_add`, `test_skills_list`, `test_skills_remove`, `test_upgrade`, `test_harness_cli_literacy` run 81 tests, OK, 31 s.
- L4 — First measured inventory (`scratch/u2d-seam-blackbox.log`, real exit 1, pre-fix): 47 `_blackbox` modules, 1187 tests, 21 FAIL + 1 ERROR. Classified: (A) `test_doctor_tracker_card` 11 FAIL: stub gate lacks the native digest/version/self-test trust root; pinned through `WORKTREE_GATE_BIN` (documented seam), trust covered by gatebinary tests. (B) refresh-bundled standalone assertions 9 FAIL: D26 makes the standalone verb a no-op; migrated to `sync` with every assertion mapped, none removed. (C) vault ERROR: `ImportError` from `from _blackbox import CLI`, consumer migrated to `cli_path()`, collection test added.
- L5 — Post-fix rerun, same seam: `Ran 1197 tests`, `OK`, `EXIT=0` (scratch `u2d-seam-blackbox.log`, 544 s). Docs corrected: `sync` is not read-only and is not claimed to be; standalone `refresh-bundled` is the Go no-op (`internal/cli/refresh_bundled_test.go`), effective refresh covered by `internal/sync/refreshbundled_test.go`.
- L6 — Slice 1 = T1, T2, cluster B and cluster C together. T3 (legacy-only modules, `test_upgrade` included) and T4 remain pending; `test_upgrade` needs its own analysis because the Go upgrade engine uses the executable path for self-update.
- L3 — Card #189 created in In Progress on board `69ec097f13e2d38ecd89a557` after `get_lists` confirmed the list ids (the first attempt used the board id as list id and returned 404; no write happened).
