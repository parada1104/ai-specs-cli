# GO-20 U2e T5 — PR2-B S11: freeze the mcprender family of Python oracles

Freeze the live Python consumer of `internal/sync/mcprender_test.go` (the
`testdata/mcprender_ref.py` driver) into a frozen golden bundle, same method
as PR2-B S1-S10. This is the LAST PR2-B family. No `lib/` file is removed;
product consumers wait for PR4a/4b.

## Specs

- S1: the standing cycle authorization (below); roadmap order after
  materialize: mcprender.
- S2: `internal/sync/mcprender_test.go` must stop execing `python3` (the ref
  driver at ~line 115 and its python3 LookPath skip guard); equivalence is a
  frozen golden bundle `internal/sync/testdata/mcprender_golden.json`
  captured once from the live Python 3.14.7 oracle, replicating the exact
  argv, cwd, env pinning and fixture setup.
- S3: the differential tests compare the Go leg against the frozen records.
  IMPORTANT — this family's stderr contract is a CONTAINMENT check: the live
  test requires `strings.Contains(ref.Stderr, tc.pythonExc)` for exception
  cases (`tracebackStderr: true`, e.g. PermissionError). Python tracebacks
  embed host paths, so the bundle must NOT freeze full traceback text: freeze
  the compared properties (rc, stdout, written tree/modes as the live test
  compares them, and for traceback cases the recorded exception-type marker);
  record any dropped stderr in the capture inventory. The Go side keeps
  asserting its own stderr contract exactly as today.
- S4: capture reuses the real in-package helpers from a scratch, uncommitted
  Go test (named `zz_scratch_capture_test.go`, deleted before returning) so
  the inputs are byte-identical to the live differential; captured strings
  are scanned for host paths; tokenize only where a host path genuinely
  appears, with a recorded inventory. RECOMMENDED: capture twice with fresh
  roots, byte-identical. MANDATORY (S7 lesson): the focused frozen tests must
  be green under BOTH the default TMPDIR (`/var/folders/...`) AND a
  `/Users/...` scratch TMPDIR; normalize both spellings symmetrically on the
  Go side (path->token) if any path is compared; expand token->path only for
  literal-message pins.
- S5: hard-coded identity inventory (exact case names in order, per-kind
  counts, no duplicates) asserted before the case loops; bundle tampering
  (renamed/missing/duplicated record, mutated byte) fails the suite; restore
  goldens by pristine copy, never JSON re-encode (sha check). If two
  pre-existing case names collide, disambiguate one and record the deviation.
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
- L2 — T1-T3 executed by the scoped `gentle-ai-worker` (seventh delegated
  family; LAST PR2-B family), parent-verified. Parent commit below.
  - Capture: scratch Go test (deleted; audit in scratch) reused the real
    `runMCPRef` helper; bundle `testdata/mcprender_golden.json` — 49 records
    (all kind `differential`; no name collisions), two passes byte-identical
    plus a pass with TMPDIR unset (/tmp): same sha. Pristine sha256
    `27bc3027d8d37027eb2c5b46947f43d4e4c84a34d763cdf6bca7b645d18acbd0`.
  - Frozen properties: rc, stdout, stderr (37 verbatim — single `error: `
    line or empty), target bytes (`target_b64`); for the 12 traceback cases
    only the exception-class marker is frozen (PermissionError x4,
    UnicodeDecodeError x4, IsADirectoryError x1, UnicodeEncodeError x3), each
    verified at capture to embed host paths. Tree/modes were NOT frozen
    (recorded deviation from the generic phrasing: this family's live test
    never compared them — the ref driver only returns target_b64).
  - Freeze: `mcpRefResult`, `runMCPRef`, `mcpB64`, the python3 skip guard and
    the `os/exec` import removed; identity/kind/dropped-stderr inventories +
    `TestMCPRenderFrozenInventoryControl` added; `mcpStr`, `runMCPGo`,
    `govPtrEqual`/`derefString`, `readGoldenBundle`, `gitignoreRepoRoot` and
    the euid-0 guards kept. `mcprender_ref.py` stays until PR4.
  - RED: bundle aside → both frozen tests fail with `read frozen oracle`.
    GREEN: package suite `-count=1` ok (scratch TMPDIR, 15.8s).
  - DUAL-TMPDIR check (worker + parent re-ran): focused family green under
    default TMPDIR (/var/folders) AND scratch TMPDIR (/Users).
  - Mutation controls (pristine restore, sha identical): renamed record →
    identity mismatch; deleted record → `48 records for 49 names`; one
    mutated stdout byte → owning case fails (`dry-run json`).
  - `gofmt -l internal/sync/` empty; `go vet ./internal/sync/` clean.
