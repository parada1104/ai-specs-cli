# Tasks: Recipe config wizard + CLI dependency management + `.envrc` scaffolding

Change: tui-hub (recipe-config extension)
Depends on: `openspec/changes/tui-hub/design-recipe-config.md` (approved)
Branch: `tui-hub` (worktree `.worktrees/tui-hub`)
Strict TDD: `true` — test runner `./tests/run.sh` (unittest discovery). Every task follows RED (write failing test) → GREEN (minimal impl) → TRIANGULATE (add a second case) → REFACTOR. `./tests/validate.sh` runs py_compile + `bash -n`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 1300–1600 (additions ~1100–1350, deletions ~50–100) |
| 400-line budget risk | High |
| Chained PRs recommended | No |
| Suggested split | single PR (size:exception approved) — commits grouped by phase P1→P6 |
| Delivery strategy | exception-ok |
| Chain strategy | size-exception |

```text
Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: size-exception
400-line budget risk: High
```

**Rationale:** the change adds 9 new files (4 library modules + 1 shell shim + 4 test suites) and modifies ~17 existing files (schema, doctor, hub, init_tui, bin/ai-specs, 6 catalog recipes, 4 docs, 4 existing test files). Estimated additions exceed 1100 lines, well over the 400-line single-PR budget. However, the session delivery strategy explicitly approved `size:exception` (proposal design decision 5: "Same branch, single PR"). The change extends the `tui-hub` branch as one PR with commits grouped by phase: schema → catalog → dep-check → wizard → config-write → envrc → hub/init → docs. This avoids a cross-branch dependency on `init_tui.py` which is a `tui-hub` deliverable. Each phase leaves `./tests/run.sh` green, providing natural commit boundaries with clean start/finish/verify/rollback per phase (proposal rollback items map 1:1 to phases).

**Files created (9):** `lib/_internal/dep_check.py`, `lib/_internal/config_wizard.py`, `lib/_internal/recipe-config-write.py`, `lib/_internal/envrc-scaffold.py`, `lib/recipe-config.sh`, `tests/test_dep_check.py`, `tests/test_config_wizard.py`, `tests/test_recipe_config_write.py`, `tests/test_envrc_scaffold.py`

**Files modified (17):** `lib/_internal/recipe_schema.py`, `lib/_internal/recipe-read.py`, `lib/_internal/doctor.py`, `lib/_internal/hub.py`, `lib/_internal/init_tui.py`, `bin/ai-specs`, `lib/_internal/recipe_init.py` (ENV_REFERENCE_RE reuse only — no edit, just import), `tests/test_recipe_schema.py`, `tests/test_doctor.py`, `tests/test_hub_tui.py`, `tests/test_init_tui.py`, `catalog/recipes/git-pr-flow/recipe.toml`, `catalog/recipes/gitlab-mr-flow/recipe.toml`, `catalog/recipes/bitbucket-pr-flow/recipe.toml`, `catalog/recipes/trello-mcp-workflow/recipe.toml`, `catalog/recipes/vault-canonical-store/recipe.toml`, `catalog/recipes/worktree-flow/recipe.toml`, `docs/recipe-schema.md`, `docs/recipes-catalog.md`, `docs/ai-specs-toml.md`, `README.md`, `CHANGELOG.md`

---

## P1 — Schema: `[[deps.cli]]` parsing + serialization

**Goal:** add the `CliDep` dataclass and `_parse_cli_deps` parser to `recipe_schema.py`, wire `Recipe.cli_deps` into `validate_recipe_toml`, and serialize `cli_deps` in `recipe_to_dict`. Purely additive — recipes without `[deps]` parse unchanged. RED → GREEN → TRIANGULATE per test.

### Task P1.1 — `CliDep` dataclass + `_parse_cli_deps` + `Recipe.cli_deps` (RED → GREEN → TRIANGULATE)

**Files:**
- Modify `lib/_internal/recipe_schema.py`
- Modify `tests/test_recipe_schema.py` (add `CliDepParsingTests`)

**`recipe_schema.py` changes (design §2):**
- Add `_opt_str(data, key, ctx) -> str` helper next to `_require_string` (line ~159): returns `""` when key absent; raises `RecipeValidationError` when present but not a string. Eliminates triplication across three optional-string fields.
- Add `CliDep` dataclass after `ConfigSchema` (line 122), before `InitWorkflow` (line 124):
  ```python
  @dataclass
  class CliDep:
      binary: str
      purpose: str
      required: bool = True
      install_url: str = ""
      version_check: str = ""
      min_version: str = ""
  ```
- Add `cli_deps: list[CliDep] = field(default_factory=list)` to `Recipe` (after `config_schema` at line 150, before `init` at line 151 — keeps declared-before-optional grouping).
- Add `_parse_cli_deps(raw, context) -> list[CliDep]` (mirrors `_parse_config` discipline — design §2):
  - `raw is None` → `[]`
  - Non-list → `RecipeValidationError("expected array of tables")`
  - Per item: non-dict → raise; unknown key → raise (allowed set: `binary, purpose, required, install_url, version_check, min_version`); `binary`/`purpose` via `_require_string`; `required` via `isinstance(bool)` guard defaulting `True`; `install_url`/`version_check`/`min_version` via `_opt_str`.
- Wire into `validate_recipe_toml` (line ~598): `deps_table = data.get("deps", {})` (guard non-dict → `{}`), then `cli_deps=_parse_cli_deps(deps_table.get("cli"), "[deps.cli]")` in the `Recipe(...)` kwargs.

**`test_recipe_schema.py` — `CliDepParsingTests` (RED first):**
- `test_valid_cli_dep_parses` — full `[[deps.cli]]` block (all 6 fields) → `Recipe.cli_deps[0]` fields exact.
- `test_optional_defaults` — only `binary` + `purpose` → `required is True`, three strings `""`.
- `test_missing_binary_raises` — `[[deps.cli]]` without `binary` → `RecipeValidationError`, message contains `binary`.
- `test_missing_purpose_raises` — without `purpose` → raises, message contains `purpose`.
- `test_unknown_key_raises` — `[[deps.cli]]` with `foo = "x"` → raises, message contains `foo`.
- `test_required_non_bool_raises` — `required = "yes"` → raises.
- `test_absent_deps_yields_empty_list` (TRIANGULATE backward compat) — recipe with no `[deps]` section → `cli_deps == []`.

**Acceptance criteria:**
- `./tests/run.sh` green (all `CliDepParsingTests` pass).
- An existing recipe without `[deps]` still parses identically (backward compat proven by `test_absent_deps_yields_empty_list`).
- Unknown-key rejection message names the offending key (matches `_parse_config` error phrasing).

**Estimated lines:** ~80 modified (`recipe_schema.py` ~50; `test_recipe_schema.py` ~30)

---

### Task P1.2 — `recipe_to_dict` serialization (RED → GREEN)

**Files:**
- Modify `lib/_internal/recipe-read.py`
- Modify `tests/test_recipe_schema.py` (add to `CliDepParsingTests`)

**`recipe-read.py` changes (design §2):**
- Add `"cli_deps"` key to the dict returned by `recipe_to_dict` (sibling of `"provides"`):
  ```python
  "cli_deps": [
      {"binary": d.binary, "purpose": d.purpose, "required": d.required,
       "install_url": d.install_url, "version_check": d.version_check,
       "min_version": d.min_version}
      for d in recipe.cli_deps
  ],
  ```

**Test (RED → GREEN, add to `CliDepParsingTests`):**
- `test_recipe_to_dict_serializes_cli_deps` — build a `Recipe` with `cli_deps`, call `recipe_to_dict`, assert the `cli_deps` list shape (list of dicts with the 6 keys). Guards the display contract used by dep-check panels.

**Acceptance criteria:**
- `recipe_to_dict` output includes `"cli_deps"` key (empty list when no deps).
- `./tests/run.sh` green.

**Estimated lines:** ~20 modified (`recipe-read.py` ~12; test ~8)

---

### Task P1.3 — Catalog `[[deps.cli]]` blocks (GREEN — additive data)

**Files:**
- Modify `catalog/recipes/git-pr-flow/recipe.toml`
- Modify `catalog/recipes/gitlab-mr-flow/recipe.toml`
- Modify `catalog/recipes/bitbucket-pr-flow/recipe.toml`
- Modify `catalog/recipes/trello-mcp-workflow/recipe.toml`
- Modify `catalog/recipes/vault-canonical-store/recipe.toml`
- Modify `catalog/recipes/worktree-flow/recipe.toml`

**Blocks to add (design §9 — exact TOML):**

Insert a `[deps]` section with `[[deps.cli]]` array-of-tables after `[[capabilities]]`/`[[hooks]]` and before the first `[config.*]` table in each recipe.

- `git-pr-flow` → `gh`: binary, purpose, required=true, install_url=https://cli.github.com/, version_check=`gh --version`, min_version=`2.0.0`
- `gitlab-mr-flow` → `glab` (required, install_url, version_check=`glab --version`, min_version=`1.0.0`) + `jq` (required, install_url=https://jqlang.github.io/jq/download/)
- `bitbucket-pr-flow` → `bb`: binary, purpose, required=true, install_url=https://bitbucket.org/atlassian/bb
- `trello-mcp-workflow` → `npx`: required, install_url=https://nodejs.org/en/download, version_check=`npx --version`, min_version=`8.0.0`
- `vault-canonical-store` → `npx`: required, install_url=https://nodejs.org/en/download, version_check=`npx --version`, min_version=`8.0.0`
- `worktree-flow` → `git`: required, install_url=https://git-scm.com/downloads, version_check=`git --version`, min_version=`2.20.0`

`tdd-flow` — **no block** (test command is config-driven; no fixed binary).

**Tests (extend `CliDepParsingTests`, catalog round-trip):**
- `test_catalog_git_pr_flow_has_cli_deps` — load `git-pr-flow` recipe from catalog, assert `cli_deps[0].binary == "gh"` and `required is True`.
- `test_catalog_gitlab_mr_flow_has_two_deps` (TRIANGULATE multi-dep) — assert `gitlab-mr-flow` has 2 `cli_deps` entries, binaries `glab` and `jq`.
- `test_catalog_worktree_flow_has_git_dep` — assert `worktree-flow` `cli_deps[0].binary == "git"`.
- `test_recipe_conflicts_tolerates_deps_block` — run `recipe-conflicts.py` on a recipe with `[[deps.cli]]` → no crash (additive parsing; conflict logic ignores the block). Guard against the risk flagged in proposal risks.

**Acceptance criteria:**
- All 6 catalog recipes with `[[deps.cli]]` blocks parse cleanly via `validate_recipe_toml`.
- Recipes without the block (`tdd-flow` and any others) remain valid.
- `recipe-conflicts.py` does not crash on recipes containing `[deps]`.
- `./tests/validate.sh` clean (`test_recipe_schema` round-trips every catalog recipe).

**Estimated lines:** ~80 modified (6 files × ~8–13 lines each; tests ~25)

---

## P2 — Dependency check: `dep_check.py` + Doctor integration

**Goal:** a reusable guidance-only CLI dependency checker (`shutil.which` for existence, `subprocess(shell=True)` for version_check) and a Doctor WARN integration that never changes the exit code.

### Task P2.1 — `dep_check.py` core: `DepResult`, `check_cli_deps`, `check_project_deps` (RED → GREEN → TRIANGULATE)

**Files:**
- Create `lib/_internal/dep_check.py`
- Create `tests/test_dep_check.py`

**`dep_check.py` contents (design §3):**

`DepResult` dataclass:
```python
@dataclass
class DepResult:
    binary: str
    found: bool
    version: str          # "" if unknown/not run
    ok: bool              # found AND (min_version satisfied or no min_version)
    install_url: str
    purpose: str
    required: bool
    recipe_id: str = ""   # populated by check_project_deps
    detail: str = ""      # human note, e.g. "found 1.9.0 < required 2.0.0"
```

Public functions:
- `check_cli_deps(recipe: Recipe) -> list[DepResult]` — one `DepResult` per `recipe.cli_deps` entry; never raises. `ok = found and version_ok`.
- `check_project_deps(project_root: Path) -> list[DepResult]` — load manifest, resolve enabled `[recipes.*]`, load each catalog recipe, aggregate `check_cli_deps` across all. `recipe_id` populated. No de-dup (same binary across recipes yields separate rows for recipe-scoped guidance).

Internal helpers:
- `_which(binary) -> bool` — `shutil.which(binary) is not None` (POSIX-safe, no shell).
- `_run_version_check(cmd) -> str` — `subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=5)`. Returns stdout+stderr; `""` on any exception. Never raises.
- `_parse_version(text) -> tuple[int, ...]` — extract first `r"\d+(?:\.\d+)*"`, split on `.`, int-tuple. `()` when no numeric token.
- `_version_ge(have, want) -> bool` — zero-pad shorter tuple, lexicographic compare.

`ok` computation: `found` → if `min_version` and `version` parseable → `_version_ge`; else `True` (found, no comparable version). Unparseable version with `min_version` set → `ok=True`, `detail="version unknown"` (never blocks — guidance-only).

`_load_sibling(name)` — `importlib.util.spec_from_file_location` by absolute path (mirror `hub.py` 19–30) to load `recipe-read.py`, `toml-read.py`.

`_catalog_dir() -> Path` — `AI_SPECS_HOME` env or `Path(__file__).resolve().parents[2]`, then `/catalog/recipes`.

**`test_dep_check.py` (RED first, mock `shutil.which` and `_run_version_check`):**
- `test_found_binary_ok` — `which` returns a path, no `min_version` → `found=True, ok=True`.
- `test_missing_binary_not_ok` — `which` returns `None` → `found=False, ok=False`.
- `test_version_meets_min` — `_run_version_check` returns `"gh 2.40.0"`, `min_version="2.0.0"` → `ok=True`.
- `test_version_below_min` — output `"gh 1.9.0"`, `min_version="2.0.0"` → `ok=False`, `detail` names the shortfall.
- `test_unparseable_version_does_not_block` — output `"weird"`, `min_version="2.0.0"`, found → `ok=True`, `detail` notes unknown version.
- `test_optional_missing_not_failure` — `required=False`, missing → `ok=False` but `required is False` (caller maps to INFO).
- `test_version_check_subprocess_error_degrades` — patch `_run_version_check` to raise → swallowed, `version==""`, no exception.
- `test_check_project_deps_aggregates` — temp project with two enabled recipes (temp catalog fixture), assert one `DepResult` per declared dep with `recipe_id` set.
- `test_version_ge` (TRIANGULATE pure unit) — `2.0` vs `2.0.0` (equal); `10` vs `9` (true); `2.0` vs `2.1` (false).
- `test_parse_version` — `()` for no-numeric; `(2, 40, 0)` for `"gh 2.40.0"`.

**Acceptance criteria:**
- `check_cli_deps` never raises on subprocess failure (degrades to `found=False`/`version=""`).
- `required=False` deps never produce a `True` failure — `ok=False` but `required is False`.
- `check_project_deps` populates `recipe_id` per row.
- `./tests/run.sh` green.

**Estimated lines:** ~230 (`dep_check.py` ~110; `test_dep_check.py` ~120)

---

### Task P2.2 — Doctor integration: `_check_recipe_cli_deps` (RED → GREEN)

**Files:**
- Modify `lib/_internal/doctor.py`
- Modify `tests/test_doctor.py`

**`doctor.py` changes (design §3):**
- Add `_check_recipe_cli_deps(self) -> None` method, registered in `run()` **append after `_check_enabled_agents`** (runs last; never reorders existing checks — keeps `test_doctor` line expectations stable except for new rows it explicitly adds):
  - `data = self._load_manifest()`; `recipes = data.get("recipes", {})` — bail if absent/non-dict/empty.
  - `try: results = self._collect_recipe_dep_results()` except `Exception: return` (degrade silently; never break `ai-specs doctor`).
  - For each `r`: `r.ok` → `Check(Severity.OK, "recipe-dep", f"{r.binary} available for {r.recipe_id}")`; `r.required and not r.ok` → `Check(Severity.WARN, "recipe-dep", f"{r.binary} missing/unusable for {r.recipe_id}: {r.purpose}", guidance=r.install_url or "install the required CLI")`; `not r.required and not r.ok` → `Check(Severity.INFO, "recipe-dep", f"optional {r.binary} not found for {r.recipe_id}: {r.purpose}", guidance=r.install_url)`.
- `_collect_recipe_dep_results()` loads `dep_check.py` + `recipe-read.py` via the proven `importlib` seam (template: `_brief_render_disabled` at 273–290). Resolves `catalog_dir = AI_SPECS_HOME / "catalog" / "recipes"`. Only enabled recipes checked (`entry.get("enabled") is True`). Delegates to `check_project_deps(self.root)` or inlines the loop.
- **Never raises; emits at most WARN** → exit code unchanged (WARN ≠ ERROR; `Doctor.run()`'s `any(ERROR)` return untouched).

**`test_doctor.py` (extend — RED first):**
- `test_recipe_cli_deps_warn_when_missing` — temp project with an enabled recipe whose required CLI is absent (patch `shutil.which` to miss `gh`); assert a `WARN recipe-dep` row appears; assert exit code is `0` (unchanged — no ERROR).
- `test_recipe_cli_deps_info_when_optional_missing` — `required=False` dep missing → `INFO recipe-dep` row.
- `test_recipe_cli_deps_ok_when_found` — patch `which` to find the binary → `OK recipe-dep` row.
- `test_doctor_no_crash_when_no_recipes` — manifest with no `[recipes]` → no recipe-dep rows, no exception.
- `test_doctor_exit_code_unchanged` (TRIANGULATE) — assert `Doctor.run()` returns `0` when only WARN/INFO recipe-dep rows are present (guards the exit-code contract).

**Acceptance criteria:**
- `ai-specs doctor` on a project with an enabled recipe whose required CLI is missing emits a `WARN` row with install guidance.
- Exit code unchanged (non-zero only on ERROR — proven by `test_doctor_exit_code_unchanged`).
- `_check_recipe_cli_deps` never raises (degrades to "no recipe-dep rows" on load failure).
- Existing `test_doctor.py` checks remain green (new registered check runs last, never reorders).
- `./tests/run.sh` green.

**Estimated lines:** ~80 modified (`doctor.py` ~50; `test_doctor.py` ~30)

---

## P3 — Config wizard: `config_wizard.py` + `recipe-config-write.py` + shim

**Goal:** the questionary-driven per-field config wizard and the surgical comment-preserving write-back path. The wizard reuses `ConfigField` metadata for validation; the writer guards with `tomllib.loads` + restore-on-failure.

### Task P3.1 — `recipe-config-write.py`: surgical line replacement (RED → GREEN → TRIANGULATE)

**Files:**
- Create `lib/_internal/recipe-config-write.py`
- Create `tests/test_recipe_config_write.py`

**`recipe-config-write.py` contents (design §5):**

`RecipeConfigWriteError(Exception)` — module-local, so callers distinguish write failure from questionary cancel.

`update_recipe_config(manifest_path: Path, recipe_id: str, values: dict) -> None`:
- `values == {}` → no-op (file unchanged, content/mtime identical).
- `text = manifest_path.read_text()`; `lines = text.splitlines(keepends=True)`.
- 1. Locate `[recipes.<id>]` header line (exact match via `init_tui._toml_key(recipe_id)`, tolerant of surrounding blanks). Not found → APPEND path: build `\n[recipes.<id>]\n...\n[recipes.<id>.config]\n<k=v>\n` (safety net; wizard normally writes for already-enabled recipes).
- 2. Locate `[recipes.<id>.config]` within the recipe's region (from recipe header to next top-level `[section]` that is NOT ` recipes.<id>.*`). Not found → INSERT a `"[recipes.<id>.config]"` header at end of recipe region, treat all values as inserts.
- 3. Determine config block span: from config header to next header line (`"["`) or EOF.
- 4. For each `key, val` in `values` (sorted for determinism): scan block span for line matching `r'^\s*<key>\s*='` → replace that single line with `"<key> = <toml_value(val)>\n"` preserving leading indentation; else record as pending insert.
- 5. Insert pending keys before block's terminating header (or at EOF) as `"<key> = <toml_value(val)>\n"`.
- 6. `new_text = "".join(lines)`.
- 7. `try: tomllib.loads(new_text)` except `TOMLDecodeError`: `manifest_path.write_text(original)`; `raise RecipeConfigWriteError`. Else: `manifest_path.write_text(new_text)`.

Values serialized through `toml_write.toml_value` (bool → `true/false`, str → quoted) — never str-format raw.

`_load_sibling(name)` + direct `import toml_write` (underscore module is directly importable).

**`test_recipe_config_write.py` (RED first — temp manifests with comments, load via `load_module`):**
- `test_replace_existing_key` — manifest has `base_branch = "main"  # keep-me-comment`; write `{"base_branch": "develop"}` → line replaced, value quoted, comment on other lines preserved.
- `test_insert_missing_key` — config block exists without `board_id`; insert → new line before next header, existing lines untouched.
- `test_comments_preserved` — a comment line inside the config block survives byte-for-byte.
- `test_insert_config_block_when_absent` — `[recipes.x]` exists, no `.config` → block created + key written; `tomllib.loads` round-trips.
- `test_append_full_block_when_recipe_absent` — empty-ish manifest → full `[recipes.x]` + `[recipes.x.config]` appended; parses.
- `test_bool_serialization` — `{"auto_remove_merged": True}` → `true` (not `True`).
- `test_invalid_write_restores_original` (TRIANGULATE) — inject a value that breaks TOML (monkeypatch `toml_value` or duplicate key) → original text restored byte-for-byte, `RecipeConfigWriteError` raised.
- `test_empty_values_is_noop` — `values={}` → file unchanged (content/mtime identical).
- `test_quoted_key_id` — recipe id needing quoting (e.g. `my.recipe`) handled via `_toml_key`.

**Acceptance criteria:**
- Surgical writer preserves all comments and blank lines inside the block (only matched `key =` lines rewritten; new keys inserted before next header or at EOF).
- Invalid writes restore the original byte-for-byte and raise `RecipeConfigWriteError` (same guarantee as `recipe-add.py`).
- Empty `values` is a true no-op.
- `./tests/run.sh` green.

**Estimated lines:** ~210 (`recipe-config-write.py` ~100; `test_recipe_config_write.py` ~110)

---

### Task P3.2 — `config_wizard.py`: per-field wizard + validators (RED → GREEN → TRIANGULATE)

**Files:**
- Create `lib/_internal/config_wizard.py`
- Create `tests/test_config_wizard.py`

**`config_wizard.py` contents (design §4):**

Imports `questionary` (vendored), imports `recipe_schema` directly, loads `recipe-config-write.py` + `dep_check.py` + `recipe-read.py` + `toml-read.py` via `_load_sibling`.

Public API:
- `run_config_wizard(recipe: Recipe, existing_config: dict) -> dict` — prompt for each `ConfigField` in `recipe.config_schema.fields` (never `.extra`). Returns `{key: collected_value}`. Keys left at default/blank are **omitted** from result (write-back keeps existing/default, never forces empty override). Cancel → `{}`.
- `configure_selected_recipes(project_root: Path, recipe_ids: list[str], existing_manifest: Path) -> dict` — for each recipe id: load catalog recipe, run `check_cli_deps` (render panel + dep gate), run `run_config_wizard(existing values)`, write via `update_recipe_config`. Returns `{recipe_id: collected_values}`. Skips a recipe if the user aborts on a missing required dep.

Per-field dispatch (design §4 table, `for key, field in sorted(recipe.config_schema.fields.items())`):
- `field.enum` set → `questionary.select(msg, choices=field.enum, default=...)`.
- `field.type == "bool"` → `questionary.confirm(msg, default=bool(existing_or_default))`.
- `field.validation.get("regex")` → `questionary.text(msg, default=..., validate=_regex_validator(rx))`.
- `field.required` (string, no enum) → `questionary.text(msg, default=..., validate=_required_validator)`.
- plain `field.type == "string"` / other → `questionary.text(msg, default=str(existing_or_default or ""))`. Blank → omit (keep default).

`msg` = `f"{recipe.id}.{key}"` plus `(required)`/`(optional)` and the type. `default` precedence: `existing_config[key]` → `field.default` → `""`/`False`.

Testable validators (extracted, not inline lambdas — design §4):
- `_required_validator(value: str) -> bool | str` — `True` if `value.strip()` else `"This field is required."`.
- `_regex_validator(pattern: str) -> Callable` — empty → `True` (caller decides blank), non-empty → regex match or `"Must match {pattern}"`. Compose when both required + regex: empty → "required", non-empty → regex check.

Dep gate (design §4):
- `_dep_gate(recipe, console) -> bool` — `_check_cli_deps(recipe)`; render dep panel (WARN missing required, INFO optional). If no missing required → `True`. Else `questionary.confirm("N required CLI tool(s) missing. Configure anyway?", default=False).ask()` → `False` skips recipe (config block untouched/omitted).

`main(argv)` — resolve `project_root` (arg or cwd), `util.ensure_deps(vendor_dir())`, TTY guard (`sys.stdin.isatty()`), read enabled recipes from manifest, call `configure_selected_recipes`, then offer `.envrc.example` generation.

`_load_sibling(name)` — mirror `hub.py` 19–30 for hyphen-named siblings.

**`test_config_wizard.py` (RED first, mock questionary per `test_hub_tui` idiom):**
- `test_required_validator_rejects_blank_accepts_value` — pure unit on `_required_validator`: blank → `"This field is required."`; non-blank → `True`.
- `test_regex_validator` — matches board-id regex; rejects bad, accepts 24-hex.
- `test_enum_field_uses_select` — patch `questionary.select`, assert called with `choices=` = enum list.
- `test_bool_field_uses_confirm` — patch `questionary.confirm`, `field.type="bool"` → confirm called, bool returned.
- `test_default_prefill_kept_when_blank` — text `.ask()` returns `""` → key omitted from result.
- `test_existing_value_prefilled_as_default` — `existing_config` value passed as `default=`.
- `test_dep_gate_abort_skips_recipe` — `check_cli_deps` (patched) reports missing required; confirm `.ask()` → `False`; `configure_selected_recipes` does not call `update_recipe_config` for it.
- `test_dep_gate_proceed_continues` — confirm True → proceeds to prompting.
- `test_extra_fields_never_prompted` — recipe with `config_schema.extra` (e.g. `board_isolation`) → those keys never appear in prompts (assert prompt count == `len(fields)`).
- `test_configure_selected_writes_each` (TRIANGULATE integration) — real `update_recipe_config` on a temp manifest: values land in `[recipes.<id>.config]`.

**Acceptance criteria:**
- Reuses `ConfigField` metadata for prompt type, validation, enums, defaults — no duplicated validation logic.
- Only `config_schema.fields` are prompted (never `.extra`).
- Dep gate shows install guidance and offers proceed/abort on missing required deps.
- `./tests/run.sh` green.

**Estimated lines:** ~290 (`config_wizard.py` ~160; `test_config_wizard.py` ~130)

---

### Task P3.3 — `lib/recipe-config.sh` shim + `bin/ai-specs` dispatch (GREEN)

**Files:**
- Create `lib/recipe-config.sh`
- Modify `bin/ai-specs`

**`recipe-config.sh` (design §4 — mirrors `recipe-add.sh` exactly):**
- `#!/usr/bin/env bash` + `set -euo pipefail`.
- `SCRIPT_DIR` / `AI_SPECS_HOME` / `WIZARD_PY` resolution (byte-identical pattern to `lib/recipe-add.sh`).
- `usage()` + arg-parse loop (`--help|-h`, `--`, `-*`, positional `[path]`).
- Default `TARGET_PATH` to `pwd`, `cd` to resolve.
- `exec python3 "$WIZARD_PY" "$TARGET_PATH"`.

**`bin/ai-specs` (design §7):**
- Add case arm (after `recipe)`):
  ```sh
  configure-recipes) bash "$LIB_DIR/recipe-config.sh" "$@" ;;
  ```
- Add a help line under Commands.

**Tests:**
- `test_configure_recipes_dispatch` (in `test_hub.py` or `test_hub_tui.py`) — `ai-specs configure-recipes /nonexistent` → non-zero, no crash, clean "not initialized" message.
- `test_recipe_config_sh_help` (optional, mirrors `test_skills_add.py`) — `recipe-config.sh --help` prints usage, exit 0.

**Acceptance criteria:**
- `bash -n lib/recipe-config.sh` clean (checked by `./tests/validate.sh`).
- `ai-specs configure-recipes [path]` routes to `recipe-config.sh` → `config_wizard.py`.
- `./tests/run.sh` green.

**Estimated lines:** ~50 (`recipe-config.sh` ~45; `bin/ai-specs` ~5)

---

## P4 — Hub action + init wizard integration

**Goal:** wire the config wizard into the hub as `Action.CONFIGURE_RECIPES` and into the init wizard as step 3.5. Fix PTY test offsets for the menu count change (10 → 11).

### Task P4.1 — Hub: `Action.CONFIGURE_RECIPES` + `_MENU` entry (RED → GREEN)

**Files:**
- Modify `lib/_internal/hub.py`
- Modify `tests/test_hub_tui.py`

**`hub.py` changes (design §7):**
- Add `Action` enum member (values are CLI subcommand strings):
  ```python
  CONFIGURE_RECIPES = "configure-recipes"
  ```
  Placed after `RECIPES` (line 54) to keep recipe-related actions adjacent.
- Add `_MENU` entry (insert after `Recipes` row — new index 4, menu grows 10 → 11):
  ```python
  (Action.CONFIGURE_RECIPES, "Configure recipes", "Set up recipe config, CLI deps, env vars"),
  ```
- `_SUB_ARGS` — **omit** the entry (empty default `[]` already covers it; adding `[]` is noise — design decision 8). `DelegateRunner.run(CONFIGURE_RECIPES)` shells `ai-specs configure-recipes <target>` → `recipe-config.sh` → `config_wizard.py`.

**`test_hub_tui.py` changes (design §7 — same commit as `_MENU` change):**
1. `test_menu_has_exact_ten_entries` (line 88–105) → rename/update to `_eleven`, expect `len(_MENU) == 11`, add `"Configure recipes"` to the titles list **after `"Recipes"`** (index 4).
2. PTY navigating to Quit (`b"\x1b[B" * 9` at line 247) → `* 10` (Quit moves index 9 → 10).
3. PTY navigating to Version (`b"\x1b[B" * 6` at line 258) → `* 7` (Version moves 6 → 7).
4. PTY doctor-delegates scenario stages (`b"\x1b[B" * 9` at line 269) → `* 10` (Quit moved).
5. Add `test_configure_recipes_in_menu` — assert the label `"Configure recipes"` and help `"Set up recipe config, CLI deps, env vars"` are in `_MENU`.

**Acceptance criteria:**
- Menu has exactly 11 entries with `"Configure recipes"` at index 4 (after `"Recipes"`).
- `test_prompt_returns_each_action` (iterates `Action`) covers the new member automatically once `bin/ai-specs` routes it.
- All 3 PTY offset fixes land in the **same commit** as the `_MENU` edit (no broken intermediate state).
- `./tests/run.sh` green.

**Estimated lines:** ~20 modified (`hub.py` ~3; `test_hub_tui.py` ~17)

---

### Task P4.2 — Init wizard: step 3.5 `_configure_recipes` + `_render_manifest` config block (RED → GREEN → TRIANGULATE)

**Files:**
- Modify `lib/_internal/init_tui.py`
- Modify `tests/test_init_tui.py`

**`init_tui.py` changes (design §8):**

New `_configure_recipes(recipes, console, catalog_dir) -> dict`:
- For each selected recipe:
  1. `recipe = recipe_read.read_recipe(catalog_dir, rid)`.
  2. If `recipe.cli_deps`: `results = check_cli_deps(recipe)`; render dep panel (WARN missing required).
  3. If `recipe.config_schema.fields`: `questionary.confirm("Configure {rid} now?", default=True)`; if yes → `values = run_config_wizard(recipe, existing_config={})`; else skip ("Later" → recipe omitted from map → defaults/placeholder behavior preserved). Mitigates "wizard too long" risk.
  4. Collect into `configured[rid] = values`.
- `run_wizard` calls it after building `recipes` (step 3, ends ~line 235), passing `catalog_dir = _ai_specs_home() / "catalog" / "recipes"`. Result threaded into `_render_manifest`.

Extend `_render_manifest` (currently 135–158):
- Signature: `_render_manifest(tw, project_name, agents, recipes, configured=None)` — `configured` defaults `None` (existing callers/tests unchanged).
- Body: after `enabled = true` + `version = ...`, if `vals = (configured or {}).get(rid) or {}` is non-empty → `lines.append("")`, `lines.append(f"[recipes.{_toml_key(rid)}.config]")`, for `key in sorted(vals): lines.append(f"{_toml_key(key)} = {tw.toml_value(vals[key])}")`. Still `lines.append("")` after.

Preview + envrc offer:
- Preview panel (step 4) gains a `config` line summarizing configured recipes.
- After manifest written (~line 259), offer `.envrc.example` generation only if any enabled recipe has MCP env vars: `questionary.confirm("Generate ai-specs/.envrc.example?")` → `generate_envrc_example(target)`. Guarded so defaults-only init stays fast.

**`test_init_tui.py` (extend — RED first):**
- `test_render_manifest_writes_config_block` — unit on `_render_manifest` with a `configured` map → output contains `[recipes.<id>.config]` and the values (bool as `true`), parses via `tomllib`.
- `test_render_manifest_no_config_backward_compat` (TRIANGULATE) — `configured=None` → identical to current output (guards existing `_render_manifest` callers).
- PTY (at least one happy-path): extend a scenario to select a recipe with config, answer "later" → manifest has no config block (skip path); answer "now" with a value → config block present. One assertion for the skip/later gate (heavier scenarios optional but at least one skip-path PTY assertion required).

**Acceptance criteria:**
- Init step 3.5 collects config per selected recipe with a "Configure now / later (defaults)" gate.
- `_render_manifest` writes real `[recipes.<id>.config]` values (not placeholders) when a `configured` map is passed; `None` preserves current behavior.
- Dep-check panel shows missing prerequisites during init.
- `.envrc.example` offer is guarded (defaults-only init stays fast, no forced prompt).
- `./tests/run.sh` green (existing `test_init_tui.py` passes with zero changes to non-step-3.5 tests).

**Estimated lines:** ~130 modified (`init_tui.py` ~80; `test_init_tui.py` ~50)

---

## P5 — `.envrc.example` scaffolding

**Goal:** derive `ai-specs/.envrc.example` from enabled recipes' `[[provides.mcp]]` env tables. Never write `.envrc` (gitignored, user-owned).

### Task P5.1 — `envrc-scaffold.py`: `collect_env_vars` + `generate_envrc_example` (RED → GREEN → TRIANGULATE)

**Files:**
- Create `lib/_internal/envrc-scaffold.py`
- Create `tests/test_envrc_scaffold.py`

**`envrc-scaffold.py` contents (design §6):**

Public API:
- `collect_env_vars(project_root: Path) -> dict[str, str]` — scan enabled recipes' `[[provides.mcp]]` entries; for every value in each server's `env` table that is a `$VAR`/`${VAR}`/`${env:VAR}` reference (`ENV_REFERENCE_RE` imported from `recipe-init.py` via `_load_sibling`, reused — do not re-declare the regex), collect `VAR_NAME -> purpose`. Purpose = `f"required by {mcp_id} ({recipe_id})"`. First declaration wins (multiple sources do not clobber; first recipe's purpose is kept).
- `generate_envrc_example(project_root: Path) -> Path` — write `ai-specs/.envrc.example` with one `export VAR=""  # <purpose>` line per collected var, sorted. Header comment explains it is a committed template and `.envrc` (gitignored) is user-owned. If `.envrc.example` already exists → back up to `.envrc.example.bak` before overwriting. **NEVER writes or reads-to-write `.envrc`.** Returns the written `Path`.

Details:
- Enabled recipes: `toml-read.read_recipes(load_toml(manifest))` → ids with `enabled is True`.
- Load each catalog recipe (`recipe-read.read_recipe`), iterate `recipe.mcp` (list of `McpPreset`; `preset.config` holds `env`, `args`, etc.). Match the **value** against `ENV_REFERENCE_RE`. Example: trello `env = { TRELLO_API_KEY = "$TRELLO_API_KEY", ... }` → collects `TRELLO_API_KEY`, `TRELLO_TOKEN`.
- First slice scans **env tables only** (proposal non-goal: `args` `${...}` derivation deferred). The vault `${CANONICAL_VAULT_PATH}` in `args` is still captured because it also appears in the `env` table (verified in catalog).
- Output path: `project_root / "ai-specs" / ".envrc.example"`. Body header:
  ```
  # ai-specs/.envrc.example — committed template (safe to regenerate).
  # Copy to a project .envrc (gitignored) and fill in real values.
  # Generated from enabled recipes' [[provides.mcp]] env references.
  ```
- Backup rule: never destructive; existing `.envrc.example` → `.envrc.example.bak` first. `.envrc` never opened for write.

`_load_sibling(name)` + `_catalog_dir()` (same pattern as P2.1).

**`test_envrc_scaffold.py` (RED first — temp project + temp catalog fixture, load via `load_module`):**
- `test_collect_from_mcp_env` — enabled trello recipe → `{TRELLO_API_KEY, TRELLO_TOKEN}` present with non-empty purpose naming the recipe.
- `test_non_reference_env_ignored` — an env value that is a literal (not `$VAR`) is not collected.
- `test_generate_writes_export_lines` — file created, one `export VAR=""` per var, sorted, purpose comments present.
- `test_envrc_never_written` — after generate, assert `(ai-specs/.envrc)` does NOT exist.
- `test_existing_example_backed_up` (TRIANGULATE) — pre-create `.envrc.example` → after generate, `.bak` exists with old content, main file has new content.
- `test_no_enabled_mcp_recipes_writes_empty_template` — no env vars → header-only file (or documents "no env vars required"), still no `.envrc`.
- `test_disabled_recipe_excluded` — recipe present but `enabled = false` → its vars not collected.

**Acceptance criteria:**
- `generate_envrc_example` writes `ai-specs/.envrc.example` from MCP env vars and never writes `.envrc`.
- Existing `.envrc.example` is backed up before overwrite; never destructive.
- Disabled recipes excluded; non-reference env values ignored.
- `./tests/run.sh` green.

**Estimated lines:** ~200 (`envrc-scaffold.py` ~95; `test_envrc_scaffold.py` ~105)

---

### Task P5.2 — Wire envrc offer into wizard + init

**Files:**
- Modify `lib/_internal/config_wizard.py` (already created in P3.2)
- Modify `tests/test_config_wizard.py` (already created in P3.2)

**Changes:**
- In `config_wizard.main()`, after `configure_selected_recipes`, offer `.envrc.example` generation: `questionary.confirm("Generate ai-specs/.envrc.example?")` → `generate_envrc_example(project_root)`. Guarded so it only prompts when enabled recipes have MCP env vars.
- Init wizard wiring is already part of P4.2 (`_render_manifest` extension + envrc offer after manifest write). This task confirms the `envrc-scaffold.py` import and call are wired in both paths.

**Tests (extend `test_config_wizard.py`):**
- `test_main_offers_envrc_generation` — mocked `questionary.confirm` → True → `generate_envrc_example` called with `project_root`.
- `test_main_skips_envrc_when_no_mcp_recipes` — no enabled recipes with MCP env → envrc offer not shown.

**Acceptance criteria:**
- Hub re-config path offers `.envrc.example` generation after wizard completes.
- Init wizard offers `.envrc.example` generation after manifest write (P4.2).
- `./tests/run.sh` green.

**Estimated lines:** ~30 modified (`config_wizard.py` ~15; `test_config_wizard.py` ~15)

---

## P6 — Catalog validation + docs + full verification

**Goal:** ensure every success criterion is met, docs reflect the new schema/behavior, and the full verification suite (`./tests/run.sh` + `./tests/validate.sh`) is green.

### Task P6.1 — Docs update (GREEN — documentation)

**Files:**
- Modify `docs/recipe-schema.md` (add `[[deps.cli]]` section in V2 additions, mirroring `[[capabilities]]` style)
- Modify `docs/recipes-catalog.md` (CLI prerequisites per recipe — table or per-recipe notes)
- Modify `docs/ai-specs-toml.md` (note on `.envrc.example` generation)
- Modify `README.md` (if user-facing hub behavior changes — new "Configure recipes" menu action)

**Content:**
- `docs/recipe-schema.md` — document `[[deps.cli]]` array-of-tables: fields (`binary`, `purpose`, `required`, `install_url`, `version_check`, `min_version`), placement convention (after `[[capabilities]]`/`[[hooks]]`, before `[config.*]`), and that recipes without it parse unchanged.
- `docs/recipes-catalog.md` — per-recipe CLI prerequisites table (recipe → binary(s) → purpose → required).
- `docs/ai-specs-toml.md` — note that `.envrc.example` is generated from enabled recipes' `[[provides.mcp]]` env references; `.envrc` is user-owned and gitignored.
- `README.md` — add "Configure recipes" to the hub menu description if the README lists menu actions; note `ai-specs configure-recipes [path]` subcommand.

**Acceptance criteria:**
- Docs updated per proposal item 11.
- No claim that `.envrc` is auto-written (only `.envrc.example`).
- `bash -n` clean (if docs have shell examples).

**Estimated lines:** ~120 modified

---

### Task P6.2 — Full verification + CHANGELOG (GREEN — verification)

**Files:**
- Modify `CHANGELOG.md` (if present; else create)
- (No code changes — verification only)

**Work:**
- Add a `recipe-config` entry summarizing: `[[deps.cli]]` schema, `dep_check.py` + Doctor WARN, config wizard, surgical write-back, `.envrc.example` scaffolding, hub action, init step 3.5, catalog blocks.
- Run `./tests/run.sh` → all unit + PTY (dep-gated) tests green.
- Run `./tests/validate.sh` → `py_compile` + `bash -n` clean (covers `lib/recipe-config.sh`).
- Manually verify every proposal success criterion (`proposal-recipe-config.md:189–204`):
  1. Recipe with `[[deps.cli]]` parses into `Recipe.cli_deps`; unknown keys raise `RecipeValidationError`; missing `binary`/`purpose` raise; optional fields default correctly → `test_valid_cli_dep_parses`, `test_unknown_key_raises`, `test_missing_binary_raises`, `test_optional_defaults`, `test_absent_deps_yields_empty_list`.
  2. `check_cli_deps` reports found/missing per binary and honors `version_check` + `min_version`; `required=false` never produces a failure → `test_found_binary_ok`, `test_missing_binary_not_ok`, `test_version_meets_min`, `test_version_below_min`, `test_optional_missing_not_failure`.
  3. `ai-specs doctor` on a project with an enabled recipe whose required CLI is missing emits a `WARN` row with install guidance, and exit code unchanged → `test_recipe_cli_deps_warn_when_missing`, `test_doctor_exit_code_unchanged`.
  4. Hub "Configure recipes" runs the wizard for enabled recipes; values land in `[recipes.<id>.config]` with comments preserved; an invalid write leaves the manifest intact → `test_configure_selected_writes_each`, `test_replace_existing_key`, `test_comments_preserved`, `test_invalid_write_restores_original`.
  5. Init wizard step 3.5 collects config per selected recipe (with skip/later option) and writes real config values; dep-check panel shows missing prerequisites → `test_render_manifest_writes_config_block`, `test_render_manifest_no_config_backward_compat`, PTY skip-path assertion.
  6. `generate_envrc_example` writes `ai-specs/.envrc.example` from MCP env vars and never writes `.envrc` → `test_generate_writes_export_lines`, `test_envrc_never_written`.
  7. Docs updated (`recipe-schema.md`, `recipes-catalog.md`, `ai-specs-toml.md`) → P6.1.
  8. `./tests/run.sh` and `./tests/validate.sh` pass; new test suites present and green → P6.2.

**Acceptance criteria:**
- `./tests/run.sh` exit 0.
- `./tests/validate.sh` exit 0.
- Every success criterion from `proposal-recipe-config.md:189–204` mapped to a passing test or manual check (list above).
- `CHANGELOG.md` updated.

**Estimated lines:** ~20 (CHANGELOG)

---

## Summary

| Phase | New files | Modified files | Est. lines | Tests |
|-------|-----------|----------------|-------------|-------|
| P1 | — | `recipe_schema.py`, `recipe-read.py`, 6 catalog recipes, `test_recipe_schema.py` | ~180 | `CliDepParsingTests` (7 unit + 3 catalog round-trip) |
| P2 | `dep_check.py`, `test_dep_check.py` | `doctor.py`, `test_doctor.py` | ~310 | found/missing, version pass/fail, optional, aggregation, Doctor WARN + exit code |
| P3 | `recipe-config-write.py`, `config_wizard.py`, `recipe-config.sh`, `test_recipe_config_write.py`, `test_config_wizard.py` | `bin/ai-specs` | ~550 | surgical replace/insert/restore, validators, enum/bool/default, dep gate, extra exclusion |
| P4 | — | `hub.py`, `init_tui.py`, `test_hub_tui.py`, `test_init_tui.py` | ~150 | menu count 11, PTY offsets, render_manifest config block + backward compat, skip/later gate |
| P5 | `envrc-scaffold.py`, `test_envrc_scaffold.py` | `config_wizard.py` (envrc offer wiring), `test_config_wizard.py` | ~230 | collect from MCP env, non-reference ignored, export lines, `.envrc` never written, backup, empty/disabled |
| P6 | — | `docs/recipe-schema.md`, `docs/recipes-catalog.md`, `docs/ai-specs-toml.md`, `README.md`, `CHANGELOG.md` | ~140 | full `run.sh` + `validate.sh`, manual success-criteria sweep |
| **Total** | **9** | **~17** | **~1360–1560** (excl. vendored tree) | |

**Verification commands:** `./tests/run.sh` (TDD RED/GREEN), `./tests/validate.sh` (py_compile + bash -n).

**Commit grouping** (one PR, size:exception): P1 (schema + catalog) → P2 (dep-check + Doctor) → P3 (config-write + wizard + shim) → P4 (hub + init) → P5 (envrc) → P6 (docs + verify). Each phase commit leaves `./tests/run.sh` green.