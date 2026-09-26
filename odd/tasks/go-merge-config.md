# go-merge-config

## Objective

Migrate the `merge_config` decision (recipe config schema defaults + manifest
overrides + structured-config validation) from Python
(`lib/_internal/recipe-materialize.py:1473`, `lib/_internal/recipe_schema.py`)
into the existing Go gate binary as a pure stdin `--plan-merge-config` command
in `catalog/recipes/worktree-flow/gate/`. Schema acquisition stays Python
(already-loaded `Recipe`); Go owns the decision.

## Constraints

- Work only in this worktree (`feat/go-merge-config` from development `8021fe3`).
- No commits/push/PR/merge/native review from this worker; parent owns git.
- Envelope contract (stdin): `recipe_name`, ordered `fields`
  `[{key, required, has_default, default, enum}]`, ordered `tables`
  `[{key, shape}]`, ordered `manifest` pairs `[{key, value}]`.
- Envelope contract (stdout): ordered `config` object, `warnings` (inner warn
  text, Python formatting stays in the bridge), `error` (empty when ok).
- Semantic validation errors (required/enum/structured) exit **0** with `error`
  set (must not trigger the bridge fallback); malformed request / process
  errors exit **2**.
- Ordered parity: never rely on Go map iteration — ordered decode/encode for
  fields, manifest pairs, structured values, shapes, and merged config.
- Python value semantics: `json.Number` decoding, bool/int/float distinctions,
  `str()` exact (`True`/`False`, canonical number literals), required = presence
  not truthiness, defaults exclude only `None` (`has_default` transport),
  `gate_scope` blank/whitespace/falsy → `auto`.
- Structured grammar: scalar `string|integer|boolean`, tables (unknown-key
  errors in value order, sub-checks in shape order), lists capped at 32, Python
  type names and message text byte-identical.
- No TOML re-read, no classifier duplication; schema port constrained to this
  merge decision.
- Do not edit the Python bridge in WU1.

## Tracker

- card_id: 6ab0a5124574ef6689258890
- url: https://trello.com/c/KjIIxJBH/145-go-03-migrate-mergeconfig-decision-to-go
- list: In Progress

## TDD

- Enabled by AGENTS.md (red-green-refactor mandatory).
- Runner: `./tests/validate.sh` (full validation, deferred to WU2+WU3);
  focused: `cd catalog/recipes/worktree-flow/gate && go test -count=1 -run MergeConfig ./...`.

## Work Units

- **WU1 — Go core + tests** (this task, in progress): `mergeconfig.go`,
  `mergeconfig_test.go`, `main.go` flag wiring, this document.
- **WU2 — Python bridge + parity + digests** (bridge + parity done in this
  unit; digests/full `./tests/validate.sh` deferred to WU3): envelope builder in
  the Python materialize path calling the Go binary, warn/fail formatting,
  parity tests, full `./tests/validate.sh`.
- **WU3 — delivery** (pending): two chained PRs (WU1 Go core; WU2 bridge),
  coherent >400-line slices accepted, no cosmetic compression.

## Route Triggers

- Multi-file write rule fired (3 files in WU1) → delegated to gentle-ai-worker
  by the parent; single writer thread, allowed surfaces only:
  `odd/tasks/go-merge-config.md`,
  `catalog/recipes/worktree-flow/gate/mergeconfig.go`,
  `catalog/recipes/worktree-flow/gate/mergeconfig_test.go`,
  `catalog/recipes/worktree-flow/gate/main.go`.

## Delivery Strategy

Two chained PRs per the human decision: PR1 = WU1 (Go core + tests, no Python
edits), PR2 = WU2 (Python bridge/parity/digests). Parent owns git actions.

## Forecast

Aggregate 950–1250 lines; WU1 550–750 lines (impl + tests + task doc).

## Progress

- [x] Explore Python reference and Go CLI patterns.
- [x] Write task document (this file).
- [x] WU1 RED: 15 tests / 43 cases fail behaviorally (flag undefined → fail-open,
      no envelope) before implementation.
- [x] WU1 GREEN: `--plan-merge-config` implemented, focused suite green.
- [x] WU1 verification: focused suite, full package suite, `git diff --check`,
      gofmt clean — all observed passing (see Evidence).
- [x] WU1 complete, awaiting parent git ownership (no commits made).
- [x] WU1 independent verification PASS (worker-observed evidence, see WU2
      Evidence section below): focused/full Go tests and bounded differential
      probes; the delivery slicing remains parent-owned and unresolved.
- [x] WU2 RED: 36/36 tests in tests/test_merge_config_bridge.py error on the
      missing bridge seams (`_python_merge_config`,
      `GO_MERGE_CONFIG_BRIDGE_FALLBACK`) before implementation.
- [x] WU2 GREEN: bridge implemented; 36/36 pass against the real temp-built
      binary; combined focused run 223 tests OK (4 pre-existing dist skips).
- [x] WU2 independent verification PASS (parent-verified: 226 tests OK, four
      missing-dist skips; bridge 39 tests zero skips).
- [x] WU3 build/digests/full validation complete (see WU3 Evidence below).
- [ ] WU3 delivery: native reviews + PR slicing — PENDING (parent-owned;
      aggregate diff exceeds two forecast slices, slicing unresolved).

## Evidence

- WU1 RED: `cd catalog/recipes/worktree-flow/gate && go test -count=1 -run MergeConfig ./...`
  → FAIL, 15 top-level tests / 43 cases, all on
  "flag provided but not defined: -plan-merge-config" (fail-open exit 0, empty
  stdout). One earlier build-fail run on the type-only stub preceded it.
- WU1 GREEN: same command → `ok ai-specs.dev/worktree-gate` after implementation.
- WU1: `go test -count=1 ./...` → gate ok (38.1s), ledger ok. `git diff --check` →
  clean. `gofmt -l` → no output.
- WU1 full `./tests/validate.sh` NOT run — deferred to WU2+WU3 per plan.
- WU1 independent verification (parent-reported): three failed verifier attempts
  (environment/tooling issues, no verdict) followed by a successful fourth
  attempt that yielded PASS evidence: focused and full Go test suites plus
  bounded differential probes comparing Go and Python decisions on real recipe
  schemas. WU1 diff measured ~1152 lines including docs; review slicing (how to
  split past the 400-line guidance) remains parent-owned and UNRESOLVED — the
  prior two-PR plan is recorded but does not by itself solve the sizing.

### WU2 evidence

- Envelope/bridge built per WU1 handoff notes; Python acquisition kept, Go owns
  the decision. `merge_config` is now the Go-first bridge; the original body is
  renamed `_python_merge_config` (TEMPORARY fail-open fallback,
  `GO_MERGE_CONFIG_BRIDGE_FALLBACK`, one warning per degraded run).
- Transport semantics implemented: unavailable/nonzero/timeout/non-JSON/
  malformed-envelope/serialization errors each fall back exactly once with one
  warning; the whole response is validated before any warning is emitted, so a
  degraded run never emits partial output. Semantic errors (`error` set, exit 0)
  raise RuntimeError without fallback. Values cross the JSON boundary verbatim:
  anything JSON cannot carry (TOML date/datetime/time, nonfinite floats) fails
  envelope serialization (`json.dumps(allow_nan=False)` / TypeError) and the
  whole merge falls back once, preserving the Python authority's typed values
  and its own validation failures. No coercion at the boundary.
- WU2 RED: `WORKTREE_GATE_BIN=<temp binary> python3 -m unittest
  tests.test_merge_config_bridge` → FAILED (errors=36), all on
  `AttributeError: ... does not have the attribute '_python_merge_config'`.
- WU2 RED (correction round):
  `WORKTREE_GATE_BIN=<temp binary> python3 -m unittest
  tests.test_merge_config_bridge.DatetimeTransportBoundaryTests` → FAILED
  (failures=4): the interim str() coercion returned `'2024-01-02 03:04:05'` for
  a datetime manifest value and WRONGLY ACCEPTED a datetime supplied to a
  structured string field instead of raising `expected string, got datetime`.
  One test-helper message-extraction fix followed; behavior verified unchanged.
- WU2 GREEN (correction round): same command → OK (4/4). Full bridge suite
  `python3 -m unittest tests.test_merge_config_bridge` → `Ran 39 tests ... OK`,
  0 skips.
- Temp binary for parity (default digest binary does not exist in this
  worktree; no SHA256SUMS regenerated): `cd catalog/recipes/worktree-flow/gate
  && go build -o "${TMPDIR:-/tmp}/go-merge-config-wu2/worktree-gate" .`
  then `WORKTREE_GATE_BIN="${TMPDIR:-/tmp}/go-merge-config-wu2/worktree-gate"
  python3 -m unittest tests.test_merge_config_bridge ...` (see verification
  commands below).
- Combined focused run WITHOUT a binary → `Ran 195 tests ... OK (skipped=6)`
  (2 loud skips in the new file's Go-path classes + 4 pre-existing
  "no Go gate binary in dist/" skips).
- Combined focused run WITH the freshly rebuilt temp binary (correction round)
  → `Ran 226 tests ... OK (skipped=4)`, all 4 skips pre-existing dist-resolution
  tests unrelated to this unit; new file contributes 0 skips (no skipped parity
  claimed).
- `git diff --check` → clean (exit 0).
- Full `./tests/validate.sh` NOT run — WU3 owns digest/full validation; not
  claimed here.

### WU2 verification commands (observed)

- `python3 -m unittest tests.test_merge_config_bridge tests.test_recipe_materialize
  tests.test_worktree_gate_dist_config tests.test_worktree_flow_recipe
  tests.test_plan_build_flow_recipe tests.test_trello_mcp_workflow_recipe`
  → exit 0, `Ran 195 tests ... OK (skipped=6)`.
- `WORKTREE_GATE_BIN="${TMPDIR:-/tmp}/go-merge-config-wu2/worktree-gate"` + the
  same command → exit 0, `Ran 226 tests ... OK (skipped=4)` (pre-existing dist
  skips only).
- `git diff --check` → exit 0, no output.
- `go build -o "${TMPDIR:-/tmp}/go-merge-config-wu2/worktree-gate" .` in
  `catalog/recipes/worktree-flow/gate` → success; the binary answers
  `--plan-merge-config` with `{"config":{},"warnings":[],"error":""}`.

### WU2 limitations

- JSON transport boundary (correction round, honest statement): the envelope
  carries JSON-serializable values only. TOML date/datetime/time manifest or
  default values and nonfinite floats (inf/nan) cannot cross; the bridge falls
  back exactly once and the Python authority keeps the original typed objects
  and its own semantic validation (a datetime supplied to a structured string
  field remains a Python type error — it is never coerced to accepted text).
  A prior interim str()-coercion approach was a parity defect and was retracted
  per parent correction; the previous "date-string parity" claim no longer
  stands.

- Python acquisition is unchanged. The only value-level behavior at the JSON
  boundary is the documented fallback above (typed retention, no coercion); no
  values are transformed on either path.
- The parity suite runs against whatever binary `WORKTREE_GATE_BIN` (or
  `dist/worktree-gate-current`) resolves; without any binary the Go-path classes
  skip loudly and the fallback tests still run. Digest/full validation and the
  rebuilt dist binary are WU3.
- Review slicing for the combined WU1+WU2 diff (well past 400 lines) remains an
  open parent decision; WU2 adds ~169 lines to recipe-materialize.py plus a
  760-line test file on top of the measured ~1152-line WU1 diff.

## WU3 Evidence (observed)

- Canonical toolchain: `go version` → `go version go1.24.13 darwin/arm64`
  (mise-managed, `/Users/robert/.local/share/mise/installs/go/1.24.13/bin/go`,
  already active — nothing installed or downloaded). The build emitted NO
  non-canonical-toolchain warning.
- Build: `scripts/build-gate.sh` (env: `go` resolved to the canonical 1.24.13
  shim; cwd = worktree root) → exit 0, "done — 4 targets built", VERSION 0.24.0,
  `dist/worktree-gate-current` created on darwin/arm64 as the native copy.
  Log: `$TMPDIR/wu3-build-gate.log`.
- Digests regenerated in `catalog/recipes/worktree-flow/bin/SHA256SUMS`
  preserving format/order (darwin-amd64, darwin-arm64, linux-amd64,
  linux-arm64) plus the conventional regeneration comment, from
  `shasum -a 256` of the four built outputs (no guessed hashes):
  darwin-amd64 80c4081e…, darwin-arm64 04a6a143…, linux-amd64 7dba1f48…,
  linux-arm64 a6d8f2fa…. One transcription error during the edit (extra
  leading `a` on each digest line) was caught and corrected before any
  verification ran.
- Independent comparison: `(cd dist && shasum -a 256 -c
  ../catalog/recipes/worktree-flow/bin/SHA256SUMS)` → all four `OK`, exit 0.
- Focused suite WITHOUT `WORKTREE_GATE_BIN` (default binary resolution via
  `dist/worktree-gate-current`): `python3 -m unittest
  tests.test_merge_config_bridge tests.test_recipe_materialize
  tests.test_worktree_gate_dist_config tests.test_worktree_flow_recipe
  tests.test_plan_build_flow_recipe tests.test_trello_mcp_workflow_recipe` →
  exit 0, `Ran 226 tests in 41.069s — OK`, ZERO unittest skips (the four
  pre-existing dist skips are resolved by the built binary; remaining
  "skipped" lines in the log are recipe-validation output, not skips).
  Log: `$TMPDIR/wu3-focused-tests.log`.
- Full validation: `perl -e '$SIG{ALRM}=sub{exit 142}; alarm 900; exec @ARGV'
  ./tests/validate.sh` → exit 0, `Ran 2302 tests in 686.614s — OK
  (skipped=2)`. The two skips are environmental (config-wizard harness-env
  offer path; exact names not printed by the runner). Log:
  `$TMPDIR/wu3-validate-full.log` (8471 lines).
- `git diff --check` → exit 0, no output.
- Git scope after WU3: only `catalog/recipes/worktree-flow/bin/SHA256SUMS`
  modified by this unit (8+/4−: one regeneration comment block + four digest
  lines) on top of the pre-existing WU1/WU2 files; `dist/` is gitignored
  (`.gitignore:42`) so binaries stay uncommitted.

## WU3 remaining

- Native review + delivery slicing remain PENDING and parent-owned: the
  combined WU1+WU2+WU3 diff exceeds the two forecast slices, so review
  readiness is NOT claimed.

## WU2 Handoff Notes

- Envelope contract: see mergeconfig.go doc comments (stdin request, stdout
  result). Semantic errors exit 0 with `error` set; process errors exit 2 with
  a stderr diagnostic and no envelope.
- Warning strings are the inner warn text; the bridge must apply the Python
  `warn()` formatting (`  ! <msg>` on stderr).
- Config is emitted as an insertion-ordered JSON object; Python `json.loads`
  restores the exact merged_cfg order.
- JSON transport gaps (bridge-side, WU2 must handle, not claimed solved in Go):
  - TOML date/datetime/time config values: `tomllib` yields datetime objects
    that `json.dumps` rejects (TypeError) — RESOLVED in WU2: the bridge does
    NOT stringify; it falls back once so Python keeps the typed value and its
    own validation (an earlier str()-coercion suggestion was a parity defect,
    retracted).
  - Nonfinite floats (TOML `inf`/`nan`): `json.dumps` emits `Infinity`/`NaN`,
    which Go's decoder rejects (would become exit 2) — the bridge must
    sanitize or reject these at envelope build time.
  - Python `bool` config values for an enum field arrive as JSON booleans and
    are `str()`-rendered (`True`/`False`) inside Go — verified by tests.

## JD Fix Pass (bounded ordinary fix pass, post-freeze)

Four WARNING findings fixed from the frozen dual-judge ledger
(`odd/tasks/go-merge-config-jd-ledger.json`, which now carries a `fixes`
section per ID; frozen finding rows untouched). One atomic work unit per
finding, TDD with observed RED before each fix and GREEN after. No
commits/push/PR; JD-A-002 (SUGGESTION, INFO-only) untouched.

- JD-A-001 (subprocess text=True locale decode crash):
  - RED: `FailOpenFallbackTests.test_unicode_decode_error_falls_back_with_one_warning`
    (mocked UnicodeDecodeError) and
    `.test_undecodable_go_output_falls_back_with_one_warning` (real invalid-UTF-8
    stub) both errored with `UnicodeDecodeError` escaping `go_merge_config`.
  - GREEN: `go_merge_config` now passes `encoding="utf-8", errors="replace"`
    (undecodable bytes become U+FFFD and degrade through the JSON-parse
    fallback) and the transport except clause also catches
    `UnicodeDecodeError`. Other bridges' `text=True` pattern untouched.
- JD-B-001 (bridge ignored the active CLI home):
  - RED: `MergeConfigHomeResolutionTests.test_binary_at_active_home_is_found_when_home_is_passed`
    failed with `TypeError: merge_config() got an unexpected keyword argument
    'home'`.
  - GREEN: keyword-only `home` param on `merge_config`/`go_merge_config`
    (default None preserves package-root resolution), resolved via
    `_orphans_bridge_home(home)`; the production `materialize_recipes` call
    site passes `cli_home`, mirroring the sibling bridges. The companion test
    also pins that without `home` the active-home binary is NOT consulted.
  - Note: the with-home RED/GREEN composes with the JD-B-002 digest re-check
    (a bare temp home has no committed SHA256SUMS, so sidecar acceptance holds).
- JD-B-002 (stale `.verified` sidecar trusted without digest re-check):
  - RED: `ResolveVerifiedBinaryDigestRecheckTests.test_stale_cache_binary_with_sidecar_is_not_resolved`
    failed — stale bytes with a valid sidecar resolved today.
  - GREEN: `gate_binary.resolve_verified_binary` re-checks the cache
    candidate's sha256 against `load_expected_digests` when the asset has a
    committed digest; mismatch → None (acquire may refresh). WORKTREE_GATE_BIN
    override and digest-less homes keep prior behavior.
    `tests.test_binding_bridge` re-run OK (shared resolution authority).
- JD-AB-001 (pyRepr diverged from CPython repr()):
  - RED: six new `TestMergeConfigEnumUsesPythonStr` cases failed (apostrophe
    wrap, \x01/\x7f/\x85, \u2028, dict-with-apostrophe).
  - GREEN: `pyRepr` now wraps in double quotes when the string holds an
    apostrophe and no double quote (single-quote default kept), names
    `\n \r \t`, escapes backslash and the active quote, and renders other
    non-printables as `\xNN` (≤0xFF) / `\uXXXX` (≤0xFFFF) / `\UXXXXXXXX` via
    `unicode.IsPrint`. Bridge parity seam case added in
    `tests/test_merge_config_bridge.py` (Go error == Python error).
  - Build: `scripts/build-gate.sh` rebuilt all four targets with the canonical
    go1.24.13 toolchain; `catalog/recipes/worktree-flow/bin/SHA256SUMS`
    regenerated preserving format/order (new regeneration comment appended);
    independent checksum verification all OK.

### JD fix pass verification (observed)

- `cd catalog/recipes/worktree-flow/gate && go test -count=1 ./...` → ok
  (main package + ledger), exit 0.
- `gofmt -l catalog/recipes/worktree-flow/gate` → no output.
- `go version` → `go version go1.24.13 darwin/arm64` (canonical; build emitted
  no non-canonical-toolchain warning).
- `(cd dist && shasum -a 256 -c ../catalog/recipes/worktree-flow/bin/SHA256SUMS)`
  → all four OK, exit 0.
- Focused unittest run of the six named modules → `Ran 234 tests in 40.780s —
  OK`, exit 0 (226 baseline + 8 new fix-pass tests).
- Full suite: `perl -e '$SIG{ALRM}=sub{exit 142}; alarm 900; exec @ARGV'
  ./tests/validate.sh` → exit 0, `Ran 2310 tests in 712.883s — OK (skipped=2)`;
  the two skips are the same pre-existing environmental ones recorded in WU3.
- `git diff --check` → exit 0, no output.

### Judgment Day round 2 (bounded WARNING fix, JD2-AB-001)

Scope: exactly one confirmed WARNING from the round-2 dual-judge ledger
(JD2-AB-001, confirmed by both judges, fix-caused in round 1). JD2-A-002
(SUGGESTION) recorded as INFO-only, untouched. No commits/push/PR.

- JD2-AB-001 (unreadable/non-UTF-8 SHA256SUMS crashed the "never raises"
  resolver, taking the doctor tracker-ledger check down with it):
  - RED: `ResolveVerifiedBinaryDigestRecheckTests.test_unreadable_sums_degrades_to_not_verified`
    (SHA256SUMS chmod 0o000) errored with `PermissionError: [Errno 13]`
    escaping `resolve_verified_binary`; `.test_non_utf8_sums_degrades_to_not_verified`
    errored with `UnicodeDecodeError` escaping the same call.
  - GREEN: the digest re-check in `gate_binary.resolve_verified_binary` now
    wraps `load_expected_digests(home)` in `except (OSError, UnicodeDecodeError)`
    and fails closed (returns None, sidecar not accepted); docstring updated to
    state that an unreadable/undecodable trust root fails closed.
    `load_expected_digests` contract for other callers unchanged; doctor.py
    untouched — it degrades transitively to its designed ERROR check once the
    resolver never raises. Readable-digest and digest-less-home behavior
    pinned by the existing matching-digest/no-digest tests (still passing).

### JD round-2 verification (observed)

- `cd catalog/recipes/worktree-flow/gate && go test -count=1 ./...` → ok, exit 0
  (no Go changes this round; confirmed unaffected).
- `python3 -m unittest tests.test_worktree_gate_dist_config
  tests.test_merge_config_bridge tests.test_recipe_materialize` → OK, exit 0.
- Full suite: `perl -e '$SIG{ALRM}=sub{exit 142}; alarm 900; exec @ARGV'
  ./tests/validate.sh` → exit 0 (observed result below).
- `git diff --check` → exit 0, no output.
