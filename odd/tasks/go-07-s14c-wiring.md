# Go 07 S14c — Wire the native materializer into the three call sites

Wiring unit of the S14 series. Base `8c09329` on `epic/go-single-binary` (S14b merged). Worktree
`.worktrees/go-07-s14c-wiring`, branch `change/go-07-s14c-wiring`. The core already exists (S14a/S14b,
unwired); this unit removes the three `python3 lib/_internal/recipe-materialize.py` execs from the pipeline.

## Specs
- S1 — Session request (verbatim): "entonces con los slices que queden péndintes para finalizar la migración
  a go".
- S2 — The three exec sites become in-process native calls, with no Python sibling and no strangler flag
  (`odd/tasks/go-07-s14-divergences.md:36-37`; S15 precedent, `internal/cli/cli.go:97`):
  1. `internal/sync/sync.go:330-367` — the spine capture block with the three temp paths and
     `--refresh-gates`.
  2. `internal/sync/syncagent.go:255-261` — standalone fan-out, `--resolved-config-out <temp>
     --resolved-config-only`, merged onto the script's stdout, rc ignored but a non-zero rc printed as
     `WARNING: resolved-config generation failed; subrepo AGENTS.md will be rendered without structured fields.`
  3. `internal/sync/syncagent.go:380-387` — per-target fallback with no output paths, stdout+stderr merged
     into one temp file, marker `RECIPE_MCP_TEMP:` parsed, `rc != 0` →
     `ERROR: recipe materialize failed` + the temp replayed minus the marker line, then rc 1.
- S3 — Every observable byte of those sites is unchanged: the capture files, the replay rules
  (`replayRaw` on failure, `printStepOutput` filtering on success), the `RECIPE_NAMES` extraction from
  `▸ recipe <id>` stdout, the rc pass-through, the `RECIPE_MCP_TEMP:` marker line (frozen until S16,
  `docs/go-migration-parity-contract.md:204-209`), the temp-file cleanup and the merged-descriptor write
  order.
- S4 — `--resolved-config-only` gains a native mode with Python's exact semantics
  (`lib/_internal/recipe-materialize.py:4143-4205`): no cache ensure, no legacy-origin cleanup, no
  per-recipe loop, no hooks, no lock writes, no orphan cleanup; auto-bindings through the same native
  authority with the caller's home; a manifest-validation failure prints
  `ERROR: binding validation failed: <msg>` on stderr and returns 1; any other acquisition failure degrades
  silently to explicit-only bindings; the document is written with the S14b enrichment; any outer failure
  prints `WARNING: resolved-config generation failed: <msg>` and returns 1. Nothing is printed on success.
- S5 — The Python-coupled spine tests observe the native step: `tests/test_sync_recipe_capture.py` (its
  `python3` stub no longer serves the step) is re-pointed, and the sync parity fixtures
  (`all-recipes-enabled`, `idempotent-resync`, `refresh-gates`, the sync-agent matrix) stay green in both
  gate modes.
- S6 — The gate module and `catalog/recipes/worktree-flow/bin/SHA256SUMS` are untouched; no Python module is
  deleted and no marker is collapsed (S16 owns both).

## Tasks
- T1 — S4: native `--resolved-config-only` mode + differential. Route: inline, test-first. Commit: pending.
- T2 — S2/S3: wire the two sync-agent sites. Route: inline, test-first. Commit: pending.
- T3 — S2/S3: wire the spine capture block. Route: inline, test-first. Commit: pending.
- T4 — S5: re-point the Python-coupled tests and run the parity fixtures. Route: inline. Commit: pending.
## Progress
- T1 done: `internal/sync/resolvedconfigonly.go` (`BuildResolvedConfigOnly`, 4 differential scenarios in
  `resolvedconfigonly_test.go` against a new `testdata/resolvedconfigonly_ref.py`). RED was the new test
  failing to build; GREEN is the four scenarios byte-identical on rc, both streams and the document.
- T2 done, T3 done: the three execs are gone (`sync.go`, `syncagent.go`), the two dead runner helpers
  (`execMerged`/`execToFileMerged`) are replaced by `materializeMerged`, and `TestStandaloneMaterializeRunsNatively`
  records every `python3` invocation through a delegating stub: RED showed `recipe-materialize.py` in the log
  for both the single-target body and the standalone fan-out, GREEN shows it never appears while
  `target-resolve.py` still does.
- T4 done: `tests/test_sync_recipe_capture.py` is re-driven against the native step (the fixtures enable real
  catalog recipes; the failure fixture trips the internal-test guard, whose bytes are identical on both
  authorities), and `tests/_sync_stub.py` gained `mktemp_calls()` so a specific temp is located by call
  number instead of by the removed argv. `tests/test_sync_output_verbosity.py` needed no edit: its two
  failures were the missing gate binary, and `scripts/build-gate.sh` turns them green.
- `docs/go-migration-parity-contract.md` updated where the wiring made it false: the materializer's rc is no
  longer passed through from a child, and the `RECIPE_NAMES` comment no longer names a Python script.
- **Parity harness normalization extended (the only harness change).** Retiring the Python route stopped the
  Go leg from emitting the legacy route's degradation notices, so six gate-absent fixtures reported deltas that
  are legacy-only by construction:
  - `N7-legacy-bridge-fallback-notice` now also applies to `stdout`, because `sync-agent` runs its
    resolved-config child with both streams on one descriptor, so `GO_RESOLVED_CONFIG_BRIDGE_FALLBACK` lands on
    stdout there (`sync-agent-standalone`, `nested-fanout-suppression`).
  - `N8-legacy-hook-bridge-reconcile-notice` (new) removes only the legacy hook-bridge reconciliation lines
    (`  ! the Go bridge produced no usable output; reconciled the lock with the on-disk gate bytes` and its
    `wrote without a record` / `…not proven CLI-rendered…` siblings, `lib/_internal/recipe-materialize.py:2041-2085`),
    which the native hook materializer cannot print because it reconciles nothing against a bridge envelope
    (`all-recipes-enabled`, `cache-layout`, `hooks-five-runtimes`, `doctor-healthy`).
  Both rules are anchored to exact prefixes, gate-present stays untouched, and every other warning, exit code and
  report line is still compared verbatim. Evidence: `~/.cache/s14c-validate.log` (the six deltas) and
  `~/.cache/s14c-parity.log`.

## Evidence
Run from this worktree at base `8c09329`, with the real toolchains first on `PATH` (`python 3.14.7`,
`go 1.24.13`) and a gate binary built by `scripts/build-gate.sh` (the gate module and its
`bin/SHA256SUMS` are untouched):

| Command | Result | Exit |
| --- | --- | --- |
| `go test ./internal/sync -run TestResolvedConfigOnlyDifferential -count=1` | RED: `undefined: BuildResolvedConfigOnly`; GREEN: 4 scenarios byte-identical | 0 (GREEN) |
| `go test ./internal/sync -run TestStandaloneMaterializeRunsNatively -count=1` | RED: the stub log showed `recipe-materialize.py` twice; GREEN: it never appears (log `~/.cache/s14c-coupled.log`) | 0 (GREEN) |
| `go test ./cmd/... ./internal/... -count=1` | every package ok (log `~/.cache/s14c-gotest.log`) | 0 |
| `python3 -m unittest tests.test_sync_recipe_capture` | `Ran 10 tests` → `OK` (re-pointed to the native step) | 0 |
| `python3 -m unittest tests.test_sync_output_verbosity` | `Ran 22 tests` → `OK` (the earlier 2 failures were the missing gate binary) | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; `gate-absent failing=0, gate-present failing=0` (log `~/.cache/s14c-parity.log`) | 0 |
| `./tests/validate.sh` | exit 0 · `fixtures: 29, failing: 0` · `parity summary: gate-absent failing=0, gate-present failing=0 — PASS` · `Ran 2482 tests in 1691.938s` → `OK` (log `~/.cache/s14c-validate2.log`, exit `~/.cache/s14c-validate2.exit`) | 0 |

The first `validate.sh` run (log `~/.cache/s14c-validate.log`) is the evidence for the six gate-absent deltas
and for the two parity normalizations recorded under Progress.

## Size
Disclosed: `resolvedconfigonly.go` (about 120) + its differential and driver (about 190), the sync-agent
stub test (about 140), the three call-site edits (about 70), the re-pointed Python contract suite (about 200),
the two parity normalization rules with their justifications (about 60), the contract-doc corrections (6) and
this document. Roughly 800 authored lines, above the plan's ~700 soft cap; the natural split, if the reviewer
wants one, is the parity normalization plus the re-pointed Python suite as their own follow-up commit.

## Follow-ups
- **Fatal-message divergence on a missing catalog recipe (registered, not fixed).** A manifest enabling a
  recipe absent from the catalog now reports `  ✗ the manifest is not valid TOML (parser exit status 1)`
  (the native primitive-conflict reader's error) where the Python authority reported
  `  ! GO_PRIMITIVE_CONFLICTS_BRIDGE_FALLBACK: …` plus `  ✗ recipe directory not found: <catalog>/<id>`. The
  exit status (1) and the fatal `  ✗ ` shape are unchanged; only the diagnostic text differs, and the native
  root has no bridge to fall back from, so reproducing the Python warning verbatim is not obviously right.
  Evidence: `~/.cache/s14c-ghost-native.txt` (native) versus the Python authority on the same manifest.
- The S14a note "the wiring unit should revisit whether the differential should also run without the
  `WORKTREE_GATE_BIN` pin" is NOT done here: the Python leg of every differential still pins the gate binary
  so its bridges never announce a fallback that is not part of the orchestration under test. Registered for
  the loop/differential owner.

## Log
- L1 — "ok vamos con el follow up" → "entonces con los slices que queden péndintes para finalizar la
  migración a go".
- L2 — Recon: the three exec sites are `sync.go:330-367`, `syncagent.go:255-261` and
  `syncagent.go:380-387`; `MaterializeRequest` has no resolved-config-only mode; the S14a follow-up about
  re-running the loop differential without the `WORKTREE_GATE_BIN` pin belongs to this unit.
- L3 — `gentle_review assess` for S14b needed an explicit `workspaceRoot`: without it the facade resolved
  the main checkout and scored 498 paths instead of 6.
