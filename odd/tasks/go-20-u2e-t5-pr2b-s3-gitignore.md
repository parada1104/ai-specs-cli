# GO-20 U2e T5 — PR2-B S3: freeze the gitignore family of Python oracles

Freeze the live Python consumers of `internal/sync/gitignore_test.go` (the
`testdata/gitignore_ref.py` differential driver and the direct
`lib/_internal/gitignore-{render,root-refresh}.py` execs) into a frozen golden
bundle, same method as PR2-B S1/S2. No `lib/` file is removed; product
consumers (`lib/sync.sh`, `lib/sync-agent.sh`, `lib/init.sh`) wait for PR4a/4b.

## Specs

- S1: "si, vamos a ello" — proceed with the agreed roadmap unit: freeze the
  gitignore family oracle (Robert, 2026-11-02, executing the roadmap of
  Engram 377 / epic doc L60 option A).
- S2: `internal/sync/gitignore_test.go` must stop execing `python3`
  (8 live execs today: ref driver + direct module runs); equivalence is a
  frozen golden bundle `internal/sync/testdata/gitignore_golden.json`
  captured once from the live Python 3.14.7 oracle.
- S3: `TestGitignoreDifferential` (11 cases), `TestGitignoreDifferentialUnmatchedBeginMarker`
  and `TestGitignoreDifferentialReadFailures` (2 subtests) compare the Go leg
  against the frozen records; the `python3` skip guards disappear.
- S4: host-bound paths in frozen stdout/stderr are tokenized (`<DIR>`) with a
  position/field inventory recorded in the capture evidence; no host path
  inside the committed bundle.
- S5: hard-coded identity inventory (exact case names in order, per-mode
  counts, no duplicates) asserted before the case loops; bundle tampering
  (missing/renamed/duplicated record) fails the suite; restore goldens by
  pristine copy, never JSON re-encode (sha check).
- S6: RED/GREEN per unit; `gofmt -l` clean; `go vet` clean; full
  `tests/validate.sh` green at the exact pushed head; `gentle_review assess`
  on the committed range; PR against `epic/go-single-binary`; stop before
  merge (review/merge is the human's).

## Tasks

- T1 (S2-S5): capture golden bundle from the live oracle (scratch capture
  script, not committed; umask replicated per case). Commit: 13579da
  (bundle) — capture: `go20-t5-pr2b-s3-gitignore/capture_gitignore.py`.
- T2 (S2-S3): rewrite the three tests to consume the frozen bundle; drop the
  python3 skips. Commit: 13579da.
- T3 (S4-S5): tokenization + identity inventory + in-memory tamper controls.
  Commit: 13579da.
- T4 (S6): docs L1+ appended per milestone; validate + assess + push + PR.
  Commit: docs.

## Log

- L1 — User request (verbatim): "si, vamos a ello" — following the proposal
  "abrir la sesión de la primera familia (propongo materialize o gitignore
  como quick win...)" and Robert's earlier instruction "ok vamos con el merge
  y establezcamos un roadmap para ir cerrando la migracióon por favor".
- L2 — T1-T3 executed on `change/go-20-u2e-t5-pr2b-s3-gitignore` (base epic
  `26c8499`). Code commit `13579da` (374+/173-).
  - Capture: scratch `capture_gitignore.py` (not committed) ran the 15-case
    matrix against the live python3 3.14.7 oracle, replicating each case's
    prepare and umask. Tokenization: 6 `<DIR>` positions (5 render stdout
    output-path, 1 missing-template stderr template path); the three
    module-direct records drop stderr (uncaught tracebacks embed host paths;
    same scope as the live test, which never compared that stderr) — recorded
    in the capture inventory as DROPPED. No host path survives tokenization
    (script asserts against worktree/scratch/TMPDIR roots).
  - Freeze: `TestGitignoreDifferential` (12 cases),
    `TestGitignoreDifferentialUnmatchedBeginMarker` and
    `TestGitignoreDifferentialReadFailures` (2 subtests) now consume
    `testdata/gitignore_golden.json` via `loadGitignoreGolden`, which asserts
    the hard-coded identity inventory (15 names in order, per-kind counts
    5/7/1/1/1, no duplicates) before any case loop. The three `python3` skip
    guards are gone; `gitignore_ref.py` stays on disk until PR4 (S2
    convention: frozen `_ref.py` files are not removed by the freeze).
  - RED: with the bundle moved aside, all three tests fail with "read frozen
    oracle: open testdata/gitignore_golden.json: no such file or directory".
  - GREEN: all 12 cases + 3 tests pass in a fresh process.
  - Mutation controls (bundle restored from the pristine scratch copy after
    each; sha256 `adec1f1b…` identical before/after): a renamed record fails
    the identity check; a deleted record fails the count check; one mutated
    artifact byte fails that case's sha256 assertion. Intact bundle validates.
  - `gofmt -l internal/` clean; `go vet ./internal/sync/` clean.
  - Known environmental failure, not caused by this slice:
    `TestUpgradeUsageMatchesTheShellSurface` in `internal/cli` fails from a
    bare worktree because the legacy leg needs `dist/ai-specs` (or
    `AI_SPECS_GO_BIN`); validate.sh builds the binaries it needs.
