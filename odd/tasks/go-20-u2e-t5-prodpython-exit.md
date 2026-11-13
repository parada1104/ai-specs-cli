# GO-20 U2e T5 — production Python exit (installer + the gate's writer seam)

Base: `epic/go-single-binary` @ `118694e1` (after #525). Branch:
`change/go-20-u2e-t5-prodpython-exit`.

Parent request (U2e T5, PR4b pre-work): the product must run with no `python3`
anywhere in its production paths before `lib/` is deleted. Two production seams
were named — `catalog/recipes/worktree-flow/gate/ledger_reconcile.go` (manifest
acquisition for ledger reconcile) and
`catalog/recipes/worktree-flow/gate/shared/recipeconfigwrite.go` (recipe-config
TOML reads) — plus `install.sh`, which still requires `python3` and pip-installs
`rich`/`questionary` into `lib/_vendor`.

## Specs

- S1 — Human decision (verbatim, Robert): "ok,vos dale debemos ir terminando
  esto". Standing decision for the slice: finish PR4a/PR4b work with the usual
  discipline (own worktree, test-first, one `validate.sh` run).
- S2 — Human decision (verbatim, Robert): "excelente, continuemos así muchas
  gracias". Continuation of the same method, no change of scope.
- S3 — Program rule, verbatim from `odd/tasks/go-20-python-exit.md` S1: "zero
  Python under the product tree; bash only as wrappers".
- S4 — Measured scope correction (this slice): the parent inventory named TWO
  production `python3` seams. There are FOUR in the gate module, and one of them
  is not reachable from the two allowed files:
  - `ledger_reconcile.go` `runManifestParser` → manifest read (IN scope, retired).
  - `shared/recipeconfigwrite.go` three readers (IN scope, retired).
  - `resolved_config.go:111` and `recipe_toml.go:223` share the same bounded
    subprocess helpers (`manifestInterpreter`, `cappedBuffer`,
    `classifyManifestParserError`, `manifestParseTimeout`) declared in
    `ledger_reconcile.go`; those two callers are OUTSIDE this slice's edit
    surfaces, so the helpers stay and `python3` remains reachable through them
    (`shared/recipe_toml.go:142,179` too). Recorded as the remaining PR4b work:
    `python3` is NOT yet absent from the gate's production paths.
- S5 — No new dependency: the gate module keeps its zero-third-party `go.mod`.
  The repository's TOML reader is `internal/toml` (1.535 source lines), which the
  gate module can never import: `go` rejects a cross-module `internal/` import
  (recorded at `internal/config/recipeconfig.go:34-46`). The reader is therefore
  duplicated into the gate's `shared` package — the only file both seams share —
  and `internal/toml` stays the root module's authoritative copy.
- S6 — Byte contracts: the frozen refusal framings stay identical
  (`cannot read the existing value for '<root>': …`, `invalid TOML after config
  write: …`, `the manifest is not valid TOML (…)`). The parse-error DETAIL is the
  Go parser's diagnostic (ADR 0002 class, `docs/go-migration-parity-contract.md`
  § doctor malformed-manifest: parse text is TOLERANT, framing/exit code FROZEN).
- S7 — The gate digest trust root
  (`catalog/recipes/worktree-flow/bin/SHA256SUMS`) is regenerated in this slice
  under explicit human approval (L11), with the canonical generator. The parity
  corpus pins one derived byte-count (`doctor-healthy`: `cache <N> KiB`), which a
  gate-source change also moves; that pin is not an allowed surface here (L15).

## Tasks

- **T1** — `install.sh`: drop the `python3` requirement and the pip/vendor step;
  keep every surviving installer behaviour (git requirement, mode-only dirt
  repair, narrow checkout, verified release asset or local build, atomic entry,
  dist copy, PATH note).
- **T2** — `shared/recipeconfigwrite.go`: replace the three embedded-Python
  tomllib readers with the in-process parser; add
  `shared.ReadManifestRecipesJSON` for the gate main package.
- **T3** — `ledger_reconcile.go`: `readManifestRecipes` parses in process; delete
  the manifest reader program and `runManifestParser`; keep the subprocess
  helpers that `resolved_config.go` / `recipe_toml.go` still use, with their
  comments corrected.
- **T4** — Tests: RED per seam (a `python3` shim first on PATH that records and
  fails), the reference-domain pins the retired readers used to provide, and the
  no-leak contract for an unparseable manifest.
- **T5** — `validate.sh` once on the result. Digests regenerated (L13); parity
  reduced from 3 failing fixtures per mode to 1 (L15), blocked on a surface
  decision for the gate cache-size pin.

## Log

- L1 — L1/L2 of the parent request = S1/S2 above.
- L2 — Measured premise correction. The parent brief said "the gate module
  already depends on a TOML library for its other packages — reuse it; do not add
  new dependencies". The gate module's `go.mod` declares `go 1.22` and ZERO
  requires, and no TOML reader exists in the gate tree: every TOML read there is
  a `python3` subprocess. The reuse target is `internal/toml`, unreachable across
  modules (S5), so the reader is copied into the gate's `shared` package — the
  only surface both seams share. The copy first landed inside
  `shared/recipeconfigwrite.go` (the one authorized file); Robert then approved
  extracting it into `shared/tomlparse.go` as a pure move (L11/L12).
- L3 — RED (installer). New `tests/test_install_no_python.py` (5 static guards +
  1 functional install): `Python3 -m unittest tests.test_install_no_python`
  rc=1, 5 failures. The functional leg proves the behaviour: with a recording
  `python3` shim first on PATH the installer invoked it twice —
  `-c import rich, questionary` and `-m pip install --upgrade --quiet --target
  …/lib/_vendor rich>=13.0.0,<15 questionary>=2.0.0,<2.1`
  (`logs/red-install-python.log`).
- L4 — RED (shared writer seam). `go -C catalog/recipes/worktree-flow/gate test
  ./shared -run 'TestRecipeConfigSeamNeverInvokesAnInterpreter|…'` rc=1:
  `readRecipeConfigTable: the manifest is not valid TOML (parser exit status 97)`
  — the shim was executed by the current code (`logs/red-shared-seam.log`).
- L5 — RED (manifest seam). `go -C … test . -run 'TestLedgerReconcile*'` rc=1:
  `readManifestRecipes: … (parser exit status 97)` (shim executed), and the
  subset pin observed the retired reader's JSON separators
  (`["a", "b"]` / `{"mode": "fast", "retries": 3}`) against the in-process
  compact form (`logs/red-main-seam.log`).
- L6 — GREEN. `go -C … build ./...` rc=0; the four new `shared` tests PASS
  (`logs/green-shared-seam.log`); `go -C … test . -run TestLedgerReconcile`
  rc=0 (`logs/green-main-seam.log`); whole gate module
  `go -C … test ./... -count=1` rc=0 — `main 27.0s`, `ledger 2.2s`,
  `shared 0.2s` (`logs/gate-all.log`). Removing `requireRecipeConfigSeam` also
  un-skipped the 21 writer tests that used to depend on an ambient `python3`.
- L7 — PRESERVE. Root module: `go vet ./cmd/... ./internal/...` rc=0,
  `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, and
  `go test ./internal/config/... ./internal/gatebinary/... ./internal/recipe/...
  ./internal/sync/...` rc=0 — including
  `TestUpdateRecipeConfigRefusesADatetimeTheGateCopyCorrupts`, the pinned
  divergence that requires the gate copy to keep lowering a TOML date/time to a
  quoted string (`fromTOML` keeps that by design). `scripts/build-gate.sh` rc=0
  (4 targets, canonical go1.24.13, no version warning).
  `tests/test_recipe_config_write.py` 20/20 OK. Gate `go vet` rc=0.
- L8 — Assessed, not changed (out of surfaces): `docs/go-migration-parity-contract.md`
  should gain the TOLERANT entry for the gate writer's parse-error detail; the
  remaining `python3` seams are S4.
- L9 — **Open blocker (human decision, S7).** `./tests/validate.sh` rc=1 with
  `fixtures: 35, failing: 3` in BOTH modes, and standalone
  `python3 -m unittest discover -s tests -p 'test_*.py'` = 1807 tests,
  `failures=17 errors=1 skipped=1 expected failures=3`. Every failure is one
  cause: the rebuilt gate binary no longer matches the committed trust root
  (`digest mismatch: expected e6f6fbcb…, got 5ac57460…; candidate never
  executed`), so the acquisition refuses it and 15 Python checks
  (`test_committed_digests_match_locally_built_assets`,
  `test_ci_generated_sums_file_parses_and_matches_build`,
  `test_canonical_sums_comparison_ignores_header_and_order`, the gate-dependent
  topology/ledger/hooks/init rows) plus 3 parity fixtures fail as a consequence.
  Regenerating `catalog/recipes/worktree-flow/bin/SHA256SUMS` is the only fix,
  and it is (a) not an allowed surface and (b) explicitly escalation-gated by the
  epic contract ("Escalame si una regeneración de sums tocara el trust root de
  release"). Every earlier gate-source PR did regenerate it (7384c654, fe6e7cff,
  de50ef50, 08f1174b, 2de231de).
- L10 — Test-count reconciliation (superseded by L13): this slice adds 6 Python
  tests (`tests/test_install_no_python.py`) and deletes no assertion (only the
  inert `AI_SPECS_SKIP_TUI_DEPS` env entries in three install fixtures). The
  discover count measured while the digest was stale was 1807, which L10
  mis-read as a base of 1801; it was an artifact of the broken gate acquisition
  (see L13).
- L11 — Human decision, verbatim intent (Robert, 2026-11-13): approved both
  recommendations — (1) regenerate the release trust root
  `catalog/recipes/worktree-flow/bin/SHA256SUMS` with the digests of the rebuilt
  gate binary in this worktree (go1.24.13 = canonical, as `build-gate.sh`
  reported), and (2) the pure layout move of the copied TOML reader out of
  `shared/recipeconfigwrite.go` into the new `shared/tomlparse.go` (same package,
  block moved verbatim, provenance comment kept, no renames, no signature
  changes). Both paths were added to the allowed edit surfaces.
- L12 — The move, executed as a pure move: `shared/tomlparse.go` (1333 lines of
  parser + the provenance header, `package shared` clause, its own import block)
  and `shared/recipeconfigwrite.go` (2578 → 1229 lines, losing exactly that block
  plus the now-unused `math`/`math/big` imports). No other file changed in the
  move. `gofmt -l` empty, `go -C … vet ./shared/` rc=0, `go -C … test ./shared/`
  rc=0, and `go -C … test ./... -count=1` rc=0 in the final `validate.sh` run
  (gate main 40.9s, ledger cached, shared 0.2s).
- L13 — Trust root regenerated with the canonical generator, exactly as the file's
  own header prescribes (no hand-edited digests):
  `bash scripts/build-gate.sh` (rc=0, four targets, no toolchain warning) then
  `cd dist && shasum -a 256 worktree-gate-darwin-amd64 \
    worktree-gate-darwin-arm64 worktree-gate-linux-amd64 \
    worktree-gate-linux-arm64` → verified with
  `bash scripts/verify-gate-sums.sh <generated> catalog/recipes/worktree-flow/bin/SHA256SUMS`
  (`verify-gate-sums.sh: ok — 4 digest entries match the committed trust root`).
  New digests: darwin-amd64 `465bda94…`, darwin-arm64 `def34f0e…`, linux-amd64
  `35262310…`, linux-arm64 `5ed65ea8…`; the file's regeneration log gained the
  matching GO-20 U2e T5 entry. `go build -o dist/ai-specs ./cmd/ai-specs` rc=0,
  `go vet ./cmd/... ./internal/...` rc=0, gate `vet ./...` rc=0.
- L14 — Python suite reconciled after the digest regeneration:
  `python3 -m unittest discover -s tests -p 'test_*.py'` rc=0,
  `Ran 1810 tests … OK (skipped=1, expected failures=3)`, zero failures — i.e. the
  documented base 1804 plus this slice's 6 new tests, with nothing removed. The
  earlier 1807 reading (L10/L9) was the stale-digest state: 3 gate-dependent rows
  did not run while the acquisition refused the binary.
- L15 — **Resolved by decision** (L17). `./tests/validate.sh` rc=1 with
  `parity summary: gate-absent failing=1, gate-present failing=1 — FAIL`
  (fixtures 35). The digest class was gone (it had been 3 failing fixtures in
  both modes); the single remaining delta was `doctor-healthy`, and the only
  difference in it was the `worktree-gate` line's cache size:
  golden `cache 3115 KiB` vs live `cache 3261 KiB`. That number is
  `internal/doctor/gatebinary.go:68 gateCacheSize` — the byte total of
  `<home>/cache/bin/worktree-gate` — so it is a function of the built gate binary
  bytes, which this slice necessarily grew (+146 KiB of parser code). Not a
  behaviour, no contract entry, and no gate source had changed since the goldens
  were frozen at 276fa856 (T2a), which is why the number had never moved.
- L16 — Final production `python3` grep (install.sh, `catalog/**/*.go` non-test,
  `internal/` non-test, `cmd/`, `bin/`): `install.sh` none; `bin/` none;
  `internal/` and `cmd/` none (comments only); one hit class remains in the gate
  module — `exec.LookPath("python3")` at `ledger_reconcile.go:453` and
  `shared/recipeconfigwrite.go:79`, which now serve only the out-of-scope seams
  `resolved_config.go:105`, `recipe_toml.go:216` and
  `shared/recipe_toml.go:142,179` (PR4b).
- L17 — Human decision, verbatim intent (Robert, 2026-11-13, after the SHA256SUMS
  regeneration): approve option 1 — ONE narrow N-normalization for the doctor
  gate cache byte count, goldens stay frozen, no assertion retired.
  Implemented as
  `N12-gate-cache-size` (`surfaces=("stdout",)`, verb-scoped to `doctor`): it
  rewrites ONLY the number in the one line's exact
  `; selftest passed (cache <N> KiB)` suffix to `<CACHE_KB>`; the line's identity,
  label, selftest text and every other doctor field stay compared verbatim. The
  justification records that the metric is the cached-binary byte total, so it
  moves with every legitimate gate change and — unlike N10/N11 — never expires
  with a corpus recapture.
- L18 — Making the rule effective on the FROZEN leg (measured, minimal). Live
  legs already pass through `apply_normalizations`; the frozen leg did not, so
  the golden still read 3115 and the delta survived the first validate run
  (`logs/validate-n12.log`, rc=1, failing=1). `load_golden_leg` now applies the
  same normalizations with each step's argv. Idempotency was measured over the
  whole corpus before the change: across all 35 fixtures in both gate modes,
  re-normalizing the frozen text changes exactly 2 steps — the two
  `doctor-healthy` golden `doctor` stdouts, only through N12. No golden byte was
  rewritten.
- L19 — Final validation. `python3 tests/parity/run.py` rc=0,
  `fixtures: 35, failing: 0`; then `./tests/validate.sh` rc=0 with
  `fixtures: 35, failing: 0` in both modes,
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`, all Go
  legs `ok` (gate main 40.9s, ledger cached, shared 0.2s), and the discover leg
  `Ran 1810 tests in 532.884s` → `OK (skipped=1, expected failures=3)`.
  Count reconciliation unchanged: 1810 = documented base 1804 + this slice's 6
  new Python tests; no assertion removed.
