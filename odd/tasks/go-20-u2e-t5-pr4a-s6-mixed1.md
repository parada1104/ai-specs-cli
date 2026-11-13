# GO-20 U2e T5 PR4a S6-3 — cut `lib/` out of ten mixed Python suites (batch A)

Base: epic/go-single-binary b639b70. Branch: change/go-20-u2e-t5-pr4a-s6-mixed1.
Scope: `tests/test_gate_binary_dist.py`, `test_orca_aware_delegation_skill.py`,
`test_worktree_gate_dist_config.py`, `test_worktree_gate_harness_phase4.py`,
`test_ledger_mode_config.py`, `test_runtime_brief_baseline.py`,
`test_brief_render_policy.py`, `test_envrc_scaffold.py`, `test_upgrade_mode_dirt.py`,
`test_upgrade_notices.py` (172 discover cases: 148 own methods + the 24 cases
`test_upgrade_notices` inherits from `test_upgrade_output.UpgradeOutputTests`).

PR #521 deletes the eleven S6-2 suites in parallel; those files are disjoint and are
not touched here. Batch B (`test_sync_pipeline`, `test_init_tui`,
`test_tracker_card_gate_hook`, `test_tracker_ledger_witness`,
`test_agents_render_brief_fragments`, `test_bitbucket_pr_flow_recipe`,
`test_doctor_tracker_card`, `test_external_dirs`) is a separate slice.

## Specs

- S1: Standing decision (Robert): finish PR4a. These ten files are NOT pure legacy
  suites — they also test behavior that survives `lib/` (deleted in PR4b). Every method
  falls in exactly one class: **(a)** legacy-implementation detail that dies with
  `lib/`; **(b)** bridge/lib transport contract that dies with PR4b; **(c)** behavior
  that survives and must be covered by a Go test (`file:line` + case) or be added
  test-first in the allowed surfaces.
- S2: Every method of the ten files is classified in the mapping tables below; every
  (c) citation names the Go test and its case; every gap is marked UNCOVERED.
- S3: A file is deleted only when ALL its methods are (a)/(b). A file whose (c)
  methods drive `bin/ai-specs` or the gate binary end to end keeps those methods and
  loses only the `lib/` transport: the method stays, the lib import/exec/source-pin
  goes, and every native assertion is preserved.
- S4: Every UNCOVERED (c) behavior gets a Go test added test-first (RED observed with a
  wrong expectation, GREEN with the value taken from the Python assertion). New
  coverage reuses the owning package's existing helpers; no golden is minted.
- S5: Baseline before any change: the ten files run green and the discover count is
  recorded, so the drop after the slice is reconcilable.
- S6: No surviving test may import `lib/_internal`, exec `python3 lib/...`, copy `lib/`
  into a fixture, or pin a Python source file. Production code, goldens, `tests/parity`,
  `tests/evals` and every other Python test stay untouched.
- S7: `gofmt -l` on the touched Go packages is empty, `go vet` and `go test` on them
  rc=0.
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh` rc=0;
  the Python discover count drops by exactly this slice's deletions (172 → 126 for
  these ten files).

## Baseline (before any change)

`python3 -m unittest test_gate_binary_dist test_orca_aware_delegation_skill
test_worktree_gate_dist_config test_worktree_gate_harness_phase4 test_ledger_mode_config
test_runtime_brief_baseline test_brief_render_policy test_envrc_scaffold
test_upgrade_mode_dirt test_upgrade_notices -v` → `Ran 172 tests in 62.197s — OK`, rc=0
(`logs/02-baseline-ten.log`). Per-file discover counts: 19 + 19 + 21 + 10 + 19 + 13 +
13 + 11 + 10 + 37 = 172. Repo-wide discover at this base: 2097
(`logs/02b-base-discover.log`). This worktree is cut before PR #521 lands, so the
2097 → 1879 delta is #521's, not this slice's.

## Mapping tables

Totals: 172 cases = 148 own methods + 24 inherited.
Classification: (a) 26, (b) 13, (c) 133 — of which 17 were UNCOVERED and are closed by
5 Go test additions. 46 cases deleted, 126 kept.

### tests/test_gate_binary_dist.py — 19 methods: 19 (a), 0 (b), 0 (c) → DELETE

Every method loads `lib/_internal/gate_binary.py` with `importlib` and asserts on the
Python module's own functions. The native port `internal/gatebinary` carries a 1:1 test
for each, so the file dies with `lib/`.

| # | method | class | superseded by |
|---|---|---|---|
| 1 | `test_uname_mapping_covers_matrix_and_rosetta` | (a) | `gatebinary_test.go:108` `TestDetectPlatformMatrixAndRosetta` |
| 2 | `test_uname_mapping_unknown_platform_is_empty` | (a) | `:125` |
| 3 | `test_cache_path_is_version_keyed` | (a) | `:134` |
| 4 | `test_cache_path_defaults_to_installed_version_and_host_platform` | (a) | `:143` |
| 5 | `test_digest_match_installs_executable_binary` | (a) | `:156` |
| 6 | `test_stale_executable_cache_is_revalidated_and_reacquired` | (a) | `:189` |
| 7 | `test_cache_version_mismatch_is_not_executed_before_reacquisition` | (a) | `:227` |
| 8 | `test_cache_selftest_failure_forces_reacquisition` | (a) | `:259` |
| 9 | `test_verification_receipt_failure_leaves_cache_unselected` | (a) | `:290` |
| 10 | `test_digest_mismatch_never_installs_and_records` | (a) | `:315` |
| 11 | `test_partial_download_never_installed` | (a) | `:337` |
| 12 | `test_opt_in_local_build_populates_cache` | (a) | `:511` |
| 13 | `test_offline_auto_and_go_degrade_without_bash_fallback` | (a) | `:563` `TestOfflineAutoAndGoDegradeWithoutBashFallback` |
| 14 | `test_acquire_runs_for_auto_and_go_no_bash_early_return` | (a) | `:563` (asserts `Attempted` for both impls) |
| 15 | `test_unsupported_platform_warns_no_install` | (a) | `:588` |
| 16 | `test_acquisition_never_raises` | (a) | `:608`, `:617` |
| 17 | `test_committed_trust_root_lists_the_release_matrix` | (a) | `:632` `TestCommittedTrustRootListsTheReleaseMatrix` |
| 18 | `test_asset_url_uses_canonical_repository_owner` | (a) | `:670` |
| 19 | `test_gate_binary_module_has_no_divergent_repository_owner` | (a) | `:678` `TestModuleHasNoDivergentRepositoryOwner` (scans `gatebinary.go`) |

### tests/test_orca_aware_delegation_skill.py — 19 methods: 1 (a), 0 (b), 18 (c) → KEEP

The skill markdown assertions are permanent: the skill ships and is parsed by the native
`ai-specs skills validate` route. Only method 1 loads `lib/_internal/skill_contract.py`.

| # | method | class | disposition |
|---|---|---|---|
| 1 | `test_skill_has_complete_metadata_and_explicit_triggers` | (a) for the loader; the asserted metadata survives | port the metadata contract to `internal/skills` (UNCOVERED) → `syncmeta_test.go` `TestOrcaAwareDelegationSkillContract`; delete the Python method |
| 2–19 | markdown/section/budget assertions on `SKILL.md` and its references file | (c) | keep in Python; already lib-free (`setUpClass` only reads the two markdown files once method 1 goes) |

### tests/test_worktree_gate_dist_config.py — 21 methods: 9 (a), 4 (b), 8 (c)

Materialization reachable through `ai-specs sync`, launcher resolution through the
shell launcher, and the digest recheck already ported to `internal/gatebinary`.

| # | method | class | disposition |
|---|---|---|---|
| 1 | `test_recipe_declares_gate_impl_enum_with_default_auto` | (a) Python `recipe_schema` API; the contract survives | UNCOVERED as an explicit enum pin → Go: `mergeconfig_test.go` `TestNativeMergeConfigRealCatalogSchema/stranger gate_impl` (RED), delete the Python method |
| 2 | `test_gate_impl_defaults_to_auto_and_stamps` | (c) | keep-and-de-lib: drive `ai-specs sync` instead of `recipe-materialize.py` |
| 3 | `test_gate_impl_bash_rejected_at_sync` | (c) | keep-and-de-lib via `sync` (rc 1 + `bash`/`removed`/`auto \| go`, no legacy hook) |
| 4 | `test_gate_impl_go_stamps` | (c) | keep-and-de-lib via `sync` |
| 5 | `test_ordinary_sync_never_creates_legacy_hook` | (c) | keep-and-de-lib via `sync` |
| 6 | `test_leftover_legacy_hook_is_not_rewritten` | (c) | keep-and-de-lib via `sync` |
| 7 | `test_invalid_gate_impl_rejected_at_sync_with_enum` | (c) | keep-and-de-lib via `sync` (rust leg) + Go enum pin row 1 |
| 8 | `test_merge_config_rejects_invalid_gate_impl` | (a) Python `merge_config` API | Go: `mergeconfig_test.go:776` (`bash`, exact message) + the stranger leg added in row 1; delete the Python method |
| 9 | `test_sentinel_upgrade_replaces_pre_go_gate` | (c) | keep-and-de-lib via `sync` / `sync --refresh-gates` |
| 10 | `test_launcher_forwards_the_raw_stamped_mode_to_go` | (c) | keep (shell source pin, no lib) |
| 11 | `test_launcher_keeps_literal_staleness_sentinel` | (c) | keep (shell source pin, no lib) |
| 12 | `test_bin_override_wins_over_project_pin` | (c) | keep; `_cache_platform()` stops loading `lib/_internal/gate_binary.py` |
| 13 | `test_project_pin_wins_over_cache` | (c) | keep, same de-lib |
| 14 | `test_cache_binary_used_when_no_pin_or_override` | (c) | keep, same de-lib |
| 15 | `test_non_executable_override_ignored` | (c) | keep, same de-lib |
| 16 | `test_stale_cache_binary_with_sidecar_is_not_resolved` | (b) Python resolver; behavior native | `gatebinary_test.go:760` (`stale digest` leg) |
| 17 | `test_matching_cache_binary_with_sidecar_still_resolves` | (b) | `:760` (`receipt plus a matching committed digest`) |
| 18 | `test_no_committed_digest_keeps_sidecar_acceptance` | (b) | `:760` (`receipt without a committed digest`) — the native port deliberately FAILS CLOSED here (hardened in U2e); the legacy acceptance is dead with `lib/` |
| 19 | `test_unreadable_sums_degrades_to_not_verified` | (b) | `:760` (undecodable trust root) + `:653` `TestLoadExpectedDigestsFailsClosedOnUndecodableRoot` |
| 20 | `test_non_utf8_sums_degrades_to_not_verified` | (b) | `:653`, `:760` |
| 21 | `test_gate_impl_bash_rejected_instead_of_rollback_rehearsal` | (a) duplicate of row 3 through the Python entry point | delete |

### tests/test_worktree_gate_harness_phase4.py — 10 methods: 0 (a), 5 (b), 5 (c)

| # | method | class | disposition |
|---|---|---|---|
| 1 | `test_renderer_output_is_byte_identical_to_preexisting_shape` | (b) transport into `hooks-render.py`; the bytes survive | keep-and-de-lib: render through `ai-specs sync` and diff two runs |
| 2 | `test_all_five_harness_artifacts_reference_the_materialized_path` | (b) same transport | keep-and-de-lib via `sync` |
| 3 | `test_blocked_write_maps_to_permission_deny` | (b) transport (subclass `setUpClass` renders via `hooks-render.py`) | keep-and-de-lib: Cursor wrapper emitted by `sync` |
| 4 | `test_allowed_write_maps_to_permission_allow` | (b) | keep-and-de-lib |
| 5 | `test_deny_message_not_degraded_by_empty_binary_stdout` | (b) | keep-and-de-lib |
| 6 | `test_materialized_launcher_is_directly_executable` | (c) | keep (lib-free) |
| 7 | `test_launcher_runs_without_shell_and_passes_stdin` | (c) | keep (lib-free) |
| 8 | `test_blocked_write_on_protected_branch` | (c) | keep (lib-free) |
| 9 | `test_allowed_write_inside_linked_worktree` | (c) | keep (lib-free) |
| 10 | `test_missing_binary_fails_open_without_legacy_exec` | (c) | keep (lib-free) |

### tests/test_ledger_mode_config.py — 19 methods: 1 (a), 0 (b), 18 (c)

The hosts are shell transports; effective mode resolution is Go
(`catalog/recipes/worktree-flow/gate/ledger_mode.go`, outside this slice's editable
surface) and every host test drives the native binary or a stub gate.

| # | method | class | disposition |
|---|---|---|---|
| 1–6, 11–19 | host forwarding / advisory-vs-blocking / no-predicate assertions | (c) | keep (no lib dependency) |
| 7 | `test_no_hardcoded_recipe_lookup_remains_at_any_host_site` | (c) for the shell pins, (a) for `lib/_internal/doctor.py` | keep-and-de-lib: point the `recipe_id` pin at the native `internal/doctor/checks_tracker.go` (the file already pins `internal/gates/gates.go`, so a Go source anchor is the established shape) |
| 8 | `test_recipe_declares_ledger_mode_enum_and_default` | (a) Python `recipe_schema` API | UNCOVERED as a native config pin → Go: `mergeconfig_test.go` `TestNativeMergeConfigRealCatalogSchema/trello ledger_mode` (RED), delete the Python method |
| 9 | `test_plan_build_gate_drops_the_dead_bridge_seam` | (c) | keep (asserts `__TRACKER_LIB_INTERNAL__` is gone from the shell) |
| 10 | `test_production_go_ledger_has_no_provider_vocabulary` | (c) | keep (Go source scan, no lib) |

### tests/test_runtime_brief_baseline.py — 13 methods: 4 (a), 4 (b), 5 (c)

| # | method | class | disposition |
|---|---|---|---|
| 1 | `test_template_default_enables_session_context` | (b) `--resolved-config-only` transport | UNCOVERED natively as a template contract → Go: `init_test.go` `TestInitTemplateDefaultEnablesSessionContextWithoutDogfoodTokens` (RED), delete |
| 2 | `test_template_default_no_project_specific_tokens` | (b) same transport | same Go addition, delete |
| 3 | `test_fresh_init_produces_behavioral_brief` | (c) | keep (`invoke`); `_home_for` stops copying `lib/` |
| 4 | `test_init_render_failure_falls_back_to_placeholder` | (c) | keep |
| 5 | `test_init_then_sync_is_byte_stable` | (c) | keep |
| 6 | `test_force_init_preserves_runtime_brief_marker` | (c) | keep |
| 7 | `test_no_project_specific_tokens_in_baseline_agents_md` | (c) | keep |
| 8 | `test_session_context_key_wins_over_second_recipe` | (b) craft-resolved-json transport | UNCOVERED natively → Go: `agentsrender_test.go` `TestCollectRecipeBriefFragmentsFirstWinsKeyAndExactText` (RED), delete |
| 9 | `test_session_context_key_dedup_appears_exactly_once_in_full_render` | (b) | same Go addition, delete |
| 10 | `test_sync_preserves_user_edited_agents_md_with_runtime_brief_marker` | (c) | keep |
| 11 | `test_sync_preserves_a_truly_untracked_agents_md` | (c) | keep |
| 12 | `test_sync_without_marker_preserves_user_modified_agents_md` | (c) | keep |
| 13 | `test_no_unrendered_config_placeholders_in_baseline_agents_md` | (c) | keep |

### tests/test_brief_render_policy.py — 13 methods: 11 (a), 0 (b), 2 (c)

| # | method | class | disposition |
|---|---|---|---|
| 1–11 | `test_no_brief_table_defaults_true`, `test_brief_without_render_defaults_true`, `test_render_true`, `test_render_false`, `test_render_string_raises`, `test_render_int_raises`, `test_load_from_toml_file`, `test_cli_prints_false`, `test_render_uppercase_true_is_toml_error`, `test_cli_non_validate_invalid_render_defaults_to_true`, `test_cli_validate_rejects_string` — all exec `python3 lib/_internal/brief-render-policy.py` | (a) | covered by `internal/sync/briefpolicy_test.go:26` `TestBriefRenderPolicy` (verdict matrix) and `:211` `TestBriefRenderPolicyDifferential` (frozen rc/stdout/stderr over 6 manifests × 2 modes, captured from that script); delete |
| 12 | `test_has_dead_recipe_fragments_true` | (c) | keep-and-de-lib (doctor verb; drop the `lib/` fixture copy) |
| 13 | `test_has_dead_recipe_fragments_false` | (c) | keep-and-de-lib |

### tests/test_envrc_scaffold.py — 11 methods: 1 (a), 0 (b), 10 (c)

| # | method | class | disposition |
|---|---|---|---|
| 1–7, 9–11 | `_sync`-driven scaffolding and `recipe configure --inspect` help-text assertions | (c) | keep; `_make_home` stops copying `lib/` |
| 8 | `test_prompt_env_vars_uses_password_api_for_secrets` | (a) — drives `lib/_internal/envrc-scaffold.py prompt_env_vars` with a stubbed `questionary`; the regression is Python-dependency-specific (`password=` kwarg crashes questionary 2.x) and the TTY-gated prompt path is Python-only | delete |

### tests/test_upgrade_mode_dirt.py — 10 methods: 0 (a), 0 (b), 10 (c) → KEEP

| # | methods | class | disposition |
|---|---|---|---|
| 1–6, 8–10 | `ai-specs upgrade` / `install.sh` mode-only dirt remediation and content-dirt blocking | (c) | keep; the fixture stops copying `lib/upgrade.sh` (vestigial: `install.sh` never sources it) |
| 7 | `test_install_sh_tree_is_clean_after_mode_only_remediation` | (c) | keep, same de-lib |

### tests/test_upgrade_notices.py — 37 cases (13 own + 24 inherited): 1 (a), 0 (b), 36 (c)

| # | method | class | disposition |
|---|---|---|---|
| 1–12 | summary/notice replay against the fake native install | (c) | keep (notice extraction itself is pinned in `internal/changelog/changelog_test.go:112`, `:133`) |
| 13 | `test_missing_parser_does_not_break_the_upgrade` — seeds `lib/_internal/changelog.py` into the install then deletes it | (a) — the "missing parser" state cannot exist once the parser is compiled in; the surviving assertion (rc 0, `Upgraded`, no traceback) is already pinned by `test_unreadable_changelog_still_upgrades` and `test_malformed_changelog_degrades_to_plain_line` | delete |
| 14–37 | the 24 cases inherited from `test_upgrade_output.UpgradeOutputTests` | (c) | keep (fixture no longer seeds `lib/`, `test_upgrade_output.py:260`) |

### UNCOVERED (c) closed by this slice

| Go addition | closes |
|---|---|
| `internal/skills/syncmeta_test.go` `TestOrcaAwareDelegationSkillContract` | orca row 1 |
| `internal/sync/mergeconfig_test.go` `TestNativeMergeConfigRealCatalogSchema/{stranger gate_impl, trello ledger_mode}` | worktree-gate rows 1, 8; ledger row 8 |
| `internal/cli/init_test.go` `TestInitTemplateDefaultEnablesSessionContextWithoutDogfoodTokens` | runtime-brief rows 1, 2 |
| `internal/sync/agentsrender_test.go` `TestCollectRecipeBriefFragmentsFirstWinsKeyAndExactText` | runtime-brief rows 8, 9 |
| (no new file needed) `internal/doctor/checks_tracker.go` source pin is carried by the kept ledger row 7 | ledger row 7 |

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S4 | inline | |
| T2 | S5, S6 | inline | |
| T3 | S7, S8 | inline | |

## Log

- L1 (Robert): "ok,vos dale debemos ir terminando esto" — finish PR4a: retire/de-lib the
  legacy Python suites whose behavior is already native.
- L2 (Robert): "excelente, continuemos así muchas gracias" — keep going with the same
  slice discipline (map, verify in Go, then delete).
- L3 — Baseline (S5): the ten files ran green, `Ran 172 tests in 62.197s — OK`, rc=0
  (`logs/02-baseline-ten.log`); repo-wide discover 2097 (`logs/02b-base-discover.log`).
  Builds before the run: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0
  (`logs/01-build-cli.log`) and `scripts/build-gate.sh` rc=0 (`logs/01b-build-gate.log`).
- L4 — Go additions, test-first. Four additions in three packages:
  - `internal/skills/syncmeta_test.go` `TestOrcaAwareDelegationSkillContract`. RED
    `logs/04-red-skills.log` rc=1 — the wrong expectation `scope = [repo]` was reported
    against the real `[root]`; GREEN `logs/06b-green-skills.log` rc=0.
  - `internal/sync/agentsrender_test.go`
    `TestCollectRecipeBriefFragmentsFirstWinsKeyAndExactText`. RED
    `logs/04b-red-dedupe.log` rc=1 — expecting two conflict_policy fragments reported the
    real single first-wins fragment; GREEN `logs/06c-green-sync.log` rc=0.
  - `internal/sync/mergeconfig_test.go`, two subtests of
    `TestNativeMergeConfigRealCatalogSchema`. RED `logs/03-firstrun-sync.log` rc=1: the
    real messages are `invalid gate_impl 'rust'; bash has been removed; allowed: auto | go`
    and `recipe 'Trello MCP Workflow': config field 'ledger_mode' value 'bogus' is invalid;
    allowed: always | ask | warn` — both fixed to the observed values; GREEN
    `logs/06c-green-sync.log` rc=0. Finding: the native enum guard emits the
    bash-specific sentence for ANY out-of-enum gate_impl (the retired Python schema
    message named only allowed values). It still satisfies the retired assertions
    (`rust` named, `auto | go` named, no ` | bash`), so the message is pinned as-is.
  - `internal/cli/init_test.go`
    `TestInitTemplateDefaultEnablesSessionContextWithoutDogfoodTokens`. RED
    `logs/03d-firstrun-cli.log` rc=1 — `the default manifest leaks the dogfood token
    "ai-specs-cli"`, which is a doc comment naming the published CLI repository, not a
    config value; the scan now skips comment lines (faithful to the retired assertion,
    which ran on comment-free resolved JSON). GREEN `logs/06a-green-cli.log` rc=0.
- L5 — Python dispositions (S3), all unstaged: deleted `tests/test_gate_binary_dist.py`
  (19 methods, 1:1 in `internal/gatebinary`); de-libbed `test_orca_aware_delegation_skill`
  (18 kept), `test_worktree_gate_dist_config` (13 kept, 8 deleted: 5 digest-recheck +
  rollback + 2 Python-API schema assertions), `test_worktree_gate_harness_phase4`
  (10 kept, rendering now driven by `ai-specs sync`), `test_ledger_mode_config` (18 kept,
  1 deleted, the doctor source pin now reads `internal/doctor/checks_tracker.go`),
  `test_runtime_brief_baseline` (9 kept, 4 deleted), `test_brief_render_policy` (2 kept,
  11 deleted), `test_envrc_scaffold` (10 kept, 1 questionary-prompt method deleted),
  `test_upgrade_mode_dirt` (10 kept, `lib/upgrade.sh` fixture copy dropped: `install.sh`
  never sources it), `test_upgrade_notices` (36 kept, the parser-seeding case deleted).
  `Ran 126 tests in 60.711s — OK`, rc=0 (`logs/07-ten-after.log`).
- L6 — (S6) no `lib/_internal` import, `python3 lib/...` exec, lib fixture copy or Python
  source pin remains in the ten files; the only survivors are docstring/comment mentions
  (`grep -n 'lib/_internal'` over the ten → 6 comment lines, 0 code lines).
- L7 — (S7) `gofmt -l internal/skills/ internal/sync/ internal/cli/` empty rc=0
  (`logs/05-gofmt.log`); `go vet` on the three packages rc=0 (`logs/08-vet.log`);
  `go test -count=1` on them rc=0 — skills 0.265 s, sync 13.062 s, cli 4.347 s
  (`logs/09-test.log`).
- L8 — (S8) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/10-build-cli.log`),
  then `./tests/validate.sh` rc=0 in ~616 s (`logs/11-validate.log`): gofmt clean, both Go
  modules green, `parity summary: gate-absent failing=0, gate-present failing=0 — PASS`,
  and the Python leg `Ran 2065 tests in 615.662s — OK (skipped=1, expected failures=3)`.
  Count reconciliation: 2111 (the count the merged S6-1 slice left, = this worktree's
  discover 2097 + 14 cases the loader adds at run time) − 46 deletions = 2065 exactly.
  Post-#521 the same delta applies to #521's base (1879 → 1833). The single skip and the
  three expected failures are pre-existing and elsewhere.
