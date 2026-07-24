## Verification Report

**Change**: relocate-bundled-commands
**Version**: N/A (delta change; on-disk specs updated for skill-source-precedence, sync-lock, project-doctor)
**Mode**: Strict TDD (`openspec/config.yaml` → `strict_tdd: true`)
**Worktree**: `.worktrees/relocate-bundled-commands` @ `change/relocate-bundled-commands`
**Verified**: 2026-07-24

### Completeness

| Metric | Value |
|--------|-------|
| Implementation tasks total | 30 |
| Implementation tasks complete | 30 |
| Validation tasks total | 5 |
| Validation tasks complete | 3 (verify-report.md + spec promotion are this document + the archive step) |

All Phase 1–7 checkboxes in `tasks.md` are `[x]`. `apply-progress.md` records
9 commits with phase-level RED/GREEN evidence.

### Build & Tests Execution

**Build**: ✅ Passed (via `./tests/validate.sh`: `py_compile` + `bash -n`)

**Tests**: ✅ 1044 passed / ❌ 0 failed / ⚠️ 0 skipped — independently
re-run by the verifier (not just trusted from apply-progress.md):

```text
Ran 1044 tests in 264.436s
OK
```

**Coverage**: ➖ Not available (no coverage tool in project capabilities)

### Live end-to-end verification (beyond unit tests)

Ran the actual built CLI against this repo's own dogfooded, pre-existing
`ai-specs/ai-specs.toml` project (real legacy state: `cli_version = "0.15.0"`,
`[commands]`/`[opted-out]` sections, two committed byte-identical
bundled-command copies) — not a synthetic fixture:

1. `./bin/ai-specs sync .` → lock trimmed to `[meta]` only
   (`cli_version = "0.16.0"`), `ai-specs/commands/{rules-audit,skills-as-rules}.md`
   removed from disk by `remove_bundled_command_leftovers`, fan-out to
   `.claude/commands/`, `.cursor/commands/`, `.opencode/commands/`,
   `.omp/commands/` still serves both (7 commands each) from the cache merge.
2. `./bin/ai-specs doctor .` → correctly WARNed
   `tracked-bundled-leftover  2 removed CLI-bundled command(s) still tracked in git`
   with exact `git rm --cached` guidance naming both paths.
3. Applied the doctor-recommended `git rm --cached` (never done automatically
   by the CLI — confirmed the index was otherwise untouched by `doctor`/`sync`).
4. Re-ran `doctor` → clean: `OK bundled-commands` (both, resolved from
   `{cache}/.bundled/commands/`), no tracked-leftover WARN.

This is real proof the migration path works end-to-end, not just against
`tempfile.mkdtemp()` fixtures.

### Strict TDD Evidence

| Check | Result | Notes |
|-------|--------|-------|
| TDD Cycle Evidence table in apply-progress | ✅ Present | Phase-level RED/GREEN for Phases 1–7, with actual failure messages quoted |
| Covering test files exist | ✅ | `test_project_cache`, `test_external_dirs`, `test_command_merge`, `test_lock`, `test_doctor`, `test_harness_cli_literacy`, `test_rules_audit` |
| GREEN still green at verify | ✅ | Full suite 1044/1044 OK, independently re-run |
| Live (non-unit-test) confirmation | ✅ | Self-sync against this repo's own dogfood project (see above) |
| Per-task TRIANGULATE / SAFETY NET columns | ⚠️ Partial | apply-progress uses phase rows, not a full strict per-task matrix (matches this repo's established convention, e.g. `cli-bound-recipes`) |

### Spec Compliance Matrix

| Capability | Scenario | Test / evidence | Result |
|------------|----------|-----------------|--------|
| skill-source-precedence | Merge and fan-out | `test_command_merge.py::test_local_wins_over_managed` (pre-existing, re-verified) | ✅ COMPLIANT |
| skill-source-precedence | Bundled command resolves from cache, not project | `test_harness_cli_literacy.py` init-flatten test; live `sync .` (no `ai-specs/commands/*.md` written for bundled names) | ✅ COMPLIANT |
| skill-source-precedence | Local command shadows a CLI-bundled command of the same name | `test_command_merge.py::test_local_wins_over_bundled_and_managed_with_warning`; live smoke test customized-copy case | ✅ COMPLIANT |
| skill-source-precedence | Recipe-managed command shadows a CLI-bundled command of the same name | `test_command_merge.py::test_managed_silently_overrides_bundled` | ✅ COMPLIANT |
| sync-lock | Lock contents after sync | `test_lock.py::test_legacy_commands_opted_out_dropped_on_write`; live `sync .` lock now `[meta]`-only | ✅ COMPLIANT |
| sync-lock | Legacy command hash sections dropped on migration | `test_lock.py::test_legacy_lock_with_commands_opted_out_sections_normalized`; live migration of this repo's own 0.15.0 lock | ✅ COMPLIANT |
| project-doctor | Bundled command present | `test_doctor.py::test_bundled_command_present_reports_ok_by_name`; live `doctor .` OK output | ✅ COMPLIANT |
| project-doctor | Bundled command missing | `test_doctor.py::test_bundled_command_missing_reports_error` | ✅ COMPLIANT |
| project-doctor | Tracked bundled-command leftover guidance | `test_doctor.py::test_tracked_bundled_command_leftover_warns_without_git_rm`; live WARN + guidance reproduced exactly, git index untouched by doctor | ✅ COMPLIANT |

**Compliance summary**: 9/9 scenarios ✅ COMPLIANT.

### Correctness (Static Evidence)

| Requirement | Status | Notes |
|------------|--------|-------|
| `bundled_commands_root` / `bundled_command_ids` / `remove_bundled_command_leftovers` / `tracked_bundled_command_leftovers` | ✅ Implemented | `project-cache.py`; shared git-plumbing helper factored out rather than duplicated |
| 3-tier `merge_commands` (bundled < recipe < local) | ✅ Implemented | Ascending copy order; local warns on either lower-tier collision, recipe/bundled silent |
| `command-merge.py` removed | ✅ Confirmed dead | Repo-wide grep pre- and post-deletion shows zero callers |
| `refresh-bundled.py` flatten-only for both asset kinds | ✅ Implemented | `iter_bundled`/`project_path_for`/`display_name`/`lock_get`/`lock_set`/`lock_del`/`save_new_sidecar`/`sha256_of` removed; `flatten_bundled_commands` added |
| Lock drops `[commands]`/`[opted-out]` | ✅ Implemented | `lock.py` + `recipe-remove.sh` mirrored |
| `doctor` per-bundled-command-id check | ✅ Implemented | Replaces the old aggregate "any commands present" check |
| `doctor` tracked-leftover extension | ✅ Implemented | Found and removed a duplicate-method bug during implementation that would have silently no-op'd this check — now covered by a dedicated test |
| `init.sh` step 2b removed | ✅ Implemented | Comment mirrors the existing skills-removal explanation |
| Dogfood `ai-specs/.ai-specs.lock` (this repo) | ✅ Verified live, then reverted | Ran real `sync`, applied doctor's `git rm --cached` guidance, confirmed clean — then reverted all three resulting file changes (`git revert`) per `dogfood-verification-isolation`: verification output is evidence, not a shippable part of this PR. This repo's own dogfood migration happens later, as its own act, once the feature is released. |

### Coherence (Design)

| Decision | Followed? | Notes |
|----------|-----------|-------|
| D1 — separate `{cache}/.bundled/commands/` tier | ✅ Yes | Not reusing the recipe-managed `{cache}/commands/` |
| D2 — precedence local > recipe > bundled, silent CLI-tier shadow | ✅ Yes | Local-vs-either-lower-tier still warns (existing UX preserved) |
| D3 — leftover migration by content/lock-hash match | ✅ Yes | Both paths tested; live-verified on this repo's own legacy copies |
| D4 — drop `[commands]`/`[opted-out]` | ✅ Yes | |
| design.md "refresh() opens no lock at all" | ⚠️ Deviation (documented) | Contradicted the design doc's own next paragraph and the shipped skill-side precedent; implementer correctly followed tasks.md 3.4's literal ordering + working precedent instead. `load_lock`/`write_lock` are still called (needed for `[meta]`/`[agents.*]` and the in-memory legacy-hash migration signal) — only the per-file command bookkeeping loop was removed. This is the *right* call; design.md prose should be corrected at archive. |
| doctor check name plural (`bundled-commands`) vs skill's singular (`bundled-skill`) | ⚠️ Deviation (documented, justified) | Required to avoid breaking a pre-existing substring-filtering test; purely cosmetic, no functional impact |

### Unplanned fixes discovered during implementation (all test-covered)

1. `doctor.py`'s `_check_agent_outputs` "expected commands" set never unioned
   the CLI-bundled cache tier — would have produced permanent false "stale
   command" WARNs on every project once bundled commands started flowing
   through the 3-tier merge into agent `commands_dir`s. Fixed
   (`_bundled_command_names()`), covered by the pre-existing
   `CacheAwareCommandsDoctorTests` suite.
2. A duplicate `_check_tracked_bundled_leftovers` method silently shadowed the
   new command-aware version. Fixed; covered by
   `test_tracked_bundled_command_leftover_warns_without_git_rm` — this test
   would have passed-but-done-nothing without the fix, per the implementer's
   own risk note. **Spot-checked this diff directly** (Phase 5 commit
   `3f1dfbf`): confirmed only one method definition remains and it correctly
   handles both skill and command leftovers.
3. `tests/test_sync_pipeline.py::SkillSyncScriptTests` pointed at a path
   (`ai-specs/skills/skill-sync/assets/sync.sh`) removed by the *already
   merged* `minimal-project-materialization` change — confirmed failing on
   `development` before this branch touched it (pre-existing, unrelated
   breakage). Repointed to the canonical `bundled-skills/` source. This is a
   one-line test-only fix; acceptable to carry in this PR rather than a
   separate one (`./tests/validate.sh` was already broken on `development`
   without it).
4. `tests/test_upgrade.py::test_help_lists_upgrade` incidentally depended on
   now-removed `refresh-bundled` help wording. Reworded to check stable
   `upgrade`-specific text.

### Proposal success criteria

| Criterion | Met? |
|-----------|------|
| Bundled commands resolve from `{cache}/.bundled/commands/`; `ai-specs/commands/` hand-authored only after sync | ✅ |
| `ai-specs init` does not write bundled commands into `ai-specs/commands/` | ✅ |
| Clean upgrade migration (byte-identical removed, customized preserved) | ✅ — unit-tested AND live-verified on this repo |
| `.ai-specs.lock` has no `[commands]`/`[opted-out]` after sync | ✅ |
| `doctor` per-bundled-command + tracked-leftover WARN | ✅ |
| Per-agent fan-out unaffected | ✅ — live-verified, 7 commands each across claude/cursor/opencode/omp |
| `./tests/validate.sh` passes | ✅ |

### Issues Found

**CRITICAL**: None

**WARNING**: None blocking.

**SUGGESTION**:
1. Correct design.md's "refresh() opens no lock at all" line at archive time
   to match what was actually (correctly) built, so the archived design
   record doesn't contradict its own implementation.
2. Consider renaming doctor's bundled-command check id from plural
   `"bundled-commands"` to singular `"bundled-command"` in a follow-up,
   updating the one coupled test at the same time, for exact symmetry with
   `"bundled-skill"` — cosmetic only, not spec-relevant.
3. `tests/test_sync_pipeline.py::SkillSyncScriptTests` fix and
   `tests/test_upgrade.py::test_help_lists_upgrade` reword are both
   pre-existing/incidental — flagging for judgment-day awareness so they're
   not mistaken for scope creep; both are one-line, test-only, and necessary
   for `./tests/validate.sh` to pass at all.

### Dogfood / fixture consistency

| Item | Status |
|------|--------|
| Dogfood `ai-specs/.ai-specs.lock` (this repo) | ✅ Verified live (`[meta]`-only, `cli_version = "0.16.0"` observed), then **reverted** — not part of this PR's shipped diff |
| Dogfood `ai-specs/commands/` (this repo) | ✅ Verified the leftover-cleanup + tracked-leftover-WARN cycle live, then **reverted** — the two files remain committed as-is on `development` until a real post-release `ai-specs sync` migrates them |
| README.md bundled-commands section | ✅ Checked, still accurate (describes this repo's own CLI source-tree, not the per-project materialization model) |

### Verdict

**PASS**

30/30 implementation tasks done, 1044/1044 tests green (independently
re-verified), all 9 spec scenarios COMPLIANT, and the migration was proven
live against this repo's own real (not synthetic) legacy state end-to-end
(sync → doctor WARN → git rm --cached → doctor clean). That live-verification
state change was reverted before shipping — see `dogfood-verification-isolation`
skill (added in a follow-up PR): verification output is evidence, not a
deliverable, and must not be committed as part of the feature branch. Two
pre-existing, unrelated test fixes were necessary to make `./tests/validate.sh`
pass at all and are called out explicitly, not hidden. No CRITICAL or blocking
WARNING issues for judgment-day.

**Next recommended**: judgment-day (parent launches, blind adversarial
review).
