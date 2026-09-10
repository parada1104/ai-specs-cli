# Proposal — baseline-hub-recipe-flow

- **Change**: `openspec/changes/baseline-hub-recipe-flow`
- **Tier**: Light — planning chain `proposal.md` → `tasks.md` (no spec deltas, no design doc)
  - Requested depth: **light** (user explicitly selected option A: create the change folder with `proposal.md` + `tasks.md` before lib/ implementation)
  - Signal depth: **light** (bounded baseline corrections; concrete files and expected edits already named by exploration; contract pinned by existing RED tests)
  - Decided depth: **light** (requested == signal; no conflict. Decision source: user)
- **Status**: Proposed. Next: parent links the tracker card, dispatches the Tasks phase, then Apply in this worktree.
- **Store**: openspec (this canonical file-backed change folder; any Engram copy is a mirror only).
- **Location**: worktree `.worktrees/baseline-hub-recipe-flow`, branch `fix/baseline-hub-recipe-flow` (clean cut from `development`).

## Intent

Fix the baseline defects found during the jinna recipe live run so recipe onboarding is discoverable, correctly scoped, dependency-safe, and linguistically consistent:

1. **Hub discoverability**: `lib/_internal/hub.py` defines `Action.CONFIGURE_RECIPES` (line 59) but `_MENU` (lines 61–76) omits it. Configure recipes is only reachable through the nested Recipes → Configure submenu, so first-run env/dependency setup is effectively hidden.
2. **Recipe-add bypasses the dependency gate and over-prompts env vars**: `recipe-add.py`'s Configure-now flow never invokes the existing `config_wizard._dep_gate` for `recipe.cli_deps`, and it calls `env_scaffold.collect_env_vars(project_root)` / `offer_harness_env(project_root)` unscoped — which by design aggregates **all** enabled recipes' MCP env refs, so adding one recipe prompts Trello/Vault/… variables too.
3. **Mixed-language UI**: user-facing strings in the touched baseline modules mix Spanish and English (env prompt headings, confirmations, direnv messages, recipe-add success/next-step text, uninitialized-project errors).

The aggregate behavior of `env_scaffold.collect_env_vars(project_root)` is load-bearing for `sync` example generation, `doctor`, and the global `configure-recipes` path — scoping must be **additive**, never a behavior flip.

## User-observed evidence

From the manual live run in `.worktrees/jinna-mcp-live` and the baseline exploration:

- The user added/enabled the jinna recipe and ran Hub: the UI mixed English and Spanish; no provider install was ever offered; env configuration prompted every variable of the aggregate example (Trello, Vault, …) instead of only the selected recipe's variables.
- Code evidence matches: `hub.py` `_MENU` omits `CONFIGURE_RECIPES`; `recipe-add.py` routes Configure-now through `config_wizard.configure_selected_recipes` and unscoped `env_scaffold.offer_harness_env`, never through `_dep_gate`; `config_wizard._dep_gate` — which already offers explicit TTY installation via `dep_install.offer_and_install` and re-checks — is only reached through the configure-recipes/init paths today.

## Accepted product decisions (confirmed pre-proposal handoff — captured exactly)

1. Add Recipe → Configure now prompts only the selected recipe's env vars, while a visible global Configure recipes action handles all enabled recipes.
2. Hub gets a top-level Configure recipes action and keeps the Recipes submenu alias.
3. All user-facing CLI strings in the touched baseline paths become English, but stored data values and technical artifacts stay unchanged.
4. Missing CLI dependencies are offered for explicit installation during Add Recipe → Configure now, before env prompts, never silently and still safe in non-TTY.

## Scope (minimal implementation)

Four pieces; no new modules, no schema changes.

### 1. Optional recipe scoping in `env_scaffold.py`
- Add an optional `recipe_ids: list[str] | None = None` parameter to `collect_env_vars()`, `prompt_env_vars()`, and `offer_harness_env()`.
- `None`/omitted preserves today's aggregate behavior exactly (sync example generation, doctor checks, global `configure-recipes`, non-interactive `main`).
- A selected list narrows collection and prompting to those recipes (a selected-but-disabled recipe yields `{}`); `offer_harness_env(..., recipe_ids=[...])` forwards the scope to `prompt_env_vars` and merges only prompted values into `ai-specs.env`. Example generation, the root `.envrc` managed block, and direnv handling remain global/aggregate.

### 2. Recipe-add wiring (`recipe-add.py`)
- In the Configure-now path (TTY): invoke the existing `config_wizard._dep_gate(recipe, console)` for `recipe.cli_deps` **before** env prompts, passing a real `rich.console.Console`. The gate's existing TTY behavior provides the explicit installation offer (decision 4).
- Pass `recipe_ids=[recipe_id]` to `collect_env_vars()` and `offer_harness_env()` so only the added recipe's env vars are prompted and written.
- On an unresolved gate, print explicit guidance — each missing dep's install URL plus a "required CLI dependencies are still missing" line — via `dep_install.resolve_install_plan`. `recipe-add` itself never calls an installer; `offer_and_install` stays inside `_dep_gate`'s explicit TTY offer.
- Non-TTY stays guidance-only: no dep gate, no env prompts, message pointing at `ai-specs configure-recipes`.
- Keep the existing pre-write TTY gate (`util.ensure_deps` + questionary availability) unchanged.

### 3. Hub menu (`hub.py`)
- Insert `(Action.CONFIGURE_RECIPES, "Configure recipes", "Set up recipe config, CLI deps, env vars")` into `_MENU` immediately after Recipes (12 entries total, per the pinned RED test). Recipes → Configure keeps working as the alias for the same action.

### 4. English UI strings
- Translate all user-facing CLI strings in the touched baseline paths: `env_scaffold.py` (env prompt headings, "¿Configurar ahora los valores?", secret input hint, direnv messages, main() warnings), `recipe-add.py` (success/next-step messages, errors), `config_wizard.py` ("Proyecto no inicializado"), `recipe-list.py` and `recipe-init.py` (uninitialized-project messages) — e.g. "Proyecto no inicializado" → "Project not initialized".
- **Unchanged by design** (decision 3): stored data values and technical artifacts — env var names and purpose strings, `ai-specs.env` / `ai-specs.env.example` / `.envrc` file formats, managed-block markers, manifest TOML content, exit codes, catalog `recipe.toml` data, generated sync outputs.

## Non-goals

- No change to aggregate behavior for `sync`, `example`, `doctor`, or global `configure-recipes`; `recipe_ids=None` must be a pure no-op default.
- No per-recipe selector inside the Hub/global Configure action, no `[[deps.env]]` schema, no config-schema changes (targeted selection is later refinement).
- No installer invocation from `recipe-add` itself; no silent installs anywhere.
- No changes to catalog recipes, provider repos, other worktrees (including `jinna-mcp-live`), or generated sync state.
- The **known unrelated stale Bitbucket fixture test failure is out of scope** — reproduce-and-ignore it as pre-existing; do not chase or "fix" it in this change.
- No revert of the prior worker's RED tests.

## Affected modules

| Path | Change |
|---|---|
| `lib/_internal/hub.py` | `_MENU` entry for `CONFIGURE_RECIPES` (Recipes submenu alias retained) |
| `lib/_internal/env_scaffold.py` | optional `recipe_ids` on collect/prompt/offer; English UI strings |
| `lib/_internal/recipe-add.py` | `_dep_gate` wiring, `[recipe_id]` env scoping, English UI strings |
| `lib/_internal/config_wizard.py` | English UI strings only (gate and global flow unchanged) |
| `lib/_internal/recipe-list.py` | English uninitialized-project message |
| `lib/_internal/recipe-init.py` | English uninitialized-project message |
| `tests/` (hub_tui, env_scaffold, recipe_add, config_wizard, recipe_list, recipe_init) | preserve prior RED tests; make them green without rewriting assertions |

## Existing RED tests (preserve — do not revert)

A prior worker left failing tests in this worktree that pin this contract; implement to them:

- `tests/test_hub_tui.py` — `_MENU` has exactly 12 entries, titles ordered with "Configure recipes" after "Recipes"; the entry carries a non-empty description.
- `tests/test_env_scaffold.py` — selected-only vs aggregate collection/prompting (`recipe_ids=["trello-mcp-workflow"]` vs omitted/`None`), disabled selected recipe → `{}`, `offer_harness_env` forwards `recipe_ids` and writes only prompted values, English main() warning (`! VAR has no value in ai-specs.env — run ai-specs configure-recipes`).
- `tests/test_recipe_add.py` — cli_deps recipe reaches `config_wizard._dep_gate` with a real Console; scoped `collect_env_vars`/`offer_harness_env` with `[recipe_id]`; unresolved gate → guidance on stdout, `offer_and_install` never called by recipe-add, env setup still runs scoped; non-TTY is guidance-only; "Project not initialized".
- `tests/test_recipe_list.py`, `tests/test_recipe_init.py` — "Project not initialized".

## Acceptance criteria

1. Hub main menu shows the top-level "Configure recipes" action (12 entries, positioned after Recipes) with a useful description; Recipes → Configure still runs the same action.
2. Add Recipe → Configure now (TTY) prompts only the added recipe's MCP env vars; only those prompted values are merged into `ai-specs.env` by that session.
3. With `recipe_ids` omitted or `None`, `collect_env_vars`, `prompt_env_vars`, and `offer_harness_env` behave exactly as before; sync example generation, doctor, and global configure-recipes outputs are unchanged.
4. Add Recipe → Configure now evaluates `recipe.cli_deps` through the existing `_dep_gate` before env prompts; missing deps are offered for explicit installation on TTY; installation is never silent; an unresolved gate prints explicit guidance while scoped env setup still completes; non-TTY prints guidance only.
5. No Spanish prose remains in user-facing output of hub/env_scaffold/recipe-add/config_wizard/recipe-list/recipe-init; stored data values, file formats, and exit codes are unchanged.
6. All prior-worker RED tests pass unmodified; the focused suites (hub_tui, env_scaffold, recipe_add, config_wizard, recipe_list, recipe_init) are green; the full suite shows no regressions beyond the known unrelated stale Bitbucket fixture failure (out of scope).

## Risks & rollback

- **Aggregate regression (highest)**: a scoping mistake could break sync example generation or doctor. Mitigation: additive optional parameter defaulting to `None`; existing aggregate tests must stay green; scoped behavior covered by the RED tests.
- **Translation drift into artifacts**: restrict edits to user-facing prose; file-format tests (examples, `.envrc`, manifest) guard the boundary.
- **Menu count/PTY breakage**: the RED test already pins the 12-entry menu; update only stale count offsets elsewhere.
- **Double gating**: `_dep_gate` complements the existing pre-write `ensure_deps` TTY gate; keep both, gate only `recipe.cli_deps`, and only in the Configure-now path.
- **Rollback**: single fix branch off `development`; no data migrations or stored-format changes — reverting the fix commits fully rolls back. Generated user state (env files) is untouched by design.

## Delivery notes

- This phase wrote only `proposal.md`. `tasks.md` is produced by the parent-dispatched Tasks phase and must record `Depth: light`.
- No commit, no push, no PR, and no archive in this phase; the worktree keeps the prior worker's RED tests intact.
- Per the project tracker gate, the parent must create/link a Trello card and complete `## Tracker` below **before** production Apply begins.

## Tracker

- **Status**: linked — card in list `In Progress` (linked before production Apply).
- card_id: `6aa2c7c637cfffb4424c052a`
- shortLink: `RZ0moUYj`
- url: https://trello.com/c/RZ0moUYj/123-baseline-hub-and-recipe-configuration-fixes
- list: In Progress
- Board: ai-specs-cli Roadmap (`69ec097f13e2d38ecd89a557`)
