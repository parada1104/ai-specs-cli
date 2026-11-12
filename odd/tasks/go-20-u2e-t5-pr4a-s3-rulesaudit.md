# GO-20 U2e T5 PR4a S3 — rulesaudit live oracle

Base: epic/go-single-binary b86ff01. Branch: change/go-20-u2e-t5-pr4a-s3-rulesaudit.
Goal: `internal/rulesaudit/legacy_differential_test.go` stops exec'ing python3 and reading `lib/_internal/rules-inventory.py`, without silent coverage loss.

## Specs

- S1: `TestDifferentialRulesInventory` reads a frozen golden (`internal/rulesaudit/testdata/rules_inventory_golden.json`) captured once from `lib/_internal/rules-inventory.py` `main` over the same 11 fabricated trees; no python3, no `lib/` access, no skip. The 11 case names are pinned by a hard-coded inventory (count, uniqueness, order) asserted before comparing, and a missing golden fails.
- S2: Every legacy case is frozen, not retired: the only host-dependent field in the payload is `sources.target` (the resolved scanned root), so the capture records the payload with the scanned root/home/cache directories tokenized as `<ROOT>` / `<HOME>` / `<CACHE>`. Per-case exit code, stdout, and stderr are all frozen and all asserted (the live test compared stderr only for the two warning/error cases; the capture proved both streams already agree on the other ten, so the frozen test pins all three without a per-case skip switch).
- S3: The frozen inputs equal the in-test corpus: each case stores a `tree` manifest (`sha256  <tokenized path>`, sorted by path) of the tree the case builds under `<ROOT>`/`<HOME>`, asserted before any comparison, so input drift fails as input drift rather than as an opaque payload mismatch.
- S4: Negative controls: mutating one frozen result, deleting a case, duplicating a case, and drifting a build-func input each fail rc=1.
- S5: `gofmt -l internal/rulesaudit` empty, `go vet ./internal/rulesaudit/`, focused tests, `go build ./cmd/ai-specs` and full `tests/validate.sh` green at the exact head.

## Per-test decision

| legacy case | decision | reason |
|---|---|---|
| empty | FREEZE | payload is `mode`/relative paths only; `target` tokenized |
| cursor-agents-manifest | FREEZE | same; all paths are project-relative |
| manifest-widened-bigint | FREEZE | BigInt renders in full through json.dumps; no host data |
| manifest-float-widened | FREEZE | Python float repr / Infinity / NaN are host-independent |
| manifest-datetime | FREEZE | exit 1 + `ERROR: Object of type datetime is not JSON serializable`; no host data |
| mode-b-stack-hints | FREEZE | stack hints are lockfile names |
| manifest-parse-error | FREEZE | parse_error flag only |
| recipes-list-and-monolithic-agents | FREEZE | recipe ids only |
| agents-heading-depths | FREEZE | headings/bodies from the tree |
| mdc-scalar-globs-and-snake-always | FREEZE | globs/always_apply from the tree |
| skills-cache-atl | FREEZE | resolved skills emit `id`+`source` only (never the cache path); the duplicate-recipe warning names skill/recipe ids, not paths |

No RETARGET and no RETIRE: unlike doctor's `cache_key` (which hashes the absolute resolved TempDir), nothing in this oracle depends on a host path once `target` is tokenized, so freezing keeps full payload coverage. 11 cases: `empty`, `cursor-agents-manifest`, `manifest-widened-bigint`, `manifest-float-widened`, `manifest-datetime`, `mode-b-stack-hints`, `manifest-parse-error`, `recipes-list-and-monolithic-agents`, `agents-heading-depths`, `mdc-scalar-globs-and-snake-always`, `skills-cache-atl`.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4 | inline | |
| T2 | S5 | inline | |

## Log

- L1 (Robert): "si, avancemos con las entregas" — authorizing continuing the PR4a S3 deliveries with the rulesaudit slice.
- L2: Baseline at b86ff01: live python differential green for all 12 cases (`logs/00-baseline-frozen-legacy.log`, rc=0).
- L3: Finding: the frozen oracle is a snapshot; regenerating it needs a human decision (same rule as the doctor cliversion golden).
- L4: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s3-rulesaudit (Robert's standing scratch convention).
- L5: Capture harness was a temporary `internal/rulesaudit/zz_scratch_capture_test.go` (python3 exec, deleted before commit); it cross-checked the live port against the legacy run before freezing, so the golden is legacy output, not port output.
- L6: Captured twice, `cmp` byte-identical (rc=0), oracle metadata `python: 3.14.7`. `json.Encoder.SetEscapeHTML(false)` was needed so the tokens stay readable (`<ROOT>` not `\u003cROOT\u003e`).
- L7: Capture showed port and legacy stderr already agree on all 11 cases, so the frozen test asserts exit+stdout+stderr for every case; the legacy `compareErr` flag survives only as a corpus-vs-golden consistency assertion.
- L8: Negative controls (each rc=1, golden/corpus restored and `cmp`-verified after each): mutate one result → only that case fails; delete a case → count 10 vs 11; duplicate a case → count 12 vs 11; input drift in a build func → tree manifest mismatch.
- L9: `gofmt -l internal/rulesaudit` empty, `go vet ./internal/rulesaudit/` rc=0, `grep python3|exec.Command|lib/_internal` on the test file matches one comment line only.
