# GO-20 U2e T5 PR4a S4 — conflicts core live oracle

Base: epic/go-single-binary 3cbd0b5. Branch: change/go-20-u2e-t5-pr4a-s4-conflictscore.
Goal: `internal/sync/conflicts_core_test.go` stops exec'ing python3 and reading
`lib/_internal/recipe-materialize.py` without silent coverage loss. Same discipline as PR #496 (doctor),
PR #497 (rulesaudit), PR #498 (skills), PR #499 (conflicts), PR #501 (briefpolicy), PR #502 (checksync),
PR #503 (envscaffold), PR #504 (refreshbundled), PR #505 (vendorskills).

## Specs

- S1: `TestNativeConflictTagDifferentialAgainstPython` (same name, no longer a live python differential) reads a
  frozen golden (`internal/sync/testdata/conflictscore_golden.json`) captured once from
  `python3 lib/_internal/recipe-materialize.py`'s `check_tag_conflicts(catalog_dir, recipe_ids)` over the same
  catalog and the same four case ids the live leg built. No python3, no `lib/` access, no skip: a missing golden
  fails.
- S2: The frozen payload carries, per case, the enabled ids and the graded conflicts (or the raised exception
  type), so the native `PlanTagConflicts` still compares against the legacy grade for every case the live leg
  graded.
- S3: Every native assertion of the live leg survives case by case — the same catalog (three tag-sharing recipes,
  one unparsable `recipe.toml`, one `recipe.toml` without `[recipe]`, one enabled directory without a
  `recipe.toml`, one absent `ghost` id), the graded conflict list in first-seen tag order with sorted recipe
  names and severity, and the two boundary cases grading through the gate's lax reader instead of aborting.
- S4: The vacuity guard survives: for a case whose frozen grade is non-empty, the frozen grade must actually
  carry conflicts, and the native list must equal it (not merely be non-nil).
- S5: The test asserts a hard-coded independent inventory of the case names (count, uniqueness, order) against
  both the in-test corpus and the golden before comparing anything. A golden that drops, duplicates or reorders a
  case fails as an inventory error.
- S6: Frozen inputs equal the in-test corpus: the golden pins the pre-run catalog tree (paths, the file bytes and
  the one directory that carries no `recipe.toml`) and every case's ids, and the test snapshots the catalog it
  just wrote and compares it before running the port, so fixture drift fails as input drift, not as an opaque
  payload mismatch.
- S7: No absolute host path leaks into the golden: the payload carries no host path at all (the legacy grade
  names tags and recipe names only), every frozen path is root-relative, the capture refuses to write a payload
  matching `/Users/`, `/private/` or `/var/folders`, and the same grep over the committed golden is 0.
- S8: Negative controls: mutating one frozen conflict, deleting a case, duplicating a case and drifting a frozen
  input each fail rc=1.
- S9: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, the frozen test free of live `python3` /
  `lib/_internal` (only the provenance comment matches), `go test ./internal/sync/ -run
  'TestNativeConflict' -count=1` rc=0 (also under `umask 077`) and `go test ./internal/sync/ -count=1` rc=0.
- S10: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestNativeConflictTagDifferentialAgainstPython` (`python3 lib/_internal/recipe-materialize.py`'s `check_tag_conflicts(catalog_dir, ids)` — which grades through the gate binary pinned by `WORKTREE_GATE_BIN` — versus the native `PlanTagConflicts`, compared by `reflect.DeepEqual` on the conflict list) | FREEZE | the payload is host-independent: the driver printed only `{"type","tag","recipes","severity"}` records plus the raised exception type, and the catalog directory never reaches the output. RETIRE was rejected: `internal/conflicts/testdata/recipe_conflicts_golden.json` (PR #499) freezes the sibling `recipe-conflicts.py` oracle over `conflicts.CheckTagConflicts(recipes)` on a *different* catalog with *different* case ids, and it does not cover the materialize call site's *acquisition*: the gate's lax reader is what omits the absent id, the directory without a `recipe.toml`, the unparsable `recipe.toml` and the file without `[recipe]`, and `PlanTagConflicts` (`internal/sync/conflicts_core.go:14`) is the only frozen oracle that grades tag conflicts from a catalog *directory*. Ids, catalog bytes and expected grades are not case-by-case identical, so RETIRE cannot be proven. RETARGET was rejected for the same reason: no other frozen test covers `PlanTagConflicts`, and the native `TestNativeConflictTagBoundary` is a hand-written expectation, not a frozen oracle. The live leg ran exactly these four cases over one catalog, so the freeze is 1:1. |

Normalizations, all documented in the test and in the golden's `oracle.note`:

- **None needed.** The legacy payload is already host-independent: no path, no clock, no cache key, no git value.
  The catalog is frozen as bytes plus one directory marker so the fixture itself is an input of the payload.
- **Modes are not frozen.** Fixture files are written by the shared `writeSyncCatalogRecipe` (`os.WriteFile`
  `0o644`) and directories by `os.MkdirAll(0o755)`, so both follow the process umask, and this package also runs
  `t.Parallel()` tests that must not have the umask pinned under them. Only paths, "is a directory" and file
  bytes are frozen. A run under `umask 077` in L9 proves it.
- **Authority note.** The legacy driver's primary path (`check_tag_conflicts` → `worktree-gate
  --resolve-tag-conflicts`) already delegates to the Go gate, so the frozen grade is a snapshot of the recorded
  reader lacunae and first-seen ordering at capture time, not of a second implementation. It still pins the port:
  any later change to the gate's reader or to `PlanTagConflicts` breaks the comparison.

Shapes the live leg never exercised stay untouched, and the helpers it shares with other files are reused
read-only: `writeSyncCatalogRecipe` and `syncRecipeHeader` from `stamps_test.go` (which stays, still used by the
copyapply, hookactuator, templatedocs and hooksrunner tests), `readGoldenBundle` and `validateGoldenIdentity` from
`agentsrender_test.go`, and `materializeGateBinary` from `materialize_test.go` only inside the throwaway capture.
The two native tests in the file (`TestNativeConflictPrimitiveBoundary`, `TestNativeConflictTagBoundary`) are
preserved byte for byte: they never called the python oracle.

Cases (4, one shared catalog): `shared tags with an absent directory skipped`, `absent recipe.toml is skipped`,
`unparsable recipe.toml is omitted`, `recipe.toml without [recipe] is omitted`.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S8 | inline | |
| T2 | S9, S10 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then the materialize-helper consumers; one worker at a time, merge after validate.
- L2: Baseline at 3cbd0b5: the live leg is green — the four differential subtests plus the two native boundary
  tests pass (`logs/01-baseline-live-leg.log`, rc=0).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-conflictscore (Robert's
  standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing at its `tmp/`.
- L4: RED before the golden existed: `conflicts_core_test.go:200: read frozen oracle: open
  testdata/conflictscore_golden.json: no such file or directory`, rc=1
  (`logs/02-red-missing-golden.log`).
- L5: The capture harness was temporary code in the test file (the legacy Python driver plus a
  `CONFLICTSCORE_CAPTURE=<path>` gate) and was removed before the commit, so the golden's shape is exactly the
  struct the frozen test decodes. It rebuilt the catalog from the same corpus, set `WORKTREE_GATE_BIN` to the gate
  binary (the live leg's invocation) and ran the real legacy driver with `PYTHONDONTWRITEBYTECODE=1`, then refused
  to write a payload containing an absolute host path. So the golden is legacy output, not port output.
- L6: Captured twice into fresh scratch files and `cmp` byte-identical (`logs/04-capture-cmp.log`, rc=0 —
  `logs/03-capture1.log`, `logs/03-capture2.log`): 3024 B, 11 catalog nodes and 4 cases. Oracle metadata
  `python: 3.14.7`, `script: lib/_internal/recipe-materialize.py`,
  `driver: check_tag_conflicts(catalog_dir, recipe_ids) with WORKTREE_GATE_BIN set to the gate binary`. No
  wall-clock value and no host path is frozen; the capture's own `/Users/`, `/private/`, `/var/folders` guard and
  the same `grep -c` over the committed golden both report 0.
- L7: Negative controls (each rc=1; the golden was restored from a scratch copy and `cmp`-verified byte-identical
  after each): mutating one frozen conflict record → `tag conflicts diverge`
  (`logs/06-nc-mutate.log`), deleting a case → `conflictscore: 3 records, want 4` (`logs/06-nc-delete.log`),
  duplicating a case → `conflictscore: 5 records, want 4` (`logs/06-nc-duplicate.log`), drifting one frozen
  catalog byte → `catalog input: b/recipe.toml bytes differ` (`logs/06-nc-inputdrift.log`).
- L8: `gofmt -l internal/sync` empty (`logs/11-gofmt.log`, rc=0), `go vet ./internal/sync/` rc=0
  (`logs/12-vet.log`), and `grep -n "python3\|lib/_internal\|os/exec\|exec\.Command\|syncPythonAuthority\|
  runSyncPythonOracle"` over the test matches the provenance comment line only
  (`logs/13-grep-live-legs.log`).
- L9: `-run 'TestNativeConflict'` rc=0 (`logs/05-green-focused.log`), the same under `umask 077` rc=0
  (`logs/07-green-umask077.log` — the catalog freezes no mode, so the umask cannot change the payload),
  `go test ./internal/sync/ -count=1` rc=0 in 15.9s (`logs/08-green-package-tests.log`).
- L10: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/09-build.log`), `./tests/validate.sh` rc=0
  (`logs/10-validate.log`; the legacy-vs-Go parity harness reports zero deltas on every scenario).
- L11: The worktree carries only the three intended paths (test, golden, this doc); the capture harness, both
  captures and all logs live in the scratch dir, and the commit is left to the orchestrator (worker boundary).
