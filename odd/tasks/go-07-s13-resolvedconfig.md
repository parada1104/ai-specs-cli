# Go 07 S13 PR3 — Resolved config

## Objective
Give the root single binary a native resolved-config authority that shares ONE decision core with the
gate binary, and retire the duplicate logic: extract the gate's `--plan-resolved-config` decision core
into the importable shared package and add the root-side authority that calls it in-process.

Nothing about the gate's JSON command contract changes: the extraction is behavior-preserving and its
oracle is the existing gate suite plus the resolved-config bridge suite.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no card could be
  created or linked for this slice.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed
  without recording a decision. `tracker.none` is presentation and evidence only and is never written
  by the host, so none was created here.
- Close-out follow-up: link the S13 card in this section once the tracker is reachable.

## Scope
- Base: `64f9217` on `epic/go-single-binary` (S13 PR1, PR2a and PR2b merged).
- Worktree: `.worktrees/go-07-s13-resolvedconfig`. Branch: `change/go-07-s13-resolvedconfig`.
- Move the resolved-config decision core out of
  `catalog/recipes/worktree-flow/gate/resolved_config.go` into
  `catalog/recipes/worktree-flow/gate/shared/resolvedconfig.go`, leaving the main package as a thin
  wrapper with the historical names aliased so the existing gate tests compile unchanged.
- Add the root-side native authority `internal/sync/resolvedconfig.go` on top of the shared core.
- Regenerate only the four canonical gate checksum entries that the extraction requires.

## In-scope units
### Gate core (`catalog/recipes/worktree-flow/gate/resolved_config.go`, 382 lines)
| Symbol group | Lines | Class | Destination |
| --- | --- | --- | --- |
| topology constants | 25-34 | pure | `shared/resolvedconfig.go` |
| `manifestEntry/Payload/Topology/Config` types | 36-70 | pure | `shared/resolvedconfig.go` |
| `projectRecipes/projectEnabled/projectBindings` | 72-127 | pure | `shared/resolvedconfig.go` |
| `topologyConfigured/legacyRepoTopology/nonEmptyString` | 128-180 | pure | `shared/resolvedconfig.go` |
| `detectSubmodules` | 181-215 | pure **with a git-fact seam** | `shared/resolvedconfig.go` |
| `resolveRepoTopology` | 216-238 | pure | `shared/resolvedconfig.go` |
| `buildResolvedConfig` | 239-256 | pure (uses `shared.RealPath`) | `shared/resolvedconfig.go` |
| `manifestEnvelope/decodeManifestPayload` | 278-325 | pure decode | `shared/resolvedconfig.go` |
| `resolvedConfigReader` (embedded Python) | 257-277 | I/O | **stays in main** |
| `runResolvedConfigParser` | 326-356 | I/O (needs the main ledger-reconcile helpers) | **stays in main** |
| `runPlanResolvedConfig` | 357-382 | flag wrapper | **stays in main** |

Moved total: ~295 lines. `detectSubmodules` calls the gate's `git()` and `isFile`, which cannot cross
the package boundary as an unexported package variable: forward the fact as a function value, the seam
shape already established by `shared/gitcommon.go` (`GitCommonDirWith(root, fact)`) and by the S12.3-A
refresh seam.

### Python seam (`lib/_internal/recipe-materialize.py`), unchanged by this delivery
`GO_RESOLVED_CONFIG_BRIDGE_FALLBACK` :3656 (timeout :3657, warn :3662-3667),
`_is_resolved_config_envelope` :3680-3698, `go_resolved_config` :3701-3761 (flag
`--plan-resolved-config --project <root>` :3736, **no stdin**; the bridge is read-only and never
acquires a binary, :3653-3655), the fail-open body `_python_build_resolved_config` :3763-3832 (the
differential oracle) and `build_resolved_config` :3835-3842. Callers at :3901 (no-enabled branch),
:4121 (after the recipe loop; the gate's bindings value is overridden by auto-bound
`resolved_bindings` at :4125) and :4155 (`build_resolved_config_only`).

### Root-side acquisition (all native readers already exist)
- manifest: `internal/config.LoadManifest` (`config.go:47`) with `readRecipes` (:449, ordered via
  `recipes.Keys()`) and `readBindings` (:483);
- topology: `internal/target/topology.go` — `detectSubmodules` (:105), `resolveRepoTopology` (:133),
  `legacyRecipeTopology` (:193), `topologyConfig` (:220), `ProjectRepoTopology` (:257);
- document-ordered TOML: `internal/toml` (`toml.go:24-25`);
- project root canonicalization: `shared.RealPath`.
No new reader is expected; the root authority is a thin assembly (~110 lines) mirroring the shape of
`internal/sync/bindings.go`.

## Constraints
- Budget: **1400 additions plus deletions** (measured estimate 700-900). If the work is heading past
  1400, STOP and report the measured number with the remaining work instead of exceeding it.
- Never remove or weaken tests to fit the budget. The extraction is a behavior-preserving refactor
  whose checks are the existing gate tests plus the resolved-config bridge suite, so no new RED cycle
  is claimed for it.
- The gate JSON contract is unchanged: same stdout envelope fields and order, same error strings and
  exit codes.
- `--plan-resolved-config` is read-only: this delivery must not add any write.
- No production Python change, no CLI wiring (S14 owns the wiring), no dependency change.
- If the moved core turns out to be shared with another already-merged domain (as `orderedMap` was in
  PR2a), STOP and report the exact consumers and lines instead of editing outside the surfaces or
  duplicating a container.
- Do not modify `development` and do not publish a release.
- Parent owns the commit, push, PR, merge and cleanup.

## Tasks
- [ ] T1: Enumerate every symbol `gate/resolved_config_test.go` and any other gate file references, so
      the export surface is exactly what those tests need and they compile unchanged.
- [ ] T2: Move the pure core into `gate/shared/resolvedconfig.go` with the git-fact seam forwarded as a
      function value; leave `gate/resolved_config.go` as the wrapper plus the parser and the flag
      wrapper.
- [ ] T3: Add `internal/sync/resolvedconfig.go`: root boundary types + `BuildResolvedConfig` over the
      shared core, assembling the payload from `internal/config` and `internal/target` natively.
- [ ] T4: Add `internal/sync/resolvedconfig_test.go` and `internal/sync/testdata/resolvedconfig_ref.py`:
      a real differential against `_python_build_resolved_config` (importing the real body) plus the
      topology label/source and sorted-submodule contracts.
- [ ] T5: Regenerate the four `SHA256SUMS` digest lines with go1.24.13 and verify them with
      `scripts/verify-gate-sums.sh`.

## Acceptance criteria
- The gate stdout envelope fields, order, error strings and exit codes are unchanged.
- The moved bodies are textually identical apart from package, visibility and seam changes.
- Existing gate tests pass against the shared implementation without edits (or with mechanical-only
  edits, reported explicitly).
- The root authority produces the same envelope as the Python authority for the covered scenarios,
  including the topology source label passthrough and the sorted submodule list.
- The four trust-root digests match a rebuild through `scripts/build-gate.sh` under go1.24.13.
- The complete authored candidate stays within the 1400-line budget.

## Checks
- Gate: `go -C catalog/recipes/worktree-flow/gate test ./... -count=1` and `go vet ./...` there.
- Bridge: `python3 -m unittest tests.test_resolved_config_bridge` (build the gate first if the module
  reports skips; the classes skip when `dist/worktree-gate-current` is absent).
- Root: `go test ./cmd/... ./internal/... -count=1`, `gofmt -l`, `git diff --check`.
- Trust root: `scripts/build-gate.sh` plus `scripts/verify-gate-sums.sh`.
- Full: `./tests/validate.sh` with durable logs and captured exit evidence (run by the parent, with the
  gate binary built so the bridge classes are not skipped).
- Environment: the real toolchains with the Python 3.14.7 oracle prefixed on PATH
  (`~/.local/share/mise/installs/{go/1.24.13,python/3.14.7}/bin`), never the mise shims.
- Native review uses the exact committed slice base `64f9217`, not the whole epic.

## Progress
- Slice prepared: worktree and branch created at base `64f9217`; scope mapped read-only with measured
  line counts.
- Implemented by a delegated writer: the pure core moved to `gate/shared/resolvedconfig.go` (327) with
  the git fact forwarded as a function value (`DetectSubmodulesWith`/`ResolveRepoTopologyWith`, the
  `shared/gitcommon.go` seam shape); `gate/resolved_config.go` keeps the embedded TOML reader, the
  parser and the flag wrapper plus the historical aliases, and `resolved_config_test.go` needed **zero**
  edits. `internal/sync/resolvedconfig.go` (111) adds the root authority; the differential
  `internal/sync/resolvedconfig_test.go` (423) and `testdata/resolvedconfig_ref.py` (49) import the real
  Python bodies. Measured 1226 authored lines against the 1400 budget.
- Two deliberate deviations from the brief, both justified by the oracle: the authority reads the raw
  `[recipes.*]` and `[[bindings]]` tables instead of `config.readRecipes`/`readBindings`, because those
  normalize each recipe to `{enabled, version, config}` and drop the flat keys the Python loop keeps;
  and the envelope's topology comes from the SHARED core rather than `internal/target`, so there is no
  second topology implementation (the shared core is pinned against Python's `util.project_repo_topology`
  on a real superproject).
- First independent verification: PASS on the static claims and the suite, and it found four real items.
  One was **introduced here**: the bindings acquisition only accepted `[]*toml.Table`, so a legal inline
  `bindings = [{capability = "x", recipe = "y"}]` was silently dropped while Python returns
  `{"x":"y"}`. Two were **pre-existing in the moved core**: an empty recipe table (`[recipes.foo]`) is
  dropped where Python keeps `{"foo":{}}`, and a flat key colliding with the same key inside `[config]`
  resolves by Go map iteration order (the verifier observed 12 runs returning `2` and one returning `1`)
  where Python is always the document-ordered value. The fourth was dead code.
- Repair round: the acquisition now accepts every shape Python accepts (array of tables, inline arrays
  with mixed items, document order preserved, non-string pairs still rejected) with a captured RED and a
  green differential; the two pre-existing divergences are pinned on BOTH sides by explicit,
  self-describing tests that do not claim parity (`TestNativeResolvedConfigEmptyRecipeTableDivergence`,
  `TestNativeResolvedConfigFlatConfigCollisionNondeterminism`, the latter asserting only that the native
  value is one of the two observable ones so it cannot flake); and the three unreferenced gate wrappers
  (`legacyRepoTopology`, `nonEmptyString`, `detectSubmodules`) were removed.
- Trust root: removing the dead wrappers changes the compiled gate, so the four digests were regenerated
  with go1.24.13 and the provenance paragraph was rewritten to describe this delivery accurately.
- Re-verification: **PASS** on all six re-verified claims, including 13 independently authored binding
  shapes (all matching the oracle), a 20-iteration flake check on the nondeterminism test, byte-unchanged
  gate tests, an independent gate rebuild reproducing all four digests, and the 1226 authored lines.
  Residual: two sibling shapes of the already-pinned non-string coercion divergence (an integer `recipe`
  and a boolean `capability`) are the same class but are not separately pinned.
- Full validation with the gate binary present: `./tests/validate.sh` exit 0 with
  `Ran 2482 tests in 1609.815s` then `OK`, and zero `FAIL:`/`ERROR:` lines — the same count the
  previous slices reported, so the binary-backed bridge classes really ran. Evidence:
  `~/.cache/s13-pr3-validate.log` and `~/.cache/s13-pr3-validate.exit`.

## Follow-ups (registered here, not fixed in this delivery)
- **Empty recipe table** (`[recipes.foo]` with no keys): the shared core drops the key where Python
  keeps `recipes:{"foo":{}}`. Pre-existing in the moved code; the shared file's `ProjectRecipes` doc
  comment wrongly claims to mirror Python for that shape. Pinned on both sides by
  `TestNativeResolvedConfigEmptyRecipeTableDivergence`; a gate-core fix needs its own slice and a fresh
  trust-root regeneration.
- **Flat-versus-`[config]` key collision is nondeterministic** in the shared core: it resolves by Go map
  iteration order (observed twelve `2` and one `1` over thirteen runs) where Python is always the
  document-ordered value. Pinned non-flakily by
  `TestNativeResolvedConfigFlatConfigCollisionNondeterminism`; a gate-core defect for a follow-up slice.
- **Non-string binding coercion**: the shared str-keyed contract drops a non-string `capability` or
  `recipe` pair while the Python fail-open authority coerces it. The class is pinned; an integer `recipe`
  and a boolean `capability` are the same class and are not separately pinned.
- **Spaced submodule paths**: the shared reader splits with `strings.Fields` while Python uses
  `split(None, 1)`, so a path containing a space could be listed by one side and not the other. Untested
  and not exercised.
- **Unpinned envelope internals**: `reflect.DeepEqual` on decoded maps cannot see JSON key order inside
  `recipes`/`bindings`, so that order is correct by construction but unpinned (Go marshals maps sorted
  and Python preserves document order; semantically equivalent). The datetime boundary is also unpinned:
  the root rejects at TOML parse while Python fails at serialization, so both refuse to emit an envelope
  by different mechanisms.

## Rollback
Revert this PR's gate extraction, its four checksum entries, the root authority, the tests and this
document together. PR4 (hooks + recipe-mcp) must not integrate before this shared API exists.
