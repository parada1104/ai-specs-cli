# Tasks: `hub-fixes` — full interactive consistency for the `ai-specs` TUI hub

Status: tasks (SDD)
Reference:
- `openspec/changes/hub-fixes/proposal.md`
- `openspec/changes/hub-fixes/design.md`

Source files (worktree `hub-fixes`):
- `lib/_internal/hub.py` — module under change
- `lib/skills-list.sh` — categorization fix
- `lib/_internal/doctor.py`, `lib/_internal/recipe-list.py`,
  `lib/_internal/skill-resolution.py`, `lib/_internal/util.py` — read-only reuse
- `tests/test_hub.py`, `tests/test_hub_tui.py` — test files

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 320–420 (hub.py ~260, skills-list.sh ~50, tests ~90, README ~20) |
| 400-line budget risk | Medium |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (Stage A: bugs 1–4) → PR 2 (Stage B: consistency helpers + Skills submenu + _SUB_ARGS deletion + README) |
| Delivery strategy | auto-chain |
| Chain strategy | stacked-to-main |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium
```

## Conventions for every task

- Strict TDD is active per `openspec/config.yaml` (`strict_tdd: true`). Runner: `./tests/run.sh`. Validation: `./tests/validate.sh`.
- Sequence for every code task: **RED → GREEN → TRIANGULATE → REFACTOR**. Record RED evidence (test fails against current source) before implementing.
- Dependency-free contract: `hub.py` MUST import with no `rich`/`questionary` on the path. Any helper that touches these libs imports them inside the function body, never at module scope. The existing `test_imports_without_third_party_deps` is the guard.
- Pure layer (module-level, dep-free) holds data builders: `_read_version`, `recipe_add_choices`, `recipe_remove_choices`, `categorize_skills`, `_skill_description`. Widget layer (lazy `import questionary`) holds `pick_one`, `pick_many`, `confirm_action`, `pause`.
- New sibling loads added once at import (stdlib-only modules, contract preserved):
  - `_recipes = _load_sibling("recipe-list")`
  - `_skillres = _load_sibling("skill-resolution")`
- Tasks are grouped by phase. Each task has a concrete file target, start/finish/verify, and rollback.

## Stage A — Bugs (independently shippable)

### A.0 Sibling-load scaffolding

#### A.0.1 Load recipe-list and skill-resolution as siblings
- File: `lib/_internal/hub.py` (lines 33–34 area).
- Add `_recipes = _load_sibling("recipe-list")` and `_skillres = _load_sibling("skill-resolution")` beside the existing `_util` / `_doctor`.
- Verify: `./tests/run.sh` — `test_imports_without_third_party_deps` still green (both modules are stdlib-only per design §1). No widget code added yet.
- Rollback: delete the two load lines.

### A.1 Bug 1 — importlib NameError

#### A.1.1 RED — assert module-level importlib exists
- File: `tests/test_hub.py` (dep-free suite).
- Add a guard test: `assert hasattr(hub, "importlib")` (module-scope), plus a behavioral guard that exercising the Agents branch path no longer raises `NameError`. The behavioral half may live in `tests/test_hub_tui.py` gated by `_has_deps()`.
- RED evidence: guard fails against current source (`importlib` is a local-only import in `_load_sibling`).

#### A.1.2 GREEN — module-level importlib.util
- File: `lib/_internal/hub.py` (top of file, near line 10–16).
- Add `import importlib.util` at module scope; delete the redundant local `import importlib.util` inside `_load_sibling` (lines 21→dead).
- Verify: `./tests/run.sh` — guards from A.1.1 green; full suite green.
- Rollback: revert the one-line module import.

### A.2 Bug 4 — version in StatusPanel

#### A.2.1 RED — _read_version pure function
- File: `tests/test_hub.py`.
- Tests: `_read_version()` returns raw `<home>/VERSION` contents (point `AI_SPECS_HOME` at a temp dir with a `VERSION` file); returns `"unknown"` when the file is absent.
- RED: `AttributeError: module 'hub' has no attribute '_read_version'`.

#### A.2.2 GREEN — _read_version + refactor _print_version
- File: `lib/_internal/hub.py`.
- Add pure `def _read_version() -> str:` reading `_util.ai_specs_home() / "VERSION"`, stripped; `"unknown"` when not a file.
- Refactor `_print_version()` to `print(_read_version())` (no behavior change).
- Verify: `./tests/run.sh` — A.2.1 green.

#### A.2.3 RED — StatusSummary.version populated
- File: `tests/test_hub.py`.
- Assert `status_summary(root).version == _read_version()`.
- RED: dataclass has no `version` field.

#### A.2.4 GREEN — add version field + populate
- File: `lib/_internal/hub.py` (lines 76–110).
- Add `version: str` to `StatusSummary`; set `version=_read_version()` in `status_summary()`.
- Verify: `./tests/run.sh` — A.2.3 green.

#### A.2.5 RED — non-interactive shows version
- File: `tests/test_hub.py` (`TestNonInteractiveStatus`).
- Assert `_run_noninteractive` output contains the version string.
- RED: no version line is printed today.

#### A.2.6 GREEN — non-interactive version line
- File: `lib/_internal/hub.py` (`_run_noninteractive`, lines 118–130).
- Print a `version:` line under the headline (before/with the `target:` line).
- Verify: `./tests/run.sh` — A.2.5 green.

#### A.2.7 RED — StatusPanel renders version row
- File: `tests/test_hub_tui.py` (`TestStatusPanelRender`, `_has_deps()`-gated).
- Assert rendered panel text contains the version string.
- RED: panel has no version row.

#### A.2.8 GREEN — version row in StatusPanel.render
- File: `lib/_internal/hub.py` (`StatusPanel.render`, lines 167–193).
- Add `table.add_row("version", self.summary.version)` as the first row, before `target`.
- Verify: `./tests/run.sh` — A.2.7 green.

### A.3 Bug 3 — recipe pickers

#### A.3.1 RED — recipe choice builders
- File: `tests/test_hub.py`.
- From synthetic `list_recipes`-shaped dicts, assert:
  - `recipe_add_choices` keeps only `status == "available"`, label includes name/id/version, value is id, skips `error` rows.
  - `recipe_remove_choices` keeps `status in {"installed","disabled"}`, label includes `[status]`, value is id.
  - Both return `[]` for empty/all-error input.
- RED: builders do not exist.

#### A.3.2 GREEN — pure builders
- File: `lib/_internal/hub.py` (pure layer near `_read_version`).
- Add `recipe_add_choices(recipes)` and `recipe_remove_choices(recipes)` per design §4. Dep-free.
- Verify: `./tests/run.sh` — A.3.1 green.

#### A.3.3 TRIANGULATE — id-only and mixed-status input
- File: `tests/test_hub.py`.
- Add a builder test with an `error (...)` row plus one available and one installed row; assert exact partitioning and that ids are preserved verbatim.
- Fix builder if a label/status-build glitch appears. Verify green.

#### A.3.4 RED — recipe Add picker sources from list_recipes (no text prompt)
- File: `tests/test_hub_tui.py` (`_has_deps()`-gated).
- Mock `pick_one` to capture the `options` it was handed and return one id; assert values are real catalog ids and `questionary.text` is not invoked for id entry.
- RED: current code calls `questionary.text("Recipe id:")`.

#### A.3.5 GREEN — route recipe Add/Remove through pick_one
- File: `lib/_internal/hub.py` (Recipes branch, lines 225–266).
- Add: `choices = recipe_add_choices(_recipes.list_recipes(target))`; if empty → print informational line + `pause()` + continue; else `rid = pick_one("Recipe to add:", choices)`; `None` → continue; else `runner.run(Action.RECIPES, extra=["add", rid])`. Remove symmetric with `recipe_remove_choices` and "No recipes installed to remove."
- Note: this task introduces `pick_one` and `pause` widgets early; define them now (Stage B will migrate the remaining sites onto them). Keep definition close to design §2.
- Verify: `./tests/run.sh` — A.3.4 green.

### A.4 Bug 2 — skills categorization (Python side)

#### A.4.1 RED — categorize_skills partitioning
- File: `tests/test_hub.py`.
- Build a temp project tree + stub `bundled-skills/` under a temp `AI_SPECS_HOME`; assert `categorize_skills(project_root, cli_home)` returns buckets where:
  - a bundled-named skill lands in `bundled` (not `local`);
  - a project-only skill lands in `local`;
  - a `.recipe/*/skills` skill lands in `recipe`;
  - a `.deps/*/skills` skill lands in `dep`.
- RED: function does not exist.

#### A.4.2 GREEN — categorize_skills + _skill_description
- File: `lib/_internal/hub.py` (pure layer).
- Add `categorize_skills(project_root, cli_home) -> dict[str, list[dict]]` using `_doctor.bundled_skill_names(cli_home)` and `_skillres.collect_skills(project_root)`. Add `_skill_description(path)` reading `SKILL.md` front-matter `description:` (mirrors `skills-list.sh`). Dep-free.
- Verify: `./tests/run.sh` — A.4.1 green.

### A.5 Bug 2 — skills-list.sh categorization at source

#### A.5.1 RED — bundled skills appear under "Bundled skills", not "Local skills"
- File: `tests/test_hub.py` (subprocess shell test, no deps).
- Init a temp project with bundled `skill-creator`/`skill-sync` copied into `ai-specs/skills/`; run `skills list`; assert output has a "Bundled skills" section containing both names and that those names do **not** appear under "Local skills".
- RED: current script lists bundled skills under "Local skills".

#### A.5.2 GREEN — bundled section + Local skip
- File: `lib/skills-list.sh` (lines 117–175 region).
- Compute `BUNDLED_IDS` from `$AI_SPECS_HOME/bundled-skills/*/` (fallback `skill-creator skill-sync`). Add a new "Bundled skills (CLI-shipped)" section before Local, emitting only names present in `BUNDLED_IDS` (reuse `skill_description`), `(none)` when empty. In the Local loop, `continue 2` when `name` is in `BUNDLED_IDS` (in addition to the existing `REGISTERED_IDS` skip). Keep "Available catalog skills" header unchanged.
- Verify: `./tests/run.sh` — A.5.1 green; `bash -n lib/skills-list.sh`.

## Stage B — Consistency pass

### B.1 Shared widget helpers

#### B.1.1 RED — helper signatures and empty-list contract
- File: `tests/test_hub.py` (dep-free shapes via monkeypatched `questionary`).
- Assert `pick_one("m", [])` returns `None` without invoking questionary; `pick_many("m", [])` returns `None`; `confirm_action("m")` returns `bool`; `pause()` returns `True` normally and `False` on `EOFError`.
- RED: helpers do not exist.

#### B.1.2 GREEN — define pick_one/pick_many/confirm_action/pause
- File: `lib/_internal/hub.py` (widget layer).
- Implement the four helpers per design §2. Each imports `questionary` inside its body. `pick_one`/`pick_many` build `questionary.Choice(title=label, value=value, checked=...)` from tuples and return `None` on empty options or abort.
- Verify: `./tests/run.sh` — B.1.1 green.

### B.2 Migrate remaining submenus + pause sites

#### B.2.1 RED — every input("Press Enter") goes through pause()
- File: `tests/test_hub_tui.py`.
- Grep-backed assertion: source has no bare `input("Press Enter` call outside `pause` (a structural test that reads `hub.py` text and asserts the helper is the only pause site). Plus a behavioral test that an aborted `pause` returns `False` → hub returns 0.
- RED: four inline `try: input(...) except EOFError` sites remain.

#### B.2.2 GREEN — route all pause sites through pause()
- File: `lib/_internal/hub.py` (lines 262–265, 327–330, 342–345, and the Recipes branch pause already added in A.3.5).
- Replace each inline `try: input("Press Enter…") except EOFError: return 0` with the `if not pause(): return 0; continue` pattern.
- Verify: `./tests/run.sh` — B.2.1 green.

#### B.2.3 GREEN — Recipes submenu uses pick_one for submenu selection
- File: `lib/_internal/hub.py` (lines 225–238).
- Replace the inline `questionary.select` for the Recipes sub-choice with `pick_one` over `(label, value)` tuples. Behavior unchanged.
- Verify: `./tests/run.sh` — Recipes TUI tests green.

### B.3 Skills interactive submenu

#### B.3.1 RED — Skills submenu shows categorized headers
- File: `tests/test_hub_tui.py` (PTY, `_has_deps()`-gated).
- Extend `TestHubPTYE2E`: navigate to Skills, assert output shows "Bundled", "Local / vendored", "Provided by recipes / catalog", and no `Traceback`.
- RED: Skills currently delegates to `ai-specs skills list`.

#### B.3.2 GREEN — interactive Skills submenu (list + inspect)
- File: `lib/_internal/hub.py` (`Action.SKILLS` branch, near line 204–206).
- Replace the delegate with an interactive submenu mirroring Recipes: "List skills (categorized)" renders buckets via `rich` grouped with the headers from design §3; "Inspect a skill" uses `pick_one` over all skill ids (label shows id + origin) and prints the skill's `SKILL.md` path + description; "Back". Reuse `categorize_skills` and `_skill_description`. Empty buckets print `(none)`; empty-picker case prints informational line + `pause()` + continue.
- Verify: `./tests/run.sh` — B.3.1 green.

### B.4 Delete _SUB_ARGS indirection

#### B.4.1 GREEN — remove _SUB_ARGS and the extra-args lookup
- File: `lib/_internal/hub.py` (lines 204–206 and the `_extra = _SUB_ARGS.get(...)` caller in the dispatch loop).
- Delete the `_SUB_ARGS` dict and the `_extra = _SUB_ARGS.get(...)` lookup; call `runner.run(action)` with no extra for the remaining plain-delegate actions (Sync, Doctor, Rules audit, Upgrade, Help).
- Verify: `./tests/run.sh` — full suite green; sync/doctor/upgrade/help paths still delegate correctly.

### B.5 Final verification and docs

#### B.5.1 REFACTOR — consolidate pure/widget layers
- File: `lib/_internal/hub.py`.
- Group pure builders together and widget helpers together for readability; no behavior change.
- Verify: `./tests/run.sh` green.

#### B.5.2 GREEN — README update for user-visible hub changes
- File: `README.md`.
- Document: StatusPanel/non-interactive now show a version row; Recipes Add/Remove are pickers; Skills is an interactive categorized submenu; bundled skills are labeled correctly in `skills list`.
- Verify: README rendered sections match the four user-visible changes; `./tests/validate.sh` clean (no doc test, but py_compile/bash -n/run.sh all pass).

#### B.5.3 Final validation
- Run `./tests/validate.sh` (py_compile, bash -n, `./tests/run.sh`).
- Run `git -C .worktrees/hub-fixes diff --stat` to confirm changed-line estimate against the forecast.
- All acceptance criteria from `proposal.md` §Success criteria checked.

## Open decisions before apply

1. Confirm delivery strategy (`auto-chain` vs `single-pr`) and the Stage A → Stage B split with the parent before `sdd-apply`, since 400-line budget risk is Medium and chaining is recommended.