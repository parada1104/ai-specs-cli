# Apply progress: relocate-bundled-commands

**Mode**: Strict TDD
**Delivery**: single PR on `change/relocate-bundled-commands` (phase commits)
**Status**: 30/30 implementation tasks complete (2 Validation items intentionally
out of scope for this apply pass — see Deviations)

## Commits

1. Phase 1 — bundled-command primitives in project-cache.py (`6c70681`)
2. Phase 2 — 3-tier command precedence + delete dead command-merge.py (`557f9d9`)
3. Phase 3 — refresh-bundled.py flatten-only for commands (`965e2cd`)
4. Phase 4 — drop `[commands]`/`[opted-out]` from lock schema (`27939ee`)
5. Phase 5 — per-bundled-command-id doctor diagnostics (`3f1dfbf`)
6. Phase 6 — remove init.sh bundled-commands copy + fix help text (`9e49421`)
7. Phase 7 — end-to-end pre-upgrade migration smoke test (`070c9f7`)
8. Fix — stale `SkillSyncScriptTests` fixture path blocking `./tests/validate.sh` (`e05d12f`)

## TDD Cycle Evidence

| Phase | Tests | RED | GREEN | Notes |
|-------|-------|-----|-------|-------|
| 1 | `test_project_cache.py::BundledCommandPathTests` (3), `test_external_dirs.py::BundledCommandLeftoverCleanupTests` (4), `::TrackedBundledCommandLeftoverTests` (3) | ✅ 10 tests failed with `AttributeError: module ... has no attribute` | ✅ 58 tests in the two files pass | Shared `_tracked_bundled_leftovers(project_root, bundled_ids, path_template)` helper factored out rather than duplicating skill git-plumbing; `format_tracked_bundled_remediation` generalized with `kind`/`path_template`/`recursive` kwargs, defaults preserve exact skill-side output |
| 2 | `test_command_merge.py` +3 (`test_bundled_only_appears_in_merge_output`, `test_managed_silently_overrides_bundled`, `test_local_wins_over_bundled_and_managed_with_warning`) | ✅ 2 of 5 failed (bundled tier ignored) | ✅ 5/5 pass; repo-wide grep for `command-merge` confirmed zero callers before `git rm` | `merge_commands` copies bundled → recipe-managed → local (ascending precedence); local warns on collision with either lower tier, recipe-vs-bundled is silent |
| 3 | `test_harness_cli_literacy.py` +3 (init-flatten, byte-identical-removal, customized-keeps) | ✅ all 3 failed against old materializing `refresh()` | ✅ 11/11 in file pass | Removed `iter_bundled`/`project_path_for`/`display_name`/`lock_get`/`lock_set`/`lock_del`/`save_new_sidecar`/`sha256_of` and the per-command hash loop. Also fixed a real gap surfaced by this phase: `doctor.py`'s `_check_agent_outputs` "expected commands" set didn't union the bundled cache tier, so Phase 2's now-3-tier merge produced false "stale command" WARNs — added `_bundled_command_names()` and included it |
| 4 | `test_lock.py` — replaced `test_meta_commands_opted_out_preserved` with `test_legacy_commands_opted_out_dropped_on_write` + `test_legacy_lock_with_commands_opted_out_sections_normalized` | ✅ both failed (`[commands]`/`[opted-out]` still serialized) | ✅ 4/4 in file pass | `refresh-bundled.py`'s `refresh()` updated: since `load_lock` no longer exposes a `commands` key, `remove_bundled_command_leftovers` is called without an explicit `lock_commands` arg so it self-reads the raw on-disk lock (still present before `write_lock` normalizes it) for the legacy-hash migration signal |
| 5 | `test_doctor.py` — replaced `test_bundled_commands_missing_reports_warn`; added `test_bundled_command_present_reports_ok_by_name`, `test_bundled_command_missing_reports_error`, `test_empty_ai_specs_commands_dir_is_healthy`, `test_tracked_bundled_command_leftover_warns_without_git_rm` | ✅ 4 failures (old aggregate check, missing tracked-leftover-for-commands check) | ✅ 73/73 in file pass | Found and fixed a duplicate `_check_tracked_bundled_leftovers` method left over from an earlier edit (the old skills-only body was shadowing the new one — commands silently never fired despite the new code looking correct). Kept the per-id check `name` as `"bundled-commands"` (plural) rather than mirroring the skill side's singular `"bundled-skill"` — a pre-existing test (`CacheAwareCommandsDoctorTests`) substring-filters on `"bundled-commands"` in the rendered check line; also seeded that test's (and `RecipeCliDepsDoctorTests`') fake-cli-home fixtures with a `bundled-commands` cache tier, mirroring their existing `bundled-skills` seeding |
| 6 | `test_rules_audit.py::test_bundled_commands_distribution_after_refresh` rewritten | ✅ old assertions (`ai-specs/commands/rules-audit.md` must exist) failed against Phase 3's cleanup | ✅ passes; full suite green | Test actually turned green as a side effect of Phase 3 (leftover cleanup deletes step 2b's byte-identical copy during init's own `refresh-bundled --init` call) — Phase 6 removes the now-redundant copy step itself and corrects `init.sh`/`refresh-bundled.sh`/`bin/ai-specs` doc text. Fixed one incidental regression: `test_upgrade.py::test_help_lists_upgrade` asserted the literal word `"update"` appears in `ai-specs help` output, which only ever matched by coincidence via the old (now-removed) "Update bundled skills/commands" wording — reworded to check `"installation"` instead |
| 7 | `test_external_dirs.py::CommandRelocationMigrationSmokeTest::test_pre_upgrade_project_migrates_cleanly_on_sync` | N/A (this phase composes prior phases; test passed on first run) | ✅ passes | End-to-end: pre-upgrade project (committed byte-identical + customized bundled-command copies, legacy `[commands]`/`[opted-out]` lock, one genuine local command) → single `ai-specs sync` → byte-identical copy removed, customized copy kept with printed warning, local command untouched, lock trimmed to `[meta]` only, fan-out (`.cursor/commands/`, `.opencode/commands/`) still serves the bundled command from cache with the customized local copy winning |

### Full suite
`./tests/validate.sh` — **OK** (1044 tests, ~267s).

## Deviations

- **design.md's "no lock read, no lock write" claim (line 46-48) contradicted its own later paragraph** (line 70: "Called from `refresh()` **before** `write_lock` drops the `[commands]` section") and the pre-existing skill-side pattern already shipped by the parent change. Followed tasks.md 3.4's literal instruction and the working skill precedent: `refresh()` still calls `load_lock`/`write_lock` (needed to preserve `[meta]`/`[agents.*]` and to let `remove_bundled_skill_leftovers` consume `[skills.*]` while still in memory); only the per-file bookkeeping loop (`lock_get`/`lock_set`/`lock_del`, `touched` diffing) was removed. Since `load_lock` no longer exposes a `commands` key after Phase 4, `remove_bundled_command_leftovers` is called without that argument so it falls back to its own independent raw-lock read.
- **Doctor's per-bundled-command-id check name is `"bundled-commands"` (plural)**, not `"bundled-command"` (singular) as a literal mirror of the skill side's `"bundled-skill"` would suggest — required to keep a pre-existing test (`CacheAwareCommandsDoctorTests::test_bundled_commands_ok_when_only_cache_has_commands`) passing, which substring-filters doctor output on the literal string `"bundled-commands"`.
- **Fixed a real, unplanned bug surfaced by Phase 3**, not listed in tasks.md: `doctor.py`'s `_check_agent_outputs` "expected commands" computation only unioned the hand-authored and recipe-managed-cache tiers, never the CLI-bundled cache tier. Once Phase 2's 3-tier merge started including bundled commands in every agent's `commands_dir`, this would have produced permanent false "stale command" WARNs on every project using bundled commands. Added `_bundled_command_names()` and included it in the union.
- **Fixed `./tests/validate.sh`'s only failure, which was pre-existing and unrelated to this change**: `tests/test_sync_pipeline.py::SkillSyncScriptTests.SCRIPT` pointed at `ai-specs/skills/skill-sync/assets/sync.sh` (this repo's own dogfooded project surface), a path that stopped existing when the prior `minimal-project-materialization` change (PR #145, already archived) moved CLI-bundled skills out of the project entirely. Confirmed identical failure on `development` before touching it; repointed the constant at the canonical `bundled-skills/skill-sync/assets/sync.sh` source. Fixing it was necessary to satisfy the literal "`./tests/validate.sh` exits 0" acceptance bar.
- **Fixed one incidental test coupling** in `tests/test_upgrade.py::test_help_lists_upgrade`: its `assertIn("update", ...)` only ever passed by coincidence via refresh-bundled's old "Update bundled skills/commands" help wording, unrelated to the `upgrade` command under test. Reworded to check `"installation"` (part of the `upgrade` line's own, stable text).
- **`verify-report.md` and promoting spec deltas into `openspec/specs/{...}` were left undone** (tasks.md Validation lines 126-129), by explicit direction: the assignment's stated deliverables for this apply pass are the phase commits, updated `tasks.md` checkboxes, and this `apply-progress.md` — no PR, no archive. Those two items are archive-phase concerns, not apply-phase ones.

## Risks

- The pre-existing `SkillSyncScriptTests` fix (test-file-only, one-line constant change) is outside this change's stated scope (bundled commands) but was necessary to make `./tests/validate.sh` exit 0. A reviewer should confirm this is an acceptable inclusion rather than a separate follow-up PR.
- `doctor.py`'s `_bundled_command_names()` fix (Phase 3) and the `_check_tracked_bundled_leftovers` duplicate-method removal (Phase 5) are both unplanned-but-necessary fixes discovered mid-implementation, not literally itemized in tasks.md. Both are covered by tests (the pre-existing `CacheAwareCommandsDoctorTests` suite for the former; the new `test_tracked_bundled_command_leftover_warns_without_git_rm` for the latter, which would have silently passed-but-done-nothing without the fix — worth a close look at that specific diff).
- This repo's own dogfooded `ai-specs/.ai-specs.lock` was verified live during
  verify (commit `9a3f1ce`: trimmed to `[meta]`-only, `cli_version` 0.16.0)
  and then **reverted** (`58d7762`) before shipping — that verification output
  is evidence the migration works, not a deliverable of this PR. See the
  `dogfood-verification-isolation` skill (added in a follow-up PR): this
  repo's own dogfood migration happens later, as its own ordinary act, once
  the feature is released and re-synced for real.
- Diff size: 7 feature/test phases + 1 unrelated pre-existing-failure fix, ~14 files touched. Within a single PR per the change's stated delivery strategy (phase commits, no chaining).
