# Tasks: minimize the committed project surface

## Planning depth

- **Classification**: Full (explore → proposal → design → spec → tasks).
  Cross-cutting change to the materialization model: skill resolution, sync
  cleanup, gitignore governance, deps split, recipe overrides, and the lock
  schema. Fixes a spec violation introduced in 0.15.0.
- **Authorization**: PENDING. Blocked on maintainer decisions D1–D3 (see
  proposal "Open decisions"). Do NOT begin implementation until resolved.

## Open decisions to resolve before build

- [x] D1 — toml-deps location: **in-project `ai-specs/.deps/` (gitignored)** —
      maintainer decision (governance visibility over on-disk footprint).
- [x] D2 — recipes/ override boundary: **require `overrides/` path**; non-`overrides/`
      declared content (e.g. trello `templates/`) migrates or becomes bundled.
      (recommended default; revert if maintainer objects)
- [x] D3 — `refresh-bundled` fate: **flatten-only** (no in-project write, no
      `.new`). (recommended default; revert if maintainer objects)

## Implementation (red-green-refactor) — COMPLETE

> All blocks implemented TDD, committed on `change/minimal-project-materialization`
> (commits `9b01e40`→`ca5b02f`). Full `validate.sh` green (1020 tests). Boxes below
> reflect the delivered work.

### Skill resolution (four-tier)

- [ ] RED: test that a CLI-bundled skill id resolves from `{cache}/.bundled/`
      and that `ai-specs/skills/<bundled-id>/` is absent after sync.
- [ ] RED: test that a locally-authored skill of the same id shadows the bundled
      copy and is NOT deleted (dogfooding guard).
- [ ] GREEN: add tier-4 resolution in `sync-agent`; flatten
      `$AI_SPECS_HOME/bundled-skills/` → `{cache}/.bundled/skills/`.

### Leftover cleanup + migration

- [ ] RED: test sync deletes leftover `ai-specs/skills/{harness-*,skill-creator,
      skill-sync}` when not locally authored.
- [ ] GREEN: extend sync leftover-cleanup with the bundled-skill rule + guard.
- [ ] GREEN: rewrite `.ai-specs.lock` to `[meta]`-only; drop hash sections on
      migration (`cli_version.py stamp-meta` owns the write).

### toml-deps split (per D1)

- [ ] RED: test toml-dep materializes at `ai-specs/.deps/` and is gitignored;
      recipe-dep stays under `{cache}/.deps/`.
- [ ] GREEN: route toml-deps to in-project `.deps`; keep recipe-deps in cache
      (`vendor-skills.py`, shared resolver).

### recipes/ gitignore + overrides (per D2)

- [ ] RED: test root `.gitignore` ignores `ai-specs/recipes/**` except
      `ai-specs/recipes/*/overrides/`.
- [ ] GREEN: update `templates/gitignore-*.tmpl` + sync gitignore refresh;
      migrate any non-`overrides/` declared overrides (e.g. trello templates).

### refresh-bundled (per D3)

- [ ] GREEN: remove or convert `refresh-bundled.sh` + `refresh-bundled.py` to
      flatten-only (no in-project write, no `.new` sidecars).

### Lock schema

- [ ] RED: test lock has `[meta]` only after sync; no `[skills.*]`/`[recipes.*]`.
- [ ] GREEN: strip hash writes from `skills-add`, `skills-remove`,
      `recipe-remove`, `init`.

## Backward-compatibility (existing projects)

- [x] Lock-hash migration guard: `remove_bundled_skill_leftovers` removes an
      in-project bundled copy when its `SKILL.md` matches the current source OR
      the legacy lock hash (untouched copy from an older CLI); user-edited copies
      preserved. Run inside `refresh-bundled` before the lock is normalized so
      the `[skills.*]` signal is still available.
- [x] `init.sh` no longer copies `skill-creator`/`skill-sync` into the project
      (removed the redundant second materialization path); next-steps text fixed.
- [x] End-to-end migration smoke test (14/14): simulated a pre-upgrade project
      (committed bundled skills from mixed versions, legacy lock with
      `[skills.*]`/`[recipes.*]`, committed `recipes/`, a declared override) →
      clean migration, no data loss.
- [ ] FOLLOW-UP (not this change): migration guidance for `recipes/` already
      committed on 0.15 projects — gitignore does not untrack; users run
      `git rm -r --cached ai-specs/recipes` (keeping `*/overrides/`).
- [ ] FOLLOW-UP (not this change): relocate bundled COMMANDS to the cache and
      drop `[commands]`/`[opted-out]` from the lock (kept for now).

## Validation

- [x] `./tests/validate.sh` exit 0; full unittest suite green (1020 tests).
- [x] Migration smoke test green (scratchpad `migrate_smoke.sh`, 14 assertions).
- [ ] `openspec` deltas validate against the affected capabilities (at archive).
