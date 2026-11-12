# GO-20 U2e T5 PR4a S4 — vendorskills live oracle

Base: epic/go-single-binary f139a56. Branch: change/go-20-u2e-t5-pr4a-s4-vendorskills.
Goal: `internal/sync/vendorskills_test.go` stops exec'ing python3 and reading
`lib/_internal/vendor-skills.py` without silent coverage loss. Same discipline as PR #496 (doctor),
PR #497 (rulesaudit), PR #498 (skills), PR #499 (conflicts), PR #501 (briefpolicy), PR #502 (checksync),
PR #503 (envscaffold), PR #504 (refreshbundled).

## Specs

- S1: `TestSyncVendoredSkillsDifferential` (same name, no longer a live python differential) reads a frozen
  golden (`internal/sync/testdata/vendorskills_golden.json`) captured once from
  `python3 lib/_internal/vendor-skills.py <project>` over the same four fixtures the live differential built.
  No python3, no `lib/` access, no skip: a missing golden fails.
- S2: `TestRecipeDepSkillDifferential` (same name) reads the same golden's second inventory, captured once from
  `python3 -c <sync_dep_target shim> vendor-skills.py <payload> <project> <home>` — the `in_project=False` entry
  the recipe-dep delegation uses. No python3, no `lib/` access, no skip.
- S3: Every native assertion of both live legs survives case by case — the exit code, the stdout lines
  (`  ▸ <id>  ←  <source>`, the `(path: …)` / `(ref: …)` suffixes, `  ✓ vendored N dep(s)`,
  `  (no [[deps]] declared — nothing to vendor)`), stderr byte for byte where the port owns the text, the
  produced project tree (root `[[deps]]` route) and the produced cache `.deps` tree (recipe-dep route).
- S4: The test asserts hard-coded independent inventories of the case names per leg (count, uniqueness, order)
  against both the in-test corpus and the golden before comparing anything. A golden that drops, duplicates or
  reorders a case fails as an inventory error.
- S5: Frozen inputs equal the in-test corpus: each case pins its pre-run project tree (paths plus bytes), and the
  test snapshots the tree its own `setup` just wrote and compares it before running the port, so fixture drift
  fails as input drift, not as an opaque payload mismatch.
- S6: No absolute host path leaks into the golden: the temporary project root and cli home are tokenized as
  `<DIR>` (symlink-resolved form included, the macOS `/var` → `/private/var` case) and the missing dep source as
  `<MISSING>`; every frozen path is root-relative and sorted (enforced by the loader);
  `grep -c "/Users/\|/private/\|/var/folders"` over the golden is 0.
- S7: Negative controls: mutating one frozen result, deleting a case, duplicating a case, and drifting a frozen
  input each fail rc=1.
- S8: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, the two frozen tests free of live
  `python3` / `lib/_internal` (only the provenance comment matches; the PATH-shadow native guard keeps its own
  `python3` references by design), `go test ./internal/sync/ -run 'TestSyncVendoredSkills|TestRecipeDepSkill'
  -count=1` rc=0 (also under `umask 077`) and `go test ./internal/sync/ -count=1` rc=0.
- S9: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0, `./tests/validate.sh` rc=0.

## Per-leg decision

| legacy leg | decision | reason |
|---|---|---|
| `TestSyncVendoredSkillsDifferential` (`python3 lib/_internal/vendor-skills.py <project>` versus the native `SyncVendoredSkills`, compared on rc, stdout, stderr and the project tree) | FREEZE | the payload is host-independent once two normalizations are applied (below): stdout carries only the dep id, the stable source string and the `(path: …)`/`(ref: …)` suffixes; the project tree is the fixture manifest plus the vendored `.deps` tree and the `.ai-specs.lock` body, whose hash values are derived from frozen bytes. RETIRE was rejected: no other frozen test covers this surface (`internal/sync/vendorskills.go` has no other oracle, and `materialize_test.go` / `syncagent_materialize_test.go` cover the recipe-dep *call site*, not the vendoring). RETARGET was rejected for the same reason: there is no frozen Go-side coverage to cite. The live leg ran exactly these four fixtures, so the freeze is 1:1. |
| `TestRecipeDepSkillDifferential` (`python3 -c <shim> … sync_dep_target(in_project=False)` versus the native `materializeDepSkill`, compared on rc, stdout, stderr and the cache `.deps` tree) | FREEZE | the payload is host-independent: stdout is the same `  ▸ …` shape, stderr is empty on both sides, and the cache tree is compared relative to the per-project cache root, so the cache key (derived from the project's absolute temp path) never reaches a frozen path. RETIRE was rejected for the same reason as above; the `in_project=False` branch (cache staging + `ensure_cache`) has no other frozen oracle. 1:1 with the live leg's single fixture. |

Normalizations, all documented in the test and in the golden's `oracle.note`:

- **Tokenization.** `<DIR>` replaces the temporary project root and cli home in stdout, stderr and frozen file
  bytes (both as created and as symlink-resolved); `<MISSING>` replaces the non-existent dep source of the
  clone-failure case.
- **Git fixture determinism.** The live leg used a temp git repository *as the manifest source*, so the rendered
  SKILL.md frontmatter and the lock hashes embedded a host path and differed on every run (first capture pass
  proved it: the `dep with ref` lock hash changed between captures). The frozen fixture keeps the stable
  `https://github.com/kepano/obsidian-skills.git` source and points `AI_SPECS_VENDOR_FIXTURE_ROOT` at a temp git
  repository, so the clone path still runs (`cloneVendorSource`'s git branch, `--branch v1`), while the rendered
  bytes and lock hashes are host-independent. No commit hash or timestamp is frozen: git's own stderr is never
  byte-pinned.
- **Modes.** Directory modes are **not** frozen: vendored targets are created with `MkdirAll(0o777)` and
  `CopyTree`'s `Mkdir(0o777)`, so they follow the process umask, and this package also runs `t.Parallel()` tests
  that must not have the umask pinned under them. Directory *paths* are still frozen. Regular-file modes are
  frozen except for SKILL.md, which the port renders with a `0o666` create: its final mode is `0o666 &^ umask`,
  so the golden freezes the token `umask` and the test resolves it from a probe file (`0o666` create in a temp
  dir) instead of pinning the process umask. Every other file is a `copy2` copy, which chmods the source's
  explicit mode, so it is umask-independent. The `umask 077` run in L10 proves it.
- **stderr of git children.** A `null` stderr in the golden means the case's stderr text is git's own —
  version-dependent `--depth is ignored in local clones` / detached-HEAD advisory chatter, or a clone failure's
  `fatal:` line — so only a non-empty stderr is required, exactly the live leg's requirement for the
  clone-failure case. Freezing either would pin git's wording (and, before the fixture fix, a random commit SHA),
  not the port's behaviour. This is the one narrowing against the old differential, which byte-compared the
  `dep with ref` stderr between two legs that shared one repository in one run.

Shapes the live legs never exercised stay untouched, and the helpers they share with other files are reused
read-only: `writeRefreshFile`, `refreshCacheRoot`, `stripBridgeFallback` and `withoutEnv` from
`refreshbundled_test.go`, `readGoldenBundle` and `validateGoldenIdentity` from `agentsrender_test.go`,
`gitignoreRepoRoot` from `gitignore_test.go`, `shellSingleQuote` from `spine_test.go`. The PATH-shadowed
`TestSyncVendoredSkillsStepIsNative` guard is preserved byte for byte: it is a native assertion (the Go path
never calls python3), not a live oracle.

Cases: sync leg (4) `no deps`, `dep with ref`, `dep with path and ancillary dirs`, `missing source`; recipe leg
(1) `recipe dep skill` — 5 frozen records in one bundle, 2 separate inventories.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1, S2, S3, S4, S5, S6, S7 | inline | |
| T2 | S8, S9 | inline | |

## Log

- L1 (Robert): "ok" — approving S4 in that order: briefpolicy, checksync, envscaffold, refreshbundled,
  vendorskills, then spine+syncagent, then the materialize-helper consumers; one worker at a time, merge after
  validate.
- L2: Baseline at f139a56: the live python differential is green — all four sync subtests, the recipe leg and the
  PATH-shadow native guard pass (`logs/01-baseline-live-legs.log`, rc=0).
- L3: Scratch/TMPDIR: /Users/robert/.hermes/profiles/cody/cache/scratch/go20-pr4a-s4-vendorskills (Robert's
  standing scratch convention); every go test/build/validate ran with `TMPDIR` pointing at its `tmp/`.
- L4: RED before the golden existed: `read frozen oracle: open testdata/vendorskills_golden.json: no such file
  or directory`, rc=1 on both legs (`logs/02-red-missing-golden.log`).
- L5: Capture harness was temporary code in the test file, gated by `VENDORSKILLS_CAPTURE=<path>` and removed
  before the commit, so the golden's normalization is the same code the frozen tests compare with. It rebuilt each
  case's fixture from the same corpus, ran the real legacy oracle with `PYTHONDONTWRITEBYTECODE=1` and without
  `WORKTREE_GATE_BIN` (the live leg's invocation), froze the pre-run project tree alongside the legacy output,
  stripped the lock writer's `GO_LOCK_WRITE_BRIDGE_FALLBACK` line from the frozen stderr, and failed the capture
  if an absolute host path survived tokenization. So the golden is legacy output, not port output.
- L6: First capture pass was **not** reproducible (`cmp` differed): the `dep with ref` case used its temp git
  repository as the manifest source, so the rendered SKILL.md frontmatter and the lock's SKILL.md hash embedded a
  host path, and git's detached-HEAD note embedded the random commit SHA. Fixed by cloning the stable fixture URL
  from a temp git fixture root (fixture determinism, no commit hash frozen) and by not byte-pinning git's stderr.
- L7: Captured twice into fresh scratch files and `cmp` byte-identical (`logs/05-capture-cmp.log`, rc=0 —
  `logs/03-capture1.log`, `logs/03-capture2.log`): 8905 B, 4 sync + 1 recipe case. Oracle metadata
  `python: 3.14.7`, `script: lib/_internal/vendor-skills.py`. No wall-clock value and no git value is frozen.
- L8: Negative controls (each rc=1; golden restored and `cmp`-verified byte-identical after each): mutate one
  frozen stdout → only the stdout comparison fails (`logs/07-nc1.log`), delete a case →
  `vendorskills sync: 3 records, want 4` (`logs/07-nc2.log`), duplicate a case → `5 records, want 4`
  (`logs/07-nc3.log`), drift the frozen input manifest of the first case →
  `project input: ai-specs/ai-specs.toml bytes` (`logs/07-nc4.log`).
- L9: `gofmt -l internal/sync` empty, `go vet ./internal/sync/` rc=0, and
  `grep -n "python3\|lib/_internal\|exec.Command\|exec.LookPath\|os/exec"` over the test matches the provenance
  comment, the PATH-shadow native guard (kept), and the git fixture's own `exec.Command("git", …)` only
  (`logs/13-grep-live-legs.log`). Host-path grep over the golden: 0.
- L10: `-run 'TestSyncVendoredSkills|TestRecipeDepSkill'` rc=0 (`logs/06-green-focused.log`) — the first GREEN
  run exposed that the snapshot side had recorded the `umask` token instead of the real mode, so `vendorTree`
  now records the observed bits and only the golden carries the token; the same under `umask 077` rc=0
  (`logs/14-green-umask077.log`), `go test ./internal/sync/ -count=1` rc=0 (`logs/16-green-package-tests.log`).
- L11: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/17-build.log`), `./tests/validate.sh` rc=0
  (`logs/18-validate.log`).
- L12: The worktree carries only the three intended paths (test, golden, this doc); the capture harness, both
  captures and all logs live in the scratch dir, and the commit is left to the orchestrator (worker boundary).
