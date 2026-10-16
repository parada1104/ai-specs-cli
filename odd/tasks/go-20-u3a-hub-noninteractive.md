# Go 20 / U3a — The hub's non-interactive contract in Go (no TUI, no dependencies)

Unit of the program `odd/tasks/go-20-python-exit.md` (U3, first slice). Base `20e653a`. Worktree
`.worktrees/go-20-u3-hub`, branch `change/go-20-u3-hub`.

## Specs
- S1 — Program decision (human): port the remaining verbs first and flip the launcher at the end. U3 is
  the `hub` verb, which is also the DEFAULT verb (bare `ai-specs`).
- S2 — Human decision recorded for the TUI: the interactive hub gets a Go TUI **with a dependency**
  (the analogue of questionary: Select/MultiSelect/Confirm), vendored by the repository's own precedent
  (`lib/_vendor`), together with three explicit amendments in the same commit — D8 (`go.mod:9`, the
  release workflow's no-`go.sum` note, `scripts/build-gate.sh`, and the phase-4 regression test that
  asserts no `cache-dependency-path`), the `assets.go` "zero third-party dependencies" sentence, and
  the parity contract's FREE entry for rich/questionary rendering. That work is **U3b**.
- S3 — This slice is the dependency-free half, and it is what makes the verb native: the mode decision,
  the non-interactive status summary, the uninitialized error, the delegation and the flag contract. The
  INTERACTIVE branch is bridged to `lib/_internal/hub.py` fail-open until U3b, exactly as the doctor
  bridges what it still needs, so the interactive UX is unchanged and the PTY suites keep passing.
- S4 — Byte parity for the non-interactive output, including its two quirks: `{error} error(s)` is
  LITERAL while warnings pluralize (`1 warning` vs `2 warnings`), the topology line appears only when
  the manifest says so (`source != "default"` or worktree-flow enabled), and the menu column is
  `{title:12s}` followed by two spaces.
- S5 — The exit code is the DOCTOR's, not 0 (the D5 fix): a project with errors exits non-zero, and the
  Summary line reports the same counts.
- S6 — The delegation keeps the shell's argv shape `[cli, action, ...extra, target]`, but `cli` is the
  RUNNING BINARY (`selfPath()`) rather than `$AI_SPECS_HOME/bin/ai-specs`: that is the end-state shape
  (the hub delegates to itself), while the shell route keeps delegating to the launcher.
- S7 — `lib/hub.sh`'s contract that the native route must keep: the usage text, exit 2 for an unknown
  flag and for a second positional, the absolute target resolution, and the anti-hang guard — when the
  target is uninitialized and there is no TTY, the same two stderr lines and exit 2 WITHOUT launching
  Python and therefore without requiring its dependencies.
- S8 — No deletion and no amendment in this slice: `lib/hub.sh` and `lib/_internal/hub.py` keep serving
  the shell route, and the three amendments land with U3b.

## Tasks
- T1 — S3/S4/S5/S6/S7: RED — the Go tests for the mode matrix, the output bytes, the exit code, the
  delegation argv and the flag/guard contract. Route: inline.
- T2 — S3: `internal/hub` with the mode decision, the status summary over the native doctor, the menu
  table and the delegation. Route: inline.
- T3 — S3/S7: the native `hub` route in `internal/cli` with the interactive branch bridged to
  `hub.py`. Route: inline.
- T4 — Evidence: focused packages, the PTY suites unchanged, `go test ./cmd/... ./internal/...`, parity,
  `./tests/validate.sh`. Route: inline.

## Log
- L1 — Session request (verbatim): "quiero que este como si se hubiera hecho con go desde 0".
- L2 — Reconnaissance (delegated, read-only) established the whole contract: the four-state matrix
  (`decide_mode`), the 12 menu entries, the exact non-interactive lines, the delegation table, the
  widget layer and its call sites, `hub.sh`'s flags and guard, and the two Python suites — 1 448 lines
  that drive the verb through `bin/ai-specs` and a REAL PTY, never importing `lib/_internal`.
- L3 — Slicing decision: U3a/U3b. The TUI decision (S2) adds a dependency and must amend three records,
  which is a different risk from porting the contract; and U3a can land with the interactive path
  bridged, so nothing user-visible changes while the dependency question is implemented properly.
- L4 — Consequence recorded for U3b: the PTY suites' `TestNoOpenptyInHub` and `TestPauseOnlySite` are
  Python-source invariants that become meaningless in Go, and `TestStatusPanelRender` is coupled to
  rich's exact panel text. Retargeting those three is part of U3b, not a surprise.

- L5 — The first evidence chain failed, and the failure was in U1b-1's own test rather than in this
  slice: `test_host_platform_digest_is_reproducible_locally` compared the committed trust root against
  a build of the WORKING TREE. `bin/SHA256SUMS` describes a RELEASED version's bytes (v0.24.0), so
  every Go change rebuilds to a different binary and the assertion is a false positive on any
  development commit — and its message blamed the trust root instead. Fixed with two guards that skip
  with their own reason: the comparison runs only when HEAD is exactly the digests' release tag (the
  release workflow performs it there, in its checksum job), and a non-canonical local toolchain skips
  rather than failing. That closes the two toolchain-coupling advisories the U1b-1 review raised
  (`R3-host-digest-test-toolchain-coupled`, `R4-repro-test-toolchain`) and the commit coupling they did
  not name; the program rule is recorded as L14 of the program document.
- L6 — S3/S8's boundary held: the interactive branch is bridged, so `lib/hub.sh` and
  `lib/_internal/hub.py` are untouched and the two PTY suites (1 448 lines) still pass. Nothing
  user-visible changed in this slice.

- L7 — The review binding has three traps for this workflow, and all three were learned by failing.
  (a) `assess` without `workspaceRoot` measures the SESSION CWD: here that was `development` in the main
  checkout, so it reported 543 paths / 120 486 lines and a `high` tier justified by paths this unit
  never touched, with the epic tip as the only sane reference — every call made from inside a worktree
  must carry `workspaceRoot`. (b) The provider's default projection is "since the last release"
  (`e773514a`, the 0.24.0 bump), so a unit whose candidate is the delta since the epic tip must pass an
  explicit `baseRef` + `committedOnly`, which is what narrowed the projection to these 7 paths. (c) An
  over-long `lensesReason` is rejected as `lenses-invalid` with `lineage_created: false` and no
  mutation, which is safe to retry with a shorter reason — the lens NAMES were never the problem.
- L8 — The advisories were verified instead of waved through, and that was the right call: three lenses
  converged on the same eight lines (`internal/cli/cli.go:158-165`), and the differential in the
  Verification section confirmed a real divergence rather than a style preference — under the same
  `stdin=/dev/null` + `stdout=/dev/null` conditions the shell route prints the guard and exits 2 while
  the Go route launches Python, which is exactly the state A4 is heading for. Per the contract the
  approved receipt stands (it is not a reason to re-run review on this candidate); per this program a
  confirmed defect is written down with its evidence and gets its own unit.
- L9 — The fix is cut as **U3a-fix** rather than folded into U3b, so the reviewed bytes stay the
  published bytes and the repair gets its own tier, tests and receipt. It carries four items: a real
  `isatty` behind build tags (no dependency), the `shutil.which("git")` guard in the release-commit
  scope, pinning the two format quirks and the `unknown` version fallback, and the four `R2-*`
  readability suggestions if they survive reading.

## Review

- Lineage `review-97428fc2534aa06c`: tier `high`, four lenses (`review-risk`, `review-resilience`,
  `review-readability`, `review-reliability`) over four host-relay reviewer runs (`pi_host_relay`,
  53 277-53 323 prompt bytes each), against the provider-frozen trees (`base_tree 1d024831`,
  `candidate_tree d1a537b1`, 7 paths / 790 lines). Closed `approved`
  (`native-last-event-closure`), then acknowledged: authority **burned**
  (`gentle-ai.review-acknowledged/v1`), delivery back to ordinary repository policy.
- Ten advisory findings, all `informational`: none opened a correction and no correction transition was
  offered. Three lenses converge on one site — `R1-001` (risk, `internal/cli/cli.go:158-165`),
  `R3-istty-chardevice-not-isatty` (reliability, same site) and
  `R4-istty-chardevice-defeats-antihang-guard` (resilience, same site) — two attack this unit's tests,
  `R3-noninteractive-coverage-gaps` (`internal/hub/hub_test.go:60-105`) and
  `R3-trust-root-git-missing` (`tests/test_cli_release_trust_root.py:85-88`), and five are readability
  suggestions (`R2-1` `hub.go:251`, `R2-2` `hub.go:315-323`, `R2-3` `hub.go:195-203`, `R2-4`
  `hub.go:140-142`, `R2-5` `test_cli_release_trust_root.py:85-93`).

## Verification (inline; the delegated verifier is unavailable in this environment)

The method is deliberately different from the unit's tests: those drive the Go functions with
`bytes.Buffer`s, so the verification drove the BUILT BINARY and the SHELL LAUNCHER side by side under the
same conditions, with a marker `python3` on `PATH` to DETECT a Python launch instead of inferring it.

- **R1-001 / R3 / R4 — CONFIRMED, and it is a product divergence.** `/dev/null` IS a character device
  (`stat.S_ISCHR = True`), so a char-device stat is not `isatty(3)`.
  - Control A (`stdin=/dev/null`, `stdout` a pipe): the guard fires correctly — the two frozen lines,
    exit 2.
  - Control B (the shell launcher, same case): guard, exit 2, **no** Python launched.
  - **Failure** (`stdin=/dev/null` AND `stdout=/dev/null`, uninitialized target): the Go route takes the
    interactive branch and launches Python (`rc=9`, `PYTHON_WAS_LAUNCHED (argv: …/hub.py …)`) where the
    shell route prints the guard and exits 2. With Python absent the user gets
    `ai-specs hub: exec: "python3": executable file not found in $PATH` and exit 1 instead of the frozen
    message and exit 2 — the exact state A4 aims for, so this matters beyond the corner case.
- **R3-trust-root-git-missing — CONFIRMED.** With `git` absent from `PATH` the test errors with
  `FileNotFoundError` instead of skipping: the release-commit scope calls `git describe` without the
  `shutil.which` guard its sibling `go` lookup already has.
- **R3-noninteractive-coverage-gaps — CONFIRMED and narrowed.** `TestNoninteractiveOutputShape` pins the
  line shapes, the blank line before `Commands:`, the menu line count and `rc == DoctorExitCode`, but
  nothing pins the two format quirks this port deliberately preserves — `{error} error(s)` staying
  LITERAL while warnings pluralize, and the conditional topology line — nor the `unknown` version
  fallback. Those are exactly the details a later refactor would "fix" without noticing.
- **Two probes are reported as inconclusive, not as evidence**: `PATH=/bin` broke the launcher's own
  `dirname`, and a `PATH` without `tail` broke the reporting pipeline. They tested nothing.
- The five `R2-*` readability suggestions were not examined in substance and stay recorded as advisories.

## Progress
- T1 (RED): `internal/hub/hub_test.go` was written first and observed failing — `undefined: Mode`,
  `ModeInteractiveHub`, `ModeNoninteractiveStatus`, `ModeOfferInit`, `ModeErrorUninitialized`,
  `DecideMode`, `RunNoninteractive`, `Menu` and `DoctorExitCode`, plus `RunHub`, `Action` and
  `delegateArgv`. Six tests: the mode matrix, the output shape (menu size, summary line, conditional
  topology line), the uninitialized guard, the delegation argv, the flag contract, and the menu copy
  against `lib/hub.sh`'s own entries.
- T2: `internal/hub/hub.go` — the mode matrix, the menu table, `RunNoninteractive` over the native
  doctor, `DoctorExitCode`, `readVersion`, the conditional topology line, `delegateArgv`, the guard and
  the flag loop, and the interactive bridge to `hub.py` with `AI_SPECS_HOME` pinned for the child.
- T3: `internal/cli` — `hub` moved out of the shims map into the native table, `Route(nil)` now returns
  the native hub, the `routeHub` case, and `isTTY` as a char-device stat (no terminal library, no
  per-platform syscall). `internal/cli/cli_test.go`'s route table moved in the SAME commit and BEFORE
  the chain, per L11's rule — the third time this session that rule was needed and the first time it
  was applied ahead of the evidence.
- T4 (evidence, `~/.cache/u3a-evidence2.log`): `go test ./cmd/... ./internal/...` exit 0 (the whole
  `internal/hub` and `internal/cli` packages green; the latter includes the smoke differential, which
  now compares the NATIVE hub against the shell launcher in both non-TTY cases);
  `scripts/build-gate.sh` exit 0; `python3 tests/parity/run.py` exit 0 (`fixtures: 29, failing: 0`);
  `./tests/validate.sh` exit 0 → `Ran 2495 tests in 1654.638s`. The first chain
  (`~/.cache/u3a-evidence.log`) failed on the coupling recorded in L5 and is kept as evidence.
- Commit: `c493744` (code and the U1b-1 test fix; this document plus the program's L12/L13/L14 follow
  in the unit's docs commit).
