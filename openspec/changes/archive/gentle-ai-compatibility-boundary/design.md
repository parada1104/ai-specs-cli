# Design: ai-specs-owned topology context and conservative gate refresh

## Technical Approach

Two bounded work units. **WU1** resolves one ai-specs request context (`owner_root`, `subrepo_path`, `planning_root`, `topology`) from proven Git facts, propagating it `util.py` → `target-resolve.py` → `sync.sh`/`sync-agent.sh` → resolved-config → `agents-render.py`/`premerge_guardian.py`, pinning the `/worktree-new` contract. **WU2** extends the managed-byte lock model to gate hooks: baseline classification, cache-only immutable backups, explicit refresh flag. No Gentle/provider machinery, no external dependency.

## Architecture Decisions

### WU1: request context

| Option | Tradeoff | Decision |
|---|---|---|
| Generic provider framework | Over-engineered; user forbids | Rejected |
| `resolve_request_context()` in `util.py`, JSON over existing argv/temp-file seams | Minimal; reuses proven `resolve_subrepo`/`resolve_repo_topology` | Chosen |

`resolve_request_context(cwd, explicit_subrepo=None)`: owner_root = `git rev-parse --show-toplevel`; subrepo_path = `resolve_subrepo(...)` under `monorepo-submodules`; planning_root = proven superrepo for submodule owners, else owner_root. Missing/ambiguous/detached/uninitialized → fail safe to owner_root; no planning-root exception.

### WU1: worktree seam

`/worktree-new` stays generated Markdown executed by the agent; no new executable helper (no new routing surface). Contract: `resolve_request_context()` unit tests plus one real-git integration test (superproject + initialized submodule): subrepo cwd → owner=subrepo, planning_root=super; `git -C` create yields subrepo-owned worktree (assert both repos' `git worktree list`); superrepo cwd without explicit subrepo → hard error before any `git worktree add`.

### WU1: fan-out

`project.subrepos` stays sole authoritative target set; `.gitmodules` never expands it; empty list = no fan-out; stop-on-first-failure preserved. `melon-alquimia` (`[]`), `salones` (selected), `venturi_coffee` (unverified) not reclassified; `monorepo-apps` remains distinct.

### WU2: baseline storage

Reuse the lock: `[managed."ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh"]`, `kind="gate"`, `policy="auto"`, sha256 of last rendered bytes, via existing `set_managed_override`/`classify_managed_override`. `materialize_hook_script` classifies before write: baseline match → write + record; mismatch/missing → preserve + warn. Legacy reference copy stays unconditional (never the launcher).

### WU2: refresh + immutable cache backup

New `--refresh-gates` flag on `recipe-materialize.py` (never set by ordinary sync). Sequence: write backup → write gate → write lock; any failure deletes the new backup and restores prior bytes (all-or-nothing). Backup: `cache_root()/backups/<sha256(rel-path)>/<sha256(content)>.sh`; content-hash name immutable, collision-safe, cache-only, outside user projects.

## Data Flow

```
cwd → resolve_request_context → {owner_root, subrepo_path, planning_root, topology}
      ▼
sync.sh → recipe-materialize (resolved-config += project_root, topology)
      │  ▼ agents-render · premerge_guardian --root
      ▼
sync-agent.sh → per-target fan-out (declared only)
WU2: materialize_hook_script → classify → write|preserve
     --refresh-gates → cache backup → atomic replace
```

## File Changes

| File | Action | Description |
|---|---|---|
| `lib/_internal/util.py` | Modify | `resolve_request_context()` (WU1) |
| `lib/_internal/target-resolve.py` | Modify | Plan JSON: `planning_root`, `topology`, `declared_only` (WU1) |
| `lib/sync.sh`, `lib/sync-agent.sh` | Modify | Consume/propagate context (WU1) |
| `lib/_internal/recipe-materialize.py` | Modify | resolved-config `project_root` (WU1); gate classification + `--refresh-gates` (WU2) |
| `lib/_internal/agents-render.py` | Modify | Verify only: already reads `resolved.project_root` (WU1) |
| `lib/_internal/premerge_guardian.py` | Modify | `--root` required; callers pass planning root (WU1) |
| `lib/_internal/project-cache.py` | Modify | `backups_root()` (WU2) |
| `lib/doctor.sh` / `_internal/doctor.py` | Modify | Gate provenance warnings (WU2) |
| `catalog/recipes/{worktree-flow,plan-build-flow}/**`, `openspec/specs/{worktree-flow,plan-build-flow,override-ownership}/spec.md` | Modify | Docs/spec alignment (both) |

## Interfaces / Contracts

```json
{"owner_root": "...", "topology": {"resolved": "...", "via": "auto|config"},
 "subrepo_path": "apps/api|null", "planning_root": "...",
 "worktrees_dir": ".worktrees", "fanout_targets": ["..."], "declared_only": true}
```

## Testing Strategy

| Layer | What | Approach |
|---|---|---|
| Unit | Request-context resolver; gate classifier; backup naming | Extend `test_repo_topology.py`, `test_override_ownership.py`, `test_recipe_materialize.py`, `test_target_resolve.py` |
| Integration | Submodule fixture worktree ownership; fan-out JSON; sync pipeline; atomic gate refresh | Temp repos + `git worktree list`; `test_worktree_root_propagation.py`, `test_sync_pipeline.py`, `test_worktree_flow_recipe.py`, `test_plan_build_gate_hook.py` |
| E2E/smoke | `./tests/validate.sh` with no Gentle present | RED/GREEN evidence |

Replace `test_override_ownership.py::test_hook_materialization_remains_unconditional` (pins obsolete behavior).

## Threat Matrix

| Boundary | Minimum adversarial cases | Applicability | Design response | Planned RED tests |
|---|---|---|---|---|
| Documentation-like paths | `requirements.txt`, `CMakeLists.txt`, executable Markdown/MDX, `README.sh` | Applicable: generated gate hook is executable | Baseline sha256; preserve on mismatch/missing | Customized gate preserved; match refreshes; missing provenance preserved |
| Git repository selection | `git -C`, relative paths, absolute paths | Applicable: resolvers + documented create | show-toplevel + `.gitmodules` + longest-prefix; fail safe | Subrepo cwd → subrepo owner; superrepo w/o subrepo → hard error; explicit/inferred mismatch |
| Commit state | staged, `commit -a`, empty index | N/A: no index/commit automation | — | — |
| Push state | tracking branch, first push, explicit refspec | N/A: no push automation | — | — |
| PR commands | explicit `--head`, environment prefix, composed commands | N/A: guardian is local read-only | — | — |

Applicable rows: safe = preserve bytes / no worktree on unresolved context; failure = hard error before mutation. Carry unchanged into `tasks.md`.

## Migration / Rollout

Gates without baseline → preserved + warning (no seeding). Rollback = revert branch; cache backups out-of-tree, harmless. No user-project mutation before explicit refresh.

## Open Questions

None blocking.
