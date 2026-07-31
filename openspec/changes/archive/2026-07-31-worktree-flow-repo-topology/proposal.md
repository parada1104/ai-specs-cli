# Proposal: worktree-flow repo topologies

## Intent

`worktree-flow` encodes a single-repo mental model:

```bash
git worktree add <worktrees_dir>/<slug> -b <branch> <integration_branch>
```

That is correct for standalone repos and single-git monorepos (`monorepo-apps`),
and **wrong** for submodule monorepos: each app is its own git repository, and
lived melon-alquimia layout already creates linked worktrees as
`<superproject>/<worktrees_dir>/<subrepo>-<slug>` from the **submodule** repo.
Agents keep recreating the failure (worktree-add at the superproject root)
because the recipe skill, commands, cleanup script, and always-on brief never
mention topology.

This change adds an explicit `repo_topology` dimension so `/worktree-new`,
`/worktree-clean`, skill/brief guidance, and (where needed) sync/doctor
diagnostics branch correctly across `standalone`, `monorepo-apps`, and
`monorepo-submodules`.

Exploration: `openspec/changes/worktree-flow-repo-topology/explore.md`.

## Modes / topologies

| Topology | `git worktree add` target | Cleanup enumeration | Mechanical notes |
|----------|---------------------------|---------------------|------------------|
| `standalone` | repo root → `<worktrees_dir>/<slug>` | one `git worktree list` | Today's behavior |
| `monorepo-apps` | same as standalone | same as standalone | Naming-only (optional `<app>-` prefix in docs/skill); venturi_coffee evidence |
| `monorepo-submodules` | `git -C <subrepo_path>` → `<super>/<worktrees_dir>/<subrepo>-<slug>` | iterate initialized submodules | Requires `<subrepo>` (infer or explicit) |
| `auto` (default) | resolve then dispatch | resolve then dispatch | Initialized `.gitmodules` → `monorepo-submodules`; else `standalone`; **never** auto-picks `monorepo-apps` |

## Scope

### In scope

1. **`repo_topology` config** — enum `auto` \| `standalone` \| `monorepo-apps` \| `monorepo-submodules`, default `auto`, schema-validated like `gate_mode`.
2. **Auto-detect + wizard surfacing + hub/status surfacing** (user decision 1, verbatim):
   > 1 si, el auto debería ser default idealmente en el wizard quizá avisarle al usuario porque creo que se configura como el nombre del proyecto, ahi mismo podemos pedirle confirmar si su proyecto es standalone, monorepo-app o git module, también podemos mostrarlo en el hub en la parte de arriba
3. **Single shared layout at configurable `worktrees_dir`** (user decision 2, verbatim):
   > 2 en melon creo que no hay worktrees sueltos por submodulo pero si, la idea es que vivan en .worktree en la raiz del monorepo o una ruta relativa configurada(como hoy en día) pero bueno el default sigue siendo eso
   - Supported layout: `<superproject>/<worktrees_dir>/<subrepo>-<slug>` (default `worktrees_dir = ".worktrees"`).
   - Per-submodule `.worktrees/` dirs are unsupported/legacy.
4. **`not_exists` drift → refresh/WARN** (user decision 3, verbatim):
   > 3 si
   - Keep cleanup template `condition = "not_exists"` (do not silently overwrite consumer overrides).
   - When catalog cleanup script differs from an existing materialized override, emit a non-blocking **sync WARN** (and optionally a doctor WARN) with explicit refresh instructions.
5. **`/worktree-new`** — `<subrepo>` require/infer only under resolved `monorepo-submodules`; locked `git -C` create contract (see Verified git command contract).
6. **`/worktree-clean` + cleanup script** — iterate initialized submodules when topology resolves to `monorepo-submodules`; optional `<subrepo>` scope (default = all).
7. **Brief + SKILL** — topology table; strengthen pre-delegation check to verify *which git repository* (`rev-parse --show-toplevel`), not only branch + worktree list.
8. **Docs** — `docs/recipes-catalog.md`, `docs/ai-specs-toml.md` (+ notes as needed); recipe README.
9. **Tests** — enum default/reject/materialize; submodule fixture coverage for cleanup loop and create-path contract docs/assertions.

### Out of scope

- Changing `worktree-gate.sh` decision logic (already correct per git-dir; optional stderr hint only).
- Stamping `repo_topology` into the gate or into the `not_exists` cleanup template as the delivery mechanism.
- Nested/recursive submodules (`git submodule status --recursive`).
- Per-submodule `integration_branch` overrides.
- Reusing `project.subrepos` as the submodule source of truth (different axis: sync fan-out).
- Auto-classifying `monorepo-apps`.
- Supporting per-submodule `.worktrees/` layouts.
- Creating/deleting worktrees or branches in consumer repos as part of this planning change.
- Rewriting cleanup merge helpers to plumb `git -C` everywhere (v1 prefers outer loop + `cd` / scoped ROOT reuse).

## Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| `worktree-flow` | **Modified** | Add `repo_topology`; topology-aware `/worktree-new`, `/worktree-clean`, skill/brief; submodule cleanup iteration; shared `<worktrees_dir>/<subrepo>-<slug>` layout |
| `project-doctor` / sync materialize diagnostics | **Modified** | WARN when `not_exists` cleanup override is stale vs catalog |
| Init TUI / hub status surfaces | **Modified** | Detect + confirm topology near project identity; show resolved topology in hub/status header |

## Approach

1. **Schema** — add `[config.repo_topology]` to `catalog/recipes/worktree-flow/recipe.toml` (enum + default `auto` + help_text). Rely on existing `merge_config()` enum reject; no new stamp plumbing required for the key itself.
2. **Resolve `auto`** — initialized `.gitmodules` entries (via `git submodule status`, skip `-` uninitialized) → `monorepo-submodules`; else `standalone`. `monorepo-apps` is config-declared only.
3. **Create path (`monorepo-submodules`)** — resolve `<subrepo>` (cwd inference → explicit arg → `.gitmodules` path then unique name); reject uninitialized; then run the locked `git -C` contract with an **absolute** destination under the superproject `worktrees_dir`.
4. **Cleanup** — when resolved topology is `monorepo-submodules`, outer-loop initialized submodule paths and run existing merge/cleanup helpers per module with shared superproject `WT_PREFIX=<super>/<worktrees_dir>/`. Optional `--subrepo` / positional scope; default all.
5. **Wizard** — after project-name prompt in `init_tui.py`, detect + confirm topology; write `recipes.worktree-flow.config.repo_topology` into the staged `ai-specs.toml` (also available later via `config_wizard` enum select when configuring the recipe).
6. **Hub/status** — extend `StatusPanel` / noninteractive status header to show resolved topology (no dedicated “hub header” concept exists today beyond that panel).
7. **Brief** — extend `_section_project` (and/or worktree-flow `workflow_rules`) so agents see topology alongside project name / integration branch.
8. **Stale cleanup WARN** — on sync (and optionally doctor), if materialized cleanup override exists and differs from catalog template, WARN with refresh steps; do not auto-overwrite under `not_exists`.
9. **Gate** — leave functional behavior unchanged.

## Verified git command contract (melon-alquimia, read-only)

Repo: `/Users/robert/proyectos/nnodes/melon/melon-alquimia`  
Probe submodule: `alquimia-front-web`  
Linked worktree sample: `.worktrees/alquimia-front-web-backoffice-usuarios`  
**No worktrees/branches were created or deleted.**

### Results

| Probe | Context | Observed |
|-------|---------|----------|
| `git worktree list` | superproject root | **Only** the superproject main worktree — submodule linked wts are invisible |
| `git -C alquimia-front-web worktree list` | from superproject root | Same list as `cd alquimia-front-web && git worktree list` — primary + linked wts under super `.worktrees/alquimia-front-web-*` |
| `cd` into submodule + `git worktree list` | submodule primary | Identical to `-C` form |
| `git rev-parse --show-superproject-working-tree` | submodule **primary** (`-C` or `cd`) | Returns superproject path; exit 0 |
| `git rev-parse --show-superproject-working-tree` | **linked** wt under `.worktrees/<subrepo>-…` | **Empty** stdout; exit 0 — **do not** rely on this for inference from feature worktrees |
| `git rev-parse --show-toplevel` | submodule primary | `…/melon-alquimia/alquimia-front-web` |
| `git rev-parse --show-toplevel` | linked wt | `…/melon-alquimia/.worktrees/alquimia-front-web-<slug>` |
| `git submodule foreach 'git worktree list'` | superproject | Correctly lists each initialized submodule’s worktrees (including shared `.worktrees/<subrepo>-*` entries; also shows rare legacy per-submodule `.worktrees/` e.g. `alquimia-diseno`) |

### Locked create command

`git -C <subrepo_path> …` from the **superproject root works as-is** — **no `cd` / pushd-popd required** for list or (by the same `-C` mechanism) add.

**Absolute destination is mandatory** when using `-C`: with `git -C <subrepo_path>`, relative paths resolve relative to the submodule directory, so
`worktree add .worktrees/<subrepo>-<slug>` would incorrectly create **inside** the submodule. Prefer:

```bash
git -C <subrepo_path> worktree add \
  <superproject_abs>/<worktrees_dir>/<subrepo>-<slug> \
  -b <branch> <integration_branch>
```

Equivalent relative climb (`../.worktrees/…`) only works when `worktrees_dir` is exactly `.worktrees` at the superproject root; absolute path honors any configured `worktrees_dir`.

### Locked cleanup enumeration

```bash
git submodule foreach --quiet 'git worktree list --porcelain'
# or equivalent explicit loop:
#   for each initialized submodule path: git -C <path> worktree list --porcelain
```

Never use superproject `git worktree list` as the sole source of candidates under `monorepo-submodules`.

### Inference rule (from verified caveat)

- From submodule primary: `--show-superproject-working-tree` is usable.
- From linked feature worktrees: walk upward / match registered `.gitmodules` paths against `rev-parse --show-toplevel` (and/or compare to known `<worktrees_dir>/<subrepo>-*` prefixes). Do **not** depend on `--show-superproject-working-tree` alone.

## Wizard + hub/status integration points

### Wizard (project identity)

| Touch point | File | Exact locus | Integration |
|-------------|------|-------------|-------------|
| **Primary — project identity** | `lib/_internal/init_tui.py` | Step 1 “Project name:” (`questionary.text`, ~L239–244) inside `run_wizard` | **New question node immediately after project name**: run topology auto-detect; present select/confirm (`standalone` / `monorepo-apps` / `monorepo-submodules`, with `auto`/detected default); persist as `recipes.worktree-flow.config.repo_topology` in `_render_manifest` / staged `ai-specs.toml` when worktree-flow will be enabled (or always record intent for later enable). Matches user “ahi mismo” next to project name. |
| Recipe config enum | `lib/_internal/config_wizard.py` | `run_config_wizard` enum branch (`questionary.select` over `field.enum`, ~L100–108) | Once `repo_topology` exists in recipe schema, configuring `worktree-flow` (init step 3.5 `_configure_recipes` or `ai-specs` configure) already prompts the enum — keep help_text clear; init identity prompt remains the human-facing confirm for first-time setup. |

### Hub / status header

There is **no** existing single “hub header” that prints project name today.

| Surface | File | Exact locus | Verdict |
|---------|------|-------------|---------|
| Interactive hub top panel | `lib/_internal/hub.py` | `StatusPanel.render` (~L311–341) — Rich `Panel(title="ai-specs")` with rows `version`, `target`, `Summary:` | **Most natural existing surface** — add row(s) for project name (from manifest) + **resolved** `repo_topology` (and maybe `worktrees_dir`). This is the “parte de arriba” of the hub. |
| Noninteractive status | `lib/_internal/hub.py` | `_run_noninteractive` (~L214–221) — `ai-specs status — {headline}` | Mirror topology on the status banner / detail lines for scripts/CI humans. |
| Agent brief Project section | `lib/_internal/agents-render.py` | `_section_project` (~L174+) — already emits **Project** + **Integration branch** from worktree-flow config | Secondary agent-facing surface: add **Repo topology** (resolved) here so sessions see it without opening the hub. |
| `hooks-render.py` | `lib/_internal/hooks-render.py` | Hook shim generation only | **Not** a status/header surface — do not use. |

## Affected Areas

| Area | Impact | Description |
|------|--------|-------------|
| `catalog/recipes/worktree-flow/recipe.toml` | Modified | `repo_topology` config; brief rule tweak; version bump |
| `catalog/recipes/worktree-flow/README.md` | Modified | Topology table + config row + layout contract |
| `catalog/recipes/worktree-flow/skills/worktree-flow/SKILL.md` | Modified | Per-topology create/clean matrix + `git -C` contract |
| `catalog/recipes/worktree-flow/commands/worktree-new.md` | Modified | `<subrepo>` signature + resolution + locked create command |
| `catalog/recipes/worktree-flow/commands/worktree-clean.md` | Modified | Optional scope; submodule iteration |
| `catalog/recipes/worktree-flow/templates/worktree-cleanup.sh` | Modified | Submodule loop / flags; shared `WT_PREFIX` |
| `catalog/recipes/worktree-flow/hooks/worktree-gate.sh` | Optional | Message-only hint for `/worktree-new <subrepo>` |
| `lib/_internal/init_tui.py` | Modified | Detect + confirm topology after project name; write config key |
| `lib/_internal/config_wizard.py` | Modified | Picks up new enum via schema (help_text); no special-case required unless init writes before recipe configure |
| `lib/_internal/hub.py` | Modified | StatusPanel + noninteractive status show resolved topology |
| `lib/_internal/agents-render.py` | Modified | `_section_project` surfaces resolved topology |
| `lib/_internal/recipe-materialize.py` | Modified | Sync WARN when `not_exists` cleanup override is stale vs catalog |
| `lib/_internal/doctor.py` | Optional Modified | Mirror stale-cleanup WARN |
| `docs/ai-specs-toml.md`, `docs/recipes-catalog.md` | Modified | Document `repo_topology` + layout |
| `templates/ai-specs.toml.tmpl` | Optional | Commented `repo_topology` example |
| `openspec/specs/worktree-flow/spec.md` (via delta) | Modified | Topology + brief “which repo” scenarios |
| `tests/test_worktree_flow_recipe.py` | Modified | Enum default/reject/materialize |
| `tests/test_worktree_cleanup.py` | Modified | Submodule iteration fixtures |
| `tests/test_worktree_gate_hook.py` | Only if message changes | Optional |
| `tests/test_init_tui.py`, hub/doctor tests | Modified | Wizard confirm + status surfacing + WARN |

## Risks

| Risk | Likelihood | Mitigation |
|------|------------|------------|
| `auto` misclassifies apps+vendored-submodules as `monorepo-submodules` | Medium | Default still confirmable in wizard; document explicit `monorepo-apps` / `standalone` override; never auto-pick `monorepo-apps` |
| Existing melon/venturi keep stale cleanup via `not_exists` | High (expected) | Sync/doctor WARN + refresh instructions; do not silent-overwrite |
| Inference from linked worktrees using `--show-superproject-working-tree` fails | High if naive | Locked contract: that probe is empty on linked wts; walk `.gitmodules` / path prefix instead |
| Relative `worktree add` path under `git -C` lands inside submodule | High if undocumented | Mandate absolute `<super>/<worktrees_dir>/…` in command/skill |
| Legacy per-submodule `.worktrees/` dirs ignored | Low | Document unsupported; melon happy path is shared super dir |
| Hub panel clutter | Low | One compact row: `topology  monorepo-submodules (auto→…)` |

## Rollback Plan

1. Revert the change branch / PR (recipe version, init/hub/materialize diagnostics, docs, tests).
2. Manifests without `repo_topology` keep today's standalone behavior via default/`auto`→standalone when no gitmodules — or treat missing key as `standalone` on rollback.
3. Consumer cleanup overrides under `not_exists` remain untouched (WARN-only path is non-destructive).
4. No data migration; no force-refresh of templates on rollback.

## Dependencies

- Existing `worktree-flow` recipe (≥ v1.2.4) and `gate_mode` enum/validation pattern.
- Git ≥ recipe `min_version` (`2.20.0`) with `worktree` + `submodule foreach`.
- Lived layout evidence: melon-alquimia (submodules) + venturi_coffee (apps, naming-only).

## Success Criteria

- [ ] `repo_topology` enum accepted/rejected by sync like `gate_mode`; default `auto`.
- [ ] `auto` resolves to `monorepo-submodules` only when initialized gitmodules exist; never auto-selects `monorepo-apps`.
- [ ] Init wizard confirms topology next to project name and writes `recipes.worktree-flow.config.repo_topology` into `ai-specs.toml`.
- [ ] Hub `StatusPanel` and noninteractive status surface the **resolved** topology; brief `_section_project` does too.
- [ ] `/worktree-new` under `monorepo-submodules` uses:
      `git -C <subrepo_path> worktree add <super_abs>/<worktrees_dir>/<subrepo>-<slug> -b <branch> <integration_branch>`
      with no required `cd`, absolute destination, and verified `<subrepo>` resolution rules.
- [ ] `/worktree-clean` / cleanup script enumerates via per-submodule `git -C` / `submodule foreach`, not superproject `worktree list` alone.
- [ ] Shared `<worktrees_dir>/<subrepo>-<slug>` at superproject root is the only supported submodule layout (`worktrees_dir` configurable; default `.worktrees`).
- [ ] Stale `not_exists` cleanup override produces sync (and/or doctor) WARN with refresh guidance; no silent overwrite.
- [ ] Gate behavior unchanged aside from optional message.
- [ ] `monorepo-apps` documented as naming-only (same git mechanics as standalone).
- [ ] Tests cover enum, cleanup submodule loop, wizard/status surfacing, and WARN path.
- [ ] `./tests/validate.sh` passes.

## Planning depth

**Classification: `domain_change` → full chain** (`design.md` + delta specs under
`openspec/changes/worktree-flow-repo-topology/specs/worktree-flow/` + `tasks.md`)
after this proposal. Implementation remains `worktree_required: true`; this proposal
artifact is planning-only (no worktree, no code apply).
