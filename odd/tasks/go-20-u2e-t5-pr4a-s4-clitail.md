# GO-20 U2e T5 PR4a S4 — internal tail of the live `lib/` reads

Base: epic/go-single-binary 212016f. Branch: change/go-20-u2e-t5-pr4a-s4-clitail.
Goal: the last live `lib/` reads the PR #511 S4 inventory listed under `internal/` stop executing
the repository's `lib/` tree, without silent coverage loss — `internal/cli/recipe_test.go`'s
`lib/recipe-config.sh` and `lib/recipe.sh` dispatcher differentials, the caller-less legacy helpers
PR #513 had to keep, and `internal/sync/materialize_loop_test.go`'s repo-`lib/` symlink. Same
discipline and the same bundle harness as PR #513 (`skills_test.go` + `skills_*_golden.json`) and
PR #514 (`recipe_test.go` + `recipe_*_golden.json`).

## Specs

- S1: `TestConfigureRecipesDifferentialAgainstShell` (same name) no longer execs
  `bash lib/recipe-config.sh` nor probes `python3`: it replays the frozen legacy output of
  `internal/cli/testdata/recipe_config_golden.json` against the built binary. No bash, no python3,
  no `lib/` read, no environment skip — a missing bundle fails.
- S2: `TestRecipeDispatcherDifferentialAgainstShell` reads
  `internal/cli/testdata/recipe_dispatch_golden.json` the same way.
- S3: `TestConfigureRecipesMissingPathDeviation` no longer execs the shell: its legacy side is the
  bundle's `missing path` record. The native side and the recorded divergence (bash's
  `script: line N: ` prefix, ADR 0002) stay asserted exactly as before.
- S4: Every native (non-legacy) assertion of the two tables survives: the exact exit code, stdout
  and stderr in the tokenized space, the per-case input tree, the whole-table read-only snapshot,
  and the in-process `*NativeNeedsNoBashOrPython` / loop tests.
- S5: Each bundle carries a hard-coded case inventory in capture order (count, uniqueness, order),
  checked against the bundle and the in-test corpus before any comparison.
- S6: Frozen inputs equal the in-test corpus: every record pins the pre-run project tree, and the
  test compares its own fixture before running the port, so fixture drift fails as input drift.
- S7: No absolute host path leaks into a bundle: `<DIR>` (project root or working directory),
  `<HOME>` (cli home) and `<ROOT>` (the checkout, for bash's own script-name prefix) are tokenized
  in stdout, stderr and file bytes, resolved spelling included.
- S8: Negative controls: mutating a frozen result, mutating a frozen rc, deleting a case,
  duplicating a case, drifting a frozen input and dropping the bundle each fail rc=1.
- S9: Caller-less legacy helpers are retired from `internal/cli/skills_test.go` after a package-wide
  grep: `skillsOracle`, `skillsRemoveGateBinary` (with its `sync.Once` state) and
  `skillsProjectTree`. `skillsRemoveEnv` is kept — it still has callers — and its doc comment plus
  the one in `recipe_test.go` that named the deleted helper are corrected.
- S10: `internal/sync/materialize_loop_test.go`'s two `addLibSymlink` cases stop symlinking the
  repo's `lib/`: the native legs never read `<home>/lib`, so the fixture is dropped and both
  scenarios run identically with the repo `lib/` absent. No golden byte was added for them.
- S11: `gofmt -l internal/cli internal/sync` empty, `go vet ./internal/cli/ ./internal/sync/` rc=0,
  `go test ./internal/cli/ ./internal/sync/ -count=1` rc=0, the pre-existing goldens
  (`skills_*_golden.json`, `recipe_{list,add,remove,configure}_golden.json`) byte-identical.
- S12: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestConfigureRecipesDifferentialAgainstShell` (`bash lib/recipe-config.sh <args>` → `lib/_internal/config_wizard.py`, compared on rc, stdout, stderr) | FREEZE | the payload is host-independent once `<DIR>`/`<HOME>`/`<ROOT>` are tokenized: the wrapper's usage, both exit-2 usage errors, the manifest guard, and config_wizard main's TTY gate. The interactive loop is not in this table and stays in-process (`TestConfigureRecipesFlow`). 1:1 with the eight live cases plus the `missing path` record. |
| `TestRecipeDispatcherDifferentialAgainstShell` (`bash lib/recipe.sh <args>`, compared on rc, stdout, stderr) | FREEZE | only the dispatcher's OWN arms are compared (usage on stdout exit 0, bare `recipe` on stderr exit 2, the two unknown-subcommand lines exit 1, `recipe init` missing id through `lib/recipe-init.sh`) — all frozen bytes, no host path. 1:1 with the eight cases. |
| `TestConfigureRecipesMissingPathDeviation` (bash leg plus native assertions) | FREEZE (legacy side only) | the divergence itself stays a test-local assertion (the port drops bash's `lib/recipe-config.sh: line N: ` prefix by ADR 0002); the frozen record supplies the legacy rc/stdout/stderr, checked by substring. |
| `skillsOracle` (python3 ≥3.11 probe) | RETIRE | its two callers were the two live differentials above; after their freeze no test in the package execs the legacy route. |
| `skillsRemoveGateBinary` (builds the worktree gate per run) | RETIRE | caller-less before this slice too: PR #514's capture harness was temporary, so the only remaining mention was `recipe_test.go`'s doc comment (corrected here). |
| `skillsProjectTree` (twin-project snapshot helper) | RETIRE | caller-less: PR #514 replaced the twin-tree comparisons with the bundles' frozen `tree` records. |
| `skillsRemoveEnv` | KEEP | still called by four frozen tables (`skills remove`, `recipe remove`, two `recipe configure` legs). |
| `materialize_loop_test.go` `<home>/lib` → repo `lib/` symlink | DROP THE FIXTURE | the Python legs the bundle was captured from resolved delegations through `<home>/lib`; the frozen test only runs the native loop, which never reads `<home>/lib` (`internal/sync` production code only spells that path inside two bash-diagnostic format strings). Proven: with the symlink present but dangling (repo `lib/` renamed away) the test already passed, and it passes identically after the fixture is removed — so no `internal/sync/testdata/` golden was needed. |

Normalizations, documented in the test and in each bundle's `oracle.note`:

- **Tokenization.** `<DIR>` replaces each run's temporary project root and working directory,
  `<HOME>` the isolated cli home, `<ROOT>` the checkout (bash prefixes its own diagnostics with the
  script path), raw and symlink-resolved.
- **Modes.** File and directory modes are not frozen: fixtures are created with `os.WriteFile` under
  the process umask and this package runs `t.Parallel()` tests that must not have the umask pinned
  under them.

Cases: config 9 (`8` table + `missing path`), dispatch 8.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6, S7, S8 | inline | |
| T2 | S9, S10, S11, S12 | inline | |

## Log

- L1 (Robert): "a" — convert the `internal/cli` live `lib/` reads now; this slice holds what
  PR #514 surfaced.
- L2: Baseline at 212016f: the affected tests pass with no skip — cli
  `TestConfigureRecipesDifferentialAgainstShell` (8 subtests), `TestConfigureRecipesMissingPathDeviation`
  and `TestRecipeDispatcherDifferentialAgainstShell` (8 subtests) rc=0 (`logs/baseline-cli.log`),
  `TestMaterializeLoopDifferentialFrozen` rc=0 (`logs/baseline-sync.log`).
- L3: Scratch/TMPDIR: `/Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-clitail`
  (Robert's standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing at
  its `tmp/`, logs in its `logs/`.
- L4: RED before either bundle existed: all three converted tests failed rc=1 with
  `read frozen oracle recipe_{config,dispatch}_golden.json: … no such file or directory`
  (`logs/red-cli.log`).
- L5: The capture harness was temporary code (`internal/cli/zz_pr4a_capture_test.go`, gated by
  `PR4A_CAPTURE_DIR=<dir>`) and was deleted before the handoff. It ran the real legacy routes
  (`bash lib/recipe-config.sh`, `bash lib/recipe.sh`) and froze each case's pre-run tree and output;
  a dangling repo `lib/` is not needed by it.
- L6: Captured twice into fresh scratch dirs and `cmp` byte-identical for both bundles: config
  4275 B / 9 cases, dispatch 3981 B / 8 cases (`logs/capture1.log`, `logs/capture2.log`, `cmp`
  rc=0). Oracle metadata `python: 3.14.7`, `base: 212016f`, script blob ids
  `recipe-config.sh 8cf00951…`, `_internal/config_wizard.py 9f949007…`, `recipe.sh 35d0f3ea…`,
  `recipe-init.sh 963de7aa…`. Host-path grep over both bundles: 0.
- L7: GREEN on the first replay (`logs/green-cli.log`, rc=0: 16 subtests + the deviation case).
- L8: Negative controls, each rc=1 with the bundle restored and `cmp`-verified afterwards: config —
  mutate a frozen stdout, drop a case, duplicate a case, drift a frozen input, break the oracle blob
  id; dispatch — mutate a frozen stdout, mutate a frozen rc, drop a case, duplicate a case, break
  the oracle blob id; plus the removed-bundle case (`logs/neg-*.log`).
- L9: Independence: with the whole `lib/` directory temporarily renamed, the affected cli and sync
  tests PASS — `TestConfigureRecipes*`, `TestRecipeDispatcher*` and
  `TestMaterializeLoopDifferentialFrozen` (`logs/libabsent-independence.log`, rc=0, 0 SKIP);
  `git status --short lib/` empty afterwards.
- L10: Helper retirement (grep-proven caller-less): `skillsOracle`, `skillsRemoveGateBinary`
  (+ `skillsRemoveGateOnce/Path/Err`) and `skillsProjectTree` deleted, `fmt`/`os/exec`/`sync`
  imports dropped, and the two doc comments that named `skillsRemoveGateBinary` corrected (the
  `skillsRemoveEnv` one in `skills_test.go`, the `recipe remove` capture note in `recipe_test.go`).
  `skillsRemoveEnv` stays for its four callers.
- L11: `materialize_loop_test.go`: the `addLibSymlink` field, the symlink branch and the two
  `addLibSymlink: true` scenario rows removed; `materializeLoopWriteStatic`'s doc comment records
  the decision (the native legs never read `<home>/lib`; the repo `lib/` was only needed by the
  Python legs already frozen into `materialize_golden.json`). No `internal/sync/testdata/` change.
- L12: `gofmt -l internal/cli internal/sync` empty, `go vet ./internal/cli/ ./internal/sync/` rc=0
  (`logs/vet.log`), `go test ./internal/cli/ ./internal/sync/ -count=1` rc=0
  (`logs/pkg-tests.log`). Under `umask 077` the cli package is rc=0 and the sync package fails only
  on `TestGitignoreDifferential` / `TestHooksRenderDifferential`, which fail identically at base
  212016f under the same umask (`logs/base-umask077-sync.log`, 94 mode diffs) — pre-existing and
  unrelated; `TestMaterializeLoopDifferentialFrozen` alone is rc=0 under `umask 077`
  (`logs/sync-umask077-focused.log`).
- L13: The four `skills_*_golden.json` and the four `recipe_{list,add,remove,configure}_golden.json`
  bundles are byte-identical (`git diff --quiet internal/cli/testdata` rc=0).
- L14: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/build.log`; required by
  `TestUpgradeUsageMatchesTheShellSurface`), `./tests/validate.sh` rc=0
  (`logs/validate.log`; `Ran 2345 tests … OK (skipped=165, expected failures=3)`,
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`).
- L15: Final inventory `grep -rn '"lib"\|lib/\|exec.Command("python3"\|exec.Command("bash"'
  internal/ --include=*_test.go` (`logs/final-inventory.log`, 196 hits, 161 of them comment lines,
  6 `exec.` lines). **No live repo-`lib/` read or exec remains anywhere under `internal/`.** The six
  `exec.` hits are classified: `internal/toml/toml_test.go:39,288,373` run inline `python3 -c`
  reference oracles for the TOML reader (no `lib/`); `internal/sync/vendorskills_test.go:516` and
  `internal/sync/spine_test.go:811` prove a temp-dir `python3` shim is on PATH and logs (a
  non-vacuity probe for a "no python3 spawned" guard); `internal/sync/hooksrender_test.go:1154` runs
  a `bash` wrapper written into a temp dir. Every other hit is a provenance comment, an assertion
  string, or a fixture path inside a temp home. The one remaining `runOne("bash", …)` in
  `internal/cli` execs `bin/ai-specs`, not `lib/` (`upgrade_route_test.go`, out of this slice).
