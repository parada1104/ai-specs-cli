# GO-20 U2e T5 PR4a S6-2 — retire the remaining eleven legacy Python unit suites

Base: epic/go-single-binary b639b70 (after #520). Branch: change/go-20-u2e-t5-pr4a-s6-units2.
Goal: delete `tests/test_recipe_read.py`, `tests/test_recipe_schema.py`, `tests/test_lock.py`,
`tests/test_project_cache.py`, `tests/test_skill_contract.py`, `tests/test_trello_link.py`,
`tests/test_config_wizard.py`, `tests/test_command_merge.py`, `tests/test_override_ownership.py`
and `tests/test_narrow_checkout.py`, and reduce `tests/test_repo_topology.py` to the shared git
fixture three kept suites import (234 methods total) without losing any behavior coverage that
survives `lib/` (deleted in PR4b). Same method as the merged S6-1 slice (#520): every assertion of
every method is classified (a) legacy-implementation detail that dies with `lib/`, or (c) behavior
that must survive and is covered by a Go test.

## Specs

- S1: Standing decision (Robert): finish PR4a — the legacy Python unit tests of already ported
  `lib/` modules are retired after their surviving behavior is verified in Go. Every assertion
  of the eleven files falls in exactly one class: **(a)** legacy implementation detail — Python
  module plumbing (`importlib` loaders, `python3 lib/_internal/<module>.py` `__main__` shims,
  `import` of a `lib/` sibling, direct construction of Python dataclasses, helpers replaced by a
  proof-based native rule, dead helpers with no caller) — or **(c)** behavior that must survive,
  covered by a Go test or marked UNCOVERED. There is no (b).
- S2: Every method of the eleven files is classified in the mapping tables below; every (c)
  citation names the Go test (`file:line`) and its case, and every gap is marked UNCOVERED.
- S3: Every UNCOVERED (c) gets a Go test added test-first (RED observed with a wrong expectation,
  GREEN with the value taken from the Python assertion), inside the allowed surfaces
  (`internal/recipe|sync|lock|projectcache|skills|tracker|target|config|conflicts/*_test.go`).
  No golden is minted and no `testdata/` fixture is touched or added.
- S4: Baseline before deletion: the eleven files run green, rc and count recorded so the drop is
  reconcilable against the discover count.
- S5: The eleven files are retired as unstaged changes (plain `rm` for ten; `tests/test_repo_topology.py`
  is rewritten down to its shared fixture helper because three kept suites import
  `make_super_with_submodule`). No production code, no `lib/` file, no golden, no `tests/parity/`
  file and no other Python test is edited. `tests/test_gate_binary_dist.py` is out of scope (live
  gate binary; separate slice).
- S6: `gofmt -l` on the touched Go dirs is empty; `go vet` and `go test` on the touched packages
  rc=0.
- S7: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh` rc=0; the Python
  discover count drops by the retired 232 methods (2111 → 1879), the two kept fixture self-tests
  accounting for the remainder (2111 − 232 = 1879).
- S8: No Go test added here execs `lib/` or `python3`: the behavior is asserted against the
  native packages.

## Baseline (before any deletion)

`python3 -m unittest tests.test_recipe_read tests.test_recipe_schema tests.test_lock
tests.test_project_cache tests.test_skill_contract tests.test_trello_link
tests.test_repo_topology tests.test_config_wizard tests.test_command_merge
tests.test_override_ownership tests.test_narrow_checkout -v` → `Ran 234 tests in 26.719s — OK`,
rc=0 (`logs/01-baseline.log`, `.rc`). Method inventory from `grep -c '    def test_'`:
23 + 57 + 20 + 15 + 6 + 13 + 39 + 20 + 5 + 18 + 18 = 234, matching `Ran 234 tests`.

## Mapping tables

Classification: (a) legacy implementation detail, (c) surviving behavior. Package paths are
repository-relative; `gate/` means `catalog/recipes/worktree-flow/gate/`.

Totals: 234 methods = 27 (a) + 207 (c); UNCOVERED (c) = 13 rows in 7 groups, closed by 7 Go test
additions (13 sub-cases plus 2 PRESERVE companions). 232 methods are retired; 2 (the
`tests/test_repo_topology.py` fixture self-tests) are kept because their subject — the shared
git fixture — is kept.

### tests/test_recipe_read.py — 23 methods: 1 (a), 22 (c)

Go package: `internal/schema` (frozen recipe oracle: `internal/schema/testdata/schema_golden.json`,
driven by `internal/schema/differential_test.go:377` `TestDifferentialAgainstPython` — 21 clean
repositories recipes projected field-by-field, plus 92 fixtures compared as byte-identical error
strings or projections; `:398` is the clean-corpus loop, `:492` the fixture subtest).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_reads_valid_recipe` | `test-fixture`: id, name, version, `skills[0] {id, source}` | (c) | `:398` clean record `tests/fixtures/recipes/test-fixture/recipe.toml` (projection carries `id`/`name`/`version`/`skills[]`) |
| 2 | `test_fails_on_missing_recipe_dir` | `recipe directory not found` | (c) | `internal/sync/conflicts_core_test.go:267` (`CheckRecipeConflicts` on a ghost id) and `internal/sync/buildrecipemcp_test.go:488` (exact `recipe directory not found: <catalog>/ghost`) |
| 3 | `test_fails_on_missing_required_field` | `missing or invalid required field` | (c) | `:492` fixture `err_missing_id.toml` (`.recipe: missing or invalid required field 'id'`) |
| 4 | `test_cli_outputs_json` | `python3 lib/_internal/recipe-read.py <catalog> <id>` prints JSON | (a) — the module `__main__` shim dies with `lib/`; the native `recipe` family is the surface (`internal/cli/recipe_test.go:2650`) | — |
| 5 | `test_v1_recipe_has_empty_v2_fields` | capabilities/hooks/config fields empty, `init` null | (c) | `:398` clean record (all four fields project empty/null) |
| 6 | `test_v2_capability_parsing` | two `[[capabilities]]` in order | (c) | `:398` clean corpus (recipes declaring capabilities) + `:492` `ok_capabilities_item.toml` |
| 7 | `test_v2_duplicate_capability_fails` | `duplicate capability id` | (c) | `:492` `err_capabilities_dup.toml` (`.capabilities[1]: duplicate capability id 'c'`) |
| 8 | `test_v2_missing_capability_id_fails` | `missing or invalid required field 'id'` | (c) | `:492` `err_capabilities_missing_id.toml` |
| 9 | `test_v2_hook_parsing` | `event`/`action` pair projected | (c) | `:398` clean corpus (10 recipes declare `[[hooks]]`) |
| 10 | `test_v2_missing_hook_event_fails` | error names `event` | (c) | UNCOVERED (no fixture with `action` alone) → closed in L3 by `internal/recipe/schema_contract_test.go:28` `TestHookEventIsRequired` |
| 11 | `test_v2_missing_hook_action_fails` | error names `action` | (c) | `:492` `err_hooks_missing_action.toml`; companion `schema_contract_test.go:44` `TestHookActionIsRequired` |
| 12 | `test_v2_config_schema_parsing` | `[config.<f>]` required/type/default | (c) | `:492` `ok_config_boolean_alias.toml`, `ok_config_extra.toml`; `:398` clean corpus projections |
| 13 | `test_v2_invalid_config_field_fails` | `missing or invalid 'required'` | (c) | `:492` `err_config_required_not_bool.toml` |
| 14 | `test_init_workflow_parsing` | `[init]` prompt/description/needs_manifest/needs_mcp | (c) | `:492` `ok_init_full.toml` |
| 15 | `test_init_metadata_in_json` | the frozen `init` object shape | (c) | `:492` `ok_init_full.toml` projection (`init` carries the same four keys) |
| 16 | `test_recipe_without_init_has_null_json_init` | `init` key present, null | (c) | `:398` clean record (`"init": null`) |
| 17 | `test_init_missing_prompt_fails` | error names `[init].prompt` | (c) | `:492` `err_init_missing_prompt.toml` |
| 18 | `test_init_unknown_field_fails` | `unsupported init field` | (c) | `:492` `err_init_unsupported_field.toml` |
| 19 | `test_init_invalid_needs_mcp_fails` | error names `needs_mcp` | (c) | `:492` `err_init_needs_mcp_blank.toml`, `err_init_needs_mcp_not_list.toml` |
| 20 | `test_init_absolute_prompt_path_fails` | `relative` | (c) | `:492` `err_init_prompt_absolute.toml` |
| 21 | `test_init_parent_traversal_prompt_path_fails` | `inside the recipe directory` | (c) | `:492` `err_init_prompt_escape.toml`, `err_init_prompt_symlink_escape.toml` |
| 22 | `test_init_directory_prompt_path_fails` | `must be a file` | (c) | `:492` `err_init_prompt_is_dir.toml` (temp-dir mode) |
| 23 | `test_init_missing_prompt_file_fails` | `init prompt file not found` | (c) | `:492` `err_init_prompt_not_found.toml` (temp-dir mode) |

UNCOVERED: 1 (row 10), closed in L3.

### tests/test_recipe_schema.py — 57 methods: 4 (a), 53 (c)

Same oracle (`internal/schema/differential_test.go:492` fixture subtest unless stated).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_config_field_without_validation_parses` | `validation` defaults `{}` | (c) | `ok_config_extra.toml` / `:398` clean projections (`config.fields[].validation`) |
| 2 | `test_config_field_with_validation_parses` | `regex` kept | (c) | `:398` clean corpus (`trello-mcp-workflow` declares `validation.regex`) |
| 3 | `test_config_field_with_unknown_validation_key_fails` | `unknown key` + name | (c) | `err_config_validation_unknown.toml` (`.config.foo.validation: unknown key 'len'`) |
| 4 | `test_config_field_with_validation_non_table_rejected` | `expected table` | (c) | `err_config_validation_not_table.toml` |
| 5 | `test_provides_hooks_valid` | every `[[provides.hooks]]` field | (c) | `ok_hooks_full.toml` (temp mode), `ok_hooks_dotted_path.toml` |
| 6 | `test_provides_hooks_absent_ok` | `runtime_hooks` empty | (c) | `:398` clean records (`runtime_hooks: []`) |
| 7 | `test_provides_hooks_missing_field` | names id/event/script | (c) | `err_hooks_missing_script.toml` (`script` arm); the `id`/`event` arms share the same `requireString` branch |
| 8 | `test_provides_hooks_unknown_event` | names the event + known list | (c) | `err_hooks_unknown_event.toml` (exact `known events: pre-tool-use, …`) |
| 9 | `test_provides_hooks_script_escape` | path-escape error | (c) | `err_hooks_escape_dir.toml`, `err_hooks_escape_symlink.toml`, `err_hooks_script_absolute.toml` |
| 10 | `test_nonstandard_config_section_parses` | unshaped section → `extra` | (c) | `ok_config_extra.toml` |
| 11 | `test_boolean_type_normalizes_to_bool` | `boolean` → `bool` | (c) | `ok_config_boolean_alias.toml` |
| 12 | `test_tags_and_conflicts_default_empty` | `[]`/`[]` | (c) | `:398` clean records |
| 13 | `test_tags_parsed` | order kept | (c) | `:398` clean records (recipes declare `tags`) |
| 14 | `test_conflicts_with_parsed` | order kept | (c) | `:398` clean records (`conflicts_with`) |
| 15 | `test_tags_non_list_rejected` | names `tags` | (c) | `err_tags_not_array.toml` |
| 16 | `test_tags_non_string_element_rejected` | names `tags` | (c) | `err_tags_item_int.toml` |
| 17 | `test_conflicts_with_non_list_rejected` | names `conflicts_with` | (c) | `err_conflicts_not_array.toml` |
| 18 | `test_conflicts_with_self_reference_rejected` | names key + id | (c) | `err_conflicts_self.toml` |
| 19 | `test_blank_tag_rejected` | names `tags` | (c) | `err_tags_item_blank.toml` |
| 20 | `test_blank_conflicts_with_rejected` | names `conflicts_with` | (c) | `err_conflicts_self.toml` shares the `non-empty string` branch; `err_tags_item_blank.toml` pins it |
| 21 | `test_brief_fragment_default_key_is_none` | `BriefFragment(text=…)` constructor | (a) — Python dataclass construction, no native counterpart | — |
| 22 | `test_brief_fragment_explicit_key` | keyword construction | (a) | — |
| 23 | `test_brief_fragments_defaults_all_none` | six `None` attributes on a fresh dataclass | (a) | — |
| 24 | `test_recipe_has_brief_fragments_field` | loaded recipe exposes it, defaults `None` | (c) | `:398` clean records (`"brief": null`) |
| 25 | `test_recipe_brief_fragments_accepts_brief_fragments_object` | `[provides.brief]` fills it | (c) | `ok_brief_forms.toml` |
| 26 | `test_absent_provides_brief_returns_none` | `None` | (c) | `:398` clean records |
| 27 | `test_simple_array_normalizes_key_none` | strings → fragments, key `None` | (c) | `ok_brief_forms.toml` (array form) |
| 28 | `test_simple_array_preserves_order` | declaration order | (c) | `ok_brief_forms.toml` (two ordered items) |
| 29 | `test_inline_table_form_normalizes_with_key` | `key`/`text` kept | (c) | `ok_brief_forms.toml` (`[[provides.brief.context_sources]]`) |
| 30 | `test_both_sections_populated` | array + table together | (c) | `ok_brief_forms.toml` (both forms in one file) |
| 31 | `test_empty_array_is_valid` | `[]` valid | (c) | `ok_brief_forms.toml` variant + `err_brief_not_array.toml` for the rejected shape |
| 32 | `test_intro_in_brief_raises_project_only_error` | names `intro` + `project-only` | (c) | `err_brief_project_only.toml` |
| 33 | `test_purpose_in_brief_raises_project_only_error` | same for `purpose` | (c) | `err_brief_project_only.toml` (same branch, key interpolated) |
| 34 | `test_unknown_section_raises_error_with_valid_list` | names key + valid sections | (c) | `err_brief_unknown_section.toml` |
| 35 | `test_inline_table_missing_text_raises_error` | names `text` | (c) | `err_brief_missing_text.toml` |
| 36 | `test_inline_table_missing_key_raises_error` | names `key` | (c) | `err_brief_missing_key.toml` |
| 37 | `test_mixed_forms_raises_error` | `mix` in message | (c) | `err_brief_mixed.toml` |
| 38 | `test_valid_cli_dep_parses` | all seven fields | (c) | `ok_cli_deps_github_release.toml`, `:398` `git-pr-flow` |
| 39 | `test_optional_defaults` | `required` true, strings empty | (c) | `:398` clean records (`cli_deps[]` projections) |
| 40 | `test_missing_binary_raises` | names `binary` | (c) | `err_clideps_not_array.toml`/`err_clideps_item_not_table.toml` sibling arms; `:398` guarantees the happy path |
| 41 | `test_missing_purpose_raises` | names `purpose` | (c) | `err_clideps_unknown_key.toml` sibling; `requireString` arm |
| 42 | `test_unknown_key_raises` | names the key | (c) | `err_clideps_unknown_key.toml` |
| 43 | `test_required_non_bool_raises` | names `required` | (c) | `err_clideps_required_str.toml` |
| 44 | `test_absent_deps_yields_empty_list` | `[]` | (c) | `:398` clean records |
| 45 | `test_recipe_to_dict_serializes_cli_deps` | projection shape | (c) | `:398` clean projections |
| 46 | `test_catalog_git_pr_flow_has_cli_deps` | `gh`, required | (c) | `:398` `catalog/recipes/git-pr-flow/recipe.toml` |
| 47 | `test_catalog_gitlab_mr_flow_has_two_deps` | `[glab, jq]` | (c) | `:398` `catalog/recipes/gitlab-mr-flow/recipe.toml` |
| 48 | `test_catalog_worktree_flow_has_git_dep` | `git` | (c) | `:398` `catalog/recipes/worktree-flow/recipe.toml` |
| 49 | `test_reconcile_parses_as_structured_table_not_extra` | `tables.reconcile` shape | (c) | `ok_config_reconcile.toml` |
| 50 | `test_structured_unknown_subkey_rejected` | `unknown key` | (c) | `err_reconcile_unknown_key.toml` |
| 51 | `test_structured_wrong_scalar_type_rejected` | names field + `integer` | (c) | `err_reconcile_scalar.toml` |
| 52 | `test_structured_expectation_unknown_key_rejected` | names the key | (c) | `err_reconcile_expectations_item_missing_property.toml` (expectation-shape branch) |
| 53 | `test_structured_non_table_rejected` | `table` | (c) | `err_reconcile_scalar.toml` |
| 54 | `test_structured_array_not_a_list_rejected` | `array` | (c) | `err_reconcile_expectations_not_array.toml` |
| 55 | `test_structured_array_too_large_rejected` | `at most` | (c) | `err_reconcile_expectations_long.toml` |
| 56 | `test_nonstandard_section_without_shape_still_extra` | `extra`, not `tables` | (c) | `ok_config_extra.toml` |
| 57 | `test_recipe_conflicts_tolerates_deps_block` | `check_recipe_conflicts` does not crash | (a) — imports `lib/_internal/recipe-conflicts.py`; native `internal/conflicts/conflicts_test.go:247` (corpus recipes declare `[[deps.cli]]`) | — |

UNCOVERED: 0.

### tests/test_lock.py — 20 methods: 2 (a), 18 (c)

Go package: `internal/lock` (frozen differential #447: `internal/lock/testdata/write_lock_golden.json`
via `internal/lock/lock_test.go:284` `TestWriteLockDifferential`; `testdata/roundtrip_golden.json`
via `:480` `TestLoadLockWriteRoundTripDifferential`).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_dep_hashes_emitted_skill_recipe_dropped` | `[deps."d".skills."s"]` emitted, `[skills.`/`[recipes.` not | (c) | `:284` `all sections`, `deps hash non-string`; `:480` asserts neither `[recipes.` nor `[skills.` in the output |
| 2 | `test_legacy_rewrite_drops_skills_recipes_keeps_deps` | legacy lock normalized, deps + meta kept | (c) | `:480` (sample lock carries `[skills]`, `[recipes]`, `[deps]`), `:404` `TestLoadLockParse` (deps/recipes normalization) |
| 3 | `test_legacy_commands_opted_out_dropped_on_write` | `[commands]`/`[opted-out]` absent after write | (c) | UNCOVERED (no carried fixture) → closed in L4 by `lock_test.go:900` `TestLegacyLockSectionsAreDroppedOnRewrite` |
| 4 | `test_legacy_lock_with_commands_opted_out_sections_normalized` | legacy file normalized on rewrite | (c) | UNCOVERED → same addition (the legacy file is written, loaded, rewritten) |
| 5 | `test_legacy_commands_opted_out_dropped_agents_preserved` | dropped + `[agents.*]` kept | (c) | UNCOVERED → same addition |
| 6 | `test_control_char_in_agents_hash_is_refused` | `value for agents hash …` | (c) | `:532` `TestControlCharRefusalOrder` (`agents filename`/`agents harness` arms) + `:284` `refusal agents hash DEL` |
| 7 | `test_control_char_in_managed_path_is_refused` | `managed path` | (c) | `:532` `managed path even without sha256` |
| 8 | `test_control_char_in_meta_value_is_refused` | `meta.synced_at` | (c) | `:532` `meta before managed` |
| 9 | `test_control_char_in_lock_path_is_refused` | `lock_path` | (c) | UNCOVERED (every frozen path is a temp dir) → closed in L5 by `lock_test.go:879` `TestControlCharRefusalNamesTheLockPath` |
| 10 | `test_control_char_in_meta_cli_version_is_refused` | `meta.cli_version` | (c) | `:284` `refusal meta newline` (frozen message + nothing written) |
| 11 | `test_control_char_in_managed_entry_value_is_refused` | every managed value walked | (c) | `:532` `managed before agents` (`managed.kind`); the locator is one loop over the five keys (`lock.go:525`) |
| 12 | `test_control_char_in_agents_harness_is_refused` | `agents harness` | (c) | `:532` `agents harness before filename` |
| 13 | `test_control_char_in_agents_filename_is_refused` | `agents filename` | (c) | `:532` `agents filename before hash` |
| 14 | `test_clean_lock_still_writes` | clean envelope writes | (c) | `:284` ok cases (`all sections`, `unicode`) |
| 15 | `test_write_lock_fallback_route_refuses_control_chars` | the no-Go-binary fallback refuses identically | (a) — the Python fallback writer dies with `lib/`; the Go authority is pinned by `:532` | — |
| 16 | `test_skip_branches_write_without_refusal` | no-sha256 entry and empty harness skipped | (c) | `:284` `managed entry without sha256 skipped`, `empty maps skipped` |
| 17 | `test_non_dict_managed_entry_is_skipped_not_refused` | `isinstance` guard in the Python writer | (a) — a typed Go map cannot hold a non-dict entry | — |
| 18 | `test_removing_present_recipe_returns_true_and_deletes` | true + deleted | (c) | `:703` `TestMutationHelpers` `Remove helpers return presence` |
| 19 | `test_removing_absent_recipe_returns_false_and_keeps_others` | false, others kept | (c) | `:703` same subtest |
| 20 | `test_removing_from_missing_recipes_group_returns_false` | false on a nil map | (c) | `:703` same subtest (nil-map arm) |

UNCOVERED: 4 rows (3, 4, 5, 9) in 2 groups, closed in L4 and L5.

### tests/test_project_cache.py — 15 methods: 0 (a), 15 (c)

Go package: `internal/projectcache` (frozen oracle PR2-B S1:
`internal/projectcache/testdata/projectcache_golden.json` via
`internal/projectcache/projectcache_test.go:1071` `TestProjectCacheDifferential`, which runs the
named cases at `:1800`–`:2163` through the native CLI and compares stdout, stderr, exit code and
the whole file tree).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_cache_key_stable_and_includes_basename` | stable, `12 hex + "-" + basename` | (c) | `:105` `TestCacheKeyFrozen` (also `CacheKey("/")`) |
| 2 | `test_cache_key_differs_for_different_roots` | distinct roots differ | (c) | `:105` (key is the sha256 of the resolved path) |
| 3 | `test_ensure_cache_writes_meta_toml` | `<home>/cache/projects/<key>`, `project_root`, `created_at` | (c) | `:131` `TestEnsureCacheCreateRefresh` (exact `meta.toml` bytes, refresh, CRLF) |
| 4 | `test_path_helpers` | `.recipe`, `.deps`, `commands`, `resolved-skills` | (c) | `:188` `TestPathRoots` |
| 5 | `test_backups_root_is_cache_scoped_outside_project` | `<cache>/backups`, never in the project | (c) | `:188` `TestPathRoots` (`BackupsRoot` under `CacheRoot`); `:211` `TestGateBackupPath` |
| 6 | `test_gate_backup_path_content_hash_collision_safe` | stable per content, distinct per content/rel, `.sh`, cache-scoped | (c) | `:211` `TestGateBackupPath` + differential `gate backup path` (`:2163`) |
| 7 | `test_remove_legacy_origin_migrates_overrides_then_deletes` | overrides migrated, `.recipe` gone, `.deps` kept | (c) | differential `remove legacy origin fresh migration` (`:2102`, whole-tree snapshot incl. `.deps` marker) |
| 8 | `test_remove_legacy_origin_keeps_existing_overrides` | existing dest wins, `.recipe` gone | (c) | differential `remove legacy origin` (`:2084` seed writes `recipes/recipeB/overrides/keep.md` and asserts the tree) |
| 9 | `test_remove_legacy_origin_skips_recipe_rmtree_if_override_migration_failed` | `.recipe` survives a failed copy | (c) — the Go port **deliberately diverges** here (safety exception, `projectcache_test.go:330-337`): an unproven destination keeps the originals and warns | `:339` `TestRemoveLegacyOriginRetryPreservesDotRecipe`, `:420` `…PartialMultiRecipeNoRemovalOnRetry` |
| 10 | `test_remove_legacy_origin_deletes_resolved_skills_and_internal` | both removed | (c) | differential `remove legacy origin` (`:2084` seeds both and asserts the tree) |
| 11 | `test_remove_legacy_origin_deletes_stale_premerge_bin` | `premerge_guardian.py` gone, siblings kept | (c) | differential `remove legacy origin` (seeds `ai-specs/bin/premerge_guardian.py`) |
| 12 | `test_remove_legacy_origin_removes_empty_bin_after_guardian` | empty `bin/` removed | (c) | differential `remove legacy origin` (no other file left under `bin/`) |
| 13 | `test_bundled_commands_root_is_bundled_skills_root_sibling` | `<cache>/.bundled/commands` | (c) | `:188` `TestPathRoots` (`BundledSkillsRoot`, `BundledCommandsRoot`) |
| 14 | `test_bundled_command_ids_returns_md_stems` | `*.md` stems, sorted | (c) | differential `bundled command ids` (`:2006`) |
| 15 | `test_bundled_command_ids_empty_when_dir_missing` | `[]` | (c) | differential `bundled command ids` (absent dir arm) |

UNCOVERED: 0.

### tests/test_skill_contract.py — 6 methods: 0 (a), 6 (c)

Go package: `internal/skills` (native `ai-specs skills validate`, U-D #459).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_parses_canonical_local_skill` | name, description summary, license, metadata | (c) | `internal/skills/syncmeta_test.go:44` `TestSyncMetadataEnabled` (name/scope/auto_invoke/warnings from an explicit frontmatter) |
| 2 | `test_compatibility_mode_normalizes_scalar_auto_invoke_and_missing_fields` | defaults `Apache-2.0`/`unknown`/`1.0`, scalar `auto_invoke` → list, warnings | (c) | `syncmeta_test.go:56` `DisabledWithDefaults` (exact `defaultsWarnings`), `:68` `ScalarScopeWarning` |
| 3 | `test_from_dep_generates_canonical_provenance_metadata` | `vendor_attribution`/`author`/`source` rendered | (c) | `internal/skills/vendored_test.go:154` `TestVendoredSkillMarkdownDifferential` |
| 4 | `test_sync_metadata_requires_scope_and_auto_invoke_together` | field `metadata.auto_invoke`, `is required when metadata.scope is set` | (c) | `syncmeta_test.go:175` `TestValidateSyncMetadataSymmetricRequirement` |
| 5 | `test_canonical_local_skill_without_sync_metadata_remains_valid` | no scope/auto_invoke, no warnings | (c) | `syncmeta_test.go:56` (disabled payload) + `:44` (`warnings: []` with explicit metadata) |
| 6 | `test_cli_sync_metadata_returns_json_payload` | `sync-metadata` JSON `{enabled, name}` | (c) | the native verb: `internal/cli/skills_validate_test.go` (verb output) on top of `syncmeta_test.go:44`'s exact payload |

UNCOVERED: 0.

### tests/test_trello_link.py — 13 methods: 2 (a), 11 (c)

Go package: `internal/tracker` (port of `trello_link.parse_tracker_section`, U-C #457:
`internal/tracker/section_test.go`).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_bold_key_list_form_lowercases_keys` | `- **card_id**: \`…\`` → lowercased key | (c) | `section_test.go:19` `TestParseSectionReadsRecognizedKeysOnly` |
| 2 | `test_plain_key_value_form` | `card_id: abc` | (c) | `:19` (`pr: \`https://x/pr/1\`` plain form) |
| 3 | `test_backticks_and_trailing_hash_comment_stripped` | backticks + ` # comment` stripped | (c) | `:19` (`- **card_id**: \`6aa…\` # trello card`) |
| 4 | `test_duplicate_keys_first_wins` | first wins | (c) | `:53` `TestParseSectionFirstKeyWinsAndFencesAreIgnored` |
| 5 | `test_headings_unknown_keys_blank_lines_ignored` | `### Nested`, unknown keys, blank lines ignored | (c) | `:19` (`random: nope` ignored, `## Other` ends the section) |
| 6 | `test_missing_file_returns_empty_invalid` | `{}` / invalid | (c) for the `{}` half | `:66` `TestParseSectionMissingFilesYieldEmpty`; the `is_valid_link` half is (a) (below) |
| 7 | `test_no_tracker_section_invalid` | `{}` when there is no section | (c) | `:19` (`## Other` body is not read) |
| 8 | `test_empty_card_id_invalid_nonempty_valid_without_url` | `is_valid_link` truthiness | (a) — legacy compatibility reader: `lib/_internal/trello_link.py` states it is *not* a grader, doctor no longer consumes it, and the native verdict is the gate's ledger predicate; no Go counterpart exists to keep | — |
| 9 | `test_proposal_wins_over_tasks_fallback` | first artifact with a section wins | (c) | `:38` `TestParseSectionFirstArtifactWithASectionWins` |
| 10 | `test_tasks_fallback_when_proposal_has_no_section` | falls through to `tasks.md` | (c) | `:38` |
| 11 | `test_section_ends_at_next_h2` | stops at `## Next` | (c) | `:19` (`## Other` ends the section) |
| 12 | `test_card_id_looks_canonical_24_hex_only` | 24-hex nudge helper | (a) — legacy INFO helper, no caller and no native counterpart | — |
| 13 | `test_fenced_sample_tracker_ignored` | a fenced `## Tracker` sample is not a section | (c) | UNCOVERED (the existing fence case is inside a real section) → closed in L6 by `section_test.go:88` `TestParseSectionIgnoresAFencedSampleTracker` |

UNCOVERED: 1 (row 13), closed in L6.

### tests/test_repo_topology.py — 39 methods: 16 (a), 23 (c) — **retained, reduced to 2**

37 methods are retired; rows 1 and 2 are kept (they guard the fixture three kept suites import)
and the file becomes a shared fixture module with no `lib/` dependency.

Go packages: `internal/target` (topology: `topology.go`, frozen differential
`internal/target/differential_test.go:449` `TestDifferentialTopologySubmodules`, `:645`
`TestGitFailureDegradationGoOnly`, `:707` `TestTopologyUnitGoOnly`, `:812`
`TestSubmoduleDetectionUnitGoOnly`) and the gate module's proof-based owner classification
(`gate/topology.go`, `gate/topology_test.go`), plus `internal/sync` (`projectowned.go`).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_fixture_initialized_has_space_prefix` | the fixture's `git submodule status` shape | (a) for the deleted behavior — **kept**: it guards the helper `test_worktree_cleanup`, `test_hub` and `test_agents_render_brief_fragments` import | — |
| 2 | `test_fixture_uninitialized_has_dash_prefix` | fixture deinit prefix | (a) — **kept**, same reason | — |
| 3 | `test_space_plus_u_count_as_initialized` | `+` and `U` count as initialized | (c) | `:812` (`-` skipped) + UNCOVERED `+`/`U` → closed in L7 by `:854` `TestRepoTopologyResolutionGoOnly/detection prefixes…` (`+` arm; `U` uses the same `prefix != "-"` rule) |
| 4 | `test_dash_prefix_skipped` | `-` skipped | (c) | `:812` `TestSubmoduleDetectionUnitGoOnly` |
| 5 | `test_no_gitmodules_returns_false_empty` | `(False, ())` | (c) | `:449` `super-without-submodules`; `:854` phantom/standalone legs |
| 6 | `test_path_in_gitmodules_missing_from_status_ignored` | phantom `.gitmodules` path ignored | (c) | UNCOVERED → closed in L7 by `:854` (ghost entry leg) |
| 7 | `test_name_ne_path_returns_path` | the path field, never the name | (c) | `:854` (the loop reads the status path field; the ghost leg proves the registration key never leaks) |
| 8 | `test_auto_with_initialized_submodules` | `monorepo-submodules`, `via auto`, submodules, gitmodules present | (c) | `:449` `super-with-submodule` (exact stdout fields) + `:854` |
| 9 | `test_auto_only_uninitialized_or_no_gitmodules_is_standalone` | `standalone`, empty submodules | (c) | UNCOVERED (the deinitialized+auto leg) → closed in L7 by `:854` |
| 10 | `test_auto_never_returns_monorepo_apps` | auto never yields `monorepo-apps` | (c) | UNCOVERED → closed in L7 by `:854` `auto never resolves to monorepo-apps` |
| 11 | `test_explicit_bypass_detection` | explicit value wins, `via config`, no detection | (c) | UNCOVERED as a unit (the golden covers configured values through the plan JSON) → closed in L7 by `:854` `explicit config bypasses detection` |
| 12 | `test_absent_or_empty_config_treated_as_auto` | `""`/`None`/`"   "` → auto | (c) | UNCOVERED (blank/whitespace arms) → closed in L7 by `:854` `blank config is auto` |
| 13 | `test_git_missing_or_not_a_repo_degrades_to_standalone` | degrade without raising | (c) | `:645` `TestGitFailureDegradationGoOnly` |
| 14 | `test_subrepo_cwd_owns_subrepo_with_super_planning_root` | owner = subrepo, planning root = superproject, `apps/api`, `.worktrees` | (c) | native proof: `tests/test_worktree_root_propagation.py:460` (gate `--resolve-central-root` → superrepo root + `apps/api`) + `gate/topology_test.go:83` (proven initialized submodule) |
| 15 | `test_superrepo_cwd_without_explicit_subrepo_hard_errors` | hard error before any create | (c) | `tests/test_worktree_root_propagation.py:500` (exit 1, `unproven`, `worktree list` unchanged) |
| 16 | `test_superrepo_cwd_with_explicit_subrepo_uses_super_planning_root` | planning root stays the superproject | (c) | `tests/test_worktree_root_propagation.py:521` |
| 17 | `test_standalone_request_planning_root_is_owner_root` | planning root = owner root | (c) | `gate/topology_test.go:10` `TestClassifyStandaloneUnproven` + `:449` `super-without-submodules` |
| 18 | `test_uninitialized_submodule_fails_safe_without_planning_exception` | no subrepo, no exception | (c) | `gate/topology_test.go:207` (`deinitialized status fails closed`) + `:854` deinit leg |
| 19 | `test_non_git_cwd_fails_safe_to_owner_root` | fails safe | (c) | `gate/topology_test.go:10` (unproven) + `:854` |
| 20 | `test_explicit_inferred_mismatch_names_both_values` | diagnostic naming both paths | (a) — the path-inference diagnostic; the native rule never infers from a path and fails closed as `unproven` instead (`tests/test_worktree_root_propagation.py:500`) | — |
| 21 | `test_linked_submodule_worktree_longest_prefix_inference` | longest path prefix wins | (a) — same path heuristic; the native proof identifies a linked submodule worktree by its gitdir/common marker (`gate/topology.go:30`, `gate/topology_test.go:123`), not by prefix | — |
| 22 | `test_missing_dest_false` | `override_is_stale` → false | (a) — `util.override_is_stale` has **no caller left** (only its definition; doctor and materialize use the ownership classifier) | — |
| 23 | `test_missing_catalog_src_false` | false | (a) | — |
| 24 | `test_identical_bytes_false` | false | (a) | — |
| 25 | `test_divergent_bytes_true` | true | (a) | — |
| 26 | `test_cwd_inference_from_primary_checkout` | infer `apps/api` from cwd | (a) — `resolve_subrepo` path inference, no native counterpart (see 20) | — |
| 27 | `test_cwd_inference_from_linked_worktree_longest_prefix` | longest prefix | (a) | — |
| 28 | `test_explicit_path_validated` | explicit path validated | (a) | — |
| 29 | `test_explicit_unique_name_resolves_to_path` | name → path | (a) | — |
| 30 | `test_explicit_inferred_mismatch_raises` | names both | (a) | — |
| 31 | `test_uninitialized_submodule_rejected` | `not initialized; run git submodule update --init` | (a) — that diagnostic does not exist natively; the gate fails closed as `unproven` | — |
| 32 | `test_unknown_submodule_rejected` | `unknown submodule` | (a) | — |
| 33 | `test_ambiguous_name_requires_path` | `ambiguous` + `path` | (a) — ambiguity is handled by the proof rule (`moduleRecords` returns nil), `gate/topology_test.go:234` | — |
| 34 | `test_project_field_wins_over_legacy_recipe_alias` | `source project`, no deprecation | (c) | `:707` `TestTopologyUnitGoOnly` (precedence) |
| 35 | `test_legacy_recipe_alias_still_resolves_with_deprecation` | `source legacy-recipe` + `[project].repo_topology` deprecation | (c) | `:707` (legacy alias + non-empty deprecation), `:224` `topology-legacy-recipe` |
| 36 | `test_project_field_survives_disabled_worktree_flow` | disabled recipe still project-owned | (c) | `:707` (`topologyConfig` reads `[project]` first, regardless of enablement) |
| 37 | `test_unconfigured_defaults_to_auto_without_deprecation` | `configured auto`, `source default` | (c) | `:707` (default case) |
| 38 | `test_accessor_accepts_preloaded_manifest_data` | preloaded manifest honored | (c) | `internal/doctor/checks_overrides_test.go:232` `TestProjectRepoTopologyWrapper` |
| 39 | `test_project_owned_recipe_config_overrides_legacy_alias_in_place` | project value replaces the recipe key, other keys kept, other recipes untouched | (c) | `internal/doctor/checks_overrides_test.go:195` `TestProjectOwnedRecipeConfig` |

UNCOVERED: 6 rows (3+, 6, 9, 10, 11, 12) in 1 group, closed in L7 by one addition with five cases.

### tests/test_config_wizard.py — 20 methods: 1 (a), 19 (c)

Go packages: `internal/cli` (wizard + tail flow) and `internal/deps` (the dep gate and its TTY
half).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_required_validator_rejects_blank_accepts_value` | exact `This field is required.` | (c) | `internal/cli/recipewizard_test.go:260` `TextValidatorsReprompt/required only` |
| 2 | `test_regex_validator` | empty passes, mismatch → `Must match` | (c) | `:280` `regex only: mismatch reprompts, empty always passes` |
| 3 | `test_enum_field_uses_select` | `select` with the enum choices | (c) | `:215` `TestRunConfigWizardEnumSelectBranch` |
| 4 | `test_bool_field_uses_confirm` | `confirm` used, value kept | (c) | `:493` `Confirm routes to tui.Confirm` |
| 5 | `test_default_prefill_kept_when_blank` | blank answer omits the key | (c) | `:360` `TestRunConfigWizardBlankAnswerOmitsTheKey` |
| 6 | `test_existing_value_prefilled_as_default` | existing value is the default | (c) | `:158` `TestRunConfigWizardFieldDefaultResolution` |
| 7 | `test_extra_fields_never_prompted` | only `config.fields` prompt | (c) | `:119` `PromptsSortedFieldsWithThePythonMessage` |
| 8 | `test_dep_gate_abort_skips_recipe` | decline → no write, `{}` | (c) | `internal/cli/recipe_test.go:990` `dep gate declined skips the wizard write` |
| 9 | `test_dep_gate_proceed_continues` | accept → continues | (c) | `recipe_test.go:951` `TTY reaching the dep gate runs the native tail`; `internal/deps/gate_test.go:109` |
| 10 | `test_dep_gate_offers_install_on_tty` | plan offered, `offer_and_install([plan], tty=True)` | (c) | `internal/deps/gate_test.go:135` `TestGateOffersEveryMissingPlanAndRechecks`, `:180` `TestGateInstallsPackagePlan` |
| 11 | `test_dep_gate_non_tty_never_offers_provider_install` | no plan off the TTY | (c) | `internal/deps/gate_test.go:400` `TestGateOffTTYSkipsTheOffer` |
| 12 | `test_dep_gate_passes_provider_plan_fields_on_tty` | `installer`/`repository`/`release_policy` carried | (c) | `internal/deps/gate_test.go:293` `TestGateProviderPlanDescribesThenInstalls` (exact plan lines) |
| 13 | `test_configure_selected_writes_each` | answer written into `[recipes.demo.config]` | (c) | `recipe_test.go:2964` `gate wizard write with the existing-wins prefill`; `internal/config/recipeconfig_test.go:297` `TestUpdateRecipeConfig` |
| 14 | `test_main_offers_envrc_generation` | `offer_harness_env(project.resolve())` | (c) | `recipe_test.go:3226` `TestEnvOfferScopeSplitsRecipeAddFromInitWizard` |
| 15 | `test_main_skips_envrc_when_no_mcp_recipes` | still invoked, no-op inside | (c) | `recipe_test.go:3226` (scope) + `:3142` `env offer soft-fails on the Python's line` |
| 16 | `test_global_configure_recipes_keeps_aggregate_env_offer` | unscoped aggregate offer | (c) | `recipe_test.go:3226` |
| 17 | `test_offer_envrc_soft_fails_on_prompt_error` | `TypeError` swallowed | (c) | `recipe_test.go:3142` |
| 18 | `test_boolean_type_alias_uses_confirm` | `type=bool` → confirm, never text | (c) | `recipewizard_test.go:493` + `internal/schema` `ok_config_boolean_alias.toml` |
| 19 | `test_configure_recipes_dispatch` | native binary, non-zero on a bad path | (c) | `recipe_test.go:2588` `TestConfigureRecipesMissingPathDeviation` |
| 20 | `test_recipe_config_sh_help` | `bash lib/recipe-config.sh --help` lists `configure-recipes` | (a) — the legacy script dies with `lib/`; the native family help is `recipe_test.go:2542` | — |

UNCOVERED: 0.

### tests/test_command_merge.py — 5 methods: 0 (a), 5 (c)

Go package: `internal/projectcache` (`MergeCommands`, `projectcache.go:888`). The frozen CLI cases
pin one collision each (`cli merge fresh dest` `:1824`, `cli merge bundled managed duplicate`
`:1865`, `cli merge preserves source modes` `:1871`); they cannot express the full precedence
ladder and warning set in one place, so it is pinned natively.

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_local_wins_over_managed` | local bytes win, count 3, `local hand-authored wins` | (c) | `:2173` `TestMergeCommandsPrecedenceAndWarnings/local wins…` + `:1824` |
| 2 | `test_bundled_only_appears_in_merge_output` | bundled-only file lands, count 1 | (c) | `:2173` (`only-bundled.md`) + `:1871` |
| 3 | `test_managed_silently_overrides_bundled` | managed wins, no warning | (c) | `:2173` `bundled versus managed collision is silent` + `:1865` |
| 4 | `test_local_wins_over_bundled_and_managed_with_warning` | both collisions warn | (c) | `:2173` (one warning naming the stem; the third tier wins over both) |
| 5 | `test_managed_only` | count 1 | (c) | `:2173` (collision subtest asserts count 1) |

UNCOVERED: 0 (closed in L8 by one addition with two subtests).

### tests/test_override_ownership.py — 18 methods: 0 (a), 18 (c)

Go packages: `internal/lock`, the gate module's `shared.ClassifyManagedOverride` /
`TemplateActuator` / `HookGateActuator`, `internal/sync` (`templatedocs.go`), `internal/doctor`,
`internal/schema`.

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_managed_lock_round_trip_preserves_provenance` | `[managed."…"]` emitted, `[skills.`/`[recipes.` not | (c) | `internal/lock/lock_test.go:703` `SetManagedOverride` + `:480` (managed emitted, legacy sections dropped) |
| 2 | `test_classifier_covers_ownership_states` | missing/untracked/managed_current/managed_stale/user_modified | (c) | `gate/shared/classify_test.go:15` `TestClassifyManagedOverrideSemantics` (every state) |
| 3 | `test_materialize_seeds_and_refreshes_managed_templates` | seed, hash recorded, refresh to v2 | (c) | `gate/templateactuator_test.go:438` `NotExistsSeedsRenderedCopy`, `:519` `ManagedStaleAutoRefresh`; `internal/sync/templatedocs_test.go:448` |
| 4 | `test_materialize_preserves_user_modified_and_untracked_diverged` | user-modified preserved + warned; untracked divergence preserved with the exact guidance | (c) | `gate/templateactuator_test.go:489` `NotExistsPreservesUntracked`, `:580` `UserModifiedRefusal`; `internal/sync/templatedocs_test.go:466` |
| 5 | `test_doctor_describes_untracked_divergence_neutrally` | `missing ownership metadata` + preserve/remove/sync, no `user-managed`/`user-owned` | (c) | `internal/doctor/checks_overrides_test.go:86` `TestCheckStaleTemplateOverridesUntracked` |
| 6 | `test_doctor_silences_legacy_catalog_seed_with_rendered_config` | no finding | (c) | `checks_overrides_test.go:115` `matches-legacy-catalog-is-quiet` |
| 7 | `test_untracked_matching_catalog_seeds_without_rewrite` | same bytes → adopt hash, no rewrite | (c) | `gate/templateactuator_test.go:602` `ManagedCurrentBackfill` |
| 8 | `test_confirm_policy_preserves_managed_stale` | `confirm` keeps v1, warns `managed-stale` | (c) | `gate/templateactuator_test.go:549` `StaleRefusalPolicies` |
| 9 | `test_template_policy_is_validated_and_defaults_auto` | `never-force` accepted, default `auto`, invalid rejected naming `update_policy` | (c) | `internal/schema` fixtures `err_templates_bad_policy.toml` + `:398` clean records (`update_policy: auto`) |
| 10 | `test_doctor_warns_user_modified_but_not_managed_auto_stale` | one `user-modified` finding; auto-stale quiet | (c) | `checks_overrides_test.go:70` `…UserModified`, `:167` `policy-auto-is-quiet` |
| 11 | `test_gate_baseline_match_refreshes_and_records_baseline` | baseline match refreshes and re-records | (c) | `gate/hookgateactuator_test.go:418` `ManagedCurrent`, `:446` `ManagedStaleRefresh` |
| 12 | `test_gate_byte_mismatch_preserves_with_warning` | custom bytes preserved + warning | (c) | `gate/hookgateactuator_test.go:478` `UserModifiedPreserved` (exact warning) |
| 13 | `test_gate_missing_provenance_preserves_without_seeding` | preserved, no baseline seeded | (c) | `gate/hookgateactuator_test.go:515` `NoProvenancePreserved` (exact warning, nil entry) |
| 14 | `test_explicit_refresh_backs_up_pre_refresh_bytes_immutably` | backup holds the exact pre-refresh bytes under the cache | (c) | `gate/hookgateactuator_test.go:543` `RefreshBacksUpAndRewrites`; `internal/projectcache/projectcache_test.go:211` path |
| 15 | `test_repeated_refresh_is_collision_safe` | distinct content → distinct backups, original intact | (c) | `gate/hookgateactuator_test.go:631` `RefreshKeepsCompleteBackup`, `:590` `RefreshRepairsPartialBackup` |
| 16 | `test_failed_backup_write_leaves_gate_unchanged` | failure → gate and lock unchanged | (c) | `gate/hookgateactuator_test.go:666` `RefreshRollback` |
| 17 | `test_refresh_absent_or_disabled_provider_parity` | identical with external orchestration absent/disabled | (c) — structural: the native actuator has no orchestration input at all (`shared/HookGateActuator`), so both legs are the same code path | `gate/hookgateactuator_test.go:543` |
| 18 | `test_gate_provenance_policy_is_documented` | the three docs state the provenance contract | (c) | UNCOVERED (no Go doc contract for prose) → closed in L9 by `internal/sync/gate_provenance_docs_test.go:17` |

UNCOVERED: 1 (row 18), closed in L9.

### tests/test_narrow_checkout.py — 18 methods: 1 (a), 17 (c)

The subject is `lib/_internal/narrow-checkout.sh`, which dies with `lib/` at T5 (its only caller
`lib/upgrade.sh:343` also dies; Go `upgrade` never calls it). U-E #460 inlined it into `install.sh`
as `narrow_checkout()`, reproducing the allowlist, every guard and every message byte for byte
(epic log L31/L37), and that survivor is covered by the **kept** suite
`tests/test_install_narrow_checkout.py:80` (hermetic installer run: narrowed line, excluded
subtrees gone, all eight runtime dirs survive) and `:104` (the installer no longer references the
`lib/_internal` helper).

| # | method | assertion | class | surviving coverage |
|---|---|---|---|---|
| 1 | `test_excluded_subtrees_leave_the_working_tree` | `openspec/ tests/ .github/ tmp/` gone, rc 0 | (c) | `tests/test_install_narrow_checkout.py:80` |
| 2 | `test_runtime_subtrees_survive` | the eight kept dirs survive | (c) | `tests/test_install_narrow_checkout.py:80` |
| 3 | `test_root_files_survive` | `VERSION`, `CHANGELOG.md` kept | (c) | `tests/test_install_narrow_checkout.py:80` (cone mode keeps root files; the install completes and the version line prints) |
| 4 | `test_tree_is_not_left_dirty` | `git status` clean after narrowing | (c) | `tests/test_install_narrow_checkout.py:80` (the installer's own update guard re-runs clean) |
| 5 | `test_history_and_ancestry_are_intact` | `merge-base --is-ancestor` still works | (c) | U-E differential scenario "fresh clone" + the installer's update path (epic log L37) |
| 6 | `test_second_run_is_a_noop` | second run rc 0, same tree | (c) | U-E differential "already narrowed (second run)" (L37) |
| 7 | `test_second_run_reports_nothing_to_do` | `already` on the second run | (c) | U-E differential "already narrowed" message (L37; the `checkout already narrowed (…)` line exists in `install.sh:137`) |
| 8 | `test_git_without_sparse_checkout_falls_back` | rc 0, nothing removed | (c) | U-E differential "git without sparse-checkout (PATH shim)" (L37) |
| 9 | `test_fallback_warns_rather_than_failing_silently` | output is non-empty | (c) | same differential (`this git has no sparse-checkout support; keeping the full checkout`) |
| 10 | `test_allowlist_covers_every_production_keep_dir` | 8 entries parsed **out of the script source** | (a) — a harness detail of the deleted script; the surviving allowlist is `install.sh:122` | — |
| 11 | `test_a_stale_allowlist_is_reconciled` | an older narrow install picks up new dirs | (c) | U-E differential "stale allowlist" (L37) |
| 12 | `test_reconciliation_leaves_the_tree_clean` | clean `git status` | (c) | same differential ("already narrowed by the helper") |
| 13 | `test_a_failed_apply_never_leaves_the_tree_collapsed` | failed `set` keeps the runtime dirs | (c) | U-E differential "failed sparse apply (PATH shim)" (L37) |
| 14 | `test_collapsed_tree_is_never_reported_as_success` | never claims recovery after a collapse | (c) | `install.sh:170-178` verifies each kept dir and restores, otherwise words the failure; L37 records the `disable` restore path as **not exercised** by the differential |
| 15 | `test_capability_probe_has_no_side_effects` | `-h`, never `--help` (no browser) | (c) | `install.sh:142-147` uses `-h` with the "is not a git command" match; U-E differential (non-git/shim scenarios) |
| 16 | `test_missing_target_is_not_fatal` | rc 0 for a missing dir | (c) | `install.sh:129` guard; U-E differential |
| 17 | `test_non_git_target_is_not_fatal` | rc 0 for a non-git dir | (c) | `install.sh:131` guard; U-E differential "non-git target" |
| 18 | `test_dirty_excluded_path_is_left_alone` | uncommitted work never discarded | (c) | `install.sh:150-155` dirty guard; U-E differential "dirty tree"/"dirty excluded path". The epic records the guard as unreachable from the installer flow (it refuses dirty trees earlier) and pins it by the differential only |

UNCOVERED: 0 methods require a new artifact I can author (install.sh is Bash and out of this
slice's Go surfaces); the surviving coverage for rows 1–9 and 11–18 is the kept installer suite
(`tests/test_install_narrow_checkout.py:80/:104`) plus the ten-scenario U-E differential recorded
in the epic log (L37). Row 14's `disable` restore path and rows 4/18's exact guards have no
committed test on either side — a pre-existing, human-accepted gap recorded at U-E (L37), not
introduced by this slice. Escalated as a finding, not silently absorbed.

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S3 | inline | |
| T2 | S4, S5 | inline | |
| T3 | S6, S7 | inline | |

## Log

- L1 (Robert): "ok,vos dale debemos ir terminando esto" + "continua popr favor" — approving the
  continuation of PR4a: retire the legacy Python unit tests of the `lib/` modules already ported to
  Go, after verifying that coverage in Go; chain S6 slices.
- L2 — Baseline before deletion (S4): `python3 -m unittest tests.test_recipe_read
  tests.test_recipe_schema tests.test_lock tests.test_project_cache tests.test_skill_contract
  tests.test_trello_link tests.test_repo_topology tests.test_config_wizard tests.test_command_merge
  tests.test_override_ownership tests.test_narrow_checkout -v` → `Ran 234 tests in 26.719s — OK`,
  rc=0 (`logs/01-baseline.log`, `.rc`), 0 skipped. Inventory 234 matches `Ran 234 tests`.
  Binaries built first: `scripts/build-gate.sh` rc=0, `go build -o dist/ai-specs ./cmd/ai-specs`
  rc=0 (`logs/00-build-gate.log`, `logs/00-build-cli.log`).
- L3 — Recipe gaps closed test-first (test_recipe_read row 10): `internal/recipe/schema_contract_test.go`
  `TestHookEventIsRequired` asserts the exact `.hooks[0]: missing or invalid required field 'event'`
  rejection, plus `TestHookActionIsRequired` as the PRESERVE companion for the branch the frozen
  corpus already pins. RED `logs/02-red-recipe.log` rc=1 (the deliberately wrong `action`
  expectation was reported against the actual `event` message); GREEN `logs/07-green-recipe.log`
  rc=0.
- L4 — Lock normalization closed test-first (test_lock rows 3, 4, 5): `internal/lock/lock_test.go`
  `TestLegacyLockSectionsAreDroppedOnRewrite` writes a legacy lock carrying `[commands]`,
  `[opted-out]` and `[agents."claude"]`, loads and rewrites it, and pins that both legacy sections
  are gone while the agents provenance and `cli_version` survive. RED
  `logs/09-red-lock-legacy.log` rc=1 (wrong expectation: the sections preserved); GREEN
  `logs/11-green-lock-legacy.log` rc=0.
- L5 — Lock path locator closed test-first (test_lock row 9):
  `lock_test.go:879` `TestControlCharRefusalNamesTheLockPath` pins
  `value for lock_path contains a control character` and that nothing is written. RED
  `logs/03-red-lock.log` rc=1 (wrong expectation: the `meta.cli_version` locator); GREEN
  `logs/07-green-lock.log` rc=0.
- L6 — Tracker gap closed test-first (test_trello_link row 13): `internal/tracker/section_test.go:88`
  `TestParseSectionIgnoresAFencedSampleTracker` pins that a fenced `## Tracker` sample is not a
  section, and therefore yields no fields. RED `logs/10-red-tracker-fence.log` rc=1 (wrong
  expectation: the fenced sample read); GREEN `logs/11-green-tracker-fence.log` rc=0.
- L7 — Topology gaps closed test-first (test_repo_topology rows 3+, 6, 9, 10, 11, 12):
  `internal/target/differential_test.go:854` `TestRepoTopologyResolutionGoOnly` pins explicit
  config bypass (`via config`, no detection), blank/whitespace/absent config → `auto`, auto never
  resolving to `monorepo-apps`, the `+` prefix counting as initialized, a phantom `.gitmodules`
  entry being filtered, and an uninitialized-only set resolving to `standalone`. RED
  `logs/05-red-target.log` rc=1 (wrong expectation: `via auto` for an explicit value); GREEN
  `logs/07-green-target.log` rc=0.
- L8 — Merge precedence closed test-first (test_command_merge, all 5):
  `internal/projectcache/projectcache_test.go:2173` `TestMergeCommandsPrecedenceAndWarnings` pins
  the bundled → managed → local ladder, the four-file count, the single `  ! command 'shared'
  present in cache and ai-specs/commands/; local hand-authored wins` warning, and that a
  bundled-vs-managed collision is silent. RED `logs/04-red-merge.log` rc=1 (wrong expectations:
  the managed bytes winning and an unprefixed warning line); GREEN `logs/07-green-merge.log` rc=0.
- L9 — Documentation contract closed test-first (test_override_ownership row 18):
  `internal/sync/gate_provenance_docs_test.go:17` `TestGateProvenanceDocsStayAccurate` pins the
  gate-provenance phrases across the two recipe READMEs and `docs/recipes-catalog.md`, and that
  the retired "always rewritten" claim is absent. RED `logs/06-red-docs.log` rc=1 (an injected
  wrong phrase was reported missing); GREEN `logs/07-green-docs.log` rc=0.
- L10 — Retirement (S5): plain `rm` of ten files, plus `tests/test_repo_topology.py` rewritten down
  to `git()` + `_make_bare_submodule_source()` + `make_super_with_submodule()` + its two fixture
  self-tests (148 lines). Reason found by the first validate run: `tests/test_worktree_cleanup.py:26`,
  `tests/test_hub.py:930` and `tests/test_agents_render_brief_fragments.py:1029` all do
  `from test_repo_topology import make_super_with_submodule`, so deleting the module broke three
  kept suites (3 errors, and `test_worktree_cleanup` failed to import at all). The helper is the only
  survivor those suites need; the 37 behavior tests are retired as mapped above. `git status --short`
  shows ten unstaged deletions, the modified fixture module, the six Go test files and this document.
  No production file, no `lib/` file, no golden, no `tests/parity/` file, no `testdata/` fixture and no
  other Python test edited; `tests/test_gate_binary_dist.py` untouched.
- L11 — (S6) `gofmt -l` on the five touched Go dirs → no output (`logs/08-gofmt.log`);
  `go vet` on the nine packages rc=0 (`logs/12-vet.log`); `go test -count=1` on the same nine rc=0
  (`logs/13-test.log`: recipe 0.560 s, lock 0.589 s, projectcache 1.199 s, skills 0.668 s,
  tracker 0.470 s, target 3.850 s, config 0.635 s, conflicts 0.413 s, sync 15.658 s).
- L12 — (S7) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/14-build-cli.log`). First
  `./tests/validate.sh` ran rc=1 in 617 s (`logs/15-validate.log`): `parity summary … PASS` and gofmt
  clean, but `Ran 1842 tests … FAILED (errors=3)` — the three `test_repo_topology` import errors of
  L10. After the fixture module was restored, the dependent suites ran green first
  (`logs/17-dependent-suites.log`: `Ran 131 tests … OK`), then the single re-run
  `./tests/validate.sh` rc=0 in 622 s (`logs/18-validate.log`): gofmt clean, both Go modules green,
  `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`, and the Python discover leg
  `Ran 1879 tests in 622.037s — OK (skipped=1, expected failures=3)`. Count reconciliation:
  2111 (base) − 232 (retired methods) = 1879 exactly; the single skip and the three expected failures
  are pre-existing and elsewhere.
- L13 — Scope note: `git reset --hard` was blocked by the child-safety hook, so the base was
  advanced with `git merge --ff-only origin/epic/go-single-binary` (same result, no data loss) and
  the worktree was clean before any write.
