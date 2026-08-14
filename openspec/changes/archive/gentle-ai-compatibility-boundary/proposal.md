# Proposal: ai-specs-owned topology context and conservative gate refresh

## Intent

Separate three concepts currently disjoint: the code/VCS owner (subrepo vs superrepo), the explicit fan-out target set, and the canonical superrepo planning root. Propagate one ai-specs request context (owner, fan-out, planning root) to worktree creation, plan paths, render metadata, plan-build, VCS calls. Make gate refresh conservative and reversible via recorded baseline bytes and a cache-only immutable backup.

## Scope

### In Scope

**Work unit 1 — request context/root propagation**
- Subrepo request: resolve via `show-toplevel`/validated `.gitmodules`; subrepo-owned worktree at `<super>/.worktrees/<subrepo>-<slug>`; planning only under `<super>/openspec/changes/<slug>/`. Superrepo: worktree at `<super>/.worktrees/<slug>`, same planning root.
- Propagate source/target/planning roots through `target-resolve.py`, `sync.sh`/`sync-agent.sh`, render metadata, plan-build, `premerge_guardian.py`.
- `project.subrepos` stays authoritative; `monorepo-apps` explicit.

**Work unit 2 — gate provenance and refresh**
- Record last-CLI-rendered gate bytes in lock/cache: baseline match → update; mismatch or missing provenance → preserve on sync.
- Explicit refresh of a customized gate saves exact pre-refresh bytes to a cache-only immutable backup; atomic lock/cache update.
- RED/GREEN tests + OpenSpec/recipe/docs updates.

### Out of Scope

- Gentle recipe/capability, authority envelope, lifecycle protocol/gate, review START, receipts/lineage.
- Automatic `.gitmodules` fan-out; silent topology reclassification (incl. `venturi_coffee`); root-cause claims on external `invalid_request`.
- Gentle repo repair; new external dependencies.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `worktree-flow`: owner-vs-planning-root facts; subrepo worktree creation contract; fan-out target semantics.
- `plan-build-flow`: planning-root propagation to artifact writers; central-write boundary without orchestration side effects.
- `override-ownership`: gate provenance baseline; refresh policy and cache-only backup for runtime hook scripts.

## Approach

Reuse proven Git facts as one explicit ai-specs request context for worktree/fan-out/planning/render/VCS call sites; extend the managed-byte model to gates with conservative provenance and cache-only refresh backups. Two reviewable code work units; gate evaluation read-only; no-Gentle-AI behavior unchanged.

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `lib/_internal/util.py` | Modified | Owner/planning-root context |
| `lib/_internal/target-resolve.py` | Modified | Manifest fan-out + root propagation |
| `lib/sync.sh`, `lib/sync-agent.sh` | Modified | Explicit source/target/planning context |
| `lib/_internal/recipe-materialize.py` | Modified | `project_root`; gate provenance/refresh |
| `lib/_internal/agents-render.py`, `premerge_guardian.py` | Modified | Use resolved planning root |
| `lib/_internal/lock.py`, `project-cache.py` | Modified | Gate baseline + cache backup namespace |
| `catalog/recipes/{worktree-flow,plan-build-flow}/**`, `openspec/specs/{worktree-flow,plan-build-flow,override-ownership}/spec.md` | Modified | Docs/spec alignment |
| `tests/test_{repo_topology,target_resolve,sync_pipeline,worktree_root_propagation,plan_build_gate_hook,worktree_flow_recipe,recipe_materialize,override_ownership}.py` | Modified | RED/GREEN fixtures |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Fan-out expansion overwrites real ownership (empty `subrepos` in `melon-alquimia`) | Med | `project.subrepos` stays authoritative |
| Unknown gate provenance treated as unmodified → custom hook lost | Med | Fail safe: preserve on missing/mismatch |
| `/worktree-new` is generated Markdown, not executable | Med | Choose seam + integration test first |

## Rollback Plan

Revert the branch (code + tests only; no external/authority writes). Gate refresh restores prior bytes from cache backup; plan-path change reverts to relative-path behavior. No user-project mutation before explicit refresh.

## Dependencies

None.

## Success Criteria

- [ ] Subrepo request: subrepo-owned worktree; plan only under superrepo `openspec/changes/<slug>/`.
- [ ] Fan-out preserves explicit targets and one planning root; stops on first incompatible target.
- [ ] Unchanged gates refresh; customized gates preserved with one immutable cache backup only on explicit refresh; unknown provenance preserved.
- [ ] `./tests/validate.sh` passes with Gentle AI absent/disabled.

## Tracker

- card_id: `6a7cadebfb3b957529926508`
- url: https://trello.com/c/omB2CUU8/73-story-add-optional-gentle-ai-compatibility-boundary
