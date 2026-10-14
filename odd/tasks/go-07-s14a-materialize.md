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
   **Revised for WU2 (with the human):** the WU2 reconnaissance proved that premise wrong.
   `internal/doctor/gatebinary.go` only reads and verifies; the Python `acquire()`
   (`lib/_internal/gate_binary.py:437-681`: download, local build, quarantine, digest records) has no Go
   twin. WU2 therefore keeps a second named, documented delegation, `acquire` through Python, exactly like
   the dependency-skill one, and **WU3 ports `acquire` natively before S14c wires anything**.
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
- [x] T4 (WU2): the per-recipe loop: copies, skill-hash registration, the shared lock-write helper,
      template/doc/hook actuation with their diagnostics, runtime-hooks collection, and the resolved-hooks
      dump, including the single documented dependency-skill delegation.
- [x] T5 (WU2): the loop's differential coverage, plus the parity corpus remaining zero-delta. Done in
      `6ac0a83` (11 scenarios; parity 58 legs zero deltas).
- [ ] T6: full check set with real exit codes; native review; PR and squash-merge per unit. WU1: done, PR
      #352 squash-merged as `2de231d`. WU2: worktree `.worktrees/go-07-s14a-loop`, branch
      `change/go-07-s14a-loop`, base `2de231d`.
- [ ] T7 (WU3): port the gate-binary `acquire` to Go (one grader, shared with the doctor's read/verify
      primitives) and replace the WU2 delegation; its own differential against `gate_binary.py`.
      WU2 merged as PR #353 (`5933bb6`). WU3: worktree `.worktrees/go-07-s14a-gate-acquire`, branch
      `change/go-07-s14a-gate-acquire`, base `5933bb6`. Plan, from a read-only reconnaissance:
      - C1: new `internal/gatebinary` package porting `lib/_internal/gate_binary.py` in full (paths,
        platform, trust root, verification and receipts, mismatch records, quarantine, local build,
        `Acquire`), with unit tests mirroring `tests/test_gate_binary_dist.py`. Not shared with
        `internal/upgrade`: its `Acquire` has another contract (no self-test, receipts or quarantine) and its
        `ParseTrustRoot` validates differently.
      - C2: the doctor delegates its read-only twins to `internal/gatebinary`; `materialize_loop.go` calls
        the native `Acquire` instead of `delegateGateAcquireToPython`, keeping both frozen messages; a
        differential against Python `acquire` on temp homes (local HTTP server, fake `go`, offline mode).
      - Deliberate divergence: Python's `urlretrieve` has no timeout; Go uses 60 s, like `internal/upgrade`.
        Tests never reach the network.
      - State (C1 only, C2 not started): `2fecbcc` (port), `d0998ae` (receipt/download hardening),
        `f3614d9` (unresolved home fails closed), `637c171` (untrusted-cache threat model, decided with the
        human: private O_NOFOLLOW verify copies, install by rename, no-digest fails closed). Native review
        did not converge in four high-tier rounds (`review-870228bb459afd27`, `review-cdd02e2b042ca0f3`,
        `review-a35ede353bc9dc90`, `review-53df2adaf4e4ad6b`): every round confirms new CRITICAL findings for
        an attacker who can write into the user's own cache directory (latest: symlink-following mismatch
        record, resolve-then-exec race, staging-parent rename). The bounded-correction route is unusable on
        committed-only candidates (it ends in `captured_artifacts_unverifiable`). Open decision for the
        human: the security boundary for a same-user cache writer (see Follow-ups).

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
- WU2 implemented. The loop is `materialize_loop.go`; the project-owned grader is promoted to
  `internal/sync/projectowned.go` and doctor now delegates to it (one grader, doctor tests untouched).
  The differential table is `materialize_loop_test.go`: eleven scenarios (bundled skill + command with an
  overwrite warning and the lock bytes; templates and docs fresh / managed-current / user-modified; runtime
  hooks fresh / current / user-modified / refresh-gates; the UPPER-only env projection including `A1_B`,
  `1_A`, `a_B` and `A-B`; the dep delegation; an `execute_hooks` failure; and the worktree-flow gate
  acquisition). Each scenario runs the Python oracle and the Go loop on the SAME project/home paths with the
  project tree and CLI cache rebuilt between legs, comparing rc, stdout, stderr, the documents, the lock
  bytes and the whole materialized tree. RED was observed by disabling the WU2 call: all eleven scenarios
  fail, then all pass with it restored.
- Two WU2 facts the pass proved. First, the differential driver had a latent defect — its
  `except BaseException` handler called `mod.fail`, which prints to the restored stderr and raises
  `SystemExit` past `main`, so any Python exception (as opposed to an in-authority `fail()`) lost the
  captured fatal line and exited nonzero; the handler now writes `  ✗ {exc}` into the captured buffer. The
  bug only surfaced because the `execute_hooks` failure is the first scenario that reaches a deep exception.
  Second, the resolved-config document WITH enabled recipes is the one document the bare spine cannot yet
  match: Python adds `brief_fragments` (and catalog defaults), which is S14b by decision 6, so the WU2
  differential does not pin it and the WU1 no-enabled scenario keeps covering that writer.
- WU2 checks, all exit 0: `go test ./internal/sync/ -count=1`, `go test ./internal/doctor/ ./internal/lock/
  -count=1`, `go test ./cmd/... ./internal/... -count=1`, `gofmt -l cmd internal`, `go vet ./...`,
  `git diff --check`, and `python3 -m unittest tests.test_recipe_materialize tests.test_sync_pipeline
  tests.test_sync_recipe_capture` (174 tests). `scripts/build-gate.sh` was NOT run: it writes the tracked
  gate binary outside the WU2 edit surfaces, and the differential builds its own gate binary in a temp dir.
- WU2 size, disclosed: about 1370 authored lines — `materialize_loop.go` (722, including the two embedded
  delegation scripts and their doc comments), `materialize_loop_test.go` (571), `projectowned.go` (35), and
  the small edits to `materialize.go`, the ref driver and doctor. That is above the plan's ~1200 soft cap;
  the split, if the reviewer wants one, is the differential table (`materialize_loop_test.go`) as its own
  follow-up commit, since the implementation and its coverage are otherwise inseparable.
- WU2 non-native steps, the exact two the revised decision 2 permits: `delegateDepSkillToPython`
  (vendor-skills.py `sync_dep_target`, closed with the deps card) and `delegateGateAcquireToPython`
  (gate_binary.py `acquire`, replaced natively by WU3/T7). Both resolve their module through
  `<AI_SPECS_HOME>/lib/_internal/`, the same sibling layout the Python authority uses.
- WU2 review: `6ac0a83` reviewed against `2de231d` as `review-03a4bff3b005d59b` (high, four lenses, because
  the delegations start processes), approved and acknowledged. One `review-readability` capture failed with
  a transport error (`Stream ended without finish_reason`); fresh STATUS reoffered the same slot and the
  retry captured. Six informational advisories, listed under Follow-ups.
- WU2 full check set at `d6106ed`, all exit 0: `scripts/build-gate.sh`, then `./tests/validate.sh` (gate and
  root Go packages ok, parity harness 58 legs with zero deltas in both gate modes, 2482 Python tests OK),
  logs in `~/.cache/s14a-wu2-{build,validate}.log`; the tree stayed clean.
- WU3 C1 (worktree `.worktrees/go-07-s14a-gate-acquire`, branch
  `change/go-07-s14a-gate-acquire`, base `5933bb6`): `internal/gatebinary` ports
  `lib/_internal/gate_binary.py` in full — `CLIVersion`, `DetectPlatform`/`DetectHostPlatform`,
  `SupportedPlatforms`, `AssetName`, `CacheBinPath`, the mismatch record, `LoadExpectedDigests`
  (fail-closed on an unreadable/undecodable trust root, the doctor twin's deliberate divergence),
  `Sha256Of`, the atomic verification receipt and its reader, `ResolveVerifiedBinary` (with
  `WORKTREE_GATE_BIN`), `VerifyCachedBinary` (all reasons verbatim, joined with `; `),
  `QuarantineCachedCandidate`, `LocalBuild`, `BinaryVersion`/`Selftest`, `DegradationHint`,
  `AssetURL`, and `Acquire` (never errors; unsupported → cache revalidation → local build → offline
  → missing digest → download, with temp-then-rename and a receipt that deletes the binary when the
  receipt write fails). `gatebinary_test.go` mirrors `tests/test_gate_binary_dist.py` symptom by
  symptom via `httptest` and fixture shell binaries; no test reaches the network. RED was the new
  package failing to build; GREEN is `go test ./internal/gatebinary/ -count=1` exit 0 (19 tests).
  The package has no importer yet: C2 rewires the doctor twins and `materialize_loop.go`, so
  `internal/doctor/**` and `internal/sync/**` are untouched here. Checks: `gofmt -l cmd internal`,
  `go vet ./...` and `git diff --check` all exit 0; `go test ./cmd/... ./internal/... -count=1`
  exits 1 only on pre-existing environment failures (the `python3` on `PATH` is 3.9 without
  `tomllib`, and `internal/projectcache`, `internal/provider`, `internal/sync`, `internal/target`
  pin Python 3.14 / recursion behavior), none in a package that imports `internal/gatebinary`.
  Size, disclosed: 931 lines of implementation plus 758 of tests (1689 authored), above the plan's
  ~1300 soft stop; if the reviewer wants a split, the natural cut is the test table as its own
  follow-up commit, as in WU2.

## Follow-ups
- Review advisories of WU1 (C1, C3, C4 above), to be triaged into the follow-up register.
- Review advisories of WU2 (`review-03a4bff3b005d59b`): R1-python-import-shadowing
  (`materialize_loop.go:715-716`), R2-dup-lock-record (`:323-330`), R2-fake-runtime-error (`:691`),
  R2-truthy-parse-error (`:563-565`), R3-silent-skip-and-swallowed-walk-errors
  (`materialize_loop_test.go:190-200`), R3-untested-failure-paths (`:420-571`). The import-shadowing one is
  the first to look at when WU3 removes the gate delegation.
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
