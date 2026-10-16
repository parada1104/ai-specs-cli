# Go 20 / U3a-fix — The confirmed advisories of the hub's non-interactive contract

Unit of the program `odd/tasks/go-20-python-exit.md`. Base `0ba47aa` (U3a, PR #366). Worktree
`.worktrees/go-20-u3a-fix`, branch `change/go-20-u3a-fix`.

Cut from U3a's closed review (`review-97428fc2534aa06c`): its ten advisories are non-blocking and its
receipt stands, so the reviewed bytes stayed the published bytes and the repair is its own unit. Three of
the advisories were CONFIRMED by U3a's inline verification (that unit's document, `## Verification`); the
evidence is repeated here because it is what this unit's tests must reproduce.

## Specs

- S1 — **Real `isatty` (product fix, confirmed defect).** `internal/cli`'s `isTTY` is a character-device
  stat, which is not `isatty(3)`: `/dev/null` IS a character device. Under `stdin=/dev/null` and
  `stdout=/dev/null` over an uninitialized target, the Go route takes the interactive branch and launches
  Python (exit 9 with a marker `python3`; exit 1 and `exec: "python3": executable file not found in $PATH`
  when Python is absent) where the shell route prints the anti-hang guard and exits 2. The fix asks the
  kernel whether the descriptor is a terminal, per platform, **without adding a dependency** (D8: `go.mod`
  stays dependency-free), and keeps the character-device stat only as the documented fallback for
  platforms the program does not ship.
- S2 — **The `git` probe must skip, not error (test fix, confirmed).** The release-commit scope added by
  U1b-1 calls `git describe` without the `shutil.which` guard its sibling `go` lookup has, so with `git`
  absent from `PATH` the test errors with `FileNotFoundError` instead of skipping. A missing tool is not a
  finding about the trust root.
- S3 — **Pins for the frozen strings.** Two details this port deliberately preserves had nothing pinning
  them (U3a's `R3-noninteractive-coverage-gaps`): `{error} error(s)` stays LITERAL while warnings
  pluralize (`1 warning` / `2 warnings`), and the topology line appears only when the manifest is not the
  default resolution or when worktree-flow is enabled. Plus the `unknown` version fallback. These are
  PINS: they pass against today's code by design, and exist so that a later refactor cannot change a
  frozen string silently. The worktree-flow predicate also pins Python's `is True` semantics — `enabled =
  "true"` as a STRING must not count — which a port gets wrong easily.
- S4 — **The readability advisories, judged by reading.** `R2-1`..`R2-5` were not examined in U3a (only
  ids and locations were available). This unit reads the five sites and changes what is genuinely an
  improvement, recording what it deliberately leaves alone. No behavior may change from S4.

## Tasks

- T1 — S1/S1-fix: RED — `/dev/null` and `/dev/zero` must not be terminals, driven through the real `cli`
  seam so the assertion fails against today's characteristic-device code. Route: inline.
- T2 — S1: `internal/tty` (`IsTTY`, `IsTerminal`, one `ioctl` file per shipped platform, a documented
  fallback), and `internal/cli`'s `isTTY` delegating to it. Route: inline.
- T3 — S2: the `shutil.which("git")` guard plus a test that runs the single test in a subprocess whose
  `PATH` has no `git` and requires a SKIP. Route: inline.
- T4 — S3: the pins, including the pure `headlineForCounts` extraction the bytes need. Route: inline.
- T5 — S4: read the five sites, apply what improves them, record what does not. Route: inline.
- T6 — Evidence: focused packages, `go test ./cmd/... ./internal/...`, parity, `./tests/validate.sh`.
  Route: inline.

## Log

- L1 — Session request (verbatim, from the program's L1): "quiero que este como si se hubiera hecho con
  go desde 0".
- L2 — The human's sequencing decision (verbatim option label): "Publicar U3a ahora y luego U3a-fix".
  U3a was published as `0ba47aa` (PR #366) with its approved bytes, and this unit was opened immediately
  after, from that tip.
- L3 — Why this is not folded into U3b: the reviewed bytes must stay the published bytes, and a confirmed
  product defect deserves its own tier, tests and receipt rather than riding along with the TUI
  dependency and its three amendments.

## Progress
- T1 (RED): `internal/cli/isatty_test.go` observed failing against the published code —
  `isTTY(/dev/null, /dev/null) = true; /dev/null is a character device but not a terminal`, and the same
  for `/dev/zero`. The frozen-string pins were RED as a missing symbol (`undefined:
  headlineForCounts`), and the `git` guard's test was RED as the child erroring with
  `FileNotFoundError` instead of skipping.
- T2 (S1): `internal/tty` with `IsTTY`/`IsTerminal` asking the kernel per platform (`TIOCGETA` on
  darwin, `TCGETS` on linux) and a documented character-device FALLBACK behind `!darwin && !linux` so the
  build stays portable. No dependency: `go.mod` stays free of third-party modules (D8), which is why the
  answer comes from `syscall.Syscall` rather than a terminal library. `internal/cli`'s `isTTY` is now a
  four-line delegation. The package's own behavioural assertions live in `internal/tty/tty_test.go`; the
  DEFECT's regression assertion stays at the `cli` seam where the bug was, so a dispatcher that stops
  delegating fails there.
- T3 (S2): the `shutil.which("git")` guard, plus `test_reproducibility_skips_without_git`, which runs the
  single test in a subprocess whose `PATH` has no `git` and requires a SKIP — the real behaviour under
  that `PATH`, not the shape of the source.
- T4 (S3): the pins. `headlineForCounts` is extracted as the pure function the frozen bytes need (so the
  quirk is testable without manufacturing doctor severities), `TestReadVersionFallsBackToUnknown` pins
  the third string, and `TestWorktreeFlowEnabledKeepsPythonIsTrueSemantics` pins Python's `is True`
  semantics — `enabled = "true"` as a STRING must not count — which a port gets wrong easily.
- T5 (S4): the five flagged sites read one by one. R2-1 (five positional values from `runDoctor`) became
  the named `severityCounts`; R2-3 (the `default:` branch) became explicit cases, documenting that BOTH
  interactive states need the bridge; R2-4 (`Options.Target`) now says it is the caller's pre-resolved
  target, which only a test sets; R2-2 (`delegateArgv`) is kept and now says why it exists before U3b
  needs it; R2-5 was folded into T3's rewrite. No behaviour changed.
- T6 (evidence, `~/.cache/u3afix-evidence.log`): `go test ./cmd/... ./internal/...` exit 0 (50 packages,
  no failures); `scripts/build-gate.sh` exit 0; `python3 tests/parity/run.py` exit 0 (`fixtures: 29,
  failing: 0`); `./tests/validate.sh` exit 0 → `Ran 2496 tests in 1650.795s` (`OK`, `skipped=1`) — one
  more than U3a's 2495, which is exactly the new `test_reproducibility_skips_without_git`.
- **The fix is proven where it mattered, end to end.** The experiment that confirmed the defect (U3a's
  document) now agrees with the shell: with `stdin=/dev/null`, `stdout=/dev/null` and an uninitialized
  target, the Go route reports the two frozen lines and exits 2, and with a marker `python3` first on
  `PATH` it launches nothing — where U3a's run of the same case exited 9 with `PYTHON_WAS_LAUNCHED`.
- Commit: `54197dd` (code and tests); the document plus the program's record follows in the unit's docs
  commit.
