# GO-20 U2e T5 — PR2-B S4: freeze the agentsrender + briefgov families of Python oracles

Freeze every live Python consumer of `internal/sync/agentsrender_test.go` into
frozen golden bundles, same method as PR2-B S1-S3. The file spans two named
roadmap families (agentsrender `agentsrender_ref.py`, briefgov
`briefgov_ref.py`) plus the composed-step oracle (the real
`brief-render-policy.py` + `agents-render.py` sequence sync ran before T3), so
this slice retires both families. No `lib/` file is removed; product consumers
wait for PR4a/4b.

## Specs

- S1: "ok con que seguimos avanzando?" — continue the roadmap (Engram 377)
  after merging S3; Robert chose "Merge #483 y siguiente familia"
  (2026-11-02). Next family per epic order: agentsrender; briefgov rides the
  same test file and is frozen in the same slice.
- S2: `internal/sync/agentsrender_test.go` must stop execing `python3`
  (12 live execs: 2 ref drivers + the composed step sequence); equivalence is
  three frozen golden bundles captured once from the live Python 3.14.7
  oracle: `agentsrender_golden.json` (13 cases: 11 render + 2 error),
  `briefgov_golden.json` (13 governance cases), `agentsstep_golden.json`
  (11 composed-step cases; the 2 `policy_validate` cases are off the sync
  path and stay out, as today).
- S3: the three differential tests compare the Go leg against the frozen
  records; the `python3` skip guards disappear (including the vestigial guard
  on the Go-only `TestRenderAgentsStepPolicyFailurePreservesTheSilentSkip`).
- S4: capture reuses the real in-package helpers (`agentsRenderCases`,
  `briefGovCases`, `setupBriefGovDir`, `runAgentsRenderRef`, `runBriefGovRef`,
  `runPythonAgentsStep`) from a scratch, uncommitted Go test so the inputs are
  byte-identical to the live differential; captured strings are scanned for
  host paths (worktree/scratch/TMPDIR); none expected (the live test already
  compares outputs across different temp roots).
- S5: hard-coded identity inventory per bundle (exact case names in order,
  per-kind counts, no duplicates) asserted before the case loops; bundle
  tampering fails the suite; restore goldens by pristine copy, never JSON
  re-encode (sha check).
- S6: RED/GREEN per unit; `gofmt -l` clean; `go vet` clean; full
  `tests/validate.sh` green at the exact pushed head; `gentle_review assess`
  on the committed range; PR against `epic/go-single-binary`; stop before
  merge (review/merge is the human's).

## Tasks

- T1 (S2, S4): capture the three bundles via the scratch capture test.
  Commit: 987474f (bundles + rewrite) — capture: scratch
  `zz_scratch_capture_test.go` (deleted after use; bundles + pristine copies
  in scratch `captures/`).
- T2 (S2-S3): rewrite the three differential tests against the bundles; drop
  the python3 skips; delete the now-unused Python-leg helpers
  (`runAgentsRenderRef`, `runBriefGovRef`, `runPythonAgentsStep`,
  `briefGovRefInput`) keeping `materializeGateBinary` (other tests use it).
  Commit: 987474f.
- T3 (S5): identity inventories + tamper controls. Commit: 987474f.
- T4 (S6): docs L1+ per milestone; validate + assess + push + PR. Commit:
  docs.

## Log

- L1 — User request (verbatim): "ok con que seguimos avanzando?" with the
  choice "Merge #483 y siguiente familia", continuing the roadmap of Engram
  377 after PR #483 merged as f55ac6b.
- L2 — T1-T3 executed on `change/go-20-u2e-t5-pr2b-s4-agentsrender` (base
  epic `f55ac6b`). Code commit `987474f` (788+/230-).
  - Capture: scratch Go test (not committed) reused the real in-package
    helpers (`agentsRenderCases`, `briefGovCases`, `setupBriefGovDir`,
    `runAgentsRenderRef`, `runBriefGovRef`, `runPythonAgentsStep`) so the
    captured inputs are byte-identical to the live differential. Bundles:
    `agentsrender_golden.json` (13: 11 render + 2 ValueError records),
    `briefgov_golden.json` (13 governance records with policy/render surfaces,
    states, action, wrote, AGENTS.md/lock shas), `agentsstep_golden.json`
    (11 composed-step records with stdout/stderr/rc and AGENTS.md/lock bytes).
    Leak scan against TMPDIR/repo root: clean (the live test already compared
    outputs across different temp roots). Pristine copies in scratch
    `captures/`; restore by copy, never re-encode.
  - Freeze: the three differential tests consume the bundles via typed
    loaders; hard-coded identity inventories (13/13/11 names in order, no
    duplicates; per-kind pins: agentsrender errors=2, briefgov wrote=true=6,
    agentsstep AGENTS.md present=10) asserted before the loops. All four
    `python3` skip guards are gone (including the vestigial one on the
    Go-only `TestRenderAgentsStepPolicyFailurePreservesTheSilentSkip`).
    `agentsrender_ref.py` and `briefgov_ref.py` stay on disk until PR4.
  - RED: with the bundles moved aside, the three tests fail with "read frozen
    oracle" (3 occurrences). GREEN: package suite green with `-count=1`.
  - Mutation controls (restored pristine after each, shas identical):
    renamed agentsrender record → identity mismatch; deleted agentsstep
    record → count failure (`10 records, want 11`); mutated briefgov
    `render_stdout` → that case's render-stdout assertion fails. Intact
    bundles validate.
  - `gofmt -l internal/` clean; `go vet ./internal/sync/` clean.
