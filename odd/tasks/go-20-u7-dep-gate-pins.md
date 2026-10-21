# Go 20 / U7 item 4 — the three dep-gate pins

Unit U7-4 of `odd/tasks/go-20-python-exit.md`. Branch `change/go-20-u7-dep-gate-pins` off
`epic/go-single-binary` @ `774061c`, worktree `.worktrees/go-20-u7-dep-gate-pins`. It adds **evidence**,
not features: the three advisories U6e recorded against the native dep gate become proved against the
real legacy code, and anything they expose is fixed here or recorded with its reason.

## Specs

- S1 — Program request, verbatim (2026-10-15): "quiero que este como si se hubiera hecho con go desde 0".
  Program rule (S4 of the program doc): zero Python under the product tree; bash only as wrappers.
- S2 — Delivery lane, unchanged: every unit goes to `epic/go-single-binary` through one PR, never to
  `development`; workers never commit or push.
- S3 — **The advisory this unit answers**, verbatim from `odd/tasks/go-20-u6-recipe.md:61`:

  > **RECORDED, owner U7**: the loop order, the dep-guidance bytes and the tail-seam close are pinned
  > in-process, not differentially; a PTY differential is where they belong and the table has none
  > (L4's constraint)

  The three ids are `R3-LoopOrderUnproved`, `R3-AptGuidanceUnproved` and `R3-TailSeamClose`. Their review
  text was never recorded, so **the scope of this unit is that sentence and nothing else**: no case is
  invented for a defect nobody can quote. What each id names is derived from the code it points at and
  quoted in T1–T3.
- S4 — One grader per behaviour, and the differential is the grader: every new case compares the port
  against the REAL legacy code (`lib/_internal/config_wizard.py:_dep_gate` and
  `dep_install.offer_and_install`, driven through `internal/deps/testdata/gate_driver.py`), never against
  a second copy of the port's own expectations. A case that only re-asserts an in-process test adds no
  authority and does not close an advisory.
- S5 — The frozen bytes, quoted from the legacy so no case can drift from them.
  `lib/_internal/dep_install.py`: `:183` `  → {plan.binary}: {plan.display}` (guidance arm, stderr);
  `:193` `describe_install_plan(plan)` (github-release describes itself before asking); `:207`
  `  ! install {plan.binary} failed: {exc}`; `:209` `    see {plan.guidance_url}`; `:213`
  `  ✓ {plan.binary} installed ({result.version})`; `:219` the same `failed:` line for the
  `subprocess.run` arm **with no guidance line**; `:223` `  ! install {plan.binary} exited
  {proc.returncode}` **followed by** `:227` `    see {plan.guidance_url}`; `:232`
  `  ✓ {plan.binary} installed`; `:234` `  ! {plan.binary} still not on PATH after install` followed by
  `:239` `    see {plan.guidance_url}`. And `config_wizard.py:192-195`:
  `{len(missing_required)} required CLI tool(s) missing. Configure anyway?`, `default=False`.
- S6 — The loop order is `missing_required`'s and nobody's else: `config_wizard.py:165`
  `[r for r in results if r.required and not r.ok]` — the probe's result order, filtered — and the per-plan
  arms run in that single pass (`dep_install.py:182` `for plan in plans:`). No sort, no grouping by kind,
  no two passes. A proof of the order therefore needs one batch whose plans have **different kinds**.
- S7 — The tail seam belongs to the adapter, not the gate: `internal/cli/depgate.go:recipeDepGate` opens
  ONE raw-mode window for the gate's questions and closes it on the way out (`defer terminal.Close()`),
  while `initDepGate` hands the wizard's own window to it and never closes it. "The terminal is handed
  back" is only observable through a real terminal, so it is a PTY case in `tests/test_init_tui.py`, the
  file that already drives `init --tui` against the built binary.
- S8 — No observable behaviour changes without evidence: RED observed before each fix, and the gates of
  S9 apply. A case that exposes a real divergence is a finding: fix it here with its RED, or record it
  with its reason — never match the port to the legacy silently in a fixture.
- S9 — Gates: `go build ./...`, `gofmt -l`, `go vet`, the focused packages (`go test ./internal/deps/...
  ./internal/cli/...` and the touched Python legs under `PATH=/tmp/pybin:$PATH`), then the parent runs
  `./tests/validate.sh` once. A worker runs only the focused set.

## Tasks

- **T1** — `R3-LoopOrderUnproved`: differential cases whose batch mixes plan kinds (guidance arrow,
  package question, github-release describe + question) in one `missing_required` list, proving the
  emitted order and each arm. S3, S4, S6. Commit `test(deps): pin the gate's mixed-kind loop order
  differentially`.
- **T2** — `R3-AptGuidanceUnproved`: the apt failure arms on `linux` — `apt-get install -y jq` exiting 1
  with an `install_url` (the `exited` line **and** the `    see` line) and the `subprocess.run` error arm
  (the `failed:` line and **no** `    see` line). S5. Commit `test(deps): pin the apt guidance bytes
  differentially`.
- **T3** — `R3-TailSeamClose`: the PTY case proving the window comes back after the gate returns, plus the
  stale U6e comment at `tests/test_init_tui.py:384-386` ("U6e lands the native dep gate and retires the
  seam; until then…"), which describes a seam U7-1 T3 already removed. S7. Commit `test(cli): prove the
  dep gate hands the terminal back`.
- **T4** — Verification and close: focused gates, the parent's full suite, and the `## Next` item 4 of
  `odd/tasks/go-20-python-exit.md` rewritten to the delivered state (its coordinates line also says the
  epic is at `8dd87e6`; the epic is at `774061c`). S9.

## Log

- L1 — Session request, verbatim (2026-10-21): "bien, entonces en que quedamos?" — followed by the choice
  of the offered option, verbatim: "Redefinir U7-4 como diferencial del dep gate". No new scope was asked
  for beyond U7-4's own advisory.
- L2 — **The previous hand-off was wrong about U7-4 being unactionable**, and this unit exists because of
  that correction. It claimed the three advisory ids "only exist as ids (without text)" and that "the
  differential is where they belong" had lost authority because `tests/parity/` has no dep-gate legs.
  Measured before writing: the text survives at `go-20-u6-recipe.md:61`; the differential the advisory
  asked for **was built** by U7-1 T3 (`internal/deps/gate_differential_test.go`, 321 lines, 13 cases,
  driving the real `_dep_gate` through `testdata/gate_driver.py`); and the legacy modules are still in the
  tree (`lib/_internal/dep_check.py`, `dep_install.py`, S9 of the program doc keeps them until U2's flip).
  So the advisory is actionable, and its remaining gap is a gap in that corpus, not a missing differential.
- L3 — The gaps L2 measured in the current corpus, which is what T1–T3 close: the `apt-on-linux` case
  succeeds (`presentJQ` after install), so no apt failure bytes are compared; `package-exit-1`,
  `package-timeout` and `installed-not-on-path` all run `brew` on `darwin`; `two-missing-declined` is two
  package plans of the same kind, so the order across kinds is unproven; and the PTY case asserts the
  confirm rendered (`.assertIn("Configure anyway", plain)`) but not that the terminal is handed back.
