# Feature: go-classify-override

Strangler slice Rank 3: migrate the `classify_managed_override` grader into Go,
plus the `ai_specs_home` forwarding fix folded in by user decision (2026-09-21).
Worktree: `.worktrees/go-classify-override` — branch `feat/go-classify-override`
from development @ `9fb6e9d`. Budget: native-review candidate < ~1000 changed
lines; split into chained PRs only if exceeded.

## Tasks

- [x] T1 — Go `--plan-classify` subcommand: pure SHA-256 classification
      (missing/untracked/user_modified/managed_current/managed_stale) with a
      JSON envelope, following the established `--plan-orphans` /
      `--plan-resolved-config` pattern. Go unit tests RED→GREEN.
- [x] T2 — Python bridge fail-open: `util.classify_managed_override` delegates
      to the Go binary first (single `GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK`
      warning line, never raises); Python stays as fallback only. Regen
      `catalog/recipes/worktree-flow/bin/SHA256SUMS` + digest tests in the same
      slice.
- [x] T3 — Parity suite with `_forbid_python_authority` semantics (style of
      `tests/test_materialize_bridge.py`) pinning the Go/Python contract at the
      classify seam.
- [x] T4 — Fix `ai_specs_home` forwarding at `lib/_internal/recipe-materialize.py:2031`
      and `:2251` (`build_resolved_config(project_root)` → pass `ai_specs_home`)
      + tests. User decision: addressed inside this slice, not deferred.
- [x] T5 — Docs/bridge contract touchpoint + full `./tests/validate.sh` green +
      final work-unit commit.

## Conventions

- Conventional Commits, no AI attribution.
- One work-unit commit per task with tests/docs alongside behavior.
- Red-green-refactor; evidence recorded per task.

## Evidence

- T1 commit `e223ba6` feat(gate): add Go --plan-classify managed-override
  projection — RED: undefined symbols in classify_test.go before
  implementation; GREEN: `go test ./...` ok (worktree-gate + ledger), 15
  subtests pass including CRLF normalization and CLI envelope.
- T2+T3 commit `c3dd891` feat(override): bridge classify_managed_override
  decision to Go fail-open — util.classify_managed_override delegates to
  `--plan-classify`, one `GO_CLASSIFY_OVERRIDE_BRIDGE_FALLBACK` line per
  degraded run, Python retained as `_python_classify_managed_override`;
  SHA256SUMS regenerated (go1.24.13) with re-pinned matrix digests; parity
  suite tests/test_classify_bridge.py with _forbid_python_authority
  semantics. RED: 15 failed pre-bridge; GREEN: focused suites 60 passed.
- T4 commit `6dff0ff` fix(materialize): forward ai_specs_home to
  build_resolved_config call sites — lines 2031/2251 forwarding pinned by
  two regression tests (both branches); RED 2 failed → GREEN, focused file
  71 passed.
- T5 full validation: `./tests/validate.sh` exit 0 — `Ran 2253 tests ... OK
  (skipped=2)` (2 pre-existing skips).
- Deviations worth recording: flags.go untouched (repo convention registers
  flags in main.go); `sha256Bytes` added in classify.go (Python-parity
  CRLF→LF normalization; no pre-existing hashing helper); util.py loads
  gate_binary.py lazily by path to avoid the circular import
  (recipe-materialize loads util via _load_util); would_write travels as
  UTF-8 text (non-UTF-8 degrades to the Python fallback).
