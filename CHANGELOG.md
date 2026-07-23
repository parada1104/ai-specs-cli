# Changelog

All notable changes to the ai-specs CLI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
