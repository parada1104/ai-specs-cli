# Proposal: Recipe config wizard + CLI dependency management + `.envrc` scaffolding

## Why (motivation)

The TUI hub (`hub.py`) and init wizard (`init_tui.py`) can enable recipes, but they stop at
`enabled = true` + version — the init wizard's `_render_manifest` writes no
`[recipes.<id>.config]` block, so users must hand-edit `ai-specs/ai-specs.toml` to configure a
recipe. Worse, several recipes silently require an external CLI (`gh`, `glab`, `jq`, `bb`, `npx`,
`git` worktrees) that is declared only in SKILL.md prose, never in `recipe.toml`. A user can
enable `git-pr-flow` and only discover `gh` is missing when a command fails. Env-var setup for MCP
servers (Trello, vault) is likewise undocumented per-project.

The schema layer already carries everything needed to fix this: `ConfigField`
(required/type/default/enum/validation.regex) fully describes each config field, and
`[[provides.mcp]]` env tables already name the required env vars. What is missing is (a) a machine-
readable CLI-dependency declaration, (b) a wizard that turns `ConfigField` metadata into prompts,
(c) a comment-preserving write-back path, and (d) an env-var scaffold. This change wires those in
as an additive extension of the hub/init deliverables on the same `tui-hub` branch.

## Intent

Make recipe configuration and prerequisites first-class and self-service:

1. Declare CLI prerequisites in `recipe.toml` via a new `[[deps.cli]]` array-of-tables.
2. Check those prerequisites (guidance-only, no auto-install) in a reusable `dep_check.py`, surfaced
   as Doctor `WARN` rows for enabled recipes and as a panel in the wizards.
3. Collect per-recipe config through a questionary-driven wizard that reuses `ConfigField` metadata,
   reachable both from a new hub action (`CONFIGURE_RECIPES`) and inline during init (step 3.5).
4. Write collected values back into `[recipes.<id>.config]` with a surgical, comment-preserving
   updater guarded by `tomllib.loads` validation + restore-on-failure.
5. Generate `ai-specs/.envrc.example` (committed template) derived from enabled recipes'
   `[[provides.mcp]]` env tables — never touch the user-owned, gitignored `.envrc`.

## Scope (in)

1. **`[[deps.cli]]` schema** — `recipe_schema.py`: add a `CliDep` dataclass, `_parse_cli_deps(raw,
   context)` (allowed-keys set → `RecipeValidationError` on unknown keys, mirroring `_parse_config`
   discipline), and a `Recipe.cli_deps: list[CliDep]` field wired into `validate_recipe_toml` via
   `data.get("deps", {}).get("cli", [])`. Fields: `binary` (required), `purpose` (required),
   `required` (bool, default `true`), `install_url`, `version_check`, `min_version` (all optional
   strings). `recipe-read.py`'s `recipe_to_dict` serializes `cli_deps`.
2. **Catalog updates** — add `[[deps.cli]]` blocks to the recipes that need a CLI:
   `git-pr-flow`→`gh`; `gitlab-mr-flow`→`glab` + `jq`; `bitbucket-pr-flow`→`bb`;
   `trello-mcp-workflow`→`npx`; `vault-canonical-store`→`npx`; `worktree-flow`→`git`. `tdd-flow`
   has no fixed binary (test command is config-driven) — left without a `[[deps.cli]]` block.
3. **Dependency check** — new `dep_check.py`: `check_cli_deps(recipe) -> list[DepResult]` runs
   `command -v <binary>` (POSIX, no new dependency); when `version_check` is set, runs it and
   compares parsed output against `min_version` with a simple string/tuple compare (no semver lib).
   Doctor integration: a new `_check_recipe_cli_deps` emits `Severity.WARN` `Check` rows (with
   `guidance` = install_url) for a required CLI dep missing on an enabled recipe.
4. **Config wizard** — new `config_wizard.py`: `run_config_wizard(recipe, existing_config) -> dict`,
   questionary-driven per-field prompts. Reuses `ConfigField` metadata: `questionary.select` for
   `enum`, `questionary.confirm` for `type == bool`, `questionary.text` + validate callback for
   `required`/`validation.regex`, defaults pre-filled from `ConfigField.default`. Only prompts for
   `config_schema.fields` (never `config_schema.extra`). Runs `check_cli_deps` first; on a missing
   required dep, shows install guidance and offers proceed-anyway / abort.
5. **Config write-back** — new `recipe-config-write.py`:
   `update_recipe_config(manifest_path, recipe_id, values)` performs surgical line replacement
   inside the `[recipes.<id>.config]` block (replace existing `key = value` line, else insert before
   the next section header), preserving all surrounding comments. NOT a TOML round-trip. Validates
   the result with `tomllib.loads`; restores the original text on parse failure (same guard as
   `recipe-add.py`).
6. **Hub integration** — `hub.py`: new `Action.CONFIGURE_RECIPES` enum member, a `_MENU` entry
   ("Configure recipes", "Set config values for enabled recipes"), and delegation to the config
   wizard entrypoint. This is the re-configuration path for already-enabled recipes.
7. **Init wizard integration** — `init_tui.py`: new step 3.5 after recipe selection and before
   preview — for each selected recipe with `config_schema.fields`, run the config sub-wizard; for
   each with `cli_deps`, show a dep-check panel. `_render_manifest` writes real
   `[recipes.<id>.config]` values (not placeholders) from the collected map. Offer `.envrc.example`
   generation.
8. **`.envrc.example` generation** — new `envrc-scaffold.py`:
   `generate_envrc_example(project_root) -> Path` scans enabled recipes' `[[provides.mcp]]` env
   tables, collects referenced `$VAR` names (reusing `recipe-init.py`'s `ENV_REFERENCE_RE` /
   `SECRET_KEY_RE`), and writes `ai-specs/.envrc.example` with `export VAR=""  # <purpose>` lines.
   Never writes `.envrc` (gitignored, line 30).
9. **Shell shim** — new `lib/recipe-config.sh` dispatching to `config_wizard.py`, parallel to
   `recipe-add.sh` / `doctor.sh` (resolve home/target, dep gate, exec python3).
10. **Tests** (strict TDD, `./tests/run.sh`):
    - `test_recipe_schema.py` → `CliDepParsingTests` (valid parse, missing `binary`, missing
      `purpose`, unknown key raises, optional defaults).
    - `test_dep_check.py` → found/missing binary, version-check pass/fail, `required=false` →
      no failure.
    - `test_config_wizard.py` → required re-prompt, regex re-prompt, enum select, bool confirm,
      default fill; dep-gated abort path.
    - `test_recipe_config_write.py` → replace existing key, insert new key, comments preserved,
      invalid write restores original.
    - `test_envrc_scaffold.py` → env vars derived from MCP env, purpose comments, `.envrc` never
      written.
11. **Docs** — `docs/recipe-schema.md` (`[[deps.cli]]` section in V2 additions, mirroring
    `[[capabilities]]` style); `docs/recipes-catalog.md` (CLI prerequisites per recipe);
    `docs/ai-specs-toml.md` (note on `.envrc.example` generation). README update if user-facing hub
    behavior changes.

## Non-goals

- Auto-install of CLI deps (brew/apt/scoop dispatch) — guidance/links only.
- `[[deps.env]]` schema for non-MCP env vars (e.g. deriving `CANONICAL_VAULT_PATH` from
  `OBSIDIAN_VAULT_PATH`) — first slice derives env only from `[[provides.mcp]]`.
- Direct `.envrc` write — stays user-owned and gitignored; only `.envrc.example` is generated.
- Full TOML-library round-trip manifest editor — the manifest stays hand-built to preserve comments.
- Authoring `validation.regex` for git-branch-name config fields — that is a per-recipe schema edit,
  not this change (`validation.regex` support already exists and is honored by the wizard).
- Semver-library dependency — keep a simple stdlib string/tuple compare for `min_version`.
- Changing `recipe-materialize.py` `merge_config` or sync behavior — sync remains the config
  authority; the wizard only fails fast at collection time.

## Design decisions (proposed)

1. **`[[deps.cli]]` lives in `recipe.toml`, NOT the manifest's `[[deps]]`.** The manifest's
   `[[deps]]` means vendored skills — a different concept. `recipe.toml` is a separate file, so
   `[[deps.cli]]` under a `[deps]` table is unambiguous and namespaced. Rationale: no overload of an
   existing key's meaning; additive and backward-compatible (recipes without the block parse
   unchanged).
2. **Config write-back = surgical line replacement, NOT TOML round-trip.** The manifest is built via
   string concat specifically to preserve comments; a `tomllib`+dump round-trip would strip them.
   `update_recipe_config` replaces only the target `key = value` lines and validates the result with
   `tomllib.loads`, restoring the original on failure.
3. **Dep check = guidance-only, Doctor `WARN` integration.** No package-manager automation.
   `_check_recipe_cli_deps` reuses the `Check(Severity.WARN, name, message, guidance=install_url)`
   shape (closest existing pattern: `_check_bundled_assets`). Non-blocking but visible on every hub
   open (hub status panel already runs Doctor).
4. **`.envrc.example` generation, NOT `.envrc` write.** `.envrc` is gitignored (secret-bearing);
   the tool must not write it. `.envrc.example` is committed and safe to regenerate.
5. **Same branch (`tui-hub`), single PR.** The config wizard is a hub action, init step 3.5 modifies
   `init_tui.py` (a tui-hub deliverable), and the schema change is additive. A separate branch would
   force a cross-branch dependency on `init_tui.py`. Ship as one PR (`size:exception` per session
   delivery strategy) with commits grouped by phase: schema → catalog → dep-check → wizard →
   config-write → envrc → hub/init → docs.

## Impact (files)

**New**
- `lib/_internal/dep_check.py`
- `lib/_internal/config_wizard.py`
- `lib/_internal/recipe-config-write.py`
- `lib/_internal/envrc-scaffold.py`
- `lib/recipe-config.sh`
- `tests/test_dep_check.py`, `tests/test_config_wizard.py`, `tests/test_recipe_config_write.py`,
  `tests/test_envrc_scaffold.py`

**Modified**
- `lib/_internal/recipe_schema.py` (`CliDep` + `_parse_cli_deps` + `Recipe.cli_deps`)
- `lib/_internal/recipe-read.py` (`recipe_to_dict` serializes `cli_deps`)
- `lib/_internal/doctor.py` (`_check_recipe_cli_deps`, registered in `run()`)
- `lib/_internal/hub.py` (`Action.CONFIGURE_RECIPES` + `_MENU` entry + delegation)
- `lib/_internal/init_tui.py` (step 3.5 config sub-wizard + dep panel; `_render_manifest` writes
  config block)
- `tests/test_recipe_schema.py` (`CliDepParsingTests`)
- `docs/recipe-schema.md`, `docs/recipes-catalog.md`, `docs/ai-specs-toml.md`; `README.md` if hub
  behavior surface changes
- `catalog/recipes/{git-pr-flow,gitlab-mr-flow,bitbucket-pr-flow,trello-mcp-workflow,`
  `vault-canonical-store,worktree-flow}/recipe.toml` (add `[[deps.cli]]`)

**Untouched (intentional):** `recipe-materialize.py` (`merge_config` unchanged — sync stays the
authority), `recipe-add.py` (keeps placeholder-writing for the non-wizard path), `toml-read.py`,
`toml_write.py`.

## Risks

- **Config write-back corrupts the manifest if the regex is wrong.** Mitigation: validate the
  rewritten text with `tomllib.loads` and restore the original text on any parse failure — same
  guard `recipe-add.py` already uses. Tests assert restore-on-invalid.
- **Init wizard grows longer (step 3.5 prompts per recipe) and may overwhelm users.** Mitigation:
  a per-recipe "configure later" / skip option that leaves defaults (or omits the config block),
  so init stays fast for users who just want defaults.
- **`[[deps.cli]]` is a new `recipe.toml` section that other tooling must tolerate.**
  `recipe-conflicts.py` and any recipe-diffing logic must ignore or handle the new block; because
  parsing is additive with defaults, recipes without it are unaffected. Verify conflicts tooling
  still passes.
- **Doctor integration must not break non-hub `ai-specs doctor` invocation.** `_check_recipe_cli_deps`
  must degrade gracefully when a recipe has no `cli_deps` and when `command -v`/version_check
  subprocesses fail; it emits at most `WARN`, never raises, and never changes the exit code
  (WARN ≠ ERROR).
- **`command -v` / `version_check` subprocess portability.** Use a POSIX-safe invocation and
  tolerate missing shells; treat any check error as "not found" WARN rather than crashing.

## Rollback

1. `[[deps.cli]]` parsing is additive with defaults — revert `recipe_schema.py`/`recipe-read.py`
   and catalog blocks; recipes without the section parse unchanged.
2. New modules (`dep_check.py`, `config_wizard.py`, `recipe-config-write.py`, `envrc-scaffold.py`,
   `recipe-config.sh`) are additive/unreferenced once `hub.py` + `init_tui.py` hooks are reverted →
   delete.
3. `_check_recipe_cli_deps` is a single registered check — remove its registration in `Doctor.run()`
   to disable; other checks unaffected.
4. `init_tui.py` step 3.5 revert restores placeholder/`enabled`-only manifest writing.
5. `.envrc.example` is a generated committed file — delete it; `.envrc` was never touched.

## Success criteria

- A recipe with `[[deps.cli]]` parses into `Recipe.cli_deps`; unknown keys raise
  `RecipeValidationError`; missing `binary`/`purpose` raise; optional fields default correctly.
- `check_cli_deps` reports found/missing per binary and honors `version_check` + `min_version`;
  `required=false` deps never produce a failure.
- `ai-specs doctor` on a project with an enabled recipe whose required CLI is missing emits a
  `WARN` row with install guidance, and exit code is unchanged (non-zero only on ERROR).
- Hub "Configure recipes" runs the wizard for enabled recipes; values land in
  `[recipes.<id>.config]` with comments preserved; an invalid write leaves the manifest intact.
- Init wizard step 3.5 collects config per selected recipe (with skip/later option) and writes real
  config values; dep-check panel shows missing prerequisites.
- `generate_envrc_example` writes `ai-specs/.envrc.example` from MCP env vars and never writes
  `.envrc`.
- Docs updated (`recipe-schema.md`, `recipes-catalog.md`, `ai-specs-toml.md`).
- `./tests/run.sh` and `./tests/validate.sh` pass; new test suites present and green.

## Phase breakdown

1. **P1 Schema (additive, no recipe broken):** `CliDep` + `_parse_cli_deps` + `Recipe.cli_deps`;
   `recipe_to_dict` serialization; `CliDepParsingTests` (RED→GREEN). Catalog `[[deps.cli]]` blocks.
2. **P2 Dep check + Doctor:** `dep_check.py` (`command -v` + version compare); `_check_recipe_cli_deps`
   WARN integration; `test_dep_check.py`.
3. **P3 Config write-back + wizard:** `recipe-config-write.py` (surgical, validated, restore-on-fail)
   + `test_recipe_config_write.py`; `config_wizard.py` reusing `ConfigField` + `test_config_wizard.py`;
   `lib/recipe-config.sh`.
4. **P4 Hub + init integration:** `Action.CONFIGURE_RECIPES` + menu; init step 3.5 (config sub-wizard
   + dep panel + configure-later); `_render_manifest` writes config block.
5. **P5 `.envrc.example`:** `envrc-scaffold.py` deriving from `[[provides.mcp]]` env +
   `test_envrc_scaffold.py`; wizard/hub offer to generate.
6. **P6 Docs + polish:** `recipe-schema.md`, `recipes-catalog.md`, `ai-specs-toml.md`, README;
   `./tests/validate.sh`; verify every success criterion.

## Classification

`domain_change` per `openspec/config.yaml` decision matrix (new `recipe.toml` schema section +
cross-cutting hub/init/doctor behavior) → proposal → design → tasks, worktree required. Delivered on
the existing `tui-hub` worktree as a single `size:exception` PR.
