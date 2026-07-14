# Design: Recipe config wizard + CLI dependency management + `.envrc` scaffolding

Companion to `proposal-recipe-config.md`. Extends the `tui-hub` branch (worktree
`.worktrees/tui-hub`). This document freezes the module boundaries, function signatures, data
flow, write-back algorithm, integration seams, catalog edits, and per-module test strategy so the
`sdd-tasks` and `sdd-apply` phases can execute mechanically under strict TDD (`./tests/run.sh`).

## 0. Grounding — verified facts about the current code

Read before designing; every decision below is anchored to these:

- `recipe_schema.py`: `_parse_config` (lines 383–446) is the discipline template — allowed-keys set,
  `RecipeValidationError` on unknown keys, type guards, per-field context strings. `Recipe`
  (132–152) is a flat dataclass with `field(default_factory=list)` list members. `validate_recipe_toml`
  (577–618) reads `data.get("recipe")`, `data.get("provides")`, `data.get("config")` and constructs
  `Recipe`. `RecipeValidationError` is the only exception type.
- `recipe-read.py`: `recipe_to_dict` (44–95) hand-builds a JSON-able dict; `read_recipe(catalog_dir,
  id)` (37–41) loads via `load_recipe_toml`. Hyphenated file → loaded through `importlib` by
  siblings (`_load_recipe_read`).
- `doctor.py`: `Check(severity, name, message, guidance="")` (37–48); `Severity` enum OK/INFO/WARN/
  ERROR (30–34); `Doctor.run()` (118–125) calls the `_check_*` methods in sequence and returns
  `1 if any ERROR else 0`. `_load_manifest()` (262–271) returns the parsed manifest dict or `{}`.
  `_check_bundled_assets` (230–256) is the closest `WARN`/guidance pattern. `AI_SPECS_HOME =
  parents[2]` (18).
- `hub.py`: `Action` enum (50–60) values are the CLI subcommand strings; `_MENU` (63–74) is a list of
  `(Action, label, help)` tuples (currently 10 entries; `test_hub_tui.test_menu_has_exact_ten_entries`
  locks the count → must update to 11). `_SUB_ARGS` (200–203) supplies trailing args. `DelegateRunner.run`
  (153–156) shells `[cli, action.value, target, *extra]` — so a new action needs a matching case in
  `bin/ai-specs`.
- `init_tui.py`: `run_wizard` (161–272) is the 4-step flow (name → agents → recipes → preview);
  `_render_manifest(tw, name, agents, recipes)` (135–158) writes `[recipes.<id>]` with `enabled`+
  `version` only — **no config block today**. `recipes` is a list of dicts `{id,name,version,
  description}` from `_catalog_recipes()` (82–110). PTY E2E tests drive real questionary.
- `recipe-add.py`: `add_recipe` (56–152) is the canonical comment-preserving append + `tomllib.loads`
  guard + restore-on-failure (107–125). Writes config **placeholders** (94–105). Stays untouched.
- `toml_write.py`: `toml_value(v)` (17–36) — bool→`true/false`, str→`json.dumps`, list/dict inline.
- `toml-read.py`: `read_recipes(data)` (141–158) → `{id: {enabled, version, config}}`; `load_toml`
  (25–). Manifest recipes live under `[recipes.<id>]`.
- `recipe-init.py`: `ENV_REFERENCE_RE = re.compile(r"^\$(?:\{env:)?([A-Za-z_][A-Za-z0-9_]*)\}?$")`
  (56); `SECRET_KEY_RE` (57). Matches `$VAR`, `${VAR}`, `${env:VAR}`.
- `util.py`: `ai_specs_home()` (19), `vendor_dir()` (26), `ensure_deps(vendor)` (35), `is_initialized`
  (30). Catalog dir = `ai_specs_home()/catalog/recipes` (mirrors `recipe-add._resolve_catalog_dir`).
- Catalog: no recipe.toml currently declares `[deps]` (verified — zero matches) → additive and safe.
  MCP env references: `trello` → `TRELLO_API_KEY`/`TRELLO_TOKEN`; `vault-canonical` →
  `CANONICAL_VAULT_PATH` (in both `env` table and `args` `${...}`).
- Tests: `tests/run.sh` = `python3 -m unittest discover -s tests -p 'test_*.py'`. Two idioms:
  `load_module(path,name)` importlib helper for hyphen files; subprocess CLI E2E; questionary mocked
  via `mock.patch.object(questionary, "select"/"text"/"confirm", ...)`.

### Module naming convention (load model)

| File | Naming | Import model | Rationale |
|---|---|---|---|
| `dep_check.py` | underscore | importable library (`import dep_check`) + `_load_sibling` | pure library, reused by doctor + wizard |
| `config_wizard.py` | underscore | importable library + `main()` entrypoint | tests import it; shim execs it by path |
| `recipe-config-write.py` | hyphen | `importlib`-loaded script + `main()` | mirrors `recipe-add.py` (command-style writer) |
| `envrc-scaffold.py` | hyphen | `importlib`-loaded script + `main()` | mirrors command-style script |

Underscore modules are directly importable (like `recipe_schema.py`, `toml_write.py`); hyphen
modules follow the `recipe-add.py`/`recipe-read.py` script convention and are loaded via
`importlib.util.spec_from_file_location`. `config_wizard.py` is underscore because tests import it
and it is loaded as a sibling by the shim; being exec'd by absolute path does not require a valid
module name.

## 1. Data flow (config wizard end-to-end)

```mermaid
sequenceDiagram
    actor User
    participant Shim as recipe-config.sh
    participant CW as config_wizard.py
    participant DC as dep_check.py
    participant Q as questionary
    participant RW as recipe-config-write.py
    participant M as ai-specs.toml
    participant ES as envrc-scaffold.py

    User->>Shim: ai-specs configure-recipes [path]
    Shim->>CW: python3 config_wizard.py <project_root>
    CW->>M: load manifest, read enabled [recipes.*]
    loop each enabled recipe with cli_deps or config_schema.fields
        CW->>DC: check_cli_deps(recipe)
        DC->>DC: command -v <binary>; optional version_check + min_version
        DC-->>CW: list[DepResult]
        CW->>User: render dep panel (WARN missing required, INFO optional)
        alt required dep missing
            CW->>Q: confirm("Proceed anyway?")
            Q-->>CW: yes/no  (no → skip this recipe)
        end
        CW->>M: read existing [recipes.<id>.config]
        loop each ConfigField in config_schema.fields
            CW->>Q: prompt by type (text/confirm/select) + validate
            Q-->>CW: value (or default kept if blank)
        end
        CW->>RW: update_recipe_config(manifest, id, values)
        RW->>M: surgical line replace/insert inside [recipes.<id>.config]
        RW->>RW: tomllib.loads(new_text) guard
        alt parse fails
            RW->>M: restore original text
            RW-->>CW: raise / return error
        end
    end
    CW->>User: confirm("Generate ai-specs/.envrc.example?")
    opt yes
        CW->>ES: generate_envrc_example(project_root)
        ES->>M: scan enabled recipes' [[provides.mcp]] env for $VARs
        ES->>ES: write ai-specs/.envrc.example (backup if exists)
        ES-->>CW: written Path
    end
    CW-->>User: summary (configured recipes, deps, envrc)
```

The init-wizard path (area 7) reuses the same `run_config_wizard` + `check_cli_deps` +
`collect_env_vars` primitives, but writes config inline through `_render_manifest` rather than
`update_recipe_config` (the manifest is being created fresh, not surgically edited).

---

## 2. Area 1 — `[[deps.cli]]` schema (`recipe_schema.py`)

### `CliDep` dataclass

Placed after `ConfigSchema` (line 122), before `InitWorkflow`:

```python
@dataclass
class CliDep:
    binary: str                      # required
    purpose: str                     # required
    required: bool = True
    install_url: str = ""
    version_check: str = ""          # shell command, e.g. "gh --version"
    min_version: str = ""            # e.g. "2.0.0"; compared only when version_check set
```

`binary`/`purpose` required (non-empty strings); `required` defaults `True`; the three optional
strings default `""` (empty = "not declared"), matching how existing optional strings like
`SkillRef.url`/`.path` default `""`.

### `Recipe` gains the field

In `Recipe` (after `config_schema`, before `init` to keep declared-before-optional grouping):

```python
cli_deps: list[CliDep] = field(default_factory=list)
```

Additive with a default — recipes without `[deps]` parse unchanged (rollback item 1).

### `_parse_cli_deps` — mirrors `_parse_config` discipline

```python
def _parse_cli_deps(raw: Any, context: str) -> list[CliDep]:
    """Parse [[deps.cli]] array-of-tables. Returns [] when absent.

    context example: "[deps.cli]". Each entry must be a table with required
    'binary' and 'purpose'; unknown keys raise RecipeValidationError.
    """
    if raw is None:
        return []
    if not isinstance(raw, list):
        raise RecipeValidationError(f"{context}: expected array of tables, got {type(raw).__name__}")
    allowed = {"binary", "purpose", "required", "install_url", "version_check", "min_version"}
    out: list[CliDep] = []
    for idx, item in enumerate(raw):
        ctx = f"{context}[{idx}]"
        if not isinstance(item, dict):
            raise RecipeValidationError(f"{ctx}: expected table, got {type(item).__name__}")
        for k in item:
            if k not in allowed:
                raise RecipeValidationError(f"{ctx}: unknown key '{k}'")
        binary = _require_string(item, "binary", ctx)
        purpose = _require_string(item, "purpose", ctx)
        required = item.get("required", True)
        if not isinstance(required, bool):
            raise RecipeValidationError(f"{ctx}.required: expected boolean, got {type(required).__name__}")
        install_url = _opt_str(item, "install_url", ctx)
        version_check = _opt_str(item, "version_check", ctx)
        min_version = _opt_str(item, "min_version", ctx)
        out.append(CliDep(binary, purpose, required, install_url, version_check, min_version))
    return out
```

Add a tiny `_opt_str(data, key, ctx)` helper (returns `""` when absent; raises when present but not a
string) next to `_require_string`, since three optional-string fields repeat the guard. Reuses the
existing `_require_string` for the two required fields (consistent error phrasing).

### Wiring into `validate_recipe_toml`

Inside the `Recipe(...)` constructor call (line 598+), add:

```python
deps_table = data.get("deps", {})
if not isinstance(deps_table, dict):
    deps_table = {}
# ... in the Recipe(...) kwargs:
    cli_deps=_parse_cli_deps(deps_table.get("cli"), "[deps.cli]"),
```

`data.get("deps", {})` is a **table** in recipe.toml (unlike the manifest's list-shaped `[[deps]]`) —
no collision (design decision 1 in proposal, verified: zero existing `[deps]` in catalog).

### `recipe_to_dict` serialization (`recipe-read.py`)

Add a top-level `"cli_deps"` key to the returned dict (sibling of `"provides"`), used by CLI display
and by the dep-check panels:

```python
"cli_deps": [
    {
        "binary": d.binary, "purpose": d.purpose, "required": d.required,
        "install_url": d.install_url, "version_check": d.version_check,
        "min_version": d.min_version,
    }
    for d in recipe.cli_deps
],
```

### Test strategy — `test_recipe_schema.py::CliDepParsingTests`

New `unittest.TestCase` using the existing `load_module` + temp-recipe idiom:
- `test_valid_cli_dep_parses` — full block → `Recipe.cli_deps[0]` fields exact.
- `test_optional_defaults` — only `binary`+`purpose` → `required is True`, three strings `""`.
- `test_missing_binary_raises` / `test_missing_purpose_raises` — `RecipeValidationError`, message
  names the missing field.
- `test_unknown_key_raises` — `[[deps.cli]]` with `foo = "x"` → raises, message contains `'foo'`.
- `test_required_non_bool_raises` — `required = "yes"` → raises.
- `test_absent_deps_yields_empty_list` — recipe with no `[deps]` → `cli_deps == []` (backward compat).
- `test_recipe_to_dict_serializes_cli_deps` — via `read_mod.recipe_to_dict`, assert the `cli_deps`
  list shape (guards the display contract).

---

## 3. Area 2 — `dep_check.py`

Underscore library module. No third-party deps (POSIX `command -v` via `subprocess`).

### `DepResult` dataclass

```python
@dataclass
class DepResult:
    binary: str
    found: bool
    version: str          # parsed version string, "" if unknown/not run
    ok: bool              # found AND (min_version satisfied or no min_version)
    install_url: str
    purpose: str
    required: bool
    recipe_id: str = ""   # populated by check_project_deps for aggregation
    detail: str = ""      # human note, e.g. "found 1.9.0 < required 2.0.0"
```

`ok` is the single truth for "usable": `found and version_ok`. A `required=False` dep that is missing
yields `found=False, ok=False, required=False` — callers treat non-required failures as INFO, never
WARN/ERROR (proposal success criterion).

### Public functions

```python
def check_cli_deps(recipe: Recipe) -> list[DepResult]:
    """One DepResult per recipe.cli_deps entry. Never raises — subprocess
    errors degrade to found=False (proposal risk: subprocess portability)."""

def check_project_deps(project_root: Path) -> list[DepResult]:
    """Load manifest, resolve enabled [recipes.*], load each catalog recipe,
    aggregate check_cli_deps across all. recipe_id populated. De-dup by
    (recipe_id, binary) is not applied — the same binary across recipes yields
    separate rows so guidance stays recipe-scoped."""
```

### Internal helpers

```python
def _which(binary: str) -> bool:
    # POSIX-safe: shutil.which(binary) is not None. (Avoids spawning a shell;
    # equivalent to `command -v` for existence and honors PATH. Chosen over
    # subprocess `command -v` to sidestep shell portability — see below.)

def _run_version_check(cmd: str) -> str:
    # subprocess.run(cmd, shell=True, capture_output=True, text=True, timeout=5).
    # Returns stdout+stderr; "" on any exception (FileNotFoundError, timeout,
    # non-zero). Never raises.

def _parse_version(text: str) -> tuple[int, ...]:
    # Extract first r"\d+(?:\.\d+)*" from text, split on '.', int-tuple.
    # () when no numeric token found.

def _version_ge(have: tuple[int, ...], want: tuple[int, ...]) -> bool:
    # Zero-pad shorter tuple, lexicographic compare. Empty have -> False only
    # when want is non-empty and we required a check.
```

**Decision — `shutil.which` for existence, `subprocess(shell=True)` only for `version_check`.** The
proposal says `command -v`; `shutil.which` is the stdlib, no-shell, portable equivalent for the
existence probe and avoids the "missing `/bin/sh`" risk entirely. `version_check` is an
author-declared command string that genuinely needs a shell (may contain pipes), so it runs with
`shell=True` under a 5s timeout, and any failure is swallowed to `""` → `version=""`. When
`min_version` is set but the version cannot be parsed, `ok` stays `True` (found), `detail` notes
"version unknown" — we never block on an unparseable version (guidance-only philosophy).

`ok` computation:
```
found = _which(dep.binary)
version = _parse_or_"" if dep.version_check else ""
if not found: ok = False
elif dep.min_version and version: ok = _version_ge(parse(version), parse(dep.min_version))
else: ok = True   # found, no comparable version
```

### Doctor integration — `_check_recipe_cli_deps`

New method on `Doctor`, registered in `run()` (append after `_check_enabled_agents`, so it runs last
and never reorders existing checks — keeps `test_doctor` line expectations stable except for the new
rows it explicitly adds):

```python
def _check_recipe_cli_deps(self) -> None:
    data = self._load_manifest()
    recipes = data.get("recipes", {})
    if not isinstance(recipes, dict) or not recipes:
        return
    try:
        results = self._collect_recipe_dep_results()  # loads dep_check as sibling
    except Exception:
        return  # degrade silently; never break `ai-specs doctor`
    for r in results:
        if r.ok:
            self.checks.append(Check(Severity.OK, "recipe-dep",
                f"{r.binary} available for {r.recipe_id}"))
        elif r.required:
            self.checks.append(Check(Severity.WARN, "recipe-dep",
                f"{r.binary} missing/unusable for {r.recipe_id}: {r.purpose}",
                guidance=r.install_url or "install the required CLI"))
        else:
            self.checks.append(Check(Severity.INFO, "recipe-dep",
                f"optional {r.binary} not found for {r.recipe_id}: {r.purpose}",
                guidance=r.install_url))
```

`_collect_recipe_dep_results()` loads `dep_check.py` and `recipe-read.py` via the same
`importlib` seam Doctor already uses (`_brief_render_disabled` at 273–290 is the template), resolves
`catalog_dir = AI_SPECS_HOME / "catalog" / "recipes"`, and delegates to `check_project_deps(self.root)`
(or inlines the enabled-recipe loop to reuse the already-parsed manifest). Only enabled recipes are
checked (`entry.get("enabled") is True`). **Never raises; emits at most `WARN`** → exit code unchanged
(proposal risk: doctor must not change exit code). WARN ≠ ERROR, so `Doctor.run()`'s
`any(ERROR)` return is untouched.

### Test strategy — `test_dep_check.py`

Import `dep_check` via `load_module`. Mock `shutil.which` and `_run_version_check`:
- `test_found_binary_ok` — `which` returns a path, no min_version → `found=True, ok=True`.
- `test_missing_binary_not_ok` — `which` returns None → `found=False, ok=False`.
- `test_version_meets_min` — `version_check` output "gh 2.40.0", `min_version="2.0.0"` → `ok=True`.
- `test_version_below_min` — output "gh 1.9.0", min "2.0.0" → `ok=False`, `detail` names the shortfall.
- `test_unparseable_version_does_not_block` — output "weird", min "2.0.0", found → `ok=True`.
- `test_optional_missing_not_failure` — `required=False`, missing → `ok=False` but caller-facing:
  assert `r.required is False` (Doctor maps to INFO, tested there).
- `test_version_check_subprocess_error_degrades` — patch `_run_version_check` to raise → swallowed,
  `version==""`, no exception.
- `test_check_project_deps_aggregates` — build a temp project with two enabled recipes (temp catalog
  fixture), assert one `DepResult` per declared dep with `recipe_id` set.
- `test_version_ge` / `test_parse_version` — pure unit tests over the tuple compare boundaries
  (`2.0` vs `2.0.0`, `10` vs `9`).
- Doctor rows: extend `test_doctor.py` — a project with an enabled recipe whose required CLI is
  absent (patch `shutil.which` to miss it) emits a `WARN recipe-dep` row **and exit code stays 0**.

---

## 4. Area 3 — `config_wizard.py`

Underscore library module + `main()`. Imports `questionary` (already vendored/ensured), imports
`recipe_schema` directly, loads `recipe-config-write.py`, `dep_check.py`, `recipe-read.py`,
`toml-read.py` via `_load_sibling` (the `hub.py`/`init_tui.py` pattern at lines 19–30 / 52–63).

### Public API

```python
def run_config_wizard(recipe: Recipe, existing_config: dict) -> dict:
    """Prompt for each ConfigField in recipe.config_schema.fields (never .extra).
    Returns {key: collected_value}. Keys the user left at default/blank are
    OMITTED from the result (so write-back keeps the existing/default and never
    forces an empty override). Raises no exception on normal cancel — returns {}
    or a sentinel handled by the caller."""

def configure_selected_recipes(
    project_root: Path,
    recipe_ids: list[str],
    existing_manifest: Path,
) -> dict:
    """For each recipe id (in order): load catalog recipe, run check_cli_deps
    (render panel + dep gate), run run_config_wizard(existing values), and write
    via update_recipe_config. Returns {recipe_id: collected_values} for the
    summary. Skips a recipe if the user aborts on a missing required dep."""
```

### Per-field prompt mapping (the core dispatch)

For each `key, field in sorted(recipe.config_schema.fields.items())`:

| `ConfigField` shape | questionary prompt | notes |
|---|---|---|
| `field.enum` set | `questionary.select(msg, choices=field.enum, default=...)` | default = existing or `field.default` if in enum |
| `field.type == "bool"` | `questionary.confirm(msg, default=bool(existing_or_default))` | returns Python bool |
| `field.validation.get("regex")` | `questionary.text(msg, default=..., validate=_regex_validator(rx))` | re-prompt via validate |
| `field.required` (string, no enum) | `questionary.text(msg, default=..., validate=_required_validator)` | re-prompt until non-empty |
| plain `field.type == "string"` / other | `questionary.text(msg, default=str(existing_or_default or ""))` | blank → omit (keep default) |

`msg` = `f"{recipe.id}.{key}"` plus `(required)` / `(optional)` and the type, so the prompt is
self-describing. `default` precedence: `existing_config[key]` → `field.default` → `""`/`False`.

### Testable validators (extracted, not inline lambdas)

Re-prompt behavior lives in named module-level functions so tests assert them directly (a mocked
`.ask()` returns one value and cannot exercise questionary's internal validate loop):

```python
def _required_validator(value: str) -> bool | str:
    return True if value.strip() else "This field is required."

def _regex_validator(pattern: str) -> Callable[[str], bool | str]:
    rx = re.compile(pattern)
    def _v(value: str) -> bool | str:
        if not value:            # empty allowed when field not required; caller decides
            return True
        return True if rx.match(value) else f"Must match {pattern}"
    return _v
```

When a field is **both** required and regex-constrained, compose: empty → "required", non-empty →
regex check.

### Dep gate (before prompting a recipe's fields)

```python
def _dep_gate(recipe: Recipe, console) -> bool:
    results = check_cli_deps(recipe)
    _render_dep_panel(results, console)          # rich panel: WARN missing required, INFO optional
    missing_required = [r for r in results if r.required and not r.ok]
    if not missing_required:
        return True
    return bool(questionary.confirm(
        f"{len(missing_required)} required CLI tool(s) missing. Configure anyway?",
        default=False).ask())
```

Returning `False` skips the recipe (its config block is left untouched / omitted).

### `main()` + shell shim

`main(argv)` resolves `project_root` (arg or cwd), calls `util.ensure_deps(vendor_dir())`, requires a
TTY (same `sys.stdin.isatty()` guard as `init_tui.run_wizard` at 168), reads enabled recipes from the
manifest, and calls `configure_selected_recipes`. New shim `lib/recipe-config.sh` mirrors
`recipe-add.sh` exactly (resolve `AI_SPECS_HOME`, parse optional `[path]`, `exec python3
"$AI_SPECS_HOME/lib/_internal/config_wizard.py" "$TARGET_PATH"`).

### Test strategy — `test_config_wizard.py`

Mock questionary (`mock.patch.object(config_wizard.questionary, ...)`) following `test_hub_tui`:
- `test_required_validator_rejects_blank_accepts_value` — pure unit on `_required_validator`.
- `test_regex_validator` — matches board-id regex; rejects bad, accepts 24-hex.
- `test_enum_field_uses_select` — patch `questionary.select`, assert called with `choices=` = enum.
- `test_bool_field_uses_confirm` — patch `confirm`, field `type="bool"` → confirm called, bool returned.
- `test_default_prefill_kept_when_blank` — text `.ask()` returns "" → key omitted from result.
- `test_existing_value_prefilled_as_default` — `existing_config` value passed as `default=`.
- `test_dep_gate_abort_skips_recipe` — `check_cli_deps` (patched) reports missing required, confirm
  `.ask()` → False; `configure_selected_recipes` does not call `update_recipe_config` for it.
- `test_dep_gate_proceed_continues` — confirm True → proceeds to prompting.
- `test_extra_fields_never_prompted` — recipe with `config_schema.extra` (e.g. `board_isolation`)
  → those keys never appear in prompts (assert prompt count == len(fields)).
- `test_configure_selected_writes_each` — integration with real `update_recipe_config` on a temp
  manifest: values land in `[recipes.<id>.config]`.

---

## 5. Area 4 — `recipe-config-write.py`

Hyphen script (mirrors `recipe-add.py`). The surgical, comment-preserving updater.

### API

```python
def update_recipe_config(manifest_path: Path, recipe_id: str, values: dict) -> None:
    """Write values into [recipes.<id>.config], preserving all comments.
    - Replace an existing `key = value` line in the block.
    - Insert a missing key before the next section header (or block end).
    - If [recipes.<id>.config] absent but [recipes.<id>] exists: insert a new
      config sub-block after the recipe header block.
    - If [recipes.<id>] absent entirely: append a full [recipes.<id>] +
      [recipes.<id>.config] block (handles recipe-not-yet-in-manifest).
    Validates the result with tomllib.loads; restores original on failure and
    raises RecipeConfigWriteError. No-op when values is empty."""
```

Values serialized through `toml_write.toml_value` (bool→`true/false`, str quoted) — never str-format
raw (the `recipe-add.py` placeholder path already relies on this).

### Algorithm (line-oriented, string-based — NOT a TOML round-trip)

```
text = manifest_path.read_text()
lines = text.splitlines(keepends=True)          # keep newlines to rebuild verbatim

1. Locate the [recipes.<id>] header line index (exact match on the rendered
   header via init_tui._toml_key(recipe_id), tolerant of surrounding blanks).
   - not found  -> APPEND path: build "\n[recipes.<id>]\n... \n[recipes.<id>.config]\n<k=v>\n"
                   (only enabled+version if we must synthesize the parent — but
                   the wizard only writes config for recipes already enabled, so
                   parent normally exists; append path is the safety net).
2. Locate [recipes.<id>.config] header within the recipe's region (from the
   recipe header to the next top-level [section] that is NOT [recipes.<id>.*]).
   - not found  -> INSERT a "[recipes.<id>.config]" header at the end of the
                   recipe region, then treat all values as inserts.
3. Determine the config block's line span: from the config header to the next
   header line ("[") or EOF.
4. For each key,val in values (sorted for determinism):
   - scan the block span for a line matching r'^\s*<key>\s*=' (bare or quoted
     key via _toml_key) -> replace that single line with "<key> = <toml_value(val)>\n"
     preserving the line's leading indentation.
   - else -> record as pending insert.
5. Insert all pending keys immediately before the block's terminating header
   line (or at EOF), as "<key> = <toml_value(val)>\n".
6. new_text = "".join(lines)
7. try: tomllib.loads(new_text)
   except tomllib.TOMLDecodeError: manifest_path.write_text(original); raise
   else: manifest_path.write_text(new_text)
```

Comment lines (`# ...`) and blank lines inside the block are never touched (we only rewrite matched
`key =` lines and append before the next header). A commented placeholder such as
`# optional_key = ""  # optional` (written by `recipe-add.py`) is treated as **absent** (the
`^\s*key\s*=` scan requires a non-comment line), so the wizard inserts a real line rather than
editing the comment — acceptable and safe (validated by `tomllib`).

Custom exception `RecipeConfigWriteError(Exception)` (module-local), so callers can distinguish a
write failure from questionary cancel.

### Test strategy — `test_recipe_config_write.py`

Load via `load_module`. Temp manifests with comments:
- `test_replace_existing_key` — manifest has `base_branch = "main"  # keep-me-comment`; write
  `{"base_branch": "develop"}` → line replaced, comment on the following/other lines preserved,
  value quoted.
- `test_insert_missing_key` — block exists without `board_id`; insert → new line before next header,
  existing lines untouched.
- `test_comments_preserved` — a comment line inside the config block survives byte-for-byte.
- `test_insert_config_block_when_absent` — `[recipes.x]` exists, no `.config` → block created +
  key written; `tomllib.loads` round-trips.
- `test_append_full_block_when_recipe_absent` — empty-ish manifest → full `[recipes.x]` +
  `[recipes.x.config]` appended; parses.
- `test_bool_and_default_serialization` — `{"auto_remove_merged": True}` → `true` (not `True`).
- `test_invalid_write_restores_original` — inject a value that would break TOML (e.g. monkeypatch
  `toml_value` to emit garbage, or a key that duplicates) → original text restored byte-for-byte,
  `RecipeConfigWriteError` raised.
- `test_empty_values_is_noop` — `values={}` → file unchanged (mtime/content identical).
- `test_quoted_key_id` — recipe id needing quoting (e.g. `my.recipe`) handled via `_toml_key`.

---

## 6. Area 5 — `envrc-scaffold.py`

Hyphen script + `main()`. Derives env vars from enabled recipes' `[[provides.mcp]]` env tables.

### API

```python
def collect_env_vars(project_root: Path) -> dict[str, str]:
    """Scan enabled recipes' [[provides.mcp]] entries; for every value in each
    server's `env` table that is a $VAR / ${VAR} / ${env:VAR} reference
    (ENV_REFERENCE_RE), collect VAR_NAME -> purpose. Purpose = a human string
    'required by <mcp-id> (<recipe-id>)'. Later recipes do not clobber an
    existing var's purpose (first declaration wins; multiple sources appended)."""

def generate_envrc_example(project_root: Path) -> Path:
    """Write ai-specs/.envrc.example with one `export VAR=""  # <purpose>` line
    per collected var, sorted. Header comment explains it is a committed
    template and that .envrc (gitignored) is user-owned. If .envrc.example
    already exists, back it up to .envrc.example.bak before overwriting.
    NEVER writes or reads-to-write .envrc. Returns the written Path."""
```

### Details

- Enabled recipes: `toml-read.read_recipes(load_toml(manifest))` → ids with `enabled is True`.
- Load each catalog recipe (`recipe-read.read_recipe`), iterate `recipe.mcp` (list of `McpPreset`;
  `preset.config` holds `env`, `args`, etc.). The `env` table maps var name → reference string;
  we match the **value** against `ENV_REFERENCE_RE` (imported from `recipe-init.py` via `_load_sibling`,
  reused per proposal — do not re-declare the regex). Example: trello `env = { TRELLO_API_KEY =
  "$TRELLO_API_KEY", ... }` → collects `TRELLO_API_KEY`, `TRELLO_TOKEN`.
- First slice scans **env tables only** (proposal non-goal: `args` `${...}` derivation deferred). The
  vault `${CANONICAL_VAULT_PATH}` in `args` is still captured because it also appears in the `env`
  table (`env = { CANONICAL_VAULT_PATH = "$CANONICAL_VAULT_PATH" }`) — verified in the catalog.
- Output path: `project_root / "ai-specs" / ".envrc.example"`. Body:
  ```
  # ai-specs/.envrc.example — committed template (safe to regenerate).
  # Copy to a project .envrc (gitignored) and fill in real values.
  # Generated from enabled recipes' [[provides.mcp]] env references.

  export CANONICAL_VAULT_PATH=""  # required by vault-canonical (vault-canonical-store)
  export TRELLO_API_KEY=""        # required by trello (trello-mcp-workflow)
  export TRELLO_TOKEN=""          # required by trello (trello-mcp-workflow)
  ```
- Backup rule: never destructive; existing `.envrc.example` → `.envrc.example.bak` first. `.envrc`
  is never opened for write (proposal decision 4, gitignored at line 30).

### Test strategy — `test_envrc_scaffold.py`

Load via `load_module`; temp project + temp catalog fixture (or point at the real catalog with a
manifest enabling trello + vault):
- `test_collect_from_mcp_env` — enabled trello recipe → `{TRELLO_API_KEY, TRELLO_TOKEN}` present with
  non-empty purpose naming the recipe.
- `test_non_reference_env_ignored` — an env value that is a literal (not `$VAR`) is not collected.
- `test_generate_writes_export_lines` — file created, one `export VAR=""` per var, sorted, purpose
  comments present.
- `test_envrc_never_written` — after generate, assert `(ai-specs/.envrc)` does NOT exist.
- `test_existing_example_backed_up` — pre-create `.envrc.example` → after generate, `.bak` exists
  with old content, main file has new content.
- `test_no_enabled_mcp_recipes_writes_empty_template` — no env vars → header-only file (or documents
  "no env vars required"), still no `.envrc`.
- `test_disabled_recipe_excluded` — recipe present but `enabled = false` → its vars not collected.

---

## 7. Area 6 — Hub integration (`hub.py` + `bin/ai-specs`)

### `Action` enum
Add member (values are CLI subcommands):
```python
CONFIGURE_RECIPES = "configure-recipes"
```
Placed after `RECIPES` (keeps recipe-related actions adjacent).

### `_MENU` entry
Insert after the `RECIPES` row (menu grows 10 → 11):
```python
(Action.CONFIGURE_RECIPES, "Configure recipes", "Set up recipe config, CLI deps, env vars"),
```
**Inserting after `Recipes` (new index 4) shifts every later menu offset — three test edits, same
commit as the `_MENU` change (P4):**
1. `test_hub_tui.test_menu_has_exact_ten_entries` (line 88–105) → expect `len(_MENU) == 11` and add
   `"Configure recipes"` to the titles list after `"Recipes"`.
2. PTY `test_hub_tui` navigating to Quit (`b"\x1b[B" * 9`, ~line 247) → `* 10` (Quit moves index 9→10).
3. PTY `test_hub_tui` navigating to Version (`b"\x1b[B" * 6`, ~line 258) → `* 7` (Version moves 6→7).

`test_prompt_returns_each_action` iterates `Action` so it covers the new member automatically once
`bin/ai-specs` routes it.

### `_SUB_ARGS`
No trailing args needed (the target is already appended by `DelegateRunner`). Either omit the entry
(defaults to `[]`) or add `Action.CONFIGURE_RECIPES: []` for explicitness. **Decision: omit** — the
empty default already covers it; adding a `[]` entry is noise. (The proposal's phrase "_SUB_ARGS
maps to ... entrypoint" is imprecise; the real wiring is `Action.value` → `bin/ai-specs` case.)

### `bin/ai-specs` dispatch
Add a case (after `recipe)`):
```sh
configure-recipes) bash "$LIB_DIR/recipe-config.sh" "$@" ;;
```
And a help line under Commands. `DelegateRunner.run(CONFIGURE_RECIPES)` shells
`ai-specs configure-recipes <target>` → `recipe-config.sh <target>` → `config_wizard.py <target>`.

### Behavior
The hub action runs `config_wizard.main([target])`: reads enabled recipes, runs the dep panel +
config wizard per recipe (surgical write-back), then offers `.envrc.example` generation. This is the
re-configuration path for already-enabled recipes.

### Test strategy
- `test_hub.py` / `test_hub_tui.py`: update the menu-count test to 11; add
  `test_configure_recipes_in_menu` asserting the label/help; assert `bin/ai-specs help` and dispatch
  route `configure-recipes` (subprocess: `ai-specs configure-recipes /nonexistent` → non-zero,
  no crash / clean "not initialized" message). `DelegateRunner` argv test already generic.
- `test_recipe_config_sh` (optional, mirrors `test_skills_add.py`): shim resolves home, forwards
  target, `--help` prints usage.

---

## 8. Area 7 — Init wizard integration (`init_tui.py`)

### New step 3.5 — `_configure_recipes`

Inserted in `run_wizard` between recipe checkbox (step 3, ends ~line 235) and preview (step 4,
~line 237):

```python
def _configure_recipes(recipes: list[dict], console, catalog_dir: Path) -> dict:
    """For each selected recipe: load catalog Recipe, show dep-check panel, and
    if it has config_schema.fields, run run_config_wizard (with a per-recipe
    'Configure now / later (defaults)' gate). Returns {recipe_id: values} for
    _render_manifest. 'Later' -> recipe omitted from the map (defaults/placeholder
    behavior preserved -> mitigates the 'wizard too long' risk)."""
```

Flow per selected recipe:
1. `recipe = recipe_read.read_recipe(catalog_dir, rid)`.
2. If `recipe.cli_deps`: `results = check_cli_deps(recipe)`; render dep panel (WARN missing required).
3. If `recipe.config_schema.fields`: ask `questionary.confirm("Configure {rid} now?", default=True)`;
   if yes → `values = run_config_wizard(recipe, existing_config={})`; else skip.
4. Collect into `configured[rid] = values`.

`run_wizard` calls it after building `recipes`, passing the catalog dir
(`_ai_specs_home()/"catalog"/"recipes"`). Result threaded into `_render_manifest`.

### `_render_manifest` writes real config

Extend the signature and body (currently 135–158):
```python
def _render_manifest(tw, project_name, agents, recipes, configured: dict | None = None) -> str:
    ...
    for recipe in recipes:
        rid = recipe["id"]
        lines.append(f"[recipes.{_toml_key(rid)}]")
        lines.append("enabled = true")
        lines.append(f"version = {tw.toml_value(recipe.get('version') or '0.0.0')}")
        vals = (configured or {}).get(rid) or {}
        if vals:
            lines.append("")
            lines.append(f"[recipes.{_toml_key(rid)}.config]")
            for key in sorted(vals):
                lines.append(f"{_toml_key(key)} = {tw.toml_value(vals[key])}")
        lines.append("")
```
`configured` defaults to `None` so existing callers/tests keep working; when absent, behavior is
identical to today (enabled+version only). This satisfies "writes real config values (not
placeholders)" while keeping the render function's rebuild-from-scratch contract (init creates the
manifest fresh — no surgical writer needed here; `update_recipe_config` is the hub re-config path).

### Preview + envrc offer
Preview panel (step 4) gains a `config` line summarizing configured recipes. After the manifest is
written (step ~259), offer `.envrc.example` generation only if any enabled recipe has MCP env vars:
`questionary.confirm("Generate ai-specs/.envrc.example?")` → `generate_envrc_example(target)`. Guarded
so a defaults-only init stays fast.

### Test strategy
Existing PTY E2E (`test_init_tui.py`) drives real questionary; add:
- `test_render_manifest_writes_config_block` — unit on `_render_manifest` with a `configured` map →
  output contains `[recipes.<id>.config]` and the values (bool as `true`), parses via `tomllib`.
- `test_render_manifest_no_config_backward_compat` — `configured=None` → identical to current output
  (guards existing `_render_manifest` callers).
- PTY: extend a scenario to select a recipe with config, answer "later" → manifest has no config
  block (skip path); answer "now" with a value → config block present. (Heavier; at least one
  happy-path PTY assertion for the skip/later gate.)

---

## 9. Area 8 — Catalog `[[deps.cli]]` blocks

Exact blocks to add. Placement: a `[deps]` section with `[[deps.cli]]` array-of-tables, inserted
**after the `[recipe]` header + `[[capabilities]]`/`[[hooks]]` blocks and before the first
`[config.*]` table** (keeps prerequisite declaration near the top, unambiguous scope). Additive;
recipes without it already parse (verified).

**`git-pr-flow`** (after `[[capabilities]]`/`[[hooks]]`, before `[config.base_branch]`):
```toml
[[deps.cli]]
binary = "gh"
purpose = "Create and manage GitHub pull requests"
required = true
install_url = "https://cli.github.com/"
version_check = "gh --version"
min_version = "2.0.0"
```

**`gitlab-mr-flow`** (two deps — glab required, jq required for JSON parsing):
```toml
[[deps.cli]]
binary = "glab"
purpose = "Create and manage GitLab merge requests"
required = true
install_url = "https://gitlab.com/gitlab-org/cli#installation"
version_check = "glab --version"
min_version = "1.0.0"

[[deps.cli]]
binary = "jq"
purpose = "Parse GitLab API JSON responses in the MR flow"
required = true
install_url = "https://jqlang.github.io/jq/download/"
```

**`bitbucket-pr-flow`**:
```toml
[[deps.cli]]
binary = "bb"
purpose = "Create and manage Bitbucket pull requests"
required = true
install_url = "https://bitbucket.org/atlassian/bb"
```

**`trello-mcp-workflow`** (npx runs the MCP server):
```toml
[[deps.cli]]
binary = "npx"
purpose = "Run the Trello MCP server (@delorenj/mcp-server-trello)"
required = true
install_url = "https://nodejs.org/en/download"
version_check = "npx --version"
min_version = "8.0.0"
```

**`vault-canonical-store`**:
```toml
[[deps.cli]]
binary = "npx"
purpose = "Run the filesystem MCP server for the canonical vault store"
required = true
install_url = "https://nodejs.org/en/download"
version_check = "npx --version"
min_version = "8.0.0"
```

**`worktree-flow`**:
```toml
[[deps.cli]]
binary = "git"
purpose = "Create and manage git worktrees under .worktrees/"
required = true
install_url = "https://git-scm.com/downloads"
version_check = "git --version"
min_version = "2.20.0"
```

`tdd-flow` — **no block** (test command is config-driven; no fixed binary). `install_url` values are
best-effort canonical download pages; `sdd-apply` may refine the `bb` URL if the project pins a
specific Bitbucket CLI. Each edit is guarded by the existing catalog validation
(`./tests/validate.sh` + `test_recipe_schema` round-trips every catalog recipe).

### Catalog test strategy
- `test_recipe_schema` already loads catalog recipes; add an assertion that `git-pr-flow.cli_deps`
  (and one multi-dep recipe, `gitlab-mr-flow`) parse with the expected `binary`/`required`.
- `recipe-conflicts.py` tolerance (proposal risk): add/confirm a test that conflict detection still
  passes with `[[deps.cli]]` present (parsing is additive; the block is ignored by conflict logic).

---

## 10. Cross-cutting: catalog dir + sibling-load helper

Every new module that needs the catalog resolves it identically to `recipe-add._resolve_catalog_dir`:
```python
def _catalog_dir() -> Path:
    home = os.environ.get("AI_SPECS_HOME")
    root = Path(home).resolve() if home else Path(__file__).resolve().parents[2]
    return root / "catalog" / "recipes"
```
`config_wizard.py`, `dep_check.py`, `envrc-scaffold.py` each carry a `_load_sibling(name)` (the
`hub.py` 19–30 copy) to load hyphen-named siblings (`recipe-read.py`, `recipe-config-write.py`,
`toml-read.py`, `recipe-init.py`) and to import `recipe_schema`/`toml_write` directly.

## 11. Contracts summary

| Module | Public symbol | Signature | Raises |
|---|---|---|---|
| `recipe_schema.py` | `CliDep` | dataclass(binary,purpose,required=True,install_url="",version_check="",min_version="") | — |
| `recipe_schema.py` | `_parse_cli_deps` | `(raw, context) -> list[CliDep]` | `RecipeValidationError` |
| `recipe_schema.py` | `Recipe.cli_deps` | `list[CliDep] = field(default_factory=list)` | — |
| `recipe-read.py` | `recipe_to_dict` | adds `"cli_deps"` key | — |
| `dep_check.py` | `DepResult` | dataclass (see §3) | — |
| `dep_check.py` | `check_cli_deps` | `(recipe) -> list[DepResult]` | never |
| `dep_check.py` | `check_project_deps` | `(project_root) -> list[DepResult]` | never |
| `doctor.py` | `Doctor._check_recipe_cli_deps` | `(self) -> None` | never (WARN/INFO only) |
| `config_wizard.py` | `run_config_wizard` | `(recipe, existing_config) -> dict` | never (cancel → {}) |
| `config_wizard.py` | `configure_selected_recipes` | `(project_root, recipe_ids, existing_manifest) -> dict` | `RecipeConfigWriteError` bubbles |
| `recipe-config-write.py` | `update_recipe_config` | `(manifest_path, recipe_id, values) -> None` | `RecipeConfigWriteError` |
| `envrc-scaffold.py` | `collect_env_vars` | `(project_root) -> dict[str,str]` | never |
| `envrc-scaffold.py` | `generate_envrc_example` | `(project_root) -> Path` | never (backup, no `.envrc` write) |
| `hub.py` | `Action.CONFIGURE_RECIPES` | `= "configure-recipes"` | — |
| `init_tui.py` | `_configure_recipes` | `(recipes, console, catalog_dir) -> dict` | never |
| `init_tui.py` | `_render_manifest` | `(tw, name, agents, recipes, configured=None) -> str` | — |

## 12. Test inventory (strict TDD, `./tests/run.sh`)

New suites: `test_dep_check.py`, `test_config_wizard.py`, `test_recipe_config_write.py`,
`test_envrc_scaffold.py`. Extended: `test_recipe_schema.py` (`CliDepParsingTests`), `test_doctor.py`
(recipe-dep WARN + exit code unchanged), `test_hub_tui.py` (menu count 10→11), `test_init_tui.py`
(`_render_manifest` config block). RED→GREEN per phase (proposal P1–P6). All tests use the existing
`load_module`/temp-dir/subprocess/questionary-mock idioms — no new test infrastructure.

## 13. Risks & mitigations (design-level)

- **Menu-count test breaks (`test_menu_has_exact_ten_entries`).** Expected and intentional; the task
  list must update it to 11 in the same commit as the `_MENU` edit (P4). Not a regression.
- **Doctor sibling-load of `dep_check` at runtime.** Use the proven `importlib` seam; wrap in
  `try/except -> return` so a load failure degrades to "no recipe-dep rows" rather than breaking
  `ai-specs doctor` (exit-code contract).
- **`shutil.which` vs `command -v` semantics.** `which` honors PATH and returns None when absent —
  equivalent for existence and shell-free. Documented deviation from the proposal's literal
  `command -v`; `version_check` still uses a shell (author-supplied command).
- **Surgical writer corrupting the manifest.** `tomllib.loads` guard + byte-for-byte restore + a
  dedicated restore-on-invalid test (same guarantee `recipe-add.py` provides).
- **Init wizard length.** Per-recipe "Configure now / later" gate keeps a defaults-only init fast.
- **`bb` install URL uncertainty.** Flagged; `sdd-apply` confirms against the bitbucket recipe's
  skill docs before finalizing.
