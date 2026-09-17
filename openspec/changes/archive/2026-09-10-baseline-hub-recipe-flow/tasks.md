# Tasks — baseline-hub-recipe-flow

Depth: light
Requested depth: light
Signal depth: light
Decided depth: light
Decision source: user

Explore: skipped — Light tier; concrete files and expected edits are named in `proposal.md` and pinned by existing RED tests.

- **Change**: `openspec/changes/baseline-hub-recipe-flow`
- **Worktree**: `.worktrees/baseline-hub-recipe-flow` (`fix/baseline-hub-recipe-flow`)
- **Store**: openspec (this file is the canonical tasks artifact)
- **Tracker**: card_id `6aa2c7c637cfffb4424c052a` — https://trello.com/c/RZ0moUYj/123-baseline-hub-and-recipe-configuration-fixes
- **TDD**: `strict_tdd: true` in `openspec/config.yaml`; test command `./tests/run.sh`; final verify `./tests/validate.sh`
- **RED status**: prior-worker failing tests already exist in this worktree. Do **not** revert them. Do **not** mark implementation tasks complete because those tests exist; they stay unchecked until GREEN production edits make them pass.

Light planning chain is `proposal.md` → `tasks.md`. There is no change-folder spec delta or design doc; `proposal.md` plus the RED tests are the contract. Canonical `openspec/specs/recipe-cli/spec.md` still quotes Spanish uninitialized copy — do not rewrite it in this Light change.

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~520–650 authored (`additions + deletions`): ~342/+17 already in RED tests (~359), plus ~180–280 production in six `lib/_internal` modules |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Suggested split | Single PR (accepted `size:exception`). Logical slices retained for review notes only: Hub menu (A) → English CLI strings (B) → `recipe_ids` API (C) → recipe-add `_dep_gate` + scoped env (D) |
| Delivery strategy | exception-ok |
| Chain strategy | size-exception |

Accepted delivery decision (user: `Una sola PR con excepción`): **one PR** with explicit **`size:exception`**. Reason: honest forecast is **520–650** changed lines, and existing RED tests already account for roughly **359** lines, so no cohesive split would bring the whole change under the 400-line review budget without splitting tests from the production they prove. Do not chain PRs. Keep the A→B→C→D slicing notes as intra-PR work units, not as separate PRs.

## Non-goals

- Do not change aggregate `collect_env_vars` / `prompt_env_vars` / `offer_harness_env` behavior when `recipe_ids` is omitted or `None` (`sync` example generation, `doctor`, global `configure-recipes`, non-interactive `env_scaffold.main`).
- No per-recipe selector inside Hub/global Configure; no `[[deps.env]]` or config-schema changes.
- `recipe-add` MUST NOT call `dep_install.offer_and_install`; no silent installs.
- No catalog, provider-repo, other-worktree (`jinna-mcp-live`), or generated sync-state edits.
- Do not chase the known unrelated stale Bitbucket fixture failure; record it as pre-existing if `./tests/validate.sh` still shows it.
- Do not revert or rewrite assertions in the prior-worker RED tests.
- No commit, push, PR, or archive in the Tasks phase; Apply still waits for authorization.

## Rollback

Single fix branch off `development`. No migrations or stored-format changes. Revert the fix commits (or restore `hub.py`, `env_scaffold.py`, `recipe-add.py`, `config_wizard.py`, `recipe-list.py`, `recipe-init.py`) to roll back. Generated user env files are untouched by design.

---

## 1. Confirm RED (do not treat as done)

- [x] From the worktree root, run the focused suites and confirm they fail for missing Hub entry, missing `recipe_ids`, Spanish copy, and missing `_dep_gate` wiring — not import/syntax errors. Commands: `./tests/run.sh tests/test_hub_tui.py tests/test_env_scaffold.py tests/test_recipe_add.py tests/test_recipe_list.py tests/test_recipe_init.py tests/test_sync_env_scaffold.py tests/test_config_wizard.py`. Preserve every existing assertion. <!-- sdd-owner: implementation -->

## 2. GREEN — (A) Hub main-menu Configure recipes

Start: `_MENU` in `lib/_internal/hub.py` has 11 entries and omits `Action.CONFIGURE_RECIPES` (enum already exists at line 59). Recipes submenu at `hub.py` ~445–498 already calls `runner.run(Action.CONFIGURE_RECIPES)`.

Finish: `_MENU` has exactly 12 entries; `"Configure recipes"` sits immediately after `"Recipes"` with description `"Set up recipe config, CLI deps, env vars"` (or any non-empty stripped description the RED test accepts). Submenu alias unchanged.

- [x] Insert `(Action.CONFIGURE_RECIPES, "Configure recipes", "Set up recipe config, CLI deps, env vars")` into `_MENU` in `lib/_internal/hub.py` immediately after the Recipes row. Do not add a new Action. Do not change submenu dispatch. <!-- sdd-owner: implementation -->
- [x] GREEN: `./tests/run.sh tests/test_hub_tui.py` — `test_menu_has_exact_twelve_entries` and `test_configure_recipes_visible_in_main_menu_with_description` pass; existing PTY/loop tests still pass. If another test hard-codes menu length 11, update only that stale offset. <!-- sdd-owner: implementation -->

Rollback boundary: `lib/_internal/hub.py` `_MENU` only; `tests/test_hub_tui.py` stays with the branch.

## 3. GREEN — (B) English user-facing CLI strings

Start: Spanish/mixed copy in `lib/_internal/env_scaffold.py`, `recipe-add.py`, `config_wizard.py`, `recipe-list.py`, `recipe-init.py`. Stored keys, purpose strings, file formats, markers, TOML, and exit codes stay unchanged.

Finish: no Spanish prose in those modules' user-facing output; RED English assertions pass unmodified.

- [x] Translate user-facing strings in `lib/_internal/env_scaffold.py`: prompt heading `"Variables de entorno requeridas"`, confirm `"¿Configurar ahora los valores?"`, secret `instruction="(input oculto)"`, write/error/direnv messages (including `"direnv no está instalado…"`, `"Instalalo con…"`, `"Despues corre…"`, `"las variables quedan activas…"`, `"direnv allow falló"`, `"No se pudieron configurar…"`, `"escrito"`), `main()` uninitialized line, and `main()` missing-value warning to exactly `! {var} has no value in ai-specs.env — run ai-specs configure-recipes`. Keep env var **names** and catalog **purpose** text as stored data. <!-- sdd-owner: implementation -->
- [x] Translate user-facing strings in `lib/_internal/recipe-add.py`: uninitialized `"Proyecto no inicializado. Ejecuta: ai-specs init"` → include `"Project not initialized"`; catalog/manifest/interactive-deps/TOML-rollback/success/next-step/configure-now/non-TTY guidance currently in Spanish (e.g. `"¿Configurar ahora?"`, `"Podés configurar después…"`, `"Siguientes pasos"`, `"Configurar valores requeridos"`, `"agregada al manifest"`). Non-TTY must still mention `ai-specs configure-recipes`. <!-- sdd-owner: implementation -->
- [x] Translate uninitialized copy in `lib/_internal/config_wizard.py` (`"Proyecto no inicializado: missing {manifest}"`), `lib/_internal/recipe-list.py` (`"Proyecto no inicializado. Ejecuta: ai-specs init"`), and `lib/_internal/recipe-init.py` (`RecipeInitError("Proyecto no inicializado. Ejecuta: ai-specs init")`) so stderr/exception text includes `"Project not initialized"`. <!-- sdd-owner: implementation -->
- [x] GREEN: `./tests/run.sh tests/test_recipe_list.py tests/test_recipe_init.py tests/test_sync_env_scaffold.py tests/test_config_wizard.py` plus `tests/test_env_scaffold.py TestEnvScaffold.test_main_warns_missing_values_nonfatal` and `tests/test_recipe_add.py TestRecipeAdd.test_cli_uninitialized_project`. File-format tests (`write_env`, `.envrc` managed block, examples) stay green. <!-- sdd-owner: implementation -->

Rollback boundary: string literals in the five modules above; no format/marker/TOML changes.

## 4. GREEN — (C) Optional `recipe_ids` scoping

Start: `collect_env_vars(project_root)`, `prompt_env_vars(project_root)`, `offer_harness_env(project_root, *, offer_direnv_install=True)` in `lib/_internal/env_scaffold.py` always aggregate enabled recipes.

Finish: optional `recipe_ids: list[str] | None = None` on all three; `None`/omitted is a no-op vs today; a list narrows collection/prompt/write; example + root `.envrc` + direnv stay global/aggregate.

- [x] Add `recipe_ids: list[str] | None = None` to `collect_env_vars` in `lib/_internal/env_scaffold.py`. When `None`, keep the current enabled-recipe loop. When a list is passed, collect only those ids (enabled); a selected-but-disabled recipe contributes nothing (`{}` if that is the only selection). First-declaration-wins purpose text unchanged. <!-- sdd-owner: implementation -->
- [x] Forward `recipe_ids` from `prompt_env_vars` into `collect_env_vars`. Preserve secret vs non-secret prompting (`questionary.password` vs `text`). <!-- sdd-owner: implementation -->
- [x] Forward `recipe_ids` from `offer_harness_env` into `collect_env_vars` and `prompt_env_vars`. Merge **only** prompted values into `ai-specs.env`. Keep `generate_env_example`, `ensure_root_envrc`, and direnv handling global/aggregate. Callers that omit `recipe_ids` (sync/example/doctor/global configure) stay aggregate. <!-- sdd-owner: implementation -->
- [x] GREEN: `./tests/run.sh tests/test_env_scaffold.py tests/test_sync_env_scaffold.py` — `test_collect_env_vars_selected_recipe_only`, `test_collect_env_vars_aggregate_when_omitted`, `test_collect_env_vars_selected_skips_disabled_recipe`, `test_prompt_env_vars_selected_recipe_only`, `test_offer_harness_env_selected_recipe_only`, `test_offer_harness_env_aggregate_when_omitted`, plus existing aggregate/write/direnv tests. <!-- sdd-owner: implementation -->

Rollback boundary: `lib/_internal/env_scaffold.py` signatures and filter loop; default `None` restores aggregate behavior.

## 5. GREEN — (D) recipe-add `_dep_gate` then scoped env

Start: TTY Configure-now in `lib/_internal/recipe-add.py` calls `config_wizard.configure_selected_recipes` and unscoped `collect_env_vars` / `offer_harness_env`; never `config_wizard._dep_gate`. Pre-write `util.ensure_deps` TTY gate stays.

Finish: TTY Configure-now runs `_dep_gate(recipe, Console)` for `recipe.cli_deps` **before** env prompts; unresolved gate prints install URLs + `"required CLI dependencies are still missing"` via `dep_install.resolve_install_plan` only; `offer_and_install` is not called from recipe-add; scoped env still runs; non-TTY is guidance-only.

- [x] In the TTY Configure-now path of `lib/_internal/recipe-add.py`, after the user confirms configure-now and **before** env prompts, load `config_wizard` and call `_dep_gate(recipe, console)` with a real `rich.console.Console`. Keep the existing pre-write `util.ensure_deps` + questionary gate. Do not invoke `_dep_gate` when not a TTY. <!-- sdd-owner: implementation -->
- [x] Pass `recipe_ids=[recipe_id]` to both `collect_env_vars` and `offer_harness_env` in that path. <!-- sdd-owner: implementation -->
- [x] On `_dep_gate` returning false/unresolved: call `dep_install.resolve_install_plan` per missing `cli_deps` binary (use each dep's install URL), print each URL and a line containing `required CLI dependencies are still missing`, then still run scoped env setup. Never call `dep_install.offer_and_install` from `recipe-add.py` (install offers stay inside `_dep_gate`). <!-- sdd-owner: implementation -->
- [x] Non-TTY: no `_dep_gate`, no `offer_harness_env`; print guidance that includes `ai-specs configure-recipes`. <!-- sdd-owner: implementation -->
- [x] GREEN: `./tests/run.sh tests/test_recipe_add.py` — `test_add_routes_cli_deps_through_dep_gate`, `test_add_reports_install_guidance_when_dep_gate_unresolved`, `test_add_non_tty_cli_deps_is_guidance_only`, plus existing add/idempotency/TTY-ensure_deps tests. Also `./tests/run.sh tests/test_config_wizard.py` so `_dep_gate` TTY offer/abort/proceed behavior is unchanged. <!-- sdd-owner: implementation -->

Rollback boundary: Configure-now block in `lib/_internal/recipe-add.py` only.

## 6. TRIANGULATE

- [x] If GREEN left gaps, add **new** tests without editing prior-worker assertions: (1) Hub Recipes submenu still dispatches `Action.CONFIGURE_RECIPES`; (2) `collect_env_vars(..., recipe_ids=[])` yields `{}`; (3) global `config_wizard` / `offer_harness_env` without `recipe_ids` still aggregate. Keep each test in the matching file under `tests/`. <!-- sdd-owner: implementation -->
- [x] Re-run `./tests/run.sh tests/test_hub_tui.py tests/test_env_scaffold.py tests/test_recipe_add.py tests/test_recipe_list.py tests/test_recipe_init.py tests/test_sync_env_scaffold.py tests/test_config_wizard.py`. <!-- sdd-owner: implementation -->

## 7. REFACTOR

- [x] Under green, dedupe `recipe_ids` forwarding in `env_scaffold.py` if collect/prompt/offer duplicated filter logic. Do not change behavior. Re-run the focused suites after each meaningful cleanup. <!-- sdd-owner: implementation -->

## 8. Verify (advisory for Light)

- [x] Run `./tests/validate.sh` from the worktree root. Focused baseline suites MUST be green. If the full suite still fails a stale Bitbucket fixture unrelated to this change, record the exact test id and treat it as out of scope — do not "fix" it here. Coverage/lint/type-check/format are unavailable per `openspec/config.yaml`; do not claim they passed. README: update `README.md` only if it currently quotes the old Spanish CLI copy for these commands; Hub menu copy does not require a new README section. <!-- sdd-owner: implementation -->

## Work-unit map (single PR; slices are review notes only)

Land A–D in **one** PR under accepted `size:exception`. The rows below are intra-PR session boundaries, not chained PRs.

| Unit | Starts | Finishes | Verify | Rollback |
|------|--------|----------|--------|----------|
| Slice A | 11-entry `_MENU` | 12-entry Hub with Configure recipes after Recipes | `./tests/run.sh tests/test_hub_tui.py` | `hub.py` `_MENU` |
| Slice B | Spanish CLI copy in five modules | English user-facing strings; artifacts unchanged | list/init/uninitialized/main-warning/sync warning + config_wizard | string literals only |
| Slice C | Unscoped collect/prompt/offer | Optional `recipe_ids`; `None` aggregate | `test_env_scaffold.py` scoped + aggregate + `test_sync_env_scaffold.py` | `env_scaffold.py` API default `None` |
| Slice D | recipe-add skips `_dep_gate`, unscoped env | TTY gate then scoped env; non-TTY guidance | `test_recipe_add.py` three new tests + config_wizard dep tests | Configure-now block in `recipe-add.py` |

Keep tests in the same unit as the production they prove.

## Apply stop

Superseded: the human explicitly authorized Apply for this change (baseline implementation approved; single-PR `size:exception` accepted after the honest 520–650 line forecast). No commit, push, PR, or archive was performed during Apply.

---

## Apply evidence (2026-09-session, `fix/baseline-hub-recipe-flow`)

Test command base: `PYTHONPATH=. python3 -m unittest <modules>` from the worktree root (`openspec/config.yaml` `testing.test_runner.command` is `./tests/run.sh`). No commit or SHA is recorded because Apply does not commit.

### RED (before production edits)

`PYTHONPATH=. python3 -m unittest tests.test_hub_tui tests.test_env_scaffold tests.test_sync_env_scaffold tests.test_recipe_add tests.test_config_wizard tests.test_recipe_list tests.test_recipe_init`

- Result: `Ran 128 tests` — `FAILED (failures=15, errors=6)` (exit 1) = 21 RED.
- Failure classes (all missing-behavior, not import/syntax errors):
  - Hub: `test_menu_has_exact_twelve_entries` (11-entry `_MENU`), `test_configure_recipes_visible_in_main_menu_with_description` (no entry), 4 PTY index failures (`test_quit_immediately`, `test_version_inline_then_quit`, `test_doctor_delegates_and_resumes`, `test_skills_shows_categorized_headers`).
  - env_scaffold: 6 errors — `recipe_ids` keyword not accepted by `collect_env_vars` / `prompt_env_vars` / `offer_harness_env`; `test_main_warns_missing_values_nonfatal` on Spanish copy; `test_sync_warns_missing_env_values_nonfatal` on Spanish copy.
  - recipe_add: `test_add_routes_cli_deps_through_dep_gate` and `test_add_reports_install_guidance_when_dep_gate_unresolved` (`_dep_gate` called 0 times), `test_cli_uninitialized_project`, `test_mcp_env_deps_gate`, `test_tty_missing_interactive_deps_does_not_mutate_manifest` (Spanish copy).
  - recipe_list / recipe_init: `Project not initialized`, `Recipe 'missing' not found` (Spanish copy).
- `tests/test_config_wizard.py` was green at RED; it is included as the regression guard for `_dep_gate` behavior.

### GREEN (after minimal production edits)

- `PYTHONPATH=. python3 -m unittest tests.test_hub_tui` → `Ran 16 tests` … `OK`
- `PYTHONPATH=. python3 -m unittest tests.test_env_scaffold tests.test_sync_env_scaffold` → `Ran 48 tests` … `OK`
- `PYTHONPATH=. python3 -m unittest tests.test_recipe_add tests.test_config_wizard` → `Ran 41 tests` … `OK`
- `PYTHONPATH=. python3 -m unittest tests.test_recipe_list tests.test_recipe_init` → `Ran 26 tests` … `OK`
- Combined: `PYTHONPATH=. python3 -m unittest tests.test_hub_tui tests.test_env_scaffold tests.test_sync_env_scaffold tests.test_recipe_add tests.test_config_wizard tests.test_recipe_list tests.test_recipe_init` → `Ran 131 tests in 28.793s` … `OK` (exit 0). 131 = 128 prior tests + 3 TRIANGULATE additions.

### Full validation (advisory for Light)

- `PYTHONPATH=. ./tests/validate.sh` → `Ran 1817 tests in 679.976s` … `FAILED (failures=1, skipped=133)` (exit 1).
- Only failure: `test_bitbucket_pr_flow_recipe.BitbucketPrFlowGoldenContentTests.test_apply_progress_omits_absolute_host_and_worktree_paths` — asserts `openspec/changes/bitbucket-bb-cli-alignment/apply-progress.md` exists, but that change is archived as `openspec/changes/archive/2026-09-07-bitbucket-bb-cli-alignment/` (commit `cf09e1d`). Pre-existing and unrelated to this change; left untouched.
- `py_compile lib/_internal/*.py tests/*.py`, `bash -n lib/*.sh bin/ai-specs tests/*.sh`, and `./tests/run.sh` all ran as part of validate.sh. Coverage / linter / type-checker / formatter are unavailable per `openspec/config.yaml` and are not claimed.
- `README.md` was not changed: it already uses English for these commands and does not quote the old Spanish CLI copy.

### TRIANGULATE additions (new tests, no prior assertion edited)

- `tests/test_hub_tui.py` — `test_recipes_submenu_configure_alias_still_dispatches` (Recipes → Configure alias still runs `Action.CONFIGURE_RECIPES`).
- `tests/test_env_scaffold.py` — `test_collect_env_vars_empty_selection_collects_nothing` (`recipe_ids=[]` → `{}`).
- `tests/test_config_wizard.py` — `test_global_configure_recipes_keeps_aggregate_env_offer` (global `configure-recipes` still calls `offer_harness_env(project)` with no `recipe_ids`).

### Test-harness correction (assertions unchanged)

`tests/test_recipe_add.py::test_add_reports_install_guidance_when_dep_gate_unresolved` captured stdout with `contextlib.redirect_stdout(io.StringIO())`, which replaces `sys.stdout` and therefore made the TTY-gated path under test look non-interactive (`sys.stdout.isatty()` → `False`). The capture target is now a `_TtyStringIO` that reports `isatty() == True`; every pre-existing assertion is byte-identical. Production TTY detection keeps the repository convention `sys.stdin.isatty() and sys.stdout.isatty()`.

### Authored size and PR boundary

- Authored changed lines (`git diff --stat` on tracked `lib/` + `tests/`): **528 insertions + 65 deletions = 593** (excludes the untracked Light plan folder). Within the accepted 520–650 honest forecast → single PR under explicit `size:exception`, strategy `size-exception` / delivery `exception-ok`.
- Review notes (not chained PRs): Slice A (Hub menu) → Slice B (English strings) → Slice C (`recipe_ids`) → Slice D (`_dep_gate` + scoped env).
- Rollback: revert the six `lib/_internal` modules and the seven `tests/` files; no migrations, no stored-format changes, no generated-state edits.

### Deviations from the plan

1. `_dep_gate` runs before env prompts for **any** recipe with `cli_deps`, as instructed. For recipes that also declare config fields, `config_wizard.configure_selected_recipes` still runs its own internal `_dep_gate`, so the dependency panel is rendered twice in that path (a second confirm prompt only appears when the user declines install and then chooses to configure anyway). No silent install is introduced; `offer_and_install` stays inside `_dep_gate`. Follow-up candidate: let `configure_selected_recipes` accept a pre-resolved gate result to avoid the duplicate.
2. No `recipe_ids` de-duplication was needed in `env_scaffold.py` (only `collect_env_vars` filters; `prompt_env_vars` and `offer_harness_env` only forward). One readability cleanup was applied in `recipe-add.py` with the focused suites re-run green.
