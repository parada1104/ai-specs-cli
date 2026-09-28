# go-05-config-layer — Go port of the manifest config layer

Card: 6a84e78dedc3992f53376822 ([Go 05]). All changes left uncommitted per
card discipline (parent orchestrator stages/commits/opens the PR).

## Card tasks

- [x] 1. ADR 0002 — TOML decision (docs/go-adr/0002-toml-line-editor.md)
- [x] 2. internal/toml — TOML value parser + TOMLValue serializer (differential vs python3 tomllib, 27 files)
- [x] 3. internal/config — section readers (156 byte-identical JSON pairs) + 3 surgical writes (15/15 differential)
- [x] 4. internal/schema — recipe_schema.py port (21 clean + 82 fixture differentials, exact FROZEN error strings)
- [x] 5. internal/lock — lock.py port (17-case write differential, 6 refusal-equality cases, atomicity pins)
- [x] 6. internal/target — target-resolve.py + util topology chain (31/31 differential incl. real git submodule pair)
- [x] 7. Full verification: go build/vet/test + ./tests/run.sh (2358 tests OK)

## Final verification (task 7, observed)

```
$ CGO_ENABLED=0 go build ./... && go vet ./... && echo BUILD_OK VET_OK
BUILD_OK VET_OK   (exit 0)
$ go test ./... -count=1
ok  internal/cli      2.186s
ok  internal/config   6.586s
ok  internal/home     0.434s
ok  internal/lock     1.500s
ok  internal/schema   5.065s
ok  internal/target   3.295s
ok  internal/toml     1.264s
exit 0
$ ./tests/run.sh
run.sh: go found — running Go gate tests ... ok (ai-specs.dev/worktree-gate, /ledger)
run.sh: running root Go module tests (go test ./cmd/... ./internal/...) ... all ok
python3 -m unittest discover -s tests -p 'test_*.py'
Ran 2358 tests in 1694.528s
OK (skipped=164)
```

## internal/toml evidence (task 2)

- RED: tests first, implementation absent → `undefined: Parse / Table /
  TOMLValue / TOMLValueOrdered`, FAIL build, exit 1. First implementation
  run then exposed an infinite loop in the multi-line-string backslash
  branch (found by differential corpus 02) — fixed.
- GREEN: `ok ai-specs.dev/ai-specs/internal/toml 1.167s`, exit 0.
- Differential corpus: 27 files vs `python3 -c '...tomllib...'` (3.14.7):
  3 testdata + 21 catalog recipe.toml + 1 tests/fixtures recipe.toml +
  2 tests/fixtures ai-specs.toml — all PASS.
- Serializer: TOMLValue (map-driven) + TOMLValueOrdered (explicit key order
  for Python dict insertion-order call sites); Python `str(float)` repr
  formatting and json.dumps ensure_ascii escaping with surrogate pairs.

Branch: `change/go-05-config-layer` (cut from `epic/go-single-binary`).
Contract: byte-identical output vs `lib/_internal/toml-read.py` (§3 FROZEN surface)
and byte-identical file results vs the Python heredocs in
`lib/recipe-remove.sh`, `lib/skills-add.sh`, `lib/skills-remove.sh`.

## Tasks

- [x] RED: differential read tests (`TestDifferentialReadSections`) + write
      differential tests (`TestDifferentialWrites`) + atomic/mode tests
      (`TestAtomicReplaceAndMode`) + unit error tests, all against stubs.
- [x] RED: capture observed failure evidence.
- [x] GREEN: implement `internal/config` (readers, JSON serializer, segment
      editor, three write ops, atomic replace).
- [x] GREEN: capture observed pass evidence.
- [x] TRIANGULATE: synthetic fixtures (env-list, environment fallback,
      timeout/enabled variants, recipes variants, bindings, nested inline
      tables, non-ASCII, empty manifest, guard cases, escaping cases).
- [x] Verification: `CGO_ENABLED=0 go build ./... && go vet ./internal/config/... && go test ./internal/config/... && go test ./...`
- [x] Evidence block appended below.

## go-05-config-layer.2 — target-resolve port (`internal/target`)

- [x] RED: differential tests (`TestDifferentialPlanJSON`,
      `TestDifferentialTopologySubmodules`, `TestDifferentialErrors`, Go-only
      degradation pins) against the unimplemented package.
- [x] RED: capture observed failure evidence.
- [x] GREEN: implement `internal/target` (plan, topology chain, JSON
      indent/compact writers, Run).
- [x] GREEN: capture observed pass evidence + differential counts.
- [x] Verification: `CGO_ENABLED=0 go build ./... && go vet ./internal/target/... && go test ./internal/target/... && go test ./...`

## Notes

- `internal/toml` helpers `quoteJSONString`/`formatPyFloat` are unexported and
  outside the allowed edit surface → replicated in `internal/config/json.go`.
- Parse-error text in the valid→invalid guard is diagnostic (ADR 0002), not
  FROZEN: differential tests compare the guard-message prefix, not the full
  tomllib-vs-Go error detail.
- `skills-add` heredoc uses non-atomic `write_text` and universal-newline
  reads (CRLF→LF translation); the Go port uses atomic replace and raw bytes
  per the task contract (atomic for ALL writes). Corpus is LF-only.

## Evidence

### Differential counts

- Reads: corpus = 28 manifests (2 × tests/fixtures/**/ai-specs.toml,
  12 × catalog/recipes/*/recipe.toml, repo ai-specs/ai-specs.toml copied to
  a temp dir, 13 synthetic internal/config/testdata manifests) × 6 sections
  = 168 (file, section) pairs → **156 compared byte-for-byte identical,
  12 skipped** (the two already-invalid bases × 6 sections; python exits 1
  on invalid TOML — logged per-pair in the test output).
- Writes: **15 differential cases** (5 recipe removal incl. sub-tables,
  repo-manifest removal, not-found, guard-fires, already-invalid;
  5 dep removal; 5 appends) — file bytes, stdout messages, and error
  strings identical to the verbatim heredoc replicas in
  internal/config/testdata/*_ref.py. Guard messages compared up to the
  "invalid TOML:" prefix (parse-error detail is diagnostic per ADR 0002).

### RED tail (stub implementation, `go test ./internal/config/...`)

```
    differential_test.go:141: differential read: compared 0 (file, section) pairs, skipped 12
    differential_test.go:146: only 0 (file, section) pairs compared — corpus unexpectedly small
--- FAIL: TestDifferentialWrites (0.52s)
    --- FAIL: TestDifferentialWrites/remove_recipe_with_sub-tables (0.04s)
        writes_differential_test.go:295: python succeeded but Go op failed: not implemented
    --- FAIL: TestDifferentialWrites/remove_nonexistent_recipe (0.04s)
        writes_differential_test.go:295: error mismatch:
              go: "not implemented"
              py: "  ✗ recipe 'missing' not found in <DIR>/manifest.toml"
    --- FAIL: TestDifferentialWrites/remove_recipe_valid→invalid_guard_fires (0.04s)
        writes_differential_test.go:295: error mismatch:
              go: "not implemented"
              py: "ERROR: removing recipe 'doom' would produce invalid TOML:"
...
--- FAIL: TestAtomicReplaceAndMode (0.00s)
    writes_differential_test.go:316: mode 755: remove failed: not implemented
FAIL	ai-specs.dev/ai-specs/internal/config	6.461s
exit=1
```

### GREEN tail (`go test ./internal/config/...`)

```
--- PASS: TestReaderErrors (0.00s)
    differential_test.go:141: differential read: compared 156 (file, section) pairs, skipped 12
--- PASS: TestDifferentialReadSections (5.55s)
--- PASS: TestDifferentialWrites (0.52s)
    --- PASS: TestDifferentialWrites/remove_recipe_with_sub-tables (0.04s)
    --- PASS: TestDifferentialWrites/remove_recipe_valid→invalid_guard_fires (0.04s)
    --- PASS: TestDifferentialWrites/remove_dep_first-of-two (0.04s)
    --- PASS: TestDifferentialWrites/append_escaping_quotes_and_backslashes (0.03s)
    (15/15 write subtests PASS)
--- PASS: TestAtomicReplaceAndMode (0.00s)
PASS
ok  	ai-specs.dev/ai-specs/internal/config	6.481s
exit=0
```

### Verification (observed exit codes)

```
CGO_ENABLED=0 go build ./...   → exit 0
go vet ./internal/config/...   → exit 0
go test ./internal/config/...  → exit 0 (ok 6.253s)
go test ./...                  → exit 0 (internal/cli, config, home, toml all ok)
```

### Recipe schema port (internal/schema, go-05 addendum)

Strict TDD port of `lib/_internal/recipe_schema.py` → `internal/schema`
(Recipe dataclasses + validation, FROZEN error strings).

#### RED tail (stub implementation, `go test ./internal/schema/...`)

```
--- FAIL: TestDifferentialAgainstPython (4.50s)
    differential_test.go:445: .../catalog/recipes/bitbucket-pr-flow/recipe.toml: go LoadRecipeToml failed: not implemented
    (all 21 clean-corpus recipes: "go LoadRecipeToml failed: not implemented")
    --- FAIL: TestDifferentialAgainstPython/err_blank_name.toml (0.04s)
        differential_test.go:522: error mismatch
              go: "not implemented"
    (82/82 fixture subtests FAIL; 0 compared)
--- FAIL: TestLoadRecipeTomlMissingFile (0.00s)
    got: "not implemented" want: "recipe.toml not found: ..."
FAIL	ai-specs.dev/ai-specs/internal/schema	4.879s
exit=1
```

#### GREEN tail (`go test ./internal/schema/... -v`)

```
    differential_test.go:499: differential: clean corpus 21 recipes ok both sides; fixtures 82 byte-identical (18 ok, 64 error)
--- PASS: TestDifferentialAgainstPython (4.19s)
--- PASS: TestLoadRecipeTomlMissingFile (0.00s)
PASS
ok  	ai-specs.dev/ai-specs/internal/schema	4.388s
exit=0
```

#### Differential counts

- Clean corpus: **21/21** real recipe.toml files (12 catalog + 9 fixture
  recipes) loaded end-to-end via `load_recipe_toml` vs `LoadRecipeToml`;
  full canonical Recipe projections compared byte-for-byte.
- Synthetic fixtures: **82/82** byte-identical (18 ok — coercions,
  structured shapes, extra bucket, brief forms, silent non-table paths;
  64 error — one per validation branch, incl. leading-dot `.capabilities[0]`
  context, `{policy!r}` repr, needs_mcp no-type message, reconcile shapes
  incl. 33-entry list bound, init prompt FS checks with a real temp
  recipe dir on both sides, brief project-only/unknown/mixed forms).
- TRIANGULATE: covered by the 64 error fixtures + 18 ok fixtures
  (negative/alternate cases per branch); focused suite stayed green
  throughout.

#### Verification (observed exit codes)

```
CGO_ENABLED=0 go build ./...   → exit 0
go vet ./internal/schema/...   → exit 0
go test ./internal/schema/...  → exit 0 (ok 4.388s)
go test ./...                  → exit 0 (cli, config, home, schema, toml all ok)
```

### GO-05 lock port (`internal/lock` from `lib/_internal/lock.py`)

#### Scope

- New `internal/lock` package: `LockHeader`, `Sha256Bytes`/`Sha256OfFile`,
  `LoadLock` (+`NewLock`), `TOMLString`, `HasControlChar`, `WriteLock`
  (refusal-first, render, 0600 temp + rename), and the seven mutation
  helpers (`SetManagedOverride`, `SetGateBaseline`, `SetBriefBaseline`,
  `SetRecipeSkillHashes`, `SetDepSkillHashes`, `RemoveRecipeLockEntries`,
  `RemoveDepLockEntries`) with exact Python semantics incl. setdefault
  chains. `go_write_lock` bridge glue deliberately NOT ported (disappears in
  the single binary). Conventions mirrored from
  `catalog/recipes/worktree-flow/gate/lockwrite.go` without importing it.
- Tests: `internal/lock/lock_test.go` (13 top-level tests, 37 subtests across
  differential/refusal-order/atomicity groups), driver
  `internal/lock/testdata/write_lock_ref.py`, fixture
  `internal/lock/testdata/sample.ai-specs.lock`.

#### RED (observed before implementation)

```
$ CGO_ENABLED=0 go test ./internal/lock/...
# ai-specs.dev/ai-specs/internal/lock [ai-specs.dev/ai-specs/internal/lock.test]
internal/lock/lock_test.go:54:5: undefined: LockHeader
internal/lock/lock_test.go:77:13: undefined: TOMLString
internal/lock/lock_test.go:85:7: undefined: HasControlChar
...
internal/lock/lock_test.go:122:15: too many errors
FAIL	ai-specs.dev/ai-specs/internal/lock [build failed]
exit code: 1
```

#### GREEN + TRIANGULATE (observed after implementation)

- First GREEN run exposed two test-expectation bugs (control-char table
  listed `""`/`"ok"` as true; CRLF hash fixture had a trailing-newline
  mismatch) — fixed in the TEST, not the implementation; suite then green.
- Differential: 17 spec cases vs `python3 _write_lock_python` (direct call,
  bridge bypassed): empty/meta-only/blank-meta/all-sections/sha256-only/
  no-sha256-skip/escaping/unicode/empty-maps/unsorted-insertion + 6 refusal
  cases (meta newline, managed value 0x07, agents hash DEL, deps skill name
  newline, deps hash newline, agents-vs-deps order) with byte-equal refusal
  messages and no file written on either side. 0 skipped (python3 3.14.7,
  PYTHONDONTWRITEBYTECODE=1).
- Refusal-order: 11 first-locator cases pin the Python walk (agents BEFORE
  deps; managed values skipped without sha256; sorted-first-wins).
- Atomicity: fresh 0600, replaced-file mode becomes 0600 (os.replace swaps
  the temp's mode), parent auto-created, CreateTemp failure leaves original
  byte-identical, rename failure (target is a directory) removes the temp,
  success replaces bytes entirely, no `.ai-specs.lock.*.tmp` residue.
- Round-trip differential: `WriteLock(LoadLock(f))` vs Python
  `load_lock`+`_write_lock_python` on the same file — byte-equal; `[recipes]`
  and `[skills]` dropped by the writer on BOTH sides (recorded parity).

#### Verification (observed exit codes)

```
CGO_ENABLED=0 go build ./...   → exit 0
go vet ./internal/lock/...     → exit 0
go test ./internal/lock/...    → exit 0 (ok 1.142s, 13 tests, 0 FAIL/SKIP)
go test ./...                  → exit 0 (cli, config, home, lock, schema, toml all ok)
```

### GO-05 target-resolve port (`internal/target` from `lib/_internal/target-resolve.py` + util.py topology chain)

#### Scope

- New `internal/target` package: `ResolveTargetPlan` (plan dict with exact
  Python key order), `Run` (main() port: usage exit 2, FileNotFoundError →
  `error: <msg>` exit 1, ResolutionError → compact JSON envelope exit 1,
  success → `json.dumps(plan, indent=2)` + newline exit 0), `ResolutionError`,
  `_normalize_declared_relpath` (incl. local `posixpath.normpath` port),
  `_validate_target` (`resolve()` non-strict mirror incl. dangling-symlink
  realpath semantics), DERIVED_ARTIFACTS + VERSION_POLICY verbatim, and the
  ported util.py topology chain (`TopologyResolution`, `ProjectTopology`,
  `detect_submodules`, `resolve_repo_topology`, `topology_config`,
  `project_repo_topology`, `project_manifest_data`, `_legacy_recipe_topology`).
  Local JSON writers (compact + indent=2, ensure_ascii) because
  `internal/config`'s `Obj`/serializer are unexported and outside the allowed
  edit surface; manifest loading reuses `config.LoadManifest`.
- Tests: `internal/target/differential_test.go` — differential vs
  `python3 lib/_internal/target-resolve.py` (cwd = worktree root, so argv[0]
  matches the Go usage literal) + Go-only pins.

#### RED tail (no implementation, `CGO_ENABLED=0 go test ./internal/target/...`)

```
internal/target/differential_test.go:72:10: undefined: Run
internal/target/differential_test.go:592:19: undefined: detectSubmodules
internal/target/differential_test.go:599:9: undefined: resolveRepoTopology
internal/target/differential_test.go:619:15: undefined: normalizeDeclaredRelpath
internal/target/differential_test.go:648:29: undefined: topologyConfig
FAIL	ai-specs.dev/ai-specs/internal/target [build failed]
exit=1
```

#### GREEN + TRIANGULATE (observed after implementation)

- 31/31 differential subtests PASS, byte-identical stdout, stderr, and exit
  codes on both sides (python3 3.14.7, PYTHONDONTWRITEBYTECODE=1):
  - PlanJSON (22): fixtures root-only + multi-target (copied to temp),
    backslash/`./` normalization + duplicate collapse, normpath collapse
    (trailing slash, `d1/../dup`), empty/whitespace entries, non-string
    scalars filtered by read_project (int/bool/float), absolute-path error,
    escape error, missing dir, not-a-dir, worktrees_dir via config dict /
    flat style / falsy / int, topology project / legacy-recipe / bogus (auto
    branch), symlink-inside, symlink-escape, dangling-symlink-escape
    ("escapes the root after resolution"), symlink-to-file, nonexistent root.
  - TopologySubmodules (2): real super+child pair via `git submodule add`
    (protocol.file.allow=always) → monorepo-submodules with
    submodules=["sub"], full plan bytes compared; plain repo → standalone.
  - Errors (7): missing manifest (`error: <root>/ai-specs/ai-specs.toml not
    found`), usage 0 args and 2 args (exit 2, exact bytes), and the four
    ResolutionError JSON envelopes (`{"error": {"path": …, "reason": …}}`).
- Go-only pins: git absent (PATH stripped) degrades to (present=true,
  no submodules → standalone/auto); normalizeDeclaredRelpath normpath edges
  (`a//b`, `a/./b`, `a/b/../c`, `./`, `a/..`) + non-string repr candidates
  (`1`, `True`, `0.5`); topology_config precedence (project > legacy-recipe
  > default, strip + deprecation marker); uninitialized submodule (`-`
  prefix) skipped by detectSubmodules.
- First GREEN run exposed two TEST defects (Go Run called with nil args;
  missing-manifest expectation used the unresolved /var symlink path) —
  fixed in the TEST, not the implementation; suite then green.

#### Verification (observed exit codes)

```
CGO_ENABLED=0 go build ./...   → exit 0
go vet ./internal/target/...   → exit 0
go test ./internal/target/...  → exit 0 (ok 2.955s)
go test ./...                  → exit 0 (cli, config, home, lock, schema, target, toml all ok)
```
