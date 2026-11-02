# GO-20 U2e T5 — PR2-B S9: freeze the hooksrunner family of Python oracles

Freeze the live Python consumer of `internal/sync/hooksrunner_test.go` (the
`testdata/hooksrunner_ref.py` driver, invoked with the authority resolved by
the shared `syncPythonAuthority` helper) into a frozen golden bundle, same
method as PR2-B S1-S8. No `lib/` file is removed; product consumers wait for
PR4a/4b.

## Specs

- S1: the standing cycle authorization (below); roadmap order after
  hooksrender: hooksrunner.
- S2: `internal/sync/hooksrunner_test.go` must stop execing the live Python
  oracle (`runHooksRunnerRef`; `syncPythonAuthority` itself is SHARED by 11+
  other tests in the package and must stay); equivalence is a frozen golden
  bundle `internal/sync/testdata/hooksrunner_golden.json` captured once from
  the live Python 3.14.7 oracle, replicating the exact argv
  (`run <authority> <catalog> <recipe> <pyRoot> <cliHome> <merged>`), cwd, env
  pinning and fixture setup.
- S3: the differential tests compare the Go leg against the frozen records
  (error message, stdout, written artifacts as the live test compares them);
  skip guards tied to the frozen Python leg disappear (keep guards that are
  not Python guards, e.g. git availability).
- S4: capture reuses the real in-package helpers from a scratch, uncommitted
  Go test (named `zz_scratch_capture_test.go`, deleted before returning) so
  the inputs are byte-identical to the live differential; captured strings
  are scanned for host paths; tokenize only where a host path genuinely
  appears, with a recorded inventory; tracebacks embed host paths — freeze
  the compared properties only (marker/rc/stdout/preserved bytes), never
  traceback text. RECOMMENDED: run the fixture capture twice with fresh
  roots and require byte-identical records. MANDATORY (S7 lesson): the
  focused frozen tests must be green under BOTH the default TMPDIR
  (`/var/folders/...`) AND a `/Users/...` scratch TMPDIR; normalize both
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
- L2 — T1-T3 executed by the scoped `gentle-ai-worker` (fifth delegated
  family), parent-verified. Parent commit below.
  - Capture: scratch Go test (deleted; audit copy in scratch) reused the real
    helpers with the exact argv `run <authority> <catalog> <recipe> <pyRoot>
    <cliHome> <merged>` and `PYTHONDONTWRITEBYTECODE=1`; 13 records (11
    differential + 2 repr), two passes with fresh roots byte-identical,
    host-path scan 0 hits, zero tokens, zero dropped stderr (the driver
    catches its exceptions; every stderr is the plain warning stream).
    Pristine sha256 `35d0879ecbd114a517614f6e6236185cc462d2fb4a38f9899f7bae293b2a3393`.
  - Freeze: `hooksRunnerRefPath`/`hooksRunnerRefOut`/`runHooksRunnerRef`
    deleted; `syncPythonAuthority` kept (shared); tests renamed to
    `TestRecipeHooksDifferentialFrozen`; identity/kind/marker inventories +
    `TestHooksRunnerFrozenInventoryControl` added; marker bytes compared
    exactly. `hooksrunner_ref.py` stays until PR4.
  - REMAINDER for PR4a: `pythonFullMatchStatus` still execs python3 in
    `TestRecipeHooksRegexEngineDivergence` / `RegexPythonOnlyConstructs` —
    that is the Python-re-engine vs RE2 divergence pin, not the ref-driver
    oracle of this family; its freeze (or retirement) belongs to the PR4a
    triage.
  - RED: bundle aside → 3 tests fail with `read frozen oracle`. GREEN:
    package suite `-count=1` ok (scratch TMPDIR, 28.6s).
  - DUAL-TMPDIR check (worker + parent re-ran): focused tests green under
    default TMPDIR (/var/folders) AND scratch TMPDIR (/Users).
  - Mutation controls (pristine restore, sha identical): renamed record →
    identity mismatch; deleted record → `12 names, want 13`; one mutated
    marker byte → owning case fails (`bootstrap_marker_with_config_values`).
  - `gofmt -l internal/sync/` empty; `go vet ./internal/sync/` clean.
