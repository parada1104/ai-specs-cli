# [Go 07.S6] Project cache layer — byte-exact Go port of `project-cache.py`

Card: `[Go 07.S6]` — https://trello.com/c/2xa9qgPL (parent epic `go-single-binary`)
Branch: `change/go-07-s6-cache` · Worktree: `.worktrees/go-07-s6-cache`
Base: `fd0aa68` (S5 merge) · Plan: `odd/tasks/go-07-sync-plan.md` (S6 row)

## Goal

Port `lib/_internal/project-cache.py` (679 lines) to Go as the new package
`internal/projectcache`, byte-exact including the FROZEN cache key and the
`meta.toml` sidecar, proven by a module differential against the real module
(CLI for the `main()` surface, importlib driver for the library-only
functions), plus the `cache-layout` parity fixture.

## Scope decision (S2/S4/S5 precedent)

Callers today are all Python/Bash: `vendor-skills.py:161`,
`refresh-bundled.py:48`, `recipe-materialize.py:72`, `skill-resolution.py:38`,
`doctor.py:300/1418`, `lib/skills-list.sh:143`. **`project-cache.py` stays
alive** until those cards land (plan slicing rule). S6 ships the Go package +
differential only: no wiring, no bridge changes, no deletions.

## Tasks

- [x] T1a — core: `_sanitize_basename`, `cache_key` (FROZEN:
      `sha256(realpath)[:12]-<basename>`), `cache_root`, `_ai_specs_home`,
      `ensure_cache` (+ `meta.toml` create/refresh byte format), path-kind
      roots (`recipe/deps/bundled/inproject/commands/backups/resolved_skills`),
      `gate_backup_path` + unit tests.
- [x] T1b — mutations & CLI: `remove_bundled_skill/command_leftovers`,
      `remove_recipe_command_leftovers`, legacy lock hashes, git-aware
      tracked-leftovers detection + `format_tracked_bundled_remediation`,
      `remove_legacy_origin`, `merge_commands`, CLI entry `main()` semantics
      (exit 2 usage/unknown, printed paths) + full kind-by-kind differential
      against the real module.
- [x] T2 — `cache-layout` parity fixture, zero deltas × both gate modes.
      Implemented in `9252a50`; the independent verifier confirmed all 19
      fixtures, including `cache-layout` and `hooks-five-runtimes`, in both
      gate modes. The post-harness-correction parity run also passed.
- [ ] T3 — Reconcile evidence, independently verify, close real findings, and
      record the work-unit commits and native review outcomes in this doc.
- [ ] F-safe-retry — **user-authorized safety exception (GO-07.S6).** `R3-001`
      (`RemoveLegacyOrigin` retry after a partial migration) stays
      ESCALATED/FAILED, not closed. The old "closed as frozen-oracle-consistent"
      claim is withdrawn: mirroring project-cache.py L497-531 reproduces data
      loss, because a retry treats a partial destination as proof of a complete
      migration and removes `ai-specs/.recipe/` together with the files the
      first run never copied. The user authorized ONE exception to frozen Python
      parity: an existing destination whose completeness is unproven marks the
      migration incomplete, warns, and preserves the legacy originals, and is
      never overwritten. Fresh fully-successful migrations still copy every file
      and remove `.recipe/`. Observed RED/GREEN and the differential exception
      are under "Safety exception — R3-001". Pending the parent's commit and
      review; no native approval is claimed.

## Lessons carried from S4/S5

- Error paths are parity: match which exceptions Python catches vs propagates,
  and every side effect that already happened before the failure.
- Differential compares whole written tree (bytes + modes), stdout, stderr, rc;
  uncaught-exception cases pin rc + exception class (traceback mode).
- Reuse package-local helpers; the new package must not import `internal/sync`
  (use `internal/toml` for TOML reads; sha256 from stdlib).

## Commit file sets (the parent commits)

- **Commit A** (core): `internal/projectcache/projectcache.go` — the
  `--- Commit A ---` section (`SanitizeBasename`, `ResolvePath`, `CacheKey`,
  `AISpecsHome`, `CacheRoot`, `EnsureCache`, `pySplitLines`, all path-kind
  roots); `internal/projectcache/projectcache_test.go` — the
  `Commit A: core unit tests` block.
- **Commit B** (mutations + CLI): `internal/projectcache/projectcache.go` —
  the `--- Commit B ---` section (lock hashes, `remove_*`, git helpers,
  remediation, `RemoveLegacyOrigin`, `MergeCommands`, `copy2/copyTree`,
  `RenderProjectCache`); `internal/projectcache/projectcache_test.go` — the
  `Commit B: full differential` block + `pcCases`;
  `internal/projectcache/testdata/projectcache_ref.py` (new).
- **Commit C** (parity + doc): `tests/parity/parity.py`
  (`_setup_cache_layout` + the `cache-layout` fixture); this doc.

Note: A and B share `projectcache.go`/`projectcache_test.go`; only the
`--- Commit A/B ---` section split separates them, so the parent should stage
by hunk (or land A+B as one commit and C as the second). The allowed edit
surfaces permit only the single `projectcache.go`, so a separate
`projectcache_mutate.go` was not added. The two-state split (objective 1) and
its verification are recorded under "Two-state split" below.

## Resume checkpoint

Base: `fd0aa68`; implementation tip: `20d58ae` (before this bookkeeping update).

- A: `7905e0f` — core and unit tests (491 added lines).
- A2: `d74339f` — corrected realpath derivation.
- B1: `20426dd` — mutations, git remediation and CLI.
- B1 correction: `fcbb61a` — copy-error propagation; its unsafe retry pin was superseded.
- B2: `e530264` — full differential and reference driver.
- B2 correction: `2e81099` — seven non-vacuous seeded-tree cases.
- C: `9252a50` — additive parity fixture and task document.
- D: `ee6671a` — copy/rmtree/suffix/home fixes.
- E: `c9172c4` — realpath parity in `internal/skills`.
- F: `20d58ae` — user-authorized safety exception for legacy migration retries.

Native review checkpoints:

- C1 `review-02dc35a397b4a5fc`: corrected candidate A2 approved and acknowledged-burned.
- C2 `review-8d76bff593c59383`: escalated after failed validation, NOT approved.
  The new F safety fix addresses its unresolved loss-of-overrides finding; F review is pending.
- C3 `review-544ae236bc2f3aa0`: corrected candidate `2e81099` approved and acknowledged-burned.
- Remaining fixes (C/D/E) and F require separate reviews before delivery.

Independent verification: full suite at pre-harness-fix `4043434` passed
(2380 tests, 164 environment skips, rc 0). The later change to seven test
fixtures and this bookkeeping require a refreshed final verification record.
The unrelated untracked `qodana.yaml` is user/IDE-owned and remains untouched.

The historical RED entries below are mutation-sensitivity/cleanup evidence,
not proof that all initial production code was written after a failing test.
Strict initial RED ordering is not established for the original port. The
symlink correction does have observed behavioral RED/GREEN evidence.

Independent verification completed in this pass: its P1–P6 findings are
closed under "Divergences → Closed" with observed RED/GREEN. Native review and
the work-unit commits remain the parent's steps.

## Evidence

### RED / GREEN (strict TDD)
- **RED (observed):** `go test ./internal/projectcache/ -run TestCacheKeyFrozen -count=1`
  fails against a deliberately mutated key derivation (`[:11]` instead of the
  FROZEN `[:12]`):
  `CacheKey("/") = "8a5edab2826-project", want "8a5edab28263-project"` —
  proving the frozen-key test actually rejects a wrong realpath/slice. Reverted
  to `[:12]` immediately after.
- **RED (observed, harness):** the first full differential run failed
  `TestProjectCacheDifferential/remove_bundled_skill_oserror_keeps_dir`
  (`TempDir RemoveAll cleanup: permission denied`) before the read-only-dir
  cleanup hook existed.
- **GREEN:** `go test ./internal/projectcache/ -count=1` → `ok` (unit +
  differential, 47 differential subtests).
- **RED/GREEN (rmtree-on-symlink, objective 2):** the three new cases
  (`cli merge symlink dest rmtree refuses`, `remove bundled skill symlink dir
  kept`, `remove legacy origin symlink dir kept`) failed against the pre-fix
  `os.RemoveAll` (stdout + links differed) and pass after `removeTreePy`.
- **TRIANGULATE:** the differential triangulates every branch: source/lock/
  managed/absent provenance, git vs non-git vs detached, malformed lock,
  dotfile stems, CLI usage/unknown-kind/unknown-action, and uncaught
  `RuntimeError` (traceback mode).

### Verification (exact commands, observed, latest pass)
- `gofmt -l internal/projectcache internal/skills` → prints nothing (empty).
- `go vet ./internal/projectcache/ ./internal/skills/` → clean.
- `go test ./internal/projectcache/ ./internal/skills/ -count=1` → both `ok`.
- `go test ./... -count=1` → all packages `ok` (projectcache `7.081s`).
- `python3 tests/parity/run.py` → `fixtures: 19, failing: 0`;
  `gate-absent failing=0, gate-present failing=0 — PASS`; `cache-layout` and
  `hooks-five-runtimes` zero deltas in both modes.
- `python3 -m unittest tests.test_project_cache` → `Ran 15 tests in 0.033s ... OK`
  (untouched).

Safety-exception pass (F-safe-retry, this pass; exact parent-authorized
commands, run one at a time):
- `gofmt -l internal/projectcache` → prints nothing (empty).
- `go vet ./internal/projectcache/ ./internal/skills/` → clean.
- `go test ./... -count=1` → all packages `ok` (projectcache `9.817s`).
- `python3 -m unittest tests.test_project_cache` → `Ran 15 tests in 0.034s` `OK`.
- `python3 tests/parity/run.py` → `fixtures: 19, failing: 0`;
  `gate-absent failing=0, gate-present failing=0 — PASS`; `cache-layout` and
  `hooks-five-runtimes` zero deltas in both gate modes.
- Focused RED/GREEN runner and the differential exception: RED/GREEN recorded
  under "Safety exception — R3-001"; the differential is now 47 strict cases +
  the single `safetyException` case.

Earlier in this card: `go build ./... && go vet ./internal/projectcache/ &&
 gofmt -l internal/projectcache` clean (incl. the `testdata/.stateA-*.go`
snapshots); State A swap `go test ./internal/projectcache/ -count=1` →
`ok ... 0.586s`. The historical RED/GREEN entries below are
mutation-sensitivity/cleanup evidence; the verifier-confirmed P1–P6 RED/GREEN
is recorded under "Divergences → Closed" below.

### Parity fixture note (T2)
`cache-layout` enables `worktree-flow` + `tdd-flow` (command-shipping catalog
recipes) plus a local `ai-specs/commands/tdd.md` colliding with the
recipe-managed `tdd` command, then runs `sync` and `sync-agent --all`. This
drives `ensure_cache`, `path <kind>`, and `merge-commands` through the whole
CLI (`sync-agent.sh`). **Both legs still run `project-cache.py`** — the Go
package is not wired until its callers move — so the fixture is zero-delta by
construction today and becomes the Go gate at wiring time.

## Quirks reproduced (vs the real module)
1. `cache_key`/`cache_root` use `Path.resolve()` on the project root AND on an
   explicit `cli_home` (symlink resolution, dangling-symlink component walk).
2. `meta.toml` refresh splits with Python `str.splitlines()` (CRLF/`\v`/`\f`/
   `\x1c-\x1e`/`\x85`/`\u2028`/`\u2029` boundaries), rewrites every line whose
   `strip()` starts with `project_root` (so `project_rootish` is rewritten
   too), inserts at the top when absent, joins with `\n` + trailing `\n`.
3. `merge_commands` does `rmtree(dest)` then `copy2` per file: mode bits
   (`& 0o7777`, incl. setuid/setgid/sticky) and mtime preserved; `sorted(...)`
   ascending precedence bundled → managed → local; local wins with a warn.
4. git: `git -C <root> rev-parse --is-inside-work-tree` (stdout `.strip()`
   must equal `true`); `git -C <root> ls-files -- <pathspec>`; a missing git
   binary or a non-zero exit degrades to `false`/`[]`.
5. `created_at` uses Python `%Y-%m-%dT%H:%M:%SZ` UTC → Go
   `2006-01-02T15:04:05Z`.
6. Output channels: successful removals to stdout (`  ✓ ...`), `_warn` to
   stderr (`  ! ...`), usage/unknown to stderr with exit 2, uncaught
   `ensure_cache` failure exit 1 (one `error: ...` Go line).
7. `bundled_command_ids` uses `PurePath.stem` under the epic's Python
   reference (3.11-3.13): `.md` → `.md`, `..md` → `.`, `.hidden.md` →
   `.hidden`; dotfiles ARE matched by `glob("*.md")`; directories named
   `*.md` are excluded (`is_file`). Python 3.14 changed leading-dot handling
   and would report `..md` → `..md`; the epic reference is ≤3.13 (see the
   R3-001 correction below).
8. `bundled_skill_ids` uses the unresolved `Path(cli_home)` for the bundled
   source root (unlike `cache_root`, which resolves it).

## Two-state split (State A / State B)

The port is splittable into two compiling states (parent materializes A first):

- **State A (core, 320 + 171 lines):** the `.stateA-*.go` snapshots in
  `internal/projectcache/testdata/`:
  `.stateA-projectcache.go` (imports minus the B-only `os/exec` and
  `internal/toml`) and `.stateA-projectcache_test.go` (core tests only; the
  A-block `TestPyStem` is excluded because it exercises the B-section `pyStem`).
  Verified by temporary swap: `go build ./...`, `go vet ./internal/projectcache/`
  and `go test ./internal/projectcache/ -count=1` all pass (`ok ... 0.586s`).
- **State B (final):** the full `projectcache.go` / `projectcache_test.go`
  (Commit A + Commit B sections) plus `testdata/projectcache_ref.py`.

The parent moves the `.stateA-*` files into place for commit A, then restores
State B for commit B and deletes the `.stateA-*` files.

## Divergences

### Closed (verifier-confirmed)

An independent verifier ran 22 probes against the real Python module and
confirmed the divergences below. Each was closed with the differential RED
first (observed failing output against the pre-fix Go, captured by reverting
only the production hunk and re-running the named subtest), then GREEN.

**P1 — `merge_commands` copy2 tolerance.** Python `shutil.copy2` is unguarded:
an unreadable source raises, aborting the merge with the destination holding
only the files copied before the failure and the remaining tiers never copied.
Go warned, incremented the count and continued. Fixed: `MergeCommands` returns
the error; `RenderProjectCache merge-commands` prints one `error: …` line,
rc 1.
- RED `cli_merge_unreadable_command_aborts` (0o000 bundled `.md`) →
  `stdout differs; rc: go=0 ref=1; files differ; go stderr is not exactly one
  "error: " line: "  ! open …/b.md: permission denied"`.
- GREEN: bundled `a.md` copied, `b.md` aborts, managed tier untouched.

**P2 — `removeTreePy` deleted regular files.** `shutil.rmtree` refuses a
symlink and a non-directory (`NotADirectoryError`); `os.RemoveAll` removed
regular files silently. Fixed with an `Lstat` guard (symlink OR `!IsDir`).
- RED (a) `cli_merge_regular-file_dest_rmtree_refuses` → `stdout differs;
  rc: go=0 ref=1; files differ` (Go deleted the dest file).
- RED (b) `remove_legacy_origin_regular-file_leftovers` (`.recipe` /
  `.resolved-skills` / `.internal` as regular files) → `files differ; dirs
  differ; go stderr missing prefix "  ! failed to remove leftover
  ai-specs/.recipe/: "` (Go removed all three silently; Python warns ×3 and
  retains).
- GREEN: both pass; the regular files are retained with the exact warn flow.

**P3 — `copyTree` recreated symlinks.** `shutil.copytree(symlinks=False)`
resolves links: symlink-to-file copies the target CONTENT as a regular file,
symlink-to-dir recurses the TARGET as a real dir, and a dangling link is
collected with the walk raising `shutil.Error` only after finishing (the
destination keeps everything copied before the failure, and the final
`copystat` is still applied). Go recreated symlinks with `os.Symlink` and
aborted on the first error. Both branches fixed; the `os.Symlink` recreation is
gone.
- RED `remove_legacy_origin_symlinks_resolved` (file link + dir link) →
  `files differ; modes differ; dirs differ; links differ` (Go wrote symlinks).
- RED `remove_legacy_origin_dangling_symlink_warns` (2 recipes × dangling) →
  `files differ; modes differ; dirs differ; go stderr missing prefix "  !
  failed to migrate overrides for 'recipeA': "`; Python warns ×2 and RETAINS
  `.recipe`.
- GREEN: both pass; `copyTree` accumulates per-entry failures into
  `copyTreeError`, flattens nested failures, and applies `copyStat` last.
  `merge_commands`' own `rmtree`/`copy2` path is unchanged.

**P4 — `ResolvePath` Clean-first broke the FROZEN key.** `filepath.Abs`/`Clean`
collapses `..` BEFORE symlink resolution; Python `Path.resolve(strict=False)`
(`posixpath.realpath`) walks component by component, resolves each component's
links first, then pops `..` against the RESOLVED prefix. A dangling component
leaves the tail unresolved; a symlink loop returns the looping link unresolved
(the `seen` map). Rewritten as that pure pathwalk; the 40-link budget hack is
gone.
- RED `TestResolvePathMatchesPythonRealpath` →
  `ResolvePath("<dir>/abslink/../target.txt") = "<dir>/target.txt", want python
  "<dir>/deep/a/b/target.txt"`; same for `rellink/../target.txt`;
  `ResolvePath("<dir>/dangling/..") = "<dir>", want python "<dir>/missing"`.
- GREEN: all nine candidates (absolute link, relative link, dangling, link
  loop, `.`/`..`/double slash) match `os.path.realpath`. `TestCacheKeyFrozen`
  still pins `8a5edab28263-project`.
- **`internal/skills` duplicate fixed too**: `resolve.go` carried the same
  Clean-first `ResolvePath`/`resolveNonStrict`, whose `..` branch was dead code.
  It now uses the identical `realpathPy`; RED there was the identical test
  failing against the same pre-fix algorithm (`resolve.go` had no `..`-aware
  test before), GREEN after the port. New `internal/skills/resolve_test.go`
  pins it against `os.path.realpath`.

**P5 — command-cleanup suffix filter.** Python `child.suffix != ".md"` (the
≤3.13 rfind rule) keeps `.md` (empty suffix) and treats `..md` as `.md`; Go
`filepath.Ext` returned `".md"` for `.md` too and removed it. Fixed with
`pySuffix` (a `PurePath.suffix` mirror).
- RED `remove_bundled_command_suffix_filter` and
  `remove_recipe_command_suffix_filter` (seeded `.md`, `..md`, `.hidden.md`,
  `a.md.bak`) → `stdout differs; files differ; modes differ` (Go removed
  `.md`/`..md`; Python removed only `.hidden.md`).
- GREEN: `pySuffix` is used by both `RemoveBundledCommandLeftovers` and
  `RemoveRecipeCommandLeftovers`.
- **Superseded semantics (R3-001):** the original P5 pins used Python 3.14's
  leading-dot rule (`..md` empty suffix, `pyStem("..md") = "..md"`). The
  `filepath.Ext` bug fix stands, but the epic reference is ≤3.13, so `..md`
  now has suffix `.md`, `pyStem("..md") = "."`, and the seeded `..md` filters
  are divergence-pinned instead of strict. See the R3-001 correction below.

The earlier `shutil.rmtree`-on-symlink fix stays closed via `removeTreePy`
(the three symlink cases warn/preserve/rc-1 exactly as Python).

### Native-review corrections (second surgery)

- **R3-001 — `RemoveLegacyOrigin` retry-after-partial-migration (BLOCKER):
  ESCALATED/FAILED, old disposition WITHDRAWN.** It is NOT closed as
  "frozen-oracle-consistent". `lib/_internal/project-cache.py` L497-531 has the
  identical flow (`if dest.exists(): continue`, a `migration_failed` flag scoped
  to a single run, `rmtree(.recipe/)` only when nothing failed in that run), so
  a retry after a failed migration skips the partial destination and removes
  `.recipe/` even though the destination may be incomplete. That is a real data
  loss, reproduced intentionally by the earlier port and pinned by the earlier
  `TestRemoveLegacyOriginRetryPreservesThenRemovesDotRecipe` (retain on the first
  run, remove on the retry). Explanation and a loss-preserving test do not
  prevent the loss. The user authorized exactly ONE behavioral divergence from
  the Python oracle to fix it: see "Safety exception — R3-001" below. No claim
  of byte-perfect parity in this case; the rest of the FROZEN contract holds.
- **R4 — merge-count-on-copy-failure (CRITICAL): closed by the C2FIX
  propagation.** `MergeCommands` now propagates the `copy2` error instead of
  warning and counting: the merge aborts mid-way, the CLI exits `rc 1` with one
  `error: …` line, and the partial destination is retained. This is the
  verifier-confirmed P1 semantics (probe `cli_merge_unreadable_command_aborts`),
  moved to `fcbb61a` so the correction precedes the mutations differential.

### Safety exception — R3-001 (user-authorized, GO-07.S6)

**R3-001 is ESCALATED/FAILED, not closed.** The earlier disposition ("closed as
frozen-oracle-consistent") is WITHDRAWN: it preserved the loss by explanation,
not by prevention. This is the single authorized divergence from the frozen
Python oracle, scoped to `RemoveLegacyOrigin` only.

Policy now implemented in `RemoveLegacyOrigin`:

- An existing `ai-specs/recipes/<name>/overrides/` whose completeness cannot be
  proven sets the migration-incomplete guard, emits a warning that names the
  recipe and both paths and asks for verification, and preserves
  `ai-specs/.recipe/`.
- An existing destination is never overwritten and its bytes are never touched.
- A fresh, fully-successful invocation still copies every file and removes
  `.recipe/`, but only after ALL migrations genuinely succeed in that run.

Deliberately NOT implemented (not requested): automatic merging, checksums,
staging areas, durable markers, journals. The warning identifies the recipe and
paths and requests human verification; it does not suggest a blanket
`rm -rf ai-specs/.recipe/`.

**Observed RED/GREEN (strict TDD, root-safe dangling-symlink trigger):**

- RED — `go test ./internal/projectcache/ -run 'TestRemoveLegacyOrigin.*(Retry|Existing|Partial|Fresh)' -count=1`
  against the pre-fix code (RC 1):
  `TestRemoveLegacyOriginRetryPreservesDotRecipe` → `run 2 removed .recipe`;
  `TestRemoveLegacyOriginExistingDestPreservesSourceAndUserContent` → `removed
  .recipe with an unverified existing destination`;
  `TestRemoveLegacyOriginPartialMultiRecipeNoRemovalOnRetry` → `run 1 removed
  .recipe/ although recipeB has an unverified destination`.
- GREEN — same runner after the shared guard: all four cases pass (RC 0).
- Cases: `TestRemoveLegacyOriginRetryPreservesDotRecipe` (runs 2 AND 3 keep
  `.recipe/` and the entry the first run never copied), `...ExistingDestPreserves
  SourceAndUserContent` (legacy source + destination sentinel bytes untouched),
  `...PartialMultiRecipeNoRemovalOnRetry` (one fresh + one existing → no
  removal), `...FreshMigrationRemovesDotRecipe` (normal success/cleanup intact).
  The last is a guard that passes before and after; the three behavioral cases
  produced the RED above.

**Differential:** the 47 strict Python differential cases are unchanged. The
case that exercises an EXISTING destination is now `remove legacy origin` with
`safetyException: true`; it runs both legs and `pcAssertSafetyException` pins the
exact divergence — the real Python oracle removes `.recipe/` while Go preserves
it — and still requires every byte/file/mode/dir/link OUTSIDE
`ai-specs/.recipe/` to match. A new strict case `remove legacy origin fresh
migration` keeps the no-existing-destination surface (`.deps` retention,
`.resolved-skills`/`.internal`/guardian removal, bundled cleanup, two fresh
recipe migrations) at byte parity. Finding: the previous `remove legacy origin`
case used `project_root: "project"`, which both legs nest under the seeded tree,
so it was vacuous; the exception case and its strict sibling use
`project_root: "."` and actually exercise the seeded tree.

No claim of byte-perfect parity in this safety case; the rest of the FROZEN
contract is intact, and no native approval is claimed.

### Corpus-gap regression guards (green before and after)

- `cli_merge_bundled_managed_duplicate`: the same command name in the bundled
  and managed tiers — managed silently overwrites bundled, count stays 1.
- `cli_merge_preserves_source_modes`: 0o755 script, 0o444 read-only and 0o600
  local sources; the seeded differential asserts the copied mode bits, so a
  dropped `chmod` fails it.

### Differential harness note

`projectcache_ref.py` and the Go snapshot mark an unreadable regular file
`"<unreadable>"` instead of aborting, so the permission-denied P1 case is
comparable.

### Correction — R3-double-root (native lineage `review-544ae236bc2f3aa0`)

Native review of the differential candidate (`e530264` vs `fcbb61a`)
returned **R3-double-root (CRITICAL, deterministic)**: the lib dispatch
joins `args.project_root` onto `root`, and several cases passed
`project_root: "project"`, so BOTH legs ran on the non-existent
`<root>/project` while the seed lived at `<root>` — the seeded project-cache
behavior was never exercised and both legs agreed on no-op/empty output. The
finding is the class, not one case (plan: 200 lines). The earlier note that
called these cases "vacuous" is superseded by this record.

Fix (test-only): every lib case whose seed lives at `root` now passes
`project_root: "."` — `remove recipe command leftovers`, `is git work tree
true`, `is git work tree false`, `git ls files`, `tracked skill leftovers`,
`tracked command leftovers detached`, `tracked skill leftovers non-git`.
`testdata/projectcache_ref.py` needs no change: its
`root / args.get("project_root", "project")` join is identical to the Go
dispatch, and every lib case passes `project_root` explicitly, so the
`"project"` default is dead. Left intentionally untouched:
`remove legacy origin symlink dir kept` (its seed lives at `root/project`, so
`"project"` already matches the seed) and `gate backup path` (pure path
constructor, no seeded tree). No seeds were widened and no scenarios added.

The change is test-only, so strict behavioral RED cannot mutate production
inside the allowed surfaces; the before/after vacuity was captured with a
temporary in-test probe (removed after capture) that seeded each case and
checked whether the lib target existed:

| case | before (target missing) | after |
|---|---|---|
| remove recipe command leftovers | stdout `""` | 3 removals: `locked.md`, `managed.md`, `src.md` |
| is git work tree true | `false` | `true` |
| is git work tree false | `false` (vacuous) | real non-git `false` |
| git ls files | `[]` | `["ai-specs/skills/alpha/SKILL.md"]` |
| tracked skill leftovers | `[]` | `[]` (empty-dir short-circuit, see risk) |
| tracked command leftovers detached | `[]` | `["cmd1"]` |
| tracked skill leftovers non-git | `[]` | non-git `[]` |

After the fix the differential is green (`--- PASS:
TestProjectCacheDifferential`, 48 subtests); no subtest changed from PASS to
FAIL, so no hidden port divergence surfaced. The safety-exception case
(`safetyException: true`) is untouched and keeps `project_root: "."` — the
record for it is the same as before this correction.

Residual (not fixed, out of the R3 class): `tracked skill leftovers` removes
only `alpha/SKILL.md`, leaving the empty `alpha/` directory, so
`(root / "ai-specs/skills/alpha").exists()` short-circuits and the case
exercises that branch rather than detection. Actual leftover detection stays
covered by `tracked command leftovers detached`, `git ls files` and the
tracked-bundled-skill unit path. Tightening the seed would change the
scenario, so it is reported rather than absorbed here.

### Correction — R3-001 pySuffix (native lineage `review-39d744ea15efd491`, plan 200 lines)

Native review returned **R3-001 (CRITICAL, deterministic)**: `pySuffix`
stripped leading dots (`strings.TrimLeft`) and so mirrored Python 3.14, while
the epic's Python reference for ambiguous dotted names is **3.11-3.13**, per
explicit user decision. Under ≤3.13, with `i = name.rfind('.')`, the suffix is
`name[i:]` iff `0 < i < len(name)-1`, else `""`. Verified reference values:
`..md` → `.md`, `...md` → `.md`, `.md` → `""`, `.hidden.md` → `.md`,
`x..md` → `.md`, `a.md.bak` → `.bak`; stem = name minus suffix
(`..md` → `.`, `.hidden.md` → `.hidden`). The local `python3` is 3.14.7
(suffix `""` for `..md`/`...md`/`.md`), so the divergence pins hold Go to the
≤3.13 reference, NOT to local `python3`.

Fix: `pySuffix` now implements the rfind rule; `pyStem` derives from it
(stem = name minus suffix when the suffix is non-empty, else name). Downstream,
`RemoveBundledCommandLeftovers`/`RemoveRecipeCommandLeftovers` now REMOVE
`..md`/`...md` project copies whose provenance matches (suffix `== ".md"`), and
`bundled_command_ids` reports stem `.` for `..md` — an intentional divergence
from local Python 3.14.

Divergence pins (established `safetyException` style; both legs run, everything
outside the named gap stays byte-strict):
- `remove bundled command suffix filter` and `remove recipe command suffix
  filter`: Go removes `ai-specs/commands/..md` (stdout shows the removal);
  Python 3.14 keeps it. Files/modes compared after dropping that one path,
  dirs/links/stdout/stderr/dropped-line-for-dropped-line strict.
- `bundled command ids`: tree and streams strict; only `result` diverges —
  Go `[".", ".hidden", "cmd1", "x.tar"]` vs Python 3.14 `["..md", …]`.
- The ref driver now reports `sys.version_info[:2]`; the pins log which
  interpreter they compared against and never branch on it.
- The `remove legacy origin` safety-exception case and every other strict case
  are untouched; the 47-strict-plus-pins structure holds.

Evidence:
- RED (fail pins first, against the pre-fix `TrimLeft` code):
  `go test ./internal/projectcache/ -run
  'TestPySuffix|TestPyStem|TestProjectCacheDifferential' -count=1` →
  `pyStem("..md") = "..md", want "."`; `pySuffix("..md") = "", want ".md"`;
  `Go kept project/ai-specs/commands/..md`; `Go ids = [..md …], want [. …]`.
- GREEN: same runner passes (`ok`), full differential 47 strict + divergence
  pins green.
- Gates: `gofmt -l internal/projectcache` empty; `go build ./...`; `go vet
  ./internal/projectcache/ ./internal/skills/`; `go test ./... -count=1`;
  `python3 -m unittest tests.test_project_cache`; `python3 tests/parity/run.py`
  (19 fixtures, zero deltas both modes) — all pass.

### Accepted (not closed)

1. `_ai_specs_home` fallback: Python's last resort is
   `Path(__file__).resolve().parents[2]` (the module's repo root); Go has no
   source-layout equivalent. `AISpecsHome` returns `""` there and callers must
   pass `cliHome` (the CLI shim always pins `AI_SPECS_HOME`) — unreachable from
   the Go binary, matching the `internal/skills` precedent.
2. Caught-`OSError` message text: Python's `PermissionError` renders
   `[Errno 13] Permission denied: '<path>'`; Go's `os` error renders
   `unlinkat …: permission denied`. The `remove_bundled_skill oserror`
   differential therefore compares rc/tree/modes/dirs/links exactly and pins
   only the shared `  ! failed to remove leftover ai-specs/skills/alpha/: `
   prefix — the errno strerror text is not byte-reproducible.
3. **P6 accepted with a loud failure.** `AISpecsHome("")` cannot reproduce
   Python's module-repo-root fallback, so `RenderProjectCache` now refuses to
   resolve the cache CWD-relative when the resolved home is empty: rc 1 and one
   `error: AI_SPECS_HOME is not set` line
   (`TestRenderProjectCacheMissingHome`). The Go binary's shim always pins
   `AI_SPECS_HOME`, so this stays out of scope and unreachable in production.
4. **Corrected: the CLI surface is NOT identical for `..`.** The earlier claim
   that `main()`/`RenderProjectCache` "`resolve()` the root, collapsing `..`, so
   the CLI surface is identical" was disproved by the verifier: `resolve()` does
   NOT collapse `..` before symlink resolution — that was exactly the P4 defect,
   and it changed the FROZEN cache key for any project root reached through a
   symlink plus `..`. With P4 fixed, both the CLI and the library paths follow
   Python's pathwalk. The library-only `inproject_deps_root` still preserves a
   raw unresolved root (no caller passes one), but that is no longer justified
   by a false "CLI is identical" claim.

## Review focus
- `internal/projectcache/projectcache.go`: the `RemoveLegacyOrigin`
  existing-destination guard and its warning (the single authorized safety
  exception); `EnsureCache` refresh byte format, `pySplitLines`,
  `MergeCommands` precedence/copy2, `RenderProjectCache` exit codes.
- `internal/projectcache/projectcache_test.go`: the `TestRemoveLegacyOrigin*`
  RED/GREEN invariants; the `safetyException` branch (`pcAssertSafetyException`,
  `pcDropLegacy`) and the strict `remove legacy origin fresh migration` case;
  the differential comparison (files/modes/dirs/links + result DeepEqual +
  traceback/prefix modes).
- `tests/parity/parity.py`: the new `cache-layout` fixture.
