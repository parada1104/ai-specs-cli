# GO-20 U2e T5 — PR2-B S7: freeze the buildrecipemcp family of Python oracles

Freeze the live Python consumer of `internal/sync/buildrecipemcp_test.go` (the
`testdata/buildrecipemcp_ref.py` driver) into a frozen golden bundle, same
method as PR2-B S1-S6. No `lib/` file is removed; product consumers wait for
PR4a/4b.

## Specs

- S1: the standing cycle authorization (below); roadmap order after
  briefref: buildrecipemcp.
- S2: `internal/sync/buildrecipemcp_test.go` must stop execing the live
  Python oracle (`runBuildRecipeMCPRef` and any other python exec path,
  including `syncPythonAuthority` if it serves a Python leg); equivalence is
  a frozen golden bundle
  `internal/sync/testdata/buildrecipemcp_golden.json` captured once from the
  live Python 3.14.7 oracle, replicating the exact argv, cwd, env pinning and
  fixture setup.
- S3: the differential tests compare the Go leg against the frozen records;
  skip guards tied to the frozen Python leg disappear (keep guards that are
  not Python guards, e.g. git availability).
- S4: capture reuses the real in-package helpers from a scratch, uncommitted
  Go test (named `zz_scratch_capture_test.go`, deleted before returning) so
  the inputs are byte-identical to the live differential; captured strings
  are scanned for host paths (worktree/scratch/TMPDIR); tokenize only where a
  host path genuinely appears, with a recorded inventory; if a captured
  stderr is an uncaught traceback embedding host paths that the live test
  never compared, freeze rc/stdout/preserved bytes only (S3/S4 precedent).
  Cheap extra evidence, recommended: run the fixtures twice with fresh roots
  and require byte-identical records (root independence).
- S5: hard-coded identity inventory (exact case names in order, per-kind
  counts, no duplicates) asserted before the case loops; bundle tampering
  (renamed/missing/duplicated record, mutated byte) fails the suite; restore
  goldens by pristine copy, never JSON re-encode (sha check).
- S6: RED/GREEN; `gofmt -l` clean; `go vet` clean; package suite green with
  `-count=1`; full `tests/validate.sh` green at the exact pushed head;
  `gentle_review assess` on the committed range; PR against
  `epic/go-single-binary`; the parent commits, pushes, PRs and merges.

## Tasks

- T1 (S2, S4): capture the golden bundle via the scratch capture test
  (worker).
- T2 (S2-S3): rewrite the differential tests against the bundle; drop the
  frozen-leg skip guards; delete the now-unused Python-leg helpers, keeping
  anything shared with other tests in the package (worker).
- T3 (S5): identity inventory + tamper controls (worker).
- T4 (S6): docs L2 (parent commits); validate + assess + push + PR + merge
  (parent).

## Log

- L1 — User request (verbatim): "dale vayamos avanzando en un 'ciclo' si
  puedes después de esto vamos con workers de herdr tu solo encargate de
  gestionar,  solo una un worker a la vez, un cambio a la vez tienes permiso
  para mergear y si el harness te obliga a validar por ejemplo push o
  limpieza  vayamos haciendo changes stackeados, pero cumplamos el ciclo de
  cada uno" y "es decir implementacion, validacion, y entrega(pr, push,
  limpieza, pull, continuamos) en caso de no poder hacer push vamos de manera
  local, commit, worktree nuevo basado desde el anterior".
- L2 — T1-T3 executed by the scoped `gentle-ai-worker` (third delegated
  family of the cycle), parent-verified with one correction. Parent commits
  below.
  - Capture: scratch Go test (deleted before commit; audit copy in scratch
    `captures/`) reused the real helpers with the live argv/PATH/cmd.Dir/
    `PYTHONDONTWRITEBYTECODE` env. Bundle: `testdata/buildrecipemcp_golden.json`
    — 16 records (build 7, resolve 9, no duplicates), captured twice with
    fresh roots byte-identical (root independence); host-path scan 0 hits
    except 3 inventoried token positions: `<CATALOG>` x2 (error/stderr of the
    ghost cases) and `<HOME>` x1 (managed jinna absolute command). Pristine
    sha256 `b2ea6bdca0c6fe8686d6c95282366cd2c8b067c710651ef94a090188f2a6b772`.
  - Freeze: `buildRecipeMCPRefPath`, `runBuildRecipeMCPRef`, `mustJSONArg`
    and the `os/exec` import deleted; six tests consume the bundle with the
    hard-coded identity inventory (16 names in order, per-kind counts) and a
    per-record token inventory validated at load
    (`TestBuildRecipeMCPFrozenInventoryControl` added);
    `syncPythonAuthority`/`materializeGateBinary` kept (11 other tests use
    them); `buildrecipemcp_ref.py` stays on disk until PR4.
  - RED: bundle aside → 6 tests fail with `read frozen oracle`. GREEN:
    package suite `-count=1` ok.
  - Mutation controls (pristine restore, sha identical): renamed record →
    identity mismatch; deleted record → `15 names, want 16`; one mutated
    document byte → owning case fails.
  - PARENT CORRECTION (the worker's environment masked it): with the default
    TMPDIR on macOS (`/var/folders/...`), the Go leg emits the symlink-
    resolved spelling (`/private/var/...`) of the managed command while the
    frozen record keeps the raw `<HOME>` form — the same divergence already
    fixed for projectcache (Engram 360). The worker never saw it because its
    scratch TMPDIR lives under `/Users/...` (no symlink). Fix: the comparison
    now normalizes the GO side to the frozen tokens via `mcpFreezeHostPath`
    (both raw and EvalSymlinks spellings), `frozenBuildRecipeMCP` validates
    tokens without expanding, and the literal-message assertion uses the
    inverse `mcpExpandTokens`. Verified green under BOTH TMPDIRs
    (`/var/folders` and the scratch) plus the full package suite.
  - `gofmt -l internal/sync/` empty; `go vet ./internal/sync/` clean.
