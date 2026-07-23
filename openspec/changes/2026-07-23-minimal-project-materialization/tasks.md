# Tasks: minimize the committed project surface

## Planning depth

- **Classification**: Full (explore → proposal → design → spec → tasks).
  Cross-cutting change to the materialization model: skill resolution, sync
  cleanup, gitignore governance, deps split, recipe overrides, and the lock
  schema. Fixes a spec violation introduced in 0.15.0.
- **Authorization**: PENDING. Blocked on maintainer decisions D1–D3 (see
  proposal "Open decisions"). Do NOT begin implementation until resolved.

## Open decisions to resolve before build

- [ ] D1 — toml-deps location: in-project `ai-specs/.deps/` (gitignored) vs cache.
- [ ] D2 — recipes/ override boundary: require `overrides/` vs widen allow-list.
- [ ] D3 — `refresh-bundled` fate: remove vs flatten-only.

## Implementation (red-green-refactor) — pending authorization

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

## Validation

- [ ] `./tests/validate.sh` exit 0; full `pytest tests/` green.
- [ ] Dogfooding self-sync on this repo produces the expected minimal surface
      without deleting locally-authored skills.
- [ ] `openspec` deltas validate against the affected capabilities.
