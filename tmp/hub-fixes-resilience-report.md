# R4 Resilience Review — hub-fixes

**Reviewer:** HubFixesResilience  
**Scope:** hub.py, skills-list.sh, recipe-list.py, skill-resolution.py, util.py, doctor.py (bundled_skill_names)  
**Branch:** hub-fixes (based on development)  
**Date:** 2026-07-13  
**Methodology:** Static analysis of changed + adjacent files per R4 resilience rules.

---

## Summary

12 findings: **2 BLOCKER**, **3 CRITICAL**, **5 WARNING**, **2 SUGGESTION**.  
The hub handles the 4 explicitly designed degraded states (missing VERSION → `"unknown"`, empty catalog → `"no recipes"`, empty skills → `"(none)"`, broken per-recipe catalog entry → error-status entry) and all questionary abort paths correctly. However, several unguarded call sites and `subprocess.run` patterns create hard crash paths in production with no observability to detect them before they hit users.

---

## BLOCKER

### B1. `list_recipes()` raising propagates unhandled → hub crash on Recipes Add/Remove

**Severity:** BLOCKER  
**Files:** `hub.py:443,454`, `recipe-list.py:17-44`  
**Rule:** *Flag failures with no fallback, retry, or graceful-degradation path.*  
**Evidence:**

- `hub.py:443` — `choices = recipe_add_choices(_recipes.list_recipes(target))` — no `try/except`.
- `hub.py:454` — `choices = recipe_remove_choices(_recipes.list_recipes(target))` — no `try/except`.
- `recipe-list.py:17-23` — `_load_toml_read()`:
  ```python
  spec = importlib.util.spec_from_file_location("toml_read_internal", module_path)
  module = importlib.util.module_from_spec(spec)       # AttributeError if spec is None
  ```
  No `if spec is None` guard. If `toml-read.py` is missing, has a syntax error, or has an unimportable dependency, `spec` is `None`, and `.loader` access raises `AttributeError: 'NoneType' object has no attribute 'loader'`. Same pattern in `_load_recipe_read()` (lines 26-32).
- `recipe-list.py:35-43` — `_resolve_catalog_dir()` calls `Path.iterdir()` (via caller `list_recipes` line 66) with no OSError guard. `PermissionError` or broken FS state crashes the submenu.

**Why it matters:** Any error — missing internal module, syntax error, broken symlink in `catalog/recipes/`, permissions — transforms a "list/add/remove recipes" menu action into an unhandled traceback crash. The hub terminates. No fallback message like "Recipes unavailable."

**Remediation:** Wrap both `_recipes.list_recipes(target)` calls in `try/except Exception` around lines 443-448 and 454-459. Catch and print `[yellow]Recipes unavailable: {exc}[/yellow]` then continue.

---

### B2. `_load_sibling()` at import time crashes `hub.py` entirely if any sibling module is broken

**Severity:** BLOCKER  
**Files:** `hub.py:22-37`  
**Rule:** *Flag failures with no fallback, retry, or graceful-degradation path.*  
**Evidence:**

```python
def _load_sibling(name: str):
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise RuntimeError(f"unable to load sibling module {path}")  # no handler
    ...

_util = _load_sibling("util")
_doctor = _load_sibling("doctor")
_recipes = _load_sibling("recipe-list")
_skillres = _load_sibling("skill-resolution")
```

All four modules load at module scope. If any of `{util, doctor, recipe-list, skill-resolution}.py` has a syntax error, missing dependency, or import-time exception, the entire `import hub` fails. The hub cannot start at all — even for features that don't use the broken module.

**Why it matters:** A single bad `doctor.py` (or even a transitively imported file) prevents the entire interactive hub from loading. Users see a Python traceback, not a degraded-but-functional hub. No "safe mode" or feature-level degradation exists.

**Remediation:** Lazy-load feature modules on first use (`_run_recipes_submenu` would `_load_recipes()` with error catch), or wrap the import site in `try/except ImportError` and set modules to sentinel objects whose methods return degraded defaults.

---

## CRITICAL

### C1. `subprocess.run()` in `DelegateRunner` has no timeout — hangs block the hub indefinitely

**Severity:** CRITICAL  
**File:** `hub.py:305`  
**Rule:** *Flag performance regressions that exceed user-visible budgets or lack measurement.* Evidence of SLO/latency impact required.  
**Evidence:**

```python
return subprocess.run(argv).returncode
```

Every menu action that delegates to the CLI (Sync, Doctor, Upgrade, Init, Help, recipe add/remove/configure) calls `DelegateRunner.run()` → `subprocess.run(argv)` with **no timeout argument**. A hanging subprocess (e.g., slow network in `ai-specs sync`, blocked I/O, deadlocked background process) blocks the interactive hub loop indefinitely. The user sees no progress indicator or error — the terminal simply freezes.

**Why it matters:** SLO for interactive hub response is measured in seconds. A 30-second hang from `ai-specs doctor` violates the implicit SLO of the menu loop with no visibility. In headless/CI non-interactive mode this is worse: `_run_noninteractive()` hangs the pipeline.

**Remediation:** Add a `timeout=` to `subprocess.run()` (suggest 120s) and catch `subprocess.TimeoutExpired`. Print an error and return a non-zero exit code, allowing the hub to continue.

---

### C2. `categorize_skills()` → `collect_skills()` → filesystem `iterdir()` not wrapped — OSError crashes Skills submenu

**Severity:** CRITICAL  
**Files:** `skill-resolution.py:33-89`, `hub.py:363-422`  
**Rule:** *Flag failures with no fallback, retry, or graceful-degradation path.*  
**Evidence:**

- `skill-resolution.py:38` — `for child in sorted(skills_dir.iterdir()):` — `iterdir()` raises `OSError` (e.g., `PermissionError`, `ENOTDIR`) on broken filesystem state. No `try/except`.
- `skill-resolution.py:49` — Same for `recipe_dir.iterdir()`.
- `skill-resolution.py:73` — Same for `deps_dir.iterdir()`.
- `hub.py:377` — `buckets = categorize_skills(target, cli_home)` is called without a `try/except` in `_run_skills_submenu`. Any of the three `iterdir()` calls raising crashes the Skills submenu.

**Why it matters:** A single unreadable directory inside the project's `ai-specs/skills/`, `.recipe/`, or `.deps/` kills the entire Skills interaction. No error message, no fallback rendering showing what *can* be read. For a "list/read" operation this is especially jarring — the user loses all skill visibility.

**Remediation:** Wrap `categorize_skills()` or the per-tier `_scan_*` functions in `try/except OSError`. On error, return empty buckets and print a non-fatal warning.

---

### C3. `KeyboardInterrupt` during `pause()` not caught — Ctrl-C prints traceback instead of graceful exit

**Severity:** CRITICAL  
**File:** `hub.py:269-275`  
**Rule:** *Flag failures with no fallback, retry, or graceful-degradation path.*  
**Evidence:**

```python
def pause(message: str = "Press Enter to return…") -> bool:
    try:
        input(message)
        return True
    except EOFError:         # catches piped stdin EOF
        return False
```

`KeyboardInterrupt` (Ctrl-C) is not caught. The exception propagates through `_run_skills_submenu`, `_run_recipes_submenu`, `_run_agents_submenu`, and `_run_interactive_hub` up to `main()` → unhandled Python traceback to stderr.

**Why it matters:** Users pressing Ctrl-C to abort a hub pause see an ugly traceback instead of a clean exit. The hub becomes fork-bomb visible. Falls under "graceful degradation" requirement.

**Remediation:** Catch `(EOFError, KeyboardInterrupt)` and return `False` (same as EOF — exit the hub cleanly).

---

## WARNING

### W1. `_run_agents_submenu` file writes have no error handling — OSError crashes hub

**Severity:** WARNING  
**File:** `hub.py:499-524`  
**Rule:** *Flag failures with no fallback, retry, or graceful-degradation path.*  
**Evidence:**

```python
text = manifest_path.read_text(encoding="utf-8")     # line 499 — OSError unhandled
...
manifest_path.write_text("".join(new_lines), encoding="utf-8")  # line 524 — OSError unhandled
```

If the manifest file becomes unreadable (deleted, permissions changed, NFS stale handle) between the start of the submenu and the write, or unwritable on write, the hub crashes with no error message. The `if not manifest_path.is_file():` guard at line 478 only checks existence at entry, not at write time.

**Why it matters:** Error during manifest write leaves the file in an indeterminate state (half-written via truncation by `.write_text`). The user sees a traceback and no recovery suggestion.

**Remediation:** Wrap the read/write in `try/except OSError` with a Rich `[red]` error message. On write failure, consider backing up the original file before overwriting.

---

### W2. `DelegateRunner.run()` has no `FileNotFoundError` guard — corrupt install crashes hub

**Severity:** WARNING  
**File:** `hub.py:297-305`  
**Rule:** *Require evidence for rollback/fix-forward readiness: a concrete recovery path must exist.*  
**Evidence:**

```python
return subprocess.run(argv).returncode
```

If the `cli` path (`_util.ai_specs_home() / "bin" / "ai-specs"`) does not exist — e.g., partial upgrade, manual deletion, stale symlink — `subprocess.run(argv)` raises `FileNotFoundError`. This propagates unhandled through `_run_interactive_hub` → hub termination.

**Why it matters:** A broken install produces a traceback, not a user-facing recovery message. No guidance like "ai-specs binary not found — reinstall with curl command."

**Remediation:** Wrap `DelegateRunner.run()` in `try/except FileNotFoundError` with a clear error message and recovery hint.

---

### W3. No logging infrastructure — all errors emitted via `print()`

**Severity:** WARNING  
**Files:** `hub.py:207-209,416,445,456,469,525,571-573,609`, `util.py:56,66,79,83,92`, `skill-resolution.py:29-30`  
**Rules:** *Flag releases that can regress without alerting/observability hooks. Require evidence for rollback/fix-forward readiness.*  
**Evidence:**

Every error path uses bare `print()` (to stdout or stderr) or Rich `console.print()`. There is no `logging.getLogger()`, no structured format, no level routing, no way to suppress or redirect in CI.

| Location | Output | Context |
|----------|--------|---------|
| `hub.py:208-209` | `print(f"ERROR: ...", file=sys.stderr)` | Uninitialized error |
| `hub.py:571-573` | `print(f"✓ done")` / `print(f"✗ exited {rc}")` | Delegate result |
| `util.py:66` | `print(f"ERROR: cannot create vendor dir ...", file=sys.stderr)` | Dep install failure |
| `skill-resolution.py:29` | `print(f"  ! ...", file=sys.stderr)` | Duplicate skill warning |

**Why it matters:** Production observability rules require alerting hooks for error-rate escalation (>1% investigate, >2% emergency, >5% all-hands). Bare `print()` cannot be:
- Routed to different handlers per environment
- Augmented with correlation IDs or timestamps
- Measured for rate-based alerting
- Filtered by severity

In headless/automated runs (`stdlib-mode`), these prints are visible but unstructured, making automated classification impossible.

**Remediation:** Add a `logging.getLogger(__name__)` at the module level; replace the most critical error prints with `logging.error()` / `logging.warning()`. At minimum, add `sys.excepthook` in `main()` to intercept unhandled exceptions and print a structured message before exit.

---

### W4. `status_summary()` re-runs doctor on every hub loop iteration — no caching

**Severity:** WARNING  
**File:** `hub.py:540`  
**Rule:** *Flag performance regressions that exceed user-visible budgets or lack measurement.*  
**Evidence:**

```python
def _run_interactive_hub(target: Path) -> int:
    while True:
        summary = status_summary(target)    # ← runs full doctor scan every iteration
```

Each menu render (every keypress → next action) re-invokes `Doctor(target).run()`, which scans the entire project filesystem. For large projects with many checks, this adds O(n) delay per iteration.

**Why it matters:** The interactive hub has an implicit SLO of ≤200ms per keypress for feel. A project with 50+ doctor checks that each read files or call subprocesses can degrade to multi-second menu pauses.

**Remediation:** Cache the status summary and only refresh on explicit "Sync" or "Doctor" actions, or on a timer (>10s stale).

---

### W5. Top-level module load of `_recipes` drags in transitive dependencies even when unused

**Severity:** WARNING  
**File:** `hub.py:36`  
**Rule:** *Flag failures with no fallback, retry, or graceful-degradation path.*  
**Evidence:**

```python
_recipes = _load_sibling("recipe-list")
```

`recipe-list.py` imports `toml-read` and `recipe-read` at load time. These modules may have their own import chains. The import happens at `hub.py` module scope, so even if the user never opens the Recipes submenu, any dependency failure in `recipe-list.py` → `toml-read.py` → transitive chain prevents the entire hub from starting.

**Why it matters:** Fragile import coupling. A minor issue in an unused feature (e.g., a syntax error in `recipe-read.py` after a botched deploy) takes down the entire hub. Violates the principle of failing closed only on genuine core dependencies.

**Remediation:** Lazy-import `_recipes` on first use inside `_run_recipes_submenu` with a `try/except` that reports recipe unavailability to the user.

---

## SUGGESTION

### S1. `_offer_init()` creates two `DelegateRunner` instances

**File:** `hub.py:583-590`  
**Evidence:**

```python
cli = _util.ai_specs_home() / "bin" / "ai-specs"
rc = DelegateRunner(cli=cli, target=target).run(Action.INIT)
...
rc = DelegateRunner(cli=cli, target=target).run(Action.SYNC)
```

Two identical `DelegateRunner` constructions. Minor — reuse the first instance.

**Remediation:** `runner = DelegateRunner(cli=cli, target=target)`, then `runner.run(...)`.

---

### S2. Missing test: `list_recipes()` raising in recipes submenu

**File:** `tests/test_hub.py`, `tests/test_hub_tui.py`  
**Evidence:**

No test validates that the Recipes submenu survives a broken `list_recipes()` call. The 44 unit + 10 TUI tests cover happy-path recipe pickers (`TestRecipeAddPicker`) and empty choices but not the exception path.

**Remediation:** Add a test that mocks `_recipes.list_recipes` to raise `RuntimeError` and asserts the hub shows an error message and continues, rather than crashing.

---

## Verified Acceptable Degraded States

| State | Handling | Location |
|-------|----------|----------|
| Missing VERSION → `"unknown"` | ✅ Explicit fallback, tested | `hub.py:86-88`, test `TestReadVersion` |
| Empty catalog (no recipes) | ✅ Prints `[yellow]No catalog recipes available[/yellow]` | `hub.py:444-448` |
| No installed recipes to remove | ✅ Prints `[yellow]No recipes installed[/yellow]` | `hub.py:455-459` |
| Empty skills bucket | ✅ Prints `  (none)` per bucket | `hub.py:352-353` |
| No skills match inspect | ✅ Prints `[yellow]No skills found[/yellow]` | `hub.py:396-400` |
| Missing bundled-skills dir (Python) | ✅ Returns hardcoded `["skill-creator", "skill-sync"]` | `doctor.py:25-26` |
| Missing bundled-skills dir (shell) | ✅ Falls back to same hardcoded list | `skills-list.sh:124-125` |
| Per-recipe catalog parse error | ✅ Adds error-status entry, continues | `recipe-list.py:82-88` |
| Aborted questionary (None) | ✅ All callers check None → return to menu | `hub.py:238-246,259,266,289` |
| EOFError on pause | ✅ Caught, returns False → clean exit | `hub.py:269-275` |
| Deps missing (interactive) | ✅ Exit code 3, install prompt | `util.py:35-94` |
| Deps missing (non-TTY/CI) | ✅ Exit code 3, no prompt | `util.py:53-54` |
| Target is not a directory | ✅ Stderr error + exit 2 | `hub.py:608-610` |
| Non-initialized + no TTY | ✅ Stderr guidance + exit 2 | `hub.py:615-617` |

---

## Risk Score Summary

| Category | Count | Highest Severity |
|----------|-------|------------------|
| Unhandled exceptions in menu callbacks | 2 (B1, C2) | BLOCKER |
| Import-time fragility | 2 (B2, W5) | BLOCKER |
| Hanging subprocess (no timeout) | 1 (C1) | CRITICAL |
| Graceful abort blind spots (KeyboardInterrupt) | 1 (C3) | CRITICAL |
| File I/O without error handling | 1 (W1) | WARNING |
| Missing binary guard (corrupt install) | 1 (W2) | WARNING |
| Observability (logging vs print) | 1 (W3) | WARNING |
| Performance (uncached doctor loop) | 1 (W4) | WARNING |
| Test coverage gap | 1 (S2) | SUGGESTION |

**Overall assessment:** The intentionally designed degraded states are well-handled and tested. However, the unguarded call sites (especially `list_recipes()` propagation and import-time cascading failures) would silently turn broken integrations into a broken hub in production. The most impactful single fix is wrapping the `_run_recipes_submenu` `list_recipes()` calls in try/except.
