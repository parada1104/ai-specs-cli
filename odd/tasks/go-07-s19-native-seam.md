# Go 07 S19a — The native seam the legacy scripts reach, and the add-dep auto-sync

Base `8f992cf` on `epic/go-single-binary` (S18 merged). Worktree
`.worktrees/go-07-s19-native-seam`, branch `change/go-07-s19-native-seam`.
S16 deferred blocker #1, first half: `lib/skills-add.sh`'s auto-sync is the last Python-launching
call site on the add-dep path.

## Specs
- S1 — Session request (verbatim): "continuemos con la migración completa a go por favor", and the
  human's decision this session for the open design fork: "Costura AI_SPECS_GO_BIN (fail-open) — el
  shim del binario Go exporta su propia ruta; los scripts la usan si está puesta y caen a la ruta
  Python si no. Cierra los call sites de init y skills-add sin decidir todavía el layout de
  instalación".
- S2 — The Go dispatcher exports `AI_SPECS_GO_BIN`, the absolute path of the running binary, into
  every shimmed child (`internal/cli/shim.go`). A stale inherited value never leaks: the child env
  drops any `AI_SPECS_GO_BIN` it finds and sets its own only when the path resolves.
- S3 — `lib/skills-add.sh`'s auto-sync prefers the native route when the seam names an executable:
  `"$AI_SPECS_GO_BIN" sync "$TARGET_PATH"` — `sync` is already native in `internal/cli/cli.go`
  (`routeSync` → `internal/sync.Run`) — and otherwise keeps `bash "$AI_SPECS_HOME/lib/sync.sh"`, so a
  direct `bash lib/skills-add.sh` invocation, or any invocation through the bash launcher, keeps the
  legacy route bit for bit.
- S4 — No observable change on either route path: the `▸ ai-specs sync <target>` banner stays where
  the script prints it, the sync's stdout/stderr and exit status propagate unchanged (the native
  spine reproduces the shell's stream contract, already pinned by the S1 differential and the parity
  corpus), and `--no-sync` still prints the "dep registered" hint and runs nothing.
- S5 — Failure is open in both directions: a seam that is unset, empty, or not executable falls back
  to the shell route instead of failing the command; a native sync that fails propagates its own exit
  status exactly as the shell route did.
- S6 — The seam is preparation, not cutover: nothing here changes `bin/ai-specs` (still the bash
  launcher) or deletes a module. In today's launcher flow the Go binary never runs, so the seam is
  unset and the behaviour is unchanged; it activates when the dispatcher is the native binary, which
  is the cutover's own unit (S20).
- S7 — `lib/init.sh`'s baseline render is *not* in this unit: it needs a native surface for
  "materialize the resolved config to a temp file, then render AGENTS.md with
  `--preserve-if-runtime-brief`/`--adopt-brief`", which does not exist yet. It is registered as S19b
  and its reconnaissance is recorded in the Log.

## Tasks
- T1 — S2: thread the running binary's path into the shim's child env. Route: inline. Commit: pending.
- T2 — S3/S4/S5: prefer the native route in `lib/skills-add.sh`. Route: inline. Commit: pending.
- T3 — S2/S3: pin both directions (seam set → native invoked with `sync <target>`; seam unusable →
  seam not invoked and the shell route entered). Route: inline. Commit: pending.
- T4 — Evidence: focused packages, `go test ./cmd/... ./internal/...`, parity both modes,
  `./tests/validate.sh`. Route: inline. Commit: pending.

## Log
- L1 — "continuemos con la migración completa a go por favor" (2026-10-15), continuing the S16
  deferred blockers after S17 (target resolution) and S18 (doctor brief probes) were merged.
- L2 — Reconnaissance (read-only subagent) established the fork and the reason it cannot be
  papered over: there is **no** bash→Go seam today (the only live one is `WORKTREE_GATE_BIN`, a pin
  for the gate binary; `GO_SYNC_STEP_*` travels the other way, from the environment into the Go
  spine), there is no `AI_SPECS_GO_BIN`, no helper in `lib/` resolves a native binary, and **no
  document records the cutover layout** (where the compiled binary will live and what `bin/ai-specs`
  becomes) — that is card 16 and the human's. Without a seam, `lib/init.sh:242-245` and
  `lib/skills-add.sh:208` cannot reach native code at all.
- L3 — Human decision: the fail-open `AI_SPECS_GO_BIN` seam. Recorded here because it shapes the
  cutover: the shell call sites become native as soon as the dispatcher is, and the cutover itself
  only has to change which path `bin/ai-specs` is, not the call sites.
- L4 — Why the seam makes this unit inert today (and why that is correct): `install.sh` links
  `bin/ai-specs` into `~/.local/bin`, and that launcher is bash — in that flow the Go binary never
  runs, the seam is unset, and the script takes its legacy route. The native route is reachable
  today only through the Go dispatcher (`internal/cli`), which is exactly the post-cutover entry
  point and the path the new tests exercise.
- L5 — Deferred to S19b with its evidence: `lib/init.sh:235-262` needs `EvaluateBriefRender`
  (`internal/sync/briefpolicy.go:41`), a resolved-config materialization to a temp file, and
  `RenderAgentsFile` (`internal/sync/agentsrender.go:1332`) with the `--preserve-if-runtime-brief`
  and `--adopt-brief` semantics. The pipeline composes those inside `renderAgentsStep`, but no CLI
  surface exposes "render the baseline brief only", so S19b adds one instead of inventing a second
  renderer.

## Progress
- T1 done, RED then GREEN. RED: `internal/cli/shim_seam_test.go` failed to build (`too many arguments
  in call to childEnv`); GREEN: `childEnv(home, invokedAs, selfPath)` drops any inherited
  `AI_SPECS_GO_BIN` and sets its own only when the path resolves, and `Run` resolves it once through
  `selfPath()` (`os.Executable()`, empty on failure so the seam stays unset).
- T2 done: `lib/skills-add.sh`'s auto-sync prefers `"$AI_SPECS_GO_BIN" sync "$TARGET_PATH"` when the
  seam names an executable and keeps `bash "$AI_SPECS_HOME/lib/sync.sh"` otherwise. The banner stays
  where the script prints it, so the printed contract is unchanged on both routes.
- T3 done, RED then GREEN on both directions: RED was the positive test failing because the seam was
  never consulted (the real shell sync ran and died vendoring a fixture with no root `SKILL.md`).
  GREEN is `tests/test_skills_add.AutoSyncRouteTests`: with an executable stub the stub receives
  exactly `sync <project>`; with a non-executable stub the stub is never executed and the shell route
  completes against the offline vendoring fixture. `tests/test_skills_add` as a whole: 17 tests OK,
  so registration, `--no-sync`, ordering and the existing outcomes are untouched.
- T4 done: see Evidence.

## Evidence
Run from this worktree at base `8f992cf`, with the real toolchains first on `PATH`
(`python 3.14.7`, `go 1.24.13`):

| Command | Result | Exit |
| --- | --- | --- |
| `go vet ./internal/cli` | clean | 0 |
| `go test ./internal/cli -run TestChildEnvExportsTheNativeDispatcherPath -count=1` | RED before the change (`too many arguments in call to childEnv`), `ok` after | 0 (GREEN) |
| `python3 -m unittest tests.test_skills_add.AutoSyncRouteTests -v` | RED before the change (the seam was never consulted), both directions `ok` after | 0 (GREEN) |
| `python3 -m unittest tests.test_skills_add` | `Ran 17 tests` → `OK` | 0 |
| `go test ./cmd/... ./internal/... -count=1` | every package ok | 0 |
| `bash scripts/build-gate.sh` | 4 targets built | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; both modes PASS | 0 |
| `./tests/validate.sh` | `Ran 2484 tests in 1670.646s` → `OK` | 0 |

Log: `~/.cache/s19a-evidence.log` (chained go test → build-gate → parity → validate). The
suite count moves from the historical 2482 to **2484**: the two new route tests are the only
addition, so the delta is accounted for rather than assumed.

## Native review (high tier, four lenses)
Bound STATUS → group capture of all four reviewers → last-event closure:

- Lineage `review-bf26b39cd805e27c`, target `sha256:db915c554b95ccb692437758e69a975ec8869da8576402e01a85990ce880ae40`,
  candidate `8f992cf90dc7b399dbe4b7841c850d94783ed7a3..0ce597c` (committed-only), 6 paths / 289
  changed lines, correction budget 145. Risk `high` with both `process_boundary` and
  `shell_source` on `lib/skills-add.sh`. Consent was resolved by the host.
- Group capture over `pi_host_relay`: 4/4 reviewers submitted, closure `approved` on the last
  admitted event (no correction offered), authority burned (`gentle-ai.review-acknowledged/v1`).
- 8 advisory findings, all informational and non-blocking (readability R2-001..R2-005, reliability
  R3-001 / R3-002, resilience R4-gofmt-switch-join). See Follow-ups.

## Independent verification
The high-risk plan requires a separate verifier. The delegated `gentle-ai-verify` subagent is
reported **unavailable** (it timed out once in S18 with the same 30-minute bash stall and is not
retried), so the obligation was discharged inline by a different method: end to end through the
built binary, with the environment poisoned, which is stronger than any of the unit's own tests.

| Check | Command (temp dirs only) | Result |
| --- | --- | --- |
| A stale inherited value never reaches the child | `AI_SPECS_GO_BIN=<recording stub> <built binary> add-dep <kepano url> <project> --subdir skills/defuddle` | rc 0, banner once, **the stub was never executed**, native sync completed offline |
| The fallback still runs when the seam is absent | same command with `AI_SPECS_GO_BIN` unset | rc 0, banner once, legacy `sync.sh` completed offline |
| The offline vendoring leg is real | `AI_SPECS_VENDOR_FIXTURE_ROOT=tests/fixtures/kepano-obsidian-skills` | no network, no `✗`/`ERROR` line in either run |

## Size
Disclosed: about 25 authored lines of product code (`selfPath()` and the child-env seam in
`internal/cli`, the nine-line route branch in `lib/skills-add.sh`), about 100 of tests (the Go seam
matrix and the two Python route tests with their hermetic stubs), and this document. Well inside the
plan's ~700-line cap.

## Follow-ups (recorded, not done here)
- Native review advisories, all non-blocking and none reopening the candidate: `R2-001`
  (`internal/cli/cli.go:88`), `R2-002` (`internal/cli/shim.go:24`), `R2-003`
  (`lib/skills-add.sh:208-214`), `R2-004` (`tests/test_skills_add.py:234`), `R2-005`
  (`internal/cli/shim.go:65`) — readability, informational; `R3-001` (`internal/cli/cli.go:88`) and
  `R3-002` (`tests/test_skills_add.py:245-269`) — reliability; `R4-gofmt-switch-join`
  (`internal/cli/cli.go:88`) — resilience. The `cli.go:88` pair is worth one pass together: it is
  the `default:` arm that now carries the extra argument.
- S19b: `lib/init.sh:235-262` needs a native surface for "materialize the resolved config to a temp
  file, then render AGENTS.md with `--preserve-if-runtime-brief`/`--adopt-brief`". The pieces exist
  (`EvaluateBriefRender`, the materializer's resolved-config output, `RenderAgentsFile`), but no CLI
  surface composes them, so S19b adds one instead of inventing a second renderer.
- S20 (the human's): flip `bin/ai-specs` to the native dispatcher and delete the Python modules.
  This seam means that flip changes which program `bin/ai-specs` is, not the scripts' call sites.
- The seam is unset in the bash-launcher flow by construction; when the promotion happens, the
  cutover notes should state that `AI_SPECS_GO_BIN` is set by the native dispatcher only, so an
  operator debugging an old-style invocation knows why the legacy route ran.
