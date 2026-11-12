# GO-20 U2e T5 PR4a S4 — refreshbundled live oracle

Base: epic/go-single-binary 19f0fa4. Branch: change/go-20-u2e-t5-pr4a-s4-refreshbundled.
Goal: `internal/sync/refreshbundled_test.go` stops exec'ing python3 and reading
`lib/_internal/refresh-bundled.py` without silent coverage loss. Same discipline as PR #496 (doctor),
PR #497 (rulesaudit), PR #498 (skills), PR #499 (conflicts), PR #501 (briefpolicy), PR #502 (checksync),
PR #503 (envscaffold).

## Specs

- S1: `TestRefreshBundledDifferential` (same name, no longer a live python differential) reads a frozen golden
  (`internal/sync/testdata/refreshbundled_golden.json`) captured once from
  `python3 lib/_internal/refresh-bundled.py <project> <home>` over the same six fixtures the live differential
  built. No python3, no `lib/` access, no skip: a missing golden fails.
- S2: Every native assertion of the live test survives case by case — the exit code, the stdout lines
  (`⇢ flattened N bundled skill(s) → cache/.bundled/skills`, the command twin, `= all bundled up-to-date`, the
  two `✓ removed leftover …` lines), the stderr byte for byte (including
  `ERROR: <DIR>/ai-specs not found. Run \`ai-specs init\` first.`), the produced project tree, and the produced
  cache tree.
- S3: The test asserts a hard-coded independent inventory of the case names (count, uniqueness, order) against
  both the in-test corpus and the golden before comparing anything. A golden that drops, duplicates or reorders a
  case fails as an inventory error.
- S4: Frozen inputs equal the in-test corpus: each case pins its pre-run project tree and pre-run cli-home tree
  (paths plus bytes), and the test snapshots the trees its own `setup` just wrote and compares them before
  running `RefreshBundled`, so fixture drift fails as input drift, not as an opaque payload mismatch.
- S5: No absolute host path leaks into the golden: the temporary project root and cli home are tokenized as
  `<DIR>` (reusing `tokenizeCheckSyncDirs` from `checksync_test.go`, read-only, which also covers the macOS
  `/var` → `/private/var` resolution), the per-project cache key is tokenized as `<CACHE>`, and every frozen path
  is root-relative (enforced by the loader). `grep -c "/Users/\|/private/\|/var/folders"` over the golden is 0.
- S6: Negative controls: mutating one frozen result, deleting a case, duplicating a case, and drifting a frozen
  input each fail rc=1.
- S7: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, the test free of live `python3` /
  `exec.Command` / `exec.LookPath` / `lib/_internal` (only the provenance comment stays),
  `go test ./internal/sync/ -run TestRefreshBundled -count=1` rc=0 (also under `umask 077`) and
  `go test ./internal/sync/ -count=1` rc=0.
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestRefreshBundledDifferential` (`python3 lib/_internal/refresh-bundled.py <project> <home>` versus the native `RefreshBundled`, compared on rc, stdout, stderr, the project tree and the cache tree) | FREEZE | the payload is host-independent once the two tokenizations below are applied. Every stdout line is fixed text; the one stderr shape names the resolved project root (tokenized `<DIR>`); the project tree is the fixture manifest plus the rewritten lock, whose `[meta] synced_at` is *fixture* data (`2026-01-01T00:00:00Z`) and never wall-clock — `lock.WriteLock` writes the loaded `[meta]` verbatim and `refresh-bundled` never stamps a timestamp; the cache tree is already compared relative to the per-project cache root, so the key never reaches a frozen path. RETIRE was rejected: no other frozen test covers this surface (the `internal/cli` tests only pin the route stub and init's call site, `RefreshBundled`'s own cache flatten, leftover removal and lock normalization have no frozen oracle). RETARGET was rejected for the same reason: no frozen Go-side coverage exists to cite. The live leg ran exactly these six fixtures, so the freeze is 1:1 — no case added and none dropped. |

Two normalizations, both documented in the test and in the golden's `oracle.note`:

- **Tokenization.** `<DIR>` replaces the temporary project root and cli home in stdout/stderr (both as created
  and as symlink-resolved, because `RefreshBundled` resolves both); `<CACHE>` replaces the single
  `cache/projects/<key>` path segment in the frozen pre-run home tree, because `CacheKey` derives from the
  project's absolute temp path.
- **Modes.** Regular-file modes are frozen (they are set explicitly: fixture files, `copystat`, and `CreateTemp`'s
  `0o600` lock, all umask-independent). Directory modes are **not** frozen: the cache tree is built with
  `MkdirAll(0o777)` and `copytree`'s `copystat`, so its directory modes follow the process umask, and this package
  also runs `t.Parallel()` tests that must not have the umask pinned under them. Directory *paths* are still
  frozen, so a created or missing directory still fails. This is the one narrowing against the old differential
  (which compared directory modes between two legs sharing one umask); it is proven umask-independent by the
  `umask 077` run in L11. That run also exposed that the fixture's own `0o755` was being masked by the umask, so
  `writeRefreshFile` now `chmod`s each fixture file to its intended mode — without that, the frozen file modes
  would have been capture-umask-dependent.

Shapes the live leg never exercised stay untouched: the fixture helpers are shared with `vendorskills_test.go`
(the next slice), so `writeRefreshFile`, `bundledAssets`, `refreshCacheRoot`, `snapshotRefreshTree`,
`compareRefreshTree`, `stripBridgeFallback` and `withoutEnv` all stay in place for that leg.

Cases (6): `fresh project`, `all bundled up to date`, `stale cache rebuilt`,
`legacy copies removed and lock normalized`, `lock without meta`, `missing ai-specs directory` — 6 frozen records.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6 | inline | |
| T2 | S7, S8 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then spine+syncagent, then the materialize-helper consumers; one worker at a time, merge after
  validate.
- L2: Baseline at 19f0fa4: the live python differential is green, all six subtests pass
  (`logs/01-baseline-live-leg.log`, rc=0).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-refreshbundled (Robert's
  standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing at its `tmp/`.
- L4: RED before the golden existed: `read frozen oracle: open testdata/refreshbundled_golden.json: no such file
  or directory`, rc=1 (`logs/02-red-missing-golden.log`).
- L5: Capture harness was temporary code in the test file, gated by `REFRESHBUNDLED_CAPTURE=<path>` and removed
  before the commit, so the golden's normalization is byte-for-byte the same code the frozen test compares with.
  It rebuilt each case's fixture from the same corpus, ran the real `lib/_internal/refresh-bundled.py <project>
  <home>` with `PYTHONDONTWRITEBYTECODE=1` and without `WORKTREE_GATE_BIN` (the same invocation the live leg
  used), froze the pre-run project and home trees alongside the legacy output, stripped the lock writer's
  `GO_LOCK_WRITE_BRIDGE_FALLBACK` line (Python bridge infrastructure, not refresh behaviour), and failed the
  capture if an absolute host path survived tokenization. So the golden is legacy output, not port output.
- L6: Captured twice into fresh scratch subdirs and `cmp` byte-identical (`logs/05-capture-cmp.log`, rc=0 —
  `logs/03-capture1.log`, `logs/04-capture2.log`). First attempt differed on the `cache/projects/<key>` path
  segment of the pre-run home tree; `refreshBundledHomeInput` now tokenizes it as `<CACHE>` for the directory
  node too. Oracle metadata `python: 3.14.7`. Golden size: 17134 B. No wall-clock value is frozen.
- L7: Negative controls (each rc=1; golden restored and `cmp`-verified byte-identical after each): mutate one
  frozen stdout → only the stdout comparison fails (`logs/07-nc1-mutate-result.log`), delete a case →
  `refreshbundled: 5 records, want 6` (`logs/08-nc2-delete-case.log`), duplicate a case →
  `refreshbundled: 7 records, want 6` (`logs/09-nc3-duplicate-case.log`), drift the frozen legacy lock body of
  the leftover case → `project input: ai-specs/.ai-specs.lock differs` (`logs/10-nc4-input-drift.log`).
- L8: `gofmt -l internal/sync` empty (`logs/11-gofmt.log`), `go vet ./internal/sync/` rc=0
  (`logs/12-vet.log`), `grep -n "exec\.Command\|exec\.LookPath\|os/exec\|python3\|lib/_internal"` over the test
  matches one comment line only and the host-path grep over the golden is 0 (`logs/13-grep-live-legs.log`).
- L9: First `umask 077` run failed on the frozen `bundled-skills/alpha/run.sh` mode: the fixture's
  `os.WriteFile(…, 0o755)` was masked to `0o700`, so the *input* mode (and therefore the copied cache mode) was
  capture-umask-dependent. `writeRefreshFile` now `chmod`s to the requested mode; the live leg is unaffected
  because both of its legs used the same helper.
- L10: `-run TestRefreshBundled` rc=0 (`logs/06-green-focused.log`), the same under `umask 077` rc=0
  (`logs/14-green-umask077.log`), `go test ./internal/sync/ -count=1` rc=0 (`logs/16-green-package-tests.log`).
- L11: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/17-build.log`), `./tests/validate.sh` rc=0
  (`logs/18-validate.log`): `Ran 2345 tests in 720.370s`, `OK (skipped=165, expected failures=3)`, parity summary
  `gate-absent failing=0, gate-present failing=0 — PASS`.
- L12: The worktree carries only the three intended paths (test, golden, this doc); the capture harness, both
  captures and all logs live in the scratch dir, and the commit is left to the orchestrator (worker boundary).
