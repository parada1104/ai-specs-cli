# Explore: topology-aware `gate_scope`

> Tracker: [Trello #65 — follow-up worktree-flow gate scope for cross-repo superrepos](https://trello.com/c/LIoDU2xL/65-follow-up-worktree-flow-gate-scope-for-cross-repo-superrepos)

## Assignment and boundary

This is an exploration artifact only. No production code, generated consumer output, tests, or recipe implementation was changed. The follow-up is intended to make the worktree gate topology-aware without weakening protection on subrepository protected branches.

## Current implementation findings

### Recipe/config vocabulary

`catalog/recipes/worktree-flow/recipe.toml` currently exposes `gate_mode = always | ask | off`, `repo_topology = auto | standalone | monorepo-apps | monorepo-submodules`, worktree layout, and integration branch. `repo_topology` is already validated at sync, defaulted to `auto`, rendered in the brief, and stamped only into cleanup (`__WORKTREE_REPO_TOPOLOGY__`). The gate is stamped only with `__WORKTREE_GATE_MODE__`; it has no topology or scope input today.

The safe follow-up vocabulary should add a separate `gate_scope` under `[recipes.worktree-flow.config]`, not overload `gate_mode` or `repo_topology`. Recommended enum: `auto | superrepo | subrepo` (with `auto` as the default). `superrepo` means the gate applies to the superproject's own main worktree; `subrepo` means it applies to initialized submodule repositories/worktrees; `auto` derives the applicable repository from Git topology. This keeps topology classification (`repo_topology`) orthogonal to enforcement scope (`gate_scope`). Invalid values must fail at sync with the same diagnostic shape as existing enum validation. The exact public spelling remains a design decision for proposal/spec; do not silently introduce aliases.

### Runtime gate behavior

`catalog/recipes/worktree-flow/hooks/worktree-gate.sh` resolves each candidate path by nearest existing ancestor, then `git rev-parse --absolute-git-dir`, `--git-common-dir`, and current symbolic branch. A linked worktree (`git_dir != common_dir`) is allowed; a primary/main worktree is gated by `WORKTREE_GATE_PROTECTED` (default `main development`). It is deliberately fail-open for malformed input, non-Git paths, ambiguous heuristics, and lookup failures.

The key topology fact is that a superproject primary checkout and a submodule primary checkout are separate Git repositories. Existing behavior therefore gates both when their current branch is protected. The requested follow-up is not a blanket bypass: in a `monorepo-submodules` setup, central superproject structure/planning writes may be allowed according to an explicit scope policy while `main`/`development` writes in subrepo primary checkouts remain blocked. Linked feature worktrees remain writable by design.

### Materialization/stamping

`lib/_internal/recipe-materialize.py` already has `REPO_TOPOLOGY_PLACEHOLDER` and substitutes merged `repo_topology` for cleanup templates. `materialize_hook_script` is the correct boundary for a new gate stamp, but the hook must not read a consumer manifest at runtime: distributed hooks may run without the CLI or Python project internals. Stamp the validated effective `gate_scope` into the hook, with an environment override only if the contract explicitly defines precedence and validation. Preserve the existing mode precedence: environment override beats stamped value, invalid override warns and falls back; invalid/missing stamp falls back safely.

Consumer overrides are materialized with `condition = "not_exists"`; existing Melón/Alquimia copies will not refresh automatically. Any implementation must document refresh/removal of the old override or provide a non-destructive sync/doctor warning. Never silently overwrite a consumer-customized hook.

### Topology utilities and path resolution

`lib/_internal/util.py::resolve_repo_topology` is pure/read-only. `auto` resolves initialized `.gitmodules` entries to `monorepo-submodules`, otherwise `standalone`; it never auto-selects `monorepo-apps`. Detection counts status prefixes space, `+`, and `U`; `-` is uninitialized and excluded. Explicit `standalone` and `monorepo-apps` bypass detection.

The runtime boundary must use Git facts from the candidate's owning repository, not `project.subrepos` (that list is advisory sync fan-out and Melón leaves it empty despite many Git submodules). Canonicalize target/cwd and probe the nearest existing ancestor. For a subrepo primary checkout, `git rev-parse --show-toplevel` identifies the subrepo and `git rev-parse --git-common-dir` points into `<super>/.git/modules/...`; verify registration in the superproject `.gitmodules` and initialized status before classifying it as a subrepo. `--show-superproject-working-tree` is useful from the primary checkout but is empty from linked submodule worktrees, so it MUST NOT be the sole signal. Reject nested/ambiguous/unproven relationships and use the conservative nearest-repository behavior.

Containment checks must be component-aware and symlink-safe. A scope decision must never turn an outside path or an arbitrary superproject path into an allowlist. The only central artifact exception should be the exact canonical `<superrepo>/openspec/changes/**` subtree (planning artifacts are already unconditionally writable in plan-build-flow); central structure paths outside that subtree need an explicitly specified scope rule, not an accidental root bypass.

### Plan-build central artifact behavior

`catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh` already derives a central planning root for a proven initialized submodule relationship. It checks the nearest repo's active plan first, then central `<superrepo>/openspec/changes/*/tasks.md`; central artifact writes are unconditionally allowed by path. Its design explicitly says central planning location does not grant a superproject-wide production-write bypass. `gate_scope` must preserve that separation: worktree gate scope controls repository/worktree protection; plan-build gate remains the owner of plan authorization and central artifact semantics.

## Safe semantic boundary for the follow-up

1. Resolve the candidate's owning Git repository before branch protection.
2. Classify that repository as superproject or initialized subrepo only after structural `.gitmodules` + Git-dir + initialized-status proof.
3. Apply `gate_scope` only to that classification. In `auto`, preserve current protection for both primary repositories; allow only the narrowly intended superrepo structural/planning scope if the proposal explicitly defines it. In `subrepo`, protected subrepo primary branches remain blocked; in `superrepo`, protected superrepo branches remain protected except for the explicitly enumerated central structure/planning paths.
4. Never use `gate_scope` to allow writes in linked feature worktrees that are already allowed, to bypass `gate_mode`, to authorize subrepo production files, or to make plan-build active-plan checks disappear.
5. Any inability to prove topology or resolve scope is fail-safe: do not grant a production-write bypass. Preserve existing fail-open behavior only for malformed/unrelated hook events, as currently documented.
6. Keep `WORKTREE_GATE_PROTECTED` branch matching exact and configurable; scope must not silently redefine protected branch names.

## Melón/Alquimia evidence

Archived topology exploration records the lived `melon-alquimia` layout: 11 `.gitmodules` entries, 10 initialized and one `-` uninitialized; shared linked worktrees live at `<super>/.worktrees/<subrepo>-<slug>` and are owned by each submodule repository; the superproject's `git worktree list` does not enumerate submodule worktrees. Names currently equal paths (for example `alquimia-front-web`), but implementation must resolve by registered path and only accept a unique name as a convenience. `alquimia-front` versus `alquimia-front-web` demonstrates why longest-prefix/name heuristics need disambiguation. Some legacy per-submodule `.worktrees` directories exist and are unsupported by the established shared-layout contract.

The observed gate facts were: superproject primary and submodule primary checkouts have `git_dir == common_dir` and are gate candidates; linked submodule worktrees have differing Git dirs and are allowed. This supports a repository-ownership scope, not a basename-only or cwd-only rule.

## Required follow-up investigation/design work

- Decide and specify the final enum vocabulary and default/override precedence for `gate_scope`.
- Define the exact superrepo paths considered “central structure/planning” versus production paths; prefer an explicit component-aware allowlist over a broad superproject bypass.
- Add focused temporary-fixture scenarios for superrepo primary, subrepo primary, linked subrepo worktree, central `openspec/changes`, unrelated superrepo files, uninitialized modules, malformed input, symlink/nonexistent targets, and unresolved topology.
- Update canonical recipe schema/spec/docs, materialization stamping, and consumer refresh diagnostics together; do not implement production code in this exploration phase.
