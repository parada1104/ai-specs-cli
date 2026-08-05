# Proposal: Centralized planning artifacts across submodule worktrees

## Intent

Make the existing plan-before-build contract work in repositories that use the
`worktree-flow` `monorepo-submodules` topology. Code worktrees remain isolated per
implicated subrepository, while the superproject remains the single home for the
change's planning artifacts. The plan-build gate must resolve that central artifact
location before deciding whether a subrepository production edit is permitted.

This is a narrowly scoped compatibility change to the plan-build gate and its
contract documentation. It does not introduce a new planning product surface or
try to orchestrate a cross-repository implementation.

## Current gap

`plan-build-gate.sh` currently walks from the target path to the nearest Git root and
looks for `openspec/changes/*/tasks.md` beneath that root. That is correct for a
standalone repository, but it is the wrong root for a linked worktree owned by a
submodule:

- `worktree-flow` creates code worktrees under the shared superproject directory,
  using names such as `.worktrees/<subrepo>-<slug>`.
- The linked worktree's Git root is the subrepository worktree, not the
  superproject.
- The intended planning folder is centralized at the superproject's
  `openspec/changes/<slug>/`.
- As a result, a plan written in the central folder is invisible to the gate. The
  agent can be blocked while implementing an authorized plan, or may be tempted to
  create duplicate per-subrepository plans to get unstuck.

The same root mismatch affects the gate's current statement that writing planning
artifacts is always allowed: a write aimed at the central superproject artifact
folder from a subrepository worktree is not recognized as a planning write by the
nearest-root calculation.

## Proposed behavior

The runtime-derived artifact root is the superproject root when the target belongs
to an initialized submodule worktree under a resolved `monorepo-submodules`
topology. Otherwise, behavior remains rooted at the current repository root.

The resolver MUST:

1. Normalize the event target and working directory before comparing repository
   paths, including non-existent files and symlinked paths.
2. Identify the Git repository that owns the target, using the existing
   `rev-parse --show-toplevel` behavior as the first step.
3. For an initialized submodule worktree, resolve the containing superproject using
   the existing topology facts and shared-worktree layout rules, rather than
   relying only on `--show-superproject-working-tree` (which is not sufficient from
   linked worktrees).
4. Use the resolved superproject as the planning-artifact root for both the active
   plan lookup and the narrowly scoped planning-write allowance.
5. Retain the current nearest-repository behavior for standalone repositories and
   non-submodule worktrees.

The gate's enforcement contract then becomes:

- A production edit in a subrepository worktree is allowed only when an active
  change folder containing `tasks.md` exists under the resolved central
  `openspec/changes/` directory.
- A production edit remains blocked when the central directory is missing, contains
  only archived changes, or contains no active change folder.
- A write under the resolved central `openspec/changes/**` path is allowed so an
  agent can create or update the plan before any production edit. This allowance is
  path-scoped; it does not allow arbitrary writes to the superproject root.
- Standalone and existing single-repository behavior remains unchanged, including
  the current archive exclusion, production-directory scope, and fail-open handling
  for malformed or out-of-repository hook events.
- The resolver MUST not turn an unresolved or unrelated outside-repository path
  into a broad superproject write allowance. A valid submodule target falls back to
  the safe nearest-repository gate behavior if central-root discovery is
  inconclusive; it does not silently gain production-write access without a plan.

The plan-build recipe remains the owner of this enforcement. `worktree-flow` supplies
the topology and worktree layout contract; this change consumes that contract but
does not duplicate or redesign submodule creation and cleanup.

## Scope

### In scope

- Topology-aware planning-root resolution in
  `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`.
- Recognition of central superproject `openspec/changes/**` writes from an
  implicated subrepository worktree.
- Active-plan lookup at the central root while preserving the existing
  `openspec/changes/*/tasks.md` and archive semantics.
- Explicit contract text and scenarios for standalone repositories,
  subrepository worktrees, missing plans, archived-only plans, and central plan
  writes.
- Focused hook tests using temporary superproject/submodule fixtures, plus recipe
  contract assertions where documentation or brief fragments change.
- User-facing recipe documentation that explains central planning artifacts and the
  boundary between artifact location and code-worktree location.

### Explicit non-goals

- No new `[sdd]` configuration section, decision matrix, or replacement planning
  classifier. Existing plan-build depth and authorization rules remain the source
  of planning behavior.
- No new manifest selector for an artifact root and no reintroduction of removed
  store-selection concepts. The central root is derived from repository topology;
  it is not user-configured in this slice.
- No per-subrepository artifact store, duplicated plan, or synchronization protocol.
  One active change folder remains canonical in the superproject.
- No multi-PR or multi-branch orchestration. The change does not coordinate one
  planning artifact with separate implementation PRs, branch dependencies, merge
  ordering, or cross-repository archive operations.
- No changes to `/worktree-new`, `/worktree-clean`, cleanup enumeration, topology
  detection, or the shared `.worktrees/<subrepo>-<slug>` layout.
- No change to production-directory selection, gate bypass behavior, tracker gates,
  memory behavior, or the pre-merge archive contract.
- No migration or automatic movement of existing subrepository-local planning
  folders. Existing repositories may clean those up separately after adopting the
  centralized convention.

## Affected capabilities and files

| Area | Impact | Role in this proposal |
|---|---|---|
| `plan-build-flow` capability | Modified | Resolve the planning root from repository topology and enforce the same plan gate across submodule worktrees. |
| `worktree-flow` capability | Referenced, unchanged | Existing `monorepo-submodules` rules define the superproject worktree layout and topology facts consumed by the gate. |
| `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` | Modified | Canonical hook implementation: root discovery, central artifact-write allowance, and active-plan lookup. |
| `catalog/recipes/plan-build-flow/skills/plan-build-flow/SKILL.md` | Modified if needed | Explain that code worktrees and centralized planning artifacts can have different repository roots without changing the planning workflow. |
| `catalog/recipes/plan-build-flow/README.md` | Modified | Document the supported cross-repository layout, resolution boundary, and non-goals. |
| `catalog/recipes/plan-build-flow/recipe.toml` | Possibly modified | Add only a brief workflow rule if the resolved-root contract needs to be surfaced to agents; do not add a new configuration field. |
| `openspec/specs/plan-build-flow/spec.md` | Modified | Promote the topology-aware gate requirement and Given/When/Then scenarios to the canonical capability contract. |
| `tests/test_plan_build_gate_hook.py` | Modified | Prove central active-plan lookup, central plan writes, archived-only blocking, and unchanged standalone behavior. |
| `tests/test_plan_build_flow_recipe.py` | Modified if needed | Guard recipe documentation/brief wording and ensure no forbidden configuration surface is introduced. |
| `docs/recipes-catalog.md` | Modified if needed | Keep the catalog description aligned if the plan-build capability summary changes. |
| `ai-specs/recipes/plan-build-flow/**` | Derived only | Regenerated consumer outputs, never hand-edited; no implementation logic belongs here. |

No changes are expected in `openspec/config.yaml`, the repository's removed product
configuration sections, or the worktree-flow implementation files.

## Alternatives considered

### Keep nearest-Git-root lookup

**Rejected.** This is the current behavior and is exactly why a central plan is
invisible from a subrepository worktree. It preserves the bug and encourages
inconsistent local plans.

### Require a plan in every subrepository

**Rejected.** It duplicates the planning contract, creates drift between plans, and
makes one cross-repository change look like several unrelated changes. It also
contradicts the desired centralized artifact convention.

### Add a user-configured planning-root setting

**Rejected for this slice.** A manifest setting would create another root-selection
surface, require validation and migration rules, and reintroduce configuration
concepts that were intentionally removed. The existing topology and worktree
contract already provide enough information to derive the root.

### Make the gate globally permissive for paths outside the subrepository

**Rejected.** It would turn a root-resolution gap into a write escape hatch. Only the
canonical superproject `openspec/changes/**` prefix should receive the additional
allowance; unrelated outside-repository paths retain existing handling.

### Build multi-repository orchestration now

**Deferred.** Coordinating branch creation, task ownership, PRs, archive timing, and
rollback across repositories is a separate product decision. The first slice should
make the existing gate correct before expanding the workflow.

## Risks and mitigations

| Risk | Likelihood | Mitigation |
|---|---:|---|
| Root detection misclassifies a nested or unusual Git layout | Medium | Reuse the existing topology/path facts, canonicalize paths, require an initialized submodule match, and retain safe nearest-root behavior when central discovery is inconclusive. |
| A central artifact write allowance becomes a broad superproject escape | Medium | Compare canonical paths against exactly the resolved `openspec/changes/**` prefix; never allow arbitrary superproject files through this rule. |
| Existing local subrepo plans become ambiguous during rollout | Medium | Central root takes precedence only for recognized submodule worktrees; do not move or delete local folders automatically, and document the one-canonical-plan convention. |
| The hook and agent guidance disagree | Low | Update the hook, canonical spec, README/SKILL/brief together and add recipe contract assertions for the public wording. |
| A future topology change invalidates root assumptions | Low | Keep root derivation coupled to the existing `worktree-flow` topology contract and explicitly defer broader topology support. |
| Central plan exists but is not visible due to permissions or an unavailable path | Low | Preserve the gate's existing deterministic lookup and fail-open behavior only for malformed/unrelated hook input; a recognized submodule production target must not be allowed merely because the central plan lookup was skipped. |

## Rollout and compatibility

1. Ship the hook and contract changes as a backward-compatible plan-build-flow
   update. No manifest migration is required because the resolver is derived from
   repository topology.
2. Standalone repositories and non-submodule worktrees keep the current root and
   gate behavior byte-for-byte where practical.
3. In a recognized `monorepo-submodules` project, existing central active plans are
   immediately eligible; no plan copying or data migration occurs.
4. The first synced version should emit the same consumer artifacts as today plus
   the clarified root behavior. Materialized files remain derived from catalog
   sources.
5. Rollback is a source revert: restore nearest-root lookup and the previous
   documentation/tests. There is no persisted state, branch migration, or artifact
   conversion to undo.
6. Adoption guidance should tell teams to create one plan under the superproject's
   `openspec/changes/<slug>/` and then implement code in the implicated
   subrepository worktrees. Teams that do not use submodules see no behavior change.

## Acceptance-oriented outcomes

The next spec phase should turn these outcomes into executable scenarios:

- **Standalone compatibility:** Given a standalone repository with no active plan, a
  production write is blocked exactly as before; an active root plan allows it.
- **Central active plan gates subrepo code:** Given an initialized submodule and a
  linked worktree under the shared superproject worktree directory, when the
  superproject contains `openspec/changes/demo/tasks.md`, a production write in the
  subrepository worktree is allowed.
- **Central absence still blocks:** Given the same submodule worktree with no active
  central `tasks.md`, a production write is blocked and the diagnostic points to the
  central planning location.
- **Archived plans do not count:** Given only
  `openspec/changes/archive/demo/tasks.md` at the superproject root, a subrepository
  production write remains blocked.
- **Central plan creation is allowed:** Given no active plan, a write to the
  superproject's `openspec/changes/demo/tasks.md` from the subrepository worktree is
  allowed, while a write to another superproject production path is not newly
  allowed by this rule.
- **Root resolution is topology-aware:** A linked submodule worktree is associated
  with the correct superproject even when `git rev-parse --show-superproject-working-tree`
  is empty or insufficient; similarly named submodules must not resolve to the
  wrong parent.
- **No configuration regression:** The recipe introduces no `[sdd]` section, old
  decision matrix, per-subrepository store setting, or user-configured artifact-root
  field.
- **No orchestration expansion:** The change does not create additional worktrees,
  PRs, branches, archive operations, or cross-repository synchronization as a side
  effect of planning or gate evaluation.

## Proposal question round

These questions are intended to improve the proposal by exposing business rules,
workflow implications, edge cases, and product tradeoffs. The first slice can
proceed with the recommended assumptions below; the user may answer, skip, correct
the framing, or request a second question round.

1. **Canonical location:** Should every recognized `monorepo-submodules` project use
   the superproject `openspec/changes/` as the one canonical planning location,
   without an opt-out in this slice? **Recommendation:** yes, to avoid a second
   configuration surface.
2. **Existing local folders:** If a subrepository already has a local
   `openspec/changes/` folder, should the central plan take precedence while the
   local folder is left untouched? **Recommendation:** yes; do not migrate or delete
   user data automatically.
3. **Gate granularity:** Should any active central `tasks.md` satisfy the existing
   gate, or must the gate match a plan to the implicated subrepository? **Recommendation:**
   retain the current any-active-change semantics and defer `gate_scope`/ownership
   matching; the first slice is root resolution, not change-to-repository routing.
4. **Allowed central writes:** Should the additional allowance cover all paths under
   central `openspec/changes/**`, including archive preparation, or only creation and
   updates to active folders? **Recommendation:** preserve the existing planning
   path allowance for the full `openspec/changes/**` subtree while keeping arbitrary
   superproject writes gated.

## Next phase

Proceed to the **spec** phase. Convert the proposed root-resolution and gate
scenarios into the canonical `openspec/specs/plan-build-flow` delta, preserving the
explicit non-goals and the standalone compatibility contract before any design or
implementation work begins.
