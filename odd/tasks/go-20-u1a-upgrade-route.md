# Go 20 / U1a — Wire the native `upgrade` verb (and record what it deliberately does not do)

Unit of the program `odd/tasks/go-20-python-exit.md` (U1, first slice). Base `ecfb843`. Worktree
`.worktrees/go-20-python-exit`, branch `change/go-20-python-exit`.

## Specs
- S1 — Program decision that makes this the first slice: the binary becomes the entry point (A4), and
  the install model is "verified release asset first, local `go build` as fallback" (S9). The native
  `upgrade` engine has to own the install kinds before that flip, or the flip hands the install story
  to a bash script.
- S2 — `internal/upgrade` is already ported and reviewed (`Run` at `upgrade.go:348`, exit codes FROZEN,
  27 tests): flag parsing, install detection, the dirty-tree guard, `--dry-run`, the fetch +
  fast-forward path and the release report. This unit is the **dispatcher wiring plus the black-box
  evidence** the card asked for, not a reimplementation.
- S3 — After this unit `ai-specs upgrade …` runs natively (route kind, not a shim), and the
  differential harness compares it against the legacy launcher at the process boundary.
- S4 — Two unrecorded divergences are settled here explicitly, because the port silently omits two
  steps `lib/upgrade.sh` runs after a successful fast-forward. Neither is a behaviour this program's
  end state wants, and neither may stay silent:
  - **TUI dependency refresh** (`lib/upgrade.sh:330`, `python3 -m pip install --target
    lib/_vendor rich questionary`): the native path does **not** call it. The shell itself treats it
    as best-effort ("init will prompt on first use"), and S4 of the program says the end state has no
    Python: `rich`/`questionary` exist only for the still-Python wizard, which U4 ports. Until then a
    fresh machine sees init's own prompt instead of a provisioned vendor dir — a graceful degradation
    by the shell's own design, now recorded rather than discovered.
  - **Checkout narrowing** (`lib/upgrade.sh:343`, `bash lib/_internal/narrow-checkout.sh`, itself
    `|| true`): the native path does **not** call it either. It only prunes subtrees the CLI does not
    read at runtime — a size cost, never a behaviour cost — and it becomes meaningless in U1b, where
    the checkout stops being the installed artifact. The helper goes with the rest of `lib/` in U8.
- S5 — Everything else stays byte-identical, and the evidence is the harness's own differential: the
  same fixture, the same environment, native binary vs `bash bin/ai-specs`.
- S6 — No deletion in this unit (B2). `lib/upgrade.sh` and `changelog.py` stay until U2's caller audit
  proves nothing reaches them.

## Tasks
- T1 — S4: record the two divergences in code comments (so the next reader does not think they were
  forgotten) and pin them with a black-box test. Route: inline. Commit: pending.
- T2 — S3: the route kind, the native map entry and the `Run` case in `internal/cli`. Route: inline.
  Commit: pending.
- T3 — S3/S5: RED→GREEN on the route and on the differential table (the usage surface end to end).
  Route: inline. Commit: pending.
- T4 — Evidence: focused packages, `go test ./cmd/... ./internal/...`, parity, `./tests/validate.sh`.
  Route: inline. Commit: pending.

## Log
- L1 — Session request (verbatim): "decisión 1 me gusta la 4 al final es 'un nuevo inicio' ya con esto
  mas limpio sin tanto python+bash+go podria ser solo go +bash como wrappers", "quiero que este como
  si se hubiera hecho con go desde 0".
- L2 — Reconnaissance found `upgrade.Run` already present (`upgrade.go:348`) and the card's remaining
  item is its wiring; an earlier grep that reported no entry point was truncated by a `head -30`, and
  that wrong note was corrected in the program todo before any work was based on it.
- L3 — The two omissions were found by diffing the shell's post-merge steps against the port's
  `realUpgrade`, then confirmed absent from the card doc, the port and the tests: they are an
  unrecorded divergence, not a reviewed decision. Recording them is the substance of this unit.
- L4 — A `lib/sync.sh` reference inside `lib/upgrade.sh` is a comment ("Mirrors the contract
  established for sync in lib/sync.sh"), not a call: the shell upgrade never runs sync, so the port
  not running it is not a divergence. Recorded so the same false positive is not chased twice.

## Progress
- T1 done: the two omissions are recorded next to the code that makes them (`realUpgrade`'s doc
  comment names the shell line numbers, the reason, and the fact that the end state wants neither),
  and pinned in both directions by the black-box test — an accidental reintroduction fails a test
  instead of passing silently.
- T2 done, RED then GREEN: RED was `undefined: routeUpgrade`; GREEN is the route kind, the native map
  entry (with the stale shims entry removed) and the `Run` case, which passes the post-verb args and
  `Env{Home, UserHome: $HOME, Executable: selfPath()}` — the running binary's own path, exactly what
  the post-flip entry point needs.
- T3 done: the discrimination is exact. The fixture looks like a standard installation (binary inside
  `~/.ai-specs/bin`, `~/.local/bin/ai-specs` symlinked to it, `.git` present, `VERSION` 1.0.0) whose
  `lib/upgrade.sh` is a marker stub, with a stub `git` that reports a fast-forward to 9.9.9 and a
  marker-stub `python3`. The test asserts rc 0, the crossed-version report, and that NONE of the three
  markers exist: the shell route was not taken, the TUI-dependency refresh did not run, and the
  checkout was not narrowed. The usage surface is pinned separately against the legacy launcher
  (identical stdout/stderr/exit).
- T4 done: see Evidence.

## Log (continued)
- L5 — Fixture finding, recorded because it is a property of the port and not of this unit: the last
  install check in `Detect` compares PHYSICAL paths (`filepath.EvalSymlinks`) while `lib/upgrade.sh`
  compares logical ones (`cd "$(dirname …)" && pwd`). Under macOS's `/var -> /private/var` symlink the
  port therefore aborts with `~/.local/bin/ai-specs resolves outside ~/.ai-specs` where the shell
  would pass. The unit's fixture resolves its scratch root to stay faithful to a real `$HOME`; the
  divergence itself is pre-existing (card [Go 12]) and is recorded here as a follow-up rather than
  changed under this unit — a path-comparison change needs its own evidence and its own parity note.
- L6 — Process lesson, recorded because it cost a wasted evidence cycle: the first full-suite run
  FAILED on `TestRouteTableCoversEveryVerb` (`cli_test.go:33`, "verb \"upgrade\": kind = 10, want
  shim"), a test this unit had to move and did not. The focused RED/GREEN I had observed used
  `go test ./internal/cli -run 'TestUpgrade…'`, which cannot see a guarantee pinned by another test
  in the same package. Rule for the rest of the program: when the change moves a contract, run the
  WHOLE package (or list the tests that pin that contract explicitly) before quoting any evidence,
  and never let a `-run` subset stand as the unit's verification.

## Evidence
Run from this worktree at base `ecfb843`, with the real toolchains first on `PATH`
(`python 3.14.7`, `go 1.24.13`):

| Command | Result | Exit |
| --- | --- | --- |
| `go vet ./internal/cli ./internal/upgrade` | clean | 0 |
| `go test ./internal/cli -run 'TestUpgrade…' -count=1` | RED before the wiring (`undefined: routeUpgrade`); all three new tests `PASS` after | 0 (GREEN) |
| the FIRST full-suite run | **FAILED**: `TestRouteTableCoversEveryVerb` (`cli_test.go:33`) still expected `upgrade` to be a shim, so the change had moved a contract pinned elsewhere in the same package | 1 |
| `go test ./internal/cli -count=1` after moving the table | `ok` — the whole package, not a `-run` subset | 0 |
| `go test ./cmd/... ./internal/... -count=1` | every package ok | 0 |
| `bash scripts/build-gate.sh` | 4 targets built | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; both modes PASS | 0 |
| `./tests/validate.sh` | `Ran 2487 tests in 1619.385s` → `OK` | 0 |

Log: `~/.cache/u1a-evidence2.log` (chained go test → build-gate → parity → validate, after the
route-table fix; the first chain's failure is `~/.cache/u1a-evidence.log`). The Python suite count does
not move (2487): this unit adds Go tests only, and it deletes nothing (B2).
