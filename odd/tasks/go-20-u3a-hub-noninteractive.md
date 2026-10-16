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
