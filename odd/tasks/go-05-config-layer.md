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

### JD round-1 fixes (CRLF parity) — 2026-02

Scope: JD-GO05-1 (internal/toml CRLF line endings, severe) + JD config-side
CRLF write round-trip and read-corpus skip whitelist. Autonomous parity-faithful
execution; no commits (parent owns git).

#### Fix 1 — internal/toml CRLF (JD-GO05-1)

RED (re-verified before implementing):

```
go test ./internal/toml/... -run TestParseDifferentialCorpus
  --- FAIL: TestParseDifferentialCorpus/04-crlf.toml
      toml_test.go:106: Go Parse failed on testdata/04-crlf.toml:
      toml: line 5: expected key, found "\r"
```

Implementation (mirrors python3.14.7 tomllib behavior pinned by the parent):
- `skipBlank`: new case `c == '\r' && p.hasPrefix("\r\n")` consuming 2 bytes,
  line++ (before the bare '\n' case) — fixes blank lines, comments, and
  multi-line arrays via parseArray's reuse of skipBlank.
- `expectLineEnd`: accepts "\r\n" (consume 2, line++) in addition to '\n'.
- `parseMultilineBasic` / `parseMultilineLiteral`: content '\r' followed by
  '\n' writes '\n' into the value (CRLF→LF normalization inside multi-line
  string values, matching tomllib) instead of the raw '\r'.
- Kept unchanged after verification: global bare-CR pre-scan in Parse,
  `trimLeadingNewline` (already accepts \r\n), `isLineEndingBackslash` +
  continuation trim loop (already accept \r), skipComment consuming \r as
  comment content (harmless).
- Empirically verified against python3 3.14.7 tomllib: CRLF inside a
  single-line basic/literal string is an ERROR on both sides; lone \r is an
  ERROR everywhere (pre-scan); multi-line CRLF content normalizes to \n;
  backslash continuation with \r\n yields 'cont joined'.

GREEN:

```
go test ./internal/toml/... -count=1 -v -run 'TestCRLFLineEndings|TestParseDifferentialCorpus'
  --- PASS: TestParseDifferentialCorpus (0.81s)   [34 corpus entries incl. 04-crlf.toml, 05-crlf-strings.toml]
  --- PASS: TestCRLFLineEndings (0.00s)
  ok  ai-specs.dev/ai-specs/internal/toml 1.026s
```

New coverage: `TestCRLFLineEndings` (CRLF after kv/header/comment/blank/
array/AOT; multiline basic+literal CRLF→\n; \r\n continuation; lone \r and
raw \r inside basic/literal strings rejected) + testdata/05-crlf-strings.toml
(real CRLF bytes, multi-line strings with CRLF content + continuation) in the
byte-identical differential corpus vs tomllib.

#### Fix 2 — internal/config

(a) CRLF write round-trip. Parity target: the heredocs' actual output.
python `Path.read_text` applies universal-newline translation (\r\n and lone
\r → \n in memory, including inside multi-line strings) before
splitlines(keepends=True) and the write-back, so a CRLF manifest round-trips
LF-normalized. Go replicated this exactly: new `readManifestText` helper
replaces the three raw `os.ReadFile` read paths in writes.go (AppendDepsBlock,
RemoveRecipeSegments, RemoveDepSegment); segmenting, guard validation, and
output all operate on translated text. Audit of remaining '\n' assumptions:
splitLinesKeepEnds already ports splitlines(keepends=True) fully; Go RE2 `\s`
includes \r like Python re; strings.TrimSpace strips \r like Python .strip().
The valid→invalid guard on a CRLF manifest leaves the ORIGINAL CRLF bytes
untouched on disk on both sides (both refuse before writing).

RED (4 new differential cases, before implementation):

```
go test ./internal/config/... -run TestDifferentialWrites
  --- FAIL: .../remove_recipe_from_CRLF_manifest_(output_LF-normalized)
  --- FAIL: .../remove_dep_first-of-two_from_CRLF_manifest
  --- FAIL: .../append_to_CRLF_manifest
  --- FAIL: .../append_to_CRLF_manifest_without_trailing_newline
  (file bytes diverge: python "a = 1\nb = 2\n\n[[deps]]..." vs go "a = 1\r\nb = 2\r\n\n[[deps]]...")
```

GREEN:

```
go test ./internal/config/... -run TestDifferentialWrites -count=1
  ok  ai-specs.dev/ai-specs/internal/config 1.089s
```

New fixtures (real CRLF bytes): write_recipe_crlf.toml, write_recipe_crlf_guard.toml
(guard fires; file untouched on both sides — passes before and after the fix,
pinning untouched-CRLF behavior), write_dep_crlf.toml, plus two inline CRLF
append cases (with and without trailing newline).

(b) Read-corpus skip whitelist (differential_test.go TestDifferentialReadSections):
any non-zero python exit is no longer logged-and-skipped. Whitelist is the
tight, explicit `strings.Contains(name, "invalid_base")` — for those, Go
LoadManifest is asserted to fail too (both-fail parity); any OTHER non-zero
python exit now t.Errorf and FAILs loudly. Dead `skips` slice removed.

GREEN:

```
go test ./internal/config/... -run TestDifferentialReadSections -count=1 -v
  differential read: compared 174 (file, section) pairs; 12 whitelisted
  invalid-base entries asserted both-fail
  ok  ai-specs.dev/ai-specs/internal/config 6.854s
```

(compared rose 156 → 174: the 3 new CRLF fixtures × 6 sections; the former
12 silent skips are now explicit both-fail assertions.)

#### Verification (observed exit codes)

```
CGO_ENABLED=0 go build ./...                          → exit 0
go vet ./internal/toml/... ./internal/config/...      → exit 0
go test ./internal/toml/... ./internal/config/... -count=1 → exit 0 (toml ok 1.178s, config ok 7.526s)
go test ./... -count=1                                → exit 0 (cli, config, home, lock, schema, target, toml all ok)
```

### JD round 1 — parity fixes (lock pyStr / schema+target repr / symlink containment / recursion)

Scope: internal/lock, internal/schema, internal/target only. Python probes
(python3 3.14.7) pinned: repr() quote switching + `\xNN` (0x00-0x1f,
0x7f-0x9f) / `\uNNNN` / `\UNNNNNNNN` for non-printables (NBSP, U+2028
escaped; é/emoji raw); str(float) shortest form; nested-list repr succeeds
to depth ~69709 and raises RecursionError at ~69710 on the default 8 MiB
stack; self-referential containers render `[[...]]`/`{...}` ellipsis;
shared (acyclic) containers render in full.

#### Fix A — internal/lock pyStr non-string formatting

RED (differential, TestWriteLockDifferential new cases + json.loads-parity
spec conversion in lockFromSpec):

```
go test ./internal/lock -run 'TestWriteLockDifferential/(agents_hash_scalar_types|agents_hash_list_and_dict|deps_hash_non-string|managed_fields_non-string|managed_sha256_falsy|agents_hash_falsy)' -count=1
  agents_hash_scalar_types:  byte mismatch go "true"/"false"/"100"/"-0" vs py "True"/"False"/"100.0"/"-0.0"
  agents_hash_list_and_dict: go write failed: value for agents hash contains a control character (python succeeded)
  deps_hash_non-string:      go write failed: ... control character (python succeeded)
  managed_fields_non-string: byte mismatch go kind="true"/policy="false" vs py "True"/"False"
  managed_sha256_falsy:      go wrote zero/false/empty entries, python skipped them (plain truthiness gate)
  agents_hash_falsy:         byte mismatch go "false" vs py "False"
```

GREEN:

```
go test ./internal/lock -count=1 → ok 1.649s (all differential + refusal + atomicity tests)
```

Implementation: pyStr → (string, bool, error) with bool→True/False, int64
decimal, float64 via package-local pyFloatStr (config copy, no internal
import), []any / []*toml.Table / *toml.Table (document-order Keys()) /
map[string]any (sorted; Table.Any() normalizes away document order —
noted divergence) through a repr-composing pyReprDepth; pyReprString with
probe-pinned escaping; pyTruthy for the Python `entry.get("sha256")`
plain-truthiness gate (False/0/[]/{} now skipped on both sides). Note:
meta.cli_version/synced_at stay string-only by the Go Lock type AND by
Python load_lock (isinstance str) — non-strings cannot reach that slot on
either side, so the slot was covered by strings only.

#### Fix B — internal/schema pyRepr string escaping

RED (6 new update_policy fixtures driven through driver.py):

```
go test ./internal/schema -run 'TestDifferentialAgainstPython/err_templates_policy_(apostrophe|quote|both_quotes|backslash|controls|unicode)' -count=1
  6× error mismatch: go 'au'to' vs py "au'to"; go 'back\\slash' vs py 'back\\\\slash';
  go raw \x01/\x7f/\x85/\n vs py '\x01'/'\x7f'/'\x85'/'\n'
```

GREEN:

```
go test ./internal/schema -count=1 → ok 5.373s
  differential: clean corpus 21 recipes ok both sides; fixtures 92 byte-identical (20 ok, 72 error)
```

Fixtures: err_templates_policy_{apostrophe,quote,both_quotes,backslash,
controls,unicode}.toml. pyReprString implements quote switching, \\ \n \r
\t + active-quote escapes, and \x/\u/\U lowercase-hex escapes for every
non-printable rune (unicode.IsPrint boundary).

#### Fix C — schema symlink containment (Path.resolve parity)

RED (empirical differential; both Python and Go run against the same temp
recipe dir with a symlinked intermediate component):

```
go test ./internal/schema -run 'TestDifferentialAgainstPython/(err_hooks_escape_symlink|ok_hooks_symlink_inside|err_init_prompt_symlink_escape|ok_init_prompt_symlink_inside)' -count=1
  err_hooks_escape_symlink.toml:     python errored but go succeeded
    py: [provides].hooks[0].script: hook script paths must stay inside the recipe directory (got 'hooks/x.sh')
  err_init_prompt_symlink_escape.toml: python errored but go succeeded
    py: [init].prompt: init prompt paths must stay inside the recipe directory
  (both symlink-to-INSIDE ok-cases passed on both sides: 2 ok)
```

GREEN:

```
go test ./internal/schema -count=1 → ok 5.373s (all 4 symlink fixtures byte-identical)
```

resolveDir replaced by resolvePy (Path.resolve(strict=False) port: deepest
existing ancestor resolved via EvalSymlinks, dangling final symlink
followed, remainder re-joined; 40-level MAXSYMLINKS cap). Runtime-hook and
init-prompt containment now compares resolvePy(recipeDir) vs
resolvePy(recipeDir/script); init stats the RESOLVED target like Python's
target.exists()/is_file(). Setup helpers in differential_test.go now take
t to allocate outside dirs.

#### Fix D — internal/target pyReprString control chars

RED:

```
go test ./internal/target -run TestPyReprStringDifferential -count=1
  7 mismatches: \x01 \x1f \x7f \u0085 \u009f \u00a0 \u2028 emitted raw, want \xNN/\uNNNN escapes
  (raw-byte \x85/\x9f argv cases dropped: surrogateescape artifact, not TOML-reachable)
```

GREEN:

```
go test ./internal/target -count=1 → ok 3.768s
```

#### Fix E — recursion probes (all three packages)

Probes: python3 repr() of nested lists — ok to ~69709, RecursionError at
~69710 (8 MiB stack); self-referential list/dict render `[[...]]`/`{...}`;
shared acyclic containers render in full.

RED (target — stack overflow via mutual pyStr↔pyRepr recursion on
*toml.Table, reachable through worktrees_dir = {a = 1}):

```
go test ./internal/target -run 'TestDifferentialPlanJSON/worktrees-dict' -count=1
  runtime: goroutine stack exceeds 1000000000-byte limit
  fatal error: stack overflow   (exit status 2)
```

RED (schema — giant message instead of a recursion error):

```
go test ./internal/schema -run TestPyReprDepthCapGoOnly -count=1
  depth-cap error = "[provides].templates[0].update_policy: invalid value [[[[..." (want recursion-depth message)
```

GREEN (all three packages share the same design; separate copies per
package by module convention): pyReprDepth core with pyReprMaxDepth =
100000 (probe-pinned, above the observed Python failure point so Go never
errors where the probe machine's Python succeeds) returning a descriptive
`value nesting exceeds the maximum recursion depth (100000)` error, and a
`{...}` ellipsis for *toml.Table re-entered on the active path (Python
parity; not producible by toml.Parse, pinned via pre-seeded active set).
pyRepr/pyStr signatures now carry the error and every call site propagates
(schema: pyStr callers + update_policy repr; target: normalizeDeclaredRelpath
plain error + worktreesDir → ResolveTargetPlan; lock: refusal walk +
renderLock → WriteLock).

```
go test ./internal/target -run 'TestPyRepr' -count=1 → PASS (string differential, nested depth-1000 differential vs python3, depth cap + ellipsis)
go test ./internal/lock -run TestPyReprUnitsGoOnly -count=1 → PASS (composition, probe pins, depth cap, ellipsis)
go test ./internal/schema -run TestPyReprDepthCapGoOnly -count=1 → PASS
```

#### Verification (observed exit codes)

```
CGO_ENABLED=0 go build ./...                                          → exit 0
go vet ./internal/lock/... ./internal/schema/... ./internal/target/... → exit 0
go test ./internal/lock/... ./internal/schema/... ./internal/target/... -count=1 → exit 0 (lock 1.704s, schema 5.620s, target 3.768s)
go test ./... -count=1                                                → exit 0 (cli, config, home, lock, schema, target, toml all ok)
```

Differential counts (schema TestDifferentialAgainstPython): clean corpus 21
recipes ok both sides; fixtures 92 byte-identical (20 ok, 72 error). Lock
differential: 23 spec cases + round-trip, byte-equal incl. refusal parity.
Target differential: 23 plan subtests incl. worktrees-dict + symlink classes,
byte-equal stdout/stderr/exit.

## JD round-1 verification (final gate, observed)

Documentation: ADR 0002 (`docs/go-adr/0002-toml-line-editor.md`) updated with
the empirically pinned tomllib line-ending semantics (\r\n accepted everywhere,
lone \r rejected everywhere, CRLF in multi-line string values normalized to \n)
and the write-ops universal-newline read translation (\r\n and lone \r → \n in
memory, CRLF manifests round-trip LF-normalized, guard-refused writes leave
original bytes untouched). Consequences section had no conflict; left as-is.
Formatting: gofmt flagged four files; gofmt applied. Three were pure
alignment/import-order fixes; `internal/toml/value_parse.go` was flagged only
for a doc comment (`'''...'''`), which gofmt would smart-quote and corrupt —
the comment was reworded ("delimited by three single quotes") so gofmt output
is clean without changing the documented meaning. `gofmt -l internal cmd` is
now empty.

```
CGO_ENABLED=0 go build ./...   → exit 0
go vet ./...                   → exit 0
go test ./... -count=1         → exit 0 (cli 2.245s, config 8.746s, home 0.518s, lock 2.052s, schema 6.466s, target 4.241s, toml 1.464s)
./tests/run.sh                 → exit 0; unittest phase final line:
  Ran 2358 tests in 1937.445s
  OK (skipped=164)
```
