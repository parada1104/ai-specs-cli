# GO-20 U2e T5 PR4a S6-1 — retire the legacy Python unit tests of four ported `lib/` modules

Base: epic/go-single-binary 668a355. Branch: change/go-20-u2e-t5-pr4a-s6-units1.
Goal: delete `tests/test_changelog.py`, `tests/test_target_resolve.py`,
`tests/test_recipe_conflicts.py` and `tests/test_spec_promotion.py` without losing any
behavior coverage that survives `lib/` (deleted in PR4b). Same method as the merged S5
slices: classify every assertion of every method as (a) legacy-implementation detail that
dies with `lib/`, or (c) behavior that must survive and must be covered by a Go test.

## Specs

- S1: Standing decision (Robert): finish PR4a — the legacy Python unit tests of already
  ported `lib/` modules are retired after their surviving behavior is verified in Go.
  Every assertion of the four files falls in exactly one class: **(a)** legacy
  implementation detail — Python module plumbing (`importlib` loaders, `inspect.getsource`
  source assertions, the `python3 lib/_internal/<module>.py` `__main__` shims,
  `ResolutionError`/`RecipeValidationError` as Python exception types, `sys.executable`
  subprocess transports) — or **(c)** behavior that must survive, which is covered by a
  Go test or is UNCOVERED. There is no (b) in this slice.
- S2: Every method and every assertion of the four files is classified in the mapping
  tables below; every (c) citation names the Go test (`file:line`) and its case, and every
  gap is marked UNCOVERED.
- S3: Every UNCOVERED (c) assertion gets a Go test added test-first (RED observed with a
  wrong expectation, GREEN with the value taken from the Python assertion). New coverage
  goes into the existing package tests (`internal/changelog/changelog_test.go`,
  `internal/target/differential_test.go`, `internal/conflicts/conflicts_test.go`) and reuses
  their fixtures; no new golden is minted and `testdata/` is untouched.
- S4: Baseline before deletion: the four files run green, rc and test count recorded so the
  later drop is reconcilable against the discover count.
- S5: The four files are deleted as unstaged deletions (plain `rm`). No production code, no
  `lib/` file, no golden and no other Python test is touched.
- S6: `gofmt -l` on the touched Go dirs is empty, `go vet` and `go test` on the touched
  packages rc=0.
- S7: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh` rc=0; the
  Python discover count drops by exactly the four files' 96 tests (2207 → 2111).
- S8: No test may exec `lib/` or `python3`: these four modules are ported, and the Go tests
  that replace them call the native packages or the frozen goldens only.

## Baseline (before any deletion)

`python3 -m unittest tests.test_changelog tests.test_target_resolve
tests.test_recipe_conflicts tests.test_spec_promotion -v` → `Ran 96 tests in 0.507s — OK`,
rc=0 (`logs/01-baseline.log`, `.rc`). Method inventory from `grep -c '    def test_'`:
42 (changelog) + 12 (target_resolve) + 19 (recipe_conflicts) + 23 (spec_promotion) = 96,
matching `Ran 96 tests`.

## Mapping tables

Classification: (a) legacy implementation detail, (c) surviving behavior.
Totals: 96 methods = 4 (a) + 92 (c); UNCOVERED (c) = 12 rows, closed by 3 Go test
additions (12 sub-cases).

### tests/test_changelog.py — 42 methods: 1 (a), 41 (c)

Go package: `internal/changelog` (frozen differential #500:
`internal/changelog/testdata/changelog_golden.json`, driven by
`internal/changelog/changelog_test.go:514` `TestDifferentialCLI`, corpus × 9 (current,new)
pairs × {summary, `--notices`}).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_parses_released_versions_and_skips_unreleased` | versions `[0.22.0, 0.21.0, 0.20.1, 0.20.0]`, no `Unreleased` | (c) | `changelog_test.go:65` `TestParsesReleasedVersionsAndSkipsUnreleased` |
| 2 | `test_section_carries_its_date_and_body` | date `2026-08-17`, body holds the bullet, stops before the next heading | (c) | `:73` `TestSectionCarriesDateAndBody` |
| 3 | `test_range_is_exclusive_of_current_and_inclusive_of_new` | `["0.22.0","0.21.0","0.20.1"]` | (c) | `:86` `TestRangeIsExclusiveOfCurrentInclusiveOfNew`; golden pair 0.20.0→0.22.0 (`:514`) |
| 4 | `test_single_version_step` | `["0.22.0"]` | (c) | `:514` golden grid pair 0.21.0→0.22.0 (both output modes) |
| 5 | `test_summary_order_is_newest_first` | first/last of the crossed range | (c) | `:86`; `:514` golden stdout order |
| 6 | `test_versions_are_ordered_by_semver_not_string` | `0.9.0` sorts below `0.22.0` | (c) | `:94` `TestVersionsOrderedBySemverNotString` |
| 7 | `test_same_version_crosses_nothing` | `[]` | (c) | `:103` `TestSameAndDowngradeCrossNothing`; golden pair 0.22.0→0.22.0 |
| 8 | `test_downgrade_crosses_nothing` | `[]` | (c) | `:103`; golden pair 0.22.0→0.21.0 |
| 9 | `test_extracts_upgrade_notice_from_a_section` | notice holds `ai-specs sync` and `falls back to Bash` | (c) | `:112` `TestUpgradeNoticeStopsAtNextSubsection` |
| 10 | `test_notice_stops_at_the_next_subsection` | no `404` / no `Fixed` | (c) | `:112` |
| 11 | `test_section_without_a_notice_returns_none` | `None` | (c) | `:126` `TestSectionWithoutNotice` |
| 12 | `test_notices_replay_oldest_first` | `[0.20.1, 0.22.0]` | (c) | `:133` `TestNoticesReplayOldestFirst` |
| 13 | `test_no_notices_in_range_returns_empty` | `[]` | (c) | `:514` golden grid pair 0.20.1→0.21.0 with `--notices` (empty notices body) |
| 14 | `test_missing_file_returns_empty_without_raising` | `[]` | (c) | `:145` `TestDegradation` |
| 15 | `test_unparseable_text_returns_empty_without_raising` | `[]` | (c) | `:145` |
| 16 | `test_malformed_heading_is_skipped_not_fatal` | `[0.22.0]` only | (c) | `:145` |
| 17 | `test_unknown_current_version_still_reports_the_target` | `[0.22.0]` | (c) | `:145` |
| 18 | `test_heading_without_a_date_is_still_parsed` | version kept, date `None` | (c) | `:161` `TestHeadingWithoutDate`; golden case `nodate` |
| 19 | `test_bullets_come_from_added_and_changed` | both Added bullets present | (c) | `:214` `TestSummaryBullets` |
| 20 | `test_bullets_exclude_the_upgrade_notice_prose` | no `falls back to Bash` | (c) | `:214` |
| 21 | `test_bullets_are_capped` | `len == 3` | (c) | `:246` `TestBulletCapAndRemaining` |
| 22 | `test_bullet_markup_is_stripped_to_plain_text` | `["Bold thing: does stuff."]` | (c) | `:235` `TestBulletMarkupStrippedAndJoined` |
| 23 | `test_multiline_bullet_is_joined` | one joined bullet | (c) | `:235` |
| 24 | `test_remaining_count_reports_what_was_dropped` | `7` | (c) | `:246` |
| 25 | `test_remaining_count_is_zero_when_nothing_dropped` | `0` | (c) | `:214` (`RemainingCount(latest, 50) == 0`) |
| 26 | `test_bullet_is_cut_at_the_first_sentence` | `["Thing: the short claim."]` | (c) | `:260` `TestFirstSentenceAndTruncation` |
| 27 | `test_long_bullet_without_a_sentence_break_is_truncated` | ≤100 runes, ends with `…` | (c) | `:260` |
| 28 | `test_truncation_does_not_split_a_word` | no `alph…` | (c) | `:260` |
| 29 | `test_short_bullet_is_left_alone` | `["A short one."]` | (c) | `:214` (short unmarked bullets pass through unchanged) |
| 30 | `test_real_changelog_bullets_stay_scannable` | every bullet of the repository `CHANGELOG.md` ≤100 runes | (c) | UNCOVERED (the golden uses the frozen `testdata/real_changelog.md`; nothing parses the live file) → closed in L3 by `:320` `TestRepositoryChangelogStaysScannable` |
| 31 | `test_section_without_bullets_returns_empty` | `[]` | (c) | `:280` `TestSectionWithoutBullets` |
| 32 | `test_en_dash_separator_is_accepted` | en dash → version + date | (c) | `:168` `TestAllDashSeparators`; golden case `endash` |
| 33 | `test_all_dash_separators_are_accepted` | `—`, `–`, `-` | (c) | `:168` |
| 34 | `test_duplicate_version_headings_are_collapsed` | `["1.0.0"]` | (c) | `:178` `TestDuplicateHeadingsCollapsed`; golden case `dupes` |
| 35 | `test_duplicate_version_is_rendered_once_in_a_range` | `["1.1.0","1.0.0"]` | (c) | `:178` |
| 36 | `test_real_changelog_has_no_duplicate_versions_after_parsing` | live `CHANGELOG.md` has no duplicate version | (c) | UNCOVERED → closed in L3 by `:320` |
| 37 | `test_heading_inside_a_code_fence_is_not_a_section_boundary` | fence is not a boundary, body keeps the fix | (c) | `:190` `TestFencedHeadingIsNotABoundary`; golden case `fenced` |
| 38 | `test_notice_is_not_truncated_by_a_fenced_heading` | notice keeps `Then you are done.`, excludes `unrelated` | (c) | `:202` `TestNoticeNotTruncatedByFencedHeading`; golden `noticefence` |
| 39 | `test_cli_notices_branch_uses_the_shared_helper` | `inspect.getsource(main)` mentions `_notices_for`/`crossed_notices` and no `upgrade_notice(` | (a) — a source assertion on Python plumbing | — |
| 40 | `test_cli_and_helper_agree` | `_notices_for(select_range(...))` equals `crossed_notices(...)` | (c) | `:514` golden `--notices` runs pin the rendered pairs byte-for-byte |
| 41 | `test_parses_the_repository_changelog` | live `CHANGELOG.md` holds `0.22.0`/`0.21.0`, not `Unreleased` | (c) | UNCOVERED → closed in L3 by `:320` |
| 42 | `test_real_changelog_crossing_is_ordered` | live crossing 0.19.0→0.22.0 is `[0.22.0,0.21.0,0.20.1,0.20.0]` | (c) | UNCOVERED → closed in L3 by `:320` |

UNCOVERED: 4 (rows 30, 36, 41, 42), closed in L3 by one addition.

### tests/test_target_resolve.py — 12 methods: 0 (a), 12 (c)

Go package: `internal/target` (frozen differential:
`internal/target/testdata/target_golden.json`, driven by
`internal/target/differential_test.go:224` `TestDifferentialPlanJSON` — the golden stores
the whole plan JSON / stderr / exit code per case).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_multi_target_fixture_contains_declared_package_directories` | the committed fixture has `packages/a` and `packages/b` | (c) — the fixture is load-bearing for the frozen record | `:224` subtest `fixture-multi-target` (golden `targets[].rel = [".","packages/a","packages/b"]` fails if the dirs go away) |
| 2 | `test_resolves_root_and_subrepos_in_manifest_order_with_dedup` | rels `[".","packages/a","packages/b"]`, `gitmodules.mode = advisory-only`, root `derived_artifacts` list | (c) | `:224` `fixture-multi-target` (whole plan JSON, including `derived_artifacts`) |
| 3 | `test_root_only_manifest_keeps_single_target` | one target, `kind = root` | (c) | `:224` `fixture-root-only` |
| 4 | `test_normalized_project_subrepos_preserve_existing_resolution_semantics` | `[" packages/a ", "packages/a", "", "packages//b", 7]` → `[".","packages/a","packages/b"]` | (c) | partial at `:224` (`backslash-and-duplicate`, `normpath-collapse`, `empty-entries`, `non-string-filtered` cover the `//`/`.`/dup/empty/non-string legs) but the padded-string leg is UNCOVERED → closed in L4 by `:737` `TestDeclaredOnlyTargetSetGoOnly/padded-whitespace-and-non-strings` |
| 5 | `test_rejects_escape_path` | `ResolutionError` whose text holds `escapes the root` | (c) for the reason text; the Python exception type is (a) | `:513` `TestDifferentialErrors/escape` (golden stderr `{"error": {"path": "../out", "reason": "escapes the root"}}`); `:668` `TestNormalizeDeclaredRelpathGoOnly` |
| 6 | `test_rejects_missing_directory_via_cli` | exit ≠ 0, stderr JSON `error.path = packages/missing`, reason holds `does not exist` | (c) | `:513` `TestDifferentialErrors/missing-dir` (exit 1 + exact envelope `"reason": "directory does not exist"`) |
| 7 | `test_plan_emits_declared_only_topology_and_planning_root` | `declared_only`, `fanout_targets`, `planning_root`, `topology.resolved/via`, `worktrees_dir = .worktrees` | (c) | `:224` `fixture-multi-target` (every field is in the frozen plan JSON) |
| 8 | `test_all_targets_share_one_planning_root` | one distinct `planning_root` across targets | (c) | `:224` `fixture-multi-target` (`targets[].planning_root` = the root path) |
| 9 | `test_empty_subrepos_emit_empty_fanout_with_gitmodules_present` | `fanout_targets = []`, `declared_only`, rels `["."]` | (c) | empty-fanout leg: `:224` `empty-entries`; the `.gitmodules`-present leg is UNCOVERED → closed in L4 by `:737` `empty-subrepos-with-gitmodules` |
| 10 | `test_gitmodules_never_expands_the_target_set` | `.gitmodules` names `packages/b`, fanout stays `["packages/a"]`, rels `[".","packages/a"]` | (c) | UNCOVERED (no golden case has a `.gitmodules` declaring an undeclared path) → closed in L4 by `:737` `gitmodules-never-expands` |
| 11 | `test_plan_topology_reflects_configured_repo_topology` | legacy recipe key → `resolved`, `via = config` | (c) | `:224` `topology-legacy-recipe` (golden: `resolved: standalone`, `via: config`, `source: legacy-recipe`) |
| 12 | `test_plan_topology_prefers_project_field_over_legacy_recipe` | `[project].repo_topology` wins, `via = config`, `source = project` | (c) | `:707` `TestTopologyUnitGoOnly` "precedence" (project beats the recipe key) + `:224` `topology-project` |

UNCOVERED: 3 (rows 4-padded, 9-gitmodules, 10), closed in L4 by one addition with three cases.

### tests/test_recipe_conflicts.py — 19 methods: 2 (a), 17 (c)

Go package: `internal/conflicts` (frozen differential #499:
`internal/conflicts/testdata/recipe_conflicts_golden.json`, driven by
`internal/conflicts/conflicts_test.go:247` `TestConflictsDifferential`, 10 fabricated
catalog cases). The corpus catalog declares capabilities inside `[provides]`, which the
recipe schema does not read, so the golden can never express the capability legs; they are
pinned natively in L5.

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_no_conflict_for_distinct_recipes` | `[]` | (c) | `:247` case `no-conflicts` |
| 2 | `test_detects_skill_conflict` | 1 conflict, type `skill`, id, recipe **names** `{A, B}` | (c) | `:247` `skill-command-tag-cap` (`{"type":"skill","id":"s2","recipes":["Rec A","Rec B"]}`) |
| 3 | `test_detects_command_conflict` | type `command`, id `shared-cmd` → same shape | (c) | `:247` `skill-command-tag-cap` (command `cmd1`, `["Rec A","Rec C"]`) |
| 4 | `test_detects_mcp_conflict` | type `mcp`, id `shared-mcp` → same shape | (c) | `:247` `mcp-collision` (`{"type":"mcp","id":"srv","recipes":["M1","M2"]}`) |
| 5 | `test_cli_exits_zero_when_no_conflict` | `python3 lib/_internal/recipe-conflicts.py <catalog> <id>` exits 0 | (a) — the legacy module `__main__` shim dies with `lib/` | — |
| 6 | `test_cli_exits_one_when_conflict` | same script exits 1 and names the conflict on stderr | (a) | — |
| 7 | `test_capability_ambiguity_warning` | two providers, no binding → 1 `warning`, id `tracker` | (c) | UNCOVERED in the golden → closed in L5 by `:164` `TestCapabilityAndTagConflictsGoOnly/ambiguity is a warning` |
| 8 | `test_capability_explicit_binding_resolves_ambiguity` | `[{capability: tracker, recipe: recipe-a}]` → `[]` | (c) | UNCOVERED → closed in L5 by `:164` `explicit binding resolves ambiguity` |
| 9 | `test_capability_duplicate_explicit_fatal` | two bindings for one capability → 1 `fatal` | (c) | `:247` `duplicate-explicit-binding-fatal`; also `:164` `duplicate explicit binding is fatal` |
| 10 | `test_capability_single_provider_no_conflict` | one provider → `[]` | (c) | UNCOVERED → closed in L5 by `:164` `single provider is not a conflict` |
| 11 | `test_capability_disabled_excluded` | a disabled second provider does not create ambiguity | (c) | UNCOVERED → closed in L5 by `:164` `single provider is not a conflict` (only `recipe-a` enabled) |
| 12 | `test_no_shared_tag_no_conflict` | distinct tags → `[]` | (c) | `:247` `no-conflicts` (tag slice empty) |
| 13 | `test_single_recipe_no_conflict` | one recipe → `[]` | (c) | UNCOVERED (the corpus always enables two or more) → closed in L5 by `:164` `single recipe has no tag conflict` |
| 14 | `test_duplicate_tag_on_single_recipe_no_conflict` | `tags = [vcs, vcs]` on one recipe does not self-conflict | (c) | `:247` `dup-inside-one-recipe-stops-registration` (`dup` holds `tags = ["solo","solo"]` and the tag slice stays empty) |
| 15 | `test_shared_tag_without_conflicts_with_is_warning` | shared tag, no `conflicts_with` → `warning`, tag, recipes | (c) | `:247` `skill-command-tag-cap` (`{"type":"tag_conflict","id":"vcs","recipes":["a","b"],"severity":"warning"}`) |
| 16 | `test_shared_tag_with_conflicts_with_is_fatal` | declared conflict → `fatal` | (c) | `:247` `skill-command-tag-cap` (tag `tracker`, `severity: fatal`) |
| 17 | `test_conflicts_with_is_symmetric` | only the *second* recipe declares, still `fatal` | (c) | `:247` `order-reversed` (`c,b,a`: `a` declares `conflicts_with=["c"]`, the pair is fatal in either order) |
| 18 | `test_to_dict_output_format` | `type = tag_conflict`, `tag`, sorted `recipes` | (c) | `:247` tag payloads (`type`/`id`/sorted `recipes` of `Conflict`, `conflicts.go:19`) |
| 19 | `test_catalog_vcs_recipes_warn_when_enabled_together` | the two shipped VCS recipes warn on tag `vcs` | (c) | UNCOVERED → closed in L5 by `:164` `catalog vcs recipes warn when enabled together` |

UNCOVERED: 5 (rows 7, 8, 10, 11, 13, 19 — 10/11 share one subtest), closed in L5 by one
addition with six subtests.

### tests/test_spec_promotion.py — 23 methods: 1 (a), 22 (c)

Go packages: `internal/specs` (U-A port) and `internal/cli` (the native verb
`ai-specs specs promote`, `internal/cli/specs.go`). The Python module's entry points
(`python3 lib/_internal/spec_promotion.py <slug> --root <root>`) survive as the native
verb, so those CLI assertions are (c), not (a).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_added_requirement_is_appended_into_existing_canonical` | ADDED appended, existing kept, `changed`, folder parity clean | (c) | `internal/specs/compose_test.go:21` |
| 2 | `test_modified_requirement_replaces_canonical_block` | MODIFIED replaces by name, untouched kept, one block | (c) | `compose_test.go:43` |
| 3 | `test_new_domain_creates_a_canonical_spec` | canonical created, no delta scaffolding | (c) | `compose_test.go:57` |
| 4 | `test_rerun_is_idempotent_and_resume_safe` | second run: no blockers, `changed=False`, same bytes, no temp leftovers | (c) | `internal/specs/promote_test.go:22` |
| 5 | `test_unrelated_canonical_requirements_and_sections_are_preserved` | untouched block, `## Notes`, title survive | (c) | `compose_test.go:88` |
| 6 | `test_added_collision_with_different_content_is_rejected` | blocker mentions `collides`, file unchanged | (c) | `compose_test.go:102` (exact blocker text) |
| 7 | `test_missing_modified_target_is_rejected` | blocker mentions MODIFIED, file unchanged | (c) | `compose_test.go:120` |
| 8 | `test_renamed_operation_is_unsupported` | blocker mentions RENAMED, file unchanged | (c) | `internal/specs/delta_test.go:37` |
| 9 | `test_removed_is_refused_without_the_explicit_safe_path` | refused without `allow_removed`; allowed path removes only that block | (c) | `promote_test.go:57` (+ verb flag `internal/cli/specs_test.go:144`) |
| 10 | `test_one_blocked_domain_aborts_every_write` | blocked domain aborts the clean domain's write | (c) | `promote_test.go:85` |
| 11 | `test_active_same_domain_collision_blocks_before_any_write` | promoted report blocked, names `sibling` + `capability`, file unchanged | (c) for the collision; the `sys.executable` subprocess leg is (a) | `promote_test.go:108` (exact `active change "sibling" …` blocker); verb-level exit 1 + `BLOCKED` on stderr: `internal/cli/specs_test.go:103` |
| 12 | `test_active_collision_on_one_domain_blocks_every_domain` | every domain blocked, clean domain not written | (c) | `promote_test.go:125` |
| 13 | `test_archived_unrelated_and_spec_less_changes_do_not_collide` | archived/sibling-other-domain/task-only do not block | (c) | `promote_test.go:146` |
| 14 | `test_symlinked_active_change_is_not_followed_by_the_collision_scan` | symlinked change ignored, outside delta untouched | (c) | `promote_test.go:180` |
| 15 | `test_slug_traversal_and_root_escape_are_rejected` | `../escape`, `escape/../..`, `/abs` all blocked, nothing written outside | (c) | `internal/specs/paths_test.go:22` |
| 16 | `test_non_planning_root_is_rejected` | no `openspec/` → blocked | (c) | `paths_test.go:36` |
| 17 | `test_symlinked_canonical_domain_is_rejected` | blocked, outside dir stays empty | (c) | `paths_test.go:45` |
| 18 | `test_symlinked_openspec_ancestor_is_rejected` | blocked, nothing written through the link | (c) | `paths_test.go:67` |
| 19 | `test_missing_specs_directory_is_a_no_op` | no blockers, `changed=False`, parity clean, specs dir empty | (c) | `promote_test.go:216` |
| 20 | `test_promoter_has_no_network_surface` | module source holds no `urllib`/`socket`/`requests`/`http.client`/`subprocess` | (a) — a source scan of the Python file | — |
| 21 | `test_cli_promotes_with_a_required_root_and_slug` | exit 0, canonical written, rerun prints `already promoted` | (c) | `internal/cli/specs_test.go:60` (exact stdout incl. `already promoted`) |
| 22 | `test_cli_requires_the_root_flag` | exit 2, stderr names `--root` | (c) | `specs_test.go:180` |
| 23 | `test_cli_exits_nonzero_when_promotion_is_blocked` | exit 1, stderr holds `BLOCKED` | (c) | `specs_test.go:103` (exit 1 + exact `spec-promotion: BLOCKED` stderr, file untouched) |

UNCOVERED: 0. The file is fully retired by existing Go coverage.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S3 | inline | |
| T2 | S4, S5 | inline | |
| T3 | S6, S7 | inline | |

## Log

- L1 (Robert): "ok,vos dale debemos ir terminando esto" — approving continuing PR4a: retire
  the legacy Python unit tests of the `lib/` modules whose behavior is already ported to Go,
  after verifying that coverage in Go.
- L2 — Baseline before deletion (S4): `python3 -m unittest tests.test_changelog
  tests.test_target_resolve tests.test_recipe_conflicts tests.test_spec_promotion -v` →
  `Ran 96 tests in 0.507s — OK`, rc=0 (`logs/01-baseline.log`, `.rc`), 0 skipped. Inventory
  42 + 12 + 19 + 23 = 96 matches `Ran 96 tests`.
- L3 — Changelog gaps closed test-first (rows 30, 36, 41, 42), one addition to
  `internal/changelog/changelog_test.go`: `TestRepositoryChangelogStaysScannable` parses the
  live `CHANGELOG.md` (not the frozen copy) and pins released versions `0.22.0`/`0.21.0`, no
  `Unreleased` section, no duplicate version, every summary bullet ≤100 runes, and the
  `0.19.0..0.22.0` crossing `[0.22.0, 0.21.0, 0.20.1, 0.20.0]`. RED `logs/02-red-changelog.log`
  rc=1: the wrong expectation `crossed = [0.22.0, 0.21.0]` was reported while the live file
  actually crosses 4 versions; GREEN with the Python-asserted list (`logs/06-green-changelog.log`
  rc=0).
- L4 — Target gaps closed test-first (rows 4-padded, 9-gitmodules, 10), one addition to
  `internal/target/differential_test.go`: `TestDeclaredOnlyTargetSetGoOnly` covers
  `padded-whitespace-and-non-strings` (`" packages/a "` trims, `packages//b` normalizes, `""`
  and `7` drop), `empty-subrepos-with-gitmodules` (fanout `[]`, rels `["."]`), and
  `gitmodules-never-expands` (`.gitmodules` names `packages/b`, fanout stays
  `["packages/a"]`). RED `logs/03-red-target.log` rc=1: the wrong expectation
  `fanout = [packages/a packages/b]`; GREEN `logs/06-green-target.log` rc=0.
- L5 — Conflicts gaps closed test-first (rows 7, 8, 10, 11, 13, 19), one addition to
  `internal/conflicts/conflicts_test.go`: `TestCapabilityAndTagConflictsGoOnly` builds a
  three-recipe catalog with top-level `[[capabilities]]` (the frozen corpus declares them
  under `[provides]`, which the schema does not read) and pins the ambiguity warning, the
  explicit binding that silences it, the duplicate-binding fatal, the single-provider and
  single-recipe non-conflicts, and the shipped `git-pr-flow` + `bitbucket-pr-flow` tag `vcs`
  warning. RED `logs/04-red-conflicts.log` rc=1: the wrong expectation `severity fatal` for
  the ambiguity; GREEN `logs/06-green-conflicts.log` rc=0.
- L6 — Deletion (S5): plain `rm` of the four files. `git status --short` →
  ` D tests/test_changelog.py`, ` D tests/test_recipe_conflicts.py`,
  ` D tests/test_spec_promotion.py`, ` D tests/test_target_resolve.py` (unstaged), plus the three
  modified Go test files and this document. No production file, no `lib/` file, no golden and no
  other Python test touched.
- L7 — (S6) `gofmt -l internal/changelog/ internal/target/ internal/conflicts/ internal/specs/
  internal/cli/` → no output, rc=0 (`logs/05-gofmt.log`). `go vet` on the same five packages rc=0
  (`logs/07-vet.log`). `go test -count=1` on them: the first run failed only in `internal/cli`
  because `TestUpgradeUsageMatchesTheShellSurface` needs `dist/ai-specs`; after
  `go build -o dist/ai-specs ./cmd/ai-specs` the same command is rc=0
  (`logs/08-test.log`: changelog 0.270 s, target 1.786 s, conflicts 0.255 s, specs 0.330 s,
  cli 4.531 s). `scripts/build-gate.sh` rc=0 (4 targets, `logs/09b-build-gate.log`) so the
  validate parity legs have their gate binary.
- L8 — (S7) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/09-build-cli.log`), then
  `./tests/validate.sh` rc=0 in ~578 s (`logs/10-validate.log`): gofmt clean, both Go modules
  green, `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`, and the Python
  discover leg `Ran 2111 tests in 577.931s — OK (skipped=1, expected failures=3)`. Count
  reconciliation: 2207 (base) − 96 (the four retired files) = 2111 exactly; the single skip and
  the three expected failures are pre-existing and elsewhere.
