# GO-20 U2e T5 — PR2-B S6: freeze the briefref family of Python oracles

Freeze the live Python consumer of `internal/sync/briefview_test.go` (the
`testdata/briefref_ref.py` driver, which runs the three legacy lib modules)
into a frozen golden bundle, same method as PR2-B S1-S5. No `lib/` file is
removed; product consumers wait for PR4a/4b.

## Specs

- S1: the standing cycle authorization of L1 (see below); roadmap order
  after bindings: briefref.
- S2: `internal/sync/briefview_test.go` must stop execing `python3`/the live
  oracle (`briefRefAuthority`/`runBriefRef`); equivalence is a frozen golden
  bundle `internal/sync/testdata/briefref_golden.json` captured once from the
  live Python 3.14.7 oracle, replicating the exact root/home/lib argv and any
  fixture setup (including env pinning like WORKTREE_GATE_BIN if present).
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
- S5: hard-coded identity inventory (exact case names in order, per-kind
  counts, no duplicates) asserted before the case loops; bundle tampering
  (renamed/missing/duplicated record, mutated byte) fails the suite; restore
  goldens by pristine copy, never JSON re-encode (sha check).
- S6: RED/GREEN; `gofmt -l` clean; `go vet` clean; package suite green with
  `-count=1`; full `tests/validate.sh` green at the exact pushed head;
  `gentle_review assess` on the committed range; PR against
  `epic/go-single-binary`; the parent commits, pushes, PRs and merges.

## Log

- L1 — User request (verbatim): "dale vayamos avanzando en un 'ciclo' si
  puedes después de esto vamos con workers de herdr tu solo encargate de
  gestionar,  solo una un worker a la vez, un cambio a la vez tienes permiso
  para mergear y si el harness te obliga a validar por ejemplo push o
  limpieza  vayamos haciendo changes stackeados, pero cumplamos el ciclo de
  cada uno" y "es decir implementacion, validacion, y entrega(pr, push,
  limpieza, pull, continuamos) en caso de no poder hacer push vamos de manera
  local, commit, worktree nuevo basado desde el anterior".
- L2 — T1-T3 executed by the scoped `gentle-ai-worker` (second delegated
  family of the cycle), parent-verified. Parent commit below.
  - Capture: scratch Go test (deleted before commit; audit copy in scratch
    `captures/`) reused the real helpers with the exact
    `briefRefAuthority`/`runBriefRef` argv, cwd, `PYTHONDONTWRITEBYTECODE` +
    `WORKTREE_GATE_BIN` env pinning and fixture setup. Bundle:
    `testdata/briefref_golden.json` — view 8 records (7 rendered state+sha +
    1 raised-error, pin `errors=1`) + dead 6 records (pin `dead=true` count
    2), hard-coded `names` inventories asserted before the loops.
  - Host-path scan: 0 hits across 52 captured strings (worktree, scratch,
    TMPDIR/GOTMPDIR/GOCACHE, HOME, `/var/folders/`, `/tmp/`, markers); the
    capture ran the fixtures twice with fresh roots producing byte-identical
    records (root independence); every case exits 0, nothing dropped.
    Pristine sha256 `966e65784d9f47b5d749aa756a3f1d017da57449a1f8f5dd97e985a150a1106b`.
  - Freeze: `briefRefOut`, `briefRefAuthority`, `runBriefRef` deleted; the
    two differential tests consume the bundle; fixture helpers stay (still
    used); `materializeGateBinary` stays (other tests use it);
    `briefref_ref.py` stays on disk until PR4.
  - RED: bundle aside → both tests fail with `read frozen oracle`. GREEN:
    `go test -count=1 ./internal/sync/` ok (31.0s fresh process; 33.7s
    re-run after controls).
  - Mutation controls (pristine restore after each, sha identical): renamed
    view record → identity mismatch; deleted dead record → `5 records, want
    6`; duplicated dead record → `7 records, want 6`; one mutated view byte
    → owning case fails on frozen sha. Intact bundle validates.
  - `gofmt -l internal/sync/` empty; `go vet ./internal/sync/` clean.
