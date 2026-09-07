# Apply Progress: bitbucket-bb-cli-alignment

## Structured status consumed

- `schemaName`: gentle-pi.sdd-status
- `changeName`: bitbucket-bb-cli-alignment
- `artifactStore`: openspec
- `applyState`: ready
- `actionContext.mode`: repo-local
- `allowedEditRoots`: change worktree (repository-relative `.worktrees/bitbucket-bb-cli-alignment`)
- `actionContext.warnings`: none
- Workload: Decision needed before apply: No; Chained PRs: No; 1200-line budget risk: Low
- Delivery: single PR within the explicit 1200-line budget (no exception)

## TDD Cycle Evidence

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| 1.1 | `tests/test_env_scaffold.py` | Unit | ✅ 8/8 DepInstallTests | ✅ Darwin/Linux brew `guidance != brew` | pending 2.1 | ✅ 3 cases | ➖ None |
| 2.1 | `tests/test_env_scaffold.py` | Unit | ✅ same class | uses 1.1 | ✅ 10/10 DepInstallTests | ✅ Darwin + Linux brew + apt-only | ➖ Map-only change |
| 3.1 | `tests/test_bitbucket_pr_flow_recipe.py`, `tests/test_recipes_catalog.py` | Unit | existing recipe/catalog suite | ✅ 22 failures vs TypeScript surfaces | pending 4.x | ✅ identity/auth/verbs/gaps | ➖ Fence regex tightened to ```bash/sh only |
| 3.2 / 4.x | same + recipe/docs | Unit | | | ✅ 56 recipe+Bitbucket catalog tests OK; 35 catalog module OK | ✅ | ➖ |
| 4.6 | `tests/test_sync_pipeline.py`, eval scenarios | Unit | | n/a fixture+eval pin | ✅ `test_defaulted_base_branch_propagates_into_brief` OK | ➖ fixture version only | ➖ |
| 5.1–5.2 | apply-progress (docs+probes) | n/a | n/a | n/a | n/a | ➖ docs/probes | ➖ |
| 6.1–6.3 | focused + full suite | Unit | ✅ focused GREEN | n/a validation | ✅ `./tests/run.sh` 1805 OK; `./tests/validate.sh` 1805 OK | ➖ | ➖ |

Triangulation skipped for 4.6 fixture version pin: single possible output (`1.3.0`).

### Task 1.1 RED evidence

Command: `python3 -m unittest tests.test_env_scaffold.DepInstallTests`

- Safety net before edits: 8 tests OK.
- After replacing `test_bb_guidance_only`: 10 tests, 2 failures.
- `test_bb_brew_plan_on_darwin`: `AssertionError: 'guidance' != 'brew'`
- `test_bb_brew_plan_on_linux`: `AssertionError: 'guidance' != 'brew'`
- `test_bb_apt_only_is_guidance`: passed coincidentally because `bb` was still in `_GUIDANCE_ONLY`. Real empty-apt path proven after 2.1.

### Task 2.1 GREEN evidence

Command: `python3 -m unittest tests.test_env_scaffold.DepInstallTests` → 10 tests OK.

Production: `"bb": ("bb-cli", "")` in `_PACKAGE_MAP`; `_GUIDANCE_ONLY = frozenset({"npx"})`; comment names only npx.

### Task 3.1 RED evidence

Command: `python3 -m unittest tests.test_bitbucket_pr_flow_recipe tests.test_recipes_catalog.BitbucketPrFlowDocsContractTests`

- 56 tests, 22 failures (PHP URL, auth save, pr show, no paulvanderlei, create/merge argv, catalog 1.3.0 / bb-cli, schema guidance-only).

### Task 3.2 / 4.x GREEN evidence

Same command → 56 tests OK. Full `tests.test_recipes_catalog` → 35 tests OK.

### Task 4.6

- `tests/test_sync_pipeline.py` fixture `version = '1.3.0'`.
- `TestVcsDropRemediations.test_defaulted_base_branch_propagates_into_brief` OK; renderer still `Bitbucket (`bb` CLI)`.
- Bitbucket eval scenarios: require `bb pr merge` + git/worktree cleanup; forbid `--close-source-branch` / `--strategy` on merge lines. `LIVE_SCENARIOS` list unchanged. Evals remain `EVALS_LIVE=1` optional.

### Task 8.1 / 8.2 RED evidence

Command: `python3 -m unittest tests.test_bitbucket_pr_flow_recipe tests.test_recipes_catalog`

- 90 tests, exit 1 as expected against the pre-hardening surfaces.
- Expected failures covered absent host metadata, missing positive identity guards, raw auth output, missing multiple-Username handling, missing feature remote deletion, absolute apply-progress paths, and missing catalog host-floor documentation.

### Task 9.1–9.6 GREEN evidence

- Added `bb --version` / minimum `1.4.1` metadata while retaining recipe version `1.3.0`.
- Added PHP identity guards, Username-only auth capture with missing/empty/multiple rejection, feature-only remote deletion, apply-progress path hygiene, and catalog documentation.
- Focused recipe/catalog tests: 90 tests OK. Focused environment/dependency/sync checks: 36 + 10 + 91 tests OK.
- `./tests/run.sh`: exit 0; 1,805 Python tests OK and Go gate passed.
- `./tests/validate.sh`: exit 0; 1,805 Python tests OK, `py_compile`, `bash -n`, and `gofmt` checks passed.

## Upstream verification (PHP bb-cli)

- Observed host `bb --version`: `Version: 1.4.1` (ANSI-colored); the active executable resolves to Homebrew `bb-cli` 1.4.1 and is a PHP script. Added `version_check = "bb --version"` and `min_version = "1.4.1"`.
- Docs fetched 2026-09-04 (apply re-confirm):
  - https://bb-cli.github.io — PHP Bitbucket Rest API CLI identity
  - https://bb-cli.github.io/installation/ — PHP >= 7, standalone binary `bb`
  - https://bb-cli.github.io/authentication — `bb auth` / `bb auth save`; `bb auth show`
  - https://bb-cli.github.io/help/ — `bb pr` methods include list/create/show/merge; global `--project`
  - https://bb-cli.github.io/docs/commands/pull-request.html — create `bb pr create [source-branch] [options]` with `-i` / `--title` / `--description`; positional source (omitted → current branch); example `bb pr create develop test 0` (second positional destination); comma-separated targets; show `bb pr show <id> [unresolved]` comments; merge `bb pr merge` with id implied, no squash/close-source flags documented
- Probes (cwd: this GitHub worktree; expected failure):
  - `bb pr list` → exit 0, stderr `Cannot get repository info. Are you sure this is a bitbucket repository?`
  - `bb pr show` → exit 0, `PR ID required. Usage: bb pr show <pr_id> [unresolved]`
  - `bb pr show 1` → exit 0, same not-a-Bitbucket-repo message
  - `bb auth show` → exit 0, redacted shape `Username: ` / `AppPassword: ` (empty locally). Do not print AppPassword.
  - Never ran `bb pr create`, `bb pr create --help`, or `bb pr merge`.
- Promoted to executable:
  - `bb pr create <branch> <base> --title "..." --description "..."` (no `-i` as agent default)
  - `bb pr show <id>` as comments viewer
  - `bb pr merge <id>` method + id only
  - `bb auth show` / `bb auth save` (no invented save flags)
  - Install: `brew install bb-cli`; `install_url` https://bb-cli.github.io
- Remaining open gaps:
  - Approval-SHA retrieval: `bb pr show` is comments, not JSON; `bb pr commits` not an allowed probe — stop and confirm in UI
  - `bb auth save` exact prompts
  - `bb auth show` Username awk is conservative, not a stable API
  - Squash / close-source-branch: not documented on PHP merge — git/UI policy only
  - No Bitbucket-enabled directory on disk for a successful `bb pr list`

## Completed tasks (persisted checkboxes)

Implementation-owned marked `- [x]`: **1.1, 2.1, 3.1, 3.2, 4.1–4.6, 5.1, 5.2, 6.1, 6.2, 6.3, 8.1, 8.2, 9.1–9.6**. Re-read of `tasks.md` confirms every implementation row is `- [x]`.

Parent close-out completed: **7.1, 7.2**.

## Files changed

- `lib/_internal/dep_install.py`
- `tests/test_env_scaffold.py`
- `tests/test_bitbucket_pr_flow_recipe.py`
- `tests/test_recipes_catalog.py`
- `tests/test_sync_pipeline.py`
- `catalog/recipes/bitbucket-pr-flow/recipe.toml`
- `catalog/recipes/bitbucket-pr-flow/README.md`
- `catalog/recipes/bitbucket-pr-flow/commands/bb-pr-create.md`
- `catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md`
- `docs/recipes-catalog.md` (Bitbucket section only)
- `docs/recipe-schema.md` (TTY map / guidance-only sentence)
- `tests/evals/scenarios/bitbucket-pr-flow/ac_protected_head_no_delete/*`
- `tests/evals/scenarios/bitbucket-pr-flow/ac_feature_head_cleanup/*`
- `tests/evals/README.md` (Bitbucket feature-head assert wording)
- `openspec/changes/bitbucket-bb-cli-alignment/apply-progress.md` — this file
- `openspec/changes/bitbucket-bb-cli-alignment/tasks.md` — implementation checkboxes

## Test commands run

| Command | Result |
|---------|--------|
| `python3 -m unittest tests.test_env_scaffold.DepInstallTests` | RED then GREEN (10 OK) |
| `python3 -m unittest tests.test_bitbucket_pr_flow_recipe tests.test_recipes_catalog.BitbucketPrFlowDocsContractTests` | RED 22 fail → GREEN 56 OK |
| `python3 -m unittest tests.test_recipes_catalog` | GREEN 35 OK |
| `python3 -m unittest tests.test_sync_pipeline.TestVcsDropRemediations.test_defaulted_base_branch_propagates_into_brief` | GREEN |
| focused env_scaffold / recipe / catalog / sync selection | GREEN (36 + 48 + 35 + 1) |
| `./tests/run.sh` | GREEN exit 0; Ran 1805 tests in 512.967s OK; Go gate passed |
| `./tests/validate.sh` | GREEN exit 0; Ran 1805 tests in 513.732s OK; py_compile, bash -n, gofmt, and Go gate passed |

## Deviations from design

- Live surfaces never contain the three-token string `brew install bb` even as a "never do this" warning (token-aware tests and substring trap). Wording is "never Homebrew formula/cask `bb` (getbb.app)".
- Live surfaces do not name `@pilatos` / paulvanderlei even as a negative example; they say "TypeScript Bitbucket CLI".
- Catalog auth note avoids the substring `bb auth login` ("not a login subcommand").
- Executable-fence helper only matches ` ```bash` / ` ```sh` so closing fences are not parsed as commands.

## Workload / PR boundary

- `git diff --stat` (tracked files): 16 files, 594 insertions, 114 deletions; OpenSpec change artifacts are intentionally untracked until the review branch commit.
- Delivery remains a single PR within the explicit 1200-line budget (no chain or `size:exception`).
- No commit/push has been performed; delivery authorization remains separate from this close-out.
- Dogfood isolation: no `ai-specs sync` against this repo's `ai-specs/`.

## Remaining tasks

No unchecked implementation or parent-owned rows remain.

## Frozen-contract evidence (6.3)

- `git diff` empty for `ai-specs/ai-specs.toml`, `ai-specs/.ai-specs.lock`, `lib/_internal/doctor.py`, `lib/_internal/agents-render.py`, `catalog/recipes/git-pr-flow/**`, `catalog/recipes/gitlab-mr-flow/**`, `openspec/specs/**`.
- Dogfood has no `bitbucket-pr-flow` enablement.
- Live Bitbucket surfaces (recipe.toml, README, command, skill, catalog Bitbucket section, recipe-schema TTY sentence): no `paulvanderlei`, `@pilatos`, `bb auth login`, `bb pr view`, three-token `brew install bb`, or Bitbucket `1.1.0`.
- No staged files. No commit/push.
