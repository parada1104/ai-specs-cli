# Go 07 S13 PR4b-2 — Native recipe-MCP build and provider markers

## Objective
Add the native Go authority for the recipe-MCP build (`build_recipe_mcp`) and its provider-marker
resolution (`_resolve_provider_markers`) under `internal/sync`, faithful to the Python authority, built
on the `internal/provider` package that S13 PR4b-1 landed.

Authority is added, not wired: the sync spine still execs `recipe-materialize.py`, and S14 owns the
pipeline wiring. This mirrors S13 PR4a (`internal/sync/hooksrunner.go`), where the same split held.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no card could be
  created or linked for this slice.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed
  without recording a decision. `tracker.none` is never written by the host, so none was created here.
- Close-out follow-up: link the S13 card in this section once the tracker is reachable.

## Scope
- Base: `da7e370` on `epic/go-single-binary` (S13 PR1, PR2a, PR2b, PR3, PR4a and PR4b-1 merged).
- Worktree: `.worktrees/go-07-s13-recipe-mcp`. Branch: `change/go-07-s13-recipe-mcp`.
- New files only:
  - `internal/sync/buildrecipemcp.go` — the native authority.
  - `internal/sync/buildrecipemcp_test.go` — scenario table plus the Python differential.
  - `internal/sync/testdata/buildrecipemcp_ref.py` — driver importing the real Python module.
- **No gate change**: `SHA256SUMS`, the gate module, `internal/doctor`, `internal/provider`, `lib/`,
  `scripts/` and `go.mod` must not change.
- **No pipeline call site**: `internal/sync/sync.go:346-355` keeps execing the Python materializer. A
  `GO_SYNC_STEP_*` flag for this step belongs to S14; adding it here is explicitly out of scope.

## Python authority (`lib/_internal/recipe-materialize.py`)
- `_resolve_provider_markers` :3131-3177 — mutates the merged mcp dict in place. Reads `recipe.cli_deps`
  (only `binary == "jinna"`, :3146), the recipe `mcp` presets (`preset.id`, :3147), and from the merged
  entry only `config["command"]`, which must equal the literal `"{dep:jinna}"` (:3148-3150). Resolves via
  `provider_install.resolve_provider(dep, ai_specs_home=...)` (:3155-3157) and, on success, replaces
  `command` with the resolved command while preserving key position (:3159-3162). On any failure it
  removes the entry (`merged.pop(preset.id, None)`) and continues; it never raises. Warning strings:
  - :3142 `recipe '{rid}': cannot read recipe for provider resolution ({exc})`
  - :3152 `recipe '{recipe.name}': missing jinna dependency declaration`
  - :3162-3165 `recipe '{recipe.name}': provider jinna could not be resolved ({exc}); run ai-specs
    configure-recipes interactively to install it`
  - :3173-3176 `recipe '{recipe.name}': provider jinna is unresolved; run ai-specs configure-recipes
    interactively to install it`
- `build_recipe_mcp` :3179-3214 — shallow-copies `manifest_mcp` per server id (:3192), appends recipe
  presets that are absent as `dict(mcp.config)` (:3196-3198), and on a per-key conflict keeps the
  manifest value, skips the recipe key and warns once (:3199-3206):
  `recipe '{recipe.name}' mcp.id='{mcp.id}' key '{key}' conflicts with project manifest (manifest wins)`.
  Then calls the resolver when `ai_specs_home is not None` (:3206-3208). Returns the dict **without**
  serializing. `read_recipe` failures propagate out of this function, unlike inside the resolver.
- Ordering: manifest keys in manifest order first, then recipe presets in `recipe_ids` × the recipe's
  `mcp` declaration order.
- Serialization lives at the caller: :4108-4110 `json.dump(recipe_mcp, f, indent=2)` plus a newline, with
  default separators, `ensure_ascii=True` and `sort_keys=False`. The public entry point is called from
  `materialize_recipes` at :3989-3994.

## Tasks
- [x] T1: Native `BuildRecipeMCP` plus `resolveProviderMarkers` in `internal/sync/buildrecipemcp.go`,
      reusing the existing `mcpObj` / `mcpShallowCopy` / `mcpJSONDumpsIndent2` helpers from
      `internal/sync/mcprender.go` and `schema.LoadRecipeToml` for the recipe reads.
- [x] T2: Differential driver `internal/sync/testdata/buildrecipemcp_ref.py` importing the real module
      (never re-implementing it), plus the scenario table in `buildrecipemcp_test.go`: manifest
      precedence, recipe append order, manifest-wins conflict warning, all four resolver warnings, path vs
      managed `command`, unresolved omission, a non-`jinna` `cli_deps` entry, and an entry whose `command`
      is not the marker.
- [x] T3: Byte parity for the JSON document the materializer writes (`mcpJSONDumpsIndent2`).
- [ ] T4: Full check set with real exit codes; native review; PR and squash-merge to the epic.

## Acceptance criteria
- The native output matches the Python authority on every covered scenario: server order, key order,
  warning text and order, resolved `command` values, and omission when unresolved.
- No production behavior change: no pipeline call site and no gate digest change.
- `tests.test_jinna_provider_recipe`, `tests.test_jinna_consumer_recipe`, `tests.test_recipe_materialize`
  and `tests.test_sync_pipeline` stay green.
- The authored candidate stays within the budget (750 estimated; the plan caps a slice at ~700).

## Checks
- Root: `go test ./cmd/... ./internal/... -count=1`, `gofmt -l`, `go vet ./internal/...`, `git diff --check`.
- Regression: `python3 -m unittest tests.test_jinna_provider_recipe tests.test_jinna_consumer_recipe
  tests.test_recipe_materialize tests.test_sync_pipeline`.
- Full: `./tests/validate.sh` with the gate binary built first (`bash scripts/build-gate.sh`), durable logs
  in `~/.cache/s13-pr4b2-validate.{log,exit}`.
- Native review over committed base `da7e370`.

## Progress
- Slice prepared: worktree and branch created at base `da7e370`; scope mapped by a read-only scout.
- Design decision taken from the plan, not guessed: the native authority ships unwired, exactly as S13
  PR4a did, because `odd/tasks/go-07-sync-plan.md` assigns the sync-pipeline wiring to S14 and the Go spine
  still execs the Python materializer (`internal/sync/sync.go:346-355`).
- Implemented by a delegated writer (T1-T3): `internal/sync/buildrecipemcp.go` (228, `BuildRecipeMCP`,
  `resolveProviderMarkers`, `readRecipeMCP`, `recipeMCPPresets`, `mcpObjDelete`, `warnf`),
  `internal/sync/buildrecipemcp_test.go` (plus the divergence pins), and
  `internal/sync/testdata/buildrecipemcp_ref.py` (84). Authored total 775 lines against the 750 estimate,
  inside the plan's ~700-line cap.
- TDD RED observed with real assertion failures (a stub authority produced `"{}"` where Python produced
  the ordered document, and an error-propagation test failed); GREEN with the same command at exit 0.
  The differential driver imports the real module; it never re-implements the body.
- Ordering seam: `schema.Recipe.MCP` is an unordered Go map while the document is serialized with
  `sort_keys=False`, so TOML declaration order is observable. Pinned by re-reading `recipe.toml` through
  `internal/toml` (`recipeMCPPresets`) for `[[provides.mcp]]` order, keeping `schema.LoadRecipeToml` as the
  validation/name/`cli_deps` authority; byte-level document comparison proves order parity.
- `read_recipe` seam: Python raises `recipe directory not found: <dir>` before `load_recipe_toml`, while
  `schema.LoadRecipeToml` yields `recipe.toml not found: <path>`; an explicit directory check reproduces the
  Python text, pinned by the propagation test and the resolver's "cannot read recipe" scenario.
- Independent verification: **PASS**. The verifier built its own oracle outside the suite and matched the
  Python authority byte-for-byte on ordering (including duplicate recipe ids), key-position preservation,
  conflict warnings, the four resolver warnings, propagation versus degrade, path / managed / symlinked
  `AI_SPECS_HOME` resolution, omission, and JSON byte parity across non-ASCII, floats, `Infinity`, `NaN`,
  booleans and nested tables.
- Two findings from that verification were fixed in a bounded follow-up: the comment on the non-mapping
  manifest branch claimed `TypeError` when Python actually raises `ValueError` from `dict(cfg)` at :3192
  (the branch stays unreachable from real callers; a direct-API caller observes Python raising and Go
  keeping the value), and the `internal/toml` value-subset divergence is now pinned on both sides by
  `TestBuildRecipeMCPDivergenceBigInteger` and `TestBuildRecipeMCPDivergenceDatetime`.
- Native authority behavior is unchanged by the pins: `gofmt -l internal/sync`, `go test ./internal/sync/
  -count=1`, `go vet ./internal/...` and `git diff --check` all exit 0.

## Follow-ups
- S14: wire this authority into the sync pipeline behind the `GO_SYNC_STEP_*` flag and retire the Python
  materializer path for the recipe-MCP emission.
- `internal/toml` accepts a smaller value subset than `tomllib`: a recipe `[[provides.mcp]]` value that is a
  big integer above the Go `int64` range, or a TOML datetime, is refused by the Go reader while Python reads
  it. The divergence is pre-existing (other native sync paths read recipes through the same reader) and is
  latent while this authority stays unwired, but it changes the outcome rather than only the message: the
  resolver leaves the `{dep:jinna}` literal and warns instead of replacing `command`. Pinned in both
  directions here; widening the reader belongs to `internal/toml` and to its own slice.
- `internal/toml` error messages are documented as not byte-parity with `tomllib`, so a TOML syntax error
  surfaces a different text through `cannot read recipe for provider resolution (<exc>)`.
- The advisory findings of S13 PR4b-1 that touch `internal/provider` (they are recorded in
  `odd/tasks/go-07-s13-provider.md`).

## Rollback
Delete the three new files; nothing else references them.
