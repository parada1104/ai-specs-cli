# Design: topology-aware `gate_scope`

## Context

`repo_topology` already identifies initialized submodules for worktree creation and central planning, but `worktree-gate.sh` only knows gate mode and protected branches. In a monorepo-submodules project this gates both the superproject primary checkout and subrepo primary checkouts, forcing an unnecessary superrepo worktree for central planning/structure writes.

## Configuration contract

Add `[recipes.worktree-flow.config].gate_scope` with enum `auto | superrepo | subrepo`, default `auto`. Keep it orthogonal to `gate_mode` and `repo_topology`.

- `auto`: preserve topology-derived behavior; in a proven `monorepo-submodules`
  relationship gate both superrepo and subrepo protected primaries, with only
  the canonical superrepo planning exception.
- `superrepo`: enforce only proven superrepo protected primaries; proven subrepo
  writes are outside this selected enforcement scope.
- `subrepo`: enforce only proven initialized subrepo primaries; proven superrepo
  writes are outside this selected enforcement scope for the Melón workflow.
  Central planning remains explicitly documented, not a general inferred path.

`gate_mode=off` resolves before scope and allows all events, preserving existing precedence. Environment `WORKTREE_GATE_SCOPE` overrides the stamped value only for the invocation; invalid overrides warn and fall back to the stamp, then `auto`.

## Materialization

Extend `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` with
`__WORKTREE_GATE_SCOPE__` and `__WORKTREE_REPO_TOPOLOGY__` plus validated
resolvers. Extend `recipe-materialize.py` placeholder replacement using the
merged validated config. The generated hook remains self-contained: it must
not read the consumer manifest or import Python project internals at runtime.

Bump the worktree recipe version and update schema/docs/specs. Existing `condition = "not_exists"` hook copies are not silently overwritten; doctor/sync must emit a migration warning with the exact remove-and-sync command when the old generated hook lacks the new stamp.

## Runtime classification

For each candidate path, canonicalize the nearest existing ancestor and obtain owning Git facts (`--show-toplevel`, `--absolute-git-dir`, `--git-common-dir`, branch). A linked worktree (`git_dir != common_dir`) remains allowed before scope evaluation.

To classify a subrepo, prove all of:

1. owning repository root is inside a registered superproject submodule path;
2. the path is registered in `.gitmodules` and initialized (`git submodule status` prefix is space, `+`, or `U`);
3. the owning Git directory/common-dir relationship matches the registered submodule, including linked worktrees where the superproject probe may be empty.

Classify the containing repository root as `superrepo` only when the superproject Git facts and registered relationship are proven. If any proof is missing, ambiguous, outside the repository, or symlink-unsafe, fall back to the existing nearest-repository branch gate without granting the central exception.

## Decision order

1. Resolve `gate_mode`; `off` exits successfully.
2. Parse event and candidate paths; malformed/unrelated events fail open as today.
3. Resolve nearest existing path and Git facts; non-Git/ambiguous lookup fails open as today.
4. Allow existing generated-runtime configuration paths and linked worktrees.
5. Resolve stamped/environment `gate_scope` and stamped `repo_topology`.
6. Require effective `repo_topology=monorepo-submodules` before topology scope
   classification; explicit `standalone`/`monorepo-apps` disables any central
   scope proof even when vendored initialized modules exist.
7. For `auto` and `superrepo`, allow only canonical
   `<superrepo>/openspec/changes/**` when the owning repository is the proven
   superrepo. For `subrepo`, superrepo enforcement is outside the selected
   scope; component-aware containment and symlink resolution remain mandatory.
8. For protected primary repositories inside the selected scope, apply exact
   `WORKTREE_GATE_PROTECTED` matching. `auto` gates both proven owner classes;
   `superrepo` gates only superrepo; `subrepo` gates only subrepo.
9. Preserve existing `ask` remediation and shell/path parity.

The worktree gate does not replace `plan-build-gate`: plan authorization and active-plan checks remain owned by plan-build-flow. `gate_scope` only controls worktree protection and the narrow central planning exception.

## Tests

Add hermetic fixtures for: standalone compatibility; proven superrepo central artifact allow; proven superrepo non-central block; subrepo primary protected block; linked subrepo worktree allow; explicit scope modes; initialized/uninitialized/ambiguous submodules; similar-name paths; symlink and nonexistent targets; exact protected branches; malformed stamps/env values; shell write parity; `gate_mode=off` precedence; and stale consumer hook migration diagnostics.

Run focused gate/schema/materialization suites, then the full project validation. Add a live smoke using a temporary superproject with an initialized local submodule and a linked subrepo worktree.

## Rollout

Consumers with existing materialized hooks must run `ai-specs sync`. If an old hook is preserved by `not_exists` and lacks the scope stamp, sync reports the exact hook path and recommends removing only that generated hook before re-running sync. Customized declared overrides remain governed by ownership policy and are never silently replaced.
