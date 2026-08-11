# Apply Progress: worktree-gate-go

## Phase 0-2 reconciliation (2026-08-10)

All Phase 0 (infrastructure), Phase 1 (frozen reference + parity oracle) and
Phase 2 (Go implementation to parity) tasks are implemented and covered by
focused evidence. Phase 3+ is intentionally not started (per assignment: do
not begin Phase 3+).

## Phase 0 — Infrastructure (tasks 0.1-0.10)

- `catalog/recipes/worktree-flow/gate/go.mod` — module `ai-specs.dev/worktree-gate`,
  `go 1.22`, no `require` block (zero third-party deps, D8).
- `gate/main.go` — full flag surface (`--gate-mode`, `--gate-scope`,
  `--repo-topology`, `--protected`, `--version`, `--selftest`, `--explain`,
  `--tokenize`); unknown flags warn and fail open (exit 0).
- `--selftest` compiles every regex + checks git, exits 1 on failure.
- `scripts/build-gate.sh` — 4-target matrix (`darwin/arm64`, `darwin/amd64`,
  `linux/amd64`, `linux/arm64`), `CGO_ENABLED=0 -trimpath -buildvcs=false`,
  version from `VERSION`, fails loudly without go. Emits
  `dist/worktree-gate-<os>-<arch>` **and** `dist/worktree-gate-current` (native)
  so the differential runners always test a fresh binary.
- Reproducibility (0.5): double build of linux/amd64 → identical SHA-256
  (verified: 1 unique digest across two builds).
- CI workflow `.github/workflows/release-worktree-gate.yml` (0.6): builds all
  four targets on tag push, runs `go vet` + `go test ./...`, emits SHA256SUMS,
  attaches assets.
- `catalog/recipes/worktree-flow/bin/SHA256SUMS` committed as trust root (0.7)
  + `README.md` stating no binaries are ever committed.
- `.gitignore` has `dist/` (0.8).
- `tests/run.sh` runs `go test ./catalog/recipes/worktree-flow/gate/...` and
  `tests/validate.sh` adds `gofmt -l` — both guarded by `command -v go`
  (0.9).

## Phase 1 — Frozen reference and parity oracle (tasks 1.1-1.19, 1.20-1.23)

- `catalog/recipes/worktree-flow/hooks/worktree-gate-legacy.sh` frozen from
  `development` @ `e080483` (URI allowlist, event-cwd precedence, bash-3.2
  fix all inside). Byte-identity pinned by
  `test_frozen_reference_hash_is_pinned` (SHA-256
  `1ee9da4a…eabe76`, task 1.2) — PASS.
- `tests/fixtures/worktree-gate-corpus/` — 16 hermetic cases shaped
  `{name, event, fixture, expected_exit, expected_stderr, target}` (1.3-1.15):
  path mode, shell mode, scrub rules, wrapper prefixes, command-source
  precedence, URIs, `.claude` exceptions, topology, fail-open set.
- `tests/test_worktree_gate_parity.py` (1.16): every corpus case runs against a
  real Git fixture on **both** implementations; asserts identical
  `(exit_code, stderr, candidates)` AND each matches the corpus `expect`.
  The Go half keys off `dist/worktree-gate-*` and skips loudly when no binary
  exists (1.17); the Bash-vs-expect half always runs.
- `tests/test_worktree_gate_tokenizer.py` (1.18): 108 pinned shlex cases fed to
  the Go binary via `--tokenize`; asserts token-for-token equality and
  `ValueError → error=true` (fail-open verdict).
- Correction tasks 1.20-1.23: no illustrative `/repo` paths (validated by
  `test_no_illustrative_repo_paths_in_corpus`), fixture builder for
  protected/feature/external/linked-worktree, Bash reference runner against
  hermetic fixtures, malformed-fixture negative coverage.

## Phase 2 — Go implementation to parity (tasks 2.1-2.18)

Tasks 2.1-2.15 implemented: `config.go` (mode/scope/topology precedence +
verbatim warnings, off short-circuits), `event.go` (stdin JSON parse, path vs
shell mode, cwd validation), `tokenize.go` (shlex-equivalent state machine),
`extract.go` pass 1 + pass 2 (Go-side quote pairing, D10), `pathutil.go`
(RealPath/Inside/ExistingAncestor), `gitfacts.go` (memoized git wrapper,
`--path-format=absolute` fallback), `topology.go` (proven module records,
ambiguity → unproven), `decide.go` (owner × scope with openspec/changes
exception), `message.go` (byte-identical block/warning strings), `main.go`
(pipeline + `--explain` JSON diagnostic).

### Tokenizer differential fixed (2.3/2.4/2.16)

The Go tokenizer originally diverged from python3 shlex in four families:

1. `#` was treated as a comment starter — shlex treats it as an ordinary
   character (`echo #foo` → `['echo', '#foo']`). Fixed: `#` is never special.
2. Outside quotes, backslash escaped only a fixed set — shlex escapes ANY
   character. Fixed to match.
3. Inside double quotes, `\$`, `` \` ``, `\newline` were consumed — shlex
   keeps the backslash literally before everything except `"` and `\`
   (escapedquotes = `"`). Fixed.
4. Whitespace-only input returned `nil` tokens instead of `[]`. Fixed.

Result: `tests/test_worktree_gate_tokenizer.py` — 3 passed, 226 subtests
(all 108 corpus cases × pin/differential/error-verdict assertions) GREEN.

### Parity gap closed: internal URI allowlist (1.12/2.16)

The Go implementation had no internal-URI allowlist: every `xd://`, `skill://`
etc. write blocked (exit 2) where the Bash reference allows (exit 0). Added
`uri.go` with `IsInternalURI(candidate, mode)` mirroring
`worktree-gate-legacy.sh:339-352` exactly: twelve schemes, PATH mode only,
traversal-masked (`/../`) and absolute-path-masked (`/` after scheme) variants
stay gated, SHELL mode never allowlisted. Unit tests in `uri_test.go` cover
all twelve schemes, shell-mode negatives, unknown schemes, traversal and
absolute-masked variants — PASS.

### Parity gap closed: topology proof (2.12/2.16)

`moduleRecords` returned submodule paths without proof. Rewrote to mirror
`module_records()` (`worktree-gate-legacy.sh:412-449`): require a real `.git`
(dir or file), RealPath the superroot (the `/var` → `/private/var` symlink
differential), reject duplicate/nested/outside registrations as ambiguity
(nil), require initialized submodule status (no `-` prefix), git-common-dir ==
`.git/modules/<rel>`, and owner == module. `classify` now walks ALL ancestors
and yields unproven on >1 matches, subrepo on exactly 1, superrepo when the
repo itself has records — matching the reference. Tests rewritten to prove
against a `file://` clone-based submodule (the only layout that materializes
`.git/modules/<rel>`, exactly as the reference proves) plus fake-.gitmodules
and ambiguous-nested negatives.

### Hook suite parameterized over both implementations (2.17)

`tests/test_worktree_gate_hook.py` gained `impl` dispatch: the same 78
scenarios run against the Bash reference (`WorktreeGateHookTests`, impl=bash)
and against the Go binary (`WorktreeGateGoHookTests`, impl=go) via
launcher-equivalent flags, with a loud skip guard when no binary exists. The
parameterization is what surfaced the URI and topology differentials above.

Result: **78 Bash + 78 Go = 156 hook scenarios PASS** (plus the parity,
tokenizer and metrics suites).

### Performance evidence (2.18)

Measured over corpus cases 01-04 (protected-main, feature-branch,
development-branch, claude-settings), 5 runs each after warm-up:

| Implementation | Per-invocation (median) |
|---|---|
| Go binary | **48.5 ms** |
| Bash reference | **145.3 ms** |

Go is ~3× faster per invocation. Git call counts: `gitfacts.go` memoizes
git facts keyed by resolved directory (`gitMemo`); `gitfacts_test.go` pins the
memoization contract. `tests/test_worktree_gate_metrics.py` records Go-side
per-invocation measurements over the corpus — PASS.

## Final focused evidence (all green)

```
go -C catalog/recipes/worktree-flow/gate test ./...   PASS
go -C catalog/recipes/worktree-flow/gate vet ./...    PASS
gofmt -l catalog/recipes/worktree-flow/gate           (clean)
python3 -m pytest tests/test_worktree_gate_tokenizer.py tests/test_worktree_gate_parity.py tests/test_worktree_gate_metrics.py tests/test_worktree_gate_hook.py
  167 passed, 1 skipped (intentional skip-guard), 277 subtests
```

## Scope

Phases 0-3 (infrastructure, frozen reference + parity oracle, Go
implementation, distribution + configuration) are implemented and covered by
focused evidence. Phase 4 (harness coverage, docs, cutover) and the V.*
verification checklist are NOT started — the assignment authorizes Phase 3
only. No commit or push was performed; all persistent changes remain under the
authorized worktree `.worktrees/worktree-gate-go-phase-3`.


## Phase 3 — Distribution and configuration (tasks 3.1-3.20)

### Launcher (3.1-3.6)

`catalog/recipes/worktree-flow/hooks/worktree-gate.sh` is now a thin launcher
(~150 lines, bash 3.2 only): stamps mode/scope/topology/impl/version, resolves
the implementation in order `$WORKTREE_GATE_BIN` → project-local
`ai-specs/recipes/worktree-flow/bin/worktree-gate` → version-keyed cache →
legacy Bash (when `gate_impl` permits) → one stderr warning + exit 0. Handoff
is `exec` (stdin and exit code untouched); `WORKTREE_GATE_VERIFY=1` opts into a
per-invocation `--selftest`. The literal `stamped_gate_scope="` sentinel is
kept so the materializer upgrades existing projects. Platform detection maps
`aarch64`→`arm64` and `x86_64|amd64`→`amd64`; unknown platform → empty target.
`bash -n` clean; `test_launcher_keeps_literal_staleness_sentinel` scans
non-comment lines for `mapfile`/`readarray`/`declare -A`/`,,`.

### Config + materialization (3.7-3.9, 3.18-3.19)

`recipe.toml` gains `[config.gate_impl]` (`enum = ["auto","go","bash"]`,
`default = "auto"`, help text) and the recipe version bumps to `1.5.0`.
`recipe-materialize.py` stamps `__WORKTREE_GATE_IMPL__` (validated at sync
against the enum, invalid → `RuntimeError` listing `auto | go | bash`) and
`__WORKTREE_GATE_VERSION__` (from the installed CLI `VERSION`), and
materializes the frozen Bash reference to
`ai-specs/recipes/worktree-flow/hooks/worktree-gate-legacy.sh` so
`gate_impl=bash` works offline with no binary. Sentinel-upgrade test proves a
pre-Go materialized gate is replaced by the launcher, not preserved.

### Acquisition + cache (3.10-3.13)

New `lib/_internal/gate_binary.py`: `detect_platform()`, version-keyed cache
path `$AI_SPECS_HOME/cache/bin/worktree-gate/<version>/<goos>-<goarch>/`,
digest-verified download (temp file → SHA-256 vs committed `SHA256SUMS` →
`chmod 0755` → atomic `os.replace` → `--selftest`), opt-in local build
(`AI_SPECS_GATE_BUILD=1`, or offline with `go`), mismatch recording for
doctor, and never-fails-sync degradation (`auto` → legacy fallback WARN;
`go` → fail-open; unsupported platform → WARN). Wired into
`materialize_recipes` after the hook materialization loop.

### Doctor check (3.14)

`doctor.py` gains `_check_worktree_gate` with the design §6.5 severity table:
OK (binary + version match + selftest), INFO (`gate_impl=bash`), WARN
(`auto` fallback / version mismatch), ERROR (`go` failing open / recorded
digest mismatch), wired into `Doctor.run()`.

### Tests (3.15-3.17, 3.20)

- `tests/test_gate_binary_dist.py` (13 tests): uname mapping incl. Rosetta,
  cache path, digest match → install, mismatch → no install + recorded,
  partial download → never installed, offline/auto → legacy WARN, offline/go →
  fail open, unsupported platform → WARN, bash skips acquisition,
  never-raises.
- `tests/test_worktree_gate_dist_config.py` (9 materialize + 1 rollback
  corpus): gate_impl enum/default, stamping, legacy materialization, invalid
  gate_impl rejection (with enum in message), sentinel upgrade, launcher
  bash-3.2 cleanliness, rollback rehearsal (`gate_impl=bash` answers the full
  16-case parity corpus through the materialized legacy copy).
- `tests/test_doctor_worktree_gate.py` (7 tests): all five severity rows.
- `tests/test_worktree_gate_hook.py`: the Bash parameterization now runs the
  frozen reference (`worktree-gate-legacy.sh`) instead of the launcher —
  the launcher delegates, so the reference contract is pinned on the legacy
  copy. 78 Bash + 78 Go scenarios PASS.
- Smoke (3.20): a scratch project synced with `AI_SPECS_GATE_BUILD=1`
  materializes a stamped launcher (`always/auto/auto/auto/0.21.0`), builds the
  cache binary, and the launcher blocks a protected-branch write (exit 2) via
  the cache binary, `WORKTREE_GATE_BIN`, and the legacy fallback path.

### Focused evidence (all green)

```
python3 -m pytest tests/test_worktree_gate_dist_config.py tests/test_gate_binary_dist.py \
  tests/test_doctor_worktree_gate.py tests/test_worktree_flow_recipe.py
  45 passed, 22 subtests
python3 -m pytest tests/test_worktree_gate_hook.py
  156 passed (78 Bash + 78 Go)
python3 -m pytest tests/test_worktree_gate_parity.py tests/test_worktree_gate_tokenizer.py \
  "tests/test_sync_pipeline.py::RuntimeHookSyncPipelineTests"
  11 passed, 1 skipped (intentional skip-guard), 277 subtests
python3 -m pytest tests/test_doctor.py
  81 passed
go -C catalog/recipes/worktree-flow/gate test ./...   PASS
```

## Scope

Phase 4 (harness coverage, docs, cutover) and the V.* verification checklist
are NOT started — the assignment authorizes Phase 3 only. No commit or push
was performed; all persistent changes remain under the authorized worktree
`.worktrees/worktree-gate-go-phase-3`.


## Phase 4 — Harness coverage, docs and cutover (tasks 4.1-4.12)

### Harness coverage tests (4.1-4.4)

New `tests/test_worktree_gate_harness_phase4.py` (10 tests, 10 subtests) proves
the launcher changed nothing for the five harnesses:

- 4.1 `hooks-render.py` output byte-identity: the resolved-hooks document is
  rendered twice in fresh projects and bytes are identical per harness, and
  every artifact references the unchanged materialized path
  `ai-specs/recipes/worktree-flow/hooks/worktree-gate.sh`.
- 4.2 Cursor wrapper: blocked shell write → `{"permission":"deny"}` through the
  launcher; allowed linked-worktree write → `allow`; empty binary stdout does
  not degrade the deny decision (message travels on stderr, deny JSON intact).
- 4.3 spawnSync contract: the materialized launcher is directly executable
  (shebang + mode 0755) and, run with no shell, passes stdin through, exits 2
  with empty stdout on a blocked write.
- 4.4 Live launcher smoke on real Git fixtures: blocked write on a protected
  branch, allowed write inside a linked worktree, and `auto` fallback to the
  frozen Bash reference when no binary is reachable.

### Release gate and digests (4.8, 4.9)

New `tests/test_worktree_gate_release_phase4.py` (6 tests, 12 subtests,
skip-guarded on `go` presence):

- darwin/arm64 asset built with CI-identical flags executes on Apple Silicon
  and `--selftest` prints `ok`; `--version` matches `VERSION`.
- Repeated builds byte-identical (linux/amd64 rebuilt → same SHA-256).
- Every matrix target has a committed digest entry; committed digests match
  the locally built assets; the CI-generated sums file parses and matches.
- `catalog/recipes/worktree-flow/bin/SHA256SUMS` **regenerated** with the real
  digests of all four targets (task 4.9; was the Phase-0 placeholder).

### Docs and release (4.5-4.7, 4.10-4.11)

- `docs/runtime-hooks.md`: new "Gate implementation and launcher" section —
  Go binary, launcher resolution order, cache layout, build matrix, `gate_impl`
  semantics, `WORKTREE_GATE_VERIFY`, zero renderer churn, and the **unchanged
  pre-existing coverage gaps** (Cursor no pre-file-write hook; opencode misses
  subagent/MCP; pi/omp per-process).
- `catalog/recipes/worktree-flow/README.md`: `gate_impl` section with offline
  behavior, rollback levers, `ai-specs doctor` guidance, digest trust root;
  Enable block version → 1.5.0 with `gate_impl`; config table row.
- `docs/recipes-catalog.md`: worktree-flow section — gate implementation
  bullet, `gate_impl` config row, version pin → 1.5.0.
- Contributor build path (task 4.7) documented in the gate README section and
  `scripts/build-gate.sh` header (`go >= 1.22` for contributors only, never a
  user prerequisite; `AI_SPECS_GATE_BUILD=1` for offline users).
- Cutover (4.10): `gate_impl` default is already `auto` in `recipe.toml`
  (Phase 3); confirmed by `test_recipe_declares_gate_impl_enum_with_default_auto`.
- `CHANGELOG.md` Unreleased: Added section for the Go gate, launcher,
  `gate_impl`, acquisition/verification/cache, doctor check, build matrix,
  performance; Changed: worktree-flow 1.4.0 → 1.5.0. `VERSION` → 0.22.0.
- Follow-up slug (4.12): `openspec/changes/worktree-gate-bash-retire/proposal.md`
  with entry criterion (one minor release with Go as default and no field
  regression) and draft removal scope.

### Focused evidence (all green)

```
python3 -m pytest tests/test_worktree_gate_harness_phase4.py
  10 passed, 10 subtests
python3 -m pytest tests/test_worktree_gate_release_phase4.py tests/test_gate_binary_dist.py
  18 passed, 18 subtests
python3 -m pytest tests/test_worktree_gate_dist_config.py tests/test_doctor_worktree_gate.py \
  tests/test_worktree_flow_recipe.py tests/test_doctor.py
  126 passed, 22 subtests
python3 -m pytest tests/test_worktree_gate_hook.py
  156 passed (78 Bash + 78 Go)
python3 -m pytest tests/test_worktree_gate_parity.py tests/test_worktree_gate_tokenizer.py \
  tests/test_worktree_gate_metrics.py
  11 passed, 1 skipped (intentional), 277 subtests
python3 -m pytest tests/test_hooks_render.py tests/test_sync_pipeline.py
  98 passed
go -C catalog/recipes/worktree-flow/gate test ./...   PASS
go -C catalog/recipes/worktree-flow/gate vet ./...    PASS
gofmt -l catalog/recipes/worktree-flow/gate           (clean)
```

## Scope

Phase 4 (harness coverage, docs, cutover) is complete. The V.* verification
checklist is intentionally NOT started per assignment (leave ready for Verify
by phases); no commit or push was performed. All persistent changes remain
under the authorized worktree `.worktrees/worktree-gate-go-phase-4`.
