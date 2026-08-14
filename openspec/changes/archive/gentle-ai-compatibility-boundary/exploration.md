## Exploration: ai-specs topology, fan-out, canonical planning, and gate refresh

### Current State
`ai-specs-cli` already has three separate concepts, but they are not propagated as one request context:

- **Git/code ownership.** `util.resolve_repo_topology()` (`lib/_internal/util.py:207-233`) resolves `auto` to `monorepo-submodules` only when initialized `.gitmodules` entries exist; otherwise it resolves to `standalone`. `monorepo-apps` is explicit-only. `resolve_subrepo()` (`lib/_internal/util.py:340-394`) validates the current `git rev-parse --show-toplevel`, `.gitmodules` path/name, initialization, and longest linked-worktree prefix. The `worktree-new.md` contract then requires `git -C <super>/<subrepo> worktree add` with an absolute destination at `<super>/.worktrees/<subrepo>-<slug>`. A correct subrepo request therefore cannot use the superproject's `git worktree add`; if the current request context is the superrepo, the resolver has no basis to infer a subrepo and should require an explicit one.
- **Derived-artifact fan-out.** `target-resolve.py:93-138` reads only `[project].subrepos` from the root `ai-specs.toml`; `.gitmodules` is explicitly advisory-only. `sync.sh:134-220` resolves the root and declared targets, performs root materialization once, and invokes `sync-agent.sh` for each target. `sync-agent.sh:109-166` implements the public-root fan-out and `:328-356` writes derived artifacts into each target. This is intentional target fan-out, not Git ownership discovery. It preserves selected subrepo lists but does not automatically fan out to every initialized submodule.
- **Canonical planning.** `plan-build-gate.sh:54-169` derives the nearest Git root from the target and lazily proves a central superproject for production-plan lookup. The active `plan-build-flow` specification requires the superproject `openspec/changes/**` tree to be canonical for recognized submodule worktrees, and existing tests prove central absolute-path writes and central active-plan lookup. However, the gate consumes the path supplied by the caller; it does not create or redirect an SDD artifact path. `premerge_guardian.py:344-399` also accepts an explicit `--root` and does not discover topology. Thus the two axes can still diverge: a request from a subrepo can own code in a subrepo worktree while an artifact writer that uses a relative `openspec/changes/...` path writes in the subrepo instead of the central superrepo. The generated plan-build documentation states the desired central behavior, but no shared resolver currently carries `owner_root` and `planning_root` to every artifact phase.
- **Root propagation gap.** `build_resolved_config()` (`lib/_internal/recipe-materialize.py:825-875`) does not include `project_root`, while `agents-render.py:237-245` optionally reads it and otherwise falls back to `Path.cwd()`. Root sync passes an explicit manifest/target to renderers, but the topology displayed in a generated brief can therefore depend on process cwd rather than the explicit source root. Existing root-propagation tests prove file placement, not this metadata path.

Local evidence is available for two reported workspaces:

- `/Users/robert/proyectos/nnodes/melon/melon-alquimia` has 11 `.gitmodules` entries, the `.melon-monorepo` marker, and a root `ai-specs/ai-specs.toml` with `project.subrepos = []`. Its worktree recipe uses `development` but does not explicitly set `repo_topology` or `gate_scope`. The marker is consumed by the project's separate skill-sync conventions; ai-specs topology detection uses `.gitmodules`. The empty fan-out list is therefore not equivalent to “fan out to all Git submodules.”
- `/Users/robert/proyectos/nnodes/salones` has 11 `.gitmodules` entries, `repo_topology = "monorepo-submodules"`, `gate_scope = "subrepo"`, and an explicit fan-out list of only `lounge`, `access-code`, `voucher-go`, and `lounge-deploy-win`. Its OpenSpec config explicitly says the umbrella owns central planning while each subrepo owns its code worktree.
- No `venturi_coffee` directory or matching local project path was found under `/Users/robert/proyectos` (nor an exact `venturi-coffee` project directory). Its topology and manifest remain unverified; no reclassification is made.

`monorepo-apps` should remain distinct from `standalone`: both use the same one-repository worktree mechanics, but the explicit label prevents an application directory from being mistaken for a subrepo and disables submodule scope classification. Existing topology and gate tests assert that it is never auto-selected and does not receive a central exception merely because `.gitmodules` happens to exist.

Gate refresh is currently asymmetric with template refresh. Governed templates use `[managed.*]` lock hashes and `auto|confirm|never-force` (`util.classify_managed_override()`, `recipe-materialize.py:369-449`, `openspec/specs/override-ownership/spec.md`). Runtime hook scripts are explicitly outside that ownership surface: `materialize_hook_script()` (`recipe-materialize.py:488-570`) rewrites them every sync, except that an old worktree gate without `stamped_gate_scope` is preserved with a warning. `materialize_legacy_gate()` is unconditional as well. The per-project CLI cache (`project-cache.py`) has recipe/dependency/bundled/command/resolved-skill directories but no backup namespace. Existing `.bak` conventions are adjacent project-file backups in `env_scaffold.py`; they do not establish cache backup semantics. Therefore “unmodified gate” cannot safely be inferred from current source bytes or stamps alone: it needs a recorded hash of the last CLI-rendered bytes, with unknown/missing provenance treated conservatively.

### Affected Areas
- `lib/_internal/util.py` — preserve topology/subrepo proof and add or expose the owner-versus-planning-root facts without making `monorepo-apps` an auto-detected submodule mode.
- `catalog/recipes/worktree-flow/commands/worktree-new.md`, `skills/worktree-flow/SKILL.md`, `templates/worktree-cleanup.sh`, and `gate/{decide.go,gitfacts.go,topology.go}` — enforce and document subrepo worktree ownership, shared destination, cleanup enumeration, and the distinction between code owner and central plan owner.
- `lib/_internal/target-resolve.py`, `lib/sync.sh`, and `lib/sync-agent.sh` — retain manifest-declared fan-out while propagating explicit source root, target owner, and central planning context; never silently expand `project.subrepos` to every `.gitmodules` entry.
- `lib/_internal/recipe-materialize.py`, `agents-render.py`, and `premerge_guardian.py` — remove cwd-dependent metadata/root assumptions and make artifact consumers use the resolved planning root supplied by ai-specs-owned context.
- `catalog/recipes/plan-build-flow/hooks/plan-build-gate.sh`, `catalog/recipes/plan-build-flow/README.md`, and `openspec/specs/plan-build-flow/spec.md` — align the documented central-write contract with the actual caller/path propagation boundary without adding orchestration side effects.
- `lib/_internal/lock.py`, `lib/_internal/project-cache.py`, and possibly the sync refresh entry point — record last-generated gate bytes and define a cache-only, collision-safe backup for explicit refresh of customized gates.
- `catalog/recipes/worktree-flow/recipe.toml`, its README/skill, `openspec/specs/worktree-flow/spec.md`, and `openspec/specs/override-ownership/spec.md` — document topology, owner/plan-root separation, gate provenance, refresh policy, and backup semantics.
- `tests/test_repo_topology.py`, `tests/test_worktree_gate_hook.py`, `tests/test_worktree_root_propagation.py`, `tests/test_plan_build_gate_hook.py`, `tests/test_target_resolve.py`, `tests/test_sync_pipeline.py`, `tests/test_agents_render_brief_fragments.py`, `tests/test_worktree_flow_recipe.py`, `tests/test_recipe_materialize.py`, and `tests/test_override_ownership.py` — extend existing fixtures rather than introduce a parallel topology model.

The supplied external error `invalid_request: negotiated review start lineage is not canonical` remains evidence only. No ai-specs root cause, Gentle authority rule, Gentle recipe, review protocol, receipt, or lineage implementation is inferred or proposed.

### Approaches
1. **Documentation and existing-test alignment only** — Clarify the two axes and add assertions to current topology/fan-out documents without changing root propagation or gate materialization.
   - Pros: smallest diff; preserves all runtime behavior; low migration risk.
   - Cons: cannot ensure a subrepo request selects the owning subrepo or that a canonical plan path is actually carried to artifact writers; cannot safely refresh generated gates.
   - Effort: Low

2. **Explicit ai-specs request context for owner, fan-out, and planning root** — Reuse proven Git facts to produce an explicit context: owning repository/worktree, superrepo when proven, declared fan-out targets, and canonical planning root. Use it for subrepo worktree creation, central plan paths, rendered root metadata, and plan-build invocation; keep `project.subrepos` authoritative and keep gate evaluation read-only.
   - Pros: directly fixes the local topology/fan-out boundary; preserves central planning for `monorepo-submodules`; prevents superrepo worktrees for subrepo requests; keeps `monorepo-apps` and standalone behavior stable.
   - Cons: crosses shell/Python/recipe boundaries; the current `/worktree-new` surface is a generated Markdown command rather than an executable helper, so an implementation seam and integration test must be chosen carefully.
   - Effort: Medium/High

3. **Lock-backed gate ownership with explicit refresh backup** — Extend the existing managed-byte model to generated worktree gates. Record the normalized rendered bytes last written by the CLI; update automatically only when the current file matches that baseline; treat missing or divergent provenance as unknown/user-modified, preserve it during ordinary sync, and place a deterministic `.bak` snapshot in the per-project CLI cache before an explicit replacement/refresh.
   - Pros: makes “user has not changed the gate” testable; prevents silent loss of custom hooks; reuses lock hashing and the existing per-project cache; keeps backups outside user repositories.
   - Cons: requires migration semantics for existing gates with no baseline; needs an explicit refresh trigger and collision/retention rules; changes the current unconditional runtime-hook contract.
   - Effort: Medium

### Recommendation
Proceed with Approach 2 plus Approach 3 as two bounded implementation work units under this change; do not revive the prior provider-neutral identity boundary or add a Gentle recipe. The first work unit should establish one ai-specs-owned context for the two independent axes:

- A request from a **subrepo** resolves the subrepo from `show-toplevel`/validated `.gitmodules`, creates the branch/worktree through that subrepo at the absolute shared-superproject path `<super>/.worktrees/<subrepo>-<slug>`, and writes/reads planning artifacts under `<super>/openspec/changes/<slug>/`.
- A request from the **superrepo** owns the superrepo, creates a superrepo worktree under `<super>/.worktrees/<slug>`, and uses that same superrepo as the planning root.
- Fan-out remains driven by the root manifest's explicit `project.subrepos`. `melon-alquimia` therefore needs an explicit product decision before adding targets; silently treating all 11 `.gitmodules` entries as fan-out targets would violate current semantics. `salones` already demonstrates the intended selected-target model.
- `monorepo-apps` remains an explicit, standalone-mechanics label. Do not temporarily change `venturi_coffee` without its manifest and marker evidence.

The second work unit should add gate provenance and refresh semantics. The safe default is: a recorded matching baseline is “unmodified” and may be force-updated; a byte mismatch is user-modified and is preserved with a warning; no baseline is unknown and is also preserved. An explicit refresh may replace a customized gate only after saving its exact pre-refresh bytes to a cache path keyed by project cache key and project-relative target. The first backup should be immutable (or use a content-hash suffix) so repeated refreshes cannot destroy the original customization. A bare `.bak` in the project tree is not appropriate because generated cache assets are already out-of-tree and `.bak` conventions are otherwise project-specific.

The first delivery slice should be code + focused tests + OpenSpec/recipe docs, but it should be split for review: topology/owner/planning/fan-out first, gate refresh second. The smallest likely work units are:

1. `util.py` plus a narrow context/root-propagation helper, `target-resolve.py`, `sync-agent.sh`/`sync.sh`, and the plan-build/renderer call sites; tests in `test_repo_topology.py`, `test_target_resolve.py`, `test_worktree_root_propagation.py`, `test_plan_build_gate_hook.py`, and `test_sync_pipeline.py`.
2. `recipe-materialize.py`, `lock.py`, `project-cache.py` (and only the explicit sync refresh entry point if required); tests in `test_worktree_flow_recipe.py`, `test_recipe_materialize.py`, and `test_override_ownership.py`.
3. Contract updates to `worktree-flow`, `plan-build-flow`, and `override-ownership` specs plus generated recipe README/skill text; no external-provider documentation or runtime dependency.

Concrete RED/GREEN coverage must include:

- a real initialized-submodule fixture where a subrepo request creates a worktree owned by the subrepo, never by the superrepo; inspect both repositories' `git worktree list` output and the absolute destination;
- a subrepo-context request that writes a central plan and proves the artifact exists only under the superrepo canonical `openspec/changes/<slug>/`, while a production write still targets the subrepo owner;
- fan-out metadata/behavior that preserves the owning target and one central plan root, does not duplicate plans, and stops on the first incompatible target as current sync does;
- explicit `monorepo-apps` and `standalone` fixtures proving identical one-repository worktree mechanics, no submodule central exception, and no auto-selection of `monorepo-apps`;
- a generated gate whose recorded bytes are unchanged, whose stamps/source version evolve, and whose customized bytes differ: unchanged gates refresh, customized gates remain intact and receive one cache backup only on explicit replacement, missing provenance is preserved conservatively, and lock/cache state is updated atomically;
- sync/plan-build behavior with no external Gentle installation, disabled gate/runtime, and absent external environment proving byte/decision compatibility and no new provider prerequisite. These tests must not invoke or inspect the external Gentle repository.

### Risks
- The current plan-build gate can prove the central root for a supplied absolute target, but artifact writers and `premerge_guardian.py` do not receive a shared planning-root context; changing only gate prose will leave the reported boundary unresolved.
- `project.subrepos` is intentionally narrower than `.gitmodules` in `salones` and empty in `melon-alquimia`; automatic expansion would overwrite the ownership policy of real consumers.
- `/worktree-new` is currently a generated Markdown instruction, not a CLI implementation; a test can prove the contract only after the implementation seam is selected.
- Existing materialized gates often have no managed-byte provenance. Treating them as unchanged from stamps or catalog similarity could overwrite custom content; unknown state must fail safe.
- Cache backups need deterministic pathing, collision handling, and privacy/retention rules; they must never expose secrets or mutate the user project before explicit refresh.
- The external Gentle failure is pre-mutation evidence only. Attributing it to topology, root selection, or planning would exceed the available evidence.

### Ready for Proposal
Yes. Replace the stale proposal scope with an ai-specs-owned topology/fan-out/planning-root change plus a separate gate-refresh ownership slice. The proposal should explicitly preserve `project.subrepos`, central superrepo planning for proven submodules, distinct `monorepo-apps`, existing absent/disabled external behavior, and user-customized gate bytes. It should not define Gentle authority, root-selection policy, lifecycle protocol, a new Gentle recipe, or a root cause for the reported external error.
