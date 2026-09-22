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
- Apply: `writer.update_recipe_config(manifest, rid, stamp)` — absent keys only, never overwrites. Writer exception → one warn line `recipe '<name>': reconcile defaults not stamped (<ExcType>: <exc>)`.

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
- Delivery: PR-based via `gh` to `development`; never direct push.
- Sequential review+delivery per WU (user preference workflow/review-delivery-sequential-slices): WU1 reviewed → PR → merge → WU2 on fresh base.
- Native review candidates ≈1000 changed lines max; WU1 and WU2 reviewed separately.
- TDD enabled; runner `./tests/validate.sh` (focused suites for per-task evidence).
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
- **Status**: implemented, parent-verified, committed

#### WU1 parent verification (spot check)
- Independent `go test -count=1 ./` in the gate module: ok (39.95s); `gofmt -l` and `go vet` clean.
- Digest verification: `shasum -a 256` over all four `dist/worktree-gate-*` targets, sorted-compared against `bin/SHA256SUMS` → all four match; toolchain `go1.24.13 darwin/arm64`.
- Smoke test on a synthetic fixture: `--plan-reconcile-stamps` stamped `base_branch: false` and `protect_base: 0` (present-but-falsy defaults stamp), preserved the raw `reconcile` table with expectations, omitted a recipe without `[config.reconcile]`, exit 0.
- Code review of the decision core and acquisition seam against `lib/_internal/recipe-materialize.py:291` and `recipe_schema._parse_config`: used-set derivation, present-default rule (TOML has no null so present == not-None), fields/extra split (boolean `required` key), and silent-skip semantics all mirror the Python authority.
- Parity ruling: assumptions 1–2 accepted as the documented minimal-reader boundary (same class as slice 1). Verified integration ordering: stamping (materialize_recipes ~:2494) runs BEFORE recipe materialization hard-fails (~:2555), so a shape-invalid recipe gets its declared defaults stamped and then the sync fails at materialization — transient manifest pollution of recipe-declared values that converges once the recipe is fixed; benign and bounded to already-broken catalogs. WU2 parity tests should pin the happy-path equality and document this degraded-path difference. Assumption 3 (datetime → string) accepted: TOML datetimes in config defaults are not a realistic recipe-author pattern; 4–6 accepted as-is.

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
4. **Duplicate `--recipe` ids**: deduplicated like the sibling readers; Python would stamp the same recipe twice (idempotent absent-keys-only, same end state).
5. **Stamp key order**: Go marshals map keys sorted, so the emitted stamp dict is alphabetical, not "reconcile first" like Python's insertion order; content is identical and the JSON bridge in WU2 parses dicts (order-insensitive), but the raw stdout bytes differ from a hypothetical Python emission.
6. **Exit contract**: unreadable/invalid recipes exit 0 and are omitted (Python silent-continue), unlike `--resolve-primitive-conflicts` which exits 2 on the same shapes — this mirrors the stamp authority's per-recipe `continue`, per the task contract.

### WU2 — Python fail-open bridge + parity tests
- **Route**: delegated writer; multi-file bridge/test change. Base on fresh development after WU1 merges.
- **Allowed edit surfaces**:
  - `lib/_internal/recipe-materialize.py` (bridge in `stamp_recipe_reconcile_defaults`)
  - `tests/test_reconcile_stamps_bridge.py` (new)
  - `odd/tasks/go-reconcile-stamps.md` (evidence)
- **Acceptance**:
  - Go-primary: verified gate binary computes stamps; Python applies each via `update_recipe_config` (absent-keys-only) preserving per-recipe writer warnings verbatim.
  - Any Go failure (missing binary, digest mismatch, exit != 0, malformed envelope) → one `GO_RECONCILE_STAMPS_BRIDGE_FALLBACK` warning, then full Python authority re-runs.
  - Parity: Go and Python stamp dicts agree on the fixture catalog (ordered recipes, false/zero defaults, missing fields, unreadable recipe skip).
  - Existing materialization tests stay green.
- **Checks**: RED bridge/parity test first; GREEN `./tests/validate.sh`; parent spot check.
- **Status**: pending

## Verification evidence

(to be filled per task)

## Blockers

(none known; `./tests/validate.sh` subprocess-spawn timeout history documented in prior slices — classify pre-existing failures on clean HEAD before claiming regression)

## Next step

Delegate WU1 to the writer with the scouting context above.
