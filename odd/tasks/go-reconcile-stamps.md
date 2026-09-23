# ODD Feature: go-reconcile-stamps

## Objective

Migrate the decision of `stamp_recipe_reconcile_defaults` (which per enabled recipe computes the reconcile stamp dict from the recipe's declared `config_schema`) from Python to the Go worktree-gate binary, preserving behavior with a fail-open Python bridge. The manifest write (`writer.update_recipe_config`) stays Python.

## Problem and decision

Slice 2 of the rank-3 Python-to-Go strangler closure (materialization/sync domain), following GO-04 (PRs #286/#288). The stamp decision — reading `[config.reconcile]` declared values, collecting used field names, and selecting declared defaults — is recipe-authoritative decision logic that belongs in Go per AGENTS.md. The TOML write stays Python (actuator chain is slice 3).

## Python authority (scouted, read-only at development b215412)

- `lib/_internal/recipe-materialize.py:291` `stamp_recipe_reconcile_defaults(project_root, catalog_dir, enabled_ids)`; call site `:2494`.
- Per enabled recipe (ordered): `read_recipe(catalog_dir, rid)`; any exception → silent continue (validation surfaces via sync path).
- `tables.get("reconcile")` from `recipe.config_schema` (`recipe_schema.py` `_parse_config`: only keys in `STRUCTURED_CONFIG_SHAPES` become tables; `values` is the raw declared `[config.reconcile]` table, shape-validated).
- Stamp dict: `{"reconcile": values}` plus each used field's declared default (`field.default is not None`), where used = `{values.get("scope_field") or ""}` ∪ `{expectation.config_field, expectation.config_field_when_set}` for dict expectations, empty strings discarded.
- Apply: `writer.update_recipe_config(manifest, rid, stamp)` — adds missing values and replaces existing values when they differ; equal values are left untouched (writer behavior unchanged by this change). Writer exception → one warn line `recipe '<name>': reconcile defaults not stamped (<ExcType>: <exc>)`.

## Scope

- New Go decision core: `--plan-reconcile-stamps` flag (catalog-dir + repeatable `-recipe` id, JSON envelope on stdout) computing per-recipe stamps.
- Go reads only what it consumes: raw `[config.reconcile]` table + `[config.<field>]` `default` values, mirroring the slice-1 `recipePrimitivesReader` minimal-reader pattern.
- Python bridge in `stamp_recipe_reconcile_defaults`: Go-primary with fail-open Python fallback (one `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK` warning); Python write path and per-recipe writer warnings unchanged.
- Reuse ONE `_load_gate_binary` variant (the one in recipe-materialize.py; do not add a third home-resolution variant).
- Go unit tests + Python bridge/parity tests; `bin/SHA256SUMS` regenerated with canonical go1.24.13.
- Parity traps to pin in tests: `default = false` / `default = 0` stamp (Python `is not None`, not truthiness); unknown field names skipped; non-dict expectations ignored; unreadable recipe silently skipped (Go omits from envelope, bridge falls back or continues without warning if Python core agrees).

## Non-goals

- `update_recipe_config` TOML write migration (actuator chain, slice 3).
- `--apply-orphans`, copy/template/hook actuators, `lock.py` serialization.
- The 8 accumulated informational advisory findings from prior slices (optional follow-up PR).

## Constraints

- Base: `development` at `b215412`.
- Worktree: `.worktrees/go-reconcile-stamps`, branch `feat/go-reconcile-stamps`.
- Delivery: one PR for this whole change, targeting `development` via `gh`; push, PR creation, and merge remain explicit human decisions.
- Review boundary (maintainer-directed): one complete WU1+WU2 candidate against `origin/development`, reviewed after WU2 verification. WU1's earlier candidate approval remains historical evidence but is NOT approval for this combined candidate. This supersedes the earlier per-WU review-candidate plan.
- Review workload (recorded decision): this is one requested full-candidate attempt at exactly 1,285 changed lines against `origin/development` (898 tracked diff lines [885 insertions + 13 deletions] + 387 untracked test-file lines), despite exceeding the usual 400 additions+deletions RDD cap and the roughly 1,000-line relayability guidance. Known history: a 1,146-line reliability failure and a 1,006-line pass. No claim is made that any model or provider change makes this size safe. The run may stop for relay/capture limits; no approval is recorded in advance.
- RDD mode confirmed on. This change is a planned migration tracked by GO-05, not a reported defect, so no clean-base defect reproduction is recorded.
- TDD enabled by `ai-specs/ai-specs.toml` `[recipes.tdd-flow]`; configured runner is `./tests/validate.sh`. WU2 focused checks: `python3 -m unittest tests.test_reconcile_stamps_bridge` and `./tests/run.sh` where useful.
- Generated artifacts in English.

## Tracker

- **card_id**: `6ab2e7045ed78dbfc506cf92`
- **shortLink**: `7ZEPdt35`
- **url**: https://trello.com/c/7ZEPdt35/147-go-05-migrate-stamprecipereconciledefaults-decision-to-go
- **list**: In Progress

## Tasks

### WU1 — Go reconcile-stamp decision core
- **Route**: delegated writer (`gentle-ai-worker`); 4-file exploration trigger + multi-file write trigger.
- **Allowed edit surfaces**:
  - `catalog/recipes/worktree-flow/gate/reconcilestamps.go` (new)
  - `catalog/recipes/worktree-flow/gate/reconcilestamps_test.go` (new)
  - `catalog/recipes/worktree-flow/gate/recipe_toml.go` (minimal reader addition, following `recipePrimitivesReader` pattern)
  - `catalog/recipes/worktree-flow/gate/main.go` (flag wiring)
  - `bin/SHA256SUMS` + gate binaries (build + digest regen, go1.24.13)
  - `odd/tasks/go-reconcile-stamps.md` (this file, evidence)
- **Acceptance**:
  - `--plan-reconcile-stamps` emits deterministic JSON: ordered `{"stamps":[{"id":...,"stamp":{...}}]}`, omitting unreadable recipes (Python silent-continue semantics).
  - Stamp content matches Python: `reconcile` = raw declared values; used fields (scope_field + expectations' config_field/config_field_when_set, empties discarded) get their declared default only when present (TOML `false`/`0` are present).
  - No write side effects; exit 0 on success, nonzero only for infrastructure/parse failure of the invocation itself.
  - `go test ./...` green in the gate module; digests verified.
- **Checks**: RED focused Go test first; GREEN `go test ./...`; build all four gate targets + `SHA256SUMS` regen + digest verification; parent spot check.
- **Status**: implemented, parent-verified, committed, native-reviewed/approved and acknowledged

#### WU1 parent verification (spot check)
- Independent `go test -count=1 ./` in the gate module: ok (39.95s); `gofmt -l` and `go vet` clean.
- Digest verification: `shasum -a 256` over all four `dist/worktree-gate-*` targets, sorted-compared against `bin/SHA256SUMS` → all four match; toolchain `go1.24.13 darwin/arm64`.
- Smoke test on a synthetic fixture: `--plan-reconcile-stamps` stamped `base_branch: false` and `protect_base: 0` (present-but-falsy defaults stamp), preserved the raw `reconcile` table with expectations, omitted a recipe without `[config.reconcile]`, exit 0.
- Code review of the decision core and acquisition seam against `lib/_internal/recipe-materialize.py:291` and `recipe_schema._parse_config`: used-set derivation, present-default rule (TOML has no null so present == not-None), fields/extra split (boolean `required` key), and silent-skip semantics all mirror the Python authority.
- Parity ruling: assumptions 1–2 accepted as the documented minimal-reader boundary (same class as slice 1). Verified integration ordering: stamping (materialize_recipes ~:2494) runs BEFORE recipe materialization hard-fails (~:2555), so a shape-invalid recipe gets its declared defaults stamped and then the sync fails at materialization — transient manifest pollution of recipe-declared values that converges once the recipe is fixed; benign and bounded to already-broken catalogs. WU2 parity tests should pin the happy-path equality and document this degraded-path difference. Assumption 3 (datetime → string) accepted: TOML datetimes in config defaults are not a realistic recipe-author pattern; 4–6 accepted as-is.

#### WU1 native RDD review
- Lineage `review-a21295f8375f4cdb`: approved; exact acknowledgement completed and authority burned.
- Candidate: 6 paths, 705 changed lines, high risk. START replayed the existing lineage; bound STATUS offered only `review-resilience`, the sole reviewer run in this session. The remaining lenses had already completed.
- Advisory findings were explicitly non-blocking: `R1-recipe-id-path-join` (`recipe_toml.go:338-343`), `R3-001` (`recipe_toml.go:259`), and `R3-002` (`reconcilestamps_test.go:242`).

#### WU1 evidence (writer, worktree `.worktrees/go-reconcile-stamps`)

Files changed:
- `catalog/recipes/worktree-flow/gate/reconcilestamps.go` (new, 114 lines): decision core `buildReconcileStamp` + `--plan-reconcile-stamps` command (`runPlanReconcileStamps`), mirroring the `runResolveTagConflicts` envelope/exit contract (exit 0 for any planned result, 2 only for unusable flags or parser failure).
- `catalog/recipes/worktree-flow/gate/reconcilestamps_test.go` (new, 346 lines): decision table tests, exact-JSON shape pin, acquisition test, end-to-end command tests.
- `catalog/recipes/worktree-flow/gate/recipe_toml.go` (+94 lines): `reconcileStampReader` Python acquisition seam + `loadReconcileStampSources` (dedupe, readable pre-check, enabled-order results, `UseNumber` decode so TOML integers survive the round trip). Reuses `runRecipeTomlReader` bounded execution unchanged.
- `catalog/recipes/worktree-flow/gate/main.go` (+9 lines): `--plan-reconcile-stamps` flag wired to the shared `--catalog-dir` / repeatable `--recipe` flags, mirroring `--resolve-primitive-conflicts`.
- `catalog/recipes/worktree-flow/bin/SHA256SUMS`: regenerated (4 digests) with the canonical go1.24.13 toolchain (`go version` → go1.24.13 darwin/arm64; no toolchain warning from build-gate.sh).

TDD evidence:
- RED: `go test ./ -run TestReconcileStamps` → build failed with `undefined: reconcileStampSource / buildReconcileStamp / reconcileStampPlan / reconcileStampEntry / loadReconcileStampSources` (test written first; the parent-suggested `-run TestReconcileStamps` selector matches none of the final test names — focused rerun used `-run ReconcileStamp`).
- GREEN: `go test ./ -run ReconcileStamp` → all 11 subtests/4 tests pass; full `go test ./` (whole gate package) → ok 39.7s; `gofmt -l .` → clean; `go vet ./` → clean.
- TRIANGULATE: negative cases in the matrix — absent default/unknown field not stamped, non-table expectations ignored, empty-string field names discarded, empty used set, recipe without `[config.reconcile]` skipped, broken TOML + missing recipe omitted while others planned, ordering follows `--recipe` order, exact stdout JSON pin. One test-fixture fix during GREEN (second expectation's raw `event` key was missing from the expected string — implementation was right, the pin was wrong).

Build + digest verification:
- `./scripts/build-gate.sh` → 4 targets built into `dist/` (darwin/arm64, darwin/amd64, linux/amd64, linux/arm64), no canonical-toolchain warning.
- `shasum -a 256 dist/worktree-gate-*` → 4 digests recorded in `bin/SHA256SUMS`; `./scripts/verify-gate-sums.sh <generated> catalog/recipes/worktree-flow/bin/SHA256SUMS` → ok, 4 digest entries match the committed trust root.
- Smoke test (`dist/worktree-gate-current --plan-reconcile-stamps`): fixture catalog → rec-a stamped (`base_branch: false` stamped, defaults preserved), rec-b (no reconcile) / rec-c (broken TOML) / ghost omitted, `exit=0`.

Parity assumptions (for parent review before WU2):
1. **Field recognition**: the reader treats a `[config.<name>]` section as a config field only when it is a table with a boolean `required` key (mirrors `_parse_config`'s fields/extra split, which decides "unknown field name"). Divergence: Python *raises* (skipping the whole recipe) when `required` is present but not boolean; Go's minimal reader instead just treats that name as unknown and still stamps the rest of the recipe.
2. **No schema validation of `[config.reconcile]`**: per the slice-1 minimal-reader boundary, shape-invalid reconcile values (non-string scope_field, non-integer max_age_seconds, expectations missing string keys, >32 expectations) do NOT skip the recipe in Go; the decision layer simply ignores the malformed pieces (type assertions fail → no contribution). Python would raise in `validate_structured_config` and skip the recipe entirely. The parent-pinned "non-table expectation entries ignored" semantics follow the stamp loop, which under full Python schema validation is unreachable.
3. **Datetime values**: TOML datetimes in reconcile values or defaults are JSON-unsafe; the reader emits them as strings via `json.dumps(default=str)` instead of failing the whole acquisition run. (Python authority would pass the datetime through to the TOML writer unchanged.)
4. **Duplicate `--recipe` ids**: deduplicated like the sibling readers; Python would stamp the same recipe twice (idempotent: equal values are writer no-ops, same end state).
5. **Stamp key order**: Go marshals map keys sorted, so the emitted stamp dict is alphabetical, not "reconcile first" like Python's insertion order; content is identical and the JSON bridge in WU2 parses dicts (order-insensitive), but the raw stdout bytes differ from a hypothetical Python emission.
6. **Exit contract**: unreadable/invalid recipes exit 0 and are omitted (Python silent-continue), unlike `--resolve-primitive-conflicts` which exits 2 on the same shapes — this mirrors the stamp authority's per-recipe `continue`, per the task contract.

### WU2 — Python fail-open bridge + parity tests

#### WU2 read-only mapping
- Sole call site: `recipe-materialize.py:2494`, after `binding_resolution(..., acquire_if_missing=True)`; retain the Python TOML write and its per-recipe warning verbatim.
- Reuse `_load_gate_binary()` at `recipe-materialize.py:132` and `gate_binary.resolve_verified_binary(ai_specs_home)`; do not add another loader or thread a binary path through `binding_resolution`. Match the self-contained `go_resolved_config` bridge pattern.
- Use the sibling bridge timeout of 60 seconds. Existing Python stamp behavior has no tests; add `tests/test_reconcile_stamps_bridge.py` using the `test_resolved_config_bridge.py` fixture style and existing `dist/worktree-gate-current` binary.
- No Go, binary, or SHA256SUMS changes are needed. Runtime harness: N/A — the Python unittest exercises the local verified gate binary with temporary recipe catalogs; there is no separate runtime environment.
- WU2 forecast (superseded — the exact combined-candidate size is recorded in the Constraints review-workload decision and the post-fix size record below): the combined WU1+WU2 candidate against `origin/development` is exactly 1,285 changed lines (898 tracked diff lines [885 insertions + 13 deletions] + 387 untracked test-file lines). Per the maintainer-directed review boundary above, one full-candidate attempt is requested.
- **Allowed edit surfaces**:
  - `lib/_internal/recipe-materialize.py` (bridge in `stamp_recipe_reconcile_defaults`)
  - `tests/test_reconcile_stamps_bridge.py` (new)
  - `odd/tasks/go-reconcile-stamps.md` (evidence)
- **Acceptance**:
  - Go-primary: verified gate binary computes stamps; Python applies each via `update_recipe_config` (existing writer behavior: adds missing values, replaces existing values when they differ; the writer itself is unchanged) preserving per-recipe writer warnings verbatim.
  - Any Go failure (missing binary, digest mismatch, exit != 0, malformed envelope) → one `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK` warning, then full Python authority re-runs.
  - Parity: Go and Python stamp dicts agree on the fixture catalog (ordered recipes, false/zero defaults, missing fields, unreadable recipe skip).
  - Existing materialization tests stay green.
- **Checks**: RED `python3 -m unittest tests.test_reconcile_stamps_bridge`; GREEN the same focused suite and `./tests/validate.sh`. Exact full runner is configured in `ai-specs/ai-specs.toml` as `./tests/validate.sh`. The mapper reports `./tests/run.sh` as unit-only (it also invokes Go tests). No known environmental failure is waived; classify a failure as pre-existing only with exact clean-base evidence.
- **Status**: implemented; focused RED/GREEN + full validation observed; pending parent verification and the maintainer-directed combined WU1+WU2 native review (then one PR with WU1).

#### WU2 evidence (writer, worktree `.worktrees/go-reconcile-stamps`)

Files changed:
- `lib/_internal/recipe-materialize.py` (+143/−9): bridge block before `stamp_recipe_reconcile_defaults` — `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK`, `GO_RECONCILE_STAMPS_BRIDGE_TIMEOUT_SECONDS = 60`, `_warn_reconcile_stamps_bridge_fallback`, `_is_reconcile_stamps_envelope`, `go_reconcile_stamps(catalog_dir, recipe_ids)` (reuses `_load_gate_binary()` + `gate_binary.resolve_verified_binary(_bridge_home(catalog_dir))`, 60s timeout, one warning per failure mode, envelope `stamps[].{id,stamp}` validated), `_reconcile_stamp_recipe_name` (best-effort declared name for the writer warning), `_stamp_recipe_reconcile_defaults_python` (the previous loop body verbatim as the temporary fallback), and `stamp_recipe_reconcile_defaults` now Go-primary: applies each envelope stamp via the same `writer.update_recipe_config` call and returns to the Python authority on any bridge failure. The write path and per-recipe warning format are unchanged; `update_recipe_config`'s existing behavior (add missing values, replace existing values when they differ) is untouched.
- `tests/test_reconcile_stamps_bridge.py` (new, 370 lines): parity (Go-applied manifest == Python-applied manifest, pinning `default = false`/`default = 0` stamped, absent default and unreferenced field not stamped, unrelated pre-existing manifest keys preserved, no-reconcile/broken/ghost silently skipped), envelope contract (enabled order, entry shape, 60s timeout pin), fully-skipped catalog writes nothing and warns nothing, fallback table (missing binary / exit 2 / non-JSON / wrong envelope → exactly one `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK` warning + full Python authority still stamps), and the verbatim writer-failure warning (`recipe 'Stamp Full Fixture': reconcile defaults not stamped (OSError: disk full)`) on both paths with the surviving recipe still stamped.

TDD evidence:
- RED: `python3 -m unittest tests.test_reconcile_stamps_bridge` (tests written first, production untouched) → 9 errors, all `AttributeError: ... does not have the attribute 'go_reconcile_stamps'` / missing `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK` / `_stamp_recipe_reconcile_defaults_python` (one fixture-dir bug fixed before the meaningful RED was recorded).
- GREEN: same command after implementation → `Ran 5 tests ... OK`.
- TRIANGULATE: fallback stub table (exit 2, non-JSON, wrong envelope, missing binary) and the dual-path writer-failure warning pin the negative contract; the fully-skipped catalog pins that a legitimate empty plan is not a fallback.

Verification:
- `python3 -m unittest tests.test_reconcile_stamps_bridge` → `Ran 5 tests ... OK` (GREEN rerun).
- `./tests/validate.sh` → exit 0; full suite `Ran 2330 tests ... OK (skipped=2)`. The `cli-version ... pinned 99.99.99` ERROR line inside the log is a recipe smoke fixture exercising a failure path, not a suite failure (validate exits 0).

Authored diff size (historical pre-fix WU2 record, superseded by the exact combined-candidate count of 1,285 changed lines): 572 additions+deletions at authoring time. Combined candidate (exact, vs `origin/development`): 1,285 changed lines — above the ~1000-line relayability guideline; per the recorded maintainer decision this is one requested full-candidate attempt against `origin/development`, with no approval claimed in advance.

Finding (pre-existing, out of WU2 scope): `update_recipe_config` overwrites existing config keys whose value differs (`pending_plain` keeps keys where `current[key] != value`), so a recipe-declared default DOES replace a differing explicit project value; the retained docstring's "this helper never overwrites an existing key" does not match the writer's actual behavior. WU2 preserves the Python authority verbatim as mandated; both authorities feed identical stamps into the same writer, so bridge parity holds. Flagged for a future docstring/writer-behavior decision.

#### WU2 verifier finding and fix (combined-candidate pre-freeze)

- Verifier finding (confirmed fail-open edge): `go_reconcile_stamps` used `subprocess.run(..., text=True)`, whose strict stdout decode raises `UnicodeDecodeError` on invalid gate output bytes; that exception is neither `OSError` nor `subprocess.SubprocessError`, so malformed output escaped the bridge uncaught instead of emitting one `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK` warning and running the full Python authority.
- Fix (minimal, same two edit surfaces): `tests/test_reconcile_stamps_bridge.py` gained `ReconcileStampsFallbackTests.test_invalid_utf8_output_falls_back_with_one_warning` — a `/bin/sh` stub emitting invalid UTF-8 bytes (`\377\376`) on stdout; it asserts exactly one fallback warning and that the Python authority still stamps the fixture. `lib/_internal/recipe-materialize.py` extends the run-failure handler to `except (OSError, subprocess.SubprocessError, UnicodeError)`. Also corrected the fallback-suite comment that wrongly claimed an empty `WORKTREE_GATE_BIN` override short-circuits cache lookup: the override is falsy, so resolution falls through to the temporary catalog home's cache path, which has no binary in the test. Sibling bridges were deliberately not touched (narrow scope; `go_recipe_conflicts` already decodes with `errors="replace"`); the WU1 parity-assumption 2 Go/Python invalid-shape divergence stays documented as-is, and the WU2 evidence above remains the historical pre-fix record (370-line test file → 387 after this fix).
- RED: `python3 -m unittest tests.test_reconcile_stamps_bridge` with production untouched → `Ran 6 tests ... FAILED (errors=1)`; the new test errored with `UnicodeDecodeError: 'utf-8' codec can't decode byte 0xff in position 0: invalid start byte` propagating out of `subprocess.run` through `go_reconcile_stamps`.
- GREEN: same command after the handler extension → `Ran 6 tests ... OK` (all 6, including the pre-existing 5).
- Post-fix full validation: `./tests/validate.sh` → exit 0; `Ran 2331 tests ... OK (skipped=2)` (2330 prior + 1 new test). `git diff --check` → clean.
- Post-fix combined candidate size (exact count, recomputed by the parent against the current worktree): 898 tracked added+deleted lines (885 insertions + 13 deletions; `git diff --numstat`) + 387 untracked test-file lines (`tests/test_reconcile_stamps_bridge.py`) = 1,285 changed lines vs `origin/development`; candidate scope unchanged.

## Verification evidence

Recorded per task above: WU1 Go tests/build/digests (see WU1 evidence), WU2 focused bridge suite and full `./tests/validate.sh` (see WU2 evidence), and the WU2 verifier finding/fix with its RED/GREEN + post-fix full validation (see WU2 verifier finding and fix). No required verification command is failing. Parent verification of the combined WU1+WU2 candidate and a fresh `gentle_review assess` over the committed range are pending.

## Blockers

- The first `gentle_review assess` failed closed on the then-untracked `tests/test_reconcile_stamps_bridge.py`; no lineage was started. Parent verification and a fresh assess over the committed range against `origin/development` remain pending.
- (`./tests/validate.sh` subprocess-spawn timeout history documented in prior slices — classify pre-existing failures on clean HEAD before claiming regression.)

## Next step

Parent verification of the combined WU1+WU2 candidate, then a fresh native review assess over the committed range against `origin/development` (one full-candidate attempt; it may stop for relay/capture limits). Publishing the single PR remains an explicit human decision.
