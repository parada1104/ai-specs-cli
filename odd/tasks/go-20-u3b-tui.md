# Go 20 / U3b — The interactive hub, native and dependency-free

Unit of the program `odd/tasks/go-20-python-exit.md`. Base `4e6da9d` (U3a-fix, PR #367). Worktree
`.worktrees/go-20-u3b-tui`, branch `change/go-20-u3b-tui`.

## Specs

- S1 — Decision (human, verbatim, 2026-10-16): "Prompt layer propio, cero dependencias (recomendada)".
  It REVERSES the earlier decision to vendor `pterm`, and the reason is measured rather than aesthetic:
  `pterm` v0.12.82 — the newest version compatible with the canonical go1.24.13 toolchain, since
  v0.12.83 and newer require Go 1.26 — vendors 15 modules, 549 Go files, 334 049 lines and 15 MB, and
  introduces a 105-line `go.sum`. D8 ("zero third-party dependencies") would stop being true and six
  records would need amending. This unit reproduces the interactive surface in our own Go on top of
  `internal/tty` (the kernel ioctl from U3a-fix), so NO record is amended and D8 stays literally true.
- S2 — The surface to reproduce, from read-only reconnaissance with `path:line` evidence:
  - the main menu (`lib/_internal/hub.py:325-327`): `questionary.select("What do you want to do?")` over
    the 12 `_MENU` entries, each label `f"{title} — {desc}"` and each value an `Action`; an aborted
    prompt is `Action.QUIT` (`:328`);
  - `pick_one` (`:276-285`), `pick_many` (`:288-298`), `confirm_action` (`:301-305`, default true) and
    `pause` (`:308-313`, `input("Press Enter to return…")`);
  - the loop (`:595-631`): render the status panel, prompt the menu, run the action, pause, repeat;
  - the submenus — skills in-process with its categorised buckets (`:439-459`), recipes DELEGATING
    `[cli, "recipe", "list"|"add"|"remove", [rid], target]` and `[cli, "configure-recipes", target]`
    (`:125-148`, `:502`), agents writing `[agents] enabled = [...]` in-process (`:515-592`), and the
    init offer (`:633-651`) which delegates INIT and then SYNC;
  - the status panel (`:352-393`): a grid table whose rows are `version`, `target`, an optional
    `topology`, the `Summary:` row, then one row per non-OK check, inside a `Panel` titled `ai-specs`
    with a red/yellow/green border.
- S3 — Zero dependencies, literally: no `vendor/`, no `go.sum`, `go.mod` untouched, and the six
  amendment sites listed in the program's L19 stay exactly as they are.
- S4 — Prompt semantics that must survive the port: an aborted select/checkbox/confirm returns the same
  "nothing chosen" value hub.py's callers branch on (`None`); the checkbox starts from the current
  state; `confirm` defaults to true; Ctrl-C and ESC abort rather than crash; and EOF on stdin (a closed
  pty) ends the loop cleanly instead of raising.
- S5 — Slicing: U3b-1 is `internal/tui` (select, multiselect, confirm, pause, panel) with its own tests
  and the hub still bridging, so the layer is reviewable in isolation; U3b-2 wires the interactive hub
  to it and retargets the three Python tests.
- S6 — The three Python-source tests, with the verdicts the reconnaissance reached:
  - `TestNoOpenptyInHub` (`tests/test_hub.py:500-507`) asserts the hub's Python SOURCE never mentions
    `openpty`/`termios`. It dies with the bridge; its INTENT (the hub never allocates a pty) is preserved
    as an equivalent Go-side invariant, so the guarantee keeps a test instead of dying with its language.
  - `TestPauseOnlySite` (`tests/test_hub_tui.py:388-422`) is a source-text invariant on the Python
    `pause()`. Its sibling `test_aborted_pause_returns_zero` (`:408`) is REAL behaviour over a real pty
    (Doctor → Ctrl-D → clean exit, no traceback) and must keep passing against the Go binary.
  - `TestStatusPanelRender` (`tests/test_hub_tui.py:274-297`) drives the hub over a real pty and asserts
    the panel's plain-text vocabulary (`ai-specs`, `Summary:`, `version`, the VERSION content). It is the
    acceptance test for the Go panel and stays unchanged.
- S7 — The PTY suites keep their acceptance role: `tests/test_hub_tui.py` drives `bin/ai-specs` through a
  real pty, so when the Go binary serves the interactive path those tests are the product contract. They
  must pass unchanged wherever they assert behaviour.

## Tasks

- T1 — S4: RED — the layer's tests: a selection moves and confirms, a checkbox toggles and confirms, an
  abort returns the empty choice, EOF ends the loop cleanly, and the panel's bytes. Route: inline.
- T2 — S4: the `internal/tui` prompt layer over `internal/tty`. Route: inline.
- T3 — S2: the status panel renderer. Route: inline.
- T4 — S2/S5: `internal/hub`'s interactive branch served natively (menu, submenus, agents write,
  delegation argv, pause, loop), removing the bridge. Route: inline.
- T5 — S6: retarget the three Python invariants and keep the pty suites green. Route: inline.
- T6 — Evidence: focused packages, the pty suites, `go test ./cmd/... ./internal/...`, parity,
  `./tests/validate.sh`. Route: inline.

## Log

- L1 — Session request (verbatim, from the program's L1): "quiero que este como si se hubiera hecho con
  go desde 0".
- L2 — The dependency episode, in order, because the order is the lesson: the human first chose `pterm`
  with vendoring; the vendoring measurement then returned 15 modules / 549 files / 334 049 lines / 15 MB
  and a `go.sum`; that number contradicted the "~6 modules" estimate the choice was made on, so the choice
  was re-presented with the real closure and the human reversed it to the native prompt layer. Recorded as
  L19 of the program, with the rule that follows: measure the closure BEFORE asking.
- L3 — Read-only reconnaissance (delegated) produced the surface in S2 and the verdicts in S6 with
  `path:line` evidence, including the fact that `hub.py` never imports `pty` or `termios` — which is what
  makes S6's first retarget a preservation of intent rather than a deletion.

## Progress

### U3b-1 — the layer (`internal/tui`), committed on its own
- T1 (RED): `internal/tui/tui_test.go` written first and observed failing — `undefined: Key`, `Choice`,
  `Select`, `KeyDown`, `KeyEnter` and the rest of the package.
- T2/T3 (GREEN): `internal/tui/tui.go` (`Select`, `MultiSelect`, `Confirm`, `Pause` over the injectable
  `KeySource`), `internal/tui/panel.go` (`PanelData`, `Counts`, `PanelCheck`, `RenderPanel`, `Headline`)
  and `internal/tui/terminal.go` with `raw_darwin.go`/`raw_linux.go`/`raw_fallback.go`.
- **Two of the RED cases were wrong, and the CODE was right**: the multiselect case forgot that its
  second `Down` lands on an entry that was already checked and untoggles it, and the confirm case fed an
  unknown key expecting it to answer. Both tests were corrected rather than the behaviour. The corrected
  multiselect case now proves more than the original: it starts from a caller-supplied checked state,
  toggles two non-adjacent entries, and asserts the result follows the CHOICE order, not the order in
  which they were toggled.
- A contract decision worth recording: `y`/`n` set the answer and ENTER submits it. If a single `y`/`n`
  closed the prompt, the newline that followed it would stay in the stream and the hub's loop would read
  it as an Enter on the NEXT prompt, silently confirming that prompt's default.
- Evidence: `go test ./internal/tui -count=1` → ok (10 tests), `go vet ./internal/tui` clean. The raw-mode
  plumbing is deliberately NOT unit-tested with a synthetic terminal; it is covered by the PTY suites in
  U3b-2, which drive the built binary through a real pty.

### U3b-2 — wiring and retargeting: pending
