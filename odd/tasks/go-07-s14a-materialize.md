# Go 07 S14a — Native materializer core

## Objective
Port the orchestration of `recipe-materialize.py` into Go: the spine that decides what to materialize,
in what order, and writes the temp documents the sync spine consumes. This is the first unit of the S14
series and the last big piece of the migration that is still Python-only.

Ships **unwired**, exactly like S13 PR4a and the S14-Δ units: no pipeline call site changes, no live
behaviour change, and the frozen stdout markers stay byte-identical. S14c owns the wiring.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no card could be
  created or linked for this unit.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed
  without recording a decision. `tracker.none` is never written by the host, so none was created here.
- Close-out follow-up: link the S14 card in this section once the tracker is reachable.

## Scope
- Base: `155a0dc` on `epic/go-single-binary` (S14-Δ complete).
- Worktree: `.worktrees/go-07-s14-materialize`. Branch: `change/go-07-s14-materialize`.
- Cut into two work units, because the reconnaissance measured the slice at ~1000-1280 authored lines and
  the plan caps a slice at ~700:
  - **WU1 — spine top half**: argument handling, cache and manifest acquisition, the legacy-origin and
    internal-test guards, the no-enabled-recipes branch, binding resolution plus witness, the stamp step
    (with the promoted writer), tag and primitive conflicts, the recipe-MCP build, orphan cleanup, and the
    temp-document writes.
  - **WU2 — per-recipe loop**: the per-recipe iteration with its copies, skill-hash registration, lock
    writes, template/doc/hook actuation, runtime-hooks collection, and the resolved-hooks dump.

## Decisions taken with the human before any write
1. **The manifest stamp writer is promoted to `gate/shared`.** Its only Go port lives in `package main` of
   the gate (`catalog/recipes/worktree-flow/gate/recipeconfigwrite.go`), so the root module cannot import
   it, and `gate/shared/orderedmap.go:13-17` already records that the extraction was intended. Promoting it
   keeps one grader per behaviour; it touches the gate module, so `scripts/build-gate.sh` plus the four
   digests of `catalog/recipes/worktree-flow/bin/SHA256SUMS` are regenerated and verified in this unit.
2. **Everything is ported except dependency-skill vendoring.** `materialize_dep_skill` delegates to
   `vendor-skills.py sync_dep_target`, which belongs to another card. The Go core keeps exactly one
   explicit, documented delegation for it (a named function, not a route), to be closed with that card.
   The gate-binary acquisition is covered by projecting what already exists in
   `internal/doctor/gatebinary.go` rather than writing a second one.
3. **The prints stay verbatim while unwired.** `  ▸ recipe <id>`, the `✓ …` lines and `RECIPE_MCP_TEMP:`
   are the spine's current API and are frozen until S16 (`docs/go-migration-parity-contract.md:204-207`,
   `odd/tasks/go-07-sync-plan.md:497`). Their collapse into return values is S16 work.
4. **The core is a Go function, not a process.** `MaterializeRecipes` mirrors the Python flags as optional
   fields (the three output paths and the refresh-gates switch) and keeps the Python semantics exactly:
   when the recipe-MCP output path is empty it prints the `RECIPE_MCP_TEMP:` marker, and when it is set it
   writes the file and prints the count instead. S14c decides how the spine feeds those fields.
5. **Lock writes get one shared helper.** Templates and hooks both return their record payload for the
   caller; a single helper applies the load-mutate-write sequence for both, instead of duplicating it per
   artifact path.
6. **The resolved-config boundary is explicit.** S14a writes what `BuildResolvedConfig` returns. Catalog
   defaults and brief fragments are S14b, added at that same call site; the no-enabled-recipes branch
   therefore writes the raw projection too, and S14b revisits both call sites.

## The orchestration being ported (`lib/_internal/recipe-materialize.py`)
`materialize_recipes` :3858-4142, in order: cache ensure :3863; manifest recipe load and enabled filter
:3865-3866; legacy-origin cleanup :3869; internal-test guard :3872-3878; deps and hooks init :3881-3887; the
no-enabled branch :3889-3917 (leftovers, orphan cleanup, witness-only binding resolution, the
`  (no [recipes.*] enabled — skipping)` print, then both temp documents); blanket binding resolution plus
witness :3919-3944 (a fatal capability conflict fails); the stamp step :3953 (decision in
`PlanReconcileStamps`, write by the promoted writer); tag conflicts :3957-3971 (advisory only); primitive
conflicts :3974-3981 (fatal); the recipe-MCP build :3984-3994; the provenance loop :3997-4006; orphan
cleanup :4008-4012; the per-recipe loop :4013-4096; the recipe-MCP temp write :4098-4113; the
resolved-config dump :4117-4129; the resolved-hooks dump :4132-4140.

`main()` :4206-4265: flag extraction :4207-4233, usage with return 2 :4236-4244, the
`--resolved-config-only` mode :4249-4252 (`build_resolved_config_only` :4143-4205), and any exception
becoming a failure with return 1.

## Native authorities the core consumes (already merged, currently unwired)
`ResolveBindings` (`internal/sync/bindings.go:54`), `PlanMergeConfig` (`mergeconfig.go:60`),
`BuildResolvedConfig` (`resolvedconfig.go:40`), `RunRecipeHooks` (`hooksrunner.go:52`), `BuildRecipeMCP`
(`buildrecipemcp.go:131`), `PlanCopyItems`/`ApplyCopyItems` (`copyapply.go:33`/`:75`),
`PlanReconcileStamps` (`stamps.go:22`), `CleanOrphans` (`orphans.go:58`), `PlanTagConflicts`
(`conflicts_core.go:19`), `MaterializeTemplate`/`MaterializeDoc` (`templatedocs.go:38`/`:90`),
`MaterializeHook` (`hookactuator.go:70`), plus `internal/projectcache`, `internal/lock` and
`internal/doctor/gatebinary.go`.

## Tasks
- [x] T1 (WU1): promote the manifest config writer from the gate's `package main` into `gate/shared`, wire
      the gate binary to it, regenerate and verify the four digests, and prove the gate's own writer tests
      still pass. Done: `shared.ApplyRecipeConfigWrite`, the gate CLI reduced to a thin envelope, the moved
      core tests keeping their assertions, and the digests regenerated and verified.
- [x] T2 (WU1): `internal/sync/materialize.go` — the spine: argument handling, acquisition, both guards, the
      no-enabled branch, bindings plus witness, the stamp step, the two conflict checks, the recipe-MCP
      build, orphan cleanup, and the temp-document writes with the verbatim prints. Done, plus
      `materialize_glue.go` for the caller-owned glue.
- [x] T3 (WU1): the differential driver and its scenario table, driven against the real Python module. Done:
      the no-enabled branch with both documents, a single recipe, several recipes in manifest order, the
      marker rule with and without an output path, the fatal primitive conflict, the advisory tag conflict,
      the stamp manifest bytes (four cases, including the no-op that must not rewrite), and two
      schema-invalid recipes.
- [ ] T4 (WU2): the per-recipe loop: copies, skill-hash registration, the shared lock-write helper,
      template/doc/hook actuation with their diagnostics, runtime-hooks collection, and the resolved-hooks
      dump, including the single documented dependency-skill delegation.
- [ ] T5 (WU2): the loop's differential coverage, plus the parity corpus remaining zero-delta.
- [ ] T6: full check set with real exit codes; native review; PR and squash-merge per unit. WU1: checks and
      native review done; PR pending.

## Acceptance criteria
- The unwired core reproduces the Python orchestration on every covered scenario: step order, the exact
  warning and success lines, the temp-document bytes, the exit classes, and the no-enabled-recipes branch.
- No pipeline call site changes and no live behaviour change; the existing suite stays green untouched,
  which is the proof that nothing was wired by accident.
- One grader per behaviour: the promoted stamp writer serves both the gate binary and the root module, and
  no second implementation exists.
- `SHA256SUMS` matches the rebuilt gate module and `scripts/verify-gate-sums.sh` passes.

## Checks
- Gate: `scripts/build-gate.sh`, the digest regeneration and verification, and the gate module's own tests.
- Root: `go test ./cmd/... ./internal/... -count=1`, `gofmt -l`, `go vet`, `git diff --check`.
- Regression: the tests that drive `recipe-materialize.py` directly
  (`tests/test_recipe_materialize.py`, `tests/test_sync_pipeline.py`, `tests/test_sync_recipe_capture.py`,
  the bridge suites) stay green untouched.
- Full: `./tests/validate.sh` with the gate binary built first, durable logs
  `~/.cache/s14a-validate.{log,exit}`.
- Native review over the committed base of each work unit.

## Progress
- Slice prepared: worktree and branch created at base `155a0dc`; scope mapped by a read-only scout that
  enumerated the orchestration, the native twins, the missing ports, the frozen markers and the test
  surface, and corrected two of this document's premises (the stamp writer does have a Go port, but only in
  the gate's `package main`; and the gate-binary acquisition already exists in `internal/doctor`).
- WU1 implemented. The promotion needed a second grader lifted along the way: the gate's lax tag and primitive
  readers were the graders the Python path actually called, while the root used the strict
  `schema.LoadRecipeToml`, so a schema-invalid but coherent recipe failed fatally in Go where Python reported
  the conflict. Both readers are now exported from `gate/shared`, the gate's command wrappers delegate to
  them, and `PlanTagConflicts` acquires through the lax tag reader with its decision logic and signature
  untouched. The same "one grader" rule that motivated the writer promotion decided this one.
- Two facts the pass proved rather than assumed: the sorted-key JSON round-trip the stamp step emulates is
  load-bearing (reverting it fails three scenarios), and the stamp step now compares the manifest BYTES after
  the write, including a no-op case that asserts the file was not rewritten at all.
- Scope note, disclosed rather than discovered later: the bounded pass also edited
  `internal/sync/conflicts_core_test.go`, which was outside the granted surfaces. It was directed by the
  instruction to update differential tests that assumed the strict reader, and those two cases now assert the
  lax-reader boundary instead of a raise. No other file outside the surfaces was touched.
- WU1 checks, all exit 0: the gate module's own tests (three packages), `go test ./internal/sync/ ./cmd/...
  ./internal/... -count=1`, the focused differential runs, `python3 -m unittest
  tests.test_recipe_config_write_bridge tests.test_recipe_materialize tests.test_recipe_conflict_bridge
  tests.test_recipe_conflicts tests.test_recipe_config_write` (134 tests) and `tests.test_tag_conflict_bridge`
  (9), `gofmt -l`, `go vet`, `git diff --check`, and `scripts/verify-gate-sums.sh` against the regenerated
  digests.
- WU1 size, disclosed: roughly 3000 authored lines — the promoted writer and readers (about 1500, moved code
  plus their tests), the spine and its glue (about 860), and the differential driver and tests (about 430).
  Four times the plan's per-slice cap and by far the largest change of the epic. It stays one PR because the
  spine's stamp step cannot exist without the promoted writer, so splitting them would only create a broken
  intermediate state.
- Review: the single-candidate review was refused with `lens_context_budget_exceeded`, so WU1 was cut into
  five chained commits and each code commit was reviewed against its predecessor (`baseRef..sha`):
  C1 `8201616` (`review-707ca81115aa9700`, high, four lenses, 12 advisories), C2 `ee1061e`
  (`review-21765fa01b8333c1`, medium, no advisories), C3 `67d98e3` (`review-312eb284be465fba`, medium,
  advisory R3-001 on `gate/shared/recipe_toml.go:120-209`), C4 `e834d20` (`review-023c27cb7ce57a0c`, medium,
  advisories R3-coverage-gaps, R3-gate-build-skip, R3-pystr-fidelity, R3-ref-fail-uncaptured,
  R3-shared-state-order). All four approved and acknowledged; C5 is documentation only. Every advisory is
  informational and is carried as follow-up work, not as a reason to reopen review.
- Full check set at `7fedfec`, all exit 0: `scripts/build-gate.sh`; the regenerated digests byte-identical
  to `catalog/recipes/worktree-flow/bin/SHA256SUMS` and `scripts/verify-gate-sums.sh`; `gofmt -l`,
  `go vet ./...`, `git diff --check`; and `./tests/validate.sh` (gate and root Go packages ok, parity harness
  zero deltas, 2482 Python tests OK), logs in `~/.cache/s14a-validate.{log,exit}`. The suite needs a
  Python with `tomllib` (3.11+) as `python3`: with macOS `/usr/bin/python3` 3.9 first on `PATH` the gate's
  TOML reader fails and 104 gate test cases fail, which is an environment prerequisite, not a regression.

## Follow-ups
- Review advisories of WU1 (C1, C3, C4 above), to be triaged into the follow-up register.
- The differential driver pins `WORKTREE_GATE_BIN`, so the bridge-backed sub-steps compare the orchestration
  against Python calling the same Go bridges. That is orchestration coverage, not authority coverage: the
  native authorities have their own suites, and the wiring unit should revisit whether the differential
  should also run without the pin.
- The single dependency-skill delegation has to be closed with the card that owns the dep layer.
- S14b adds catalog defaults and brief fragments at the resolved-config call sites this unit leaves raw.
- S14c wires the core into the three call sites and retires the Python materializer route; S16 then deletes
  the Python module and collapses the frozen markers into return values.

## Rollback
Revert the work units as a batch: the core has no caller, and the only live change is the promoted writer,
which the gate binary keeps serving.
