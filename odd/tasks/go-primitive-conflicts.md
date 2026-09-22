# ODD Feature: go-primitive-conflicts

## Objective

Migrate the Python `check_recipe_conflicts` primitive-conflict grader to the Go
worktree-gate binary, preserving behavior, with a fail-open Python bridge.

## Tracker

- card_id: GO-04 (6ab1d420edcaa0eb847b7297)
- url: https://trello.com/c/Yp3BHp1T

## Problem and decision

Rank-3 closure of the Python-to-Go strangler (materialization/sync domain), slice 1
per the inventory at development d6c820d (Engram topic_key
`follow-up/rank3-closure-inventory`). Authoritative conflict/decision logic belongs
in Go per AGENTS.md; Python keeps a thin fail-open bridge.

## Scope

- **WU1 (PR 1)**: Go core — new `recipePrimitivesReader` in recipe_toml.go +
  `--resolve-primitive-conflicts` flag in main.go + tests. Digest regen
  (go1.24.13) + SHA256SUMS.
- **WU2 (PR 2)**: Python bridge in recipe-materialize.py calling the gate binary
  with fail-open semantics, reusing `_conflicts_from_envelope`
  (recipe-materialize.py:1169). Parity tests.

## Parity traps (from inventory)

- `Conflict.recipes` uses recipe NAMES, not TOML ids.
- First collision aborts remaining claims per recipe.
- Claim order: skill → command → mcp.

## Non-goals

- No migration of stamp_recipe_reconcile_defaults (slice 2).
- No write-actuator chain (slice 3).
- No changes to _load_gate_binary dedup beyond what the bridge needs (reuse one
  home-resolution variant; no drive-by rewrites).

## Tasks

- [ ] WU1: Go conflict decision core + flag + tests + digests
- [x] WU2: Python fail-open bridge + parity tests
- [ ] Full validation (`./tests/validate.sh`)

## Evidence

### WU1 — Go conflict decision core + flag + tests + digests

**Built**

- `catalog/recipes/worktree-flow/gate/recipe_toml.go`: new `recipePrimitivesReader`
  (stdlib-TOML seam) + `recipePrimitives` type + `loadRecipePrimitives`, reading
  `[recipe].id`, `[recipe].name` and ordered `provides.skills/commands/mcp[].id`
  per recipe; ordered output reconstructed by walking the deduped id slice.
  Unlike sibling readers, an unreadable/unparseable/schema-invalid recipe.toml
  is a parser failure (exit 2 upstream), never a silent omission — the Python
  authority raises `RecipeValidationError` for the same shapes and the WU2
  bridge falls back on exit 2.
- `catalog/recipes/worktree-flow/gate/primitiveconflicts.go` (new): registry +
  conflict collection (`checkPrimitiveConflicts`) and
  `runResolvePrimitiveConflicts` (JSON stdout, exit 0 on any graded result,
  exit 2 on unusable flags/parser failure). Claim order skill → command → mcp,
  recipes in flag order, first collision aborts the recipe's remaining claims,
  colliding recipe never registered as owner, `recipes` sorted unique
  `[recipe].name` values, severity always "fatal".
- `catalog/recipes/worktree-flow/gate/main.go`: `--resolve-primitive-conflicts`
  bool flag reusing shared `--catalog-dir` / repeatable `--recipe`, dispatched
  in the flag switch.

**Tests** — `catalog/recipes/worktree-flow/gate/primitiveconflicts_test.go`
(404 lines): `TestCheckPrimitiveConflicts` (11 table cases incl. third-recipe
pairing, first-collision abort, cross-type independence, same-recipe duplicate,
sorted/deduped names), `TestCheckPrimitiveConflictsOrderFollowsRecipeOrder`,
`TestLoadRecipePrimitives` (+ Order, Dedup, RejectsUnreadable variants),
`TestPrimitiveConflictPlanJSONShape` (exact stdout pin),
`TestRunResolvePrimitiveConflictsCommand` (5 end-to-end runCLI cases).

**TDD evidence**

- RED: `go test ./... -run 'Primitive'` → build failure, all decision-core
  symbols undefined (observed before implementation).
- GREEN: same command after implementation → all Primitive tests pass; full
  `go test ./...` in the gate module → `ok ai-specs.dev/worktree-gate` +
  `ok .../ledger` (two test-expectation fixes during GREEN were test bugs:
  the first-collision abort means one recipe yields at most one conflict).
- Full gate package suite green; no unrelated tests touched.

**Digests**

- Canonical toolchain confirmed: `go version go1.24.13` (build-gate.sh emitted
  no non-canonical warning).
- `./scripts/build-gate.sh` → 4 targets built into `dist/`.
- `catalog/recipes/worktree-flow/bin/SHA256SUMS` regenerated with a
  regeneration note; `(cd dist && shasum -a 256 -c
  ../catalog/recipes/worktree-flow/bin/SHA256SUMS)` → all four `OK`.
- Live smoke test of `dist/worktree-gate-current --resolve-primitive-conflicts`:
  conflict JSON exit 0; missing recipe dir → exit 2.

**Semantic deviation from the task spec (verified, intentional)**

Spec point 4's parenthetical said "same-name re-claim does not conflict". The
Python code (recipe-conflicts.py:118-127) says otherwise: `claim` raises
unconditionally when the id is already present — there is no owner-differs
check. Per the spec's own instruction ("verify against the Python code and
replicate"), the Go core conflicts on a same-recipe duplicate id, with the
recipe pair collapsing to one sorted-unique name. Pinned by the
"one recipe claiming the same id twice conflicts with itself" test case.

**Known reader-scope boundary (risk for WU2 parity tests)**

The reader validates only what it consumes (parse, `[recipe]` id/name,
provides entries as objects with non-empty string ids, Python's swallow rules
for non-table `[provides]` and non-list values, `_require_string` stripping).
Other schema raises (missing skill `source`, `source='dep'` without `url`,
missing command `path`, description/version/tags, etc.) are not replicated, so
Go can grade a recipe Python would reject at load. Such recipes would exit 0
where the bridge would have fallen back to Python — acceptable for the strangler
slice, worth pinning in WU2 parity tests.

## Workflow rules honored

- Dedicated worktree `.worktrees/go-primitive-conflicts`, branch
  `feat/go-primitive-conflicts` from development d6c820d (user-selected
  destination via gate prompt).
- PRs against `development` via gh; no direct pushes to development.

### WU2 — Python fail-open bridge + parity tests

**Built**

- `lib/_internal/recipe-materialize.py`: new primitive-conflict bridge section
  after the tag-conflict section — `GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK` /
  `GO_PRIMITIVE_CONFLICTS_BRIDGE_TIMEOUT_SECONDS` (60) constants,
  `_warn_primitive_conflicts_bridge_fallback` (one greppable warning line),
  `go_recipe_conflicts` (mirrors `go_tag_conflicts`: `_load_gate_binary()` +
  `resolve_verified_binary(_bridge_home(catalog_dir))`, flag order
  `--resolve-primitive-conflicts --catalog-dir <dir> [--recipe <id>]...`,
  locale-proof `encoding="utf-8", errors="replace"` decode, exactly one
  fallback warning per failure reason, envelope validation accepting an empty
  list as a clean result) and `_python_check_recipe_conflicts` (TEMPORARY
  authority docstring).
- `check_conflicts` (line 348) is now the Go-primary wrapper: Go envelope via
  `_conflicts_from_envelope` when present, else the Python authority. No other
  call sites changed (sole production caller remains the sync materialization
  flow, now line 2515).

**Tests** — `tests/test_recipe_conflict_bridge.py` (13 tests):
- Go authority (stub binary, no skip): Go grades and Python authority is never
  reached (mocked to assert); argv flag/order contract; empty envelope is a
  valid clean result without warning or fallback.
- Parity (real `dist/worktree-gate-current`, loud skip without it): fixture
  catalog skill/command/mcp conflicts — Python and Go agree on shape, recipe
  NAMES, sorting, and skill→command→mcp order; clean catalog returns empty
  from Go; invalid TOML → exit 2 → fallback → the unchanged Python parse error
  propagates; missing recipe dir → exit 2 → fallback →
  `RecipeValidationError` (the exact pre-bridge raise).
- Fail-open fallback (no usable binary): missing binary / nonzero exit /
  non-JSON / malformed envelope each produce exactly ONE
  `GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK` warning and still return the Python
  result; envelope item without `type` defaults to `capability` via
  `_conflicts_from_envelope`; source marker pin (TEMPORARY docstring).

**TDD evidence**

- RED: `python3 -m unittest tests.test_recipe_conflict_bridge` → 12 tests,
  10 errors + 2 failures (bridge did not exist: `check_conflicts` went
  straight to the Python module; `GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK` and
  `_python_check_recipe_conflicts` undefined). Observed before implementation.
- GREEN: same command → 13 tests OK (added a missing-recipe-dir exit-2 case
  while triangulating, see deviation below). Real Go binary ran the parity
  and exit-2 tests (no skips).
- Focused run: `python3 -m unittest tests.test_recipe_conflict_bridge
  tests.test_recipe_conflicts tests.test_tag_conflict_bridge` → 41 tests,
  all OK, 0 skips.

**Deviation from the spec (small, verified)**

The spec's invalid-recipe.toml bullet expected `RecipeValidationError` from
the fallback. The Python authority raises a raw `tomllib.TOMLDecodeError` for
a TOML *parse* failure; `RecipeValidationError` is for missing files and
schema errors (`recipe_schema.load_recipe_toml`). Kept the invalid-TOML test
pinning the unchanged parse error, and added a missing-recipe-dir test that
pins `RecipeValidationError` exactly — both are Go exit-2 boundaries, so the
WU1 pin (Go lenient grading never masks a Python raise) holds either way.
`tests/test_recipe_conflicts.py` needed no changes: it exercises
`recipe-conflicts.py` directly and asserts nothing about the old
`check_conflicts` wiring.

**Scope confirmation**

- Only `lib/_internal/recipe-materialize.py` (+109/-2) and new
  `tests/test_recipe_conflict_bridge.py` changed; the Go gate module,
  `dist/`, and digests are untouched from WU1.
- `check_conflicts` at the sync call site keeps byte-identical fatal messages
  (the `Conflict` dataclass is reused via `_conflicts_from_envelope`).
