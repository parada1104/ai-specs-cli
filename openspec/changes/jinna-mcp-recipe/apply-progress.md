# Apply progress: `jinna-mcp-recipe`

## Authorization

Apply was explicitly authorized by the user on 2026-09-10. Implementation is being performed directly
in the dedicated worktree `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/ai-specs-cli/jinna-mcp-recipe`.
No worker modifies this change. No commit, push, PR, archive, or provider-repository change was performed.

## Provider readiness gate

Verified before and during production implementation:

- Repository: `https://github.com/parada1104/jinna-provider`
- Stable release: `v0.1.0`, published and not draft/prerelease.
- Assets: five target archives plus `SHA256SUMS` and `RELEASE.json`.
- Release Actions run `34383613223`: quality, race, vet, staticcheck, govulncheck, secret scan, packaging, and publish all passed.
- Remote download verification: host macOS arm64 archive downloaded, checksum validated, `jinna version` executed successfully.
- Archive contract: expected executable, docs, and `THIRD_PARTY_NOTICES.md` present.
- End-user path: release archive does not require Go, Mise, Python, or Git.

### Live release contract re-confirmed during apply

`GET /repos/parada1104/jinna-provider/releases/latest` returned `v0.1.0` (draft=false, prerelease=false)
with exactly these assets:

```text
jinna_v0.1.0_darwin_amd64.tar.gz
jinna_v0.1.0_darwin_arm64.tar.gz
jinna_v0.1.0_linux_amd64.tar.gz
jinna_v0.1.0_linux_arm64.tar.gz
jinna_v0.1.0_windows_amd64.zip
RELEASE.json
SHA256SUMS
```

Host archive `jinna_v0.1.0_darwin_arm64.tar.gz`:

- published `SHA256SUMS` digest: `af7e504a32c536239c41dfac7bfa84db37a9586c217d3cd72375aa0a2560bf86`
- root layout: `jinna_v0.1.0_darwin_arm64/{jinna,LICENSE,README.md,THIRD_PARTY_NOTICES.md,docs/{mcp,migration,operations}.md}`
- `jinna version` output: `jinna version v0.1.0` (exit 0)

The archive, `SHA256SUMS` line format, root directory, and executable name all match the frozen
contract in `design.md` D2 and the installer expectations in `provider_install.py`.

## Implementation scope

Implemented the approved `explore.md`, `proposal.md`, `design.md`, and `specs/jinna-mcp-recipe/spec.md`
through the planned work units:

1. Dependency schema and resolution result.
2. GitHub Release selection, verification, and atomic managed installation.
3. Interactive consent/configuration lifecycle.
4. Managed `{dep:jinna}` command resolution and MCP materialization.
5. Catalog assets and documentation.
6. Focused and full validation evidence.

## TDD cycle evidence

Strict TDD is active (`openspec/config.yaml` → `strict_tdd: true`, runner `./tests/run.sh`).
Focused RED/GREEN cycles were driven with the module-scoped unittest command plus `PYTHONPATH=.`.

Focused command:

```bash
PYTHONPATH=. python3 -m unittest tests.test_jinna_provider_recipe tests.test_recipe_schema \
  tests.test_recipe_read tests.test_dep_check tests.test_config_wizard \
  tests.test_recipe_materialize tests.test_recipes_catalog
```

RED baseline before this session's new tests: `Ran 218 tests ... OK`.

| Cycle | Behavior pinned | RED evidence | GREEN evidence |
|---|---|---|---|
| 1 | Provider version self-check must equal the selected release tag (not merely `>= min_version`) | `test_version_self_check_must_match_release_tag` FAIL — `InstallError not raised` (mismatched `0.9.9` binary was published) | `provider_install.install_github_release` compares `run_version(...) == expected_version`; test passes, target dir not published |
| 2 | Malformed release metadata must fail as a structured `InstallError` | `test_malformed_release_metadata_raises_install_error` ERROR — raw `json.JSONDecodeError` escaped | JSON decode wrapped in `try/except`; test passes |
| 3 | Managed receipt must be a trust anchor: allowlisted repository, exact target, tag/version agreement | `test_managed_candidate_with_inconsistent_receipt_is_rejected` FAIL — `'managed' != 'unresolved'` | `resolve_provider` validates repository/target/release_tag and tag↔version equality; test passes |
| 4 | Unsupported platform must list supported targets and manual guidance | `test_unsupported_platform_lists_targets_and_guidance` FAIL — message was only `provider has no artifact for this platform` | `build_release_plan` emits the five supported targets plus the guidance URL; test passes |
| 5 | Interactive consent must display the complete install plan | `test_github_release_preview_covers_consent_fields` / `test_guidance_plan_keeps_legacy_display` ERROR — `describe_install_plan` missing; `test_tty_offer_prints_full_plan_before_confirmation` FAIL — nothing printed | `dep_install.describe_install_plan` added and printed before the confirmation prompt; all three tests pass |
| 6 | A non-UTF-8 `SHA256SUMS` body must fail as `InstallError` | `test_non_utf8_sums_manifest_raises_install_error` ERROR — raw `UnicodeDecodeError` escaped from `.decode("utf-8")` | decode wrapped in `try/except`; test passes |

Refactor/triangulation under green (all pass immediately because the parent implementation already
contained the behavior; they are coverage, not RED cycles):

- real v0.1.0 asset-name contract for all five targets;
- missing expected target asset, non-allowlisted asset host, duplicate assets rejected;
- oversized HTTP response rejected, non-allowlisted fetch host rejected;
- receipt contains only non-secret provenance keys;
- resolution never touches the network (`_fetch_bytes` patched to raise);
- tar traversal/absolute/duplicate members, symlinks, unexpected members rejected;
- PATH candidate → bare `jinna`, managed candidate → absolute path, manifest command override preserved;
- cross-runtime rendering: OpenCode `command == [resolved, "mcp"]`, Claude/Cursor/Pi/OMP keep a concrete
  command with `${VAR}` env refs, and `{dep:jinna}` never reaches rendered output;
- `doctor`/`check_cli_deps` resolution never calls `install_github_release`;
- `_dep_gate` never offers a provider install without a TTY and forwards the validated
  `github-release` declaration on a TTY.

Final focused run:

```text
Ran 247 tests in 1.131s
OK (skipped=2)
```

The two skips are the opt-in release smoke tests, which require `AI_SPECS_JINNA_SMOKE_BINARY`.

## Provider-backed installer verification (task 8.3)

Executed against the real public release with the default stdlib HTTPS fetcher, installing into an
isolated temporary cache outside the repository:

```text
host target: darwin arm64
source: managed verified: True version: 0.1.0 tag: v0.1.0
path: /var/folders/.../jinna-e2e-p1z2dhdc/cache/v0.1.0/darwin-arm64/jinna is_file: True
receipt keys: ['archive', 'archive_sha256', 'binary_sha256', 'release_tag', 'repository', 'status', 'target']
archive_sha256: af7e504a32c536239c41dfac7bfa84db37a9586c217d3cd72375aa0a2560bf86
cache dirs: ['v0.1.0']
```

Opt-in smoke with the extracted released binary:

```bash
AI_SPECS_JINNA_SMOKE_BINARY=/tmp/.../jinna_v0.1.0_darwin_arm64/jinna \
  PYTHONPATH=. python3 -m unittest tests.test_jinna_provider_recipe.ProviderReleaseSmokeTests
# Ran 2 tests ... OK
```

## Full validation evidence (task 8.2)

`./tests/run.sh` (bounded full run):

```text
ok - All vault-fs-mcp.sh checks passed.
run.sh: go found — running Go gate tests
ok  	ai-specs.dev/worktree-gate	(cached)
Ran 1851 tests in 554.850s
FAILED (failures=1, skipped=135)
exit=1   (wall clock 9m16.053s)
```

The single failure is **pre-existing and unrelated**:

```text
FAIL: test_apply_progress_omits_absolute_host_and_worktree_paths
      (test_bitbucket_pr_flow_recipe.BitbucketPrFlowGoldenContentTests)
AssertionError: False is not true : missing
  <worktree>/openspec/changes/bitbucket-bb-cli-alignment/apply-progress.md
```

Diagnosis:

- `tests/test_bitbucket_pr_flow_recipe.py` is unmodified by this change (`git status` shows no diff).
- The `bitbucket-bb-cli-alignment` change is archived at
  `openspec/changes/archive/2026-09-07-bitbucket-bb-cli-alignment/`; no active change folder exists in
  any worktree (main worktree `openspec/changes/` contains only `archive`).
- The same test fails identically on the `development` main worktree, so it is a stale fixture test in
  the harness, not a regression from `jinna-mcp-recipe`.
- Fixing it is outside this change's allowed edit surfaces and is not attempted here.

Additional configured checks (`./tests/validate.sh` non-`run.sh` stages) were executed separately:

```text
python3 -m py_compile lib/_internal/*.py tests/*.py   → py_compile OK
bash -n lib/*.sh bin/ai-specs tests/*.sh              → bash -n OK
gofmt -l catalog/recipes/worktree-flow/gate           → clean (no output)
```

The `run.sh` stage of `./tests/validate.sh` is exactly the full run recorded above; the verify phase
owns the final composite `./tests/validate.sh` execution.

## Spec scenario comparison (task 8.4)

| Requirement | Evidence |
|---|---|
| R1 valid provider dependency / legacy compatibility / invalid declarations | `ProviderDependencySchemaTests`, `CatalogRecipeTests`; schema rejects unknown installer, non-allowlisted repository, and release fields without `github-release` |
| R2 PATH reuse / missing / incompatible preserved | `test_path_provider_marker_resolves_to_bare_command`, `test_managed_candidate_with_inconsistent_receipt_is_rejected`, `resolve_provider` never overwrites, `test_resolution_never_touches_the_network` |
| R3 explicit offer / decline / non-TTY | `ProviderConsentPreviewTests`, `test_non_tty_never_executes_github_install`, `test_dep_gate_non_tty_never_offers_provider_install` |
| R4 target selection / unsupported / checksum mismatch | `test_supported_platform_mapping`, `test_unsupported_platform_lists_targets_and_guidance`, `test_checksum_mismatch_does_not_publish_target` |
| R5 archive safety / atomic publication / prior version survives | traversal, symlink, duplicate, unexpected-member, corrupt/unknown-asset tests; `install.json` written post-verification, `os.replace` publication |
| R6 marker resolution / omission / runtime translation | `McpMarkerTests`, `ProviderRuntimeRenderingTests` |
| R7 env references only / install without OpenProject credentials | recipe `env` table contains only `$OPENPROJECT_*` references; real install succeeded with no OpenProject env |
| R8 local `jinna mcp` only, no `/mcp` fallback or write replay | recipe declares one local MCP preset; no hook, fallback, or replay path exists in the change |
| R9 compatibility / recovery docs | catalog README/init/skill document install, platforms, checksums, managed path, version checks, troubleshooting, rollback; existing suites green |
| R10 deterministic suite + readiness gate | focused suites green; full-suite result and the pre-existing failure are reported above |

Unresolved provider readiness or security issues: none. The remaining unchecked tasks are `8.5`
(verify-report handoff) and `8.6` (archive before merge), which belong to the verify/delivery phases
and were explicitly out of scope for this apply turn.

## Files changed

Modified (tracked):

- `lib/_internal/recipe_schema.py` — `CliDep` gains `installer`/`repository`/`release_policy` with enum and allowlist validation.
- `lib/_internal/recipe-read.py` — round-trips the new dependency fields, omitting empty values.
- `lib/_internal/dep_check.py` — `DepResult.source`/`resolved_path`; `github-release` resolution via the provider module; `check_cli_deps`/`check_project_deps` accept `ai_specs_home`.
- `lib/_internal/dep_install.py` — `github-release` plan kind, `describe_install_plan` consent preview, TTY-only execution.
- `lib/_internal/config_wizard.py` — dependency gate forwards the validated provider declaration and rechecks after install.
- `lib/_internal/doctor.py` — passes `AI_SPECS_HOME` to the dependency check without mutation.
- `lib/_internal/recipe-materialize.py` — `{dep:jinna}` resolution before writing recipe MCP JSON; unresolved entries omitted with one warning.
- `tests/test_jinna_provider_recipe.py` — installer hardening, consent preview, runtime rendering, and opt-in smoke tests.
- `tests/test_config_wizard.py` — non-TTY provider safety and TTY declaration-forwarding tests.
- `tests/test_dep_check.py` — non-downloading provider resolution tests.
- `tests/test_recipes_catalog.py` — catalog/schema doc contract update.
- `docs/recipe-schema.md`, `docs/recipes-catalog.md` — `github-release` installer fields, allowlist, non-TTY/consent semantics.

Added (untracked):

- `lib/_internal/provider_install.py` — allowlisted GitHub Release acquisition, checksum/archive verification, self-check, managed cache + receipt, atomic publication, PATH/managed resolution.
- `catalog/recipes/jinna-mcp-recipe/{recipe.toml,init.md,README.md,skills/jinna-mcp-recipe/SKILL.md}`.
- `tests/test_jinna_provider_recipe.py` provider contract suite.
- `openspec/changes/jinna-mcp-recipe/**` planning artifacts (this update).

## Persisted task checkbox updates

`openspec/changes/jinna-mcp-recipe/tasks.md` now marks `0.1`–`8.4` as `- [x]`.
Remaining unchecked lines (verify/delivery phases):

```text
### [ ] 8.5 Prepare review handoff
### [ ] 8.6 Archive before merge
```

## Deviations from design

- `preview_install(plan)` from design D5 is implemented as the pure `describe_install_plan(plan)` helper
  in `dep_install.py`, which returns text instead of printing; `offer_and_install` prints it before the
  confirmation prompt. The four-seam separation (resolve → preview → consent → install) is preserved.
- The installer additionally requires the self-checked version to equal the selected release tag, which
  tightens the design's "require the reported version to equal the selected release version".
- Managed reuse additionally requires the receipt's repository, target, and release tag to be consistent
  with the executable version before reuse.

## Workload / PR boundary

- No `Review Workload Forecast` guard block exists in `tasks.md`, so no delivery decision was required.
- Authored size is well above the 400-line review budget because the change adds a new installer module,
  a new catalog recipe, and the security/consent/materialization tests that pin it. The natural review
  slices are the six work units listed under "Implementation scope"; the parent owns PR slicing,
  commits, and delivery.
- The budget did not influence implementation: no comments, tests, docs, or formatting were removed or
  compressed to fit.

## Structured status / action context

- `actionContext.mode: repo-local`, `allowedEditRoots: [/Users/robert/proyectos/nnodes/ai-specs-cli]`;
  all edits stayed inside the allowed surfaces and the change worktree.
- No `blockedReasons` from the authoritative store blocked this phase; the provider readiness gate was
  resolved PASS before implementation.

## Verifier correction pass (second apply turn)

An independent verifier reviewed the first pass and raised seven findings. All seven were addressed
with focused RED → GREEN cycles, and no prior work was reset. The two unchecked tasks (`8.5`, `8.6`)
remain verify/delivery-owned. The unrelated stale Bitbucket test was not touched.

### Focused RED/GREEN cycles

| Cycle | Correction | RED evidence | GREEN evidence |
|---|---|---|---|
| 7 | `MAX_MEMBER_BYTES` raised from 8 MiB to a justified 32 MiB (released `jinna` is 7,907,794 bytes) | `test_member_limit_is_documented_and_above_the_released_binary` FAIL — 8 MiB < 32 MiB | constant raised with an explanatory comment; `test_member_limit_boundary_is_enforced` (limit accepted, limit+1 rejected) passes |
| 8 | `parse_release` must reject prerelease-suffixed tags even when the `prerelease` flag is false | `test_prerelease_suffix_tag_is_rejected_without_the_flag` FAIL — `v0.2.0-rc.1` accepted | tag regex tightened to `v<digits>.<digits>...`; test passes |
| 9 | Installer must require and validate the `RELEASE.json` asset (bounded, secret-safe) | 6 FAILs — missing asset, version mismatch, missing artifact, duplicate artifact, malformed/non-object body, oversized body all installed successfully | `parse_release_metadata` plus bounded fetch/validate added; all six tests pass |
| 10 | Managed-cache `OSError` must degrade to unresolved instead of aborting | 2 ERRORs — unreadable binary (`_sha256` OSError) and unreadable cache root (`_managed_candidates` OSError) escaped | `_managed_candidates` guards `iterdir()`/`is_file()`; `resolve_provider` catches `(InstallError, OSError)` and wraps the candidate scan; both tests pass |
| 11 | Materialization must degrade to one warning, not abort sync | `test_provider_resolution_error_degrades_to_omitted_entry` ERROR — `OSError` propagated out of `build_recipe_mcp` | `_resolve_provider_markers` wraps recipe read and `resolve_provider` in `try/except`, omits the entry, warns once; test passes |
| 12 | `offer_and_install` must not run a malformed empty-command plan through `subprocess` | `test_empty_command_plan_never_calls_subprocess` FAIL — `subprocess.run` called with `[]` | defensive `plan.kind != "github-release" and not plan.command` guard restored; GitHub-release plans still execute via the installer; test passes |
| 13 | Docs must match the real tag-keyed cache and rollback behavior | `test_design_documents_tag_keyed_cache_layout` and `test_recipe_readme_rollback_matches_the_managed_layout` FAIL | `design.md` documents `<ai-specs-home>/cache/bin/jinna/<release-tag>/<goos>-<goarch>/`; README replaces the non-existent "select a previous version" promise with explicit reacquire/remove instructions; tests pass |

Cycle 14 is genuine verification coverage rather than a RED cycle, because the staging/`os.replace`
publication was already correct:

- `test_prior_managed_candidate_survives_every_injected_failure` seeds a verified `v0.0.9` managed
  candidate, then injects six independent failures while installing `v0.1.0` — metadata download,
  sums download, archive download, checksum mismatch, unsafe traversal archive, and version
  self-check mismatch. Each subtest asserts the new `InstallError`, byte-identical prior binary and
  receipt, no published target directory, no leftover `.jinna-*` staging directory, and that
  `resolve_provider` still returns the prior candidate as `managed`/verified.

### Genuine failure-path test inventory (finding 1)

Added meaningful, non-happy-path coverage with accurate names:

- draft rejection, prerelease-flag rejection, prerelease-suffix rejection, wrong-repository rejection;
- malformed metadata matrix (non-object, missing tag, non-list assets, non-object asset, asset without URL);
- network failure (`URLError`) and timeout (`TimeoutError`) both surface bounded `InstallError`s with no
  credential material in the message;
- SHA256SUMS malformed / short-digest / missing-entry / duplicate-entry rejection;
- checksum mismatch that proves the binary is never executed (`run_version` never called);
- atomic preservation across all injected failures (above).

### Mid-cycle correction

Two tests initially passed for the wrong reason: they omitted the injected `run_version`, so the real
`_run_version` failed on the fixture binary and `assertRaises` was satisfied by an unrelated error.
They were corrected to inject `run_version` so the assertion pins `RELEASE.json` handling, and the
final RED run reports the dependency-contract failures explicitly.

### Final focused evidence

```text
PYTHONPATH=. python3 -m unittest tests.test_jinna_provider_recipe
Ran 66 tests in 2.681s
OK (skipped=2)

PYTHONPATH=. python3 -m unittest tests.test_jinna_provider_recipe tests.test_recipe_schema \
  tests.test_recipe_read tests.test_dep_check tests.test_config_wizard tests.test_recipe_materialize \
  tests.test_recipes_catalog tests.test_recipe_add tests.test_recipe_configure tests.test_doctor \
  tests.test_recipe_list tests.test_recipe_init
Ran 416 tests in 33.033s
OK (skipped=2)
```

These counts supersede the earlier 247-test focused run; the earlier run remains valid history.

### Final full validation evidence

`./tests/validate.sh` (bounded run, 9m26.888s wall clock):

```text
validate.sh: go found — checking Go formatting (gofmt -l)     # clean, no output
All vault-fs-mcp.sh checks passed.
run.sh: go found — running Go gate tests
ok  	ai-specs.dev/worktree-gate	(cached)
Ran 1875 tests in 565.120s
FAILED (failures=1, skipped=135)
exit=1
```

The single failure is the **same pre-existing, unrelated** stale Bitbucket fixture test reported
earlier (`test_apply_progress_omits_absolute_host_and_worktree_paths` expects an active
`openspec/changes/bitbucket-bb-cli-alignment/` folder that is archived). It was not modified or
"fixed" in this change, and it reproduces on the `development` main worktree.

### Real provider-backed re-verification

The real `v0.1.0` install was re-run after adding the `RELEASE.json` requirement, into an isolated
temporary cache outside the repository:

```text
source: managed verified: True version: 0.1.0 tag: v0.1.0
receipt: {"status":"verified","repository":"parada1104/jinna-provider","release_tag":"v0.1.0",
          "target":"darwin-arm64","archive":"jinna_v0.1.0_darwin_arm64.tar.gz",
          "archive_sha256":"af7e504a32c536239c41dfac7bfa84db37a9586c217d3cd72375aa0a2560bf86",
          "binary_sha256":"2a94f2e7005686e23c68da0bd5a912db4432230fd9f77b1c4f8874d34f00e7a5"}
```

The published `RELEASE.json` (`version: v0.1.0`, five artifacts) satisfies the new contract validation.

### Correction-pass files changed

- `lib/_internal/provider_install.py` — 32 MiB member cap with rationale comment, `MAX_METADATA_BYTES`,
  `parse_release_metadata`, strict stable tag regex, required `RELEASE.json` fetch/validation,
  `OSError`-tolerant managed-candidate scanning and reuse.
- `lib/_internal/dep_install.py` — restored empty-command guard for non-`github-release` plans.
- `lib/_internal/recipe-materialize.py` — provider resolution degrades to omitted entry + one warning.
- `catalog/recipes/jinna-mcp-recipe/README.md` — tag-keyed cache layout and accurate rollback wording.
- `openspec/changes/jinna-mcp-recipe/design.md` — tag-keyed managed layout, no-implicit-version-selection
  statement, `RELEASE.json` contract, 32 MiB member cap and metadata bounds.
- `tests/test_jinna_provider_recipe.py` — 20 new tests (integrity, atomicity, member limit, docs
  contracts) plus fixture updates.

### Delivery state

No commit, push, PR, or archive was performed in this correction pass either. `tasks.md` remains
36 checked (`0.1`–`8.4`) with `8.5` and `8.6` intentionally unchecked for the verify/delivery phases.

## Post-Apply correction — `OPENPROJECT_AUTH` example default (third apply turn)

This is a **post-Apply example-rendering correction**, not new change scope. The user requested that the
generated `ai-specs.env.example` show the provider's effective default
`OPENPROJECT_AUTH=basic` instead of a blank value, after the first dogfood `sync` left the line empty.

### Scope and authorization

- Authorized surfaces only: `lib/_internal/env_scaffold.py`, `tests/test_env_scaffold.py`,
  `openspec/changes/jinna-mcp-recipe/apply-progress.md`.
- No runtime MCP env reference changed: `catalog/recipes/jinna-mcp-recipe/recipe.toml` still declares
  `OPENPROJECT_AUTH = "$OPENPROJECT_AUTH"`, and `collect_env_vars` / `missing_required_values` semantics
  are untouched, so the variable stays a required runtime reference.
- No provider code was modified (`jinna-provider` untouched); the default value is documented in the
  recipe's own `README.md` ("Optional: basic is the provider default") and `init.md`.
- `ai-specs.env` was neither read nor modified. Root `ai-specs.env.example` (968 bytes, mtime
  10:07:41) and `ai-specs.env.example.bak` (787 bytes, mtime 10:07:41) are unchanged; both predate this
  turn's edits (10:44), and no comment rendering was changed under the managed block.
- No commit, push, PR, archive, or destructive cleanup was performed.

### TDD cycle evidence (strict TDD active)

| Cycle | Behavior pinned | RED evidence | GREEN evidence |
|---|---|---|---|
| 15 | `generate_env_example()` renders the provider's effective default `OPENPROJECT_AUTH=basic` while `OPENPROJECT_BASE_URL` / `OPENPROJECT_API_TOKEN` stay blank placeholders | `test_generate_env_example_renders_openproject_auth_provider_default` FAIL — `Regex didn't match: '(?m)^OPENPROJECT_AUTH=basic\s+#'`, actual line was `OPENPROJECT_AUTH=  # required by jinna (jinna-mcp-recipe)` | `ENV_VAR_HELP["OPENPROJECT_AUTH"]` note + `ENV_EXAMPLE_DEFAULTS = {"OPENPROJECT_AUTH": "basic"}` applied in the render loop; test passes |

Triangulation and guard coverage added under green:

- `test_generate_env_example_matches_real_catalog_jinna_recipe` — renders through the **real**
  `catalog/recipes/jinna-mcp-recipe/recipe.toml` with `AI_SPECS_HOME=<repo root>` and still produces the
  `OPENPROJECT_AUTH=basic` line (different inputs than the synthetic fixture).
- `test_generate_env_example_default_is_example_only_and_idempotent` — the prefilled example value does
  not change `missing_required_values` (runtime env still required), creates no `ai-specs.env`, and two
  consecutive renders create no `ai-specs.env.example.bak`.
- Existing backup and idempotence tests (`test_generate_env_example`, `test_generate_env_example_backup`,
  `test_generate_env_example_skips_identical_rewrite`) and the black-box `test_sync_env_scaffold.py`
  suite remain green, so the `.bak`/no-rewrite behavior is unchanged.

### Exact test commands and results

Baseline before the new tests: `Ran 36 tests in 0.080s / OK`.

```text
PYTHONPATH=. python3 -m unittest \
  tests.test_env_scaffold.EnvScaffoldTests.test_generate_env_example_renders_openproject_auth_provider_default \
  tests.test_env_scaffold.EnvScaffoldTests.test_generate_env_example_default_is_example_only_and_idempotent
# RED: Ran 2 tests in 0.017s — FAILED (failures=1)  [the render test, right reason]
# GREEN: Ran 2 tests in 0.021s — OK

PYTHONPATH=. python3 -m unittest tests.test_env_scaffold
# Ran 39 tests in 0.094s — OK

PYTHONPATH=. python3 -m unittest tests.test_env_scaffold tests.test_sync_env_scaffold \
  tests.test_envrc_scaffold tests.test_config_wizard tests.test_jinna_provider_recipe
# Ran 140 tests in 15.492s — OK (skipped=2)

PYTHONPATH=. python3 -m unittest tests.test_env_scaffold tests.test_envrc_scaffold \
  tests.test_sync_env_scaffold tests.test_recipe_schema tests.test_recipe_read tests.test_dep_check \
  tests.test_config_wizard tests.test_recipe_materialize tests.test_recipes_catalog \
  tests.test_recipe_add tests.test_recipe_configure tests.test_doctor tests.test_recipe_list \
  tests.test_recipe_init
# Ran 405 tests in 48.002s — OK (skipped=2 in the jinna suite folded above)

python3 -m py_compile lib/_internal/env_scaffold.py tests/test_env_scaffold.py  # py_compile OK
bash -n lib/sync.sh                                                          # bash -n OK
```

The two skips are the opt-in release smoke tests that require `AI_SPECS_JINNA_SMOKE_BINARY`.
`./tests/validate.sh` is not re-run in this turn; its `run.sh` stage is the ~9.5-minute full suite and
belongs to the verify phase, whose prior run is recorded above.

### Expected rendered output (real project manifest, temp project)

Rendering the real `ai-specs/ai-specs.toml` against the real catalog in a temporary project produced:

```text
OPENPROJECT_API_TOKEN=  # required by jinna (jinna-mcp-recipe)
OPENPROJECT_AUTH=basic  # required by jinna (jinna-mcp-recipe) Optional — `basic` is the provider's effective default when unset; use `bearer` for Bearer tokens
OPENPROJECT_BASE_URL=  # required by jinna (jinna-mcp-recipe)
```

The probe wrote only inside a `TemporaryDirectory`; `runtime env file created: False` and
`backup created: False`.

### Correction-pass files changed

- `lib/_internal/env_scaffold.py` — added `ENV_EXAMPLE_DEFAULTS` (example-rendering-only mapping) and a
  `ENV_VAR_HELP["OPENPROJECT_AUTH"]` note; `generate_env_example()` now renders
  `f"{var}={default}  # ..."`.
- `tests/test_env_scaffold.py` — `_jinna_toml()` fixture plus three new tests.

Diff size: 78 insertions / 1 deletion across the two files, inside the 400-line review budget.

### Persisted task checkboxes

`openspec/changes/jinna-mcp-recipe/tasks.md` is unchanged by this turn: the correction is not a new
implementation-owned checklist row, and `8.5` / `8.6` remain intentionally unchecked for the
verify/delivery phases. No completed task is left unmarked.

### Remaining action for the parent (not performed here)

Re-running `ai-specs sync <project>` regenerates root `ai-specs.env.example` with the new default line;
that write is outside this turn's allowed edit surfaces, and it would replace the current uncommitted
`ai-specs.env.example.bak`, so it was deliberately not executed.
