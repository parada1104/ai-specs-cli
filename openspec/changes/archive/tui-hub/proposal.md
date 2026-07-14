# Proposal: `ai-specs` TUI hub (no-subcommand entrypoint)

## Why (motivation)

Bare `ai-specs` currently prints static help (`bin/ai-specs` line 29 `cmd="${1:-help}"`) — a dead end. No discovery of commands, no project situational awareness, and no nudge to init on an uninitialized dir. The tool already ships `rich`+`questionary` (init_tui.py) and an importable health engine (doctor.py `Doctor` dataclass). A hub reuses both as a git-like front door.

## Intent

Make bare `ai-specs` an auto-initializing TUI hub (Option C, approved): detect project; if uninitialized+TTY offer init wizard then open hub; if uninitialized+no-TTY error exit 2; if initialized open interactive hub (status + command menu, inline or delegated execution).

## Scope (in)

1. **bin/ai-specs**: route no-args (`${1:-help}` → hub); keep explicit help; unknown still exit 2.
2. **Init/TTY decision matrix**:
   - init + TTY → interactive hub (Quit exit 0)
   - init + no TTY → non-interactive status+command list, exit 0
   - not-init + TTY → message + offer init wizard; on success open hub
   - not-init + no TTY → error exit 2
3. **lib/_internal/hub.py**: standalone `main()->int` (_internal convention); status via `Doctor(root).run()` + `.checks`; questionary menu (Sync, Doctor, Skills, Recipes, Rules Audit, Upgrade, Version, Help, Init wizard, Quit); quick inline (Version), complex delegated with inherited stdio (suspend→run→resume).
4. **lib/hub.sh**: shim mirroring doctor.sh; resolve home/target; bash-level init/TTY gating; exec python3 hub.py.
5. **Shared `is_initialized` helper** (currently ad-hoc `-f ai-specs/ai-specs.toml`).
6. **Reuse/extract `_ensure_deps()`** (exit 3, installs into lib/_vendor). rich vendored; questionary NOT yet vendored (on-demand today).
7. **Tests** (strict TDD ./tests/run.sh): unit for gating matrix + status; PTY E2E for menu (os.openpty + select.select, `_has_deps()` gate) mirroring test_init_tui.py.
8. **README update**.

## Non-goals

- NOT Textual (heavy dep rejected — keep rich+questionary)
- NOT re-implementing subcommands (compose only)
- NOT a write surface (mutations only inside delegated subcommands)
- NOT changing existing subcommand behavior/flags/output
- NOT removing `ai-specs help`
- NOT settings editor/log viewer/fleet dashboard
- NOT migrating all ad-hoc init checks (opportunistic reuse only)

## Design questions resolved

1. **Textual vs rich+questionary** → rich+questionary (established, rich vendored, questionary on-demand; Textual too heavy).
2. **Standalone vs embedded** → standalone _internal CLI `main()->int` like init_tui.py/doctor.py, wrapped by lib/hub.sh.
3. **Reuse doctor.py vs subcommand** → BOTH by role: status panel imports `Doctor` in-process; menu "Doctor" action delegates to `ai-specs doctor` for canonical output.
4. **Streaming vs immediate** → Version inline; Sync/Upgrade/Skills/Recipes/Rules Audit/full Doctor suspend TUI, run existing shim with inherited stdio (streams live), resume menu.

## Impact (files)

- `bin/ai-specs` (route)
- `lib/hub.sh` (new)
- `lib/_internal/hub.py` (new)
- `lib/_internal/init_tui.py` (extract deps-gate)
- `lib/_internal/doctor.py` (read-only reuse)
- `lib/init.sh` (reuse wizard)
- shared `is_initialized` helper (new)
- `lib/_vendor/` (questionary)
- `README.md`
- `tests/test_hub.py` (new)
- `tests/test_hub_tui.py` or extend test_init_tui.py (new PTY E2E)

## Risks

- Bare `ai-specs` in CI/pipes could hang → no-TTY paths never block (status+exit0 / error+exit2); never prompt without TTY.
- questionary not vendored → offline pip fails → reuse exit-3 guidance; consider pre-vendoring.
- Inherited-stdio delegation corrupts TUI → suspend/resume; subcommand owns terminal until return.
- Users relied on bare=help → help unchanged; Help is a menu item + non-interactive fallback prints help.
- Deps-gate extraction regresses wizard → shared module identical; test_init_tui.py guards.

## Rollback

1. Revert bin/ai-specs line 29 to `${1:-help}` (instant, subcommands unaffected).
2. hub.sh + hub.py additive/unreferenced once route reverted → delete.
3. Inline `_ensure_deps()` back if extraction undesirable (byte-identical).
4. Doctor used read-only; no rollback. questionary vendoring additive.

## Success criteria

- init+TTY opens hub w/ doctor status + working menu; Quit exit 0.
- Version inline; Sync/Upgrade/Doctor delegate streamed, return to menu.
- not-init+TTY offers init → opens hub on success.
- not-init+no-TTY error exit 2.
- init+no-TTY non-interactive summary exit 0.
- missing deps → exit 3 w/ guidance.
- `ai-specs help` unchanged.
- unit tests (4-state matrix) + PTY E2E (`_has_deps()` gated); ./tests/run.sh + ./tests/validate.sh pass.
- README documents new behavior.

## Phase breakdown

1. **P1 Infrastructure**: extract deps-gate; add is_initialized helper; questionary vendoring (RED tests first).
2. **P2 Hub core (non-interactive first)**: hub.py status via Doctor; pure gating decision (unit-testable); hub.sh; route bare ai-specs. Unit tests.
3. **P3 Interactive menu + delegation**: questionary menu; inline vs delegated; suspend/resume around streaming. PTY E2E.
4. **P4 Docs + polish**: README; validate.sh; verify every success criterion.

## Classification

domain_change per config.yaml → proposal→design→tasks, worktree required.
