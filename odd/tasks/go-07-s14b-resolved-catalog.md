# Go 07 S14b — Catalog defaults and brief fragments in the native resolved-config

Second unit of the S14 series. Base `7f19c65` on `epic/go-single-binary` (S14a complete). Worktree
`.worktrees/go-07-s14b-resolved-catalog`, branch `change/go-07-s14b-resolved-catalog`. Ships unwired, like S14a.

## Specs
- S1 — Session request (verbatim): "entonces con los slices que queden péndintes para finalizar la migración a go".
- S2 — The native resolved-config document written by `MaterializeRecipes` (both the no-enabled branch and
  the main dump) is byte-identical to Python's `json.dump(resolved, f, indent=2, sort_keys=True)` + `"\n"`,
  including `merge_catalog_defaults_into_resolved` and `attach_brief_fragments_to_resolved`
  (`lib/_internal/recipe-materialize.py:198-291`, called at :4123-4124 and :4194-4195). Python only enriches
  the main dump, not the no-enabled branch (:3900-3905): the native writer must do the same.
- S3 — Catalog defaults: for every enabled recipe whose catalog `recipe.toml` loads, each `[config]` field
  with a non-None default is added when the key is absent; manifest values win. A recipe that fails to load
  is skipped silently (best-effort, like the Python `except Exception: pass`).
- S4 — Brief fragments: every enabled recipe that loads gets `brief_fragments` = the `[provides.brief]`
  sections it declares, each fragment `{"key": <key|null>, "text": <text>}`, `{}` when it declares none;
  applied after the defaults.
- S5 — Value fidelity at that writer, as the oracle actually behaves: the projected manifest values keep the Go gate envelope's
  rendering (both legs decode that envelope, so a manifest `1.0` is `1` on both), while catalog defaults and
  brief fragments come from Python's own `tomllib` read and keep its types (a default `2.0` stays `2.0`, a wide
  integer keeps its digits); a catalog value `json.dumps` cannot serialize fails with Python's `TypeError`
  (`Object of type datetime is not JSON serializable`).
- S6 — The per-recipe loop differential stops excluding the enabled resolved-config document
  (`internal/sync/materialize_loop_test.go:301-305`); the spine differential gains scenarios for S3-S5.

## Tasks
- T1 — S2-S6: ordered resolved-config writer + catalog enrichment + differential scenarios. Route: inline,
  test-first. Commit: `c312f0f`.

## Close-out
- Work-unit commit `c312f0f` (code, differentials and this document), branch
  `change/go-07-s14b-resolved-catalog`, base `7f19c65`.
- Native assess over the committed range `7f19c65..c312f0f` (6 paths, 243 changed lines, writer profile
  `large` from runtime evidence): `risk: medium`, `reviewDue: false` (`under_budget`), plan
  `writerSelfVerification: true` / `independentVerifier: false`. No native review transaction was started,
  per the entry rule; the differentials above are the candidate's own verification.

## Evidence
Run from this worktree at base `7f19c65`, with the real toolchains first on `PATH`
(`python 3.14.7`, `go 1.24.13`) and a gate binary built by `scripts/build-gate.sh`:

| Command | Result | Exit |
| --- | --- | --- |
| `go test ./internal/sync -run TestMaterializeSpineDifferential -count=1` | RED first: 2 of the added scenarios fail (no defaults, no `brief_fragments`, wrong float text); GREEN after the port | 0 (GREEN) |
| `go test ./internal/sync -run TestMaterializeLoopDifferential -count=1` | 11 scenarios ok with the enabled resolved-config document no longer excluded | 0 |
| `go test ./internal/sync -count=1` | whole package ok | 0 |
| `go test ./cmd/... ./internal/... -count=1` | every package ok (log `~/.cache/s14b-gotest.log`) | 0 |
| `bash scripts/build-gate.sh` | gate binary built (log `~/.cache/s14b-build.log`) | 0 |
| `python3 -m unittest tests.test_recipe_materialize tests.test_sync_pipeline tests.test_resolved_config_bridge tests.test_materialize_bridge` | `Ran 194 tests` → `OK` (log `~/.cache/s14b-python-bridge.log`) | 0 |
| `gofmt -l internal/sync`, `go vet ./internal/sync`, `git diff --check` | no output | 0 |

The gate module and `catalog/recipes/worktree-flow/bin/SHA256SUMS` are untouched, so no digest regeneration
is part of this unit.

## Size
Disclosed: `resolvedcatalog.go` 104 lines plus 19 in `materialize_glue.go` and 1 in `materialize.go`; 46 lines
of new differential scenarios and 4 removed exclusion lines. About 175 authored lines, inside the plan's
~700-line slice cap.
## Log
- L1 — "ok vamos con el follow up" → clarified: "entonces con los slices que queden péndintes para finalizar
  la migración a go".
- L2 — Mapping (explore): S15 is already native; remaining are S14b, S14c (wire the three
  `recipe-materialize.py` exec sites, add the `--resolved-config-only` mode) and S16 (cleanup, blocked in
  part by the doctor brief bridge `internal/doctor/bridge.go:15`).
- L3 — Pre-existing defect found while planning: `materializeResolvedConfigDocument` round-trips through
  `encoding/json`, so a manifest float `1.0` renders `1` (Python `1.0`). S5 fixes it at this writer.
- L4 — L3 was WRONG and the differential proved it: Python decodes the Go gate's resolved-config envelope
  (`go_resolved_config`, `recipe-materialize.py:3835-3842`), so a manifest `1.0` is `1` on both legs. The
  round trip stays; S5 now states the real rule (envelope rendering for manifest values, `tomllib` types for
  catalog values). Enrichment therefore runs on the ordered document AFTER the round trip.
- L5 — RED observed before any implementation, on the added scenarios: `resolved-config carries catalog
  defaults and brief fragments` failed (Go wrote no defaults and no `brief_fragments`) and
  `no-enabled resolved-config is not enriched` failed on the float rendering; GREEN after
  `internal/sync/resolvedcatalog.go` + the enriched writer at the main dump. The loop differential's
  exclusion of the enabled resolved-config document is removed and the whole loop table stays green.