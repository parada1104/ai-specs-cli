# Go 07 S13 PR2b — Merge-config root authority

## Objective
Give the root single binary a native merge-config authority that calls the shared core extracted in
PR2a, so the root and the gate binary run one merge decision instead of two. No gate contract
changes: this delivery only adds the root side, the adapter it needs, and the differential that
proves parity with the Python authority.

**Delivery B** of S13 PR2 (A = the shared extraction, merged as `fe6e7cf`). Nothing is wired into the
sync pipeline: S14 owns the wiring.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no card could be
  created or linked for this slice.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed
  without recording a decision. `tracker.none` is presentation and evidence only and is never written
  by the host, so none was created here.
- Close-out follow-up: link the S13 card in this section once the tracker is reachable.

## Scope
- Base: `fe6e7cf` on `epic/go-single-binary` (S13 PR2a merged).
- Worktree: `.worktrees/go-07-s13-mergeconfig-root`. Branch: `change/go-07-s13-mergeconfig-root`.
- Add the root native authority `internal/sync/mergeconfig.go`: root-owned boundary types,
  `PlanMergeConfig` delegating to `shared.PlanMergeConfig`, and the adapter that converts the native
  inputs into the request the core decodes.
- Add the minimum exported surface in `internal/schema` that the adapter needs, plus its focused test.
  This surface extension was authorized by the human **for this delivery only**.
- Add the Python differential `internal/sync/mergeconfig_test.go` and
  `internal/sync/testdata/mergeconfig_ref.py`.
- No gate change is expected in this delivery; if the shared core genuinely must change, STOP and
  report instead of editing it silently (that would require a fresh trust-root regeneration and a
  fresh review of a different candidate).

## The three recorded blockers this delivery must solve
1. **Structured shape export.** `internal/schema.ConfigTable.Shape` is backed by an unexported
   `*shapeTable` (`internal/schema/recipe.go:58`, `:75`, `:157`) and `encoding/json.Marshal` of it
   yields `{}`, so the adapter cannot produce the shape the shared request needs. Add the smallest
   order-preserving exported accessor that solves it (for example a method on `ConfigTable` or a
   package-level `StructuredShapeJSON(section string) (json.RawMessage, error)`), with a focused test.
   Derive the exact required shape from the Python envelope builder `_merge_config_request_envelope`
   (`lib/_internal/recipe-materialize.py:2842-2881`) and from `shared.MergeConfigRequest`.
2. **Field order.** `internal/schema.ConfigSchema.Fields` is a map, so declaration order is lost while
   the Python envelope preserves it. The adapter must recover order by re-parsing the recipe's
   `recipe.toml` through `internal/toml` (already an allowed surface), not by guessing.
3. **Python float repr.** `json.dumps(..., allow_nan=False)` emits `1.0` for a float where Go
   `encoding/json` emits `1`. The adapter needs a Python-compatible float formatter so int/float
   survives the shared `json.Number` path; NaN and Infinity must be rejected exactly as Python does.

## In-scope units
- Root: `internal/sync/mergeconfig.go` (new).
- Schema surface: `internal/schema/recipe.go` plus one focused test file.
- Tests: `internal/sync/mergeconfig_test.go`, `internal/sync/testdata/mergeconfig_ref.py`.
- No Python production change, no CLI wiring, no dependency change.

## Python seam (the oracle, `lib/_internal/recipe-materialize.py`)
`GO_MERGE_CONFIG_BRIDGE_FALLBACK` :2830, `_merge_config_request_envelope` :2842-2881,
`go_merge_config` :2897-2975 (flag `--plan-merge-config` :2946; stdout `{config, warnings, error}` as
one JSON object; exit 0 with a non-empty `error` for a semantic failure and **no fallback** in that
case; transport/mismatch fall back with one warning), `merge_config` :2978-3003 (raises
`RuntimeError` on `error`), and the fail-open body `_python_merge_config` :3006-3068 that the
differential must use as its oracle.

## Constraints
- Budget: **1200 additions plus deletions**. If the work is heading past 1200, STOP and report the
  measured number with the remaining work instead of exceeding it.
- The differential must exercise the REAL Python bodies by importing
  `lib/_internal/recipe-materialize.py` (the shape `internal/sync/testdata/bindings_ref.py` uses),
  never a re-implementation.
- Byte contract: key order, table order, every message and error string, the `allow_nan=False`
  behavior and the semantic-error-versus-transport-error split must match the Python authority.
- Do not unify the two pre-existing repr ports (`internal/schema/recipe.go:301-467`,
  `internal/config/config.go:176-257`) with the shared one; that is later work.
- Do NOT wire the authority into `internal/sync/sync.go`; S14 owns the wiring.
- Existing tests must not be weakened or deleted; the `internal/schema` change must be additive.
- Do not modify `development` and do not publish a release.
- Parent owns the commit, push, PR, merge and cleanup.

## Tasks
- [ ] T1: Derive the exact request the shared core needs from `shared.MergeConfigRequest`,
      `_merge_config_request_envelope` and `_python_merge_config`, and write that down before coding.
- [ ] T2: Add the minimum exported, order-preserving shape accessor in `internal/schema` with its test.
- [ ] T3: Add `internal/sync/mergeconfig.go`: root boundary types, `PlanMergeConfig`, and the adapter
      (field order via `internal/toml`, Python-compatible float formatting, NaN/Infinity rejection).
- [ ] T4: Add `internal/sync/mergeconfig_test.go` and `internal/sync/testdata/mergeconfig_ref.py`:
      a real differential against `_python_merge_config` and the envelope builder, plus adapter
      coverage over real `internal/schema`, `internal/config` and `internal/toml` inputs.
- [ ] T5: Run the full check set and report real exit codes.

## Acceptance criteria
- The root authority accepts the same inputs and produces the same merged config, warnings and error
  string as the Python authority for every covered scenario, including key/table order and the
  datetime and float boundaries.
- A semantic error is data (`error` set, no exception) exactly as in the gateway contract; NaN and
  Infinity are rejected as Python rejects them.
- The `internal/schema` change is additive: no existing exported signature changes and the existing
  schema tests pass unchanged.
- The sync pipeline is untouched (it still execs Python).
- The complete authored candidate stays within the 1200-line budget.

## Checks
- Root: `go test ./cmd/... ./internal/... -count=1`, `gofmt -l .`, `git diff --check`.
- Schema: `go test ./internal/schema/... -count=1` plus the differential driver it already has.
- Bridges (regression): `python3 -m unittest tests.test_merge_config_bridge`.
- Gate (regression, unchanged): `go -C catalog/recipes/worktree-flow/gate test ./... -count=1`.
- Full: `./tests/validate.sh` with durable logs and captured exit evidence (run by the parent).
- Environment: the real toolchains with the Python 3.14.7 oracle prefixed on PATH
  (`~/.local/share/mise/installs/{go/1.24.13,python/3.14.7}/bin`), never the mise shims.
- Native review uses the exact committed slice base `fe6e7cf`, not the whole epic.

## Progress
- Slice prepared: worktree and branch created at base `fe6e7cf`.
- Carried from PR2a: `Keys()`/`Values()` on `shared.OrderedMap` expose aliased storage
  (`shared/orderedmap.go:34-41`, review advisory `R3-aliased-accessors`). Read-only use is fine; a new
  consumer must not mutate through them.
- Independent verification: **PASS** on the static claims, the adapter fidelity, the honest-differential
  check and the suite runs, with three findings. It also confirmed the `internal/schema` change is
  strictly additive (63 insertions, zero deletions), that the gate and `internal/sync/sync.go` are
  untouched, that `ShapeJSON` has no panic path, and — constructively — that the request/result parity
  holds on seven scenarios the suite did not cover (non-ASCII keys, array-of-tables manifest values,
  `[[...]]` structured sections, nested unknown keys, a 34-literal float-repr sweep across the 1e15/1e16
  and subnormal thresholds, dotted-key structured values).
- **Recorded divergence (pre-existing, not introduced here): empty config key.** The root authority
  propagates the shared core's process failure (`manifest entry missing key` / `field entry missing
  key`) where the Python fallback warns and continues (`unknown config key '' in manifest (ignored)`).
  The adapter faithfully sends what the Python envelope builder sends, so this is a gate-core-versus-
  Python-fallback divergence. It is now pinned on BOTH sides by an explicit, self-describing exclusion
  (`mergeConfigDivergenceEmptyKey`) that forbids claiming parity, mirroring the `conflicts_error`
  precedent in `internal/sync/bindings_test.go:138-152`. A gate-core fix belongs to its own gate slice,
  which would also require a fresh trust-root regeneration.
- The non-finite assertion was softened (a hardcoded `strings.Contains` plus an `exc_type`-only Python
  comparison). It now pins both LIVE texts and asserts deliberately that they are not byte-equal: the
  Python `ValueError` (`Out of range float values are not JSON compliant: nan`, a transport failure that
  never crosses the wire) versus the adapter's acquisition-time message (`manifest "ratio" value: Out of
  range float values are not JSON compliant`).
- Coverage caveat resolved: `tests.test_merge_config_bridge` reports only 12 tests with 3 class-level
  skips in a worktree without a built gate binary, and the 35 skipped tests are exactly the
  merge-decision classes (parity, transport, datetime), which is why this document requires the full
  validation run rather than the bare unittest call as the regression evidence.
- Budget after the follow-up: **1078** code lines (1196 including this document), inside the 1200 limit.
- Coverage gap closed before commit: the first full validation run in this worktree reported
  `Ran 2380 tests ... OK (skipped=164)` because no gate binary resolved, while the two previous slice
  worktrees reported 2482. The 102-test difference is the binary-backed bridge classes going dark
  (`tests/test_merge_config_bridge.py:65-74` resolves `$WORKTREE_GATE_BIN` or
  `dist/worktree-gate-current`). Building the gate with the canonical go1.24.13 toolchain into the
  gitignored `dist/` and re-running the twelve bridge modules turned every one of them green with
  zero skips, and `tests.test_merge_config_bridge` went from 12 tests with 3 class-level skips to
  **44 tests with 0 skips** — the previously dark classes are exactly the parity, transport and
  datetime-boundary coverage this delivery needs. The root differential itself ran 4 top-level tests
  and 15 subtests, including both recorded divergences.
- Second full validation with the gate binary present: `./tests/validate.sh` exit 0 with
  `Ran 2482 tests in 1600.979s` then `OK`, and zero `FAIL:`/`ERROR:` lines — the same count the two
  previous slice worktrees reported, so the binary-backed classes are genuinely included. Evidence:
  `~/.cache/s13-pr2b-validate2.log` and `~/.cache/s13-pr2b-validate2.exit` (the earlier binary-absent
  run is kept at `~/.cache/s13-pr2b-validate.{log,exit}` as the floor).
- Work unit commit: recorded below.

## Rollback
Revert this delivery's root authority, the additive `internal/schema` accessor, its tests and this
document together. PR3 (resolved-config) depends on the same shared surface, not on this file.
