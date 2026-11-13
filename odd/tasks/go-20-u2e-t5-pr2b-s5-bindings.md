# GO-20 U2e T5 — PR2-B S5: freeze the bindings family of Python oracles

Freeze the live Python consumer of `internal/sync/bindings_test.go` (the
`testdata/bindings_ref.py` differential driver, which imports the real
`lib/_internal/recipe-materialize.py` authority) into a frozen golden bundle,
same method as PR2-B S1-S4. No `lib/` file is removed; product consumers wait
for PR4a/4b.

## Specs

- S1: "dale vayamos avanzando en un 'ciclo'" — Robert's standing cycle
  authorization (2026-11-02): implementación, validación y entrega (PR, push,
  merge autorizado, limpieza, pull, siguiente); un worker a la vez, un cambio
  a la vez; sin push: commit local + worktree stackeado sobre el anterior.
- S2: `internal/sync/bindings_test.go` must stop execing the live Python
  oracle (the driver is invoked through a `python` variable, not the literal
  `python3`; find every exec path, including git-dependent fixtures);
  equivalence is a frozen golden bundle
  `internal/sync/testdata/bindings_golden.json` captured once from the live
  Python 3.14.7 oracle.
- S3: the differential tests compare the Go leg against the frozen records;
  any `python`/oracle skip guards tied to the frozen leg disappear (keep the
  `git` availability guards of the git-topology fixtures — they are not
  Python guards).
- S4: capture reuses the real in-package helpers from a scratch, uncommitted
  Go test so the inputs are byte-identical to the live differential;
  captured strings are scanned for host paths (worktree/scratch/TMPDIR);
  tokenization only where a host path genuinely appears, with a recorded
  inventory.
- S5: hard-coded identity inventory (exact case names in order, per-kind
  counts, no duplicates) asserted before the case loops; bundle tampering
  (missing/renamed/duplicated record, mutated byte) fails the suite; restore
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
- L2 — T1-T3 executed by the scoped `gentle-ai-worker` (one worker at a time,
  per the S1 cycle), parent-verified. Parent commit (code) pending below.
  - Capture: scratch Go test (named `zz_scratch_capture_test.go`, deleted
    before commit) reused the real in-package helpers (`bindingsCatalog`,
    `bindingsResolveCases`, `bindingsWitnessCases`, `runBindingsRefRaw`) so
    the captured inputs are byte-identical to the live differential. Bundle:
    `testdata/bindings_golden.json` — resolve 9 records (6 differential + 3
    error, pin `errors=3`) + witness 7 records, with hard-coded `names`
    inventories asserted before the loops. Host-path scan: none (worktree,
    scratch, TMPDIR, capture catalog dir); no strings dropped. Pristine copy
    in scratch `captures/`, sha256 `96607919d64385ebe811a6655a170b2db9175cd79a5ede3b49836d1008a41810`.
  - Freeze: `bindingsRefPath` and `runBindingsRefRaw` deleted; the two
    differential tests consume the bundle; the git-availability guards of the
    topology fixtures are kept (they are not Python guards);
    `bindings_ref.py` stays on disk until PR4.
  - RED: bundle aside → both tests fail with `read frozen oracle`.
    GREEN: `go test -count=1 ./internal/sync/` ok (32.3s, fresh process).
  - Mutation controls (pristine restore after each, sha identical): renamed
    resolve record → identity mismatch; deleted witness record → `6 records
    for 7 names`; duplicated witness record → `8 records for 7 names`; one
    mutated witness byte → owning case fails on frozen sha. Intact bundle
    validates.
  - `gofmt -l internal/sync/` empty; `go vet ./internal/sync/` clean.
