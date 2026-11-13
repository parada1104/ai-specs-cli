# GO-20 U2e T5 PR4b — atomic `lib/` deletion

Base: `epic/go-single-binary` @ 8de5556 (after #526). Branch:
`change/go-20-u2e-t5-pr4b-atomic-delete`. Epic: https://trello.com/c/qwlHQ7Xa

One atomic slice: delete `lib/` (838 tracked files) together with every surface that
existed only to serve it — the 14 gate bridge flags, the tracker lib stamp, the
`runShim` exec path, the `lib/_vendor` maintenance script, the parity harness's lib
symlink, and the Python tests that still exec/import `lib/`. Nothing is staged: the
parent owns staging, commit and PR.

## Authority

- **L1** (Robert): "ok,vos dale debemos ir terminando esto" — proceed with the T5
  deletion slice.
- **L2** (Robert): "excelente, continuemos así muchas gracias" — continue as planned.
- **L3** (Robert, 2026-11-13): approved the **ATOMIC PR4b** — one slice deletes
  `lib/` + gate flags + stamp + seams + consumers, with SHA256SUMS regeneration in
  the same slice. The trust-root regeneration mechanism is the one established by
  #526: canonical `bash scripts/build-gate.sh` (go1.24.13), digests produced by the
  command the SHA256SUMS header prescribes, verified with
  `scripts/verify-gate-sums.sh`, plus one regeneration log entry.
- **L4** (standing, recorded in `odd/tasks/go-20-u2e-t5-pr4a-*.md`): the bridge flags
  exist only so `lib/` could call Go; after the deletion they have no production
  consumer and are removed here.

## Specs

- **S1** `git ls-files lib/` is empty after the slice: 838 files deleted (19
  top-level `*.sh`, `lib/_internal` 44 = 42 `.py` + 2 `.sh`, `lib/_vendor` 775).
- **S2** The 14 gate bridge flags and their handlers are gone from
  `catalog/recipes/worktree-flow/gate/main.go`; every gate `main` file whose only
  consumer was one of those flags is deleted together with its flag-driven tests.
  No shared core (`gate/shared/`) is deleted, and no shared-core test is lost.
- **S3** No surviving production Go path spawns an interpreter:
  `grep -rn "python3\|exec.LookPath" install.sh bin/ cmd/ internal/ catalog/recipes/worktree-flow/gate --include=*.go`
  yields no production (non-test) hit that resolves an interpreter. The two live
  recipe.toml readers (`shared.LoadRecipeTagMetadata`, `shared.LoadRecipePrimitives`,
  consumed by `internal/sync`) parse TOML in process through
  `gate/shared/tomlparse.go`, and their tests are proven by a `python3` shim that
  fails loudly and is FIRST on `PATH`.
- **S4** The tracker stamp is removed as ONE step: the `stamped_lib_internal` line in
  `catalog/recipes/trello-mcp-workflow/hooks/tracker-card-gate.sh`, the
  `__TRACKER_LIB_INTERNAL__` substitution in `gate/shared/hookactuator.go`, and every
  test that pins or supplies it.
- **S5** `internal/cli/shim.go` is deleted only after proving no route produces
  `routeShim` and no non-shim caller needs `runShim`/`childEnv`.
- **S6** Every remaining `tests/` file that referenced `lib/` is either tolerant
  (the entry is simply absent) or removed; no Python test imports or copies `lib/`.
- **S7** `scripts/vendor-deps.sh`, the `.gitignore` vendor entries and the
  `tests/validate.sh` lib syntax block are removed; `validate.sh` passes on a
  checkout without `lib/`.
- **S8** SHA256SUMS is regenerated once, after all gate source changes, with the L3
  mechanism, and `scripts/verify-gate-sums.sh` accepts the regenerated trust root.
- **S9** `bash scripts/build-gate.sh` rc=0; `go build -o dist/ai-specs ./cmd/ai-specs`
  rc=0; `./tests/validate.sh` rc=0 with parity 0/0 in both gate modes and the Python
  discover count reconciled by exact arithmetic from the L5 baseline.
- **S10** Historical records that merely MENTION `lib/` are not rewritten:
  `openspec/changes/archive/**`, the frozen differential oracles
  (`internal/target/testdata/target_golden.json`), `tests/parity/parity.py` rules
  N10/N11/N12, and docstrings that describe retired behavior.

## Baseline (L5)

`./tests/validate.sh` on 8de5556: **rc=0**, `Ran 1810 tests … OK (skipped=1,
expected failures=3)`, ~531 s. `bash scripts/build-gate.sh` rc=0 (4 assets),
`go build -o dist/ai-specs ./cmd/ai-specs` rc=0, go1.24.13.

## Inventory — deletion targets and their consumers

| # | target | evidence of the consumer | disposition |
|---|---|---|---|
| 1 | `lib/` 838 files | every `bin/ai-specs` legacy route → `lib/*.sh`; `tests/_blackbox.py:57` symlinks `lib` into every isolated home; `tests/test_env_scaffold.py:62-67` copies `lib/` | delete |
| 2 | `--resolve-bindings` | `gate/main.go:81-89`; `lib/_internal/recipe-materialize.py` bridge caller (retired) | delete flag+handler |
| 3 | `--plan-classify` | `gate/main.go:110`; `lib/_internal/util.py:744` | delete |
| 4 | `--apply-copy` | `gate/main.go:125`; lib materialize bridge | delete |
| 5 | `--materialize-hook` | `gate/main.go:138`; `lib/_internal/recipe-materialize.py:13` | delete |
| 6 | `--write-lock` | `gate/main.go:122`; `lib/_internal/lock.py:write_lock` | delete |
| 7 | `--plan-orphans` | `gate/main.go:103`; lib orphan bridge | delete |
| 8 | `--apply-orphans` | `gate/main.go:109`; lib orphan bridge | delete |
| 9 | `--plan-merge-config` | `gate/main.go:117`; lib config bridge | delete |
| 10 | `--write-recipe-config` | `gate/main.go:121`; `lib/_internal/recipe-config-write.py:381` | delete |
| 11 | `--resolve-primitive-conflicts` | `gate/main.go:97`; `lib/_internal/recipe-materialize.py:686` | delete |
| 12 | `--plan-reconcile-stamps` | `gate/main.go:99`; `lib/_internal/recipe-materialize.py:334` | delete |
| 13 | `--plan-resolved-config` | `gate/main.go:106`; lib resolved-config bridge | delete |
| 14 | `--resolve-tag-conflicts` | `gate/main.go:95`; `lib/_internal/recipe-materialize.py:575` | delete |
| 15 | `--materialize-template` | `gate/main.go:134`; lib template bridge | delete |
| 16 | `internal/cli/shim.go` (`runShim`, `childEnv`) | `gate/cli.go:389` default arm; `routeShim` produced by no route (`cli.go:81-88`) | delete |
| 17 | `routeShim` const + default arm | `internal/cli/cli.go:25,389` | delete |
| 18 | tracker lib stamp | `tracker-card-gate.sh:48`; `gate/shared/hookactuator.go:46,144-149` | delete |
| 19 | `scripts/vendor-deps.sh` | `.gitignore:4-6`; only writer of `lib/_vendor` | delete |
| 20 | `tests/test_env_scaffold.py` (53 methods) | copies `ROOT/lib` then imports `home/lib/_internal` (`:62-67,94-95`) — S7a proved the ModuleNotFoundError | retire |
| 21 | `tests/validate.sh:6-12` | `py_compile lib/_internal/*.py`, `bash -n lib/*.sh` | remove |
| 22 | `tests/_blackbox.py:57` | `"lib"` in `_INSTALL_ENTRIES` | drop entry |
| 23 | 28× `(home / "lib").unlink()` | fixture drops the repo lib symlink | `missing_ok=True` |

## Classification — gate `main` production files

Compiled against the surviving flag surface. A file is **deleted** when its only
caller was a deleted flag; a *shared core* under `gate/shared/` is **kept** because
the root module imports it (`replace ai-specs.dev/worktree-gate => …` in
`go.mod`; `internal/sync` calls `shared.*` in production).

| file | only consumer | verdict |
|---|---|---|
| `bindings.go` | `--resolve-bindings` | deleted |
| `witness_write.go` | `--resolve-bindings` (witness write) | deleted |
| `mergeconfig.go` | `--plan-merge-config` | deleted |
| `recipeconfigwrite.go` | `--write-recipe-config` | deleted |
| `primitiveconflicts.go` | `--resolve-primitive-conflicts` | deleted |
| `tagconflicts.go` | `--resolve-tag-conflicts` | deleted |
| `reconcilestamps.go` | `--plan-reconcile-stamps` | deleted |
| `resolved_config.go` | `--plan-resolved-config` | deleted |
| `recipe_toml.go` | the four catalog flags above | deleted |
| `templateactuator.go` | `--materialize-template` | deleted |
| `hookgateactuator.go` | `--materialize-hook` | deleted |
| `gate/shared/*` (all cores) | `internal/sync` in production | **kept** |

`gate/shared/recipe_toml.go` **stays**: `internal/sync/conflicts_core.go:15` and
`internal/sync/materialize.go:235` call the two loaders in production, so its two
`runRecipeConfigPython` seams are replaced by in-process parsing (S3). Once that
lands, `runRecipeConfigPython` / `recipeConfigInterpreter` /
`classifyRecipeConfigParserError` / `recipeConfigCappedBuffer` have zero callers and
are deleted from `shared/recipeconfigwrite.go`, as are `manifestInterpreter` /
`classifyManifestParserError` / `cappedBuffer` in `ledger_reconcile.go`.

## Classification — gate `*_test.go`

| test file | classification | verdict | surviving coverage |
|---|---|---|---|
| `bindings_test.go` | flag-driven (`--resolve-bindings`, wrappers) | delete | `internal/sync/bindings_test.go` (7 tests) |
| `classify_test.go` | flag-driven → retargeted to `shared.RunPlanClassify` | **keep** | itself + `shared/classify_test.go` |
| `copyapply_test.go` | shared-core (`shared.RunApplyCopy`) | keep | itself + `internal/sync/copyapply_test.go` |
| `hookgateactuator_test.go` | flag-driven actuator wrapper | delete | `internal/sync/hookactuator_test.go` (4) + `internal/sync/templatedocs_test.go`; tracker-homes pin relocated to `shared/hookactuator_test.go` |
| `lockwrite_test.go` | shared-core (`shared.RunWriteLock`, `shared.RenderLock`) | keep | itself + `internal/sync` lock tests |
| `mergeconfig_test.go` | flag-driven | delete | `internal/sync/mergeconfig_test.go` (8) |
| `orphans_apply_test.go` | flag-driven helper → retargeted to `shared.RunApplyOrphans` | **keep** | itself |
| `orphans_plan_test.go` | flag-driven helper → retargeted to `shared.RunPlanOrphans` | **keep** | itself |
| `primitiveconflicts_test.go` | flag-driven + loader tests | delete | `shared/primitiveconflicts_test.go`; loaders → `shared/recipe_toml_test.go` |
| `recipe_toml_test.go` | package-main loader wrappers | delete | `shared/recipe_toml_test.go` (new) |
| `recipeconfigwrite_test.go` | flag-driven wrapper | delete | `shared/recipeconfigwrite_test.go` (15) + `internal/sync/materialize_test.go` |
| `reconcilestamps_test.go` | flag-driven | delete | `shared/reconcilestamp_test.go` + `internal/sync/stamps_test.go` |
| `resolved_config_test.go` | flag-driven wrapper | delete | `internal/sync/resolvedconfig_test.go` (10) |
| `tagconflicts_test.go` | flag-driven + loader tests | delete | `shared/tagconflicts_test.go`; loaders → `shared/recipe_toml_test.go` |
| `templateactuator_test.go` | flag-driven wrapper | delete | `internal/sync/templatedocs_test.go` |
| `witness_write_test.go` | flag-driven wrapper | delete | `shared/witness_test.go` + `internal/sync` tracker witness tests |
| all other gate tests | survive untouched | keep | — |

## Classification — remaining `lib/` references in `tests/`

| reference | class | disposition |
|---|---|---|
| `_blackbox.py:57` `"lib"` entry | live (symlink source) | drop the entry |
| 28× `(home / "lib").unlink()` | live (fixture drop) | `missing_ok=True` |
| `test_env_scaffold.py` | live (copy + import) | retire the file |
| `test_sync_pipeline.py:2049` docstring | stale prose | reword (the method already drives the native binary) |
| `test_upgrade_mode_dirt.py:358` `AI_SPECS_SKIP_TUI_DEPS` | inert env | remove |
| `test_tracker_card_gate_hook.py:125`, `test_tracker_ledger_host.py:129` | stamp consumer | remove the `__TRACKER_LIB_INTERNAL__` replace |
| `test_git_pr_flow_recipe.py:212`, `test_docs_drift_fixes.py:130,155` | negative guard / synthetic fixture | keep (asserts absence, builds its own tree) |
| `test_install_*` guards | negative guards over `install.sh` | keep |
| `tests/parity/parity.py` | negative guards + N10/N11/N12 | keep N10/N11/N12 verbatim |
| `tests/fixtures/init-native/golden.json:87` | recorder stderr, never asserted (`test_init_native.py:371-378` compares returncode+stdout only) | keep |
| `tests/evals/**` prompt/doc text | prose | keep |

## `internal/target` usage text

`internal/target/resolve.go:35` `programName = "lib/_internal/target-resolve.py"` is
printed by `Run` only on an argv arity error (`resolve.go:334`). `target.Run` has one
production caller (`internal/sync/sync.go:438`) that always passes one argument, so
the branch is reachable only from tests. The string is pinned by the frozen
differential oracle `internal/target/testdata/target_golden.json:130,135` and by
`internal/target/differential_test.go:525`, and the oracle is a recorded capture of
the legacy CLI ("captured once with each case's scratch base normalized to `<TMP>`. No
Python runs here."). Disposition: **left as recorded evidence**, reported per S10.

## Step order

1. Baseline build + `./tests/validate.sh` (rc + count recorded). ✔
2. Consumers tolerant: `_blackbox.py`, the 28 unlink sites, retire
   `test_env_scaffold.py`, stamp consumers, stale prose, inert env.
3. Gate source: remove the 14 flags + `main.go` arms + `routeShim`/`runShim`; delete
   the 11 wrapper files; RED→GREEN the `shared/recipe_toml.go` seams (python3 shim
   first on PATH); delete the dead subprocess helpers.
4. `rm -rf lib/` (plain `rm`, unstaged).
5. `scripts/vendor-deps.sh`, `.gitignore`, `tests/validate.sh`, workflow check.
6. Gate test classification cleanup + the two relocated shared tests.
7. Rebuild gate + dist, regenerate SHA256SUMS, verify with
   `scripts/verify-gate-sums.sh`.
8. Full validation + the three final greps.

## Result (executed)

Deletions: `lib/` 838 tracked files (19 top-level `*.sh`, `lib/_internal` 44, `lib/_vendor`
775) removed from the working tree as unstaged deletions. Gate `main` lost 11 wrapper
files and 14 flags (15 `fs.Bool` register calls including `--write-witness`); 11
flag-driven test files were deleted and 3 kept files were retargeted to the shared
entry points (`shared.RunPlanClassify`, `shared.RunPlanOrphans`,
`shared.RunApplyOrphans`) instead of the retired flags. Two helper funcs
(`assertFileBytes`, `assertNoTempFiles`) moved to `lockwrite_test.go`, the only
keeper that needs them.

Seams: RED observed with a failing `python3` shim first on `PATH`
(`LoadRecipeTagMetadata: the manifest is not valid TOML (parser exit status 127)`),
GREEN with both loaders parsing in process through `shared/tomlparse.go`. The
subprocess seam (`runRecipeConfigPython` + interpreter/classify/capped-buffer
helpers) and the gate main `manifestInterpreter` / `classifyManifestParserError` /
`cappedBuffer` are deleted: no production Go path resolves an interpreter.

Stamp: removed from `tracker-card-gate.sh` (line + doc line),
`shared/hookactuator.go` (const + substitution arm), the two Python stamp
consumers, and `internal/sync/hookactuator_test.go`'s fixture. The pin moved to
`gate/shared/hookactuator_test.go` (`TestRenderHookGateContentTrackerHomes` plus a
test proving the retired token is left untouched).

Shim: `internal/cli/shim.go` + `shim_seam_test.go` deleted, `routeShim` and the
`default: runShim` arm removed after proving `childEnv` had no other caller and no
route produced `routeShim`.

Also retired: `D-U2d-9`'s `expectedFailure` in `tests/test_recipe_materialize.py`
(the in-process reader now names the recipe: `recipe nonexistent: recipe.toml not
found or not parseable`).

Evidence: `bash scripts/build-gate.sh` rc=0 (zero warnings);
`scripts/verify-gate-sums.sh` rc=0 (4 digests match the regenerated trust root);
gate `go test ./...` rc=0; root `go build`/`go vet` rc=0; parity
`gate-absent failing=0, gate-present failing=0 — PASS`; Python discover
`Ran 1757 tests` = 1810 − 53 (`test_env_scaffold.py`).

## Open decision (blocks rc=0)

`internal/sync/TestMaterializeHookFrozenDifferential` fails on
`fixture layout diverges`: the frozen oracle
`internal/sync/testdata/hookactuator_golden.json` pins `hook_sha256` of the
8-placeholder hook source (`f2c83114…` = sha256 of the source WITH
`# internal=__TRACKER_LIB_INTERNAL__`). Removing the stamp changes that string, so
32 hash fields in the oracle derive from a source it no longer describes
(1 `fixture.hook_sha256`, 3 `input.managed_sha`, 6 `input.seed_sha256`, 4
`outcome.record.sha256`, 9 `outcome.disk_sha`, 9 `outcome.backup_sha`). The oracle's
own note says
"Regenerating it needs a human decision", and the PR4a S4 spec deliberately makes
hook-source drift a hard failure, so this slice does not regenerate it
unilaterally. Every other check is green.

## L5 — corpus approval

Robert, 2026-11-13 (relayed by the parent): **approved the hash-only regeneration of
`internal/sync/testdata/hookactuator_golden.json` for the 7-placeholder hook source**
(Option A). `internal/sync/testdata/hookactuator_golden.json` is added to the allowed
edit surfaces; no other file outside the original list is touched.

## Golden regeneration — narrowness proof

Two distinct source values feed all 32 frozen hash fields:

- `sha(old hook source)` `f2c83114…` → `2cf09794…` (1 field: `fixture.hook_sha256`)
- `sha(old rendered bytes)` `b455e21f…` → `18151817…` (12 fields)

The other 19 hash fields hash fixture text that never contained the retired token
(`h4`'s `local`/`other`, `h5`'s `stale`/`stale`, `h6`/`h7`'s `prior`, and the empty
`disk_sha`/`backup_sha` of the refused cases), so they are byte-identical before and
after by construction.

Before/after structural diff over the parsed JSON: **13 changed fields, nothing else.**

```
.fixture.hook_sha256                    f2c83114… -> 2cf09794…
.cases[0].outcome.record.sha256         b455e21f… -> 18151817…
.cases[0].outcome.disk_sha              b455e21f… -> 18151817…
.cases[1].input.managed_sha             b455e21f… -> 18151817…
.cases[1].input.seed_sha256             b455e21f… -> 18151817…
.cases[1].outcome.record.sha256         b455e21f… -> 18151817…
.cases[1].outcome.disk_sha              b455e21f… -> 18151817…
.cases[2].input.seed_sha256             b455e21f… -> 18151817…
.cases[2].outcome.disk_sha              b455e21f… -> 18151817…
.cases[4].outcome.record.sha256         b455e21f… -> 18151817…
.cases[4].outcome.disk_sha              b455e21f… -> 18151817…
.cases[5].outcome.record.sha256         b455e21f… -> 18151817…
.cases[5].outcome.disk_sha              b455e21f… -> 18151817…
```

Unchanged: the case matrix (9 cases, same order and names), every `input` field other
than the three hashes above, every `outcome.lines`, `mode`, `refused`, `wrote`, `dest`,
`backup`, the oracle block and the file's formatting. `go test ./internal/sync/ -run
TestMaterializeHook`: rc=0 — `TestMaterializeHookFrozenDifferential`,
`…StateParity`, `…RefusalsAndBoundary`, `…RefreshRollback` all PASS.

## Final validation

- `bash scripts/build-gate.sh`: rc=0, zero warnings, 4 targets.
- `cd dist && shasum -a 256 <the four assets>` + `scripts/verify-gate-sums.sh`:
  rc=0 — "ok — 4 digest entries match the committed trust root".
- `./tests/validate.sh`: **rc=0**.
  - parity: `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`
    (35 cases, every line `zero deltas`).
  - Python: `Ran 1757 tests … OK (skipped=1, expected failures=2)`.
  - Go: gate `ok` (3 packages), root `ok` for every `./internal/...` package; zero
    `FAIL:`/`ERROR:` lines.
  - Discover arithmetic: 1810 (L5 baseline) − 53 methods of the retired
    `tests/test_env_scaffold.py` = **1757**, and nothing else was removed. The two
    remaining expected failures are `tests/test_init_brief_route.py`'s T5-expiring
    seam rows (they still fail by design); `tests/test_recipe_materialize.py`'s
    D-U2d-9 row now passes, so its marker was retired (a pass, not a removal).

**S9 is satisfied.**

## Final greps

- (a) `grep -rn "python3" install.sh bin/ cmd/ internal/ catalog/…/gate` (non-test,
  non-comment): **empty** — every remaining hit is a doc comment. The only
  `exec.LookPath("python3")` calls left are in three *test* files
  (`gate/tokenize_diff_test.go`, `gate/ledger/identity_test.go`,
  `internal/toml/toml_test.go`) that use an interpreter only as an optional
  differential oracle and skip without it. No production path resolves an interpreter.
- (b) `grep -rn "copytree.*lib|sys.path.insert.*lib|import lib|from lib\." tests/
  scripts/`: **empty** — no live `lib/` consumer remains; the surviving mentions are
  docstrings and negative guards.
- (c) `git ls-files --deleted lib/` = 838 and `lib/` is absent on disk;
  `git ls-files lib/` still prints 838 only because the deletions are intentionally
  **unstaged** for the parent to stage.

## L6 — chained reviewable commits

Robert, 2026-11-13 (relayed by the parent): the native reviewer could not ingest
PR4b even as the logic-only commit — `lens_context_budget_exceeded` twice, on the
full candidate and on the logic-commit range `f8852e47..309b101f`. **Robert chose to
split the work into a chained sequence of smaller reviewable commits**, each under the
native budget, with the FINAL TREE kept byte-identical to the validated tree.

Regrouping only: no file content changed. Backup ref `pr4b-final-backup` → `309b101f`
(tree `c537f90f`) is never deleted.

### L6b — C3 split further

The first chain's C3 (`7fe2bc2c`, "retire the gate bridge surface") was **also
rejected** with `lens_context_budget_exceeded` (41 paths). Robert requires every commit
to be natively reviewable, so C3 was split into three sub-commits on top of C2
(`66131d46`), regrouping its 41 paths only. C1 and C2 were accepted unchanged.

The sub-commits each carry their OWN `bin/SHA256SUMS`, regenerated from the gate built
at that tree (`bash scripts/build-gate.sh`, go1.24.13, zero warnings, digests via the
file's canonical command, checked with `scripts/verify-gate-sums.sh` rc=0). C3c's
digests are the final ones, and its four-asset build is byte-identical to the final
gate build — proof that C3c's gate source equals the final source.

Because the sums are per-tree, C3a and C3b carry digest lines that the final tree no
longer has; C3c restores the final file byte-for-byte. That is the only content that is
new in this regrouping.

### Final chain

| commit | subject | scope | verification on that tree |
|---|---|---|---|
| `b690ac27` | `test: tolerate the absent lib tree across fixtures and scripts` | 34 files (32 M, 2 D) | 29 touched modules: `Ran 852 tests … OK` rc=0 |
| `66131d46` | `feat!: remove the unreachable cli shim route` | 3 files (1 M, 2 D) | `go build ./...` rc=0; `go test ./internal/cli/` rc=0 |
| `f5f77db4` | `feat!: remove the gate bridge flags and flag-only wrappers` | 28 files (27 paths + `SHA256SUMS`): 1 M, 22 D, 4 retargeted tests | gate `build`/`vet`/`test` rc=0; `scripts/build-gate.sh` rc=0 (0 warnings); `verify-gate-sums.sh` rc=0; `go test ./internal/sync/` rc=0 (old hook oracle still valid — the stamp is untouched here); `tests/parity/run.py` rc=0 |
| `5ef4c241` | `feat!: parse gate recipe TOML in process, exit the interpreter` | 8 files (7 paths + `SHA256SUMS`) | gate `go test ./...` rc=0; `build-gate` rc=0 (0 warnings); `verify-gate-sums.sh` rc=0; `go test ./internal/sync/` rc=0; parity rc=0 |
| `e720039d` | `feat!: drop the __TRACKER_LIB_INTERNAL__ stamp from the tracker hook` | 7 files (6 paths + `SHA256SUMS`) | gate `go test ./...` rc=0; `build-gate` rc=0 with digests identical to the final build; `verify-gate-sums.sh` rc=0; `go test ./internal/sync/` rc=0 (frozen hook differential green against the regenerated oracle); parity rc=0 |
| `cfb99219`→C4' | `feat!: delete the Python lib tree` | 839 files (838 D `lib/`, 1 A this doc) | `./tests/validate.sh` rc=0 on the identical tree (parity `gate-absent failing=0, gate-present failing=0 — PASS`; `Ran 1757 tests … OK`) |

`git diff --shortstat 66131d46 e720039d` is `41 files changed, 659 insertions(+),
6202 deletions(-)` — identical to the old single C3 (`7fe2bc2c`), and C3c's tree hash
equals C3's, so the split is provably a pure regrouping.

### One file moved between sub-commits

`internal/sync/syncagent_materialize_test.go` moved from the stamp sub-commit (C3c) to
the interpreter sub-commit (C3b). Its assertion is that the `python3` stub is NEVER
invoked; that becomes true when the recipe.toml readers stop spawning an interpreter,
not when the stamp is dropped. Measured on the C3b tree without the move: the stub log
no longer exists and the unmodified assertion fails with
`stub log: open …/invocations.log: no such file or directory`. With the move, C3b is
green and no other file needed reassignment.

### Byte identity

`git diff pr4b-final-backup C4' -- . ':(exclude)odd/tasks/go-20-u2e-t5-pr4b-atomic-delete.md'`
is EMPTY, and the only doc change is this log section. No file content was edited in
this turn beyond per-sub-commit `SHA256SUMS`.

### L6c — C3a re-split for reviewability

The logic commit `f5f77db4` (old C3a) ALSO exceeded the native reviewer budget: the
reviewer evidence carries the FULL CONTENT of every touched path, deleted files
included, so deletion-heavy candidates blow it regardless of their path count.

Robert's disposition, 2026-11-13: **logic commits get native review; pure-deletion
batches that do not fit stay explicitly unreviewed by his recorded disposition.** The
table above is therefore superseded by this split, which isolates the reviewable logic
from the deletion batches:

| commit | subject | files | verification |
|---|---|---|---|
| `a2e7e9aa` | `test: retire the gate bridge flag tests ahead of the flag removal` (pure deletion) | 12 (11 test files + `lockwrite_test.go`) | gate `go test ./...` rc=0; `build-gate` rc=0 (0 warnings); `verify-gate-sums.sh` rc=0 against the unchanged C2 sums |
| `69e88afb` | `feat!: remove the gate bridge flags from the dispatch surface` (LOGIC) | 5 (4 paths + own `SHA256SUMS`) | gate build/vet/test rc=0; `build-gate` rc=0 (0 warnings); `verify-gate-sums.sh` rc=0; `go test ./internal/sync/` rc=0; parity rc=0 (0/0) |
| `8bdb457b` | `feat!: delete the gate bridge flag-only wrappers` (pure deletion) | 12 (11 wrappers + `SHA256SUMS`) | gate build/vet/test rc=0; `build-gate` rc=0 (0 warnings); `verify-gate-sums.sh` rc=0; parity rc=0 (0/0) |

`8bdb457b`'s tree is `a467251b`, byte-identical to the old C3a tree, and its four
digests are identical to the old C3a build — the split is a pure regrouping.

Two measured facts recorded here: `lockwrite_test.go` had to move into `a2e7e9aa` (the
byte-comparison helpers it uses lived in a deleted test file, so `a2e7e9aa` without it does
not compile), and deleting the unreferenced wrappers DOES change the binary bytes even
though no live path changes, so `8bdb457b` regenerates the sums.

Re-chained on top: C3b' `b0edcd86`, C3c' `ec7544f4`, C4'' (this commit). C1, C2 and the
C3b/C3c contents are unchanged; only parents, and the per-sub-commit sums, differ.
