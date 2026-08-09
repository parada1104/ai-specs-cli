# Changelog

All notable changes to the ai-specs CLI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed
- `plan-build-flow` `1.4.0` → `1.5.0`: adversarial depth classification compares explicit requests with signal tiers, asks on conflicts, and records resolution annotations in `tasks.md`.
- `plan-build-flow` `1.5.0` → `1.6.0`: tier-specific proposal/spec minima, Standard/Full staged verify evidence gates before archive and merge, and grandfathering guidance for in-flight plans.

### Fixed
- Recipe add no longer mutates the manifest when interactive dependencies are unavailable; the dependency gate now runs before writing.

## [0.21.0] — 2026-08-05

### Added
- **Topology-aware worktree gate scope**: `gate_scope = auto | superrepo | subrepo` separates superrepo central-planning writes from protected subrepo production worktrees in proven `monorepo-submodules` layouts, with fail-safe topology validation and stale-hook migration guidance.

## [0.20.1] — 2026-08-05

### Fixed
- Remove untouched legacy recipe command copies from user-managed `ai-specs/commands/` during sync, while preserving customized local commands.

## [0.20.0] — 2026-08-05

### Added
- **Cross-repo worktree artifact scope**: derives central planning artifacts from recognized repository topology while keeping production writes narrowly scoped to the change subtree.
- **Override ownership and lock governance**: records managed provenance, classifies stale versus user-modified overrides, and applies `auto`, `confirm`, and `never-force` update policies with safe migration.

### Changed
- **plan-build-flow 1.4.0**: derives the central planning root from recognized repository topology for submodule worktrees, while preserving standalone nearest-root behavior and narrowly scoped artifact writes.

## [0.19.0] — 2026-08-02

### Added
- **Tracker card gate** (`trello-mcp-workflow` `gate_mode` = `off|warn|always`, default `warn`): dual pre-tool-use hooks (`tracker-card-gate` + `tracker-card-gate-shell`) require a `## Tracker` link section (or `tracker.none`) before production writes and high-confidence `gh pr create` / archive shell actions. Fail-open; never blocks `openspec/**`; does not intercept Trello MCP.
- **Doctor WARN** for active changes missing a valid `## Tracker` section when the recipe is enabled and the bootstrap marker is present.
- Hermetic tests: `tests/test_trello_link.py`, `tests/test_tracker_card_gate_hook.py`, `tests/test_doctor_tracker_card.py`, `tests/test_trello_mcp_workflow_recipe.py`.
- Live eval client: `tests/evals/run-live-trello.sh` + `eval_trello_mcp_workflow_live.py` with four notes-file scenarios.
- **Compact sync output**: `ai-specs sync` and `ai-specs sync-agent` now print
  one `syncing <label>` line per step by default instead of every detail line.
  Warnings, notices, and errors always pass through untouched, and a failing
  step always prints its full unfiltered output regardless of mode. Pass
  `-v`/`--verbose` to restore the previous full detail; it forwards through
  public-root fan-out to nested `sync-agent` invocations. During development of
  this change (before any release), fan-out briefly lost its terminal `exit 0`
  and would have fallen through into an extra silent parent sync pass; that
  pre-release regression was caught and restored so public-root fan-out still
  terminates after children finish.

- **plan-build-flow delivery contract**: recipe declares a per-project
  `artifact_store_default` (string, default `openspec`, enum
  `[openspec, engram, both]`) that materializes as a brief workflow rule via
  `{config.artifact_store_default}` interpolation — consumable by any runtime's
  session preflight as the project default, with zero dependency on external
  preflight packages. Live evals exercise the contract against real runtimes.

### Changed
- `trello-mcp-workflow` `1.2.0` → `1.3.0`: `gate_mode` config, dual hooks, brief anti-bypass / link-before-apply rules; skip hatch replaced by `tracker.none`; Decision #7 narrowed to availability-only degrade.
- `session-bootstrap`: tracker consult is mandatory for new/ambiguous changes when a tracker capability is bound.
- `openspec/config.yaml`: aligned `sdd.decision_matrix` with `sdd-adaptive-contract`; added declarative `tracking:` section.
- `plan-build-flow` `1.2.0` → `1.3.0`: declarative `artifact_store_default` delivery contract in the recipe brief.

## [0.18.0] — 2026-07-28

### Added
- **TTY opt-in CLI install**: when required `[[deps.cli]]` binaries are missing,
  configure/init may offer Homebrew / apt install (explicit confirm; never
  silent). `npx` and `bb` remain guidance-only. Doctor stays check-only.
- **Harness env layout for direnv**: wizard writes project-root `ai-specs.env`,
  generates `ai-specs.env.example`, and ensures a merge-safe managed block in
  project-root `.envrc` (`dotenv_if_exists` for app `.env` + `ai-specs.env`).
  Migrates legacy `ai-specs/.envrc` exports and nested `ai-specs/.env`. Doctor
  WARNs for missing direnv, managed block, or empty harness keys.

### Fixed
- **direnv allow path mismatch**: secrets no longer land only under `ai-specs/`
  while `direnv allow` targeted the project root (vars never loaded from root).
- **Vault filesystem MCP returned empty tool schemas**: `vault-fs-mcp.sh` now pins
  `zod@3` alongside `@modelcontextprotocol/server-filesystem@2025.7.1`. The package
  inherits `zod` from `@modelcontextprotocol/sdk`, which resolves to zod 4, and its
  `zod-to-json-schema@^3` emits `{"$schema": ...}` with no `type`/`properties` for zod
  4 definitions — so hosts that validate schemas rejected the whole `tools/list`
  (`tools[0].inputSchema.type: Invalid input: expected "object"` on Claude Code). No
  change here caused it: `npx -y` re-resolves transitives on every launch, so zod 4's
  release broke a working pin retroactively. The package pin is intentionally kept —
  `2025.7.29+` replaces argv dirs with MCP client roots with no opt-out, which either
  denies a vault outside the workspace or widens the store's scope to cwd + vault.
- **OpenCode runtime-hook matcher is case-insensitive**: generated
  `tool.execute.before` plugins now use the same `"i"` RegExp flag as pi/omp,
  so lowercase OpenCode tool ids match Claude-style recipe matchers.
- **Internal `test-*` recipes fully excluded from consumers**: fixtures moved
  out of the shipped catalog to `tests/fixtures/recipes/`; `recipe add` /
  `recipe init` reject those ids; materialize refuses them unless the test
  suite opt-in env is set.

### Changed
- **Honest runtime-hook coverage docs**: `docs/runtime-hooks.md` documents
  `omp` as a first-class harness, clarifies that pi/omp `tool_call` is
  per-process (not a cross-process subagent guarantee), and notes that
  worktree/plan-build gates must not be the sole guard for delegated work.
- **worktree-flow pre-delegation brief rule**: always-on workflow rule (plus
  skill/README guidance) requires verifying worktree/branch before dispatching
  write-capable subagents/tasks.

## [0.17.0] — 2026-07-24

### Fixed
- **Recipe override boundary completed for trello/worktree templates**:
  `trello-mcp-workflow` card templates and the `worktree-flow` cleanup
  script now materialize under `ai-specs/recipes/{id}/overrides/` so they
  are committable (previously written to gitignored `templates/`/`bin/`
  paths). Projects synced before this release may delete the orphaned
  old-path copies (`ai-specs/recipes/trello-mcp-workflow/templates/`,
  `ai-specs/recipes/worktree-flow/bin/`); they are gitignored and no
  longer used.

## [0.16.0] — 2026-07-23

### Changed
- **Minimal committed project surface**: Bundled skills and helpers stay under the CLI cache (not committed under `ai-specs/skills/`); lock stamp / toml-deps materialization keeps the project tree leaner after sync (#145).

### Fixed
- **Harness literacy pointer**: AGENTS Useful Commands no longer claim harness skills live under `ai-specs/skills/`; harness-lifecycle docs match cache-flatten / no `.new` sidecars (#146).
- **Tracked bundled leftovers**: Sync and doctor WARN when git still tracks removed bundled skill paths, with `git rm --cached` remediation — CLI never mutates the index (#146).

## [0.15.0] — 2026-07-23

### Added
- **Always-on harness CLI literacy**: Bundled playbooks for CLI lifecycle, recipes, and skills/deps, plus a fixed AGENTS.md Useful Commands pointer so agents can operate the public CLI without relying on README/help alone.
- **Playwright UI flow + MCP add-on**: `ui-browser-testing` base CLI/smoke recipe plus optional `@playwright/mcp` surface for agent UI verification.
- **vault-canonical-store 1.2.0**: Vendors kepano Obsidian skills (`obsidian-markdown`, `obsidian-bases`, `json-canvas`, `obsidian-cli`, `defuddle`) as recipe dep skills; refreshes `vault-context` cross-links and README for env-owned `CANONICAL_VAULT_PATH` (including spaced iCloud paths). Dry + live eval client (`./tests/evals/run-live-vault.sh`).
- **Plan-build pre-tool-use artifact gate**: Non-bypassable hook blocking production edits until an active change folder exists (with eval coverage).
- **Live-eval runtime hook wiring**: Harness wires `[[provides.hooks]]` into the runtime channel so gate scenarios test end-to-end.
- **VCS post-merge base-sync rule**: Surfaced across git/gitlab/bitbucket recipe briefs so agents sync the integration branch after a merged PR/MR.

### Fixed
- **omp native brief slot**: Routes the runtime brief through `.omp/AGENTS.md` (omp's highest-priority provider slot) instead of relying only on the root standalone AGENTS.md.
- **Vault MCP path via env (not `${VAR}` argv)**: `vault-canonical` now runs `vault-fs-mcp.sh`, which reads absolute `CANONICAL_VAULT_PATH` from the process env at exec time. Avoids hosts that leave a bare `"${CANONICAL_VAULT_PATH}"` argv unexpanded (the `~/${path}` workaround class of bugs).
- **OpenCode MCP env substitution**: MCP command args use `{env:VAR}` so OpenCode expands them at config load.
- **Recipe dep skill import path**: `materialize_dep_skill` ensures `lib/_internal` is on `sys.path` so `vendor-skills` can import `skill_contract` when loaded via importlib.

## [0.14.2] — 2026-07-17

### Fixed
- **Minimal project surface on sync**: Shared helpers like `premerge_guardian.py` stay under `$AI_SPECS_HOME` (not copied into `ai-specs/bin/`). Sync removes leftover in-project skill-cache dirs (`.resolved-skills/`, `.internal/`) and stale bin copies, and refreshes the root agent `.gitignore` block so existing projects pick up `.pi/` / `.omp/`.
- **Hide `test-*` catalog fixtures**: Hub, CLI recipe list, and related user-facing surfaces no longer show internal `test-*` catalog recipes.

## [0.14.1] — 2026-07-17

### Added
- **Local `release-flow` skill**: Dogfooded playbook for bump → `development`, promote via disposable `release/v*` → `main`, tag + GitHub release (product policy kept out of `vcs-pr-flow`).
- **VCS behavior evals**: Live/dry scenarios for git/gitlab/bitbucket PR flow, including protected-head and preferred release-head ACs; `cursor-agent` as a first-class eval runtime.

### Fixed
- **Protected merge heads**: Merge cleanup no longer deletes long-lived heads (`development`/`main`/`staging`); prefer `release/v*` into `main`; GitHub `delete_branch_on_merge` preflight before promote.

## [0.14.0] — 2026-07-16

### Added
- **CLI-bound recipes + off-project origin cache**: Enabled recipes bind to the installed CLI catalog (no per-recipe version pins). Origin staging (`.recipe` / `.deps` / managed commands) moves under `$AI_SPECS_HOME/cache/projects/<key>/`; project keeps `ai-specs.toml`, local skills, and recipe docs/overrides.
- **Plan-build multi-runtime evals + pre-merge guardian**: Eval harness coverage for plan-build flow; VCS pre-merge guardian template improvements.
- **VCS auth preflight (multi-account)**: `expected_owner` / `auto_switch_account` config on git-pr-flow, gitlab-mr-flow, and bitbucket-pr-flow so agents pick the right CLI account before PR/MR work.
- **Catalog config `help_text`**: Wizard shows how-to-get guidance for fields like `board_id`, `integration_branch`, `base_branch`, vault paths, and test commands.
- **MCP env var help map**: `ENV_VAR_HELP` for `TRELLO_API_KEY`, `TRELLO_TOKEN`, and `CANONICAL_VAULT_PATH` (prompt + `.envrc.example` comments with links).

### Fixed
- **`configure-recipes` / hub crash on MCP env prompts**: `questionary.text(..., password=)` is invalid in questionary 2.x; secrets now use `questionary.password`. `_offer_envrc` soft-fails instead of aborting the wizard.
- **Config `type = "boolean"`**: Schema normalizes to `"bool"` so the wizard uses confirm prompts (catalog recipes aligned).

## [0.13.1] — 2026-07-13

### Added
- **Hub: Recipes submenu + `recipe add` con wizard integrado**: Recipes ahora es submenu interactivo en el hub (list/add/remove/configure/back). `recipe add` en TTY pregunta si configurar ahora, corre el config wizard, pide MCP env vars, escribe `.envrc` y ejecuta `direnv allow` automaticamente.
- **Hub: configuracion de Agents**: Nueva opcion en el menu para seleccionar que agentes habilitar via checkbox.
- **`recipe remove`**: Nuevo comando para eliminar recipes del manifest. Limpia automaticamente el `.ai-specs.lock`.
- **MCP env vars interactivo**: Las variables de entorno (TRELLO_API_KEY, TRELLO_TOKEN, etc.) se piden interactivamente con masking. Se escriben directo a `.envrc` (no `.envrc.example`).
- **Recipe config wizard + CLI deps + `.envrc.example`**: `[[deps.cli]]` schema on recipes; `dep_check.py` + Doctor WARN for missing required CLIs; hub action **Configure recipes** / `ai-specs configure-recipes`; surgical `[recipes.<id>.config]` write-back; init step 3.5 collects config after recipe selection; generates `ai-specs/.envrc.example` from MCP env refs (never writes `.envrc`).
- **TUI hub** front door: bare `ai-specs` (and `ai-specs hub [path]`) opens an interactive status + command menu when a project is initialized on a TTY. Non-TTY prints a dep-free status summary; uninitialized no-TTY exits 2; uninitialized TTY offers init. Shared `lib/_internal/util.py` deps gate; rich+questionary pre-vendored under `lib/_vendor/`.

### Changed
- **Hub dispatch fix**: `ai-specs recipe list <path>` y `ai-specs skills list <path>` ahora funcionan correctamente desde el hub.
- **Lock cleanup en sync**: `clean_orphans` ahora limpia stale lock entries al correr `sync`.
- **Env vars scaffolding**: Reemplazado `.envrc.example` por escritura directa de `.envrc` con `direnv allow` automatico.

### Removed
- Acciones separadas "Configure recipes" y "Remove recipe" del menu principal del hub — ahora integradas en submenu Recipes.

## [0.12.4] — 2026-07-12
### Changed
- **Hub dispatch fix**: `ai-specs recipe list <path>` y `ai-specs skills list <path>` ahora funcionan correctamente desde el hub.
- **Lock cleanup en sync**: `clean_orphans` ahora limpia stale lock entries al correr `sync`.
- **Env vars scaffolding**: Reemplazado `.envrc.example` por escritura directa de `.envrc` con `direnv allow` automatico.

### Removed
- Acciones separadas "Configure recipes" y "Remove recipe" del menu principal del hub — ahora integradas en submenu Recipes.

## [0.12.4] — 2026-07-12
### Changed

- **TUI upgraded to Questionary interactive prompts** — agent and recipe
  selection now uses arrow keys + space toggle (checkboxes) instead of
  typing numbers. Project name uses a text input with inline editing.
  Confirm prompt uses y/n with default yes.

### Added

- `install.sh` and `upgrade.sh` now auto-install `rich` + `questionary`
  into `lib/_vendor` as step [2/3] of the install/upgrade flow.
  No manual pip install needed — the TUI works out of the box on fresh installs.
- Empty recipe catalog guard: skips the recipe checkbox instead of rendering
  an empty selectable list.

### Removed

- `_parse_selection` function and its 4 unit tests (dead code after the
  questionary migration replaced text-based selection with checkboxes).

### Fixed

- `upgrade.sh`: TUI deps refresh now runs before the "already up to date"
  early exit — users on latest code with missing deps can recover via
  `ai-specs upgrade`.
- Stale `_ensure_rich` comment updated to `_ensure_deps`.

## [0.12.3] — 2026-07-12

### Added

- **Interactive onboarding TUI** for `ai-specs init` (`--tui` / auto on TTY;
  `--no-tui` keeps classic path). Rich-based wizard selects project name,
  agents, and catalog recipes. Soft-fails to classic init when Rich unavailable.
  Cancel (Confirm 'n', Ctrl-C, Ctrl-D/EOF) exits cleanly without writing a manifest.
- **Recipe tags and conflict detection** (`tags` + `conflicts_with` fields in
  `recipe.toml`). `ai-specs sync` surfaces tag conflicts as advisory warnings.
  Catalog recipes tagged by domain.
- **CLI version pinning** via optional `[tool]` section in `ai-specs.toml`
  (`version` + `policy = "exact"`, or `min_version` + `policy = "min"`).
  Lock `[meta]` records `cli_version` and `synced_at` on sync.
  `ai-specs doctor` reports installed, pinned, and last-synced version.
  `--ignore-cli-version` escape hatch for bypassing pins.
- **Worktree-flow gate modes** (`always` / `ask` / `off`) configurable per project.
- **Plan-build-flow recipe** — two-verb (`/plan`, `/build`) catalog recipe over
  the existing multi-phase change ceremony. Ambient skill-only v2 workflow.
- **VCS pre-merge archive rule** — SDD/OpenSpec artifacts MUST be archived
  before merge; mirrored into GitHub, GitLab, and Bitbucket merge-workflow skills.
- **Post-merge branch cleanup** codified in the GitHub merge-workflow skill
  (worktree removal + branch deletion after squash merge).
- **Recipe eval harness** — opt-in behavior eval for recipes (runtime-level
  verification beyond materialization tests).

### Changed

- `plan-build-flow` v2 is **breaking** for existing `/plan-build-flow` users:
  the skill-only ambient workflow replaces the earlier multi-command ceremony.

### Fixed

- Tag dedup hardening: blank tag values rejected; dedup is order-preserving.

### Migration notes

- Existing projects without `[tool]` behave as before; run `ai-specs sync` once
  to populate lock `[meta]`.
- To pin production projects, add:
  ```toml
  [tool]
  version = "0.12.3"
  policy = "exact"
  ```
  after upgrading the global CLI to that version.

## [0.12.2] — 2026-06-23

Baseline reference for projects already on production tooling. Includes recipe
version pinning, upgrade command, doctor diagnostics, and bundled lock tracking.