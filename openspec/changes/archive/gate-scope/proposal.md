# Proposal: Topology-aware worktree gate scope

## Tracker

- **Card:** [Trello #65 — follow-up worktree-flow gate scope for cross-repo superrepos](https://trello.com/c/LIoDU2xL/65-follow-up-worktree-flow-gate-scope-for-cross-repo-superrepos)
- **Board:** `69ec097f13e2d38ecd89a557`
- **Change:** `gate-scope`

## Intent

Make the `worktree-flow` write gate topology-aware for repositories that use
initialized Git submodules as a cross-repository superrepo. Agents should be able
to create or update the canonical superproject planning artifacts without first
creating an unnecessary superproject worktree, while protected branches in the
subrepositories remain protected.

This proposal adds a narrowly scoped `gate_scope` policy to the existing
`gate_mode` and `repo_topology` dimensions. It does not make the gate globally
permissive: repository ownership must be proven from Git facts, protected branch
matching remains exact, linked worktrees remain allowed as they are today, and
only the canonical superproject planning subtree receives the additional
allowance.

No production code, generated consumer output, tests, or recipe implementation is
part of this proposal phase.

## Current gap

The worktree gate currently resolves a candidate path to its nearest Git
repository and then blocks writes from that repository's primary worktree when
its symbolic branch is one of the configured protected branches (`main` or
`development` by default). A linked worktree is allowed.

That repository-local rule is safe for standalone repositories, but it cannot
represent the supported cross-repository layout:

- The superproject has its own primary checkout and protected branch.
- Each initialized submodule is a separate Git repository with its own primary
  checkout and protected branch.
- Feature worktrees owned by submodules live under the shared superproject
  `.worktrees/<subrepo>-<slug>` layout and are already writable.
- The canonical OpenSpec change folder belongs to the superproject, even when an
  agent is implementing code from a submodule worktree.

When an agent writes a central `openspec/changes/**` artifact from a submodule
worktree, the candidate path resolves to the superproject primary checkout. The
current gate sees a protected superproject branch and blocks the write. The
plan-build gate already treats the central planning tree as the canonical plan
location, so the two gates disagree about whether the same planning write is
permitted.

The opposite failure is unsafe: a broad “superrepo” bypass could allow production
writes to the superproject, and a topology heuristic based only on a basename,
cwd, or `--show-superproject-working-tree` could misclassify a submodule or an
unrelated path. The follow-up must fix the central planning case without
weakening subrepository protection or creating an outside-repository escape.

## Proposed decision

### Final vocabulary recommendation

Add the following independent field under `[recipes.worktree-flow.config]`:

```toml
gate_scope = "auto"
```

The accepted values are exactly:

| Value | Meaning |
|---|---|
| `auto` | Default. Resolve the candidate's owning Git repository and classify it as the superproject or an initialized subrepository from verified topology facts. Apply the topology-aware policy for that owner. |
| `superrepo` | Explicitly select superproject scope for the policy. A proven superproject primary checkout remains protected on exact protected branches; only the explicit central planning allowlist below receives an exception. This value never grants a subrepository bypass. |
| `subrepo` | Explicitly select initialized-submodule scope for the policy. A proven subrepository primary checkout remains protected on exact protected branches. This value never grants a broad superproject bypass. |

`gate_scope` is deliberately separate from the existing dimensions:

- `gate_mode` controls whether the gate is `always`, `ask`, or `off`.
- `repo_topology` describes worktree creation/cleanup topology and remains
  `auto | standalone | monorepo-apps | monorepo-submodules`.
- `gate_scope` controls which repository ownership facts the gate evaluates and
  which narrowly defined superproject planning exception may apply.

`auto` is the only default. No aliases or alternate spellings should be added in
this change. Invalid values MUST be rejected during sync using the same
non-zero-exit and allowed-enum diagnostic shape as `gate_mode` and
`repo_topology`.

### Effective policy

The implementation should apply the following policy in order:

1. Resolve `gate_mode` first. `off` continues to disable the gate entirely; the
   new scope policy MUST NOT become a second bypass mechanism.
2. Normalize the event cwd and every candidate target path using component-aware,
   symlink-safe canonicalization. Nonexistent destination components must remain
   resolvable by canonicalizing existing ancestors.
3. Resolve the Git repository that owns the candidate using the nearest existing
   ancestor and `git rev-parse`. A linked worktree (`git_dir != common_dir`)
   remains allowed before branch protection, preserving current behavior.
4. For a primary checkout in a topology that may contain submodules, classify
   ownership only after all of these facts agree:
   - the candidate's Git common directory identifies the repository;
   - the containing superproject has a real `.git` directory and `.gitmodules`;
   - the repository is registered by the superproject's `.gitmodules` path;
   - the submodule path is initialized (`git submodule status` is not empty and
     does not have the `-` prefix); and
   - the relationship is unique and component-contained.

   `git rev-parse --show-superproject-working-tree` MAY corroborate a primary
   checkout, but MUST NOT be the sole signal because it is empty from linked
   submodule worktrees. Nested, ambiguous, symlink-escaping, or otherwise
   unproven relationships MUST NOT produce a scope-based allow.
5. Apply exact protected branch matching against `WORKTREE_GATE_PROTECTED` (or
   its default `main development`). Scope selection MUST NOT change branch names,
   add globbing, or use substring matching.
6. In a recognized `monorepo-submodules` relationship, allow a candidate in the
   superproject's exact canonical planning subtree:

   ```text
   <superrepo>/openspec/changes/**
   ```

   The comparison MUST be component-aware and use the canonical superproject
   path. This includes active change folders and the existing archive subtree,
   because planning and archive preparation are already the plan-build artifact
   contract. It does not include sibling paths such as `<superrepo>/src`,
   `<superrepo>/.gitmodules`, arbitrary root files, or another repository that
   merely has a similar basename.
7. All other writes to a protected primary checkout remain blocked. A production
   write in a subrepository primary checkout remains blocked even when a central
   active plan exists; the plan-build gate owns plan authorization and does not
   authorize production writes by itself.

The central planning exception is therefore a path-scoped exception, not a
superproject-wide bypass. It is also a separate concern from plan authorization:
`worktree-gate` answers whether the protected-worktree policy permits the path,
while `plan-build-gate` continues to require an active plan for production paths.

### Scope value behavior

The next spec phase should encode this matrix as executable scenarios:

| Candidate and topology | `auto` | `superrepo` | `subrepo` |
|---|---|---|---|
| Standalone primary checkout on a protected branch | Preserve current block | Preserve current block; no superrepo proof, so no exception | Preserve current block; no subrepo proof, so no exception |
| `monorepo-apps` primary checkout on a protected branch | Preserve current block | Preserve current block; naming-only topology does not prove a superrepo relationship | Preserve current block; no initialized-submodule proof |
| Proven superproject primary checkout, non-planning path | Block on exact protected branch | Block on exact protected branch | Do not create an allow; retain the conservative existing block |
| Proven superproject primary checkout, `<superrepo>/openspec/changes/**` | Allow after independent proof of the canonical superrepo artifact path | Allow after independent proof of the canonical superrepo artifact path | Allow after independent proof of the canonical superrepo artifact path; `subrepo` never broadens this exception |
| Proven initialized subrepository primary checkout on a protected branch | Block | Block; a superrepo selection cannot bypass subrepo protection | Block |
| Proven linked feature worktree owned by a subrepository | Allow as today | Allow as today | Allow as today |
| Uninitialized, unrelated, ambiguous, or unresolved relationship | No scope-based bypass; retain the conservative nearest-repository decision | No scope-based bypass | No scope-based bypass |

The central planning row is intentionally limited to the exact canonical subtree.
If product requirements later need protected-branch writes to `.gitmodules`,
root configuration, release metadata, or other superproject structure, those
paths MUST be named in a separate allowlist decision rather than inferred from
`superrepo` or added as a broad root exception.

### Runtime stamp and override precedence

`lib/_internal/recipe-materialize.py` should stamp the validated effective value
into the distributed hook, using a dedicated placeholder such as
`__WORKTREE_GATE_SCOPE__`. The hook MUST remain self-contained and MUST NOT read
a consumer manifest or Python project internals at runtime.

For parity with `gate_mode`, the proposal recommends an optional
`WORKTREE_GATE_SCOPE` environment override with this explicit precedence:

1. A valid `WORKTREE_GATE_SCOPE` override wins over the stamped value.
2. An invalid environment override emits a warning and falls back to the stamped
   value; it MUST NOT silently select a different scope.
3. A valid stamped value is used when no valid override is present.
4. A missing or invalid stamp warns and falls back to `auto`, whose topology
   proof is still required before any central planning exception.

A scope override MUST NOT override `gate_mode=off` or bypass branch matching for
subrepository primary checkouts.

## Goals

- Add one independently validated `gate_scope` enum with a safe `auto` default.
- Permit canonical superproject planning writes required by the existing
  central-artifact plan-build contract, without forcing a superproject worktree.
- Preserve the existing linked-worktree allowance.
- Preserve exact, configurable protected branch matching.
- Keep protected subrepository primary checkouts gated on `main`, `development`,
  or any exact names configured through `WORKTREE_GATE_PROTECTED`.
- Derive ownership from canonical Git topology facts rather than
  `project.subrepos`, cwd-only inference, basename-only heuristics, or an
  uncorroborated `--show-superproject-working-tree` result.
- Make malformed input, outside-repository paths, symlink escapes, unresolved
  topology, and ambiguous relationships fail safe: no new production-write
  allowance is granted.
- Keep `plan-build-flow` as the owner of active-plan authorization and central
  artifact semantics.
- Keep generated consumer hooks and source recipe contracts aligned, including a
  non-destructive refresh path for existing consumer overrides.

## Non-goals

- No production implementation in this proposal phase.
- No change to `gate_mode` values or its `off`/`ask` behavior.
- No removal, renaming, or reinterpretation of `repo_topology`.
- No broad superproject-main-worktree bypass.
- No authorization for subrepository production paths merely because a central
  `tasks.md` exists.
- No change to the protected branch list, its environment variable, or exact
  matching semantics.
- No change to linked feature worktree behavior; linked worktrees remain allowed.
- No new user-configured artifact root, per-subrepository plan store, plan copy,
  synchronization protocol, or change-to-repository ownership matcher.
- No changes to worktree creation, cleanup enumeration, shared `.worktrees` path
  layout, nested/recursive submodule support, or submodule initialization.
- No use of `project.subrepos` as runtime topology truth.
- No PR, branch, worktree, archive, or cross-repository orchestration side
  effects.
- No automatic migration, deletion, or movement of existing local planning
  folders.
- No silent overwrite of consumer-customized materialized hooks.
- No extension of the central exception to `.gitmodules`, root configuration,
  source directories, release files, or arbitrary paths outside
  `<superrepo>/openspec/changes/**`.

## Affected areas

| Area | Expected change | Responsibility |
|---|---|---|
| `catalog/recipes/worktree-flow/recipe.toml` | Modified | Declare `gate_scope`, its exact enum/default/help text, and the hook stamp contract. |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Modified | Resolve scope from the stamp/override, prove repository ownership, preserve linked-worktree and branch behavior, and apply the exact central planning allowlist. Path and shell modes must share the same decision function. |
| `lib/_internal/recipe-materialize.py` | Modified | Substitute the validated effective `gate_scope` into the distributed hook using a dedicated placeholder. |
| `catalog/recipes/worktree-flow/README.md` | Modified | Explain the orthogonal configuration dimensions, scope matrix, central planning boundary, and refresh guidance. |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | Modified if needed | Tell agents that superrepo planning artifacts and subrepo code worktrees have different ownership, and require a which-repository check before write-capable delegation. |
| `openspec/specs/worktree-flow/spec.md` | Modified in the next spec phase | Add Given/When/Then requirements for enum validation, stamping, topology proof, exact branch matching, central planning paths, and fail-safe cases. |
| `tests/test_worktree_flow_recipe.py` | Modified in the implementation phase | Cover enum/default/rejection and materialization contract. |
| `tests/test_worktree_gate_hook.py` | Modified in the implementation phase | Cover superproject planning paths, unrelated superproject paths, protected subrepo branches, linked worktrees, exact branch names, uninitialized modules, malformed input, symlinks/nonexistent targets, and unresolved topology. |
| `tests/test_repo_topology.py` | Modified only if shared topology helpers change | Preserve the existing initialized/uninitialized detection contract; do not make the gate depend on advisory fan-out metadata. |
| `ai-specs/recipes/worktree-flow/**` | Derived output only | Regenerate from catalog sources; never hand-edit consumer artifacts. |
| `docs/recipes-catalog.md`, `docs/ai-specs-toml.md` | Modified if recipe configuration is cataloged there | Document `gate_scope` and distinguish it from `gate_mode` and `repo_topology`. |

No changes are expected in `openspec/config.yaml`, `plan-build-flow` production
logic, or worktree creation/cleanup implementation. The plan-build hook's
existing central-root resolution remains the source of truth for active central
plans; this change only makes the worktree gate agree about the permitted artifact
path.

## Compatibility and migration

### Existing manifests

A missing `gate_scope` MUST resolve to `auto`. Existing manifests therefore need
no data migration and retain today's behavior in standalone repositories and
`monorepo-apps`: protected primary worktrees remain blocked, linked worktrees
remain allowed, and malformed/unrelated hook events retain existing handling.

For recognized `monorepo-submodules`, `auto` adds only the central planning
exception described above. It does not alter the subrepository protected-branch
rule.

### Existing materialized hooks

The worktree hook is distributed as a generated/consumer artifact. Existing
consumer copies that are protected by a `not_exists` override condition or have
been customized MUST NOT be silently overwritten. Sync/doctor should report a
non-blocking warning with explicit refresh/removal instructions when the
materialized hook does not contain the new scope contract. A team may refresh a
catalog-owned copy or intentionally retain a customized hook after reviewing the
new boundary.

The migration is source-compatible: add the config key, sync the recipe, and
refresh stale generated consumers when desired. No branches, worktrees, plans, or
repository metadata are moved.

### Existing local planning folders

The superproject central plan takes precedence for a proven submodule relationship,
but existing subrepository-local `openspec/changes/` folders are left untouched.
The change does not copy, merge, delete, or automatically reconcile them. Teams
should adopt one canonical superproject change folder for cross-repository work;
local cleanup is a separate operational decision.

### Rollback compatibility

Rollback is a source and generated-artifact revert. Manifests without the new key
continue to resolve to the old effective behavior when the pre-change recipe is
restored. No persisted scope state or artifact conversion needs to be undone.
Consumer-customized hooks remain untouched during rollback.

## Risks and mitigations

| Risk | Likelihood | Mitigation |
|---|---:|---|
| A path heuristic misclassifies a submodule as the superproject or vice versa | Medium | Use owning-repository Git facts, `.gitmodules` registration, initialized status, component-aware containment, canonical paths, and unique relationship proof. Reject ambiguous relationships. |
| `--show-superproject-working-tree` is empty from a linked submodule worktree | High if used naively | Treat common Git directory plus `.gitmodules` registration as the primary relationship proof; use the superproject probe only as corroboration. |
| A central planning exception becomes a superproject production escape | Medium | Compare only against canonical `<superrepo>/openspec/changes/**`; do not allow arbitrary superproject files or basename lookalikes. |
| A new scope setting accidentally bypasses subrepo branch protection | High impact | Keep a mandatory subrepo safety floor in every scope value; test subrepo primary checkouts under `auto`, `superrepo`, and `subrepo`. |
| Protected branch matching drifts while scope logic changes | Medium | Reuse exact token matching against `WORKTREE_GATE_PROTECTED`; add cases for `main`, `development`, similarly named branches, and slash-containing branch names. |
| Missing or invalid stamp causes inconsistent behavior across consumers | Medium | Validate at sync, warn on invalid/missing runtime stamps, and fall back to `auto` with topology proof rather than a permissive value. |
| Existing consumer override prevents the new hook from materializing | High in known consumers | Preserve `not_exists`/ownership safety and emit clear sync/doctor refresh guidance; never silently overwrite user-customized files. |
| A central active plan is mistaken for production authorization | Medium | Keep plan-build authorization separate and require both gates for production writes. |
| Symlink or nonexistent target paths escape the intended boundary | Medium | Canonicalize existing ancestors and final targets, compare path components, and reject outside or unresolved relationships. |
| Future topology changes invalidate the resolver | Low | Couple scope proof to the existing `worktree-flow` `monorepo-submodules` contract and explicitly defer nested/recursive topology. |

## Rollback plan

1. Revert the source recipe, materialization placeholder, hook logic, docs, and
   canonical spec changes as one reviewable change.
2. Restore the prior materialized hook for catalog-owned consumers through the
   normal sync/refresh path; do not force-overwrite customized hooks.
3. Remove the `gate_scope` manifest key only if a consumer explicitly added it;
   the pre-change recipe ignores it, and no migration data remains.
4. Re-run the existing worktree gate contract checks after rollback. Standalone
   and subrepository protected-branch behavior must return to the pre-change
   nearest-repository decision.
5. Leave all OpenSpec plans, branches, worktrees, submodule registrations, and
   tracker state unchanged; the feature has no persistent runtime state to
   migrate back.

## Measurable success criteria

The implementation/spec phase is complete only when all of the following are
observable:

- `ai-specs sync` accepts absent `gate_scope` as `auto`, accepts exactly
  `auto | superrepo | subrepo`, and rejects any other value with a non-zero exit
  and a diagnostic naming both the invalid value and allowed enum.
- A freshly materialized hook contains the validated `gate_scope` stamp and does
  not read a consumer manifest or Python internals at runtime.
- A valid `WORKTREE_GATE_SCOPE` override takes precedence; invalid overrides and
  invalid/missing stamps warn and fall back as specified; `gate_mode=off` still
  exits before scope evaluation.
- In a temporary fixture with an initialized submodule and a primary
  superproject checkout on a protected branch, a write under the exact
  `<superrepo>/openspec/changes/**` subtree is allowed.
- In the same fixture, writes to `<superrepo>/src/**`, `.gitmodules`, root
  configuration, and arbitrary sibling paths remain blocked on the protected
  branch.
- In a temporary fixture with an initialized submodule primary checkout on
  `main` or `development`, production writes remain blocked under every
  `gate_scope` value, even when the superproject has an active `tasks.md`.
- A linked submodule feature worktree remains writable under every valid scope,
  with no new worktree creation or cleanup behavior.
- Exact protected branch matching is preserved: `main` does not match
  `main-feature`, and configured names are neither globbed nor reinterpreted by
  scope.
- Uninitialized (`-`), unrelated, nested/ambiguous, symlink-escaping, malformed,
  nonexistent-target, and unresolved-topology cases never receive a new
  production-write bypass.
- `plan-build-flow` continues to find the central active `tasks.md` and remains
  the sole plan authorization for production paths; central artifact writes and
  plan-build decisions remain separate gates.
- Existing standalone and `monorepo-apps` fixtures retain the pre-change gate
  decision for protected primary writes and linked worktrees.
- Sync/doctor reports stale or customized materialized consumer hooks without
  silently overwriting them.
- Recipe, canonical spec, README/SKILL guidance, and generated consumer outputs
  describe the same enum, precedence, topology proof, and central-path boundary.

## Next phase

Proceed to the **spec** phase. Convert the policy and matrix above into canonical
Given/When/Then requirements before any design or implementation work. The spec
must preserve the exact central path allowlist, the subrepository safety floor,
linked-worktree compatibility, branch-name exactness, and fail-safe topology
proof as non-negotiable invariants.
