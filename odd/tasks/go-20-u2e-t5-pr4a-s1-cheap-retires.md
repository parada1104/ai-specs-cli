# GO-20 U2e T5 — PR4a S1: cheap retires and retargets before the lib/ deletion

First PR4a slice (triage by the read-only explorer of 2026-11-02): retire the
lib/-dependent test bits that are already duplicated or dead, re-target the
validation gate, and remove the expired parity constants. No lib/ file is
removed; PR4b does the deletion.

## Specs

- S1: the standing cycle authorization (below); PR4a slicing per the triage:
  this slice is the "cheap retires/retargets" slice.
- S2: `internal/sync/syncagent_test.go` `TestCachePathOracleMatchesProjectCachePy`
  (live `python3 lib/_internal/project-cache.py <project> path <kind>` at
  ~:895) is RETIRED only if `internal/projectcache/testdata/projectcache_golden.json`
  + its test cover the same path kinds — verify first and cite the covering
  records; otherwise re-target the pin to the frozen bundle instead.
- S3: `internal/sync/args_test.go` `TestSyncCDLineMatchesShellSource` READS
  `lib/sync.sh` (~:101-117) to pin the legacy byte-compare constant; retire
  the test (the pin loses meaning at PR4b) and record the control loss in the
  doc; do NOT delete the constant itself if product code uses it (check).
- S4: `tests/validate.sh` lines ~6-7 (`python3 -m py_compile lib/_internal/*.py`
  and `bash -n lib/*.sh`) are RE-TARGETED with a file-existence guard so the
  gate keeps passing now and silently degrades after PR4b deletes lib/
  (log a note, do not fail, when lib/ is absent).
- S5: `tests/parity/parity.py` expired hooks-render legacy constants (the
  "EXPIRES with lib/_internal (T5)" rule at ~:222-238) are removed together
  with their only call sites; the frozen golden fixtures are NOT touched.
- S6: RED/GREEN where a test is retired or re-targeted (the retired test
  must be shown to fail nothing else); `gofmt -l` clean; `go vet` clean;
  package suite green with `-count=1`; the touched validate.sh/parity.py
  paths re-run (`tests/run.sh` or the relevant subset); full
  `tests/validate.sh` green at the exact pushed head; `gentle_review assess`
  on the committed range; PR against `epic/go-single-binary`; the parent
  commits, pushes, PRs and merges.
- S7: every retire carries a one-line duplicate-citation (the frozen bundle /
  Go differential that covers the same behavior) recorded in the doc.

## Tasks

- T1 (S2, S7): verify projectcache coverage; retire/re-target the syncagent
  cache-path oracle (worker).
- T2 (S3, S7): retire the CD-line shell-source pin (worker).
- T3 (S4, S5): validate.sh guard + parity expired constants (worker).
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
  local, commit, worktree nuevo basado desde el anterior"; continuation
  "ok sigamos con lo demas" (2026-11-02) after PR2-B completion.
- L2 — T1-T3 executed by the scoped `gentle-ai-worker` (PR4a cycle start),
  parent-verified. Parent commit below.
  - T1/S2 RETIRED `TestCachePathOracleMatchesProjectCachePy` (live `python3
    lib/_internal/project-cache.py <project> path <kind>`): full duplicate —
    the frozen `projectcache_golden.json` cases `cli path root` and `cli path
    resolved-skills` pin the same two kinds via `TestProjectCacheDifferential`
    subtests (observed passing; `RenderProjectCache("path", kind)` dispatches
    to the same `CacheRoot`/`ResolvedSkillsDir` the retired test called).
  - T2/S3 RETIRED `TestSyncCDLineMatchesShellSource` (READ of `lib/sync.sh`,
    no exec): control loss recorded — `syncCDLine` (`sync.go:54`, printed at
    `:185`) keeps the constant as a frozen diagnostic literal once lib/ dies;
    product code still uses it, so the constant stays.
  - T3/S4 `tests/validate.sh` re-targeted: the lib/ syntax checks are wrapped
    in `if [ -d lib ]`; when lib/ is absent the gate logs one note and still
    checks `tests/*.py` + `bin/ai-specs` + `tests/*.sh`. Dry-probed with and
    without lib/ (both rc 0); `bash -n` clean.
  - T4/S5 `tests/parity/parity.py`: expired N10/T7 Cursor-deny legacy rule
    removed with its constants, helper and registration; 39 harness unit
    tests OK; golden fixtures untouched.
  - Evidence: package suite `-count=1` green (16.1s); `gofmt -l internal/`
    empty; `go vet` clean. Diff: 4 files, 9+/93-.
