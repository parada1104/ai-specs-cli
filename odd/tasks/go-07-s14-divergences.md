# Go 07 S14-Δ — Close the six registered parity divergences

## Objective
Close the six Go-versus-Python divergences that the S13 slices registered instead of fixing, so that the
native sync path can be wired (S14c) without carrying a single recorded mismatch into production.

This is the first unit of the S14 series. Ordering is deliberate: the divergence closure lands before the
native materializer is built, because the materializer calls exactly the cores these fixes change.

## Tracker
- **Unavailable.** The Trello API key is missing or expired in this environment, so no card could be
  created or linked for this unit.
- The tracker host is advisory by design and its `identity_unavailable` rule is to report and proceed
  without recording a decision. `tracker.none` is never written by the host, so none was created here.
- Close-out follow-up: link the S14 card in this section once the tracker is reachable.

## Scope
- Base: `8fd5914` on `epic/go-single-binary` (S13 complete, register merged).
- Worktree: `.worktrees/go-07-s14-divergences`. Branch: `change/go-07-s14-divergences`.
- Cut into three PRs, in this order:
  1. **D1-D4** — the four gate-shared divergences, one single digest regeneration.
  2. **D5** — the `internal/provider` port divergences.
  3. **D6** — widen `internal/toml` and revise its ADR.

## Human decisions that bound this unit
Recorded because two of them reverse a documented decision and two loosen a guard:
- **Fix all six, with no exceptions.** Explicitly chosen over the narrower option.
- **D1 reverses the gate's strictness**: the core stops rejecting keyless envelope entries. Its Python
  counterpart is `_python_merge_config`, a TEMPORARY fallback that S16 deletes; the divergence is latent
  because no wired path builds a keyless entry today.
- **D5 accepts nondeterminism in the candidate order**: aligning with Python's `listdir` means Go stops
  sorting, so Go no longer decides ties deterministically. The observable value is nil (ties only).
- **D6 reverses ADR-0002** (`internal/toml/toml.go:4-10`), which excluded datetime values and out-of-range
  integers on the argument that the Python pipeline also fails at `json.dumps`. The ADR must be revised in
  the same PR as the widening, with the new rationale and the new parity evidence.
- Unchanged from the earlier S14 reconnaissance: the materialize step ships with **`go` as its only route**
  (no Python sibling value), S14 covers **all three** `recipe-materialize.py` call sites (`sync.go:329` and
  `syncagent.go:256`, `:380`), and the series is cut as S14a materialize core, S14b resolved-config
  assembly, S14c wiring.

## The six divergences
1. **Empty config key (D1).** Go: `catalog/recipes/worktree-flow/gate/shared/mergeconfig.go:296-309` returns
   a process error for a keyless field/table/manifest entry. Python: `lib/_internal/recipe-materialize.py`
   `_python_merge_config` :3006-3062 keeps an empty field key and routes an unknown manifest key into the
   `unknown config key '' in manifest (ignored)` warning (:3039). Fix: delete the three guards so empty keys
   flow as ordinary keys. Gate digests change.
2. **Empty recipe table (D2).** Go: `shared/resolvedconfig.go:81-91` (`ProjectRecipes`) skips an entry whose
   table is empty (:87-89). Python: `recipe-materialize.py:3782-3800` sets `recipes_out[rid] = config`
   unconditionally (:3797), so `[recipes.foo]` survives as `{"foo": {}}`. Fix: remove the skip. Gate digests
   change.
3. **Flat-versus-`[config]` collision (D3).** Go: `shared/resolvedconfig.go:92-99` builds the config map by
   ranging over a Go map (:93) and merging a nested `config` table inside the same loop (:98), so the winner
   depends on iteration order. Python: `recipe-materialize.py:3788-3797` walks the document, so the nested
   `[config]` table always overwrites the flat key. Fix: carry ordered per-key data through the envelope
   instead of a map — `ManifestEntry.Table` (`shared/resolvedconfig.go:48-50`), the payload decoder
   (`shared/resolvedconfig.go:268`), the gate reader
   (`catalog/recipes/worktree-flow/gate/resolved_config.go:79+`) and the root acquisition
   (`internal/sync/resolvedconfig.go:49-60`) — then iterate document order with flat-first/`config`-update
   semantics. This is the riskiest item: it changes an internal JSON envelope contract. Gate digests change.
4. **Non-string binding coercion (D4).** Go: `shared/resolvedconfig.go:119-129` (`ProjectBindings`) requires
   both `capability` and `recipe` to be non-empty strings (:122-125) and silently skips otherwise. Python:
   `recipe-materialize.py:3803-3810` reads both with `.get(..., "")` and keys the mapping, so an integer `5`
   becomes `"5"` and `True` becomes `"True"`. Fix: coerce truthy non-string values with the shared
   Python-string helper before keying. Gate digests change.
5. **Provider port (D5).** `internal/provider/provider.go`: candidate order sorts (`os.ReadDir`, :387-405)
   where `lib/_internal/provider_install.py:363-372` uses unsorted `iterdir` (:367); the version tuple
   saturates at `MaxInt64` (`strconv.Atoi`, :147-150) where `provider_install.py:311-315` is arbitrary
   precision; and the version regex plus its boundary checks are ASCII-only (:124-126, :152-154) where
   `provider_install.py:39` uses the Unicode `\d` class (`Nd`). Fix all three to match the Python authority.
   Internal only, no digest change.
6. **`internal/toml` value subset (D6).** `internal/toml/value_parse.go:75-77` refuses datetimes and
   :152/:173 refuse integers outside `int64`, while `tomllib` reads both; the divergence changes the
   resolver outcome, not only the message (the marker stays `{dep:jinna}` with a warning instead of being
   replaced). Fix: widen the reader and teach `internal/toml/serialize.go` the new types, preserving the
   Python end-to-end outcome where it fails downstream. Internal only, no digest change.

## Tasks
- [x] T1 (D1): delete the three keyless-entry guards in `shared/mergeconfig.go`; flip
      `internal/sync/mergeconfig_test.go` (`mergeConfigDivergenceEmptyKey`, `assertMergeConfigEmptyKeyDivergence`,
      the cases inside `TestNativeMergeConfigDifferentialAgainstPython`) and the gate-side case in
      `catalog/recipes/worktree-flow/gate/mergeconfig_test.go` (`TestMergeConfigMalformedInputExitsTwo`).
- [x] T2 (D2): stop skipping empty recipe tables in `shared/resolvedconfig.go`; flip
      `TestNativeResolvedConfigEmptyRecipeTableDivergence`.
- [x] T3 (D3): carry ordered keys through the envelope and iterate document order; flip
      `TestNativeResolvedConfigFlatConfigCollisionNondeterminism` into a deterministic parity assertion and
      update `TestProjectRecipesFlatAndConfigStyle` for the new envelope shape.
- [x] T4 (D4): coerce non-string binding values; flip `TestNativeResolvedConfigNonStringBindingDivergence`
      and add the two unpinned siblings (integer `recipe`, boolean `capability`).
- [ ] T5: regenerate `catalog/recipes/worktree-flow/bin/SHA256SUMS` and verify it; run the focused and full
      check sets; native review; PR and squash-merge. Digests regenerated and verified; the focused checks,
      the full suite and the native review are done, and the PR and the squash-merge are the remaining
      closing step.
- [x] T6 (D5): align the provider port (candidate order, arbitrary-precision version tuple, Unicode digits)
      and add the missing tie-case pin. Done: 294 insertions and 62 deletions across `provider.go`,
      `provider_test.go` and the differential driver's new `candidates` mode.
- [ ] T7 (D6): widen `internal/toml` (datetime and arbitrary-precision integers), revise ADR-0002, flip the
      two `internal/sync/buildrecipemcp_test.go` pins and the `internal/toml` pins.

## Acceptance criteria
- No test in the repository asserts a recorded Go-versus-Python divergence for these six items any more;
  each flipped test asserts parity against the real Python authority.
- `SHA256SUMS` matches the rebuilt gate module and `scripts/verify-gate-sums.sh` passes.
- The four gate touched decisions keep one authoritative grader: no duplicate decision survives in
  `internal/sync` wrappers.
- The suite stays green; the differential harnesses keep driving the real Python module.

## Checks
- Gate digests: `bash scripts/build-gate.sh`, then `cd dist && shasum -a 256 worktree-gate-darwin-amd64
  worktree-gate-darwin-arm64 worktree-gate-linux-amd64 worktree-gate-linux-arm64`, paste into
  `catalog/recipes/worktree-flow/bin/SHA256SUMS`, then `scripts/verify-gate-sums.sh dist/SHA256SUMS
  catalog/recipes/worktree-flow/bin/SHA256SUMS`.
- Root: `go test ./cmd/... ./internal/... -count=1`, the gate module's own tests, `gofmt -l`, `go vet`,
  `git diff --check`.
- Full: `./tests/validate.sh` with the gate binary built first, durable logs
  `~/.cache/s14-divergences-validate.{log,exit}`.
- Native review over the committed base of each PR.

## Progress
- Implemented by a delegated writer (T1-T4): 264 insertions and 250 deletions across the seven allowed
  paths, with every now-dead divergence constant and helper removed rather than left behind.
  - D1: the three keyless-entry guards are gone; a keyless manifest key takes the warning path and an empty
    field key keeps its default, both compared against `_python_merge_config` in the differential.
  - D2: `ProjectRecipes` now skips only a `nil` table, so `[recipes.foo]` survives as `{"foo": {}}`.
  - D3: resolved without touching the transport envelope. The order-free equivalent is valid for the
    `[recipes.<id>.config]` header form because TOML guarantees the flat keys of a table precede its
    sub-tables, so applying the nested `config` table in a second pass reproduces Python's document-order
    outcome. Proven on both legs with a probe manifest before relying on it: the gate binary and the Python
    authority both report `{"a":{"foo":2}}`.
  - D4: non-string bindings are now coerced with Python truthiness and `json.dumps` key rendering. The
    writer corrected this brief: Python renders a `True` dict key as `"true"`, not `"True"`, so the
    authority was matched exactly instead of by assumption.
- Residual D3 sub-case, explicitly pinned instead of silent: an **inline** `config = { ... }` declared
  before a colliding flat key still diverges (Python is document-ordered and reports the later flat key;
  the native two-pass reports the nested table). No shipped `catalog/` recipe uses that shape, and exact
  parity would require document order to survive TOML parsing end to end, which the map-based
  `internal/toml` cannot represent. `TestNativeResolvedConfigInlineConfigBeforeFlatKeyDivergence` asserts
  both sides and was proven real by temporarily inverting an expectation (failed, then passed again).
- Gate digests regenerated and verified: `scripts/verify-gate-sums.sh dist/SHA256SUMS
  catalog/recipes/worktree-flow/bin/SHA256SUMS` reports `ok — 4 digest entries match the committed trust
  root`, exit 0.
- Focused checks, all exit 0: the gate module's own tests (`go test ./... -count=1` in the module,
  three packages ok), `go test ./cmd/... ./internal/... -count=1`, `python3 -m unittest
  tests.test_merge_config_bridge tests.test_resolved_config_bridge` (49 tests OK), `gofmt -l`, `go vet`,
  `git diff --check`.
- Full validation with the gate binary rebuilt in this worktree: `./tests/validate.sh` exit 0,
  `Ran 2482 tests in 1678.630s` then `OK`, zero `FAIL:`/`ERROR:` lines. Evidence:
  `~/.cache/s14-divergences-validate.{log,exit}`. The parity harness ran in both gate modes, which is the
  evidence that matters here because the gate module changed.
- Native review (approved, authority burned): `inspect` with `baseRef=8fd5914` plus `committedOnly` bound
  the eight-file target `fa738900…`, and the single `start` created lineage `review-e8731bd6a869d98f`
  (tier medium, one lens `review-reliability`, 720 changed lines, correction budget 200). The first capture
  closed it as `approved` (`store_revision: 0501eb93…`) and the acknowledgement burned the authority
  (`gentle-ai.review-acknowledged/v1`). Two advisory findings, both non-blocking and recorded below.
- Environment note: the package's Go tests load the Python authority, so they fail under the system
  Python 3.9 (`No module named 'tomllib'`); the real 3.14.7 binary must be prefixed. This is pre-existing
  and was reproduced on an untouched test before any edit.
- Series plan agreed with the human before any write: fix all six; S14 cut into S14-Δ (this unit), S14a, S14b
  and S14c; `go` as the only materialize route; all three call sites.
- Reconnaissance established every Go and Python location, the digest procedure and the test surfaces to
  flip, and corrected two register inaccuracies (see Follow-ups).
- D5 implemented by a delegated writer in its own worktree: the candidate order reads the directory unsorted
  (`os.Open` plus `Readdirnames`), so Go stops adding an order Python does not impose; the version tuple is
  arbitrary precision (`VersionTuple` now returns `[]*big.Int`, an in-package signature change with every
  caller updated); and the version parsing is `Nd`-aware, mapping each Unicode decimal digit to its value.
  The candidate-order divergence finally has a pin, plus a tie case where two candidates share a tuple.
- Two claims in the D5 brief were wrong and the writer proved it rather than following them: Python's
  `(?<!\d)`/`(?!\d)` lookarounds are not observable through `_version_tuple` (an exhaustive probe over
  short strings plus 300k random ones found no difference), and the candidate-order pin has to compare
  against the raw directory listing to stay filesystem-independent.
- D5 residual, enumerated and pinned instead of silently carried: the Go toolchain's `unicode.Nd` table
  (Unicode 15.0.0, 680 code points) is narrower than CPython 3.14.7's `\d` (Unicode 16.0.0, 760), so 80
  decimal digits added in the newer release are accepted by the authority and rejected by the port. No
  realistic version string uses them, and a future Go carrying the newer table closes the gap by itself, so
  hard-coding the delta would only add a table that ages. Pinned by `TestVersionTupleUnicode16NdResidual`,
  one representative per missing block, both sides asserted.
- D5 focused checks, all exit 0: `go test ./internal/provider/ -count=1`, `go test ./cmd/...
  ./internal/... -count=1`, `python3 -m unittest tests.test_jinna_provider_recipe tests.test_dep_check`
  (78 tests OK), `gofmt -l`, `go vet`, `git diff --check`.
- D5 full validation with the gate binary rebuilt in its worktree: `./tests/validate.sh` exit 0,
  `Ran 2482 tests in 1930.356s` then `OK`, zero `FAIL:`/`ERROR:` lines. Evidence:
  `~/.cache/s14-provider-validate.{log,exit}`. The gate module is untouched by this unit, so its digests
  still verify (`verify-gate-sums.sh` reports four matching entries).
- D5 native review (approved, authority burned): `inspect` with `baseRef=08f1174` plus `committedOnly`
  bound the four-file target `b0fbf61f…`, and the single `start` created lineage `review-00a23e21e966006a`
  (tier medium, one lens `review-reliability`, 435 changed lines). The first capture closed it as
  `approved` (`store_revision: ee6bd2f8…`) and the acknowledgement burned the authority. One advisory,
  recorded below.

## Follow-ups
- Advisory findings of the native review, both informational and non-blocking (no correction was opened):
  - `R3-keyless-table-untested` reliability WARNING,
    `catalog/recipes/worktree-flow/gate/shared/mergeconfig.go:295-298`: the removed keyless *table* guard
    has no direct test.
  - `R3-inline-config-divergence-open` reliability SUGGESTION,
    `odd/tasks/go-07-s14-divergences.md`: the residual inline-config divergence described above.
- The inline `config = { ... }` shape of D3 stays divergent by construction (see Progress). Closing it for
  real would need an ordered TOML representation end to end, which is a separate architectural unit; no
  shipped catalog recipe uses the shape.
- Advisory finding of the D5 native review, informational and non-blocking:
  - `R3-unicode16-pin-env-coupled` reliability WARNING, `internal/provider/provider_test.go:223-225`: the
    Unicode-generation pin is coupled to the toolchain, so it fails deliberately when Go or CPython updates
    its Unicode tables. That is the intended loud failure; the pin must then be removed on purpose.
- The Unicode-generation residual of D5 (see Progress) closes by itself once the Go toolchain carries a
  newer Unicode table; the pin must then be removed deliberately.
- `managedCandidates` tests `!info.IsDir()` where Python tests `is_file()`, so a FIFO or a device entry in
  the managed cache directory would be treated differently. Pre-existing and outside this unit.
- Amend `odd/tasks/go-07-s13-followups.md` section 2: its provider bullet claims the candidate-order
  divergence is pinned in the `internal/provider` differential, which was untrue when the register was
  written and became true once D5 added the pin, so the sentence needs rewording rather than a new test. The
  two PR3 advisory rows also record "location not recorded in source"; the locations are
  `shared/resolvedconfig.go:87-89` and `:92-99`.
- ADR-0002 must carry the revised rationale in the D6 PR, not only in the code.

## Rollback
Revert the three PRs as a batch; each is self-contained and no wired production path observes them yet.
