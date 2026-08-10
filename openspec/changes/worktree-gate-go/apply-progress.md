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

Phases 3-4 (launcher, distribution, doctor, docs, cutover) and the V.*
verification checklist are NOT started — the assignment authorizes Phase 0-2
only. No commit or push was performed; all persistent changes remain under the
authorized worktree `.worktrees/worktree-gate-go`.
