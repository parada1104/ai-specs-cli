# ODD Feature: fix-deps-layer

**Card**: 6a84ed60288e7e6cf23166bb — [Bug] The vendored deps layer is unreproducible and points at the wrong paths
**Branch**: change/fix-deps-layer (worktree .worktrees/fix-deps-layer)
**Parity contract**: read first; D7/D8/D16/D17/D18 are this card's defects. Lock header/legacy-drop semantics are deliberately re-extended for deps only (recipes gap reported separately).

## Root class

Deps moved to `ai-specs/.deps/<id>/skills/<id>/` but consumers were never
updated, and acquisition has no reproducibility (no ref pinning, no recorded
hashes).

## Defects in scope

- **D16**: revision pinning for [[deps]] (`ref` key, `rev` alias), honored by
  vendor-skills.py clone (`--branch` for tags/branches, fetch+checkout for raw
  SHAs). skills add gets `--ref`.
- **D17**: dep content hashes recorded in the lock via the imported-but-never-
  called lock.py helpers. Requires re-extending the lock schema: both writers
  (Go `lockwrite.go` + Python fallback) re-emit `[deps."<id>".skills."<skill>"]`
  (the format collapsed to provenance-stamp in c4c6d18). Header comment updated
  in both. `skills remove` prunes the dep's lock section (new helper
  `remove_dep_lock_entries`).
- **D7**: skills list checks `ai-specs/.deps/<id>/skills/<id>/SKILL.md` for the
  installed status (was the stale `ai-specs/skills/<id>` path).
- **D8**: skills list bundled section scans `{cache}/.bundled/skills/` (cache
  derivation via project-cache.py sibling import — single authority).
- **D18**: skills remove prunes `ai-specs/.deps/<id>/`; --help text fixed.

## Constraint notes

- lock.py "legacy drop" pinned by test_lock_bridge.py: deps is re-extended,
  skills/recipes stay dropped; comments updated, deps tests added. Reported.
- recipes hash sections still dropped by both writers (recipe-materialize's
  set_recipe_skill_hashes persists nothing) — same-class finding, out of lane,
  reported.
- Gate binary trust root: local builds don't match SHA256SUMS → tests exercise
  the Python fallback writer; Go change verified by `go test` in gate dir.

## Tasks

1. RED: black-box D7 test (deps installed status at real layout) + fix the
   stale-layout covering test in test_skills_list.py.
2. RED: black-box D8 test (bundled section scans cache/.bundled/skills).
3. RED: black-box D16 tests (ref/rev/SHA pinning via bin/ai-specs sync) and
   D17 test (dep hashes recorded in lock after sync) — tests/test_deps_pinning.py.
4. RED: black-box D18 tests (remove prunes .deps/<id> + lock section + help
   text) in test_skills_remove.py.
5. GREEN: implement — vendor-skills.py, skills-list.sh, skills-remove.sh,
   skills-add.sh, lock.py, lockwrite.go (+ header + envelope + fallback).
6. Go gate tests for deps emission; update test_lock_bridge.py comments/tests.
7. Full `./tests/run.sh` green; report with RED evidence.

### GREEN (post-implementation)

- Focused suite `python3 -m unittest tests.test_skills_list tests.test_skills_remove tests.test_deps_pinning tests.test_lock_bridge -v` → `Ran 36 tests ... FAILED (failures=2, skipped=1)`. D7/D8/D16 list+vendor, D18 (all three), D16 rev alias, bridge deps emission: GREEN.
- The 2 remaining failures (`test_dep_hashes_recorded_in_lock`, `test_remove_prunes_dep_lock_section`) are environmental: the pre-existing verified gate binary at `cache/bin/worktree-gate/0.24.0/darwin-arm64/` (built pre-deps, blessed by a historical `.verified` receipt; no committed SHA256SUMS re-check catches it because the committed sums still match the OLD bytes) intercepts `write_lock` and drops the deps section. Pinned by `WORKTREE_GATE_BIN=/nonexistent python3 -m unittest tests.test_deps_pinning tests.test_skills_remove` → `Ran 19 tests ... OK`.
- Go gate: `go test ./...` in catalog/recipes/worktree-flow/gate → ok (incl. new deps emission + deps control-char refusal + updated header identity).

### Out-of-surface findings (need parent decision)

1. RESOLVED (parent-authorized): `catalog/recipes/worktree-flow/bin/SHA256SUMS` regenerated via `scripts/build-gate.sh` (canonical go1.24.13, no warning) + shasum, with a convention-consistent regeneration comment; digest lines replaced only. The stale cache binary `cache/bin/worktree-gate/0.24.0/` is now rejected by the trust-root re-check (`resolve_verified_binary` → None) → Python fallback writer, as designed.
2. RESOLVED (parent-authorized): `tests/test_hub.py::test_bundled_skills_not_under_local` updated to the D8 cache-scanned bundled section (black-box inline cache_key; descriptions asserted; local exclusion asserted; name kept).
3. RESOLVED (parent-authorized, option 2): `tests/test_lock.py` covering tests renamed per the inverted D17 contract — `test_skill_recipe_dep_hashes_not_emitted` → `test_dep_hashes_emitted_skill_recipe_dropped`, `test_legacy_hash_sections_dropped_on_rewrite` → `test_legacy_rewrite_drops_skills_recipes_keeps_deps`. Both now assert deps is emitted and round-trips via load_lock while skills/recipes remain dropped legacy groups; docstrings cite D17 + the c4c6d18 collapse. No other tests touched.

### Final verification (post-authorization)

- `python3 -m unittest tests.test_skills_list tests.test_skills_remove tests.test_deps_pinning tests.test_lock_bridge tests.test_hub -v`: EXIT:0 — `Ran 73 tests ... OK`.
- `go -C catalog/recipes/worktree-flow/gate test ./...`: ok.
- `./tests/run.sh`: EXIT:1 — `Ran 2471 tests ... FAILED (failures=2, skipped=2)`; the only 2 failures are the test_lock.py deps-contract pins above (all 11 earlier failures fixed).
- `git status --short`: dist/ and cache/ artifacts absent (gitignored, confirmed via `git check-ignore`).

### FINAL (all authorizations applied)

- `python3 -m unittest tests.test_lock tests.test_lock_bridge tests.test_skills_list tests.test_skills_remove tests.test_deps_pinning tests.test_hub -v`: EXIT:0 — `Ran 93 tests in 25.720s / OK`.
- `go -C catalog/recipes/worktree-flow/gate test ./...`: ok.
- `./tests/run.sh > /tmp/fix-deps-layer-runsh.log 2>&1; echo EXIT:$?`: **EXIT:0** — `Ran 2471 tests in 834.512s` / `OK (skipped=2)`; zero FAIL/ERROR lines; dist//cache/ artifacts absent from git status.

## Evidence

- RED captured per task below (fill during work).

### RED (pre-implementation), suite: `python3 -m unittest tests.test_skills_list tests.test_skills_remove tests.test_deps_pinning -v` → `Ran 32 tests ... FAILED (failures=11)`

- D7 — `test_dep_status_installed_reports_real_deps_layout` FAIL: status checks stale `ai-specs/skills/<id>`; correctly synced dep at `.deps/<id>/skills/<id>/` reported `✗ not synced` (AssertionError on "✓ installed" absent).
- D7 — `test_dep_status_not_synced_ignores_stale_skills_dir` FAIL: legacy copy under `ai-specs/skills/<id>` satisfies the check ("✓ installed" unexpectedly found).
- D16 (list) — `test_dep_listing_shows_pinned_ref` FAIL: `AssertionError: 'v1.2.3' not found in '=== ai-specs skills list ===\nProject: ...\n  vendored-skill\n    source:     https://github.com/test/repo.git\n    status:     ✗ not synced\n...'`
- D8 — `test_bundled_section_scans_cache_bundled_skills` FAIL: `AssertionError: 'skill-creator' not found in ' (CLI-shipped) ──\n  (none)\n\n── '`
- D16 (vendor) — `test_ref_tag_pins_vendored_content` / `test_rev_alias_pins_vendored_content` / `test_ref_sha_pins_vendored_content` FAIL: `AssertionError: 'V1 body.' not found in '---\nname: vendored-demo\n...\n\n# Vendored Demo\n\nV2 body.\n'` (pin ignored, HEAD vendored).
- D17 — `test_dep_hashes_recorded_in_lock` FAIL: `AssertionError: 'vendored-demo' not found in {}` (lock has no deps group).
- D18 — `test_remove_prunes_inproject_deps_dir` FAIL: `AssertionError: True is not false` (`.deps/my-skill` survives removal).
- D18 — `test_remove_prunes_dep_lock_section` FAIL: `AssertionError: 'my-skill' unexpectedly found in {'my-skill': {'skills': {'my-skill': {'SKILL.md': 'hash-my'}}}, 'other-skill': {'skills': {'other-skill': {'SKILL.md': 'hash-other'}}}}`.
- D18 — `test_remove_help_names_deps_path` FAIL: `AssertionError: 'ai-specs/.deps' not found in ...` (help names the stale `ai-specs/skills/<id>/` path).
