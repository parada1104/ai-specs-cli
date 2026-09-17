# Implementation Tasks: Bitbucket `bb-cli` Alignment

Depth: standard
Requested depth: Standard. Signal depth: Standard. Decided depth: Standard.
Hardening stays in this change folder (no second change).

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | 708 tracked lines plus OpenSpec artifacts |
| 1200-line budget risk | Low |
| Chained PRs recommended | No |
| Suggested split | Single PR: dependency mapping → recipe tests/surfaces → verification and validation |
| Delivery strategy | ask-on-risk |
| Chain strategy | not applicable |

Decision needed before apply: No
Chained PRs recommended: No
Chain strategy: not applicable
1200-line budget risk: Low

## Frozen Contract

**Frozen — no task may touch:**

- `openspec/specs/**` canonical specs (deltas merge at archive).
- `lib/_internal/agents-render.py` `_VCS_RECIPE_LABELS`.
- `lib/_internal/doctor.py` behavior.
- This repository's `ai-specs/ai-specs.toml` and `.ai-specs.lock` (no Bitbucket enablement).
- GitHub/GitLab recipes.

## Ordered Implementation Tasks

### 1. Dependency install mapping — RED

- [x] **1.1** Add regression tests in `tests/test_env_scaffold.py` replacing `test_bb_guidance_only` with explicit `bb` Homebrew and apt-only cases: mock Darwin/Linux resolver discovery, assert `brew install bb-cli` when Homebrew is present, assert an empty command and recipe `install_url` guidance when only apt is present, and assert no `brew install bb` string or command. Run `python3 -m unittest tests.test_env_scaffold.DepInstallTests`; record the expected RED failures before production changes. <!-- sdd-owner: implementation -->

### 2. Dependency install mapping — GREEN

- [x] **2.1** Update `lib/_internal/dep_install.py` minimally: remove `bb` from `_GUIDANCE_ONLY`, add `"bb": ("bb-cli", "")` to `_PACKAGE_MAP`, and preserve `npx` guidance-only plus all TTY, non-TTY, and doctor behavior. Re-run `python3 -m unittest tests.test_env_scaffold.DepInstallTests` and confirm the new `bb` tests pass without regressions. <!-- sdd-owner: implementation -->

### 3. Recipe surface contract tests — RED/GREEN test work unit

- [x] **3.1** Extend `tests/test_bitbucket_pr_flow_recipe.py` and the Bitbucket contract classes in `tests/test_recipes_catalog.py` with failing assertions for PHP identity (`https://bb-cli.github.io` or authoritative PHP install URL), `bb auth show`/`bb auth save`, `bb pr create`/`show`/`merge`, version `1.3.0`, and absence of `paulvanderlei`, `@pilatos`, `bb auth login`, and `bb pr view` across live Bitbucket surfaces. Add assertions that unverified create/show/merge flags are either absent from executable examples or explicitly labeled open verification gaps. Run `python3 -m unittest tests.test_bitbucket_pr_flow_recipe tests.test_recipes_catalog`; record RED failures. <!-- sdd-owner: implementation -->
- [x] **3.2** Make the new recipe contract tests GREEN only after the corresponding recipe and documentation rewrites in Task 4 are complete; keep each test addition and its satisfying surface rewrite in the same ordered work-unit commit, and use the focused commands above after each meaningful unit. <!-- sdd-owner: implementation -->

### 4. Rewrite PHP `bb-cli` recipe surfaces

- [x] **4.1** Rewrite `catalog/recipes/bitbucket-pr-flow/recipe.toml` to identify PHP `bb-cli`, use `https://bb-cli.github.io` (or its authoritative install page) as `install_url`, remove the paulvanderlei account-help URL, and retain binary `bb`, recipe version `1.3.0`, renderer-facing identity, and no `version_check`/`min_version`. <!-- sdd-owner: implementation -->
- [x] **4.2** Rewrite `catalog/recipes/bitbucket-pr-flow/README.md` for the PHP project, document `bb auth show` inspection and `bb auth save` remediation, update the enablement example to version `1.3.0`, explain the binary collision/PATH risk, and avoid publishing unverified flags as executable guidance. <!-- sdd-owner: implementation -->
- [x] **4.3** Rewrite `catalog/recipes/bitbucket-pr-flow/commands/bb-pr-create.md` to retain explicit user authorization, preflight ordering, dynamic push, and stop-after-create behavior while replacing login guidance with `bb auth save`; use only verified PHP options or an explicit open-gap stop note for create input semantics. Do not run `bb pr create` or use `bb pr create --help` for discovery. <!-- sdd-owner: implementation -->
- [x] **4.4** Rewrite `catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md` to use `bb auth save`/`show` and `pr show`/`create`/`merge`, preserve protected-head, approval, archive, no-auto-merge, and cleanup policy, and replace TypeScript-shaped source/destination/body, JSON/jq, squash, and close-source-branch examples with verified options or conservative open-gap guidance. <!-- sdd-owner: implementation -->
- [x] **4.5** Retarget the Bitbucket sections in `docs/recipes-catalog.md` and `docs/recipe-schema.md`: describe PHP `bb-cli`, formula `bb-cli`, version `1.3.0`, and that only `npx` remains guidance-only; remove stale TypeScript identity and `1.1.0` examples without changing unrelated catalog entries. <!-- sdd-owner: implementation -->
- [x] **4.6** Update Bitbucket-specific expectations in `tests/test_sync_pipeline.py` from fixture version `1.1.0` to `1.3.0`, and revise `tests/evals/eval_vcs_pr_flow_live.py` or its Bitbucket scenario expectations so evals require PHP verbs and do not require unsafe create/merge execution or obsolete flags. Run focused sync/catalog/recipe tests after the updates. <!-- sdd-owner: implementation -->

### 5. Upstream verification and conservative gap handling

- [x] **5.1** During apply, consult authoritative documentation at `https://bb-cli.github.io` and use only read-only probes `bb pr list` and `bb pr show` to verify create source/destination/body handling, show positional/output behavior, merge strategy/source-branch closure semantics, `bb auth save` prompts, and `bb auth show` owner output parsing. Never execute `bb pr create`, `bb pr create --help`, or `bb pr merge` as discovery. <!-- sdd-owner: implementation -->
- [x] **5.2** Record the observed PHP version/surface and evidence in the apply/verification artifact, then update the affected README, command, skill, and catalog docs only where contracts are confirmed; where a gap remains, keep it explicitly labeled as an open verification gap and instruct agents to stop rather than execute guessed flags. <!-- sdd-owner: implementation -->

### 6. Full validation and frozen-contract checks

- [x] **6.1** Run focused checks from the repository root: `python3 -m unittest tests.test_env_scaffold`, `python3 -m unittest tests.test_bitbucket_pr_flow_recipe`, `python3 -m unittest tests.test_recipes_catalog`, and the relevant `python3 -m unittest tests.test_sync_pipeline` selection; record RED/GREEN and final results in apply/verification artifacts. <!-- sdd-owner: implementation -->
- [x] **6.2** Run `./tests/run.sh` and `./tests/validate.sh` from the repository root and resolve failures without widening scope. <!-- sdd-owner: implementation -->
- [x] **6.3** Perform frozen-contract checks: confirm the dogfood manifest and `.ai-specs.lock` remain unchanged and Bitbucket-disabled, `_VCS_RECIPE_LABELS` is unchanged, `doctor.py` behavior is unchanged, and GitHub/GitLab recipe files are untouched; inspect the final diff for forbidden paulvanderlei/`@pilatos`, `bb auth login`, `bb pr view`, `brew install bb`, and stale Bitbucket `1.1.0` references on live surfaces. <!-- sdd-owner: implementation -->

### 7. Parent close-out

- [x] **7.1** Complete the parent close-out for the final candidate; ordinary repository validation and delivery policy are authoritative. <!-- sdd-owner: parent -->
- [x] **7.2** Record the residual open verification gaps and final delivery decision before apply/merge. <!-- sdd-owner: parent -->

### 8. In-place hardening (user-authorized) — planning + RED

Supersedes the earlier "no `version_check` / `min_version`" and "no identity probe" deferrals in tasks 4.1 and design Decision 4. Keep Standard depth. Do not create a second change folder. Do not touch frozen files.

- [x] **8.1** Update this change's `proposal.md`, `design.md`, `tasks.md`, and spec deltas (`vcs-pr-flow`, `recipe-cli-deps`) so the authorized hardening is in-scope: warning-wording locks, redacted `bb auth show` (Username only; reject missing/multiple; never print AppPassword), positive PHP `bb-cli` identity guard, `version_check = "bb --version"` with `min_version = "1.4.1"` distinct from recipe `1.3.0`, explicit feature remote-branch deletion with protected-head exclusions, apply-progress path hygiene, and rename of the misleading merge test. <!-- sdd-owner: implementation -->
- [x] **8.2** Add/adjust failing RED tests in `tests/test_bitbucket_pr_flow_recipe.py` and `tests/test_recipes_catalog.py` for those hardening requirements. Do not modify production recipe/docs/installer files in this slice. Do not run tests in this slice. <!-- sdd-owner: implementation -->

### 9. In-place hardening — GREEN

- [x] **9.1** Add `version_check = "bb --version"` and `min_version = "1.4.1"` to `catalog/recipes/bitbucket-pr-flow/recipe.toml` while keeping recipe version `1.3.0`. Document the host floor as distinct from the recipe version. <!-- sdd-owner: implementation -->
- [x] **9.2** Add a positive PHP `bb-cli` identity guard to the skill and command so a different `bb` on PATH is blocked with `bb-cli` / `brew install bb-cli` / https://bb-cli.github.io guidance. <!-- sdd-owner: implementation -->
- [x] **9.3** Make every agent-facing auth check capture `bb auth show`, emit only `Username`, reject missing or multiple `Username` lines, and never print `AppPassword`. <!-- sdd-owner: implementation -->
- [x] **9.4** Add explicit feature remote-branch deletion after merge (`git push $REMOTE --delete`) while preserving protected-head exclusions. <!-- sdd-owner: implementation -->
- [x] **9.5** Remove absolute host/worktree paths from `apply-progress.md` wording. <!-- sdd-owner: implementation -->
- [x] **9.6** Re-run focused recipe/catalog tests, then `./tests/run.sh` and `./tests/validate.sh`, without widening scope or touching frozen files. <!-- sdd-owner: implementation -->
