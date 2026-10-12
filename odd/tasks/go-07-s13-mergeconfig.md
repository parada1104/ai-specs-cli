# Go 07 S13 PR2a — Merge-config core extraction (shared package)

## Objective
Move the gate's `--plan-merge-config` decision core into the importable shared package and leave the
main package as a thin wrapper. **Delivery A** of S13 PR2, mirroring the S12.3 A/B and SX0 pattern:
the root-side native authority that calls this core is **delivery B (PR2b)**, deferred.

Nothing about the gate's JSON command contract changes: it is a behavior-preserving relocation whose
oracle is the existing gate suite.

## Re-slices recorded
The human re-split S13 into four PRs after the measured sizes contradicted the original three-way
cut (PR1 bindings + witness merged as `7384c65`; PR3 resolved-config; PR4 hooks + recipe-mcp).

PR2 was then split **again** into PR2a (this one) and PR2b, for two independently blocking reasons:

1. **Budget.** The gate-side extraction alone measures 1260 additions plus deletions against the 1500
   cap, leaving 240 for a root authority measured at ~450-550.
2. **A missing export.** The root adapter cannot serialize `internal/schema.ConfigTable.Shape` into
   the shared request: `Shape` is backed by an unexported `*shapeTable`
   (`internal/schema/recipe.go:58`, `:75`, `:157`) and `encoding/json.Marshal(*shapeTable)` yields
   `{}`. PR2b therefore needs an exported, order-preserving accessor (for example
   `StructuredShapeJSON(section string) (json.RawMessage, error)`) plus its focused test. The human
   authorized extending the surface to `internal/schema/recipe.go` for **PR2b only**.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no card could be
  created or linked for this slice.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed
  without recording a decision. `tracker.none` is presentation and evidence only and is never written
  by the host, so none was created here.
- Close-out follow-up: link the S13 card in this section once the tracker is reachable.

## Scope (PR2a)
- Base: `7384c65` on `epic/go-single-binary`. Worktree: `.worktrees/go-07-s13-mergeconfig`.
  Branch: `change/go-07-s13-mergeconfig`.
- Move the pure decision core out of `catalog/recipes/worktree-flow/gate/mergeconfig.go` into the
  shared package: `shared/orderedmap.go` (the ordered container plus its decode helpers) and
  `shared/mergeconfig.go` (the request/result types, the CPython `py*` parity helpers, the structured
  validation and `PlanMergeConfig`).
- Leave `gate/mergeconfig.go` as a thin wrapper: the historical lowercase aliases, the `decodeOrdered`
  delegation and the unchanged `runPlanMergeConfig` flag wrapper.
- `orderedMap` is not exclusive to merge-config. The plan already anticipated this
  (`odd/tasks/go-07-sx0.md:177`, U8: "`orderedMap` also used by out-of-scope recipeconfigwrite.go —
  moves to shared, recipeconfigwrite.go gets a required one-line reference change"), so
  `gate/recipeconfigwrite.go` and `gate/recipeconfigwrite_test.go` get **mechanical only** reference
  updates (`.set`→`.Set`, `.get`→`.Get`, `.keys`→`.Keys()`, `.values`→`.Values()`,
  `newOrderedMap()`→`shared.NewOrderedMap()`, type positions→`*shared.OrderedMap`). That is a
  deliberate exception to "gate tests byte-unchanged": every other gate test file, `mergeconfig_test.go`
  in particular, stays byte-unchanged.
- Regenerate only the four canonical gate checksum entries that the extraction requires.

## In-scope units
### Gate core (`catalog/recipes/worktree-flow/gate/mergeconfig.go`, 571 lines pre-image;
verifier-confirmed — an earlier estimate in this document said 575)
| Symbol group | Lines | Class | Destination |
| --- | --- | --- | --- |
| `mergeConfigField/Table/Pair/Request/Result` types | 33-73 | pure decision | `shared/mergeconfig.go` |
| `orderedMap` + new/set/get/Marshal/Unmarshal | 75-133 | pure container | `shared/orderedmap.go` |
| `decodeOrdered/Value/FromToken` | 135-200 | pure helper | `shared/orderedmap.go` |
| `pyType/pyTruthy/pyString/pyRepr/pyReprString` | 202-332 | CPython parity | `shared/mergeconfig.go` |
| `structuredListMax` + `validateStructured/checkShape/checkShapeScalar` | 334-414 | pure decision | `shared/mergeconfig.go` |
| `planMergeConfig` | 416-538 | pure decision core | `shared/mergeconfig.go` |
| `runPlanMergeConfig` | 544-571 | flag wrapper | **stays in main** |

The pure core is stdlib-only: no `shared.*`, no `ledger`, no git facts, no TOML reader.

### Python seam (`lib/_internal/recipe-materialize.py`), unchanged by this delivery
`GO_MERGE_CONFIG_BRIDGE_FALLBACK` :2830, `_merge_config_request_envelope` :2842-2881,
`go_merge_config` :2897 (flag `--plan-merge-config` :2946, stdout `{config, warnings, error}`, exit 0
with a non-empty `error` for a semantic failure and no fallback in that case), `merge_config` :2978,
and the fail-open body `_python_merge_config` :3006-3068 that PR2b's differential will use as the
oracle. Caller at :4021, inside the per-recipe loop and before the `project_owned_recipe_config`
override :4025-4026.

## Measured
- Total: **1260 add+del** — tracked 682 (94 insertions, 588 deletions across `SHA256SUMS`,
  `mergeconfig.go`, `recipeconfigwrite.go`, `recipeconfigwrite_test.go`) plus 578 new lines
  (`shared/mergeconfig.go` 414, `shared/orderedmap.go` 164).

## Constraints
- Budget: **1500 additions plus deletions**.
- The extraction is a behavior-preserving refactor whose checks are the existing gate suite plus
  `tests.test_merge_config_bridge` and `tests.test_recipe_config_write_bridge`, so no new RED cycle is
  claimed for it.
- The CPython parity code moves byte-for-byte: ordered key order, `pyRepr`/`pyReprString` quote
  selection and control-character escapes, and every validation error string are the contract. No
  cleanup, no "improvement", no reordering.
- The mechanical edits must be verifiable as mechanical: no logic change and no reformatting of
  unrelated code. `mergeconfig_test.go` and every other gate test file stay byte-unchanged.
- No production Python change, no CLI wiring, no dependency change. Do not touch `development`.
- Parent owns the commit, push, PR, merge and cleanup.

## Tasks
- [x] T1: Enumerate the export surface from `mergeconfig_test.go` and the recipeconfigwrite references.
- [x] T2: Move the merge-config decision core into `shared/orderedmap.go` and `shared/mergeconfig.go`.
- [x] T3: Leave `gate/mergeconfig.go` as the thin wrapper with the historical aliases and the
      unchanged `runPlanMergeConfig`.
- [x] T4: Apply the mechanical-only `recipeconfigwrite.go` and `recipeconfigwrite_test.go` updates.
- [x] T5: Regenerate the four `SHA256SUMS` digest lines with go1.24.13 and verify them with
      `scripts/verify-gate-sums.sh`.
- [ ] T6: Independent verification of the move parity, the mechanical-only claim and the trust root.
- [ ] T7: Full `./tests/validate.sh` with durable logs and exit evidence.
- [ ] T8: Commit the work unit and run the native review to closure.
- [ ] T9: Push, PR, merge into the epic and clean the worktree.

## Acceptance criteria (PR2a)
- Gate stdin/stdout envelope fields, JSON order and exit codes remain unchanged.
- Ordered key order, `pyRepr`/`pyReprString` output and every validation error string stay
  byte-identical; the moved bodies are textually identical apart from package and visibility renames
  (the only non-body deltas are the rewritten package docs, one added doc line and the new accessor
  docs; the verifier enumerated them).
- The `recipeconfigwrite` diffs contain only the approved accessor renames.
- `mergeconfig_test.go` and every other gate test file are byte-unchanged.
- All gate tests, the root module tests and both bridge suites pass.
- The four trust-root digests match a rebuild through `scripts/build-gate.sh` under go1.24.13.
- The complete authored candidate stays within the 1500-line budget.

## Checks
- Gate: `go -C catalog/recipes/worktree-flow/gate test ./... -count=1` and `go vet ./...` there.
- Bridges: `python3 -m unittest tests.test_merge_config_bridge` and
  `python3 -m unittest tests.test_recipe_config_write_bridge`.
- Root: `go test ./cmd/... ./internal/... -count=1`, `gofmt -l`, `git diff --check`.
- Trust root: `scripts/build-gate.sh` plus `scripts/verify-gate-sums.sh`.
- Full: `./tests/validate.sh` with durable logs and captured exit evidence.
- Environment: the real toolchains with the Python 3.14.7 oracle prefixed on PATH
  (`~/.local/share/mise/installs/{go/1.24.13,python/3.14.7}/bin`), never the mise shims.
- Native review uses the exact committed slice base `7384c65`, not the whole epic.

## PR2b backlog (deferred, recorded here so it is not lost)
- Root native authority `internal/sync/mergeconfig.go`: boundary request + `PlanMergeConfig` over the
  shared core, with the adapter over `internal/schema.LoadRecipeToml` (`recipe.go:1506`, `ConfigSchema`
  `:161`) and `internal/config.LoadManifest` (`config.go:47`).
- Needs the authorized export in `internal/schema/recipe.go` (+ its focused test): an order-preserving
  structured-shape accessor, because `ConfigTable.Shape` is an unexported `*shapeTable`.
- Must also solve: `ConfigSchema.Fields` is a map, so declaration order is lost and the adapter must
  re-parse `recipe.toml` through `internal/toml` (already an allowed surface); and Python float repr
  differs (`json.dumps(allow_nan=False)` emits `1.0` where `encoding/json` emits `1`), so a
  Python-compatible float formatter is required to preserve int/float through the `json.Number` path.
- Differential `internal/sync/mergeconfig_test.go` + `internal/sync/testdata/mergeconfig_ref.py`
  against `_python_merge_config` (:3006-3068) and the envelope builder (:2842-2881).
- Do NOT unify the two pre-existing repr ports (`internal/schema/recipe.go:301-467`,
  `internal/config/config.go:176-257`) with the shared one.

## Progress
- Slice prepared at base `7384c65`; scope mapped read-only with measured line counts; the human
  confirmed the four-PR re-split and then the PR2a/PR2b split.
- T1-T5 implemented by a delegated writer. T1 finding: `mergeconfig_test.go` references only
  `mergeConfigResult` (it drives the CLI through `run`), so the real consumer of the moved container is
  the recipe-config-write domain; the mechanical updates there are 27 and 33 line substitutions
  respectively.
- Writer checks: gate suite `ok` (worktree-gate, ledger, shared) and `go vet` clean; root module `ok`;
  `gofmt -l` and `git diff --check` clean; `tests.test_merge_config_bridge` 44/44 OK;
  `tests.test_recipe_config_write_bridge` 10/10 OK; the four digests regenerated with go1.24.13 and
  `scripts/verify-gate-sums.sh` reported `ok — 4 digest entries match the committed trust root`.
- Independent verification: **PASS** on every claim (move parity, wrapper fidelity, mechanical-only
  recipeconfigwrite updates, test integrity, defect hunt, all four suites, an independent gate rebuild
  reproducing the four digests, and the 1260-line budget). The verifier confirmed `Keys()`/`Values()`
  alias the storage rather than copying it, so the replaced field reads are behavior-neutral, and it
  ran both bridge suites itself (44/44 and 10/10). Two documentation corrections it raised are applied
  above: the pre-image is 571 lines with `planMergeConfig` at :416 and `runPlanMergeConfig` at :544,
  and the "textually identical" claim holds for bodies and strings, not for the reworded comments.
- Residual note carried into PR2b: `Keys()`/`Values()` expose aliased storage, which is harmless for
  the read-only callers today but is a latent footgun for new consumers.
- Full validation: `./tests/validate.sh` reported exit 0 with `Ran 2482 tests in 1598.572s` then `OK`,
  and zero `FAIL:`/`ERROR:` lines. Durable evidence: `~/.cache/s13-pr2a-validate.log` and
  `~/.cache/s13-pr2a-validate.exit`.
- Work unit commit: recorded below.

## Rollback
Revert this PR's extraction, the mechanical recipeconfigwrite updates, the four checksum entries and
this document together. PR2b must not integrate before this shared API exists, and PR3
(resolved-config) must not start before PR2b lands, because resolved-config depends on the same
`orderedMap`/repr shared surface staying in place.
