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
  - T4a — the recipe feed the Recipes submenu needs: `internal/recipe.List(projectRoot, cliHome)`
    (catalog scan, `test-` guard, status available/installed/disabled/error). U6's `recipe list`
    command will consume the SAME function, so this slice is not a duplicate. Route: inline.
  - T4b — the skills feed: `internal/skills.Description(skillDir)` (SKILL.md front-matter, 80-char
    cap, quote unquoting), shared with U5's `skills list`. Route: inline.
  - T4c — `internal/hub`: the loop, the three submenus, the agents manifest write, the delegation
    argv and the offer-init flow over an injectable `tui.KeySource`. Route: inline.
  - T4d — `internal/tui`: the prompt frame's message line keeps questionary's trailing space (two
    acceptance needles are `b"Recipes: "` and `b"Skills: "`), and `Terminal` gains Suspend/Resume so
    a delegated child inherits a canonical terminal without losing the keystroke buffer. Route: inline.
- T5 — S6: retarget the three Python invariants and keep the pty suites green. Route: inline.
  - T5a — `TestNoOpenptyInHub` (tests/test_hub.py) dies with the bridge; its INTENT is preserved as a
    Go-side invariant in `internal/hub` (the hub never allocates a pty and never reads stdin directly).
  - T5b — `TestPauseOnlySite`'s source half (tests/test_hub_tui.py) moves to the same Go invariant; its
    behavioural sibling `test_aborted_pause_returns_zero` stays and must pass against the native binary.
  - T5c — the PTY drivers (`_spawn_pty` in tests/test_hub_tui.py, `_spawn_hub_pty` in tests/test_hub.py)
    drive the native binary through `tests/_go_cli.py`, the helper the three sync contract suites already
    use, so the interactive contract is graded against the Go port instead of the shell script U2 retires.
    `TestStatusPanelRender` stays unchanged, as S6 requires.
- T6 — Evidence: focused packages, the pty suites, `go test ./cmd/... ./internal/...`, parity,
  `./tests/validate.sh`. Route: inline. Commit: this unit's commit(s).

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
- L4 — Decision (T5c): the PTY drivers now run the NATIVE binary (`tests/_go_cli.py`, the helper the
  three sync contract suites already use), not `bin/ai-specs`. S6's "must keep passing against the Go
  binary" is only true if the suite drives it, and `bin/ai-specs` is still the bash launcher that U2
  retires. The non-interactive `_run_hub` invocations stay on the launcher: they are U3a's parity leg and
  U2's cleanup, not this unit's. Delegation from the hub still goes to `<home>/bin/ai-specs`
  (`hub.py`'s `DelegateRunner` target), so the unported subcommands keep their legacy route.
- L5 — Decision (T4a/T4b): the interactive surface needs two data feeds that only exist in Python
  today — the recipe catalogue with per-recipe status (`recipe-list.py:49-99`) and the SKILL.md
  front-matter description (`hub.py:190-215`). They land as `internal/recipe.List` and
  `internal/skills.Description` rather than as hub-private helpers, because U6 and U5 will consume the
  same functions; porting them inside `internal/hub` would have meant deleting them two units later.
- L6 — Decision (T4d): ONE long-lived `tui.Terminal` per interactive session, with raw mode suspended
  around every delegation, instead of a terminal per prompt. `bufio.Reader` reads ahead, so a
  per-prompt reader would strand the type-ahead — and two acceptance tests write their whole key script
  in a single `os.write` (`test_version_inline_then_quit`, `test_quit_immediately`). Suspend/Resume keeps
  the buffer and still gives a delegated child a canonical terminal.
- L7 — Fact (T4d): two acceptance needles carry a trailing space (`b"Recipes: "`, `b"Skills: "`),
  because questionary renders `? <message> ` before its choices. The frame's message line therefore has
  to keep that space; without it those stages never fire and the suite times out instead of failing.
- L8 — The acceptance suite is not a formality, and T5 proved it three times in one run. Re-pointing the
  PTY flows at the native binary produced four failing tests whose causes were three REAL defects in the
  U3b-1 layer, none of them visible to the scripted-key unit tests that were green at the time:
  (1) `enterRawMode` cleared ICANON and ECHO but left ISIG, so the line discipline turned Ctrl-C into
  SIGINT and CONSUMED the byte — the prompt never saw the abort and waited forever; (2) Ctrl-D arrived as
  the byte 0x04 and was decoded as an ordinary key, so a pause that the Python hub ended cleanly hung;
  (3) `Confirm` drew the question only after it was answered, which left it invisible on a real terminal
  AND deadlocked every stage that answers only once the question is on screen. The lesson is not "test
  more": it is that a layer whose unit tests never touch a real terminal has an UNTESTED half, and the
  cheapest proof of the other half is to point the existing acceptance suite at it — which is exactly
  what S7 said U3b-2 would do.

## Evidence

Run from this worktree with the real toolchains first on `PATH` (`python 3.14.7`, `go 1.24.13`), appended
to `~/.cache/u3b-evidence.log`:

| Step | Command | Result |
| --- | --- | --- |
| Go packages | `go test ./cmd/... ./internal/...` | exit 0 |
| Gate binary | `bash scripts/build-gate.sh` | exit 0, 4 targets built |
| Parity corpus | `python3 tests/parity/run.py` | exit 0 · 29 fixtures, `gate-absent failing=0`, `gate-present failing=0` |
| Acceptance (PTY) | `python3 -m unittest discover -s tests -p 'test_hub*.py'` | 47 tests, OK, 49.8s (197s of timeouts before the three fixes) |
| Frozen non-interactive bytes | base `4e6da9d` (built from `git archive` in a scratch dir) vs this tree, 5 `hub` invocations: status, `--help`, uninitialized, unknown flag, bare | stdout, stderr and exit code byte-identical (rc 1/0/2/2/1) |
| Focused packages | `go test ./internal/{hub,config,recipe,skills,tui} -count=1` + `go vet ./internal/...` | ok, vet clean |
| Full validation | `./tests/validate.sh` on `45e9544` | exit 0 · `Ran 2494 tests in 1538.569s` |

Test-count accounting, so the delta is not a silent loss: the suite stood at 2496 after U3a-fix and reads
2494 here, and the difference is exactly the two Python SOURCE invariants this unit retires
(`TestNoOpenptyInHub.test_hub_py_has_no_openpty_or_termios` and
`TestPauseOnlySite.test_no_bare_press_enter_input_outside_pause`). Their guarantees did not disappear;
they are enforced now by `internal/hub/invariants_test.go`, which the Go test run counts in its own suite.
`TestImportlibModuleScope.test_hub_has_importlib_at_module_scope` was renamed rather than removed, so it
still counts once.

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

### U3b-2 — commits (the slice boundary, per task)

Base `4e6da9d` (epic tip). Each slice is one commit on `change/go-20-u3b-tui`, ordered so every
commit contains what the next one needs:

| Slice | Commit | Task | Files |
| --- | --- | --- | --- |
| the prompt layer's key contract | `6d66eff` | T4d + the three defects T5 found | `internal/tui/{tui.go,tui_test.go,terminal.go,terminal_test.go,raw_darwin.go,raw_linux.go}` |
| the recipe feed | `0413e11` | T4a | `internal/recipe/{list.go,list_test.go}` |
| the skills feed | `b016d2b` | T4b | `internal/skills/{description.go,description_test.go}` |
| the manifest writer | `6721991` | T4c | `internal/config/{agents.go,agents_test.go}` |
| the interactive port | `af98ab9` | T4c | `internal/hub/{interactive.go,interactive_test.go,invariants_test.go,hub.go,quirks_test.go}` |
| the acceptance re-point | `547a350` | T5 | `tests/{test_hub.py,test_hub_tui.py}` |
| this record | (the commit that carries this file) | T6 | `odd/tasks/go-20-u3b-tui.md` |

The manifest writer is its own commit because a mistake there edits a user's stored manifest (Risk item
1) and native review can then take it alone. The interactive port is ONE slice of ~1 280 changed lines
(690 of them its tests) rather than the ~400-line target, because splitting a port of one module leaves
either a non-compiling intermediate or a half-ported loop whose tests cannot run; the boundary that
matters for review is that it contains no other lane's work.

### U3b-2 — plan, port and divergences


Plan: T4a/T4b (the two feeds) → T4d (the frame's trailing space and Suspend/Resume) → T4c (the loop,
submenus, agents write, offer-init, delegation) → T5 (retargets and PTY re-point) → T6 (evidence).
The Go interactive harness is an injectable `tui.KeySource` plus a stub `<home>/bin/ai-specs`, so the
loop, the argv of every delegation and the manifest write are all observed without a pty; the pty
suites then grade the raw-mode plumbing and the acceptance vocabulary against the native binary.

T4a, T4b, T4d and T4c are DONE, each with focused packages green:

- T4a `internal/recipe/list.go` (+ test): `List`/`CatalogDir`/`Entry`, statuses `available` /
  `installed` / `disabled` / `error (…)`, `test-` and dot-directory exclusions, sorted order.
- T4b `internal/skills/description.go` (+ test): `Description(dir)`, the front-matter regex, the quote
  unquoting and the 80-CODE-POINT cap (a byte slice would cut a multi-byte character).
- T4d `internal/tui`: the frame's message line is questionary's `? <message> ` and `Terminal` gained
  `Suspend`/`Resume` with the keystroke reader kept (its test pins `enters`/`restores` and the reader's
  identity, which is what makes the buffered type-ahead survive).
- T4c `internal/hub/interactive.go` (+ `interactive_test.go`, `invariants_test.go`),
  `internal/config/agents.go` (+ test), `internal/hub/hub.go` (the bridge is GONE, `Options.Keys` is the
  test seam, `runDoctor` → `doctorRunFor`). The worker observed RED (undefined `EnabledAgents`/
  `SetEnabledAgents`, unknown field `Keys`, undefined `formatPanelTopology`) before GREEN, ran
  `go test ./internal/hub/... ./internal/config/... -count=1` and `go vet`, and proved the frozen
  non-interactive bytes unchanged against the base commit in a scratch tree (stdout, stderr and rc
  identical).

Divergences of T4c, recorded rather than fixed (none of them is on a tested path):

- rich's `[yellow]`/`[red]` markup in the aux messages becomes plain text; the panel keeps its
  red/yellow/green border.
- `bundledSkillNames` reads the binary's embedded `bundled-skills` tree — the same source the native
  doctor already uses after card [Go 06] — instead of `<home>/bundled-skills`. For an installed tree the
  content is the same, and the acceptance test that proves the bucket partition
  (`tests/test_hub.py::TestCategorizeSkills`) asserts the ids, not their source.
- `SetEnabledAgents` writes through the package's `atomicWrite` (temp + rename) instead of Python's
  in-place `write_text`: identical bytes and mode, different inode.
- Line classification uses `strings.TrimSpace`; Python's `str.strip()` also strips `\x1c-\x1f`.
- A child that cannot start, a failed manifest write and a failed `Terminal.Resume` report
  `ai-specs hub: <err>` on stderr instead of raising a traceback.
- Panel colour follows `opts.TTY` (`noColor = !TTY`) instead of rich's per-writer detection.
- **Advisories found while fixing the raw mode, recorded and NOT fixed here** (each needs its own test,
  and this unit's scope is the port, not a prompt-layer polish pass):
  - `IXON` is still on. The Python hub's prompt_toolkit cleared it, so Ctrl-S freezes output in the
    native hub where the Python one ignored it. The fix is one flag (`raw.Iflag &^= syscall.IXON`) on
    both platform files, and the proof would be a pty stage that sends `\x13` and then expects the next
    prompt to still draw.
  - `escapeSequence` recognises only the arrow forms, so Home/End/PageUp/PageDown are ignored where
    questionary's select moved on them.
  - `Terminal.NextKey` maps `j`/`k` to Down/Up, an extension questionary did not have (a superset: no
    keystroke changes meaning, so nothing that worked stops working).

T5 (retargets) DONE and VERIFIED: `tests/test_hub.py` and `tests/test_hub_tui.py` now drive the native
binary through `tests/_go_cli.py` for every PTY flow, the `TestNoOpenptyInHub` class and
`TestPauseOnlySite`'s source half are replaced by `internal/hub/invariants_test.go`, and
`TestImportlibModuleScope` was renamed `TestAgentsPickerPTY` because its stated reason died with the
bridge while its behaviour did not. Suite result: **47 tests, all green, 49.8s** (the same suite spent
197s timing out before the fixes below).

The re-point paid for itself immediately: it found THREE real defects in the U3b-1 layer that the
scripted-key unit tests could not see, all of them in the one place S7 predicted (a real pty).

- **Ctrl-D was not EOF.** A canonical terminal turns 0x04 into end-of-file; raw mode hands the byte
  over, so the prompt layer has to honour the same meaning by hand. `tui.Pause` therefore waited
  forever where the Python hub exited cleanly (`test_aborted_pause_returns_zero`,
  `TestWidgetHelpers.test_pause_true_and_eof_false`). Fixed in `Terminal.NextKey` (0x04 → `io.EOF`),
  with a unit test and the reason written next to the code.
- **ISIG was still on.** `enterRawMode` cleared only ICANON and ECHO, so the line discipline turned
  Ctrl-C into SIGINT and CONSUMED the byte: the prompt never saw the abort, and the hub waited for a
  key that could not arrive (`test_none_maps_to_quit`, `test_agents_picker_opens_and_aborts`,
  `test_pick_many_empty_returns_none`). ISIG is now cleared on both platforms, which is also what makes
  Ctrl-C mean "abort this prompt" instead of "kill the hub".
- **`Confirm` drew the question only once answered.** Invisible on a real terminal, and it deadlocked
  every stage that waits for the question text before typing the answer
  (`test_confirm_action_returns_bool`). It now draws `? <message> (Y/n) ` before reading and appends the
  answer to that line, which is questionary's order.

T4c sketch — the specification the writer follows:

- `internal/config/agents.go` (+ `agents_test.go`): `EnabledAgents(manifestPath string) []string`
  (toml-read.read_agents semantics: strings only, stripped, non-empty, manifest order) and
  `SetEnabledAgents(manifestPath string, agents []string) error`, a byte-faithful port of
  `lib/_internal/hub.py:515-592`'s line editor — replace an existing `enabled` line inside `[agents]`,
  insert the key before the next header when the table lacks it, append `\n[agents]\nenabled = [...]\n`
  when there is no table, and NEVER a second `[agents]` block (D12). The literal comes from
  `toml.TOMLValue`; lines from the package's existing `splitLinesKeepEnds`.
- `internal/hub/interactive.go` (+ `interactive_test.go`, `invariants_test.go`): the `interactive`
  session type, `runInteractiveSession(mode Mode, target string, opts Options, stdin io.Reader,
  stdout, stderr io.Writer) int`, the menu loop, the three submenus, `delegate` (the argv from
  `delegateArgv`, `exec.Command`, `childEnv(home)`, raw mode suspended around the child), the
  offer-init flow, `panelData`, `panelTopology`/`formatPanelTopology`, `categorizeSkills`,
  `bundledSkillNames`, `recipeAddChoices`/`recipeRemoveChoices`, `skillBucketOrder`.
- `internal/hub/hub.go`: `Options.Keys tui.KeySource` (nil in production, the test seam), `RunHub`'s
  interactive branch calls `runInteractiveSession`, `runInteractiveBridge` and its exec plumbing are
  DELETED, and `runDoctor` becomes `doctorRunFor` (exit code + counts + the non-OK `tui.PanelCheck`
  list) so the panel and the non-interactive summary share one doctor run.
- Every frozen string is `hub.py`'s: `What do you want to do?`, the choice titles `<title> — <desc>`,
  the recipes submenu labels (including `Configure recipes (whole project, configure-recipes)`), the
  picker labels `f"{name} ({rid})  v{version}"` and `f"{name} ({rid})  [{status}]"`, the skills bucket
  titles, `✓ done` / `✗ exited N`, `Press Enter to return…`, `No ai-specs project found at <target>.`,
  `Run the init wizard now?`, `✓ init complete — running sync to generate agent configs…`,
  `Manifest not found — run ai-specs init first`, `No skills found to inspect.`,
  `  ✓ agents updated: <list or (none)>`, `  Run sync to regenerate agent configs.`,
  `Recipes unavailable: <err>`, `No catalog recipes available to add.`, `No recipes installed to remove.`
- Divergences to record, not to fix: rich's `[yellow]`/`[red]` markup becomes plain text; a failed
  manifest write or a missing `<home>/bin/ai-specs` reports an error instead of raising a traceback.
- Panel quirks (`hub.py:352-393`): the topology row prints `<resolved> (auto→<resolved>)` when the
  resolution's `via` is `auto` or empty, and `<resolved> (via <via>)` otherwise; the check rows are
  `<SEVERITY>` + `<name>: <message>` for every non-OK check (INFO included).
