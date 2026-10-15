# Go 07 S16 — Collapse the inter-module stdout couplings and amend the contract

Closing unit of the S14 series. Base `c1f6a58` on `epic/go-single-binary` (S14c merged). Worktree
`.worktrees/go-07-s16-cleanup`, branch `change/go-07-s16-cleanup`.

## Specs
- S1 — Session request (verbatim): "entonces con los slices que queden péndintes para finalizar la migración
  a go"; the human then authorized this unit's publication and left the session: "ok te dejo para que termines
  la migración yo me voy a dormir quedas a cargo".
- S2 — The Go pipeline stops reading its own human-readable stdout. `sync` takes the recipe names from the
  materializer's return value instead of grepping `▸ recipe` out of the capture file (today
  `internal/sync/sync.go:170`, `:355`), and `sync-agent` takes the recipe-MCP temp path from the result
  instead of grepping `RECIPE_MCP_TEMP:` out of the merged capture (today `internal/sync/syncagent.go:393`,
  `:1485`). Both printed lines stay byte-identical, because the coexisting shell route
  (`lib/sync.sh:256`, `lib/sync-agent.sh:198-201`) still greps them.
- S3 — The names the spine prints are exactly the recipes that actually reached their `▸ recipe` line: a
  failure after the first recipe prints still reports only that first recipe (Python's grep over the partial
  stdout), never the full enabled set.
- S4 — No output changes: the whole differential and parity corpus stays byte-identical, and the two grep
  helpers plus their tests are retired rather than kept dead.
- S5 — `docs/go-migration-parity-contract.md` is amended where the collapse makes its text stale: the
  "modules pass data by grepping human-readable stdout" finding, the §5 body that calls the print format the
  API of another module, and the §12 record of the coupling.
- S6 — No Python or bash module is deleted and no marker is removed from the output: the reconnaissance
  proved nothing is dead yet (`bin/ai-specs:40-41` still dispatches to `lib/sync.sh`/`lib/sync-agent.sh`,
  `lib/init.sh:242` and `lib/skills-add.sh:208` still exec the materializer, `lib/_internal/doctor.py:493,608`
  and `internal/doctor/bridge.py:84,99` still embed it, and the parity harness's legacy leg still runs
  `bin/ai-specs`). Deletion belongs to the cutover/doctor cards and is recorded under Deferred.

## Tasks
- T1 — S2/S3: collapse the two couplings + pin the partial-output names semantic. Route: inline. Commit: pending.
- T2 — S4: retire the grep helpers and their tests; re-run the corpus. Route: inline. Commit: pending.
- T3 — S5: amend the parity contract. Route: inline. Commit: pending.

## Log
- L1 — "entonces con los slices que queden péndintes para finalizar la migración a go" → the human authorized
  publishing this unit with the same pattern as S14b/S14c ("publish_all") and left the session in my charge.
- L2 — Reconnaissance (read-only subagent) established: (a) both couplings have exactly the consumers named
  above; (b) every Python/bash module still has a live caller, so "delete dead Python" cannot be done in this
  slice without the cutover, the doctor port, the init port and a parity leg replacement; (c) the doctor's
  needs are already ported natively (`internal/sync/resolvedconfig.go:40`, `resolvedcatalog.go:68`,
  `bindings.go:54`, `agentsrender.go:1198`) but unexported, so the doctor card only needs a small wrapper.
- L3 — Decision (evidence-backed, recorded not hidden): this unit is the collapse + contract amendment, and
  the module deletion is deferred with the four blockers named in S6. The plan's own rule is "S16 deletes only
  what is provably dead" (`odd/tasks/go-07-sync-plan.md:648-652`).

## Progress
- T1 done, RED then GREEN: `MaterializeResult.PrintedRecipeIDs` is appended at the `  ▸ recipe <id>`
  print site (`materialize.go:264`) and the spine prints its `  syncing recipes → …` line from it
  (`sync.go`), replacing the `recipeNameRe` grep over the capture file. RED was
  `materialize_names_test.go` failing to build (`result.PrintedRecipeIDs undefined`); GREEN is the new pin test
  plus the whole `internal/sync` package. The pin test asserts the partial-output semantic of S3: a fixture
  that fails before the loop prints has one enabled recipe and zero printed ones.
- T2 done: `sync-agent`'s per-target fallback takes the recipe-MCP temp path from
  `MaterializeResult.RecipeMCPPath` (`materializeMerged` now returns it), so `markerTempPath` and its oracle
  test are deleted; `replayExceptMarker` stays because the failure replay still prints the captured stream.
  `regexp` left `sync.go` with `recipeNameRe`.
- T3 done: `docs/go-migration-parity-contract.md` amended at the three places the collapse makes stale — the
  go/no-go finding 2, the §5 body (which now records the exact fields and why the printed lines stay frozen),
  and the §12 FROZEN list (the two couplings are now named there, with the freeze scoped to the shell route).

## Evidence
Run from this worktree at base `c1f6a58`, with the real toolchains first on `PATH` (`python 3.14.7`,
`go 1.24.13`):

| Command | Result | Exit |
| --- | --- | --- |
| `go test ./internal/sync -run TestMaterializePrintedNamesAreEmptyBeforeAnyPrint -count=1` | RED: `result.PrintedRecipeIDs undefined`; GREEN after the field exists | 0 (GREEN) |
| `go test ./internal/sync -count=1` | whole package ok | 0 |
| `go test ./cmd/... ./internal/... -count=1` | every package ok (log `~/.cache/s16-gotest.log`) | 0 |
| `bash scripts/build-gate.sh` | gate binary built (log `~/.cache/s16-build.log`) | 0 |
| `AI_SPECS_GATE_OFFLINE=1 python3 -m unittest tests.test_sync_recipe_capture tests.test_sync_output_verbosity` | `Ran 32 tests` → `OK` minutes after the same command without the variable failed 4 tests | 0 |
| `python3 tests/parity/run.py` | `fixtures: 29, failing: 0`; `gate-absent failing=0, gate-present failing=0` (log `~/.cache/s16-parity.log`) | 0 |
| `./tests/validate.sh` | exit 0 · parity both modes PASS · `Ran 2482 tests in 1594.229s` → `OK` (log `~/.cache/s16-validate.log`) | 0 |

Environment note, measured here: `tests/run.sh:8-9` exports `AI_SPECS_GATE_OFFLINE=1` for the whole
suite, and without it the two suites above report the legacy bridge degradation notices as unexpected
stderr (4 failures) even with the gate binary present. It is a suite prerequisite, not a behaviour of this
change: `validate.sh`, which exports it, is green.

## Size
Disclosed: the collapse is about 40 authored lines of product code (`MaterializeResult.PrintedRecipeIDs` and
its append, the spine's names branch, `materializeMerged`'s new return, the two deleted helpers), about 30 of
test changes (one new pin, one deleted oracle table) and about 25 of contract amendments, plus this document.
Well inside the plan's ~700-line cap.

## Deferred (not this unit, with evidence)

- `lib/sync.sh` / `lib/sync-agent.sh`: need the `bin/ai-specs` dispatcher cutover (`bin/ai-specs:40-41`) and a
  replacement for `lib/skills-add.sh:208`'s auto-sync.
- `lib/_internal/recipe-materialize.py`: also needs the doctor brief ops ported onto the native chain
  (`internal/doctor/bridge.py:60-101` calls `build_resolved_config`, the S14b enrichers, `resolve_bindings`,
  `agents-render._render_lines`/`brief_effective_state` and `brief-render-policy.has_dead_recipe_fragments`),
  `lib/init.sh:242-245` replaced, and the parity legacy leg retired or re-based (`tests/parity/parity.py:1371`).
- `lib/_internal/target-resolve.py`: needs a native resolver (`internal/sync/syncagent.go:225` still execs it).
  **Closed by S17** (`odd/tasks/go-07-s17-target-resolve.md`): both Go call sites — the spine
  (`internal/sync/sync.go:433`) and the standalone agent plan (`internal/sync/syncagent.go:225`) —
  now run `internal/target` in process, so no Go path execs the module. The shell route still does,
  until the dispatcher cutover.
- `gitignore-root-refresh.py`, `mcp-render.py`, `hooks-render.py`, `flatten-resolved-skills.py`: each has
  exactly one exec caller inside `lib/sync.sh`/`lib/sync-agent.sh`, so they fall with those scripts.
