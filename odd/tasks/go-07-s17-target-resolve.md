# Go 07 S17 — Run target resolution in process (retire the last `target-resolve.py` exec)

Base `3ae502a` on `epic/go-single-binary` (S16 merged). Worktree
`.worktrees/go-07-s17-target-resolve`, branch `change/go-07-s17-target-resolve`.
Closing unit of the S16 deferred blocker #3 ("`lib/_internal/target-resolve.py`: needs a
native resolver (`internal/sync/syncagent.go:225` still execs it)").

## Specs
- S1 — Session request (verbatim): "continuemos con la migración completa a go por favor",
  as the continuation of the S16 deferred list (`odd/tasks/go-07-s16-cleanup.md:87-96`).
- S2 — Both Go target-resolution call sites run the existing native port in process instead of
  exec'ing Python: the spine (`internal/sync/sync.go:433`, `resolvePlan`) and the standalone
  agent plan (`internal/sync/syncagent.go:224`, `resolvePlanStandalone`) call
  `internal/target` in process. After this unit no Go code execs
  `lib/_internal/target-resolve.py`; the only remaining callers are the shell route
  (`lib/sync.sh:177`, `lib/sync-agent.sh:112`), which stays until the dispatcher cutover.
- S3 — No output change: the plan bytes received by the callers stay byte-identical to
  `print(json.dumps(plan, indent=2))`, stderr and exit codes stay identical, and both call
  sites keep the "ERROR: target resolution failed before any writes." framing. The two
  STABLE PORTABLE standalone diagnostics (malformed plan body, root-less plan) survive as a
  defensive extraction path and stay pinned.
- S4 — The discrimination is proven, not asserted: a PATH-stubbed `python3` that records its
  invocations shows `target-resolve.py` absent after the change, and the plan the standalone
  agent fans out from is the REAL native plan (a manifest that declares its two subrepos),
  not a stub-synthesized one. The previous test drove the fan-out with a synthesized plan
  shape that no real resolver can emit (it omitted the `root` target row).
- S5 — `docs/go-migration-parity-contract.md` records that the resolver plan is produced in
  process: the "Resolver-plan diagnostic framing" section is amended (the classification and
  the frozen items stay, now scoped to the shell route and to the defensive extraction).
- S6 — No Python or bash module is deleted here. `lib/_internal/target-resolve.py` is still
  executed by `lib/sync.sh` and `lib/sync-agent.sh`; its deletion belongs to the dispatcher
  cutover slice, together with the rest of the shell route.

## Tasks
- T1 — S2/S3: replace both exec bodies with one native `resolvePlan` helper over
  `internal/target.Run`. Route: inline. Commit: pending.
- T2 — S3: extract the standalone extraction into a testable pure helper and retarget the
  diagnostics test to it (the plan body can no longer come from a stub). Route: inline.
  Commit: pending.
- T3 — S4: make the standalone fan-out fixture declare its subrepos and assert
  `target-resolve.py` never reaches the stub, plus a native-plan equivalence test for both
  call sites under a failing `python3` stub. Route: inline. Commit: pending.
- T4 — S5: amend the parity contract. Route: inline. Commit: pending.
- T5 — Evidence: focused packages, parity corpus in both modes, `./tests/validate.sh`.
  Route: inline. Commit: pending.

## Log
- L1 — "continuemos con la migración completa a go por favor" (2026-10-15). The session opens
  from the S16 close-out: epic `go-single-binary` tip `3ae502a`, clean and in sync with
  origin; remaining work = the four S16 deferred blockers, then the dispatcher cutover and
  the module deletion.
- L2 — Reconnaissance (read-only subagent) located the blocker precisely and found a second
  call site the S16 note did not name: `internal/sync/sync.go:433` (`resolvePlan`, the spine)
  execs `target-resolve.py` exactly like `syncagent.go:224` does. Both are in scope here.
- L3 — Decision (evidence-backed): `internal/target` already carries the full port with its
  own differential suite (`internal/target/differential_test.go`, 718 lines) and
  `Run(args, stdout, stderr)` mirrors the script's argv, stdout bytes, stderr and exit code,
  so this unit only rewires call sites — no new resolution logic is written.

## Progress
- T1 done, RED then GREEN. RED (observed before any product edit):
  `TestResolvePlanRunsNatively` failed with "resolvePlan failed; the native resolver must not
  depend on python3", `TestResolvePlanReportsFailureNatively` with "python3 was invoked:
  lib/_internal/target-resolve.py …", and both `TestStandaloneMaterializeRunsNatively` scenarios
  with "the plan still spawns target-resolve.py". GREEN: one native `resolvePlan` on
  `internal/target.Run` serves both call sites, `resolvePlanStandalone` is gone, and the nil-stderr
  case discards the port's diagnostics exactly as a nil `exec` stderr did.
- T2 done: `extractStandalonePlan` carries the shell's `ROOT_PATH`/`RESOLVED_TARGETS` extraction
  (the two failure classes, the null-root `None` collapse) as a defensive path with the frozen
  diagnostic bytes untouched; `TestStandalonePlanDiagnostics` now drives it directly (malformed
  body, missing root, wrong-typed root, null root) and keeps the end-to-end ordering pin — a
  resolution failure precedes any banner — plus a portability guard on the bytes.
- T3 done: the fan-out fixture declares its subrepos instead of scripting a plan body, so the
  agent's fan-out is driven by the real native plan (root row included, which the retired stub
  shape omitted); `syncagent_materialize_test.go` asserts `target-resolve.py` never reaches the
  stub. New `target_resolve_native_test.go` pins the spine's bytes against `internal/target`'s own
  output and the failure contract, both under a failing `python3`.
- T6 done: `tests/test_sync_output_verbosity.py:889` no longer PATH-stubs `target-resolve.py`
  (it drives the Go binary, so that seam was retired by design); the fault is injected at the real
  seam — a target directory without a manifest — and the assertions (rc 1, the spine's framing
  line, the resolver's own `error: …` diagnostic first, no header, no write work) all hold.
- T4 done: the parity contract's "Resolver-plan diagnostic framing" section records that the plan
  is produced in process since S17 and scopes the classification to the shell route and the
  defensive extraction. `odd/tasks/go-07-s16-cleanup.md`'s Deferred item 3 is marked closed by S17.
- T5 done: see Evidence.

## Evidence
Run from this worktree at base `3ae502a`, with the real toolchains first on `PATH`
(`python 3.14.7`, `go 1.24.13`):

| Command | Result | Exit |
| --- | --- | --- |
| `go vet ./internal/sync` | clean | 0 |
| `go test ./internal/sync -run 'TestResolvePlanRunsNatively\|TestResolvePlanReportsFailureNatively\|TestStandaloneMaterializeRunsNatively\|TestStandalonePlanDiagnostics' -count=1` | RED before the change, `ok` after | 0 (GREEN) |
| `python3 -m unittest tests.test_sync_output_verbosity.ErrexitInteractionTests.test_t4_1_target_resolution_failure_aborts_before_the_header` | `Ran 1 test` → `OK` | 0 |
| `go test ./cmd/... ./internal/... -count=1` | every package ok, including `internal/sync` and `internal/target` | 0 |
| `bash scripts/build-gate.sh` | 4 targets built | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; `gate-absent failing=0, gate-present failing=0` | 0 |
| `./tests/validate.sh` | `Ran 2482 tests in 1600.746s` → `OK`, parity both modes PASS | 0 |

Log: `~/.cache/s17-evidence.log` (chained go test → build-gate → parity → validate).

## Assessment
`gentle_review assess` over the committed range `3ae502a..3e8ede1` (committedOnly, this
worktree): risk **medium** (`executable_change`), writer profile `large` (runtime), 9 changed
paths / 480 changed lines, `reviewDue: false` (`under_budget`) → no native review transaction was
started, which is the tier's own path. Plan: writer self-verification stands, no separate
verifier required; the evidence table above is that self-verification.

## Size
Disclosed: about 45 authored lines of product code (one native `resolvePlan`, the extraction
helper move, the call site, the package comment), about 100 of test changes (three new pins plus
the retargeted diagnostics and fan-out fixtures and the CLI-level seam), about 10 of contract and
register docs, and this document. Well inside the plan's ~700-line cap.

## Follow-ups (recorded, not done here)
- `internal/sync/sync.go`'s package doc still says "every step still execs the existing Python
  module … Nothing in the Python layer is ported here", which stopped being true at S10–S16. Not
  caused by this unit; left for the cutover/docs slice.
- The remaining S16 deferred blockers are untouched: the doctor brief ops port, `lib/init.sh:242-245`,
  the `lib/skills-add.sh:208` auto-sync, the parity legacy leg, and then the dispatcher cutover that
  finally deletes the Python modules — including `lib/_internal/target-resolve.py`, which the shell
  route still executes.
