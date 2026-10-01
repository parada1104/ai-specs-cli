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
- [x] T3 — Evidence + commit ids in this doc.

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

## Two-state split (State A / State B)

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
  differential, 38 differential subtests).
- **RED/GREEN (rmtree-on-symlink, objective 2):** the three new cases
  (`cli merge symlink dest rmtree refuses`, `remove bundled skill symlink dir
  kept`, `remove legacy origin symlink dir kept`) failed against the pre-fix
  `os.RemoveAll` (stdout + links differed) and pass after `removeTreePy`.
- **TRIANGULATE:** the differential triangulates every branch: source/lock/
  managed/absent provenance, git vs non-git vs detached, malformed lock,
  dotfile stems, CLI usage/unknown-kind/unknown-action, and uncaught
  `RuntimeError` (traceback mode).

### Verification (exact commands, observed)
- `go build ./... && go vet ./internal/projectcache/ && gofmt -l internal/projectcache`
  → build + vet clean; `gofmt -l` prints nothing (empty, incl. the
  `testdata/.stateA-*.go` snapshots).
- State A (swap): `go build ./...` + `go vet ./internal/projectcache/` +
  `go test ./internal/projectcache/ -count=1` → `ok ... 0.586s`; State B
  restored afterwards.
- `go test ./internal/projectcache/ -count=1` → `ok ... 6.476s`.
- `go test ./... -count=1` → all packages `ok` (projectcache `12.9s`).
- `python3 tests/parity/run.py` →
  `gate-absent failing=0, gate-present failing=0 — PASS` (18 fixtures);
  `cache-layout` zero deltas in both modes.
- `python3 -m unittest tests.test_project_cache` → `Ran 15 tests ... OK`
  (untouched).

> **Caution (not this task's surface):** the working-tree `tests/parity/parity.py`
> diff REPLACES the S5 `hooks-five-runtimes` fixture with `cache-layout` (the
> `_setup_hooks_five_runtimes` function stays defined but unused). The parity
> run above therefore exercises `cache-layout` and no longer `hooks-five-runtimes`.
> This task's surfaces do not include `parity.py`; the parent should decide
> whether `cache-layout` was meant to be ADDED alongside `hooks-five-runtimes`.

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
7. `bundled_command_ids` uses `PurePath.stem` (Python 3.14): `.md` → `.md`,
   `..md` → `..md`, `.hidden.md` → `.hidden`; dotfiles ARE matched by
   `glob("*.md")`; directories named `*.md` are excluded (`is_file`).
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

### Closed

1. `shutil.rmtree` on a symlink-to-directory (was: `os.RemoveAll` unlinked it).
   Empirically reachable with a crafted input, so closed via `removeTreePy`:
   - `merge-commands` with a symlink dest: Python raises `OSError` uncaught →
     rc 1; `MergeCommands` now returns an error and `RenderProjectCache` emits
     one `error: …` line, rc 1 (`cli merge symlink dest rmtree refuses`).
   - `remove_bundled_skill_leftovers` / `remove_legacy_origin` on a symlink-to
     dir: Python catches the `OSError`, warns, and preserves the link;
     `removeTreePy` refuses the symlink so the port warns and preserves it
     (`remove bundled skill symlink dir kept`, `remove legacy origin symlink
     dir kept`; compared via stderr prefix + the links snapshot).
   RED/GREEN: the three new differential cases failed against the pre-fix
   `os.RemoveAll` (stdout + links differed) and pass after `removeTreePy`.

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
3. Path `..` preservation: `main()` and `RenderProjectCache` both `resolve()`
   the root, collapsing `..`, so the CLI surface is identical; the library-only
   `inproject_deps_root`/`merge_commands` local tier preserve a raw unresolved
   root, but no caller and no differential case ever passes one.

## Review focus
- `internal/projectcache/projectcache.go`: `EnsureCache` refresh byte format,
  `pySplitLines`, `MergeCommands` precedence/copy2, `RenderProjectCache` exit
  codes.
- `internal/projectcache/projectcache_test.go`: the differential comparison
  (files/modes/dirs/links + result DeepEqual + traceback/prefix modes).
- `tests/parity/parity.py`: the new `cache-layout` fixture.
