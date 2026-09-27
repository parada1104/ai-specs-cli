# Feature: fix-manifest-lock-writes (Trello card 6a84ed305c0de21af3d2c224)

Root class: business logic inlined as python3 heredocs in `recipe-remove.sh` and
`skills-remove.sh` drifted from the modules that own it.

## Tasks

- [x] 1. RED tests: D1 lock data loss (recipe remove destroys `[managed.*]`),
      D2 whole-file `\n{3,}` collapse rewriting untouched regions, and
      validate+restore / no-partial-write guarantees — black-box via the
      removal CLI entries. Files: `tests/test_recipe_remove.py` (new),
      `tests/test_skills_remove.py` (additions).
- [x] 2. Capture genuine RED output as evidence.
- [x] 3. D1: `lib/recipe-remove.sh` lock heredoc delegates to `lock.py`
      (`load_lock` → `remove_recipe_lock_entries` → `write_lock`), atomic
      `mkstemp`+`os.replace`, `_toml_string` escaping, single `LOCK_HEADER`;
      lock only written when something was actually removed.
- [x] 4. D2+D13: both removal heredocs drop the global `\n{3,}` collapse
      (deletion itself creates no new long blank runs, so zero normalization
      = least mutation); validate the result with `tomllib` and refuse (exit 1,
      no write) when a deletion would break a previously valid manifest
      (FROZEN §3 tolerance for already-invalid manifests preserved); write
      atomically via `mkstemp`+`os.replace` like `lock.py`.
- [x] 5. GREEN on focused suites, then full `./tests/run.sh` (unit-only),
      real exit code captured.

## Evidence

### RED (before fix)

- `tests/test_recipe_remove.py`: 3 tests — 2 failures (D1, D2), 1 failure
  (restore) + no temp-file-leak assertion pass-through. Real output captured
  in session transcript; summary: `FAILED (failures=3)` (see RED section below).
- `tests/test_skills_remove.py` additions: 2 new tests failed (D2 collapse,
  restore) while pre-existing suite stayed green.

Key RED lines:

```
tests/test_recipe_remove: Ran 4 tests — FAILED (failures=3)
  test_remove_preserves_managed_and_agents_in_lock:
    AssertionError: 'AGENTS.md' not found in {} : [managed."AGENTS.md"] entry destroyed by remove
  test_remove_preserves_multiline_string_blank_lines:
    AssertionError: 'first\n\nthird\n' != 'first\n\n\nthird\n'
  test_remove_invalid_result_restores_manifest:
    AssertionError: 0 == 0 : removal that would produce invalid TOML must fail
    (the broken file WAS written)

tests/test_skills_remove: Ran 14 tests — FAILED (failures=2)
  test_remove_preserves_multiline_string_blank_lines (same collapse assertion)
  test_remove_invalid_result_restores_manifest (same exit-0-on-broken-write)
```

The two `test_remove_tolerates_already_invalid_manifest` guards passed pre-fix
by design: they are regression guards for the FROZEN invalid-manifest
tolerance, not defect tests.

### GREEN

- `python3 -m unittest tests.test_recipe_remove tests.test_skills_remove -v`
  → `Ran 18 tests ... OK` (all new + pre-existing black-box tests pass).
- `./tests/run.sh` (unit-only, real exit code via redirect) → **EXIT=0**,
  `Ran 2359 tests in 845.207s / OK (skipped=168)`. The 168 skips are the
  documented no-dist-binary set (no gate binary built in this worktree);
  lock writes in tests exercise the retained Python fallback authority of
  the same `write_lock` every other path uses.

## Notes / decisions

- No new Python modules (epic contract): the two heredocs keep their FROZEN
  segment-splitting (parity contract §3) and share only a small atomic-write
  snippet; the lock path delegates to `lock.py` as the card prescribes.
- The lock is only rewritten when `remove_recipe_lock_entries` reports an
  actual removal (modern locks have no `[recipes.<id>]` section → lock left
  untouched; `[managed.*]`/`[agents.*]`/`[meta]` preserved verbatim).
- Atomic manifest writes also preserve the original file mode (`os.chmod`
  on the temp file before `os.replace`) — lock.py's writer doesn't need this
  (lock is 0600 CLI-owned), a user manifest may be 0644 in a shared repo.
- The global `\n{3,}` collapse was removed entirely rather than seam-scoped:
  per-segment deletion never creates a new long blank run (blank lines live
  in the surviving segment's tail and are pre-existing), so zero
  normalization is the least-mutation reading of D2's "scope any
  normalization to the deletion seam only".
- Legacy `[skills]`/`[deps]`/`[recipes]` sections, if present, are normalized
  away by the canonical `write_lock` writer — identical to every other lock
  write path (sync, refresh-bundled, materialize). Delegation is the card's
  prescribed fix; re-implementing a second lock serializer was rejected.
- Manifest validation is conditional: an already-invalid manifest is still
  removed from (FROZEN tolerance); only a deletion that breaks a previously
  valid manifest is refused with the original bytes intact.

## Commits

(to be recorded by orchestrator — worker leaves changes uncommitted)
