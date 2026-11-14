# GO-20 U2e T5 — PR2-B S8: freeze the hooksrender family of Python oracles

Freeze the live Python consumer of `internal/sync/hooksrender_test.go` (the
`testdata/hooksrender_ref.py` driver) into a frozen golden bundle, same method
as PR2-B S1-S7. No `lib/` file is removed; product consumers wait for
PR4a/4b.

## Specs

- S1: the standing cycle authorization (below); roadmap order after
  buildrecipemcp: hooksrender.
- S2: `internal/sync/hooksrender_test.go` must stop execing `python3` (the
  ref driver at ~line 129 and its skip guard); equivalence is a frozen golden
  bundle `internal/sync/testdata/hooksrender_golden.json` captured once from
  the live Python 3.14.7 oracle, replicating the exact argv, cwd, env pinning
  and fixture setup.
- S3: the differential tests compare the Go leg against the frozen records.
  IMPORTANT — this family's stderr contract is a CONTAINMENT check: the live
  test requires `strings.Contains(ref.Stderr, tc.pythonExc)` for exception
  cases (`tracebackStderr: true`). Python tracebacks embed host paths, so the
  bundle must NOT freeze full traceback text: freeze the frozen properties
  the live test actually checks (rc, stdout, and for traceback cases a
  recorded exception-type marker), and record any dropped stderr in the
  capture inventory. The Go side keeps asserting its own stderr contract
  (e.g. the single-line diagnostic), exactly as today.
- S4: capture reuses the real in-package helpers from a scratch, uncommitted
  Go test (named `zz_scratch_capture_test.go`, deleted before returning) so
  the inputs are byte-identical to the live differential; captured strings
  are scanned for host paths; tokenize only where a host path genuinely
  appears with a recorded inventory. REQUIRED check (S7 lesson): the focused
  frozen tests must be run under BOTH the default TMPDIR (`/var/folders/...`,
  where macOS /var -> /private/var symlink divergence appears) AND a
  `/Users/...` scratch TMPDIR, and both must be green — normalize both
  spellings symmetrically on the Go side (path->token) if any path is
  compared; expand token->path only for literal-message pins.
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
- L2 — T1-T3 executed by the scoped `gentle-ai-worker` (fourth delegated
  family), parent-verified. Parent commit below.
  - Capture: scratch Go test (deleted; audit copies in scratch `captures/`)
    reused the real helpers; bundle `testdata/hooksrender_golden.json` —
    98 records (97 render + 1 sorted-dumps), captured twice with fresh roots
    byte-identical; host-path scan 0 hits (no tokens); 42 traceback records
    freeze only the exception-type marker (UnicodeDecodeError x3, KeyError
    x5, TypeError x20, AttributeError x5, IndexError x5, UnicodeEncodeError
    x6) — full traceback stderr dropped (host paths), exactly per S3.
    Pristine sha256 `52c5aad0ed93a7d2b1266c315d8e038bde82413b1b4d1398287e28c139c46cda`.
  - Freeze: `hooksRefResult`/`runHooksRef` and both python3 skip guards
    removed; identity/kind/dropped-stderr inventories + `frozenHooksRecord`
    + `TestHooksRenderFrozenInventoryControl` added; the Go stderr contract
    (one `error: ` line + goErrContains) is asserted per record; legit guards
    kept (euid 0, bash/awk). `hooksrender_ref.py` stays until PR4.
  - Recorded deviation: the pre-existing duplicate case name
    "cursor env list negative index tolerated" was disambiguated to
    "cursor raw env list negative index tolerated" (inputs/assertions
    unchanged) so the no-duplicates inventory holds.
  - RED: bundle aside → 3 tests fail with `read frozen oracle`. GREEN:
    package suite `-count=1` ok (scratch TMPDIR, 26.0s).
  - DUAL-TMPDIR check (worker + parent re-ran): focused family tests green
    under default TMPDIR (/var/folders) AND scratch TMPDIR (/Users).
  - Mutation controls (pristine restore, sha identical): renamed record →
    identity mismatch; deleted record → `97 names, want 98`; one mutated
    tree byte in `.claude/settings.json` → owning subtest fails.
  - `gofmt -l internal/sync/` empty; `go vet ./internal/sync/` clean.
