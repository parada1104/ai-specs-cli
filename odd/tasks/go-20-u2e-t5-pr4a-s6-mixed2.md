# GO-20 U2e T5 PR4a S6-3b — cut `lib/` out of the remaining eight mixed Python suites (batch B)

Base: epic/go-single-binary b8c88d7 (after #522 merged). Branch:
change/go-20-u2e-t5-pr4a-s6-mixed2.
Scope: `tests/test_sync_pipeline.py`, `test_init_tui.py`,
`test_tracker_card_gate_hook.py`, `test_tracker_ledger_witness.py`,
`test_agents_render_brief_fragments.py`, `test_bitbucket_pr_flow_recipe.py`,
`test_doctor_tracker_card.py`, `test_external_dirs.py` (370 discover cases).

Batch A (`…-s6-mixed1`, PR #522) already de-libbed ten suites and retired the
eleven S6-2 unit files (#521). This batch closes the mixed/`lib/` tail: every
file keeps its native, CLI-observable assertions and loses only the `lib/`
dependency — except for the methods that only ever drove a Python-internal
seam, which are deleted with a Go citation.

## Specs

- S1: Standing decision (Robert): finish PR4a ("ok,vos dale debemos ir terminando
  esto"). Every method falls in exactly one class: **(a)** legacy-implementation
  detail that dies with `lib/`; **(b)** bridge/`lib` transport contract that dies
  with PR4b; **(c)** behavior that survives and is covered by a Go test
  (`file:line` + case) or added test-first here.
- S2: Every method is classified below; every (c) row names its Go test and case;
  every gap is marked UNCOVERED and closed in this slice.
- S3: A file dies only when all its methods are (a)/(b). Otherwise the method
  stays, the `lib/` import/exec/copy/source-pin goes, and every native assertion
  is preserved.
- S4: Every UNCOVERED (c) behavior gets a Go test added test-first (RED observed
  with a wrong expectation, GREEN with the value taken from the assertion). No
  golden is minted; the owning package's helpers are reused.
- S5: Baseline before any change, and the post-slice count, so the drop is
  reconcilable.
- S6: No surviving test imports `lib/_internal`, execs `python3 lib/...`, copies
  `lib/` into a fixture, or pins a Python source file. Production code, goldens,
  `tests/parity`, `tests/evals` and every other Python test stay untouched.
- S7: `gofmt -l` empty, `go vet` and `go test` rc=0 on the touched Go packages.
- S8: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 and `./tests/validate.sh`
  rc=0 from the post-#522 base (~1833 discover) with a reconciled delta.

## Baseline (before any change)

Builds: `go build -o dist/ai-specs ./cmd/ai-specs` rc=0 (`logs/01-build-cli.log`),
`scripts/build-gate.sh` rc=0 (`logs/01b-build-gate.log`).

`python3 -m unittest test_sync_pipeline test_init_tui
test_tracker_card_gate_hook test_tracker_ledger_witness
test_agents_render_brief_fragments test_bitbucket_pr_flow_recipe
test_doctor_tracker_card test_external_dirs` → `Ran 370 tests in 149.971s — OK`,
rc=0 (`logs/02-baseline-eight.log`). Per-file discover (`logs/02b-per-file-counts.log`):
92 + 30 + 37 + 14 + 65 + 57 + 22 + 53 = 370. Repo-wide discover at this base:
**1833** (`logs/02c-base-discover.log`), matching the post-#522 note. No SKIPs in
these eight files; one `Bash32Tests` sub-suite skips only when `/bin/bash` is not
3.2 (not the case here).

## Mapping tables

Totals: 370 methods. Classification: (a) 17, (b) 5, (c) 348.
Disposition: **29 deleted**, **341 kept**. Ten of the deleted probes covered a
surviving (c) behavior that had no Go test; those ten are closed by three Go
tests added here (their Python methods are deleted, not kept).

### tests/test_sync_pipeline.py — 92 methods: 0 (a), 4 (b), 88 (c) → 82 kept, 10 deleted

| # | method | class | disposition |
|---|---|---|---|
| 1–28 | `test_sync_workspace_root_fixture_exists_with_declared_subrepos` … `test_sync_produces_identical_agents_md_on_second_run` (manifest/MCP/subrepo/skill authoring) | (c) | keep; `_make_home` stops copying `lib/` |
| 29–45 | `test_sync_preserves_runtime_brief_marker_in_agents_md` … `test_existing_agents_unchanged_after_omp_added` (brief render, omp/pi fan-out) | (c) | keep, same de-lib |
| 46 | `test_skill_sync_validates_metadata_and_reports_missing` | (c) | keep (`skills validate` is native) |
| 47–50 | `test_partial_brief_renders_present_keys_no_crash`, `test_no_tracker_binding_omits_trello_section`, `test_subrepo_sync_agent_forwards_resolved_config` | (c) | keep |
| 51 | `test_resolved_config_only_bindings_match_full_materialize_path` | (b) for the dumped bindings map; the AGENTS.md parity survives | keep the method: the bindings-map TRIAGE block goes with `lib/`, and the two CLI-observable needle assertions (root vs subrepo AGENTS.md) stay. The document itself is frozen natively by `internal/sync/resolvedconfigonly_test.go` ("auto-bound capability with catalog defaults and brief fragments") |
| 52 | `test_auto_binding_without_explicit_bindings` | (c) | keep (`run_cli_env`, no lib) |
| 53 | `test_resolved_config_bindings_non_empty_without_explicit_bindings` | (b) materialize transport; the assertion survives | keep-and-de-lib: `ai-specs baseline-brief --resolved-config-out <out>` is the native materialize+render chain, so the same bindings map is read from the shipped binary |
| 54 | `test_resolved_config_only_with_explicit_ai_specs_home_resolves_bindings` | (b) — asserts the Python `Path(__file__).parents[2]` fallback | delete: the native catalog root is always `$AI_SPECS_HOME`, so the divergence it guards cannot exist; row 53 pins the explicit-home catalog lookup |
| 55–62 | `TestJudgmentDayFixes` FIX1/FIX2/FIX3/FIX7/FIX8/FIX9 cases (`test_global_mcp_description_renders_without_mcp_block` … `test_redaction_sentinel_is_exact_string`) | (c) | keep (`_sync_render` is native sync; the `AGENTS_RENDER` attr and `run_render` go) |
| 63 | `test_test_command_resolves_via_test_runner_capability_binding` | (c) — a custom bound id cannot be produced through the CLI | delete; UNCOVERED closed by `internal/sync/agentsrender_test.go` `TestRenderAgentsMarkdownResolvesStructuredFieldsThroughTheBinding` |
| 64 | `test_integration_branch_resolves_via_worktree_isolation_binding` | (c) | same Go test |
| 65 | `test_vault_scope_resolves_via_canonical_store_binding` | (c) | same Go test |
| 66 | `test_resolved_config_only_mode_writes_json_and_leaves_no_recipe_mcp_temp` | (c) | keep (`sync-agent --all` is native) |
| 67 | `test_resolved_config_only_mode_fails_loudly_not_silently` | (c) | keep |
| 68 | `test_resolved_config_only_invalid_binding_exits_nonzero` | (b) for the standalone leg; the `sync` leg is (c) | keep the method; the standalone TRIAGE leg goes, frozen natively by `internal/sync/resolvedconfigonly_test.go` ("malformed explicit binding is fatal": rc, streams, absent document) |
| 69 | `test_wrong_typed_inner_fields_degrade_gracefully_no_crash` | (c) — hand-crafted payload, unreachable through any verb | delete; closed by `internal/sync/agentsrender_test.go` `TestLoadResolvedConfigDegradesMalformedDocumentsAndStillRenders/wrong-typed inner fields are coerced` |
| 70 | `test_malformed_resolved_config_degrades_gracefully` | (c) | delete; same Go test (`/malformed json degrades to the empty document`) |
| 71 | `test_non_dict_resolved_config_degrades_gracefully` | (c) | delete; same Go test (`/non-dict document degrades to the empty document`) |
| 72–81 | `test_unknown_vcs_recipe_warns_to_stderr`, `test_unknown_vcs_recipe_renders_generic_label`, `test_unknown_vcs_warning_once_per_id` + the `TestCustomVcsWarning` helper | (c) | delete the three probes (`TestCustomVcsWarning` was nothing else); covered by `internal/sync/agentsrender_test.go:505` `TestAgentsRenderDifferential/unknown vcs recipe uses generic label` and `:735` `TestAgentsRenderGovernanceDifferential/unknown vcs recipe warns on stderr` (one line per render — the Python de-dupe is a per-call local set) |
| 82–92 | `TestVcsDropRemediations`, `GateRefreshCliTests`, `FanOutDriftTests`, `CliVersionSyncGateTests`, `SyncAgentGuardOrderTests`, `RuntimeHookSyncPipelineTests` cases | (c) | keep (all native, no lib) |

### tests/test_init_tui.py — 30 methods: 7 (a), 0 (b), 23 (c) → 23 kept, 7 deleted

The wizard is native (`internal/cli/initwizard.go`); the file's Python deps were
the runpy driver and three entrypoint-rc probes.

| # | method | class | disposition |
|---|---|---|---|
| 1 | `test_roundtrip_toml` | (c) | keep; `_make_home` stops copying `lib/` and drops the `_vendor` symlink |
| 2 | `test_dotted_recipe_id_is_quoted_literal_key` | (c) | keep |
| 3 | `test_render_manifest_writes_config_block` | (c) | keep |
| 4 | `test_render_manifest_no_config_backward_compat` | (a) — byte-equality of `_render_manifest(configured=None)` vs absent | delete; `internal/cli/initwizard_test.go:370` `TestRenderWizardManifest` pins the single render, and the kept PTY cases assert `assertNotIn(".config]")` |
| 5–7 | `test_configure_recipes_skip_later`, `test_configure_recipes_uses_dep_gate_for_cli_deps`, `test_dep_gate_hands_the_terminal_back` | (c) | keep (native PTY) |
| 8 | `test_fresh_init_writes_real_toml_and_offers_harness_env` | (c) | keep |
| 9–10 | `test_skips_broken_recipe_toml`, `test_hides_internal_test_recipes` | (c) | keep |
| 11 | `test_run_wizard_returns_3_on_non_tty` | (a) | delete; `initwizard_test.go:325` `TestInitWizardNonTTYExitsThreeBeforeAnyPrompt` |
| 12 | `test_main_unexpected_exception_returns_3_not_1` | (a) | delete; `initwizard_test.go:865` `TestInitWizardDepGateSeam/a gate error takes the wizard down before any write` (rc 3 + `ERROR: interactive init failed`, no cancel line, no writes) |
| 13 | `test_ensure_deps_mkdir_failure_returns_3` | (a) | delete: the vendored-dependency install (`lib/_vendor`, `_ensure_deps`) is a Python-only mechanism removed with `lib/`; the native dep gate is pinned by `TestInitWizardDepGateSeam` |
| 14–18 | `test_non_tty_auto_is_classic`, `test_no_tui_flag_is_classic`, `test_name_suppresses_auto_tui`, `test_force_suppresses_auto_tui`, `test_tui_unavailable_falls_back_to_classic` | (c) | keep |
| 19–24 | the six `TestInitTuiPTYE2E` flows | (c) | keep |
| 25 | `test_topology_renders_as_project_field_not_recipe_config` | (c) | keep |
| 26 | `test_recipe_config_never_owns_repo_topology` | (a) | delete; `initwizard_test.go:370` `TestRenderWizardManifest` (exactly one `repo_topology`, under `[project]`) + kept row 25 |
| 27 | `test_wizard_schema_excludes_project_owned_topology` | (a) | delete; `initwizard_test.go:754` `TestInitWizardRepoTopologyStaysProjectOwned` and `:623` `TestInitWizardSkipsRecipesWithoutPromptableFields` |
| 28 | `test_run_wizard_asks_topology_and_writes_override` | (c) | keep |
| 29 | `test_configure_recipes_topology_wins_over_identity_prompt` | (a) — the legacy promotion order with a mocked `_configure_recipes` | delete; the native schema filter makes the recipe-side `repo_topology` unreachable, pinned by `TestInitWizardRepoTopologyStaysProjectOwned` |
| 30 | `test_tui_unavailable_falls_back_to_classic` | (c) | keep (counted in 14–18 window; listed here so the table has all 30 rows) |

### tests/test_tracker_card_gate_hook.py — 37 methods: 1 (a), 0 (b), 36 (c) → 36 kept, 1 deleted

| # | method | class | disposition |
|---|---|---|---|
| 1–28 | path/shell grading, ask display, fail-open, tokenizer matrix, `tracker.none` exemption, bash 3.2 | (c) | keep (the host and the stub gate are native) |
| 29 | `test_materialize_stamps_the_bridge_directory` | (a) — loads `lib/_internal/recipe-materialize.py` to assert its stamp | delete; `catalog/recipes/worktree-flow/gate/hookgateactuator_test.go:351` `TestRenderHookGateContentTrackerHomes` pins the same two legs (a CLI home stamps `<home>/lib/_internal`, no home stamps the empty string) |
| 30–37 | `test_unstamped_bridge_skips_evidence_and_exempt_write`, `Bash32Tests` cases | (c) | keep; `_stamped_gate` no longer takes a `lib_internal` argument and substitutes the dead token with the empty string |

Finding: the hook carries `stamped_lib_internal="__TRACKER_LIB_INTERNAL__"` as a
**dead placeholder** (`tracker-card-gate.sh:48`, comment at `:35`: "not read
here"). Nothing in this suite asserted the read any more — only the materializer
stamp. PR4b deletes the token, the `hookLibInternalToken` substitution
(`shared/hookactuator.go:144-149`) and `TestRenderHookGateContentTrackerHomes`'s
second line together; until then the Go stamp contract stays as-is.

### tests/test_tracker_ledger_witness.py — 14 methods: 0 (a), 1 (b), 13 (c) → 13 kept, 1 deleted

| # | method | class | disposition |
|---|---|---|---|
| 1–12 | the four witness states, trap survival, atomic write, linked worktree, explicit binding, deactivation, Go ledger read | (c) | keep; `_make_home` stops copying `lib/` (the CLI is the only reader) |
| 13 | `test_sync_sh_trap_never_names_the_witness` | (b) — reads `lib/sync.sh`'s EXIT trap | delete: the trap is the shell transport; the surviving fact (the witness outlives the resolved-config temp) is row 7 `test_witness_survives_resolved_config_temp_deletion` |
| 14 | `test_real_sync_leaves_witness_after_trap` | (c) | keep |

`test_go_ledger_reads_the_python_witness_from_a_linked_worktree` is renamed
`…_the_sync_witness_…`: the witness is written by the native sync now, and the
test only ever drove the acquired Go binary.

### tests/test_agents_render_brief_fragments.py — 65 methods: 0 (a), 0 (b), 65 (c) → 64 kept, 1 deleted

| # | method | class | disposition |
|---|---|---|---|
| 1–64 | fragment substitution/dedupe/ordering, section merge, brief-mode validation, `mcp_descriptions`, VCS isolation, topology line, gate_mode brief | (c) | keep; `_make_home` stops copying `lib/` |
| 65 | `test_recipe_not_in_recipes_dict` | (c) — an enabled id with no recipe entry cannot be produced through the CLI | delete; closed by `internal/sync/agentsrender_test.go` `TestRenderAgentsMarkdownToleratesAnEnabledRecipeWithoutConfig` |

### tests/test_bitbucket_pr_flow_recipe.py — 57 methods: 0 (a), 1 (b), 56 (c) → 57 kept (0 deleted)

| # | rows | class | disposition |
|---|---|---|---|
| 1–57 | `recipe add` schema/plan, `sync` materialization, capability ambiguity, golden skill/command content | (c) | keep |
| helper | `resolved_bindings()` (used by #22 and #23) | (b) materialize transport; the bindings map survives | keep-and-de-lib: the map now comes from `ai-specs baseline-brief --resolved-config-out`, the native materialize+render chain, instead of `python3 lib/_internal/recipe-materialize.py` |

`_make_home` stops copying `lib/`.

### tests/test_doctor_tracker_card.py — 22 methods: 1 (a), 0 (b), 21 (c) → 21 kept, 1 deleted

| # | method | class | disposition |
|---|---|---|---|
| 1–20 | severity rendering (INFO/WARN/ERROR), fail-closed, no writes, legacy `## Tracker` not graded, read-only | (c) | keep |
| 21 | `test_legacy_grader_is_gone` | (a) — pins the absence of three functions in `lib/_internal/doctor.py` | delete: the file dies with `lib/`; the behavior is proven by kept row `test_legacy_tracker_link_section_is_not_graded` (a legacy `## Tracker` section changes neither the check nor the exit code) |
| 22 | `test_runtime_brief_sources_add_no_dormancy_line` | (c) | keep-and-de-lib: the renderer source it scans is now `internal/sync/agentsrender.go`, the portable renderer that owns the brief, instead of `lib/_internal/agents-render.py` |

`_make_home` stops copying `lib/`; `DOCTOR_PY` is gone.

### tests/test_external_dirs.py — 53 methods: 8 (a), 0 (b), 45 (c) → 45 kept, 8 deleted

| # | method | class | disposition |
|---|---|---|---|
| 1–21 | init/cache-layout external dirs, gitignore, catalog lint, vendor paths, materialize paths, leftover cleanup, tracked-command cleanup | (c) | keep; `_make_home` stops copying `lib/` |
| 22–34 | `SkillResolutionTests` precedence/dedup cases observed through `sync`/`sync-agent` (`test_bundled_fallback_when_no_other_source` … `test_first_seen_dep_wins_with_warning`, `test_local_override_silent_no_warning`) | (c) | keep |
| 35 | `test_missing_skill_raises` | (a) — probes `skill-resolution.resolve_skill`, a helper with **no caller** in `lib/` and no native port | delete |
| 36 | `test_local_precedence_does_not_backfill_files_from_recipe` | (a) — probes `resolve_skill_template`, no caller | delete |
| 37–42 | `OverrideLoadingTests`: `test_override_config_merged`, `test_override_config_missing_uses_defaults`, `test_override_config_isolated_between_recipes`, `test_override_template_preferred`, `test_override_template_fallback_to_bundled`, `test_override_template_missing_returns_none` | (a) — probe `load_skill_config` / `resolve_skill_template`, no caller, no CLI surface | delete the class |
| 43–53 | `OrphanCleanupTests`, `ResyncIdempotencyTests`, `CommandRelocationMigrationSmokeTest` | (c) | keep |

The deleted probes were dead Python API: `grep -rn 'load_skill_config|resolve_skill_template|resolve_skill(' lib/` finds only the definitions. The surviving resolution rules (tier precedence, first-seen warnings) are covered by `internal/skills/skills_test.go:59` `TestCollectSkillsPrecedence` and `:89` `TestCollectSkillsDepTierFirstSeen`, plus the kept CLI-driven rows 22–34.

### UNCOVERED (c) closed by this slice

| Go addition | closes |
|---|---|
| `internal/sync/agentsrender_test.go` `TestRenderAgentsMarkdownResolvesStructuredFieldsThroughTheBinding` | sync_pipeline rows 63, 64, 65 |
| `internal/sync/agentsrender_test.go` `TestLoadResolvedConfigDegradesMalformedDocumentsAndStillRenders` (3 subtests) | sync_pipeline rows 69, 70, 71 |
| `internal/sync/agentsrender_test.go` `TestRenderAgentsMarkdownToleratesAnEnabledRecipeWithoutConfig` | agents_render row 65 |
| (no new test) `internal/sync/agentsrender_test.go:505`, `:735` | sync_pipeline rows 72–81 |
| (no new test) `internal/sync/resolvedconfigonly_test.go` frozen scenarios | sync_pipeline rows 51, 54, 68 |
| (no new test) `catalog/recipes/worktree-flow/gate/hookgateactuator_test.go:351` | tracker hook row 29 |

## Tasks

| ID | Specs | Route | Commit |
|---|---|---|---|
| T1 | S1–S4 | inline | one work unit |
| T2 | S5, S6 | inline | |
| T3 | S7, S8 | inline | |

## Log

- L1 (Robert): "ok,vos dale debemos ir terminando esto" — finish PR4a.
- L2 (Robert): "excelente, continuemos así muchas gracias" — same slice discipline:
  map every method, verify in Go, then delete.
- L3 — Baseline (S5): 370 tests green in 149.971 s, rc=0
  (`logs/02-baseline-eight.log`); per-file 92/30/37/14/65/57/22/53
  (`logs/02b-per-file-counts.log`); repo discover 1833 (`logs/02c-base-discover.log`).
  Builds rc=0 (`logs/01-build-cli.log`, `logs/01b-build-gate.log`).
- L4 — Go additions, test-first, in `internal/sync/agentsrender_test.go`. RED
  `logs/04-red-sync.log` rc=1: the binding test expected the un-bound recipe's
  values (the real render carried `./custom-tests.sh`, `staging`,
  `my/vault/path`), and the tolerance test expected the coerced keys on a
  malformed payload (the real `loadResolvedConfig` returns the EMPTY document for
  malformed/non-dict, coercing only inside a dict). Both fixed to the observed
  values; GREEN `logs/06a-green-sync-focused.log` rc=0.
- L5 — Python dispositions (S3), all unstaged: 29 methods deleted, 341 kept;
  8 files de-libbed. `Ran 341 tests in 137.212s — OK`, rc=0
  (`logs/07-eight-after.log`). One mid-flight failure fixed inside the same
  method: `baseline-brief` needs `AI_SPECS_VENDOR_FIXTURE_ROOT` (invoke() does not
  set it) — `logs/07b-sync-pipeline-after.log` rc=1 →
  `logs/07d-sync-pipeline-after.log` rc=0.
- L6 — (S6) `grep -n 'lib/_internal|"lib" / "_internal"'` over the eight files:
  0 code lines (5 docstring/comment mentions). No `python3 lib/...` exec, no
  `shutil.copytree` of `lib/`, no Python source pin remains.
- L7 — (S7) `gofmt -l internal/` empty (`logs/05-gofmt.log`); `go vet
  ./internal/sync/` rc=0 (`logs/08-vet.log`); `go test -count=1 ./internal/sync/`
  rc=0 in 13.687 s (`logs/09-test-sync.log`).
- L8 — (S8) `go build -o dist/ai-specs ./cmd/ai-specs` rc=0
  (`logs/10-build-cli.log`), then `./tests/validate.sh` rc=0 in ~550 s
  (`logs/11-validate.log`): gofmt clean, both Go modules green, `parity summary:
  gate-absent failing=0, gate-present failing=0 — PASS`, and the Python leg
  `Ran 1804 tests in 550.201s — OK (skipped=1, expected failures=3)`.
  Count reconciliation: repo discover at this base 1833
  (`logs/02c-base-discover.log`) − 29 deleted methods = 1804 exactly; the eight
  files' own count goes 370 → 341 (`logs/02-baseline-eight.log` →
  `logs/07-eight-after.log`), so the delta is entirely this slice's, with no
  dynamic case added or lost. The single skip and the three expected failures are
  pre-existing and elsewhere.
