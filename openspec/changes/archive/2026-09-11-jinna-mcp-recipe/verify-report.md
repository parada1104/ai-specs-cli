```yaml
schema: gentle-ai.verify-result/v1
evidence_revision: sha256:9f211b849e3217d515901dc265c92fbf77334238a4777c62c647b3bfa4b47e28
verdict: pass
blockers: 0
critical_findings: 0
requirements: 11/11
scenarios: 29/29
test_command: PYTHONPATH=. python3 -m unittest tests.test_jinna_provider_recipe tests.test_recipe_schema tests.test_recipe_read tests.test_dep_check tests.test_config_wizard tests.test_recipe_materialize tests.test_recipes_catalog tests.test_recipe_add tests.test_recipe_configure tests.test_doctor tests.test_recipe_list tests.test_recipe_init
test_exit_code: 0
test_output_hash: sha256:87fafd202ee8c13af97e968631267c693a19452d20478842db1dfd0334052c5d
build_command: python3 -m py_compile lib/_internal/*.py tests/*.py
build_exit_code: 0
build_output_hash: sha256:71936118d07c56a7c62db4b48be83f9b7821ba0dea88e781ac7f02c77aa4738a
```

# Verify report: `jinna-mcp-recipe`

Change root: `openspec/changes/jinna-mcp-recipe/` in worktree
`/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/jinna-mcp-recipe`
(branch `parada1104/jinna-mcp-recipe`, HEAD `c4968c14c5428cd3309cfa1fe6fe511dc7508280`, merge-base with `development` = `6f5bf73`).
Depth: Full. Strict TDD: active (`openspec/config.yaml` → `strict_tdd: true`, runner `./tests/run.sh`).

`evidence_revision` is `sha256` over (`git rev-parse HEAD` + `git status --porcelain` + `git diff --numstat`) captured in this worktree before the report was written. `test_output_hash` is the sha256 of the exact captured output of the envelope `test_command` (`/tmp/verify_focused.out`); `build_output_hash` is the sha256 over the exact build-stage output text `py_compile OK` + `bash -n OK`.

## Result Contract

- **status**: `pass` — 11/11 requirements and 29/29 scenarios verified PASS; strict-TDD evidence verifiable with one recorded artifact-shape deviation; the single full-suite failure is a pre-existing environmental defect proven non-regressive.
- **executive_summary**: The change delivers the `jinna-mcp-recipe` catalog recipe (GitHub-release installer, `{dep:jinna}` marker resolution, safe OpenProject env references, declared `env_allowed` validation). All 29 spec scenarios PASS: 9 of them re-confirmed by this verify phase's own focused runs (423-test focused suite OK; the 9 Phase-9 tests OK), the remainder via change tests green in the same run plus parent-collected live provider evidence (release `v0.1.0` install, receipt verified, MCP handshake, sync materialization). The full suite's only failure (`test_apply_progress_omits_absolute_host_and_worktree_paths`) is pre-existing on the untouched `development` base, environmental (expects an active change folder that is archived), and outside this change's edit surfaces — recorded as a known limitation, not a regression. Review workload is ~5.2k changed lines against a 400-line budget with chained PRs recommended and the delivery decision still pending (`ask-on-risk`).
- **artifacts**: `verify-report.md` (this file; the only write performed by this phase).
- **next_recommended**: `sync` — with verify evidence in place, `sync` becomes actionable, then archive-tail (task `8.6`) after the delivery decision.
- **risks**: pre-existing bitbucket golden-test failure (harness follow-up, blocks a clean `./tests/validate.sh` exit until fixed); two accepted test-strength caveats (paired/triangulated); delivery decision pending (~13× review budget); live runtime behaviors are parent-recorded, not re-executed by this phase; `8.5`/`8.6` checkboxes remain unticked in `tasks.md` because verify is report-only.
- **skill_resolution**: `paths-injected` (`testing-foundation`, `tdd-flow` loaded from the injected project paths; strict-TDD module from `~/.pi/agent/gentle-ai/support/strict-tdd-verify.md`; no project-local override present).

## Scenario coverage — 11 requirements / 29 scenarios (all PASS)

Evidence keys: **R** = re-run by this verify phase (exit code observed firsthand); **A** = change test suite, green inside this phase's 423-test focused re-run; **P** = parent-collected live/provider evidence (not re-executed here, hash-chain partially re-verified); **F** = file/content inspection performed by this phase.

### R1 — Declare the provider dependency (3/3 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Valid provider dependency is parsed | PASS | F: `catalog/recipes/jinna-mcp-recipe/recipe.toml` declares binary `jinna`, purpose, `version_check = "jinna version"`, `min_version = "0.1.0"`, guidance URL, `installer = "github-release"`, `repository = "parada1104/jinna-provider"`, `release_policy = "latest-stable"`, and one local MCP preset with `command = "{dep:jinna}"`, `args = ["mcp"]`, env refs `$OPENPROJECT_BASE_URL` / `$OPENPROJECT_API_TOKEN` / `$OPENPROJECT_AUTH` only — no token literals, no source path. A: `ProviderDependencySchemaTests` + `CatalogRecipeTests` (`tests/test_jinna_provider_recipe.py`) green in focused run. |
| Legacy recipes remain compatible | PASS | A: existing schema/round-trip suites (`test_recipe_schema.py`, `test_recipe_read.py`) green in the focused run; `recipe-read` round-trips the new fields omitting empty values (apply-progress, task 1.2); no legacy `[[deps.cli]]` recipe requires the new fields (they are optional with strict validation). |
| Invalid installer declarations are rejected | PASS | A: `ProviderDependencySchemaTests` rejects unknown installer kinds, malformed/missing repository, unsupported policy, and non-allowlisted repositories (apply-progress task 8.4 mapping); validation failure produces no install plan (plan construction is downstream of schema validation). |

### R2 — Detect and reuse the provider (3/3 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Compatible PATH provider is reused | PASS | A: `test_path_provider_marker_resolves_to_bare_command` (`tests/test_jinna_provider_recipe.py:119`) — PATH candidate resolves to bare `jinna`; `test_resolution_never_touches_the_network` (`:892`) proves no download; resolver never overwrites a compatible PATH candidate (apply-progress R2 mapping, `ProviderDependencyCheckTests`). |
| Missing provider is identified | PASS | A: `test_provider_resolution_error_degrades_to_omitted_entry` (`:107`), `test_dep_gate_non_tty_never_offers_provider_install` (`tests/test_config_wizard.py:279`), `test_non_tty_never_executes_github_install` (`:382` of the jinna suite) — required/unresolved reporting with release guidance; MCP readiness never claimed. |
| Incompatible PATH provider is preserved | PASS | A: `ProviderDependencyCheckTests` + `test_managed_candidate_with_inconsistent_receipt_is_rejected` (`:861`) — incompatible/failed candidates are reported and never modified or deleted; managed installation only on explicit consent. |

### R3 — Offer an explicit GitHub Release installation (3/3 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| User accepts a supported installation | PASS | A: `ProviderConsentPreviewTests` / `test_github_release_preview_covers_consent_fields` (`tests/test_jinna_provider_recipe.py:1370`) pins the complete plan (repository, policy/tag, target, archive, destination, checksum source, replacement behavior) before confirmation; `test_offer_accept_runs_and_rechecks` pins execute-once + recheck. P: real v0.1.0 install into an isolated cache with checksum-verified receipt (apply-progress task 8.3 + fourth-turn re-verification). |
| User declines installation | PASS | A: `test_offer_decline_no_run` — decline performs no download, no filesystem mutation, leaves canonical release URL + manual guidance and the provider reported unresolved. |
| Non-interactive execution cannot install | PASS | A: `test_non_tty_never_executes_github_install`, `test_dep_gate_non_tty_never_offers_provider_install`, `test_resolution_never_touches_the_network` — `doctor`/`sync`/CI paths never download or install. |

### R4 — Select and verify the provider release (3/3 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Correct host artifact is selected | PASS | A: `test_supported_platform_mapping` (`:425`) + release-metadata tests pin the exact five `jinna_<version>_<os>_<arch>.tar.gz`/`.zip` names, draft/prerelease skip, and wrong-host rejection. P: live darwin-arm64 run selected `jinna_v0.1.0_darwin_arm64.tar.gz`. |
| Unsupported platform is rejected | PASS | A: `test_unsupported_platform_lists_targets_and_guidance` (`:936`) — no download/execution; message lists the five supported targets + manual guidance URL. |
| Checksum mismatch is rejected | PASS | A: `test_checksum_mismatch_does_not_publish_target` (`:535`) + SHA256SUMS malformed/short-digest/missing-entry/duplicate-entry rejection; the binary is never executed (`run_version` never called on mismatch); prior installation untouched. |

### R5 — Validate archive contents and install atomically (3/3 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Valid archive is installed | PASS | A: extraction/self-check/publication tests with `0755` mode and post-verification `install.json` receipt (ProviderReleaseTests/ProviderInstallerHardeningTests). P: live receipt `status: verified`. R (read-only, this phase): installed binary sha256 `2a94f2e7…e7a5` equals the receipt's `binary_sha256` and the digest recorded in apply-progress. |
| Archive traversal is rejected | PASS | A: absolute paths, `../` traversal, symlink/hardlink, duplicate and unexpected members, wrong root, oversize member (32 MiB cap, boundary-tested) all fail before executing any member, with staging cleanup. |
| Existing managed version survives a failed replacement | PASS | A: `test_prior_managed_candidate_survives_every_injected_failure` (`:1231`) — six injected failure stages (metadata, sums, archive download, checksum mismatch, unsafe archive, version self-check) each leave the prior `v0.0.9` binary/receipt byte-identical, no published target, no staging leftovers, and `resolve_provider` still returns the prior verified candidate. |

### R6 — Resolve the provider command for MCP materialization (3/3 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Managed provider is materialized | PASS | A: `McpMarkerTests` (`:36`) — verified managed candidate resolves to its absolute path; `{dep:jinna}` never reaches generated config. P: live `sync` materialized the resolved managed absolute path in `.mcp.json` / `opencode.json` / `.cursor/mcp.json`. |
| Missing provider does not create a broken MCP command | PASS | A: `test_provider_resolution_error_degrades_to_omitted_entry` (`:107`) — resolution failure degrades to an omitted entry plus exactly one actionable warning; `doctor` reports the required provider. |
| Runtime-specific translation remains correct | PASS | A: `test_opencode_receives_local_command_array` + `test_generic_runtimes_keep_concrete_command_and_env_refs` (`ProviderRuntimeRenderingTests`, `:167`) — OpenCode gets `[resolved_command, "mcp"]`, Claude/Cursor/Pi/OMP keep their local shape with `${VAR}` env refs, no token literals. P: live sync output matched. |

### R7 — Configure OpenProject safely (2/2 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Environment example is generated | PASS | R: `test_generate_env_example_renders_openproject_auth_provider_default` + `test_generate_env_example_matches_real_catalog_jinna_recipe` green in the 9-test/focused runs — example renders `OPENPROJECT_AUTH=basic` (example-only default; `missing_required_values` unchanged) with help text; F: recipe env table contains only `$OPENPROJECT_*` references. P: live `ai-specs.env.example` rendered the same lines. |
| Installation works without OpenProject credentials | PASS | A: installer never contacts OpenProject; the two release smoke tests are opt-in (`AI_SPECS_JINNA_SMOKE_BINARY`) and skipped in deterministic runs. P: real v0.1.0 install succeeded into an isolated cache with no OpenProject env configured; credential setup reported as a separate step. |

### R8 — Preserve protocol and operational boundaries (2/2 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Local provider is selected explicitly | PASS | F: recipe declares exactly one local preset (`jinna mcp`, direct APIv3) plus the `jinna-mcp-no-fallback` workflow rule; the four provider/OpenProject/`jinna-client`/official-`/mcp` distinctions live in README/init/skill. A: `CatalogRecipeTests` green. P: live MCP handshake `serverInfo {name: jinna-provider, version: v0.1.0}`. |
| Local write failure remains local | PASS | F: grep of `provider_install.py` / `recipe-materialize.py` / `dep_install.py` finds no fallback or write-replay path (the only `/mcp` hit is the release archive member `docs/mcp.md` at `provider_install.py:222`); no hook exists in the recipe. Errors surface to the runtime unchanged. |

### R9 — Preserve compatibility and explain recovery (2/2 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Existing runtime settings survive materialization | PASS | A: existing materialization suites (`test_recipe_materialize.py`, renderer suites) green in the focused run; ownership is limited to the recipe MCP namespace; unrelated keys untouched (existing renderer rules unchanged — `git diff` shows no renderer rewrites). |
| Recipe removal does not delete provider data | PASS | A/F: `test_recipe_readme_rollback_matches_the_managed_layout` (`:257`) + `test_design_documents_tag_keyed_cache_layout` (`:249`) pin that the managed cache is user-owned and rollback = reacquire/remove instructions; no deletion path exists in recipe disable/remove; no OpenProject data or PATH executable is touched. |

### R10 — Evidence and provider readiness gate (2/2 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Deterministic suite runs without live services | PASS | R: focused 12-module suite (423 tests) OK with no network/credentials; the only skips are the 2 opt-in smoke tests. P: full `./tests/validate.sh` / `./tests/run.sh` (1900 tests, 135 skipped) show exactly one failure — the pre-existing bitbucket golden test (see Known limitations), unrelated to recipe/schema/installer/materialization tests, all of which pass. |
| Provider readiness is incomplete → blocked | PASS | A: the gate was executed and recorded PASS before apply (apply-progress: `v0.1.0` stable, five archives + `SHA256SUMS` + `RELEASE.json`, Actions run `34383613223` green, remote download verified); no recipe result claims more than that verified state. |

### R11 — Validate declared environment values (3/3 PASS)

| Scenario | Result | Evidence |
|---|---|---|
| Constrained variable cannot take an out-of-set value | PASS | R: `test_prompt_env_vars_uses_select_for_declared_allowed_values` (green in the 9-test run) — `questionary.select` offers exactly `["basic", "bearer"]`, the persisted value comes from the choice list, and `OPENPROJECT_AUTH` is never offered as free text. F: `env_scaffold.prompt_env_vars` constrained branch. |
| Out-of-set configured value is reported early | PASS | R: `test_invalid_harness_env_value_warns_with_allowed_values` — `doctor` emits a `harness-env-value` WARN naming `DEMO_MODE` and `on, off` without echoing the configured value; F: `doctor.py:975` compares case-insensitively. |
| Malformed declaration is ignored | PASS | R: `test_collect_env_allowed_ignores_malformed_declaration` — string (`"basic"` → would become `['b','a','s','i','c']`) and non-iterable (`7` → `TypeError`) shapes both yield no constraint and no error (fail-open), in both config and doctor paths. |

**Spec coverage total: 11/11 requirements, 29/29 scenarios PASS.**

## Strict TDD verification

**Artifact-shape honesty (recorded deviation).** `apply-progress.md` does not use the strict-TDD module's canonical `TDD Cycle Evidence` column shape (RED / GREEN / TRIANGULATE / SAFETY NET / REFACTOR per task row). It instead records four project-specific evidence tables: "TDD cycle evidence" (cycles 1–6), "Focused RED/GREEN cycles" (correction pass, cycles 7–14 plus cycle 14 explained as coverage-not-RED), the third-turn cycle 15 table, and the fourth-turn RED/GREEN block for the 9 Phase-9 tests (delegated implementer, parent review). This is a shape deviation, not an evidence gap: every recorded claim names its exact command and observable failure (e.g., `AttributeError: … no attribute 'collect_env_allowed'`, `InstallError not raised`, `subprocess.run called with []`), so each RED/GREEN claim is reproducible from the recorded commands. The fourth turn additionally documents a mutation check (case-sensitive comparison makes the negative test fail), which is genuine triangulation evidence. Judged verifiable.

### TDD Compliance

| Check | Result | Details |
|-------|--------|---------|
| TDD Evidence reported | ✅ | Four tables found in `apply-progress.md` (shape deviation recorded above; content verifiable) |
| All tasks have tests | ✅ | Behavior tasks 1.1–9.4 each map to named tests; all named test files/methods verified to exist in the codebase (66 tests in `test_jinna_provider_recipe.py`, Phase-9 tests at `test_env_scaffold.py:587–735` and `test_doctor.py:1302–1337`) |
| RED confirmed (tests exist, right reason) | ✅ | RED outcomes recorded per cycle with behavior-specific failures; historical runs are not re-executable in verify, but each names the command and the exact assertion/exception — judged credible and command-reproducible |
| GREEN confirmed (tests pass now) | ✅ | Re-run by this phase: focused 12-module suite **423 tests OK**; 9 Phase-9 tests **OK**; full-suite parent evidence shows only the pre-existing bitbucket failure |
| Triangulation adequate | ✅ | Correction pass (7–14) + cycle 15 include triangulation and guard tests; the fourth turn adds a case-sensitivity mutation check proving the negative test is non-vacuous; multi-scenario behaviors (archive safety, checksum, metadata) have test matrices, not single cases |
| Safety Net for modified files | ✅ | Pre-session baseline `Ran 218 tests … OK` recorded; turn-3 baseline `Ran 36 tests … OK`; focused runs always include the neighboring existing modules (schema/read/dep/materialize/doctor/catalog) which stayed green |

**TDD Compliance**: 6/6 checks passed.

### Test Layer Distribution

| Layer | Tests | Files | Tools |
|-------|-------|-------|-------|
| Unit | 423 (this change's focused surface) | 12 modules (unittest) | `PYTHONPATH=. python3 -m unittest` (project runner `./tests/run.sh` in full mode) |
| Integration-style | included above (on-disk render/materialization/doctor runs against temp projects, e.g. `test_recipe_materialize.py`, `HarnessEnvDoctorTests`, real-catalog env-example test) | — | `unittest` + `tempfile` (no separate integration tooling configured) |
| E2E | 0 automated | — | not configured; live provider verification performed out-of-suite (parent-recorded) |
| Opt-in smoke | 2 (skipped without `AI_SPECS_JINNA_SMOKE_BINARY`) | `tests/test_jinna_provider_recipe.py::ProviderReleaseSmokeTests` | `unittest` |

Suite-wide full run: 1900 tests (parent-collected). Critical installer/materialization logic is unit-covered; the live E2E gap is covered by the parent-recorded provider run rather than automated E2E tooling (SUGGESTION-level note only).

### Changed File Coverage

Coverage analysis skipped — no coverage tool detected (`config.yaml`: `coverage.available: false`). Per-file test mapping instead (never reported as a failure):

| File | Covered by | Rating |
|------|-----------|--------|
| `lib/_internal/provider_install.py` (new, +597) | `ProviderReleaseTests`, `ProviderInstallerHardeningTests`, `ProviderReleaseIntegrityTests`, `ProviderMemberLimitTests`, `ProviderReleaseSmokeTests` (66-test suite) | ✅ thorough incl. failure paths |
| `lib/_internal/recipe-materialize.py` (+78/−17) | `McpMarkerTests`, `ProviderRuntimeRenderingTests`, `test_recipe_materialize.py` | ✅ |
| `lib/_internal/recipe_schema.py` / `recipe-read.py` | `ProviderDependencySchemaTests`, `test_recipe_schema.py`, `test_recipe_read.py` | ✅ |
| `lib/_internal/dep_check.py` / `dep_install.py` | `ProviderDependencyCheckTests`, `ProviderConsentPreviewTests`, `test_dep_check.py`, `DepInstallTests` | ✅ |
| `lib/_internal/config_wizard.py` | `test_config_wizard.py` (+106, incl. non-TTY provider safety) | ✅ |
| `lib/_internal/env_scaffold.py` (+65) | Phase-9 tests (7) + pre-existing 36-test module suite + `test_sync_env_scaffold.py` (black-box) | ✅ |
| `lib/_internal/doctor.py` (+55 test lines) | `HarnessEnvDoctorTests` + full `test_doctor.py` module | ✅ |
| `catalog/recipes/jinna-mcp-recipe/*` (new) | `CatalogRecipeTests`, docs-contract tests (`:249`, `:257`) | ✅ |
| `docs/recipe-schema.md`, `docs/recipes-catalog.md` | docs-contract tests (tasks 7.1/7.2) | ✅ |
| `openspec/changes/jinna-mcp-recipe/**` | planning artifacts — not testable code | — |

### Assertion Quality

| File | Line | Assertion | Issue | Severity |
|------|------|-----------|-------|----------|
| `tests/test_env_scaffold.py` | 674 | `test_select_default_is_always_one_of_the_choices` | Would also pass if `_select_default` returned `choices[0]` unconditionally — the preference order (existing value → example default → first choice) is pinned separately by `test_prompt_env_vars_select_default_prefers_existing_value` (`:706`) | WARNING (accepted — paired) |
| `tests/test_doctor.py` | 1326 | `test_valid_harness_env_value_case_insensitive_no_warn` | Would also pass if the whole `harness-env-value` check were deleted — only meaningful paired with the invalid-value test (`:1302`); the recorded mutation check (case-sensitive impl) makes it fail, proving the pair pins real behavior | WARNING (accepted — paired) |

**Assertion quality**: 0 CRITICAL, 2 WARNING (both documented caveats, both paired/triangulated; no tautologies, ghost loops, empty-collection orphans, smoke-only tests, or mock-heavy tests found; the audited Phase-9 tests assert real behavior — closed-choice prompting, fail-open parsing, warning content without value echo). This phase independently re-ran all 9; the independent meaning check's "all 9 non-vacuous" conclusion is confirmed for the pinning pairs.

### Quality Metrics

**Linter**: ➖ Not available (`config.yaml`: `quality.linter.available: false`). **Type Checker**: ➖ Not available. **Formatter**: ➖ Not configured (the Go gate's `gofmt -l` was recorded clean in apply-progress; Python formatting tooling does not exist here). **Coverage**: ➖ Not available (`coverage.available: false`; `coverage_threshold: 0`).

These are unavailable signals, explicitly not failures — per `config.yaml` verify rules and the testing-foundation skill, no implication of passing is made.

Build/validation stages run by this phase: `python3 -m py_compile lib/_internal/*.py tests/*.py` → OK (exit 0); `bash -n lib/*.sh bin/ai-specs tests/*.sh` → OK (exit 0). These correspond to the non-`run.sh` stages of `./tests/validate.sh`.

## Task completion status

40/42 tasks complete (`0.1`–`8.4` plus all of Phase 9). Unchecked implementation task scan (`^\s*- \[ \]` and heading-checkbox forms) finds exactly two unchecked lines in `tasks.md`:

```text
292:### [ ] 8.5 Prepare review handoff
298:### [ ] 8.6 Archive before merge
```

Neither is an incomplete implementation task:

- **8.5** is this deliverable — the `Review Workload Forecast`-gated review handoff and `verify-report.md` required by task 8.5 are satisfied by this file. Its checkbox remains unticked only because verify is report-only (the allowed edit surface here is `verify-report.md` alone); the parent should tick `8.5` when accepting this report.
- **8.6** is archive-tail by design: it depends on `8.5` and explicit delivery approval, and the workflow mandates archive *after* verify evidence and *before* merge. It cannot be complete during the verify phase without violating the ordering rule, so it is not a completeness defect.

No `- [ ]` implementation task remains; archive readiness is therefore not blocked by task state (only by the delivery decision that task 8.6 itself requires).

## Structured status and action context findings

- Status was resolved via the **manual fallback** declared by the parent: the native `gentle-ai` CLI is not installed; the status contract permits this fallback. `artifactStore: openspec`, `applyState: ready`, `dependencies: {apply: ready, verify: ready, sync: blocked, archive: blocked}` — consistent with the artifacts on disk (verify report was the missing piece; `sync`/`archive` unlock in order after this phase).
- Schema warning acknowledged: `8.5`/`8.6` have no `sdd-owner` markers so the schema counts them as implementation work; they are verify/archive-owned process rows (see above). Recorded as the reason the raw `remaining: 2` is not treated as implementation incompleteness.
- `actionContext.mode: repo-local`; `allowedEditRoots: […/.worktrees/jinna-mcp-recipe/openspec/changes/jinna-mcp-recipe]` — honored exactly: this phase wrote only `verify-report.md`. `git status --short` before and after verification shows the same 11 expected modified files and no untracked additions.
- Tracker: `proposal.md` `## Tracker` records `card_id: 6aa21ffc2e72c0ce4b22a75b` with url and list (defect B from the fourth turn — formatting that defeated `parse_tracker_section` — was fixed in the change).

## Verification commands (exact)

Re-run by this verify phase (cheap, focused — full suites deliberately NOT re-run per operator instruction):

| Command | Result | Output hash (sha256) |
|---|---|---|
| `PYTHONPATH=. python3 -m unittest tests.test_jinna_provider_recipe tests.test_recipe_schema tests.test_recipe_read tests.test_dep_check tests.test_config_wizard tests.test_recipe_materialize tests.test_recipes_catalog tests.test_recipe_add tests.test_recipe_configure tests.test_doctor tests.test_recipe_list tests.test_recipe_init` | `Ran 423 tests in 32.100s` — `OK (skipped=2)`, exit 0 | `87fafd202ee8c13af97e968631267c693a19452d20478842db1dfd0334052c5d` |
| 9 named Phase-9 tests (`EnvScaffoldTests.test_collect_env_allowed_*` ×4, `test_prompt_env_vars_uses_select_for_declared_allowed_values`, `test_select_default_is_always_one_of_the_choices`, `test_prompt_env_vars_select_default_prefers_existing_value`, `HarnessEnvDoctorTests.test_invalid_harness_env_value_warns_with_allowed_values`, `test_valid_harness_env_value_case_insensitive_no_warn`) | `Ran 9 tests in 0.190s` — `OK`, exit 0 (matches parent's `0.194s`) | `d75cf5164385228bf47a286060b88fe69980a7d8652f457c75d9947ba16b6f7d` |
| `PYTHONPATH=. python3 -m unittest tests.test_bitbucket_pr_flow_recipe.BitbucketPrFlowGoldenContentTests.test_apply_progress_omits_absolute_host_and_worktree_paths` | exit 1 — expected pre-existing failure (`missing …/openspec/changes/bitbucket-bb-cli-alignment/apply-progress.md`) | (failure output, see Known limitations) |
| `python3 -m py_compile lib/_internal/*.py tests/*.py` then `bash -n lib/*.sh bin/ai-specs tests/*.sh` | `py_compile OK`; `bash -n OK` (exit 0) | `71936118d07c56a7c62db4b48be83f9b7821ba0dea88e781ac7f02c77aa4738a` |

Parent-collected (not re-run here — ~9-minute suites):

| Command | Result |
|---|---|
| `./tests/validate.sh` | exit 1 — `Ran 1900 tests in 541.310s — FAILED (failures=1, skipped=135)`; single failure = pre-existing bitbucket golden test |
| `./tests/run.sh` | exit 1 — `Ran 1900 tests in 528.242s — FAILED (failures=1, skipped=135)` (full unittest discovery; both runners share the one failure) |

## Live provider evidence

**Re-verified by this phase (read-only; no execution in the live worktree):**

- Installed binary exists at `.worktrees/jinna-mcp-live/cache/bin/jinna/v0.1.0/darwin-arm64/jinna` with the expected release layout (`LICENSE`, `README.md`-side docs, `THIRD_PARTY_NOTICES.md`, `docs/{mcp,migration,operations}.md`).
- Its sha256 is `2a94f2e7005686e23c68da0bd5a912db4432230fd9f77b1c4f8874d34f00e7a5`, equal to the receipt's `binary_sha256` (`install.json`: `status: verified`, `release_tag: v0.1.0`, `target: darwin-arm64`) and equal to the binary digest recorded from the apply-phase isolated install in `apply-progress.md` — receipt ↔ installed-bytes integrity chain confirmed.
- `tests/test_bitbucket_pr_flow_recipe.py` last touched at `cf09e1d` and byte-identical to `development` (empty diff); `bitbucket-bb-cli-alignment` is archived in both this worktree and the main checkout (`openspec/changes/archive/2026-09-07-bitbucket-bb-cli-alignment/`, no active folder in either).

**Parent-recorded (attributed as such; not re-executed by this phase):**

- Release `v0.1.0` of `parada1104/jinna-provider` validated: installed binary sha256 equals the published `SHA256SUMS` entry for `jinna_v0.1.0_darwin_arm64.tar.gz`; `RELEASE.json` version/commit match `jinna version` (`v0.1.0`, commit `4dc0182…`, built 2026-09-09); binary links only system libraries (no Go/Mise/Python/Git needed).
- `jinna health` and `jinna whoami` authenticated against the real self-hosted OpenProject instance.
- MCP stdio handshake: `serverInfo {name: jinna-provider, version: v0.1.0}`, protocol `2025-06-18`, capabilities `tools`; `tools/list` → 32 tools (25 `readOnlyHint`; the 7 without it are `work_package_create`, `work_package_form_create`, `work_package_relations_add/remove`, `work_package_update`, `work_package_watchers_add/remove`); `tools/call projects_list` → `isError: false` with live data.
- `ai-specs sync` materialized `jinna` in `.mcp.json` / `opencode.json` / `.cursor/mcp.json` with `{dep:jinna}` resolved to the managed absolute path and env as `${OPENPROJECT_*}` references only; `doctor` → `OK recipe-dep jinna available for jinna-mcp-recipe`; `ai-specs.env.example` renders `OPENPROJECT_AUTH=basic` with help text.

## Review workload / PR boundary

- `tasks.md` contains the **Review Workload Forecast**: estimated ~5,100 authored changed lines; **chained PRs recommended: Yes**; suggested split = the 7 reviewable work units; **delivery strategy `ask-on-risk` — decision pending**; chain strategy deferred. (Note: an early apply-progress line claiming no forecast block existed is stale relative to the current `tasks.md`; the forecast governs.)
- Measured size: merge-base `6f5bf73`→HEAD commits add 4,684 insertions / 60 deletions across 26 files; the worktree's uncommitted 11-file turn adds 521 insertions / 17 deletions — ≈5.2k changed lines total, ~13× the 400-line review budget.
- No `size:exception` was used or requested — consistent with chained-PRs-recommended; the parent owns slicing (the 7 work units in `tasks.md` are the suggested chain) and the still-pending `ask-on-risk` decision.
- Scope check: implementation stayed within the planned work units. Phase 9 (declared env validation) was a live-run-found defect explicitly authorized by the user as in-change work and recorded with its own TDD cycles — not silent scope creep. No PR boundary was claimed by this phase; no review transaction created (verify does not hold review authority).

## Delivery decision (recorded by the parent after acceptance)

- **`size:exception` accepted.** The user reviewed the review-budget risk and explicitly accepted a single
  PR of ~5,200 changed lines instead of the chained PRs recommended by the forecast. The exception
  overrides the chaining recommendation for this delivery.
- Consequence for the reviewer: one PR carries the whole change (release installer, managed-cache
  resolution, MCP materialization, catalog recipe, docs, tests, and the Phase 9 env-value validation).
  The 7 work units in `tasks.md` remain the recommended commit grouping inside that single PR.

## Known limitations

1. **Pre-existing, non-regressive full-suite failure (environmental).** `tests.test_bitbucket_pr_flow_recipe.BitbucketPrFlowGoldenContentTests.test_apply_progress_omits_absolute_host_and_worktree_paths` fails in every full-suite run: it requires an active `openspec/changes/bitbucket-bb-cli-alignment/apply-progress.md`, but that change is archived. Proven non-regressive: it fails identically on the untouched `development` main checkout at base `6f5bf73` (which contains none of this branch's commits), the test file is committed (`cf09e1d`) and unmodified by this change, and the folder is archived in both checkouts. **Follow-up (parent-owned, outside this change's edit surfaces):** make the bitbucket golden test archive-aware or restore/point it at the archived fixture, so `./tests/validate.sh` can exit 0. Until then the harness's full-suite exit 1 is an expected, documented condition, not a defect of `jinna-mcp-recipe`.
2. **Test-strength caveats (accepted, WARNING):** `test_select_default_is_always_one_of_the_choices` is meaningful only alongside the preference-order test, and `test_valid_harness_env_value_case_insensitive_no_warn` only alongside the invalid-value test (see Assertion Quality). Both pairs together pin real behavior; no rewrite is required by verify, but the pairing should be preserved in any refactor.
3. **Unavailable quality signals:** coverage, linter, type checker, and formatter are not configured in this repository (`config.yaml`). Reported as unavailable — explicitly not failures.
4. **Live runtime behaviors are parent-recorded:** this phase re-verified the artifact/hash chain read-only but did not execute `jinna health`/`whoami`, the MCP handshake, or `ai-specs sync` (prohibited: touching the live worktree / running sync). E2E-style confidence therefore rests on the parent's recorded live run plus the deterministic suite.
5. **`apply-progress.md` full-suite counts** (1851/1875 tests) predate the parent's fresh collection (1900 tests); both agree on the single pre-existing failure and 135 skips.
6. **Open checkbox:** `8.5` is satisfied by this report but still unticked in `tasks.md` (report-only phase); the parent should tick it on acceptance. `8.6` remains intentionally open until archive-tail.

## Validator admission

`gentle-ai sdd-verify-validate` was attempted before persistence and is **not installed** (`command -v gentle-ai` → not found; consistent with the parent's manual status fallback). Per the operator's explicit retry instruction, this report was written manually with the envelope self-declared above; counts were taken from the retrieved spec (`specs/jinna-mcp-recipe/spec.md`: 11 requirements, 29 scenarios). **Exact blockers for this verdict: none** — the pre-existing suite failure, the pending delivery decision, and the open archive row are recorded risks/gates, not verify blockers.


## Verify evidence

- Verdict: PASS
- Command: ./tests/validate.sh
- Exit: 0
- Date: 2026-09-11
- Commit: 376d706
- ready_for_archive: true

`./tests/validate.sh` is this repository's configured verify command (`openspec/config.yaml`). Recorded
result: `Ran 1912 tests in 606.209s — OK (skipped=135)`, the first fully green full-suite run on this
branch. Focused evidence re-run on the same revision: `tests.test_jinna_provider_recipe` 66 OK
(skipped=2), `tests.test_env_scaffold` + `tests.test_doctor` 140 OK, `tests.test_change_paths` 12 OK,
`py_compile` clean.

## Success-criteria mapping

- Criterion 1: PASS — the catalog assets validate and materialize a local provider MCP entry with no provider source, developer paths, or secret literals.
- Criterion 2: PASS — a missing or incompatible provider produces an explicit allowlisted GitHub Release offer with target selection, checksum and archive verification, and atomic publication only after consent.
- Criterion 3: PASS — a compatible `PATH` or verified managed-cache provider is reused, while unresolved and non-interactive paths stay visibly incomplete and non-mutating.
- Criterion 4: PASS — enabled runtimes receive the provider-owned `jinna mcp` command with runtime-specific translation and `$OPENPROJECT_*` environment references only.
- Criterion 5: PASS — existing recipe behavior stays compatible, with deterministic schema, installer, materialization, documentation and full-validation evidence above.
- Criterion 6: PASS — provider-backed installation verification was claimed only after the real release, its five target assets, `SHA256SUMS` and `RELEASE.json` passed the readiness gate.
