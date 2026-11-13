# GO-20 U2e T5 PR4a S4 — internal/cli recipe live oracles

Base: epic/go-single-binary ac49e9f. Branch: change/go-20-u2e-t5-pr4a-s4-clirecipe.
Goal: `internal/cli/recipe_test.go` stops exec'ing the legacy `lib/recipe-*.sh` wrappers and
`python3` for its four frozen families without silent coverage loss — the last `lib/` reads the
S4 inventory of PR #511 listed for this file. Same discipline and the same bundle harness as
PR #513 (`internal/cli/skills_test.go` + `testdata/skills_{list,remove,add}_golden.json`), and as
PR #496 (doctor), #497 (rulesaudit), #498 (skills), #499 (conflicts), #501 (briefpolicy),
#502 (checksync), #503 (envscaffold), #504 (refreshbundled), #505 (vendorskills), #511 (platform.sh),
#512 (sync-agent.sh).

## Specs

- S1: `TestRecipeListDifferentialAgainstShell` (same name) no longer execs `bash lib/recipe-list.sh`
  or probes `python3`: it replays the frozen legacy output of `internal/cli/testdata/recipe_list_golden.json`
  against the built binary. No bash, no python3, no `lib/` read, no environment skip — a missing
  bundle fails.
- S2: `TestRecipeAddDifferentialAgainstShell` reads `testdata/recipe_add_golden.json` the same way.
- S3: `TestRecipeRemoveDifferentialAgainstShell` reads `testdata/recipe_remove_golden.json`.
- S4: `TestRecipeConfigureDifferentialAgainstScript` reads `testdata/recipe_configure_golden.json`;
  so does `TestRecipeConfigureSyncLegDifferential`, whose legacy side is the same
  `bash lib/recipe.sh configure` route over the stub front door.
- S5: The four `*MissingPathDeviation` tests no longer exec the shell either: their legacy side is
  the frozen `missing path` record of the family's bundle (list, add, remove). The native side and
  the recorded divergence (bash's `script: line N: ` prefix, ADR 0002) stay asserted in the test
  exactly as before.
- S6: Every native (non-legacy) assertion of the four tables survives: the exact exit code, stdout
  and stderr in the tokenized space, the produced project tree, the untouched-project assertion for
  the pre-write refusals, the `lockKept` assertion for remove, the read-only CLI home snapshot, the
  `COLUMNS=60` help case, the sync leg's front-door spawn contract and its `wantContent` report
  assertions, and the in-process `*NativeNeedsNoBashOrPython` / tail / loop / guidance tests.
- S7: Each bundle carries a hard-coded case inventory in capture order (count, uniqueness, order),
  checked against both the bundle and the in-test corpus before any comparison. A bundle that drops,
  duplicates or reorders a case fails as an inventory error.
- S8: Frozen inputs equal the in-test corpus: every record pins the pre-run project tree (paths plus
  bytes, snapshotted before the run), and the test compares its own fixture before running the port,
  so fixture drift fails as input drift.
- S9: No absolute host path leaks into a bundle: `<DIR>` (project root), `<HOME>` (cli home) and
  `<ROOT>` (the checkout, for bash's own script-name prefix and Python's own script paths) are
  tokenized in stdout, stderr and file bytes, resolved spelling included.
  `grep -c "/Users/\|/private/\|/var/folders"` over the four bundles is 0.
- S10: Negative controls: mutating one frozen result, deleting a case, duplicating a case and
  drifting a frozen input each fail rc=1 in every family.
- S11: `gofmt -l internal/cli` empty, `go vet ./internal/cli/` rc=0, the converted tests free of live
  `lib/`/`python3` execution (comments only), `go test ./internal/cli/ -count=1` rc=0 (also under
  `umask 077`).
- S12: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestRecipeListDifferentialAgainstShell` (`bash lib/recipe-list.sh <args>` vs `recipe list`, compared on rc, stdout, stderr) | FREEZE | the payload is host-independent once `<DIR>`/`<HOME>` are tokenized: the listing prints recipe ids, names and the enabled/available sections. It is the only oracle for the four list sections. 1:1 with the live table's ten cases plus the `missing path` record. |
| `TestRecipeAddDifferentialAgainstShell` (`bash lib/recipe-add.sh` vs `recipe add`, compared on rc, stdout, stderr and the twin trees) | FREEZE | stdout is the success line, the "next sync will materialize" listing, the placeholder block and the Next steps guidance, all frozen as bytes and tokenized; the tree carries the appended `[recipes.<id>]` segment and the untouched manifests of the refusal cases. 1:1 with the sixteen cases plus `missing path`. |
| `TestRecipeRemoveDifferentialAgainstShell` (`bash lib/recipe-remove.sh` vs `recipe remove`, compared on rc, stdout, stderr and the twin trees) | FREEZE | the tree carries the segment surgery AND the lock cleanup (`recipe-remove.sh:167-203`), both frozen as bytes; the `lock kept` case is asserted natively as before. 1:1 with the twelve cases plus `missing path`. |
| `TestRecipeConfigureDifferentialAgainstScript` (`bash lib/recipe.sh configure` → `lib/_internal/recipe-configure.py` vs `recipe configure`, compared on rc, stdout, stderr and the twin trees) | FREEZE | the inspect document, the reports and every guard message are frozen bytes; the manifest writes are frozen in the tree. 1:1 with the thirty cases. |
| `TestRecipeConfigureSyncLegDifferential` (`bash lib/recipe.sh configure … --sync` over a stub `<home>/bin/ai-specs` front door vs the native leg) | FREEZE (legacy side only) | the stub makes the legacy output deterministic, so the legacy rc/stdout/stderr are a frozen record and the spawn-contract log plus the report assertions stay native. 1:1 with the five cases. |
| `TestRecipe{List,Add,Remove}MissingPathDeviation` (each ran a bash leg plus native assertions) | FREEZE (legacy side only) | the divergence itself stays a test-local assertion (the port drops bash's `lib/recipe-*.sh: line N: ` prefix by ADR 0002); the frozen record supplies the legacy rc/stdout/stderr. |

Normalizations, documented in the test and in each bundle's `oracle.note`:

- **Tokenization.** `<DIR>` replaces each run's temporary project root, `<HOME>` the isolated cli
  home (which carries the fixture catalogue and the stub front door), `<ROOT>` the checkout (bash
  prefixes its diagnostics with the script path, Python its tracebacks), raw and symlink-resolved.
- **Guard detail.** The two valid→invalid refusals embed the parser's own error detail
  (`tomllib`'s vs Go's): both sides normalize it to `<parse detail>`, exactly the regexes the live
  twin comparisons already used. The message's framing — the id, "was not modified", "would produce
  invalid TOML:" — stays byte-compared.
- **Modes.** File and directory modes are **not** frozen: fixtures are created with `os.WriteFile`
  under the process umask and this package also runs `t.Parallel()` tests that must not have the
  umask pinned under them.
- **Repo catalogue.** The list family's full-home cases read the repository's own catalogue through
  the home symlink (the shape `tests/_blackbox.py:isolated_home` builds), exactly like the frozen
  skills list bundle: the frozen rows are the catalogue's rows at this base.

Shapes the live tables never exercised stay untouched, and the helpers other files use are reused
read-only: `skillsLoadGolden`, `skillsCheckCase`, `skillsTokenSet`, `skillsCompareNodes` and
`skillsTreeSnapshot` (all in `skills_test.go`, still needed by its own frozen tables). The legacy
helpers `skillsOracle`, `skillsRemoveGateBinary`, `skillsRemoveEnv` and `skillsProjectTree` stay
defined because `recipe-config.sh`'s and the `recipe.sh` dispatcher's differentials still run live
shell legs (reported as the remaining follow-up); no converted test in this file calls them.

Cases: list 11 (`10` table + `missing path`), add 17 (`16` + `missing path`), remove 13 (`12` +
`missing path`), configure 30 + 5 sync-leg records.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6, S7, S8, S9, S10 | inline | |
| T2 | S11, S12 | inline | |

## Log

- L1 (Robert): "a" — convert the `internal/cli` live `lib/` reads now, one PR per file, same cycle.
- L2: Baseline at ac49e9f: the eight affected tests pass with no skip (`logs/01-baseline-recipe.log`,
  rc=0, 82 PASS / 0 SKIP), after `go build -o dist/ai-specs ./cmd/ai-specs` (`logs/00-build.log`,
  rc=0).
- L3: Scratch/TMPDIR: `/Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-clirecipe`
  (Robert's standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing at
  its `tmp/`, logs in its `logs/`.
- L4: RED before any bundle existed: all eight converted tests failed rc=1 with
  `read frozen oracle recipe_<family>_golden.json: … no such file or directory`
  (`logs/04-red-missing-bundles.log`).
- L5: The capture harness was temporary code in `internal/cli/recipe_test.go` (gated by
  `CLI_RECIPE_CAPTURE=<dir>`, reusing the test's own corpus builders so the captured argv and the
  asserted argv cannot drift) and was deleted before the handoff. It ran the real legacy routes — the
  remove/configure legs with `WORKTREE_GATE_BIN` pinned to a freshly built gate, the configure leg's
  home plus the `lib` symlink the shell arm resolves, the sync leg over the same stub front door — and
  froze each case's pre-run tree and output. It failed if an absolute host path survived tokenization.
- L6: Captured twice into fresh scratch dirs and `cmp` byte-identical for all four bundles:
  list 8783 B / 11 cases, add 11516 B / 17, remove 12943 B / 14, configure 67878 B / 35 (30 table +
  5 sync-leg) (`logs/05-capture-a.log`, `logs/06-capture-b.log`, `logs/07-capture-cmp.log`, rc=0).
  Oracle metadata `python: 3.14.7`, `base: ac49e9f`, script blob ids
  `recipe-list.sh 08623859…`, `recipe-add.sh 2079d4d1…`, `recipe-remove.sh 73bfa6e0…`,
  `recipe.sh 35d0f3ea…`, `recipe-configure.py 01c4eec9…`. Host-path grep over the four bundles: 0.
- L7: GREEN on the first replay (`logs/08-green-families.log`, rc=0, 82 PASS / 0 SKIP).
- L8: Negative controls, each rc=1 with the bundle restored and `cmp`-verified afterwards, in every
  family: mutate one frozen result (stdout mismatch), delete a case (`N records, want N+1`),
  duplicate a case (record count), and drift a frozen input (`project input: node … differs`)
  (`logs/09-negative-controls.log`, `logs/nc-<family>-<control>.log`).
- L9: Independence: with `lib/recipe-list.sh`, `lib/recipe-add.sh`, `lib/recipe-remove.sh`,
  `lib/recipe.sh` and `lib/_internal/recipe-configure.py` temporarily renamed away, the eight
  converted tests PASS — 82 PASS / 0 SKIP (`logs/10-independence-renamed-lib.log`, rc=0);
  `git status --short lib/` empty afterwards.
- L10: Helper retirement was NOT performed: `skillsOracle` still has two live callers
  (`lib/recipe-config.sh`'s differential and the `lib/recipe.sh` dispatcher differential, both
  outside this unit's four families), `skillsRemoveEnv` and `skillsProjectTree` are still called, and
  `skillsRemoveGateBinary` loses its last caller only because the capture harness that used it was
  temporary — retiring it would also require correcting the sentence in `skillsRemoveEnv`'s doc
  comment that names it, which the edit surface excludes. `internal/cli/skills_test.go` is
  therefore byte-identical.
- L11: `gofmt -l internal/cli` empty, `go vet ./internal/cli/` rc=0; `go test ./internal/cli/
  -count=1` rc=0 (`logs/12-package-tests.log`) and the same under `umask 077` rc=0
  (`logs/13-green-umask077.log`); the three `skills_*_golden.json` bundles are byte-identical.
- L12: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/14-build.log`),
  `./tests/validate.sh` rc=0 (`logs/15-validate.log`).
- L13: Size: `internal/cli/recipe_test.go` ends at +610/−542. The ≈700-changed-line ordering rule was
  held as the fallback plan; it was not needed because each family completed and replayed green
  before the next one started, so no family is half-converted. If the reviewer needs a smaller
  slice, `list` → `add` → `remove` → `configure` is a valid chained split (each bundle is
  independent).
- L14: Final inventory `grep -rn 'exec.Command("python3"\|exec.Command("bash"\|"lib"\|lib/_internal'
  internal/ --include=*_test.go` (`logs/16-grep-inventory.log`, 79 hits): two live repo-lib reads
  remain in this file — `lib/recipe-config.sh` (its differential and its missing-path deviation) and
  the `lib/recipe.sh` dispatcher differential, both deferred families — plus
  `internal/sync/materialize_loop_test.go`'s `addLibSymlink` cases (outside this unit). Every other
  hit is a provenance comment, a fixture/stub written into a temp home, an assertion string or an
  inline `python3 -c` reference oracle.
