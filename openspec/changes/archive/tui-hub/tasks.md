# Tasks: `ai-specs` TUI hub (no-subcommand entrypoint)

Change: tui-hub
Depends on: `openspec/changes/tui-hub/design.md` (approved)
Branch: `tui-hub` (worktree `.worktrees/tui-hub`)
Strict TDD: `true` — test runner `./tests/run.sh` (unittest discovery). Every task follows RED (write failing test) → GREEN (minimal impl) → TRIANGULATE (add a second case) → REFACTOR. `./tests/validate.sh` runs py_compile + `bash -n`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 1100–1400 (additions ~1000–1250, deletions ~50–150) |
| 400-line budget risk | High |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (P1 infra + util) → PR 2 (P2 hub core + routing) → PR 3 (P3 interactive + delegation) → PR 4 (P4 docs + polish) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
```

**Rationale:** the change adds ~7 new files (`util.py`, `hub.py`, `hub.sh`, `test_util.py`, `test_hub.py`, `test_hub_tui.py`, `_vendor/` tree) and modifies `init_tui.py` and `bin/ai-specs`. Estimated additions alone exceed the 1200-line review budget for a single PR. The four phases are natural autonomous work units with clean start/finish/verify/rollback boundaries (each phase leaves the test suite green; P1 is pure extraction, P2 adds dep-free gating, P3 adds interactive-only code that is `_has_deps()`-gated, P4 is docs). Recommend stacked-to-main so each PR is reviewed against a stable base while keeping the branch linear.

---

## P1 — Infrastructure (shared helpers + vendoring)

**Goal:** extract the deps-gate into a pure-stdlib `util.py`, make `init_tui.py` delegate to it (test-preserving), and pre-vendor rich + questionary so the front door never blocks on a network pip install. Pure refactor — no user-visible behavior change.

### Task P1.1 — `lib/_internal/util.py` (new) + `tests/test_util.py` (new)

**RED → GREEN → TRIANGULATE**

**Files:**
- Create `lib/_internal/util.py`
- Create `tests/test_util.py`

**`util.py` contents (pure stdlib at import time):**
- `DEPS_SPEC = ["rich>=13.0.0,<15", "questionary>=2.0.0,<2.1"]` (moved from `init_tui.py:41`)
- `ai_specs_home() -> Path` — honors `$AI_SPECS_HOME` then falls back to `Path(__file__).resolve().parents[2]` (mirrors `init_tui._ai_specs_home` at `init_tui.py:55-59`)
- `vendor_dir() -> Path` — `ai_specs_home() / "lib" / "_vendor"`
- `is_initialized(root: Path) -> bool` — thin `(root / "ai-specs" / "ai-specs.toml").is_file()` check (design §2.1; the marker `init.sh:148/155` already uses)
- `ensure_deps(vendor: Path, *, prompt: bool = True) -> int | None` — body moved verbatim from `init_tui._ensure_deps` (`init_tui.py:66-122`), parameterized by `vendor` instead of calling `_vendor_dir()` internally. Returns `3` on failure, `None` on success.

**`tests/test_util.py` — RED first, then GREEN, then TRIANGULATE:**
- Load `util.py` via `importlib.util.spec_from_file_location("util", UTIL_PATH)` (same pattern as `test_init_tui._load` at `test_init_tui.py:20-25`). Assert the module imports **without** rich/questionary present (proves the dep-free import contract).
- `TestAiSpecsHome`:
  - `AI_SPECS_HOME` env set → returns `Path(env).resolve()`
  - env unset → returns `Path(__file__).resolve().parents[2]` (TRIANGULATE: both branches)
- `TestVendorDir`:
  - `vendor_dir()` == `ai_specs_home() / "lib" / "_vendor"`; assert under a temp `AI_SPECS_HOME` (TRIANGULATE)
- `TestIsInitialized` (design §7.1):
  - temp dir with `ai-specs/ai-specs.toml` file → `True`
  - temp dir without manifest → `False`
  - temp dir where `ai-specs/ai-specs.toml` is a **directory** (not a file) → `False` (TRIANGULATE edge)
- `TestEnsureDeps` (parity with `init_tui._ensure_deps` behavior guarded by `test_init_tui.py:113-140`):
  - `vendor` dir exists + sourceable rich/questionary (or mock imports) → returns `None`
  - vendor `mkdir` failure (`PermissionError` mock on a `BoomPath`) → returns `3`; on a non-TTY → returns `3` without prompting
  - pip install mock succeeds → returns `None`; fails → returns `3` (TRIANGULATE the install branch)

**Acceptance criteria:**
- `./tests/run.sh` green (test_util.py passes; existing test_init_tui.py may temporarily double-cover `_ensure_deps` — acceptable in P1, cleaned up in P1.3).
- `python3 -c "import importlib.util, sys; sys.path.insert(0,'lib/_vendor'); importlib.util.spec_from_file_location('util','lib/_internal/util.py')"` imports with no third-party dep at import time (verify by degrading `sys.path`).
- `util.is_initialized` covers the file-vs-directory edge.

**Estimated lines:** ~140 (`util.py` ~90 incl. docstrings; `test_util.py` ~50)

---

### Task P1.2 — Pre-vendor rich + questionary into `lib/_vendor/`

**Files:**
- Create `lib/_vendor/` containing pure-Python wheels of `rich`, `questionary`, `prompt_toolkit`, `wcwidth` (committed tree)
- Create `scripts/vendor-deps.sh` (maintenance target: `pip install --target lib/_vendor <DEPS_SPEC>`), idempotent

**Work:**
- Run `pip install --target lib/_vendor "rich>=13.0.0,<15" "questionary>=2.0.0,<2.1"` once; commit the resulting pure-Python tree. `prompt_toolkit` + `wcwidth` come in transitively.
- Add `scripts/vendor-deps.sh` that re-runs the same command (reproducible refresh on dep bump; design §6 maintenance cell).
- Verify `lib/_vendor/rich/__init__.py` and `lib/_vendor/questionary/__init__.py` exist.

**Test (RED → GREEN):** add to `tests/test_util.py`:
- `TestVendorTree`: `from lib._vendor import rich, questionary` importable when `lib/_vendor` is on `sys.path`; assert package metadata `rich.version` is within the pin range. Gated by `@unittest.skipUnless((ROOT/"lib"/"_vendor"/"rich").is_dir(), "vendor not present")` so the test is environment-safe.

**Acceptance criteria:**
- `lib/_vendor/rich/`, `lib/_vendor/questionary/`, `lib/_vendor/prompt_toolkit/`, `lib/_vendor/wcwidth/` all present.
- `scripts/vendor-deps.sh` reproducible (re-run yields the same pins; `bash -n` clean).
- Existing `test_init_tui._has_deps()` at `:237` now resolves via `lib/_vendor` instead of the system install.

**Estimated lines:** ~15 (scripts + test assertion; the vendored tree is binary artifacts, not counted as changed lines for review)

---

### Task P1.3 — Refactor `init_tui.py` to delegate to `util.py`

**Files:**
- Modify `lib/_internal/init_tui.py`

**Changes (design §2.2):**
- Add `_load_sibling("util")` using the same `importlib.util.spec_from_file_location` pattern already used by `init_tui._load_toml_write()` (`init_tui.py:156-166`). Bind `_util = _load_sibling("util")`.
- Keep module-level wrappers with identical names so existing patch-points survive:
  - `_ai_specs_home() -> Path` → `return _util.ai_specs_home()`
  - `_vendor_dir() -> Path` → `return _util.vendor_dir()`
  - `_ensure_deps() -> int | None` → `return _util.ensure_deps(_vendor_dir())`
- Remove the inline `DEPS_SPEC` constant; reference `_util.DEPS_SPEC` where `_ensure_deps` body used it (the body moves to `util`).
- Remove the inline `_ensure_deps` body (`init_tui.py:66-122`); the wrapper now t delegates.
- `run_wizard` and `main` continue to call the module-level `_ensure_deps`/`_ai_specs_home`/`_vendor_dir` names — unchanged call sites.

**Test (guard):** run the existing `tests/test_init_tui.py` suite unchanged. Specifically:
- `test_ensure_deps_mkdir_failure_returns_3` (`:113-140`) patches `self.mod._vendor_dir` → wrapper passes the patched `BoomPath` into `util.ensure_deps` → the `vendor.mkdir` boom still raises `PermissionError` → returns `3`. (RED: before this task the test expects the inline body; after the refactor it must still pass — this is the GREEN check.)
- `test_run_wizard_returns_3_on_non_tty` (`:92-101`) patches `self.mod._ensure_deps` → wrapper is patched → behavior preserved.
- `TestRenderManifest` (`:30-59`) exercises `_load_toml_write`, unrelated — must remain green.

**Acceptance criteria:**
- `./tests/run.sh` green with zero changes to `test_init_tui.py`.
- `_ensure_deps` / `_ai_specs_home` / `_vendor_dir` in `init_tui.py` are one-line delegators; logic lives only in `util.py`.
- `git diff lib/_internal/init_tui.py` is a pure extraction (net deletions of the body, additions of the delegators + `_load_sibling`).

**Estimated lines:** ~30 changed (−60 body, +45 delegators + loader, net ~+5)

---

## P2 — Hub core (non-interactive first)

**Goal:** the dep-free, TTY-free decision surface. `decide_mode`, `status_summary`, non-interactive status output, `hub.sh` shim, and `bin/ai-specs` routing. After P2, bare `ai-specs` is no longer a dead end and CI/pipe paths are fully exercised with zero deps.

### Task P2.1 — `lib/_internal/hub.py` core: `decide_mode`, `StatusSummary`, `_run_noninteractive`

**RED → GREEN → TRIANGULATE**

**Files:**
- Create `lib/_internal/hub.py`
- Create `tests/test_hub.py`

**`hub.py` core (dep-free at import time — design §1.1, §1.2, §1.3):**
- `_load_sibling(name)` — `importlib.util.spec_from_file_location` by absolute path (mirror `init_tui._load_toml_write`). Bind `_util = _load_sibling("util")`, `_doctor = _load_sibling("doctor")`.
- `class Mode(Enum)` — `INTERACTIVE_HUB`, `NONINTERACTIVE_STATUS`, `OFFER_INIT`, `ERROR_UNINITIALIZED` (design §1.2).
- `decide_mode(*, initialized: bool, tty: bool) -> Mode` — pure, no I/O.
- `@dataclass(frozen=True) class StatusSummary` — `root, ok, info, warn, error, exit_code, headline, checks` (design §1.3).
- `status_summary(root: Path) -> StatusSummary` — runs `_doctor.Doctor(root)` in-process, counts `Severity`, builds `headline` ("healthy" / "N warnings" / "N error(s)"). Dep-free (Doctor is stdlib).
- `_run_noninteractive(target: Path) -> int` — prints `status_summary(target)` as plain text + the command-list (names + one-line descriptions from `_MENU`). No rich. Returns `0`.
- `_print_uninit_error(target)` — stderr guidance + returns `2` via caller.
- `_parse_args(argv) -> argparse.Namespace` — `--help` + positional `target` (default `os.getcwd()`), mirroring `doctor.py:main`.
- `main(argv=None) -> int` — argparse → resolve target → `decide_mode(initialized=_util.is_initialized(target), tty=...)`. Route to `_print_uninit_error`/`_run_noninteractive` for the dep-free modes. Interactive modes call `_util.ensure_deps` then (placeholder) raise `NotImplementedError` (P3 implements them).
- `if __name__ == "__main__": sys.exit(main())`.

**`tests/test_hub.py` — RED first:**
- Load `hub.py` via `importlib.util.spec_from_file_location("hub", HUB_PY)` (proves dep-free import; design §7.1).
- `TestGatingDecision` (design §7.1) — 4 states; TRIANGULATE by parameterizing all `(initialized, tty)` combinations and asserting the exact `Mode`.
- `TestIsInitialized` (delegated — already in `test_util.py`; here assert `hub._util.is_initialized` is the same callable).
- `TestStatusSummary` (design §7.1): build a temp project via `subprocess.run([CLI, "init", tmp])` (reuse `test_doctor.ai_specs_init` at `:72`); assert `ok >= 1`, `exit_code == 0`; then mutate the manifest to raise a WARN/ERROR (e.g. clear `agents.enabled`) and assert counts + `headline` reflect it. Dep-free.
- `TestNonInteractiveStatus` (design §7.1, shell integration via `subprocess`, piped stdio ⇒ not a TTY):
  - initialized temp project, bare `[CLI]` → `rc == 0`, stdout contains status summary + command names (Sync/Doctor/…/Quit)
  - uninitialized temp dir, bare `[CLI]` piped → `rc == 2`, stderr mentions `init`
  - `[CLI, "help"]` → `rc == 0`, stdout equals the existing help text (regression guard)
  - `[CLI, "definitely-not-a-command"]` → `rc == 2` (default case preserved)
  - `[CLI, "hub", tmp]` explicit form → routes to `hub.sh`
  - `[CLI, "--help"]` style handled by `_parse_args`

**Acceptance criteria:**
- `TestGatingDecision`, `TestIsInitialized`, `TestStatusSummary`, `TestNonInteractiveStatus` all pass under `./tests/run.sh` with **no** third-party deps installed (dep-free cores run in every environment).
- `decide_mode` is pure: no file, env, or tty access inside it.
- `status_summary` works on a real `ai-specs init` temp project.
- Interactive code paths (`_run_interactive_hub`, `_offer_init`) may be `NotImplementedError` stubs in P2 — the dep-free tests do not reach them.

**Estimated lines:** ~220 (`hub.py` ~130 core; `test_hub.py` ~90)

---

### Task P2.2 — `lib/hub.sh` (new) — shim mirroring `doctor.sh` + CI guard

**Files:**
- Create `lib/hub.sh`

**Content (design §3):**
- `#!/usr/bin/env bash` + `set -euo pipefail`
- `SCRIPT_DIR` / `AI_SPECS_HOME` / `HUB_PY` resolution (byte-identical pattern to `lib/doctor.sh:7-9`)
- `usage()` function matching `doctor.sh:10-20` shape, documenting the non-interactive behavior
- arg-parse loop (`--help|-h`, `--`, `-*`, positional `TARGET_PATH`) — same shape as `doctor.sh:21-37`
- default `TARGET_PATH` to `pwd`, `cd` to resolve
- **One** bash guard (design §3): `if [[ ! -t 0 || ! -t 1 ]] && [[ ! -f "$TARGET_PATH/ai-specs/ai-specs.toml" ]]` → stderr error + `exit 2`. This is the defensive fast-path so bare `ai-specs` in a CI pipe never imports Python.
- `exec python3 "$HUB_PY" "$TARGET_PATH"`

**Tests:** covered by `TestNonInteractiveStatus` in `tests/test_hub.py` (the pipe-no-TTY-uninitialized path exercises the bash guard; the initialized piped path falls through to `hub.py`).

**Acceptance criteria:**
- `bash -n lib/hub.sh` clean (checked by `./tests/validate.sh`).
- Bare `ai-specs` in a pipe on an uninitialized dir exits 2 from bash **without** spawning Python (verify with `PYTHONPATH` unset and a timeout — the guard fires pre-`exec`).
- `ai-specs hub --help` prints usage, exit 0.

**Estimated lines:** ~45

---

### Task P2.3 — Route `bin/ai-specs` bare invocation to hub

**Files:**
- Modify `bin/ai-specs`

**Changes (design §4):**
- Replace `cmd="${1:-help}"` / `shift || true` (`bin/ai-specs:29-30`) with:
  ```bash
  if [[ $# -eq 0 ]]; then
      cmd="hub"
  else
      cmd="$1"
      shift
  fi
  ```
- Add the case arm `hub) bash "$LIB_DIR/hub.sh" "$@" ;;` alongside the existing arms (`bin/ai-specs:32-42`).
- `help|-h|--help)` arm unchanged — existing help text preserved byte-for-byte.
- `*)` unknown-command arm unchanged (`bin/ai-specs:67-69`).

**Tests (in `tests/test_hub.py::TestNonInteractiveStatus`, written RED in P2.1, now GREEN):**
- bare `[CLI]` → hub (initialized → 0; uninitialized piped → 2)
- `[CLI, "help"]` → exact existing help text (regression guard — design §10 "help regression")
- `[CLI, "definitely-not-a-command"]` → exit 2 (default case preserved)
- `[CLI, "hub", tmp]` explicit form → routes to `hub.sh`

**Acceptance criteria:**
- `./tests/run.sh` green.
- `ai-specs help` output byte-identical to pre-change (assert in test).
- Bare `ai-specs` ≠ `ai-specs help` (they route differently; help stays a real subcommand).
- Unknown commands still exit 2.

**Estimated lines:** ~10 changed (−2 old dispatch, +8 new dispatch + case arm)

---

## P3 — Interactive menu + delegation

**Goal:** the full interactive hub. `_has_deps()`-gated questionary menu, inline Version, delegated streaming subcommands, offer-init flow, PTY E2E. After P3 every success criterion involving a TTY is met.

### Task P3.1 — `CommandMenu` + `Action` enum

**RED (test skeleton) → GREEN → TRIANGULATE**

**Files:**
- Modify `lib/_internal/hub.py` (add `Action`, `_MENU`, `CommandMenu`)
- Modify `tests/test_hub.py` (add dep-gated `TestCommandMenu` or place in `test_hub_tui.py`)

**`hub.py` additions (design §1.4):**
- `class Action(Enum)` — `SYNC, DOCTOR, SKILLS, RECIPES, RULES_AUDIT, UPGRADE, VERSION, HELP, INIT, QUIT` (design §1.4).
- `_MENU: list[tuple[Action, str, str]]` — `(action, title, one-line description)` (design §1.4, exact table).
- `@dataclass class CommandMenu` with `prompt() -> Action` — lazily `import questionary`; build `questionary.Choice(title, value=act, description=desc)`; `.ask()`; `None` (Ctrl-C/EOF) → `Action.QUIT`.
- `_run_noninteractive` uses `_MENU` for the plain-text command list (P2.1 wired this to `_MENU`; ensure the descriptions are populated).

**Tests (`test_hub_tui.py`, `@unittest.skipUnless(_has_deps(), ...)`:**
- `TestCommandMenu`: monkeypatch `questionary.select(...).ask` to return each `Action`; assert `CommandMenu().prompt()` returns it. Assert `None` → `Action.QUIT`. TRIANGULATE across all 10 actions.

**Acceptance criteria:**
- `_has_deps()`-gated tests pass when deps present; skipped when absent.
- Menu has exactly the 10 entries in design §1.4 with the exact one-line descriptions.
- Ctrl-C/EOF at the menu ⇒ `Action.QUIT` (clean exit 0), matching `init_tui` cancel semantics.

**Estimated lines:** ~70 (`hub.py` ~45; tests ~25)

---

### Task P3.2 — `DelegateRunner` (suspend → run → resume)

**RED → GREEN**

**Files:**
- Modify `lib/_internal/hub.py` (add `DelegateRunner`)
- Modify `tests/test_hub.py` or `tests/test_hub_tui.py`

**`hub.py` additions (design §1.5, §5):**
- `@dataclass class DelegateRunner` — `cli: Path` (=`util.ai_specs_home()/"bin"/"ai-specs"`), `target: Path`.
- `run(self, action: Action, extra: list[str] | None = None) -> int` — `subprocess.run([str(self.cli), action.value, str(self.target), *(extra or [])])` with **inherited** stdio (no capture). Returns `returncode`. (Design §5.1: no raw-mode save/restore — questionary restores cooked mode after `.ask()`.)
- No openpty/termios juggling in `hub.py` (design §5.2).

**Tests (dep-gated, `test_hub_tui.py`:** wait — `DelegateRunner` is dep-free itself; test it in `test_hub.py` without a TTY:
- `TestDelegateRunner`: point `cli` at a temp script that echoes + exits N; assert `run(Action.VERSION, [])` returns N and the echo reached `subprocess.run`'s inherited stdout (capture via `subprocess.run` wrapper in the test, mocking subprocess.run to assert argv shape). TRIANGULATE with `extra=["--dry-run"]` for `UPGRADE`.

**Acceptance criteria:**
- `DelegateRunner.run` builds `argv = [cli, action.value, target, *extra]` (assert argv shape in test).
- Returns the child's returncode; does not raise on non-zero child.
- No `openpty`/`termios` usage in `hub.py` (grep-clean).

**Estimated lines:** ~45 (`hub.py` ~20; tests ~25)

---

### Task P3.3 — `StatusPanel` (rich render) + `_run_interactive_hub` loop

**RED → GREEN (snapshot) → TRIANGULATE**

**Files:**
- Modify `lib/_internal/hub.py` (add `StatusPanel`, `_run_interactive_hub`)
- Create/modify `tests/test_hub_tui.py` (add `TestStatusPanelRender`)

**`hub.py` additions (design §1.3, §1.6):**
- `@dataclass class StatusPanel` — `summary: StatusSummary`; `render()` lazily imports `rich.panel.Panel` + `rich.table.Table`, builds a severity-colored `Panel` (green/yellow/red border by worst severity), one row per non-OK check + a summary line.
- `_run_interactive_hub(target: Path) -> int` (design §1.6): loop — `StatusPanel(status_summary(target)).render()`; `CommandMenu().prompt()`; dispatch:
  - `Action.VERSION` → inline read of `util.ai_specs_home()/"VERSION"` (no subprocess)
  - `Action.QUIT` → `return 0`
  - `Action.HELP` → delegate `ai-specs help` (or inline) → resume
  - everything else → `DelegateRunner.run(action)` → print `"✓ done"` / `"✗ exited N"` → `input("Press Enter to return…")` → re-loop (recompute status each iteration)
- Wire `main()` to call `_run_interactive_hub` for `Mode.INTERACTIVE_HUB` (replace P2 `NotImplementedError`).

**Tests (`test_hub_tui.py`, `@unittest.skipUnless(_has_deps(), ...)` — design §7.2):**
- `TestStatusPanelRender`: render `StatusPanel(status_summary(root)).render()` into `rich.console.Console(file=io.StringIO(), width=80, force_terminal=True)`; assert substrings: panel title `ai-specs`, the target path, a `Summary:`-style line, severity tokens for seeded checks. Snapshot-like (substring, not byte-exact — tolerates rich version jitter).
- `TestDelegateRunnerResume` (unit): mock `DelegateRunner.run` + `builtins.input`; assert the loop calls run then input then re-renders; `Action.QUIT` breaks the loop returning 0.

**Acceptance criteria:**
- `_has_deps()`-gated StatusPanel tests pass with vendored deps; skipped without.
- Border color reflects worst severity (green=healthy, yellow=warn-only, red=any error).
- Version is rendered inline (no subprocess); assert the `VERSION` file content appears on stdout.
- `Action.QUIT` returns 0 and does not delegate.

**Estimated lines:** ~140 (`hub.py` ~95; tests ~45)

---

### Task P3.4 — `_offer_init` + interactive `OFFER_INIT` wiring

**RED (PTY) → GREEN**

**Files:**
- Modify `lib/_internal/hub.py` (add `_offer_init`, wire `Mode.OFFER_INIT` in `main()`)
- Modify `tests/test_hub_tui.py` (add `test_offer_init_decline`)

**`hub.py` additions (design §1.6):**
- `_offer_init(target: Path) -> bool` — print a short message; `questionary.confirm("Run the init wizard now?").ask()`; on yes, `DelegateRunner(cli, target).run(Action.INIT)` (invokes `ai-specs init <target>` → the existing TUI wizard under a TTY). Returns `True` iff child returned 0 **and** `util.is_initialized(target)` is now true. On decline/False confirm → returns `False` (hub exits 0, no manifest written — non-goal "NOT a write surface").
- Wire `main()`: `Mode.OFFER_INIT` → `ensure_deps` → `_offer_init` → if `False` `return 0`; if `True` fall through to `_run_interactive_hub`.

**Tests (`test_hub_tui.py`, PTY, dep-gated — design §7.3):**
- `test_offer_init_decline`: uninitialized temp dir + PTY; spawn `hub.py <target>`; feed `n` at the confirm → `rc == 0`, no `ai-specs/ai-specs.toml` written.

**Acceptance criteria:**
- `./tests/run.sh` green (dep-gated test passes when deps present).
- Declining init never writes a manifest (non-goal honored).
- Accepting init delegates to `ai-specs init` (not reimplemented — composition only).

**Estimated lines:** ~40 (`hub.py` ~25; PTY test ~15)

---

### Task P3.5 — PTY E2E suite (`test_hub_tui.py`)

**RED → GREEN**

**Files:**
- Create `tests/test_hub_tui.py` (collects the P3.1/P3.3/P3.4 PTY tests + this task's coverage)

**Tests (design §7.3, PTY + `_has_deps()`-gated — mirror `test_init_tui.TestInitTuiPTYE2E._spawn_pty`: `os.openpty` + `Popen` with slave fds + `select.select` + byte feed + deadline):**
- `test_version_inline_then_quit` — initialized temp project; spawn `hub.py <target>`; feed selection for **Version** → assert `VERSION` file content appears in stream; then select **Quit** → `rc == 0`.
- `test_quit_immediately` — select **Quit** (or Ctrl-C at menu) → `rc == 0`, no traceback.
- `test_doctor_delegates_and_resumes` — select **Doctor** → assert canonical doctor output (`ai-specs doctor` / `Summary:`) appears in-stream (proves delegation with inherited stdio), `Press Enter to return…` present, then Enter + **Quit** → `rc == 0`. Exercises `DelegateRunner` end-to-end.
- `test_offer_init_decline` (from P3.4).

**Acceptance criteria:**
- All four PTY tests pass under `_has_deps()`; suite skips cleanly when deps absent (CI-safe).
- `test_doctor_delegates_and_resumes` proves the child's live output reaches the terminal (delegation contract from proposal success criterion #2).
- No hang: every PTY test has an explicit deadline (mirror `test_init_tui`).

**Estimated lines:** ~110 (the PTY harness + 4 tests)

---

## P4 — Docs + polish

**Goal:** documentation reflects the new front door, and the full verification suite (unit + validate) is green, covering every proposal success criterion.

### Task P4.1 — README update

**Files:**
- Modify `README.md`

**Content:**
- New section documenting bare `ai-specs` behavior: the 4-state matrix (init+TTY → hub; init+no-TTY → summary+exit0; not-init+TTY → offer init; not-init+no-TTY → error exit 2), the menu actions, inline Version, delegated subcommands, the `--help`/`help` distinction, and the `exit 3` missing-deps guidance.
- Cross-reference `ai-specs hub [path]` as an explicit invocation.
- Note that non-interactive status needs **no** deps (CI-safe).

**Acceptance criteria:**
- README documents every success-criterion behavior from the proposal.
- No claim that bare `ai-specs` prints help (it routes to hub now).

**Estimated lines:** ~60

---

### Task P4.2 — CHANGELOG + full verification

**Files:**
- Modify `CHANGELOG.md` (if present; else create)
- (No code changes — verification only)

**Work:**
- Add a `tui-hub` entry summarizing the new front door + the 4-phase rollout.
- Run `./tests/run.sh` → all unit + PTY (dep-gated) tests green.
- Run `./tests/validate.sh` → py_compile + `bash -n` clean (covers `lib/hub.sh`).
- Manually verify every proposal success criterion:
  1. init+TTY opens hub w/ doctor status + working menu; Quit exit 0 → `test_quit_immediately`, `test_version_inline_then_quit`
  2. Version inline; Sync/Upgrade/Doctor delegate streamed, return to menu → `test_doctor_delegates_and_resumes`, `TestDelegateRunner`
  3. not-init+TTY offers init → opens hub on success → `test_offer_init_decline` (decline path) + manual accept path
  4. not-init+no-TTY error exit 2 → `TestNonInteractiveStatus`
  5. init+no-TTY non-interactive summary exit 0 → `TestNonInteractiveStatus`
  6. missing deps → exit 3 w/ guidance → `TestVendorFallback` (optional) / `TestEnsureDeps`
  7. `ai-specs help` unchanged → `TestNonInteractiveStatus` help regression
  8. unit tests (4-state matrix) + PTY E2E (`_has_deps()`-gated) pass → full suite
  9. README documents new behavior → P4.1

**Acceptance criteria:**
- `./tests/run.sh` exit 0.
- `./tests/validate.sh` exit 0.
- Every success criterion from `proposal.md:72-83` mapped to a passing test or manual check.

**Estimated lines:** ~20 (CHANGELOG)

---

## Summary

| Phase | New files | Modified files | Est. lines | Tests |
|-------|-----------|----------------|-------------|-------|
| P1 | `util.py`, `test_util.py`, `_vendor/`, `scripts/vendor-deps.sh` | `init_tui.py` | ~155 | test_util + existing test_init_tui guards |
| P2 | `hub.py`, `hub.sh`, `test_hub.py` | `bin/ai-specs` | ~275 | gating, status, non-interactive, routing, help regression |
| P3 | `test_hub_tui.py` | `hub.py` | ~265 | menu, delegate, panel, offer-init, PTY E2E |
| P4 | — | `README.md`, `CHANGELOG.md` | ~80 | full run.sh + validate.sh |
| **Total** | **7** | **4** | **~775** (excl. vendored tree) | |

**Verification commands:** `./tests/run.sh` (TDD RED/GREEN), `./tests/validate.sh` (py_compile + bash -n).