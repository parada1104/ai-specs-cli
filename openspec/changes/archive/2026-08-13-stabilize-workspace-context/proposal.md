# Proposal: Stabilize explicit workspace context across runtimes

- **Change slug**: `stabilize-workspace-context`
- **Depth**: full
- **Worktree**: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/stabilize-workspace-context`
- **Baseline**: `development` at `35ce32a`

## Problem

Runtime adapters and the worktree gate currently use several different meanings of
"current directory". The distinction is not consistently preserved at the process
boundary:

- Claude and Cursor use their project-directory variables when wiring the materialized
  hook, which is the existing correct pattern for those runtimes.
- OpenCode emits `directory ?? process.cwd()` in the event, but its generated plugin
  invokes a relative `SCRIPT` without setting the child process cwd. An explicit
  OpenCode directory can therefore describe one workspace while the hook process and
  its relative assets are resolved from another directory.
- Pi and OMP also use a relative `SCRIPT` and hard-code `process.cwd()` for event cwd.
  Their event limitation is real, but their launcher asset path has the same process-cwd
  defect as OpenCode.
- The shell launcher resolves the project-local Go binary and the legacy fallback from
  process `$PWD`. A hook started from an unrelated directory can miss assets that are
  present beside the materialized launcher.
- Go `event.go` accepts an absolute existing event cwd without trimming outer whitespace,
  while the legacy Bash implementation trims it. The two implementations can therefore
  classify the same event differently.

These inconsistencies make explicit CLI/worktree target propagation harder to trust and
can cause a valid gate implementation to be skipped before the gate evaluates the target.
The existing target propagation behavior covered by
`tests/test_worktree_root_propagation.py` must remain intact.

## Desired outcomes

1. The launcher derives its installation root from `BASH_SOURCE[0]`, handles relative and
   symlinked invocation safely, and resolves project-local and legacy assets through the
   `hooks/../bin` layout rather than process `$PWD`.
2. OpenCode normalizes `directory` by outer trim, requires a string absolute existing
   directory, uses one normalized value for event cwd and child `spawnSync` cwd, and
   falls back to process cwd for both when invalid.
3. OpenCode, Pi, and OMP resolve their materialized launcher from a runtime-supported
   absolute module location such as `import.meta.url` or an equivalent mechanism. No
   generated adapter uses a relative `SCRIPT` or a machine-specific sync-time absolute
   path.
4. Go and legacy Bash apply the same outer-whitespace-only cwd normalization and fallback
   contract without changing internal path bytes, gate policy, or fail-open behavior.
5. Tests prove the process boundaries and decision outcomes, not only generated source
   text. Runtime documentation describes the final contract, including the stale
   `docs/runtime-hooks.md:133-138` section.

## Scope

### In scope

- The source generator paths in `lib/_internal/hooks-render.py` for OpenCode, Pi, and
  OMP launcher-path stabilization, OpenCode explicit-directory normalization, and
  preservation of the existing Claude/Cursor project-root wiring.
- The asset-resolution and legacy-fallback boundary in
  `catalog/recipes/worktree-flow/hooks/worktree-gate.sh`, including
  `BASH_SOURCE[0]`, relative invocation, symlink behavior, and the recipe's
  `hooks/../bin` layout.
- The event cwd normalization contract in
  `catalog/recipes/worktree-flow/gate/event.go`, with direct `ParseEvent` tests and
  observable parity against the legacy Bash implementation.
- Deterministic process-boundary tests for generated OpenCode output, launcher-root
  resolution tests, and Bash fixtures whose normalized cwd changes the gate decision.
- The fixed two-slice review structure: adapter/generated-output work first, then
  launcher plus Go/Bash parity and documentation.
- Runtime hook documentation, including the explicit update to
  `docs/runtime-hooks.md:133-138`, and the relevant worktree-flow recipe documentation.

### Out of scope

- Any production, generated runtime, test, recipe, or configuration edit during this
  planning stage. This package is the only output of the current operation.
- Cleanup-root discovery or worktree cleanup mechanics.
- Changes to gate policy, protected branches, repository topology decisions, exit codes,
  or the existing fail-open policy.
- Adding a Cursor pre-file-write hook; Cursor has no such native hook.
- Extending OpenCode coverage to subagent or MCP tool calls; its pre-tool hook does not
  cover those calls.
- Inventing an authoritative workspace root for Pi or OMP. Their event context remains
  limited to the current agent process even after launcher-path stabilization.
- Replacing the launcher with a new materialized filename or changing the existing
  explicit CLI/worktree target propagation contract.

## Approved decisions

| ID | Decision | Implementation consequence |
|----|----------|----------------------------|
| D1 | Keep three contexts: event cwd, installation root, and process cwd | Context values are named and tested separately; installation root never becomes event cwd |
| D2 | Normalize OpenCode `directory` by outer trim, require string plus absolute existing directory, and use one result for event and child cwd | Invalid input uses process cwd for both; child errors and throws remain fail-open |
| D3 | Derive generated ESM adapter asset paths from module location | OpenCode, Pi, and OMP use `import.meta.url` or an equivalent runtime-supported absolute mechanism; no relative `SCRIPT` or sync-time machine path |
| D4 | Derive Bash launcher root from `BASH_SOURCE[0]` | Relative invocation is anchored once to the invocation path, symlinks resolve to the target installation, and `hooks/../bin` is deterministic |
| D5 | Preserve outer-whitespace-only Go/Bash normalization | Trim boundaries only; preserve internal path bytes and all existing invalid-context fallbacks |
| D6 | Use two review slices with a fixed dependency | Slice 1 establishes adapter/process contracts; Slice 2 consumes them for launcher, Go/Bash parity, and docs |
| D7 | Require process-boundary and decision-differentiating proof | Textual source assertions are insufficient; runtime verification is an implementation gate, not a new product decision |

## Review slices

| Slice | Contents | Dependency and exit condition |
|-------|----------|-------------------------------|
| 1. Adapters and process boundaries | OpenCode normalization and child cwd, module-location asset paths for OpenCode/Pi/OMP, generated-output tests, Node process-boundary harness, preserved Claude/Cursor/Pi/OMP limitations | First slice. It establishes the generated invocation contract and exits only when the deterministic process tests are green |
| 2. Launcher, Go/Bash parity, and docs | `BASH_SOURCE[0]` root resolution, relative/symlink/layout fixtures, direct `ParseEvent` tests, decision-differentiating Bash parity fixtures, runtime and recipe docs | Depends on Slice 1's invocation contract. It exits only when launcher, parity, docs, and full validation are green |

## Risks

| Risk | Impact | Mitigation |
|------|--------|------------|
| Event cwd and launcher installation root are conflated | The gate evaluates the wrong repository or cannot find its implementation | Keep the three-context model normative and assert event and asset contexts independently |
| OpenCode runtime path or cwd behavior is assumed rather than proven | Explicit-directory execution remains relative to the wrong process | Use a deterministic Node process-boundary harness with a supported `spawnSync` test double; do not accept source-text-only proof |
| Symlink or relative launcher invocation selects the wrong recipe root | Project-local binaries or legacy fallback are missed or an unrelated asset is used | Resolve `BASH_SOURCE[0]` to a physical launcher path, test relative and symlinked invocation, and fail open when installation-root resolution is unusable |
| Go and Bash normalization diverge again | The implementation of record and rollback path disagree | Direct Go `ParseEvent` table tests plus Bash fixtures where trimming changes block versus allow outcome |
| Fail-open behavior is changed while fixing context | A malformed event or missing asset can wedge an editor or alter gate policy | Preserve exit `0` on resolution ambiguity, invalid cwd, child errors, and child throws; assert stderr and decisions separately |
| Runtime limitations are overstated as support | Users may believe Cursor, OpenCode subagents/MCP, Pi, or OMP are fully covered | Keep the limitations normative and test that Pi/OMP still emit `process.cwd()` without workspace-authority claims |
| The cross-cutting implementation is reviewed as one oversized change | Review defects become harder to isolate | Use the fixed two-slice dependency and map every acceptance criterion to one slice |

## Tracker

- **card_id**: `6a7cadeacf7234a7093bceb5`
- **shortLink**: `zHPy3GhC`
- **url**: `https://trello.com/c/zHPy3GhC/71-story-stabilize-explicit-workspace-context-across-runtimes`
- **list**: `In Progress`
- **epic**: `https://trello.com/c/qxP4SSnS/67-epic-next-release-open-compatibility-runtime-stability`

## Approval boundary

The product and architecture decisions in this package are resolved recommendations
approved for implementation. The remaining gates are implementation proof and validation:
the runtime-supported module-location mechanism must work in the Node process harness,
the Bash root algorithm must pass relative and symlinked invocation fixtures, Go and Bash
must produce equivalent decisions, and the documented full validation command must pass.
Failure at one of these gates requires correcting the implementation, not reopening the
workspace-context product decision.
