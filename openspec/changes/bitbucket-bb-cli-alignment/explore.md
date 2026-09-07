# Exploration: Bitbucket CLI alignment

## Problem statement

`bitbucket-pr-flow` names the `bb` executable but currently targets the wrong
project. Its installation URL points to the TypeScript CLI published by
`0pilatos0` (`@pilatos/bitbucket-cli`), while the installed executable in the
reported environment is Homebrew `bb-cli` 1.4.1, a pure-PHP Bitbucket REST API
CLI from https://bb-cli.github.io. These projects share the `bb` binary name but
have different command contracts. Consequently, materialized guidance can pass
installation/auth checks and still issue commands that the installed CLI cannot
execute.

This is a catalog-recipe product fix. The repository's own manifest enables
`git-pr-flow`, not `bitbucket-pr-flow`, and its lock has no Bitbucket entry; the
change must not alter this project's dogfood configuration.

## Current surface and evidence

### Recipe manifest

`catalog/recipes/bitbucket-pr-flow/recipe.toml` currently declares:

- `[recipe]`: id `bitbucket-pr-flow`, name `Bitbucket PR Flow`, description
  `Bitbucket PR and merge flow for feature branches (via bb CLI)`, version
  `1.3.0`, author `ai-specs`, license `MIT`, and tags `vcs` and `bitbucket`.
- `[[capabilities]]`: `vcs-pr-flow`.
- `[[hooks]]`: `on-sync` / `validate-config`.
- `[[deps.cli]]`: required binary `bb`, purpose “Create and manage Bitbucket
  pull requests”, and
  `https://bitbucket-cli.paulvanderlei.com/getting-started/installation/` as
  `install_url`. It has no `version_check` or `min_version` today.
- Three optional config fields: `base_branch` (string, default `development`),
  `expected_owner` (string, default empty), and `auto_switch_account` (bool,
  default false). The latter's help text still points to the TypeScript CLI
  installation page and describes account switching as something to try,
  although the bundled guidance says `bb` has no auth switch.
- `[provides]`: bundled skill `bitbucket-merge-workflow` and bundled command
  `bb-pr-create`.
- `[provides.brief].workflow_rules`: use a PR merge workflow, identify Bitbucket
  and `bb`, prohibit direct pushes to the configured base, remove the feature
  worktree/local and remote branches after merge, and synchronize the base in
  the main worktree with `git checkout` and `git pull --ff-only`.
- `[[provides.docs]]`: copies `README.md` to
  `ai-specs/recipes/bitbucket-pr-flow/README.md`.

The CLI dependency schema in `lib/_internal/recipe_schema.py` permits exactly
`binary`, `purpose`, `required`, `install_url`, `version_check`, and
`min_version`. Thus the implementation can identify the intended package and
optionally add version probing without a schema extension.

### README and catalog documentation

`catalog/recipes/bitbucket-pr-flow/README.md` describes the provider, capability
binding, three config fields, protected-head behavior, and explicit push/no
implicit merge safety. It correctly uses `bb auth show` in its prerequisite note,
but its install URL identifies the TypeScript CLI. Its example pins recipe
version `1.1.0`, while the manifest is `1.3.0`. The config table says
`auto_switch_account` is reserved because `bb` has no auth switch, which is
consistent with the intended PHP surface but should not retain a TypeScript
installation link.

`docs/recipes-catalog.md` has the Bitbucket row in the at-a-glance table, lists
`bb` as the required binary, and documents the `bitbucket-pr-flow` section. Its
example also says version `1.1.0`; the dependency prose elsewhere says `bb`
remains guidance-only, which is stale if the opt-in Homebrew mapping is changed.
`docs/capabilities.md` correctly models `bitbucket-pr-flow` as a Specific
provider of `vcs-pr-flow` and currently labels it `Bitbucket/bb`.

### Bundled command and skill

`catalog/recipes/bitbucket-pr-flow/commands/bb-pr-create.md` performs these
operations before creating a PR:

1. `command -v bb`.
2. `bb auth show`.
3. An optional account-match script that parses `bb auth show` for a username.
4. An explicit `git push` with dynamically selected remote.
5. `bb pr create --source <branch> --destination <base> --title ... --body ...`.
6. Stop and report the PR URL.

The command's blocker remediation says `bb auth login`, which is not present on
the verified PHP CLI surface. The command therefore needs both authentication
remediation and create syntax alignment.

`catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md`
repeats the install/auth/account-match checks and create command, then adds:

- `bb pr view <pr-id> --json --jq '.source.commit.hash'` to capture and recheck
  the approved source commit (two occurrences).
- `bb pr merge <pr-id> --strategy squash --close-source-branch` for feature
  heads, or the same command without the close flag for protected heads.
- The same post-merge base synchronization and worktree/branch cleanup rules.

The skill also uses the invalid/unverified `bb auth login` remediation and
`bb pr view` spelling. The branch classification policy itself is independent
of the provider and should be preserved if it can be expressed with the PHP
CLI.

### PHP `bb-cli` evidence and command-contract drift

The verified installed surface is materially different:

- Global options include `--project <repo>`, `-i/--interactive`, `--title`, and
  `--description`.
- `bb pr` methods include `list` (`l`), `diff` (`d`), `files`, `commits` (`c`),
  `approve` (`a`), `no-approve` (`na`), `request-changes` (`rc`),
  `no-request-changes` (`nrc`), `decline`, `merge` (`m`), `create`, and `show`.
- `bb auth` methods are `save` and `show`.
- Top-level actions include `pr`, `pr-details`, `pipeline`, `branch`, `auth`,
  `browse`, `upgrade`, and `env`.
- `bb version` prints a colored `Version: 1.4.1`. ANSI escapes surround part
  of the output.
- `bb pr create --help` is unsafe as a discovery mechanism: it attempts a real
  operation and reached an auth error instead of printing help.

The complete documented invocation inventory is therefore:

| Existing invocation/guidance | PHP status | Exploration consequence |
|---|---|---|
| `command -v bb` | Generic and valid | Keep as availability preflight. It cannot identify which `bb` project owns the binary. |
| `bb auth show` | Verified method | Keep as the authentication inspection command. Its exact output shape for robust owner parsing still needs fixture/empirical coverage. |
| `bb auth login` | Not a verified PHP method; `auth` exposes `save`/`show` | Replace blocker guidance with the documented PHP authentication flow, likely `bb auth save` only after confirming its invocation and prompts. |
| `bb auth switch` (mentioned as absent) | No switch method observed | Keep the no-switch behavior, but do not tell users to run `login`; give manual `auth save`/documentation guidance. |
| `bb pr create --source ... --destination ... --title ... --body ...` | `create` exists, but `--source`, `--destination`, and `--body` are not in the verified surface; global options are `--project`, `--title`, and `--description` | Treat source/destination/body as open contract gaps. Do not publish replacement flags until verified against a real repository. |
| `bb pr view <id>` | Not observed; PHP exposes `show` | Change to `bb pr show` only after confirming positional id and output format. |
| `--json --jq '.source.commit.hash'` | JSON and jq support are unverified | Do not rely on this for the approval SHA. Define a PHP-compatible retrieval/parsing step after empirical verification. |
| `bb pr merge <id> --strategy squash [--close-source-branch]` | `merge` exists; both flags are unverified | Keep the protected/feature-head policy concept, but confirm PHP merge arguments and whether source-branch closure is a separate API/action. |
| `bb version` | Verified, returns 1.4.1 with ANSI coloring | Candidate for dependency version probing; parser behavior and a minimum-version policy need tests. |

The PHP CLI's global `--project` option also means the recipe's configured base
branch cannot be assumed to map directly to TypeScript `--destination`. The
meaning of project/repository selection, branch source selection, and title/
description inputs must be confirmed from the PHP CLI's documentation or a
Bitbucket-enabled repository.

### Dependency installation and doctor behavior

`lib/_internal/dep_install.py` currently maps `gh`, `glab`, `jq`, `direnv`, and
`git` to Homebrew/apt packages. `_GUIDANCE_ONLY` includes `npx` and `bb`, so
`bb` never receives an install offer and only displays its recipe URL. This
behavior came from commit `7e9ac67`, which introduced opt-in CLI installation
and direnv environment handling.

The proposed install design is to remove `bb` from `_GUIDANCE_ONLY` and add
`"bb": ("bb-cli", "")` to `_PACKAGE_MAP`. On macOS or Linux with Homebrew,
`resolve_install_plan` would then offer `brew install bb-cli`; it MUST NOT offer
`brew install bb`. `brew install bb` resolves to an unrelated `getbb.app` IDE
cask and is a name-collision trap. On Linux where only `apt-get` is available,
the empty apt package side intentionally falls through to guidance using the
recipe's PHP `bb-cli` URL. Windows and other unsupported systems likewise stay
guidance-only. The recipe install URL must point at the PHP `bb-cli` project or
its authoritative installation documentation, not the TypeScript site.

`lib/_internal/doctor.py` reports a missing required dependency as a warning and
uses `install_url` as guidance. This means doctor does not install anything and
will continue to work with the corrected URL. `docs/recipe-schema.md` and the
catalog dependency table should be reconciled with the new fact that `bb` may
have an opt-in Homebrew plan while still falling back to manual guidance on
Linux/Windows.

A version check is technically feasible: `lib/_internal/dep_check.py` scans
combined stdout/stderr with `\d+(?:\.\d+)*`, so it can extract `1.4.1` from
ANSI-colored `Version: 1.4.1`, and an unparseable result never blocks. A
candidate declaration is `version_check = "bb version"` with a documented
minimum, but the command's stability and the correct minimum are not yet a
product decision. A numeric check alone cannot prove that the binary is the
PHP implementation: the TypeScript `bb` might also emit a parseable version.
A provider-identity check or explicit migration guidance is still needed for
binary collisions.

### Tests and verification surfaces

The relevant tests expose the following impact:

- `tests/test_bitbucket_pr_flow_recipe.py` validates the manifest, capability,
  hooks, materialization, binding ambiguity, and golden content. Its goldens
  explicitly require `bb auth show`, `bb auth login`, TypeScript-style create
  flags (`--source`, `--destination`, `--title`, `--body`), `bb pr view`, and
  merge flags. These assertions must be changed together with the recipe
  assets; otherwise tests will preserve the wrong contract.
- `tests/test_recipes_catalog.py` checks Bitbucket README/catalog/capability
  presence and generic config symmetry. It does not currently validate the
  install URL, CLI methods, or recipe version, so targeted drift assertions are
  appropriate.
- `tests/test_sync_pipeline.py` covers Bitbucket renderer binding and default
  base branch behavior. The default-branch fixture hardcodes version `1.1.0`
  even though the catalog is `1.3.0`; update it to avoid stale examples. The
  renderer's expected label in this test remains `Bitbucket (`bb` CLI)` because
  the binary name does not change.
- `tests/test_dep_check.py` covers generic presence/version parsing, including
  the non-blocking behavior for unparseable versions, but has no Bitbucket
  fixture. Add focused coverage for ANSI version output only if a version check
  is adopted.
- The install-plan behavior is exercised in `tests/test_envrc_scaffold.py`
  (including the existing `bb` guidance-only expectation around its install
  plan), so changing the map requires updating/adding plan tests that assert
  `brew install bb-cli`, empty apt fallback, and never `brew install bb`.
- `tests/test_recipe_conflicts.py` verifies VCS tag conflict behavior and
  capability ambiguity. It does not need a new provider abstraction test for
  the recommended single-provider recipe; it should continue to ensure that
  Bitbucket and other VCS recipes remain alternative capability providers.
- `tests/evals/eval_vcs_pr_flow_live.py` runs the three Bitbucket scenarios using
  temporary materialized projects and checks planning prose/commands without a
  real remote merge. Scenario assertions currently expect the old merge flag
  vocabulary. They must be revised only after the PHP command contract is
  known; live evaluation must not pretend an unverified PHP invocation is safe.

## Design fork

### A. Retarget `bitbucket-pr-flow` entirely to PHP `bb-cli` (recommended)

Keep the existing recipe id and `vcs-pr-flow` capability, but make all bundled
assets and dependency metadata describe one concrete provider: the PHP
`bb-cli`. Replace the TypeScript URL, correct auth guidance, and rewrite
commands/skills around the PHP methods and options once empirical command
verification is complete. Keep `bb` as the executable and preserve the
Bitbucket label in `lib/_internal/agents-render.py`.

**Benefits**

- Matches the capability model: a specific recipe provides one concrete
  `vcs-pr-flow` implementation, and the bound recipe id is already the provider
  identity (`openspec/specs/vcs-pr-flow/spec.md`).
- Matches established sibling precedent from GitHub/GitLab/Bitbucket and the
  earlier provider-config decision: separate provider recipes are preferred to
  branching inside one skill/command (`openspec/changes/archive/2026-06-11-vcs-drop-provider-config/design.md`).
- Keeps the implementation scope local to the recipe, dependency installer,
  documentation, and focused tests; no multi-runtime dispatch or new schema
  abstraction is required.
- Gives the installed user's actual CLI a coherent contract instead of silently
  selecting one of two incompatible command languages.

**Costs and risks**

- Existing users of the TypeScript CLI currently pointed to by the recipe will
  see a behavior change. Their `bb` binary may pass `command -v bb` but fail the
  PHP-specific commands; migration guidance must explicitly call out the
  collision and PATH precedence.
- PHP create/merge/show output and flags still need empirical verification, so
  the recipe cannot safely be rewritten from help output alone (`bb pr
  create --help` performs a real operation).
- A version check can detect a minimum version but cannot reliably identify the
  implementation if both projects produce numeric version output.

### B. Support both runtimes as variants

Retain one Bitbucket recipe and branch its skill/command/dependency behavior on
a selected runtime/provider variant (for example a config field). The variant
would choose install guidance, auth/create/show/merge syntax, and perhaps
renderer text.

**Benefits**

- Preserves a path for users of the TypeScript CLI and reduces immediate
  breakage for existing recipe consumers.
- Makes the two projects' shared binary name explicit if the selector is
  configured and validated before any PR operation.

**Costs and risks**

- A single recipe would provide multiple incompatible implementations of one
  capability. Every command and skill would need branching, and tests/evals
  would need a matrix; this duplicates the provider selector already represented
  by recipe id plus `[[bindings]]`.
- The current config schema has simple independent fields, not a discriminated
  variant with variant-specific required fields. The installer map and doctor
  output also have no provider identity concept.
- A default variant would be dangerous: `command -v bb` cannot distinguish the
  two projects, and PATH ordering determines which binary runs. Misconfigured
  users could receive plausible but wrong guidance.
- Supporting both in one recipe makes capability binding less auditable and
  increases documentation drift. It also raises the question of whether
  `expected_owner` and branch semantics mean the same thing for both clients.

If TypeScript support is a product requirement, a safer future form is two
explicit sibling recipes with distinct ids (for example a PHP recipe and a
TypeScript recipe), both providing `vcs-pr-flow`, so explicit bindings select
one complete command contract. That is more migration work than A, but avoids
hidden runtime branching and follows the catalog's provider model.

## Open questions and empirical gates

1. What exact PHP syntax creates a PR from a source branch to the configured
   base branch? Confirm whether `--project`, `--title`, `--description`, and
   interactive mode are sufficient, and how source/destination branches are
   supplied. Do not infer this from `bb pr create --help`; it executes a real
   request.
2. What exact PHP syntax retrieves a PR and its source commit? Confirm whether
   `bb pr show` accepts a PR id, whether it emits stable machine-readable output,
   and whether JSON/jq support exists. The current approval-SHA guard cannot be
   retained until this is known.
3. What are the PHP merge arguments and source-branch closure semantics? Verify
   squash behavior and whether closing the source branch is a separate method or
   option. The current `--strategy squash` and `--close-source-branch` flags are
   unverified.
4. What does `bb auth show` print for workspace/username, and what does
   `bb auth save` require? Confirm a non-secret owner parsing strategy and the
   exact remediation text for users whose account does not match
   `expected_owner`.
5. Should the recipe enforce a minimum PHP `bb-cli` version? If so, confirm that
   `bb version` is stable, test ANSI-colored output through `_parse_version`, and
   document that version checks do not identify the implementation by themselves.
6. Can the maintainer provide a Bitbucket-enabled repository (or authoritative
   `bb-cli` command documentation) for the create/show/merge verification? A
   real repository is required before publishing executable PR commands.
7. How should users with the TypeScript `bb` already installed migrate? Guidance
   should explain PATH collision and must never recommend `brew install bb`.
8. Should the existing public recipe version advance from `1.3.0` after the
   contract correction? Regardless of the release decision, README,
   `docs/recipes-catalog.md`, and hardcoded test fixtures must stop using stale
   `1.1.0` examples.

## Recommended direction

Choose **A**: retarget `bitbucket-pr-flow` to the verified PHP `bb-cli` as one
provider implementation. First settle the empirical create/show/merge/auth
questions with a Bitbucket-enabled repository or authoritative upstream docs;
then update the recipe URL/help, bundled command, bundled skill, catalog docs,
tests, and optional dependency install plan as one atomic contract change.
Keep the binary name `bb` in renderer labels, but make installation offer
`brew install bb-cli` (never `brew install bb`) and retain guidance-only fallback
where no safe package manager mapping exists. Treat TypeScript compatibility as
a separate explicit-provider recipe only if users demonstrate a supported need;
do not hide both incompatible runtimes behind an implicit variant.

All sync/materialization smoke tests should use temporary isolated fixtures, as
`tests/evals/eval_vcs_pr_flow_live.py` already does. Do not enable the recipe in
this repository's manifest or write its `ai-specs/.ai-specs.lock` as part of
verification; generated lock/materialization output is evidence, not product
state.
