# GO-20 U2e T5 PR4a S3 — conflicts live oracle

Base: epic/go-single-binary ac59595. Branch: change/go-20-u2e-t5-pr4a-s3-conflicts.
Goal: `internal/conflicts/conflicts_test.go` stops exec'ing python3 and reading `lib/_internal/recipe-conflicts.py`
without silent coverage loss. Same discipline as PR #496 (doctor), PR #497 (rulesaudit), PR #498 (skills).

## Specs

- S1: `TestConflictsDifferential` reads a frozen golden (`internal/conflicts/testdata/recipe_conflicts_golden.json`)
  captured once from `check_recipe_conflicts` + `check_capability_conflicts` + `check_tag_conflicts` over the same
  seven-recipe fabricated catalog and the same ten enabled-recipe/binding cases; no python3, no `lib/` access, no skip.
  Missing golden fails.
- S2: Every native (non-Python) assertion of the live test survives unchanged: the port's projections, the
  `port returned a nil slice; JSON would be null, not []` guard, and the `reflect.DeepEqual` against the legacy
  projection (including the `RecipeValidationError` text).
- S3: The frozen test asserts a hard-coded independent inventory of the ten case names (count, uniqueness, order)
  against both the in-test corpus and the golden before comparing anything, so deleting, duplicating or reordering a
  golden case fails as an inventory error.
- S4: Frozen inputs equal the in-test corpus: the catalog is pinned by a sorted `sha256  <id>/recipe.toml` manifest
  and each case pins its `ids` and `bindings` in the golden, both asserted before any comparison, so input drift fails
  as input drift, not as an opaque payload mismatch.
- S5: No absolute host path leaks into the golden: the fabricated catalog directory is tokenized as `<CATALOG>` where
  it reaches the payload (`recipe directory not found: <CATALOG>/<id>`); `grep -c /Users/\|/private/\|/var/folders`
  over `internal/conflicts/testdata` is 0.
- S6: Negative controls: mutating one frozen result, deleting a case, duplicating a case, and drifting the corpus
  manifest each fail rc=1.
- S7: `gofmt -l internal/conflicts` empty, `go vet ./internal/conflicts/` rc=0, the test free of live `python3` /
  `exec.Command` / `lib/_internal`, `go test ./internal/conflicts/` rc=0.
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestConflictsDifferential` (`recipe-conflicts.py` primitive + capability + tag conflicts) | FREEZE | the payload is host-independent except for the catalog directory, which reaches it only through the `recipe directory not found: <dir>/<id>` error text; replacing that directory with `<CATALOG>` keeps the full payload — every conflict's type, id, sorted recipe set and severity, plus the exact validation text. Nothing hashes the TempDir, so no RETARGET is needed and no case is retired. |

Cases: 10 (`no-conflicts`, `skill-command-tag-cap`, `order-reversed`,
`dup-inside-one-recipe-stops-registration`, `mcp-collision`, `explicit-binding-silences-warning`,
`duplicate-explicit-binding-fatal`, `binding-missing-keys`, `missing-recipe-dir`, `invalid-recipe`).

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6 | inline | |
| T2 | S7, S8 | inline | |

## Log

- L1 (Robert): "si, avancemos con las entregas" — authorizing continuing the PR4a S3 deliveries with the conflicts slice.
- L2: Baseline at ac59595: the live python differential is green (`logs/00-baseline-live-leg.log`, rc=0).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s3-conflicts (Robert's standing scratch convention).
- L4: RED before the golden existed: `read frozen oracle testdata/recipe_conflicts_golden.json: no such file or directory`, rc=1 (`logs/03-red-missing-golden.log`).
- L5: Capture harness was a temporary `internal/conflicts/zz_scratch_capture_test.go` (python3 exec) that cross-checked the live port against the legacy run before writing, so the golden is legacy output, not port output; the cross-check caught one harness-only difference (the port's `Primitive` stays nil on error while the frozen `primitive` is `[]`) which the real test already normalizes. The file was moved out of the worktree before the commit (preserved in the scratch dir as `zz_scratch_capture_test.go.retired`) so the tree carries no scratch file.
- L6: Captured twice into two fresh scratch dirs and `cmp` byte-identical (rc=0); oracle metadata `python: 3.14.7`. `json.Encoder.SetEscapeHTML(false)` keeps the token readable (`<CATALOG>` not `\u003cCATALOG\u003e`). Golden size: 7607 B.
- L7: Finding (capability corpus): the fabricated catalog writes `capabilities = [{ id = ... }]` under `[provides]`, but `recipe_schema.py` reads `capabilities` from the recipe top level, so both legs see zero providers. Provider-ambiguity warnings are therefore exercised by no case here — pre-existing in the live leg, unchanged by freezing; `CheckCapabilityConflicts`'s ambiguity branch stays covered where it already was (the bindings differential). No corpus change in this slice.
- L8: Negative controls (each rc=1; golden restored and `cmp`-verified after each): mutate a frozen tag severity (`logs/07-nc1-mutate-result.log`), delete a case → `frozen oracle has 9 cases, inventory 10` (`08-nc2-delete-case.log`), duplicate a case → `11 cases, inventory 10` (`09-nc3-duplicate-case.log`), drift the catalog manifest → `frozen inputs differ from the corpus catalog` (`10-nc4-input-drift.log`). The first NC1 attempt mutated a severity to the value it already had and passed — the control was rewritten to flip the value, proving the comparison is live.
- L9: `gofmt -l internal/conflicts` empty (`logs/14-gofmt.log`), `go vet ./internal/conflicts/` rc=0 (`15-vet.log`), `grep -n "python3\|exec\.Command\|lib/_internal"` over the test matches comment lines only (`13-grep-live-legs.log`), `go test ./internal/conflicts/` rc=0 (`16-green-package-tests.log`).
- L10: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`17-build.log`), `./tests/validate.sh` rc=0 (`18-validate.log`): `Ran 2345 tests in 587.149s`, `OK (skipped=165, expected failures=3)`, parity summary `PASS`.
- L11: No file modes or mtimes are frozen: the snapshot is the conflict projection only, so no umask or mtime pinning is needed (unlike the flatten oracle).
