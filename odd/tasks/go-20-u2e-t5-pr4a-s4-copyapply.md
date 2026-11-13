# GO-20 U2e T5 PR4a S4 — copyapply live oracle

Base: epic/go-single-binary 932ce98. Branch: change/go-20-u2e-t5-pr4a-s4-copyapply.
Goal: `internal/sync/copyapply_test.go` stops exec'ing python3 and reading
`lib/_internal/recipe-materialize.py` without silent coverage loss. Same discipline as PR #496 (doctor),
PR #497 (rulesaudit), PR #498 (skills), PR #499 (conflicts), PR #501 (briefpolicy), PR #502 (checksync),
PR #503 (envscaffold), PR #504 (refreshbundled), PR #505 (vendorskills), PR #506 (conflictscore).

## Specs

- S1: `TestCopyApplyPythonDifferential` (same name, no longer a live python differential) reads a frozen golden
  (`internal/sync/testdata/copyapply_golden.json`) captured once from
  `python3 lib/_internal/recipe-materialize.py`'s copy entry points the materialize loop calls
  (`materialize_bundled_skill`, `materialize_command`, `materialize_doc`, over `read_recipe`) for the same
  fixture and the same enabled ids the live leg drove. No python3, no `lib/` access, no skip: a missing golden
  fails.
- S2: The frozen payload carries, per case, the enabled ids, the three cache roots the legacy authority
  resolved (`recipe_skills_root`, `commands_dir`, `project_root`) and the recorded copy items
  (kind/id/src/dest), so the native `PlanCopyItems` still compares against the legacy derivation for every
  surface the live leg compared.
- S3: Every native assertion of the live leg survives case by case — the same fixture catalog (one recipe with a
  bundled skill, a non-bundled dependency skill, a command and a doc, with the real source files), the same
  three-item plan in the same order with the same src/dest per kind, and the non-vacuous "3 items, all three
  kinds, no `alpha-dep`" guard on both sides before the equality check.
- S4: The cache-root surface is pinned, not merely supplied: the frozen roots are the ones the legacy authority
  resolved, and the native caller re-derives today's roots through the mirrored helpers
  (`projectcache.RecipeSkillsRoot`, `projectcache.CommandsDir`, `projectcache.CacheRoot`), so a root-shape
  regression fails as `cache roots diverge` instead of passing through.
- S5: The test asserts a hard-coded independent inventory of the case names (count, uniqueness, order) against
  both the in-test corpus and the golden before comparing anything. A golden that drops, duplicates or reorders
  a case fails as an inventory error.
- S6: Frozen inputs equal the in-test corpus: the golden pins the pre-run catalog tree (paths, "is a directory"
  and file bytes) and the case ids, and the test snapshots the catalog its own builder just wrote and compares
  it before running the plan, so fixture drift fails as input drift, not as an opaque payload mismatch.
- S7: No absolute host path leaks into the golden: the temporary base directory is tokenized as `<BASE>` and the
  per-project cache key directory (a sha256 of the project's absolute path) as `<CACHE>`, the loader refuses any
  root or item path that does not start with `<BASE>/`, every frozen catalog path is root-relative, the capture
  refused to write a payload matching `/Users/`, `/private/` or `/var/folders`, and the same grep over the
  committed golden is 0.
- S8: Negative controls: mutating one frozen item, deleting a case, duplicating a case and drifting a frozen
  catalog byte each fail rc=1.
- S9: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, the frozen test free of live `python3` /
  `syncPythonAuthority` / `runSyncPythonOracle` / `lib/_internal` (only the provenance comments match),
  `go test ./internal/sync/ -run 'TestCopyApply' -count=1` rc=0 (also under `umask 077`) and
  `go test ./internal/sync/ -count=1` rc=0.
- S10: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestCopyApplyPythonDifferential` (`python3` driving `lib/_internal/recipe-materialize.py`'s `materialize_bundled_skill` / `materialize_command` / `materialize_doc` with `go_apply_copy` replaced by a recorder, versus the native `PlanCopyItems`, compared by `reflect.DeepEqual` on the four-field item projection plus the resolved cache roots) | FREEZE | the payload is tokenizable and stays meaningful in token space: the only host-dependent values are the temporary base directory and the per-project cache key directory (a sha256 of the project's absolute path), so both are frozen as `<BASE>` / `<CACHE>` and everything else — the `.recipe`/`commands` cache layout, the per-kind src/dest derivation, the doc target and the item order — is preserved verbatim. RETIRE was rejected: no other frozen oracle covers `PlanCopyItems` (the function has no production call site yet — its consumers are `internal/sync/materialize_loop.go` and `templatedocs.go` through `ApplyCopyItems` — and no other bundle records the src/dest Python would send for this recipe shape). RETARGET was rejected for the same reason: the native packageless coverage of the copy surface is `TestCopyApplyDifferentialBoundary` (the plan→apply composition), which asserts the executor's outcome and the dest bytes, not the legacy src/dest derivation per kind, and its fixture is a different recipe (`beta`, no dep skill, different doc). The live leg ran exactly one case over one catalog, so the freeze is 1:1. |

Normalizations, all documented in the test and in the golden's `oracle.note`:

- **`<BASE>` / `<CACHE>` tokens.** The legacy driver printed absolute paths (the three cache roots and every
  item's src/dest). The base directory — including its macOS `/var` → `/private/var` resolution, which the test
  applies up front through `projectcache.ResolvePath` — becomes `<BASE>`; the cache root's last element only
  (the sha256-derived key directory) becomes `<CACHE>` so the rest of the cache shape stays frozen. Tokens are
  applied to both sides, so the comparison runs in the golden's host-independent space, and the golden keeps
  the tokens visible (`<BASE>/home/cache/projects/<CACHE>/.recipe`).
- **Modes are not frozen.** Fixture files are written by the shared `writeSyncCatalogRecipe`,
  `writeCopyFixtureFile` (`os.WriteFile` `0o644`) and directories by `os.MkdirAll` (`0o755`), so both follow the
  process umask, and this package also runs `t.Parallel()` tests that must not have the process umask pinned
  under them. Only paths, "is a directory" and file bytes are frozen. A run under `umask 077` in L9 proves it.
- **Cache key value.** The key itself is a hash of the host path, so it can only exist as a token; the frozen
  root and item paths still pin that the key directory sits under `<cli_home>/cache/projects/` and that the
  recipe skills root is its `.recipe` child and the commands dir its `commands` child.
- **Authority note.** The legacy driver replaced only the bridge sink (`go_apply_copy` → recorder) and no-oped
  `write_lock`, so the frozen payload is what the live Python call sites would have sent the shared copy
  authority — not a second implementation's output.

Shapes the live leg never exercised stay untouched, and the helpers it shares with other files are reused
read-only: `writeSyncCatalogRecipe` and `syncRecipeHeader` from `stamps_test.go` (which stays, still used by the
hookactuator, templatedocs and hooksrunner tests), `readGoldenBundle` and `validateGoldenIdentity` from
`agentsrender_test.go`, and `syncPythonAuthority` / `runSyncPythonOracle` only inside the throwaway capture. The
five native tests in the file (`TestCopyApplyPlan*`, `TestCopyApplyExecutor`, `TestCopyApplyDifferentialBoundary`)
are preserved: they never called the python oracle.

Cases (1): `one recipe with a bundled skill, a dependency skill, a command and a doc`.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S8 | inline | |
| T2 | S9, S10 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then the materialize-helper consumers; one worker at a time, merge after validate.
- L2: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-copyapply (Robert's standing
  scratch convention); every go test/build/validate ran with `TMPDIR` pointing at its `tmp/`.
- L3: RED before the golden existed, with the restructured test in place:
  `copyapply_test.go:602: read frozen oracle: open testdata/copyapply_golden.json: no such file or directory`,
  rc=1 (`logs/02-red-missing-golden.log`).
- L4: The capture harness was temporary code in the test file (the legacy Python driver plus a
  `COPYAPPLY_CAPTURE=<path>` gate) and was removed before the commit, so the golden's shape is exactly the
  struct the frozen test decodes. It built the fixture from the same builder, tokenized the legacy output and
  refused to write a payload containing an absolute host path. So the golden is legacy output, not port output.
- L5: Captured twice into fresh scratch files and `cmp` byte-identical (`logs/04-capture-cmp.log`, rc=0 —
  `logs/03-capture1.log`, `logs/03-capture2.log`): 2740 B, 9 catalog nodes and 1 case, 3 items. Oracle metadata
  `python: 3.14.7`, `script: lib/_internal/recipe-materialize.py`,
  `driver: read_recipe + materialize_bundled_skill / materialize_command / materialize_doc with go_apply_copy
  replaced by a recorder and write_lock a no-op`. The capture's own `/Users/`, `/private/`, `/var/folders`
  guard and the same `grep -c` over the committed golden both report 0.
- L6: Negative controls (each rc=1; the golden was restored from a scratch copy and `cmp`-verified byte-identical
  after each): mutating one frozen item dest → `copy plan diverges` (`logs/06-nc-mutate.log`), deleting the case
  → `copyapply: 0 records, want 1` (`logs/06-nc-delete.log`), duplicating the case →
  `copyapply: 2 records, want 1` (`logs/06-nc-duplicate.log`), drifting one frozen catalog byte →
  `catalog input: alpha/commands/alpha.md bytes differ` (`logs/06-nc-inputdrift.log`).
- L7: `gofmt -l internal/sync` empty (`logs/11-gofmt.log`, rc=0), `go vet ./internal/sync/` rc=0
  (`logs/12-vet.log`), and `grep -n "syncPythonAuthority\|runSyncPythonOracle\|python3\|lib/_internal\|os/exec\|
  exec\.Command"` over the test matches the provenance comments only.
- L8: `-run 'TestCopyApply'` rc=0 (`logs/05-green-focused.log`), the same under `umask 077` rc=0
  (`logs/07-green-umask077.log` — the payload freezes no mode, so the umask cannot change it),
  `go test ./internal/sync/ -count=1` rc=0 in 12.8s (`logs/08-green-package-tests.log`).
- L9: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/09-build.log`), `./tests/validate.sh` rc=0
  (`logs/10-validate.log`; parity summary gate-absent failing=0, gate-present failing=0 — PASS, Python suite
  `Ran 2345 tests in 550s — OK (skipped=165, expected failures=3)`).
- L10: The worktree carries only the three intended paths (test, golden, this doc); the capture harness, both
  captures and all logs live in the scratch dir, and the commit is left to the orchestrator (worker boundary).
