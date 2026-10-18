# Feature: go-binding-witness

Move capability binding resolution, capability conflict grading, and durable
tracker witness production into the Go gate binary (`worktree-gate`), keeping
Python as a thin TOML/JSON acquisition bridge (single-binary strangler slice
after PR #275, per `architecture/go-migration-next-slices`).

## Constraints

- Go owns the decision (binding resolution + conflict grading) and the durable
  state (witness.json write). Python owns TOML acquisition only.
- No new Go TOML parser and no new Go dependency: reuse the embedded `tomllib`
  acquisition seam from `ledger_reconcile.go` (recorded invariant).
- Witness payload schema (`v`, `capability`, `state`, `recipe_id`,
  `candidates`, `written_at`) and state semantics (bound/ambiguous/unbound/
  declared-not-bound; D6 supply-not-activation) must not change.
- Behavior parity with `resolve_bindings`, `check_capability_conflicts`, and
  `tracker_witness_payload`/`write_tracker_witness` in
  `lib/_internal/recipe-materialize.py` and `lib/_internal/recipe-conflicts.py`.
- Binary acquisition: sync must acquire the gate binary for this step even when
  worktree-flow is disabled; degrade with warning (bridge stays available).
- RED/parity tests at every migrated seam; one authoritative grader.

## Tasks

- [x] T1: Read-only map of capability binding/conflict + witness production
      (recorded in session memory; no files written).
- [x] T2: RED — Go table tests for binding resolution and capability conflict
      grading mirroring Python semantics (explicit binding validation, duplicate
      fatal, ambiguity warning, auto-bind single provider).
- [x] T3: GREEN — Go `--resolve-bindings` command: acquire capability
      declarations via the tomllib seam, resolve bindings, grade conflicts,
      emit JSON result. Work-unit commit `b09af1a`.
- [x] T4: RED/GREEN — Go witness payload + atomic durable write
      (`<git-common-dir>/ai-specs/ledger/witness.json`), same schema and
      atomicity as the Python writer. Work-unit commit `0474c07`.
- [x] T5: Python bridge — `recipe-materialize.py` and `doctor.py` call the Go
      command via the gate binary; parity/contract tests at the seam; legacy
      Python path reduced to a fail-open temporary bridge with warning.
      Work-unit commit `15a5166`.
- [x] T6: Binary acquisition for the binding step independent of worktree-flow
      enablement; degradation warning recorded; bridge tests. Work-unit commits
      `9c33f06` (acquisition seam + hermetic test suite) and `9bea7a1`
      (SHA256SUMS trust-root regeneration, canonical go1.24.13).
- [ ] T7: Full validation (`./tests/validate.sh`) green on the feature branch.

## Evidence

- Worktree: `.worktrees/go-binding-witness`, branch `feat/go-binding-witness`,
  base `070520a` (development, origin-synced).
- Commits: T2+T3 `b09af1a`; T4 `0474c07`; T5 `15a5166`; T6 `9c33f06` +
  `9bea7a1` (Conventional Commits, tests alongside behavior).
- T2/T3 worker evidence: RED build-failure (undefined graders), GREEN
  `go test -count=1 ./...` ok (main + ledger packages), gofmt/vet clean.
  Known scoped boundary: Go validates the capabilities block only, not the
  whole recipe schema (later slice owns whole-recipe validation).
- T1 map findings: binding/conflict authority in `recipe-conflicts.py` +
  `recipe-materialize.py` (`resolve_bindings`, `tracker_witness_payload`,
  `write_tracker_witness`); Go side is reader-only (`gate/ledger/witness.go`,
  dormant on missing/bad witness); no Go TOML parser (embedded tomllib seam in
  `ledger_reconcile.go`); additional consumer `doctor.py:512`.
- Acquisition risk to resolve in T6: acquire binary for sync binding step
  regardless of worktree-flow enablement; fail-open warning otherwise.
