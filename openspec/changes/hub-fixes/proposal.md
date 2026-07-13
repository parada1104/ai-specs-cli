# Proposal: `hub-fixes` — full interactive consistency for the `ai-specs` TUI hub

## Why (motivation)

The `tui-hub` change shipped a working front door (`lib/_internal/hub.py`), but real
use surfaced two problems:

1. **Correctness bugs** — selecting some menu items crashes or shows misleading
   information (details below).
2. **UX inconsistency** — the hub mixes three interaction styles: proper
   `questionary` widgets (menu, Agents checkbox, Recipes submenu), raw
   `questionary.text()` free-text prompts (recipe id entry), and plain-text
   dumps delegated to shell subcommands (Skills → `ai-specs skills list`). The
   experience is jarring: some choices are guided pickers, others force the user
   to hand-type identifiers they cannot see, and others drop them into scrolling
   text with no interaction at all.

The real goal of this change is to make the **entire hub uniformly interactive and
consistent**: every choice a user makes inside the hub is expressed through a
`questionary` widget (`select` for one-of, `checkbox` for many-of, `confirm` for
yes/no), sourced from live project/catalog state — never a blind text prompt and
never an inert text dump where a selection is the real intent. The 4 confirmed
bugs are the seed; consistency is the deliverable.

## Confirmed bugs (root causes, grounded)

1. **`NameError` in the Agents section.** `hub.py:277` calls
   `importlib.util.spec_from_file_location(...)`, but `importlib` is only imported
   *locally* inside `_load_sibling()` (`hub.py:21` `import importlib.util`); it is
   **not** a module-level import. Selecting **Agents** raises
   `NameError: name 'importlib' is not defined` and shows a traceback.

2. **Skills listing misidentifies bundled skills and mixes in recipe/catalog
   skills.** The hub delegates **Skills** to `ai-specs skills list` (`hub.py:205`
   `_SUB_ARGS[Action.SKILLS] = ["list"]` → `skills-list.sh`). That script's
   "Local skills (`ai-specs/skills/`)" section (`skills-list.sh:117-151`) lists
   *every* directory under `ai-specs/skills/` (minus registered deps) as "Local",
   including CLI-shipped bundled skills (`skill-creator`, `skill-sync`) — there is
   a source of truth for those (`doctor.py:21` `bundled_skill_names()`) that the
   script ignores. Separately, the "Available catalog skills" section
   (`skills-list.sh:154-175`) surfaces recipe/catalog-provided skills inside the
   *skills* view, conflating recipe content with the project's own skills.

3. **Recipes submenu uses blind text prompts.** In the Recipes submenu, **Add**
   and **Remove** call `questionary.text("Recipe id:")` (`hub.py:242-256`),
   forcing the user to hand-type a recipe id from memory even though
   `recipe-list.py:46` `list_recipes(project_root)` already returns every catalog
   recipe with `id`, `name`, `version`, and `status` (available/installed/disabled).

4. **Version not shown in the StatusPanel.** `_print_version()` exists
   (`hub.py:196-201`, reads `<home>/VERSION`) and there is a **Version** menu item,
   but the always-visible `StatusPanel` (`hub.py:163-193`) and its backing
   `StatusSummary` (`hub.py:76-85`) never carry or render the CLI version. The user
   must pick a menu item to learn which version they are running.

## Intent

Fix the 4 bugs and, in the same pass, make every hub interaction consistent and
data-driven with `questionary`:

- One interaction vocabulary: `select` (one-of), `checkbox` (many-of),
  `confirm` (yes/no), `text` **only** for genuinely free-form input (e.g. a project
  name), never for choosing an existing identifier.
- Every "pick an existing thing" flow (recipes to add/remove, skills to inspect,
  agents to enable) is populated from live project/catalog state via the existing
  in-process helpers, so users select from what actually exists.
- Skills becomes a first-class interactive submenu (mirroring Recipes) that clearly
  categorizes skills as **bundled** (CLI-shipped), **local/vendored** (project), and
  **recipe/catalog-provided**, instead of a delegated text dump.
- The StatusPanel always shows the CLI version.

## Scope (in)

1. **Bug 1 — `importlib` import.** Add a module-level `import importlib.util` in
   `hub.py`; the Agents section stops crashing. (Also remove the now-redundant
   local import in `_load_sibling` or keep it harmless — decided in design.)
2. **Bug 4 — version in StatusPanel.** Add a `version` field to `StatusSummary`
   (`hub.py:76-85`), populate it in `status_summary()` from the same source as
   `_print_version()` (`<home>/VERSION`), and render a version row in
   `StatusPanel.render()` and the non-interactive summary (`_run_noninteractive`).
3. **Bug 3 — interactive recipe add/remove.** Replace `questionary.text()` recipe
   id prompts (`hub.py:242-256`) with `questionary` pickers sourced from
   `list_recipes(target)`: **Add** offers recipes not yet installed; **Remove**
   offers currently installed recipes. Show `name`/`id`/`status` in the choices.
4. **Bug 2 — accurate, categorized skills view.** Make **Skills** an interactive
   submenu that presents skills grouped/labeled by origin — **bundled** (from
   `bundled_skill_names()`), **local/vendored project** skills, and
   **recipe/catalog-provided** skills — so bundled skills are never mislabeled
   "local" and recipe skills are never conflated with project skills. Fix the same
   categorization at its source in `skills-list.sh` so the delegated
   `ai-specs skills list` output is correct too.
5. **Hub-wide interaction consistency pass.** Audit every hub action and submenu
   (Sync, Doctor, Agents, Skills, Recipes, Rules audit, Upgrade, Version, Help,
   Init wizard, Quit) and ensure each user choice uses the shared `questionary`
   vocabulary. Establish a small, reused helper set (e.g. a "pick one from a live
   list", "confirm", "pause/return" pattern) so future actions stay consistent.
6. **Tests (strict TDD, `./tests/run.sh`).** Unit tests for: Agents section no
   longer raising `NameError`; `StatusSummary.version` populated and rendered;
   recipe add/remove choice construction from `list_recipes`; skills
   categorization (bundled vs local vs recipe). PTY/E2E coverage extending the
   existing hub TUI tests where an interactive widget replaced a text prompt.
7. **README update** for any user-visible change to hub behavior.

## Non-goals

- NOT adding new top-level hub actions or subcommands (compose existing behavior).
- NOT changing any subcommand's own flags/output *except* the `skills list`
  categorization fix required by Bug 2.
- NOT migrating away from `rich`+`questionary` (no Textual).
- NOT turning the hub into a write surface beyond what it already does (Agents
  manifest edit stays; mutations still delegate to subcommands).
- NOT reworking the recipe/skill data model, catalog layout, or manifest schema.
- NOT redesigning the visual theme of the StatusPanel beyond adding the version row.

## Affected areas (files / modules)

- `lib/_internal/hub.py` — Bug 1 import; Bug 4 StatusSummary/StatusPanel/noninteractive;
  Bug 3 recipe pickers; Bug 2 Skills submenu; consistency helpers.
- `lib/skills-list.sh` — Bug 2 categorization at the source (bundled vs local vs
  recipe/catalog), using `bundled_skill_names()` as the source of truth.
- `lib/_internal/recipe-list.py` — reused read-only (`list_recipes`); no change
  expected unless a thin accessor is needed.
- `lib/_internal/doctor.py` — reused read-only (`bundled_skill_names`); no change
  expected.
- `tests/test_hub.py`, `tests/test_hub_tui.py` (or the existing hub TUI test) —
  new/extended coverage.
- `README.md` — hub behavior docs if user-visible.

## Risks

- **Interactive flows are hard to test.** Mitigate by keeping choice *construction*
  in pure, unit-testable functions (build choices from `list_recipes` / skill
  categorization), and gating PTY/E2E behind the existing `_has_deps()` pattern
  used by the current hub TUI tests.
- **`skills-list.sh` categorization change alters delegated output.** It is a
  user-visible text change; keep section names stable where possible and update
  any test asserting the old wording. Doctor's bundled-skill checks are the
  contract for "bundled".
- **Empty/degraded state** (no catalog, no recipes installed, missing VERSION,
  invalid manifest): every new picker MUST handle empty lists gracefully (inform +
  return, never crash), matching the existing "Manifest not found" guard
  (`hub.py:271`).
- **questionary not vendored** — the on-demand import path is unchanged; no new
  dependency surface introduced.

## Rollback

Each item is independently revertible; none changes public subcommand contracts
except the `skills list` wording:

1. Bug 1: revert the one-line module import (trivial).
2. Bug 4: revert `StatusSummary.version` field + render row.
3. Bug 3: restore `questionary.text()` recipe prompts.
4. Bug 2: revert `skills-list.sh` section logic and the hub Skills submenu to the
   delegated `skills list` call.
5. Consistency helpers are additive; deleting them reverts callers to prior inline
   widgets.

## Success criteria

- Selecting **Agents** never raises `NameError`; the checkbox editor works end to end.
- The StatusPanel (interactive) and the non-interactive summary always show the CLI
  version.
- **Recipes → Add/Remove** present a `questionary` picker of real recipes (add =
  not-installed, remove = installed) sourced from `list_recipes`; no free-text id entry.
- **Skills** clearly distinguishes bundled, local/vendored, and recipe/catalog
  skills; bundled skills are labeled bundled (not "local"); recipe skills are not
  presented as project skills — in both the hub submenu and `ai-specs skills list`.
- Every user choice in every hub submenu is a `questionary` widget from the shared
  vocabulary; no blind text prompt remains for selecting existing entities.
- Empty/degraded states are handled without tracebacks.
- `./tests/run.sh` and `./tests/validate.sh` pass; new unit tests fail against the
  pre-fix code (RED) and pass after (GREEN).
- README reflects any user-visible hub change.

## Proposal question round (for user review before finalizing)

The executor cannot prompt the user directly; these are the product decisions the
parent SHOULD confirm. Working assumptions are stated so silence still yields a
sensible default.

1. **Skills submenu depth.** Should **Skills** become a full add/remove/inspect
   submenu (like Recipes), or an interactive *categorized listing* only for now,
   with add/remove still delegated? *Assumption:* interactive categorized listing
   + inspect this pass; add/remove parity with Recipes only if it fits the review
   budget.
2. **"Recipe skills" placement.** In the corrected Skills view, should
   recipe/catalog-provided skills appear as a clearly separated section, or be
   hidden from Skills entirely (belonging to Recipes)? *Assumption:* keep them, but
   in a distinctly labeled "Provided by recipes / catalog" section.
3. **Version source & format.** Show the raw `VERSION` file contents, or a richer
   line (e.g. version + install path)? *Assumption:* raw version string, single
   `version` row in the panel.
4. **"Press Enter to return" pauses.** Keep the current `input()` pause between
   actions, or convert to a `questionary` confirm/continue for full consistency?
   *Assumption:* keep a lightweight pause but route it through a single shared
   helper so behavior is uniform.
5. **Scope ceiling.** Is a hub-wide consistency pass (all submenus) in-scope for a
   single change, or should it be limited to the 4 bug areas + Recipes/Skills, with
   the rest as a follow-up? *Assumption:* full consistency pass, but staged so the
   4 bugs land first and are independently shippable.

## Classification

Per `openspec/config.yaml` decision matrix, this is a **domain_change**: it changes
user-facing interaction behavior across multiple hub surfaces and benefits from a
design doc defining the shared interaction vocabulary and the skill-categorization
contract. Recommended flow: **proposal → design → tasks → apply → verify**, worktree
required, strict TDD (`./tests/run.sh`).
