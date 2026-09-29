# [Go 07.S1] Sync spine — internal/sync + capture/verbosity/errexit + native sync route

Card: `6abafbb412632531a9605331` — https://trello.com/c/kG7sapO2 (parent `6a84e7acca1baf394d9b482b`)
Branch: `change/go-07-s1-sync-spine` · Worktree: `.worktrees/go-07-s1-sync-spine`
Base: epic tip `1c9806c10791d9a2d08d9247508e77560b246276` (`epic/go-single-binary`)
Plan (authoritative, already merged): `odd/tasks/go-07-sync-plan.md` (slice **S1**, §(c) strangler mechanics, §(d) slicing, §(e) R3/R7, §(f) Layer 6, Q4/Q5/Q7/Q8)

## Goal

Port the `lib/sync.sh` **pipeline spine** to Go as a forward strangler:

1. `internal/sync` — step orchestration, `run_step` semantics, verbosity +
   compact-output filter (the four glyphs), errexit discipline (cleanup before
   status), temp-file trap coverage, `mktemp` degradation, `exit $RECIPE_RC`
   pass-through, every existing flag.
2. A native `sync` route in `internal/cli` so `ai-specs sync` runs the Go spine.
3. Parity-harness `gate-present` mode (plan F3/Q4) so the corpus can observe the
   Go-authority bridge path, run in **both** gate modes.
4. Re-point `tests/test_sync_output_verbosity.py`, `tests/test_sync_run_step_errexit.py`
   and `tests/test_sync_recipe_capture.py` from `bash lib/sync.sh` to the CLI
   boundary so they keep gating the port (plan F4).

Non-goals (binding): **no step is ported in S1** — every step still execs the
existing Python module (per-step opt-in env `GO_SYNC_STEP_*=python`); no
`sync-agent` port; no fixture-corpus additions; no `--refresh-gates` work beyond
forwarding it; no deletion of any Python/Bash module (Q5: the CLI only ADDS Go).

## Design

### Forward spine, zero step ports

`internal/sync.Run` reimplements `lib/sync.sh`'s orchestration and, for every
step, execs `python3 <AI_SPECS_HOME>/lib/_internal/<module>.py …` with the same
argv/cwd/env as the Bash spine. The `GO_SYNC_STEP_<NAME>` env contract exists
(plan §(c)) but S1 has no Go step: only `python` (or unset) is accepted; anything
else fails loudly instead of silently falling back.

### `mktemp` is exec'd, not reimplemented

Measured on this machine (macOS): `mktemp` **ignores `TMPDIR` entirely** and uses
the Darwin confstr temp dir; `mktemp -t tmpl-XXXXXX.json` does **not** substitute
`XXXXXX` there — it appends a random suffix (`tmpl-XXXXXX.json.RANDOM`). Go's
`os.CreateTemp` honours `TMPDIR` and substitutes `*`, so a stdlib reimplementation
would diverge from the FROZEN behavior on this platform. The spine therefore
execs the `mktemp` binary exactly as the shell does (`os.CreateTemp` rejected),
which also makes PATH-level fault injection identical for the Bash and Go legs of
the re-pointed suites.

### Faithful contract points (measured against `lib/sync.sh`)

| Contract | Legacy | Go |
| --- | --- | --- |
| bare `run_step` failure | exits with the wrapped command's rc | `return rc` at the bare call site |
| fan-out failure | `ERROR: sync failed for target … previous writes are not rolled back.` + exit 1 | same text, exit 1 |
| recipe block failure | prints full unfiltered out/err, `exit $RECIPE_RC` | same, returns `RECIPE_RC` |
| temp cleanup | `trap … EXIT` registered after the 3 `-t` temps, `:-` expanded | `defer` registered at the same point |
| compact filter | drop blank lines + lines whose first non-space char is `✓ · ⇢ ▸`; keep `! ✗ ℹ` | same, ASCII-only leading-whitespace strip (C-locale `[:space:]`) |
| verbose | `cat` the capture byte-exact (trailing blanks preserved) | `io.WriteString` the raw bytes |
| `RECIPE_NAMES` | `grep -oE '▸ recipe [^ ]+' \| sed 's/.*recipe //' \| paste -sd, -` then `,`→`, ` | regex `▸ recipe ([^ \n]+)` + join |
| `AI_SPECS_SYNC_NESTED` | exported before the fan-out loop | set in the child env from the fan-out onward |

### Re-point strategy for the three suites

The Bash-structural tests (extract `run_step`/`print_step_output` bodies, slice the
capture block out of the source) have no CLI observable once the spine is Go.
Each **contract assertion** is preserved and driven through the real CLI with
PATH stubs:

- a `python3` wrapper that intercepts one module (scripted output + rc),
- a `mktemp` wrapper that hands out known probe paths (recorded, arbitrary rc).

Tests whose subject is literally Bash source structure (function extraction,
`${VAR:-}` trap expansion, `|| return $?` counts, Bash-3.2 matrix) are retired
with the rationale recorded here; their observable intent is covered by the
CLI-boundary replacements plus the already-CLI tests in the same files.

### Re-point mapping (per assertion)

Shared new helpers: `tests/_go_cli.py` (build `cmd/ai-specs` once per process,
CGO_ENABLED=0; fall back to `bin/ai-specs` only with a loud one-line warning, so
a go-less machine still runs the suite but knows it is not gating the port) and
`tests/_sync_stub.py` (PATH shadows for `python3` and `mktemp`). Both spines mint
temps through the `mktemp` binary and run steps through `python3` from PATH, so a
PATH stub is an implementation-independent fault seam — one CLI test then covers
whichever spine is under test, and one test asserts that explicitly.

| Retired / mixed test | CLI-boundary replacement |
| --- | --- |
| `run_step_errexit`: errexit active after a successful step | later step's stub fails with rc 7 → CLI exits 7 and the next label never appears |
| `run_step_errexit`: bare failing step keeps the real status | stub rc 7 → CLI exits 7 (not 1) |
| `run_step_errexit`: full output on failure | stub rc 5 with stdout+stderr markers → both reprinted on their streams, both modes |
| `run_step_errexit`: no temp survivors (success/failure) | `mktemp` probe dir has zero `temp*` survivors |
| `run_step_errexit`: partial mktemp failure | `mktemp` fails from call 2 → step still runs, first temp removed |
| `run_step_errexit`: degraded path forwards status / names itself / says unfiltered | `mktemp` always fails → rc is the stub's, warning names temp/TMPDIR and says unfiltered, raw glyph line reaches the terminal |
| `recipe_capture`: rc passthrough, cleanup, no abort before cleanup | stub materialize rc 3 → CLI exits 3, output reprinted, zero survivors |
| `recipe_capture`: trap from the moment a temp exists | `mktemp` fails at call 4 → exit 1, zero survivors |
| `recipe_capture`: distinct capture paths | stub logs its argv; the five temps are distinct probe paths |
| `recipe_capture`: `RECIPE_NAMES` grep | stub prints `  ▸ recipe alpha`/`  ▸ recipe beta` → `  syncing recipes → alpha, beta`; no matches → bare `  syncing recipes` |
| `verbosity`: compact drops `✓ · ⇢ ▸` + blanks, keeps `! ✗ ℹ` | stub emits all glyphs → compact drops the four, keeps the notices, stream-separated |
| `verbosity`: verbose byte-exact incl. trailing blanks | stub emits `detail\n\n\n` → verbose stream contains `syncing ...\ndetail\n\n\n` |
| `verbosity`: failure prints full output both modes | stub rc 7 with glyphs → unfiltered in compact AND verbose |
| `verbosity`: `template skipped (exists)` stays noise | compact drops `· template skipped (exists)`, verbose keeps it (static half on `recipe-materialize.py` kept) |
| `verbosity`: inherit-errexit probe (failing substitution is not swallowed) | stub `python3` fails `target-resolve.py` → exit 1 + `ERROR: target resolution failed before any writes.`; fails `project-cache.py` in the fan-out → non-zero, no `complete` |
| `verbosity`: dot-marker audit of `sync*.sh` | covered by the already-CLI compact-leak tests; Go emits its own `·` only in classified places (audited in review) |
| `verbosity`: Bash-3.2 matrix leg, `|| return $?` counts, `${VAR:-}` trap expansion, function extraction, `capture_block()` slicing, "block still present" | retired: their subject is Bash source structure that stops being authoritative in S1; observable intent is covered above |

Already-CLI tests in the three files (fan-out termination, nested framing,
compact-leak, marker hygiene, verbose forwarding, symlink-refusal propagation)
keep their assertions and only switch their `CLI` to the Go binary, so they
exercise the native `sync` route instead of the Bash spine.

## Tasks

- [x] T1 — Verify worktree/branch/base and clean pre-write baseline.
- [x] T2 — Read the merged plan, the FROZEN contract, `lib/sync.sh`, the parity harness and the three coupled suites.
- [x] T3 — ODD feature doc + visible todo.
- [x] T4 — RED: Go unit tests pinning the spine contract (filter, flags, exit codes) + cli route test.
- [x] T5 — Implement `internal/sync` spine + `run_step`/`print_step_output`/`mktemp`.
- [x] T6 — Native `sync` route in `internal/cli` (minimal, self-contained diff).
- [x] T7 — Parity harness `gate-present` mode; run the corpus in both modes.
- [x] T8 — Re-point the three sync suites to the CLI boundary.
- [x] T9 — Full `./tests/run.sh` once (bash timeout 3600s) — exit 0.

### Review chunking (measured budget)

The native-review protocol skill's own measurements cap a candidate at roughly
600–750 changed lines (readability returns empty above a ~40 KB relayed prompt;
the repo's most recent data point, card [Go 06], was 732 lines / 43.8 KB with
4/4 lenses after one reroute). S1 is therefore committed and reviewed as FIVE
chained candidates, each with its own explicit base ref:

| # | Commit scope | Changed lines | Base ref |
| --- | --- | --- | --- |
| A | `internal/sync/sync.go` + `step.go` (the port) | 719 | epic tip `1c9806c` |
| B | Go unit tests + native `cli` route + parity `gate-present` | 548 | commit A |
| C | `tests/_go_cli.py`, `tests/_sync_stub.py`, errexit suite | 694 | commit B |
| D | capture suite + this document | 485 | commit C |
| E | verbosity suite re-point | 683 | commit D |

The chunking does not change S1's scope or its evidence: every candidate is a
prefix of the same verified tree, and the full-suite run above measured the
final content.

## Evidence

### S1a — spine + route + parity mode (commits `9976672`, `98e27a1`)

| Command | Result | Exit |
| --- | --- | --- |
| `gofmt -l .` | no output | 0 |
| `go vet ./...` | no output | 0 |
| `go test ./...` | `ok` for `internal/{cli,config,home,lock,schema,sync,target,toml}` + root pkg; `cmd/ai-specs` no test files | 0 |
| `go test ./internal/sync -v` | 11 PASS (5 flag/DASH tests, `TestStepModeDefaultsToPython`, `TestSyncCDLineMatchesShellSource`, 4 `printStepOutput` tests) | 0 |
| `go test ./internal/cli -run TestDifferentialSmoke` | `ok` — 13 subtests incl. `sync-missing-path` byte-for-byte vs `bash bin/ai-specs` | 0 |
| `GO_SYNC_STEP_GITIGNORE=python python3 tests/parity/run.py` | 8/8 fixtures `zero deltas` in **gate-absent AND gate-present**; `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`; real 1m6s | 0 |
| `python3 -m unittest discover -s tests -p 'test_parity_harness.py'` | Ran 23 tests — OK | 0 |
| legacy vs Go, failing `cd` (`sync <missing>`), `AI_SPECS_HOME=<root>` | stdout empty, stderr `<root>/lib/sync.sh: line 76: cd: <path>: No such file or directory`, then byte-identical Go line | 1 / 1 |
| legacy vs Go, full pipeline on two scratch copies | normalized stdout, stderr and 43-entry tree manifest (mode + sha256/symlink target) IDENTICAL | 0 / 0 |

F3 confirmation (measured by the parity work, uncommitted diagnostic): the legacy
leg emits **62** `*_BRIDGE_FALLBACK` lines in `gate-absent` and **0** in
`gate-present`, so the pin genuinely switches the bridges to Go authority — the
zero-delta result is not a no-op.

### S1b — re-pointed suites

| Command | Result | Exit |
| --- | --- | --- |
| `python3 -m py_compile tests/_go_cli.py tests/_sync_stub.py tests/test_sync_{output_verbosity,run_step_errexit,recipe_capture}.py` | no output | 0 |
| `python3 -m unittest tests.test_sync_run_step_errexit tests.test_sync_recipe_capture tests.test_sync_output_verbosity` | `Ran 41 tests in 93.8s — OK` (9 errexit, 10 capture, 22 verbosity) | 0 |
| `python3 -m unittest discover -s tests -p 'test_sync_run_step_errexit.py'` | OK (the new sibling imports work under `tests/run.sh`'s discover mode too) | 0 |

The three suites now drive `cmd/ai-specs` (built once per process by
`tests/_go_cli.py`), never `bash lib/sync.sh`.

### Full suite — card S1 gate

| Command | Result | Exit |
| --- | --- | --- |
| `./tests/run.sh` (bash timeout 3600 s, log `/tmp/go07-s1-full-run.log`) | phase 2 gate `go test` → `ok ai-specs.dev/worktree-gate 51.766s`, `ok …/ledger 2.975s`; phase 3 root `go test ./cmd/... ./internal/...` → `ok` ×8 incl. `internal/sync`; phase 4 parity → `fixtures: 8, failing: 0` twice, `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`; phase 5 `Ran 2378 tests in 2144.776s` → `OK (skipped=164)`, zero `FAIL:`/`ERROR:` lines | 0 |

## Findings (measured, not assumed)

**F-A — macOS `mktemp` ignores `TMPDIR`, and `-t` does not substitute `XXXXXX`.**
Measured in this worktree:

```
TMPDIR=/nonexistent mktemp                     -> /var/folders/…/tmp.RANDOM   (exit 0)
TMPDIR=/nonexistent mktemp -t ai-specs-x-XXXXXX.json
                                               -> /var/folders/…/ai-specs-x-XXXXXX.json.RANDOM
```

Consequences, both of which the plan did not anticipate:
1. `os.CreateTemp` is **not** a valid port of the shell's `mktemp` (it honours
   `TMPDIR` and substitutes `*`), so the spine execs the `mktemp` binary. That
   also makes PATH-level fault injection identical for the Bash and Go legs of
   the re-pointed suites.
2. Risk R3's "unwritable `TMPDIR`" fault-injection fixture **cannot be built by
   pointing `TMPDIR` at a bad path on macOS** — the legacy spine never degrades
   there, because its temps never touch `TMPDIR`. The degradation path is only
   reachable by shadowing `mktemp` itself, which is why the re-pointed tests use
   a PATH stub rather than a bad `TMPDIR`.

**F-B — the Go differential smoke test pins a bash `cd` diagnostic the plan's S1
gate never mentions.** `internal/cli/smoke_test.go` case `sync-missing-path`
compares the Go binary with `bash bin/ai-specs` byte-for-byte for a missing
target. Bash prints `<home>/lib/sync.sh: line 76: cd: <path>: <strerror>`.
The Go spine therefore reproduces that line (errno → capitalised strerror) and a
Go test pins the constant to `lib/sync.sh`'s actual line number, so shell-source
drift fails the Go test instead of silently breaking the smoke test.

**F-C — `AI_SPECS_HOME` is exported by the launcher but not by the shell
script.** `bin/ai-specs` exports it; `bash lib/sync.sh` derives it privately, so
its Python children do not see it. The Go spine pins `AI_SPECS_HOME=<home>` for
every child (the launcher/production behavior), which is what the parity harness
and the re-pointed suites observe.

**F-D (process) — this harness refuses a second direct `edit`/`write` outside
`odd/tasks/**` once one path has been touched (ODD delegation gate).** Per the
repo's established practice the file writes were delegated to `gentle-ai-worker`
tasks with explicit `## Allowed edit surfaces`; design, RED contracts,
verification, commits and native review stayed with the card owner. No write was
routed around the gate via a shell heredoc.

**F-E — `GO_SYNC_STEP_<NAME>` is validated eagerly.** S1 has no Go step, so any
value other than `python` aborts with exit 1 and a named error instead of
silently running Python. Default and `python` are both accepted.

**F-F — the re-point needs `AI_SPECS_HOME` pinned, because the Go binary is
built outside the checkout.** The legacy launcher derives its install root from
its own path, so the previous suites could omit `AI_SPECS_HOME`. The Go binary
is built into a temp dir, so `_sync_env()` now pins `AI_SPECS_HOME=<repo>` (and
`init_workspace` uses that env); behaviour for the legacy leg is unchanged
because it already resolved to the same root. `test_t2_7` needed the same pin.

**F-G — the isolated-harness stream-separation assertion had to be narrowed.**
The retired Bash harness could assert "no `✗`/`ℹ` anywhere on stdout" because it
ran one function in isolation. Through the real CLI, downstream steps legitimately
emit their own `ℹ mcp skipped …` on stdout, so the replacement asserts stream
separation for the *stub's own* lines (the `✗` notice stays on stderr) while
keeping the global glyph assertions on the stub's output. The original isolated
assertion has no CLI equivalent and this is a deliberate, disclosed narrowing.

**F-H — the plan's "fail at the 4th mktemp" trap test does not reach the recipe
block through the whole CLI.** The pre-recipe `run_step` calls mint temps first,
so a literal `mktemp_fail_from=4` exercises the `run_step` partial-failure guard.
The genuine "cleanup registered only after the third `-t` temp" intent is covered
by a second test whose boundary is derived from a control run, so the failure
lands exactly on the first recipe capture file.

## What remains

1. The five chained native-review candidates (A–E) must each be closed
   (`approved` + `acknowledge-approved` burn); their lineage ids are reported
   in the card's `worker_done` message rather than here, because this document
   is frozen with candidate D.
2. Not in this slice, by design: no step is ported (`GO_SYNC_STEP_*` only
   accepts `python`); `sync-agent` stays a shim; the Python/Bash modules are all
   still present (Q5); `--refresh-gates` is only forwarded, not implemented;
   the two stdout couplings (`▸ recipe`, `RECIPE_MCP_TEMP:`) stay FROZEN until
   S16 (Q7).
3. The zero-`TMPDIR` fault-injection fixture the plan's R3 asks for is not
   reachable on macOS (finding F-A); if a later slice wants it, it must be
   built on the `mktemp` PATH seam the re-pointed suites now use.
4. `git status` must stay free of provisioning output: `ai-specs/.ai-specs.lock`
   was restamped once by an accidental repo-targeted differential run and was
   restored from HEAD, not committed.
