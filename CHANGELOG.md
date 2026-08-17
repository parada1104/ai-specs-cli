# Changelog

All notable changes to the ai-specs CLI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- **Version-keyed upgrade notices**: a release can declare a required
  post-upgrade action in an `### Upgrade notes` subsection under its
  `CHANGELOG.md` heading. `ai-specs upgrade` replays the notices of every
  version the user crossed, oldest release first, under **Action required**.
  Notices are prose and are never evaluated or executed: `upgrade` runs against
  `~/.ai-specs` and has no consumer project in scope, so anything
  project-dependent stays with `ai-specs doctor`, which has that state.
- **Version crossing summary**: after a successful upgrade, the versions
  crossed are summarized with up to three condensed bullets each (first
  sentence, capped at 100 characters) and an explicit "and N more" rather than
  a silent truncation.
- **Narrowed global install**: `~/.ai-specs` is now a partial clone
  (`--filter=blob:none`) with a cone-mode sparse checkout that excludes
  `openspec/`, `tests/`, `.github/` and `tmp/` — 1842 tracked files down to 958,
  with every runtime path intact. Full commit history is preserved, because
  `ai-specs upgrade` needs `git merge-base --is-ancestor` for its divergence
  guard and a shallow clone would break it. Narrowing is best effort: an
  unsupported git, a dirty tree, or any failure leaves a usable full checkout
  and the upgrade still succeeds. `git -C ~/.ai-specs sparse-checkout disable`
  restores every file.

### Changed
- `ai-specs upgrade` no longer forwards raw `git` output. It prints one labelled
  line per step, adopting the `run_step` contract already used by `ai-specs
  sync`. `-v`/`--verbose` restores the full detail, and a failing step always
  prints everything it produced. Upgrading `0.20.0` to `0.22.0` went from ~250
  lines to 21. No safety check, abort condition, or exit code changed.
- `release-flow` skill: authoring an upgrade notice is now part of the version
  bump, and the tag/release step reflects that CI creates the GitHub Release
  (`softprops/action-gh-release`), so `gh release create` fails with "already
  exists" — the ritual now uses `gh release edit`.

## [0.22.0] — 2026-08-17

### Upgrade notes
Run `ai-specs sync` in each project to acquire the verified Go worktree-gate
binary. Until you do, the gate keeps falling back to the Bash implementation.
Run `ai-specs doctor` to confirm the resolved implementation; if it reports a
preserved customized gate, use `ai-specs sync --refresh-gates`.

### Added
- **Autocontained Go worktree gate**: the `worktree-flow` gate is now a single
  zero-dependency Go binary (`catalog/recipes/worktree-flow/gate`) with
  byte-for-byte behavioral parity against the frozen Bash reference
  (`hooks/worktree-gate-legacy.sh`), proven by a hermetic 16-case parity corpus
  that runs **both** implementations and asserts identical exit code, stderr
  and candidates. The frozen Bash reference remains the rollback path for one
  minor release (`gate_impl = "bash"`).
- **Thin launcher with a stable materialized path**: `hooks/worktree-gate.sh`
  is now a bash-3.2-safe resolver that keeps the materialized path
  `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh` unchanged, so all
  five harnesses (claude, cursor, opencode, pi, omp) keep working with zero
  renderer changes and zero re-render churn. Resolution order:
  `$WORKTREE_GATE_BIN` → project-local pin → version-keyed cache → frozen Bash
  reference → one stderr warning + fail open. Handoff is `exec` (stdin and
  exit code untouched).
- **`gate_impl` configuration** (`auto | go | bash`, default `auto`) on the
  `worktree-flow` recipe: `auto` prefers the Go binary and falls back to Bash;
  `go` uses only the binary and fails open when unusable; `bash` needs no
  binary, network, or Go toolchain. The resolved value is validated and
  stamped at sync like `gate_scope` / `repo_topology`.
- **Binary acquisition, verification and cache**: `ai-specs sync` acquires the
  host-platform gate binary into
  `$AI_SPECS_HOME/cache/bin/worktree-gate/<cli-version>/<goos>-<goarch>/`,
  verifying SHA-256 against the committed
  `catalog/recipes/worktree-flow/bin/SHA256SUMS` trust root before install
  (atomic `os.replace`, mode 0755, self-test), and degrading with a warning on
  any failure — acquisition never fails sync. An opt-in local build
  (`AI_SPECS_GATE_BUILD=1`, or offline with a Go toolchain) writes into the
  same cache layout; a Go toolchain is never a user prerequisite.
- **`worktree-gate` doctor check**: reports the resolved implementation,
  binary version, digest state, and any silent fallback (OK / INFO / WARN /
  ERROR per design §6.5), including the "gate is silently failing open" ERROR.
- **Multi-arch build matrix and reproducibility**: `scripts/build-gate.sh`
  builds `darwin/arm64`, `darwin/amd64`, `linux/amd64`, `linux/arm64` with
  `CGO_ENABLED=0`, `-trimpath`, `-buildvcs=false` and the CLI version injected
  at link time; repeated builds are byte-identical. A CI release workflow
  builds all four targets on tag push, runs `go vet` + `go test ./...`, emits
  `SHA256SUMS`, diffs it against the committed digests, and attaches the
  assets to the release.
- **Performance**: the Go gate runs a single process per invocation with
  memoized Git facts (~3× faster than the Bash reference; measured 48.5 ms vs
  145.3 ms median over the corpus) and issues strictly fewer `git` invocations
  for multi-candidate events.
- **Subrepo planning context propagation**: one ai-specs request context
  (code/VCS owner, explicit fan-out target set, canonical superrepo planning
  root) now flows through `target-resolve.py`, `sync.sh` / `sync-agent.sh`,
  render metadata, `plan-build-flow`, and `premerge_guardian.py`. A subrepo
  request resolves via `show-toplevel` / validated `.gitmodules`, owns its
  worktree at `<super>/.worktrees/<subrepo>-<slug>`, and keeps planning under
  `<super>/openspec/changes/<slug>/`; `project.subrepos` stays authoritative
  and `monorepo-apps` stays explicit.
- **Conservative, reversible gate refresh**: the last CLI-rendered gate bytes
  are recorded as provenance in the lock/cache. A baseline match updates in
  place; a mismatch or missing provenance preserves the on-disk gate during
  sync. An explicit refresh of a customized gate first saves the exact
  pre-refresh bytes to a cache-only immutable backup, then updates lock and
  cache atomically.
- **Latest-canonical asset freshness for `worktree-flow`**: stale, unknown, and
  user-modified governed assets are replaced by ordinary sync/materialization
  with the latest verified canonical bytes. Replacements are atomic, report the
  prior and new state, and fail closed on any digest, version, self-test,
  backup, write, or lock failure — an unverified asset is never accepted or
  executed. The explicit refresh flag remains a retry/diagnostic path, not a
  prerequisite.
- **Release clean-materialization gate**: `tests/test_release_materialization.py`
  proves an isolated temporary consumer project materializes cleanly through
  init/sync/doctor against the in-tree CLI, and that `SHA256SUMS` declares the
  candidate version and all four platforms. This repository's dogfood
  `ai-specs/.ai-specs.lock` is explicitly not release evidence.

### Changed
- `worktree-flow` recipe `1.4.0` → `1.5.0`: `gate_impl` config, launcher
  distribution, legacy reference materialization.
- `plan-build-flow` `1.4.0` → `1.5.0`: adversarial depth classification compares explicit requests with signal tiers, asks on conflicts, and records resolution annotations in `tasks.md`.
- `plan-build-flow` `1.5.0` → `1.6.0`: tier-specific proposal/spec minima, Standard/Full staged verify evidence gates before archive and merge, and grandfathering guidance for in-flight plans.
- Removed the retired `sdd-adaptive-contract` ceremony contract: deleted the
  canonical spec, the `sdd.decision_matrix` section in `openspec/config.yaml`,
  and the `[sdd]` recipe metadata in `docs/recipe-schema.md`; `plan-build-flow`
  (Light/Standard/Full) is now the sole ceremony contract.
- **`plan-build-flow` readiness is decoupled from the artifact store**:
  `artifact_store_default` is now defined as an external-session persistence
  preference only — Engram may mirror planning artifacts, never replace them.
  Readiness enforced by the gate and the pre-merge guardian is always proven by
  the file-backed `openspec/changes/<slug>/` artifacts (`tasks.md`, tier
  minima, committed planning files, `verify-report.md`). Cross-store invariance
  tests assert gate and guardian decisions are identical across
  `openspec | engram | both`.

### Fixed
- Recipe add no longer mutates the manifest when interactive dependencies are unavailable; the dependency gate now runs before writing.
- Worktree gate no longer misclassifies known internal harness URIs (`xd://`,
  `skill://`, `artifact://`, `local://`, `vault://`, `mcp://`, and others) as
  filesystem destinations in PATH mode — they are tool interfaces, not Git
  targets. The bypass applies only to genuine internal URIs in PATH mode:
  SHELL-mode URI-looking literals and candidates masking `../` traversal or an
  absolute path stay fully gated, and unknown schemes (`https://`, `file://`,
  `custom://`) keep normal classification.
- Worktree gate resolves relative candidates against the tool event `cwd`
  (when present and usable) instead of the hook process `$PWD`, fixing false
  positives when a runtime writes external configuration from a repo-launched
  process; the process `$PWD` remains the fallback.
- `tracker-card-gate.sh` no longer fails to parse under `/bin/bash` 3.2 (stock
  macOS). Bash 3.2 mis-tracks backtick literals inside quoted heredocs that sit
  directly inside command substitution, so the embedded Python's backtick
  fence/quote strings broke the whole script at parse time (every gate
  invocation exited 2 with "unexpected EOF while looking for matching
  backtick"). The heredocs now build backticks via `chr(96)`, keeping the
  literal out of the source; behavior is unchanged.
- Gate binary acquisition no longer 404s: `lib/_internal/gate_binary.py` built
  the asset download URL from a hardcoded `nnodes` repository owner while the
  released assets are attached to `parada1104/ai-specs-cli`. The URL is now
  built from canonical `REPO_OWNER` / `REPO_NAME` constants.
- The release workflow parity job no longer depends on undeclared third-party
  `pytest`, which is absent from stock GitHub runners and failed every release.
  It now runs the repository's canonical `unittest` runner.
- Explicit workspace context is preserved across runtimes. The launcher derives
  its installation root from `BASH_SOURCE[0]` (safe under relative and
  symlinked invocation) and resolves project-local and legacy assets through
  the `hooks/../bin` layout instead of process `$PWD`, so a hook started from
  an unrelated directory no longer misses assets sitting beside it. OpenCode
  normalizes an explicit `directory` (outer trim, absolute and existing) rather
  than silently describing one workspace while resolving assets from another,
  and the Go `event.go` now trims outer whitespace on the event cwd so it
  classifies identically to the frozen Bash reference.
- `ai-specs sync` and `ai-specs sync-agent` run under `/bin/bash` 3.2 (stock
  macOS): the unconditional `shopt -s inherit_errexit` is now applied only when
  the running shell supports that option.
- `worktree-cleanup.sh` now detects squash merges for branches with more than
  one commit. The per-commit `git cherry` patch-id proof cannot match a commit
  that combines several branch patches, so cleanup adds two conservative
  second proofs — combined patch-id equivalence from the common ancestor, and
  final tree-entry equivalence for every path the branch changed (NUL-delimited
  paths, so pathnames containing newlines are compared verbatim). A squash that
  was later reverted, and a partial change set, both stay unmerged.
- The pre-merge guardian accepts the OpenSpec provider's canonical dated
  archive path `openspec/changes/archive/YYYY-MM-DD-<slug>/`, which valid
  changes were being blocked for. Exact undated archives remain supported as
  legacy compatibility; the resolver never guesses among multiple candidates
  and never accepts malformed or near-match names. Active folder,
  artifact-minimum, verification, tier, and planning-root gates are unchanged
  once a path resolves.

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
