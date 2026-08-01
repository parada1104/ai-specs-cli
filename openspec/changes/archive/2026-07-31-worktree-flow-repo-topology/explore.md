# Exploration: worktree-flow repo topologies

> Change slug candidate: `worktree-flow-repo-topology`
> Trigger: lived failure on melon-alquimia — agents ran `git worktree add .worktrees/<slug>` at the superproject root instead of inside the target submodule (`<subrepo>-<slug>` under the shared superproject `.worktrees/`). Prior agent analysis (session paste 2026-07-31) proposed three topologies; this explore validates that analysis against the catalog recipe, sync/materialize machinery, and two real consumer repos.

## Problem

`worktree-flow` v1.2.4 encodes a single-repo mental model:

```bash
git worktree add <worktrees_dir>/<slug> -b <branch> <integration_branch>
```

That is correct for standalone repos and single-git monorepos, and **wrong** for submodule monorepos where each app is its own git repository. The recipe skill, commands, cleanup script, and always-on brief never mention topology, so agents recreate the failure mode every session.

## Current state (grounded)

### Recipe surface (`catalog/recipes/worktree-flow/`)

| Asset | Role today | Topology-aware? |
|---|---|---|
| `recipe.toml` | Config: `worktrees_dir`, `integration_branch`, `auto_remove_merged`, `WORKTREE_GATE_PROTECTED`, `gate_mode` (enum). Brief `workflow_rules` include pre-delegation check. Hook `worktree-gate`. Template `worktree-cleanup.sh` with `condition = "not_exists"`. | No |
| `README.md` | Enable + gate_mode table + config table + cleanup contract | No |
| `commands/worktree-new.md` | Prose steps; no formal arg signature; single `git worktree add` at repo root | No |
| `commands/worktree-clean.md` | Invokes cleanup script with `--dir` / `--base` / `--dry-run` | No |
| `templates/worktree-cleanup.sh` (250 lines) | One `git rev-parse --show-toplevel` → `cd` → one `git worktree list --porcelain`; merge helpers use cwd-scoped `git` (no `--git-dir`/`-C` plumbing) | No |
| `hooks/worktree-gate.sh` | Sync-stamped `gate_mode` via `__WORKTREE_GATE_MODE__`; env override `WORKTREE_GATE_MODE`; protects main worktree on protected branches | Per-git-dir already (see below) |
| `skills/worktree-flow/SKILL.md` | Single create/cleanup convention | No |

### Config validation + sync materialization

- Schema: `[config.*]` fields in `recipe.toml` parsed by `lib/_internal/recipe_schema.py` (`type`, `default`, `enum`, `help_text`).
- Merge + enum reject: `merge_config()` in `lib/_internal/recipe-materialize.py` (~lines 450–490) fails sync on unknown enum values (proven by `tests/test_worktree_flow_recipe.py::test_sync_rejects_invalid_gate_mode`).
- `[[hooks]] action = "validate-config"` re-checks required fields + regex; enums are already enforced in `merge_config`.
- Runtime hook env export: only **UPPER_SNAKE** config keys become hook env (`WORKTREE_GATE_PROTECTED`). Lowercase keys like `gate_mode` / a future `repo_topology` are **not** exported that way.
- `gate_mode` stamping pattern (reusable sibling):
  1. Placeholder `__WORKTREE_GATE_MODE__` in catalog `hooks/worktree-gate.sh`
  2. `GATE_MODE_PLACEHOLDER` + replace inside `materialize_hook_script()` when copying to `ai-specs/recipes/<id>/hooks/`
  3. Runtime: stamped value + optional `WORKTREE_GATE_MODE` env override
  4. Documented in `docs/runtime-hooks.md` ("Config flow") and `docs/ai-specs-toml.md`
- Commands/skills are **byte-copied** (`materialize_command` / `materialize_bundled_skill`) — **no** `{config.KEY}` substitution.
- Brief fragments **do** get `{config.KEY}` via `agents-render.substitute_config`.
- Cleanup template uses `condition = "not_exists"` → existing consumer overrides are **never refreshed** on sync. Melon/venturi already have a materialized copy; stamping topology into that script would not reach them without a refresh policy change.

### Docs / template coverage of worktree-flow keys

| Doc | Covers today |
|---|---|
| `docs/ai-specs-toml.md` | Example `gate_mode = "ask"` only (not full key table) |
| `docs/recipes-catalog.md` | Full worktree-flow config table incl. `gate_mode` |
| `docs/runtime-hooks.md` | Env vs stamped `gate_mode` pattern |
| `docs/recipe-schema.md` | Generic `[config.gate_mode]` enum example |
| `templates/ai-specs.toml.tmpl` | **No** worktree-flow block (only `session-context` enabled by default) |

### Spec already in tree

`openspec/specs/worktree-flow/spec.md` covers merge-detection candidates, dirty skip, bounded (no-fetch) resolution, and **Pre-delegation worktree/branch check** (always-on brief rule). No topology requirements yet.

### Tests already covering worktree-flow

| File | Focus |
|---|---|
| `tests/test_worktree_flow_recipe.py` | Recipe load, gate_mode stamp/default/reject, skill/command/script materialize |
| `tests/test_worktree_cleanup.py` | Merge/squash/rebase/dirty/dual-remote cleanup behavior (single-repo fixtures) |
| `tests/test_worktree_gate_hook.py` | Gate block/allow + stamped modes + env override |
| Also references | `tests/test_init_tui.py` (gate_mode default write), `tests/test_envrc_scaffold.py` (config key list), `tests/test_recipes_catalog.py` (`WORKTREE_GATE_PROTECTED`) |

No submodule / topology fixtures exist today.

### Lived consumer evidence

**melon-alquimia** (`/Users/robert/proyectos/nnodes/melon/melon-alquimia`) — monorepo-submodules:

- `.gitmodules` with 11 entries; name == path for all (e.g. `alquimia-front-web`).
- `git submodule status`: 10 initialized, 1 uninitialized (`-… apis-designv2`).
- Superproject `git worktree list` → **only** the superproject main worktree.
- Inside `alquimia-front-web`, `git worktree list` → main module checkout + linked wts under **superproject** `.worktrees/alquimia-front-web-<slug>`.
- `.git` file in those linked wts points at `.git/modules/alquimia-front-web/worktrees/...`.
- Convention already in use: shared `<superproject>/.worktrees/<subrepo>-<slug>`, created from the submodule repo (not the superproject).
- Also has stray per-submodule `.worktrees/` dirs under some apps (`alquimia-diseno`, `alquimia-apis-design`, …) — mixed historical layout.
- `ai-specs.toml`: `worktree-flow` enabled, `integration_branch = "development"`, `project.subrepos = []` (gitmodules are **not** declared as sync subrepos).

**venturi_coffee** — monorepo-apps:

- No `.gitmodules`; single git toplevel; `apps/` packages (`admin_dashboard_react`, `api-nestjs`, …) via pnpm/turbo.
- Same `worktree-flow` enablement + `integration_branch = "development"`.
- Mechanically identical to standalone for `git worktree add` / cleanup / gate.

## Decision analysis

### 1. Is auto-detect via `.gitmodules` + `git submodule status` sufficient?

**Not sufficient as a sole classifier.**

| Signal | What it proves | Failure mode |
|---|---|---|
| `.gitmodules` present | File exists | Empty/stale file; leftover after submodule removal |
| `git submodule status` non-empty | At least one recorded submodule | **monorepo-apps that vendor deps as submodules** would be misclassified as `monorepo-submodules` and then `/worktree-new` would demand a `<subrepo>` for app work that should stay at root |
| Status line prefix `-` | Uninitialized | Must not offer that path for worktree create until `submodule update --init` |
| Name vs `path=` | Often equal (melon) | Must resolve by **path** (filesystem), accept name only when unique; never assume equality |
| Nested `.gitmodules` inside a submodule | Present in some ecosystems | `git submodule status` (non-recursive) misses them; out of scope for v1 unless `--recursive` is explicit |
| Submodule containing further app folders | Path is still one git repo | Worktrees belong to that submodule repo; inner folders are not separate topologies |

**Overlap with `project.subrepos`:** `lib/_internal/target-resolve.py` already treats `.gitmodules` as **advisory-only** for sync fan-out. Melon leaves `subrepos = []` despite 11 gitmodules. Topology for worktrees and sync subrepos are different axes — do not reuse `project.subrepos` as the submodule source of truth.

### 2. Stamping pattern for a new `repo_topology` key?

Follow **schema + enum validation** from `gate_mode`. Do **not** blindly copy hook stamping:

| Consumer | Needs stamped constant? | Better delivery |
|---|---|---|
| `worktree-gate.sh` | **No** — already correct per git dir (see §5) | Leave alone |
| Brief `workflow_rules` | Optional | `{config.repo_topology}` substitution already works |
| `SKILL.md` / commands | N/A (no substitution today) | Teach agents to read `[recipes.worktree-flow.config]` + branch on value / detect |
| `worktree-cleanup.sh` | **Avoid stamp** | `not_exists` means stamps never reach existing projects; prefer **runtime flags** (`--all-submodules`, optional `--subrepo`) + detection |

If a stamped constant is still desired later (e.g. a small `resolved-topology` sidecar), that is a separate materialize feature — not required for v1 if skill/commands/cleanup read config or detect.

### 3. `/worktree-new <subrepo>` signature

Current command doc has **no positional args** — only prose. Positional-first `<subrepo>` is workable and matches lived melon naming.

Recommended resolution order for `<subrepo>` when topology is `monorepo-submodules`:

1. If cwd is inside an initialized submodule path (or its linked worktree under the shared `.worktrees/`), **infer** that subrepo; explicit arg must match or error.
2. Else require explicit `<subrepo>`.
3. Resolve against `.gitmodules`: match `path` first; then unique submodule name; reject ambiguity.
4. Reject uninitialized paths (status `-`).
5. Create from submodule cwd:

   ```bash
   git -C <subrepo_path> worktree add <superproject>/<worktrees_dir>/<subrepo>-<slug> \
     -b <branch> <integration_branch>
   ```

   Aligns with melon’s existing layout (shared superproject `.worktrees/`, prefixed names).

**Inference caveat (evidence):** `git rev-parse --show-superproject-working-tree` works from the submodule primary checkout, but from a **linked worktree** of that submodule it does not usefully identify the superproject. Inference from feature worktrees must walk to find `.gitmodules` / compare registered submodule paths, not rely on `--show-superproject-working-tree` alone.

**Standalone / monorepo-apps:** `<subrepo>` must be absent (or ignored with a warning). Optional cosmetic `<app>-<slug>` naming for monorepo-apps is documentation-only — same `git worktree add` at root.

### 4. Cleanup: per-submodule iteration

Evidence: there is **no** superproject-wide `git worktree list` that includes submodule worktrees (`git worktree list` only supports `--porcelain`/`-v`/`--expire`; no cross-module flag). Melon root list ≠ submodule wts entirely.

Therefore cleanup for `monorepo-submodules` **must** iterate registered submodule paths and run `git -C <path> worktree list --porcelain` (or `cd` equivalent) for each initialized module.

Merge helpers (`resolve_base_candidates`, `is_merged`, `candidate_has_*`) are **not** submodule-agnostic today — they call bare `git …` against process cwd after `cd "$ROOT"`. Options:

| Approach | Notes |
|---|---|
| **A. Outer loop + `cd` per submodule** (reuse helpers) | Smallest delta; `ROOT`/`WT_PREFIX` recomputed per module; `WT_PREFIX` should still be the shared superproject `<worktrees_dir>/` when using melon layout |
| **B. Plumb `git -C` / `--cwd` through helpers** | Cleaner; more churn in a well-tested script |
| **C. Wrapper command only** (`worktree-clean.md` loops, script unchanged) | Leaves script wrong if invoked directly; weaker |

Recommend **A for v1** (script grows an optional `--submodules` / auto mode that loops), with tests mirroring current merge cases inside a submodule fixture. Keep shared `WT_PREFIX` at superproject `.worktrees/` so `<subrepo>-*` entries owned by each module are visible to that module’s `worktree list` (as they are today on melon).

`not_exists` implication: shipping a better script does not update melon/venturi until override is removed or condition policy changes — call this out in design/tasks (docs note + optional sync WARN / refresh flag).

### 5. Does topology change the gate or the pre-delegation brief?

**Gate:** On melon submodule primary checkout, `git_dir == common_dir` (both `.git/modules/<name>`) → gate blocks writes on `development`. On linked submodule worktrees, `git_dir != common_dir` → allowed. Superproject main checkout gated independently. **No topology-specific gate logic required** for correctness; optional message tweak to mention `/worktree-new <subrepo>` under submodules.

**Brief (`Pre-delegation worktree/branch check`):** Requirement intent stays. Scenarios/text should gain a topology clause: verify *which git repository* (superproject vs submodule path / `rev-parse --show-toplevel`) before dispatch, not only branch + worktree list. Asset: `recipe.toml` `[provides.brief].workflow_rules` (+ mirrored SKILL line). Spec delta on the existing requirement.

### 6. Does `monorepo-apps` need code changes?

**Refute mechanical necessity; confirm naming-only value.**

Evidence: venturi_coffee has one git root, no gitmodules, apps as folders — identical `git worktree add` / cleanup / gate path as standalone (`ai-specs-cli`). The other agent’s “same mechanics as standalone” claim holds.

So `repo_topology = "monorepo-apps"` is a **declared naming convention** (optional `<app>-` prefix in docs/skill) with **zero required behavior change** in scripts/hooks. Keeping the enum value is still useful so projects can self-describe and so agents do not invent a third code path.

## Approaches

| # | Approach | Verdict |
|---|---|---|
| 1 | Document-only in each consumer `AGENTS.md` | Reject as primary — already failed on melon; not reusable |
| 2 | Config `repo_topology` + skill/command branching; cleanup gains submodule loop; **no auto-detect** | Safe; slightly more setup friction |
| 3 | Config with `auto` default: `.gitmodules` w/ initialized entries → `monorepo-submodules`, else `standalone`; `monorepo-apps` **config-only** (never auto) | **Recommend** — low friction for melon, no false “apps” class, misclassification of apps+vendor-submodules mitigated by explicit override |
| 4 | Auto-detect only, no config key | Reject — cannot disambiguate vendored-submodule monorepos; fights `project.subrepos` advisory model |
| 5 | Reuse `project.subrepos` as submodule list | Reject — different axis (sync fan-out); melon has gitmodules with `subrepos = []` |
| 6 | Separate recipe `worktree-flow-submodules` | Reject — splits foundational recipe; gate/cleanup duplication |

### Recommended direction (for proposal; not locked until proposal)

1. Change name: **`worktree-flow-repo-topology`**.
2. Add `[config.repo_topology]` enum: `auto` \| `standalone` \| `monorepo-apps` \| `monorepo-submodules` (default `auto`).
3. Resolve `auto` as: initialized `.gitmodules` entries → `monorepo-submodules`; else `standalone`. Never auto-pick `monorepo-apps`.
4. `/worktree-new`: require/infer `<subrepo>` only in `monorepo-submodules`; create via `git -C <subrepo>` into shared `<worktrees_dir>/<subrepo>-<slug>`.
5. `/worktree-clean` + cleanup script: iterate initialized submodules when topology says so; optional `<subrepo>` scope arg; default = all.
6. `monorepo-apps`: docs/skill naming guidance only.
7. Brief + SKILL: topology table + strengthened pre-delegation “which repo” check.
8. Gate: no functional change (optional stderr hint).
9. Do not stamp topology into the gate; do not rely on stamping the `not_exists` cleanup template — use runtime detection/flags; document override refresh.
10. Tests: extend the three `test_worktree_*.py` files + recipe materialize enum reject; add submodule fixture coverage for create-path docs assertions and cleanup loop.

## Affected files (expected)

| Path | Change |
|---|---|
| `catalog/recipes/worktree-flow/recipe.toml` | `repo_topology` config + brief rule tweak; version bump |
| `catalog/recipes/worktree-flow/README.md` | Topology table + config row |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | Per-topology create/clean matrix |
| `catalog/recipes/worktree-flow/commands/worktree-new.md` | `<subrepo>` signature + resolution rules |
| `catalog/recipes/worktree-flow/commands/worktree-clean.md` | Optional scope; submodule iteration |
| `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` | Submodule loop / flags |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Optional message only (likely) |
| `lib/_internal/recipe-materialize.py` | Only if new stamp/refresh behavior is chosen (not required for enum — schema-driven) |
| `docs/ai-specs-toml.md`, `docs/recipes-catalog.md`, `docs/runtime-hooks.md` (if stamp/refresh noted) | Document key |
| `templates/ai-specs.toml.tmpl` | Commented example for `repo_topology` (optional; tmpl currently omits worktree-flow) |
| `openspec/specs/worktree-flow/spec.md` (via change delta) | Topology + brief scenario extensions |
| `tests/test_worktree_flow_recipe.py` | Enum default/reject/materialize |
| `tests/test_worktree_cleanup.py` | Submodule iteration fixtures |
| `tests/test_worktree_gate_hook.py` | Only if message/behavior changes |
| `tests/test_init_tui.py` / `tests/test_envrc_scaffold.py` | Config key lists if they assert the full set |

## Open questions / risks

1. **Apps + vendored submodules misclassification under `auto`** — residual risk. Mitigation: document explicit `repo_topology = "monorepo-apps"` (or `standalone`) override; consider doctor WARN when `.gitmodules` exists but paths look like `vendor/`/`third_party/` (optional, later).
2. **Cleanup `not_exists` drift** — melon will keep the old script until override deleted. Should design require a one-time refresh strategy (WARN + instructions vs. changing condition)?
3. **Should `/worktree-clean` require `<subrepo>`?** — Recommend **optional scope**, default all initialized submodules (matches multi-app post-merge cleanup on melon).
4. **Shared `.worktrees/` at superproject vs per-submodule `.worktrees/`** — melon uses both historically. Standardize on shared superproject dir (current happy path); mention per-submodule dirs as unsupported/legacy.
5. **Superproject-level changes** — rare on melon; should `/worktree-new` allow an explicit `--superproject` escape hatch, or tell agents to temporarily set `repo_topology = standalone`? Lean: escape hatch flag in command docs.
6. **Nested submodules** — out of scope v1 (non-recursive status).
7. **`integration_branch` per submodule** — melon’s gitmodules declare `branch = development` matching recipe config; divergent per-module defaults not handled. Keep single recipe `integration_branch` unless evidence demands per-path overrides later.
8. **Git caveat** — `git worktree` docs warn that multiple linked worktrees containing submodules are incompletely supported; here worktrees are *of* submodule repos, not worktrees *containing* submodules — different case, but worth a design note.

## Planning depth recommendation

**Classification: `domain_change` → full chain** (`proposal.md` + `design.md` + delta specs under `openspec/changes/<slug>/specs/worktree-flow/` + `tasks.md`).

Justification (per `openspec/specs/sdd-adaptive-contract/spec.md` + `openspec/config.yaml` `sdd.decision_matrix.domain_change`):

- Introduces a **new capability dimension** (repo topology) to a foundational recipe — not a localized fix.
- Cross-cuts schema, agent-facing commands/skill/brief, cleanup behavior, docs, and multiple test modules (**~12–15 files**).
- Needs architectural choices (auto vs config-only, shared `.worktrees/` layout, cleanup refresh vs `not_exists`, inference rules) that belong in `design.md`.
- Extends existing `openspec/specs/worktree-flow` rather than a greenfield spec — still domain-level because behavior and contracts expand across modules.
- `worktree_required: true` for implementation phases; this explore artifact is intentionally written without a worktree per project explore convention.

Not `behavior_change` / tasks-only: enum + docs alone would under-specify cleanup semantics and `<subrepo>` resolution; skipping design would relitigate the auto-detect misclassification risk during apply.

## Ready for proposal?

Yes, with the defaults above as the explore recommendation. Remaining product confirmations for proposal kickoff:

1. Accept `auto` default with explicit override (Approach 3), or prefer config-declared-only (Approach 2)?
2. Confirm shared superproject `.worktrees/<subrepo>-<slug>` as the one supported submodule layout.
3. Confirm cleanup override refresh strategy (`not_exists` drift).
