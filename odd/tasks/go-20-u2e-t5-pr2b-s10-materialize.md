# GO-20 U2e T5 — PR2-B S10: freeze the materialize family of Python oracles

Freeze the live Python consumers of `internal/sync/materialize_test.go` and
`internal/sync/materialize_loop_test.go` (the `testdata/materialize_ref.py`
driver) into a frozen golden bundle, same method as PR2-B S1-S9. No `lib/`
file is removed; product consumers wait for PR4a/4b.

## Specs

- S1: the standing cycle authorization (below); roadmap order after
  hooksrunner: materialize.
- S2: both materialize test files must stop execing the live Python oracle
  (`runMaterializeRef` and every python3 exec/LookPath serving the ref leg;
  the `go build` exec stays — it builds the gate binary, not Python;
  `materializeGateBinary` and `syncPythonAuthority` are SHARED and must
  stay); equivalence is a frozen golden bundle
  `internal/sync/testdata/materialize_golden.json` captured once from the
  live Python 3.14.7 oracle, replicating the exact argv
  (`<driver> <authority> <project> <home> <mcpOut> <configOut> <hooksOut>`),
  cwd, env pinning and fixture setup.
- S3: the differential tests compare the Go leg against the frozen records
  (byte equality of the written artifacts/streams as the live test compares
  them); skip guards tied to the frozen Python leg disappear (keep guards
  that are not Python guards, e.g. git availability).
- S4: capture reuses the real in-package helpers from a scratch, uncommitted
  Go test (named `zz_scratch_capture_test.go`, deleted before returning) so
  the inputs are byte-identical to the live differential; captured strings
  are scanned for host paths; tokenize only where a host path genuinely
  appears, with a recorded inventory; tracebacks embed host paths — freeze
  the compared properties only. RECOMMENDED: capture twice with fresh roots,
  byte-identical. MANDATORY (S7 lesson): the focused frozen tests must be
  green under BOTH the default TMPDIR (`/var/folders/...`) AND a `/Users/...`
  scratch TMPDIR; normalize both spellings symmetrically on the Go side
  (path->token) if any path is compared; expand token->path only for
  literal-message pins.
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
- L2 — T1-T3 executed by the scoped `gentle-ai-worker` (sixth delegated
  family), parent-verified. Parent commit below.
  - Capture: scratch Go test (deleted; audit in scratch `captures/`) reused
    the real helpers and scenario sources (`materializeSpineScenarios`,
    `materializeStampScenarios`, loop scenarios) with the exact argv/env
    (`PYTHONDONTWRITEBYTECODE=1`, `WORKTREE_GATE_BIN`, `AI_SPECS_GATE_OFFLINE=1`,
    `MATERIALIZE_REFRESH_GATES=1` for the refresh case). Bundle:
    `testdata/materialize_golden.json` — 26 records (spine 11, stamp 4,
    loop 11), two passes with fresh roots byte-identical; re-capture after
    the normalizer change produced the same sha. Pristine sha256
    `b905b5e4fea11a55893d479dc1d0775a837aecc99da747d05c929d29c487ba6c`.
  - Tokens (hard-coded, asserted per record before loops): `<HOME>`,
    `<PROJECT>`, `<CACHE_PROJECT>` (per-project cache dir basename
    `"<hash>-<project>"`, tokenized in tree KEYS too). 0 dropped values; no
    traceback text frozen. Host-path scan 0 hits.
  - Freeze: `materializeRefPath`, `runMaterializeRef`,
    `runMaterializeLoopRef`, `materializeRefOut` and the python3 LookPath
    guards removed from both files; `materializeGateBinary` (go build),
    `syncPythonAuthority` and all shared fixture helpers kept; symmetrical
    Go-side normalization (raw, Clean, EvalSymlinks) for the dual-TMPDIR
    requirement. `materialize_ref.py` stays until PR4.
  - RED: bundle aside → the three frozen tests fail with `read frozen
    oracle`. GREEN: package suite `-count=1` ok (scratch TMPDIR, 20.0s).
  - DUAL-TMPDIR check (worker + parent re-ran): focused family green under
    default TMPDIR (/var/folders, /var->/private/var) AND scratch TMPDIR
    (/Users).
  - Mutation controls (pristine restore, sha identical): renamed record →
    identity mismatch; deleted record → `25 names, want 26`; one mutated
    stderr byte → owning loop case fails (`worktree-flow gate acquisition
    degrades offline`).
  - `gofmt -l internal/sync/` empty; `go vet ./internal/sync/` clean.
