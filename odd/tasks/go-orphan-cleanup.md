# Feature: go-orphan-cleanup

Move the authoritative orphan-cleanup decision of recipe materialization into
the Go gate binary (`worktree-gate`), keeping Python as a thin bridge that
executes the deletes and lock pruning (Python→Go strangler slice per
`follow-up/next-session-20260920`; smallest ranked seam from the read-only
map of `lib/_internal/recipe-materialize.py` + `lib/sync.sh`).

## Constraints

- Go owns the decision (which materialized ids are orphaned across recipe
  skills, dep skills, in-project deps, and stale lock entries); Python
  executes the deletes and `write_lock` (thin bridge, no decision logic).
- Recipes stay declarative; no new Go TOML dependency (ids are already read
  by Python `toml-read`; the envelope passes them over JSON stdin/stdout).
- Behavior parity with `clean_orphans` in
  `lib/_internal/recipe-materialize.py` (lines 1558–1601).
- Binary acquisition: reuse `gate_binary.resolve_verified_binary` +
  `_acquire_gate_binary` seam; degrade with one greppable fallback warning
  (mirror `GO_BINDINGS_BRIDGE_FALLBACK`) and keep the Python path as the
  temporary fail-open authority.
- RED/parity tests at the seam first (pattern: `tests/test_binding_bridge.py`);
  one authoritative grader per behavior.
- Bridge contract: bool flags passed as one `--flag=false` token (Go flag
  package quirk, recorded in session memory).

## Tasks

- [x] T1: Read-only map of materialization core + sync orchestration
      (subagent explore; no files written).
- [x] T2: RED — Go table tests for the orphan-plan decision mirroring
      `clean_orphans` semantics (enabled-vs-materialized set arithmetic,
      stale lock entries, empty/none-orphaned cases).
- [x] T3: GREEN — Go `--plan-orphans` command: read orphan inputs via JSON
      envelope, emit orphan plan. Work-unit commit `c474605`.
- [x] T4: Python bridge — `clean_orphans` calls the Go command via the gate
      binary; parity/contract tests at the seam (`tests/test_materialize_bridge.py`);
      legacy Python path reduced to fail-open fallback with warning
      (`GO_ORPHANS_BRIDGE_FALLBACK`). Work-unit commit `8f00b9f`.
- [x] T5: Full validation (`./tests/validate.sh`) green on the feature branch.

## Evidence

- Worktree: `.worktrees/go-orphan-cleanup`, branch `feat/go-orphan-cleanup`,
  base `1235b28` (development, origin-synced).
- T1 map findings: `clean_orphans` (recipe-materialize.py 1558–1601) is the
  only authoritative destructive decision without a Go counterpart; called
  from `materialize_recipes` L1755 (no-enabled path) and L1866 (enabled
  path); sync.sh L250 consumes only the JSON outputs, no shell change needed.
  Parity test template: `tests/test_binding_bridge.py` (`_GoBridgeTestCase`,
  `_forbid_python_authority`, case-by-case parity vs retained Python
  authority).
- Commits: T2+T3 `c474605` (gate source + tests + SHA256SUMS trust-root
  regen, canonical go1.24.13). T4 `8f00b9f` (Python bridge + parity tests,
  `GO_ORPHANS_BRIDGE_FALLBACK`, no Go source change). Full `./tests/run.sh`
  green after T4: 2234 tests OK (skipped=2). T2/T3 RED evidence: undefined
  orphanPlan/orphanPlanInput/planOrphans/runPlanOrphans build failure.
  T4 RED evidence: 12 bridge tests erroring on missing
  `_python_orphan_plan`/`go_orphan_plan` before implementation.
- T5 evidence: `./tests/validate.sh` exit 0 (verify agent, 2026-09-20):
  py_compile + bash -n + gofmt clean, go test gate packages ok, unittest
  2234 tests OK (skipped=2), ~11.7 min. No blockers.
- Native review: lineage `review-8ce721b04e58757c`, base `origin/development`
  committed-only, tier high (process_boundary), 7 paths / 1006 lines.
  4/4 lenses admitted, zero blockers; 6 informational findings recorded as
  follow-ups (rmtree trusts envelope names; sums self-attested regen; bridge
  race window; envelope element types; stale-lock return gate; lock order).
  State approved, acknowledge-approved burned (gentle-ai.review-acknowledged/v1).

## Tracker

- card_id: #142
- url: https://trello.com/c/0Qc7CAnK (list: In Progress)
