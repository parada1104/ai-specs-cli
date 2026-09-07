# Proposal: Align `bitbucket-pr-flow` with PHP `bb-cli`

## Why

Agents following `bitbucket-pr-flow` are guided to the **wrong Bitbucket CLI**.
The recipe names the `bb` binary but its `install_url`, README, command, and skill
point at the TypeScript `@pilatos/bitbucket-cli` (paulvanderlei docs). The CLI
users actually have — Homebrew formula `bb-cli` 1.4.1, homepage
https://bb-cli.github.io, pure PHP — shares the `bb` name and nothing else.

That mismatch is a user-facing blocker, not a docs nit:

- Install/auth checks can pass (`command -v bb`, `bb auth show`) while every
  subsequent command is from a different project.
- Remediation tells people to run `bb auth login`. PHP `bb-cli` exposes
  `bb auth save` / `bb auth show` only.
- Inspection tells people to run `bb pr view`. PHP exposes `bb pr show`.
- Create/merge guidance uses unverified TypeScript-shaped flags
  (`--source` / `--destination` / `--body`, `--json --jq`,
  `--strategy squash --close-source-branch`).

The result: an agent that "followed the recipe" still cannot open or merge a
Bitbucket PR. This change exists to make the Bitbucket side of `vcs-pr-flow` one
honest provider.

## What Changes

**Root cause.** The catalog recipe is pinned to a different upstream project that
happens to ship a `bb` binary. `bitbucket-pr-flow` is supposed to be a specific
`vcs-pr-flow` provider (one host, one CLI contract), the same model as
`git-pr-flow` → `gh` and `gitlab-mr-flow` → `glab`. Today it is not.

**Selected direction.** Retarget `bitbucket-pr-flow` **exclusively** to PHP
`bb-cli`. Keep recipe id, capability, binary name `bb`, and renderer label
`Bitbucket (`bb` CLI)`. Drop `@pilatos/bitbucket-cli` / paulvanderlei from every
live recipe surface (manifest `install_url`, config help, README, command,
skill, catalog docs, tests). No dual-runtime variants and no npm install path.

**Wrong-guidance correction.**

- Recipe `install_url` and blocker text point at https://bb-cli.github.io (or
  that project's authoritative install docs).
- Auth remediation uses `bb auth save` / `bb auth show`, not `login`.
- PR inspection uses `bb pr show`, not `view`.
- Create/merge invocations use only the PHP surface that is verified from
  upstream docs or read-only probes. Unverified flags stay documented as
  **open verification gaps** until apply dogfooding confirms them. This change
  MUST NOT run `bb pr create` or `bb pr merge`.
- Opt-in install: remove `bb` from `_GUIDANCE_ONLY` in
  `lib/_internal/dep_install.py` and add `"bb": ("bb-cli", "")` to
  `_PACKAGE_MAP`. On macOS (and Linux with Homebrew) the offer is
  `brew install bb-cli`. The empty apt side means Linux-without-brew falls
  through to guidance with `install_url`. **Never** `brew install bb` — that
  cask is getbb.app, an unrelated IDE.

**Version drift.** README, `docs/recipes-catalog.md`, and test fixtures that
still show recipe version `1.1.0` must use the recipe's real version `1.3.0`.
Manifest `version` fields remain legacy/ignored by sync; examples must still
match the catalog recipe. Host CLI metadata is separate: `[[deps.cli]]` MUST
declare `version_check = "bb --version"` and `min_version = "1.4.1"` (minimum
supported PHP `bb-cli`). Recipe version `1.3.0` MUST remain distinct from that
host floor.

**In-place hardening (user-authorized).** Keep this Standard change folder.
Do not open a second change. The first apply retargeted identity and install;
this hardening phase adds warning-wording locks, redacted auth output, a
positive PHP identity guard, host `min_version` `1.4.1`, explicit feature
remote-branch deletion, apply-progress path hygiene, and a renamed merge test.

## Intent

Give Bitbucket users a recipe whose install identity, auth, and PR verbs match
the PHP `bb-cli` they actually run, without changing GitHub/GitLab providers,
this project's dogfood lock, or the `bb` renderer label.

## Scope

### In

- Retarget `catalog/recipes/bitbucket-pr-flow/` (manifest, README, bundled
  command, bundled skill) to PHP `bb-cli`.
- Opt-in Homebrew install mapping `bb` → formula `bb-cli`; empty apt fallback.
- Catalog and schema docs that still say `bb` is guidance-only or still cite
  the TypeScript CLI.
- Tests and evals that pin the TypeScript URL, `auth login`, `pr view`,
  TypeScript create/merge flags, or stale `1.1.0` examples.
- Conservative open-verification notes for any PHP flag not confirmed from
  upstream docs or read-only probes (`bb pr list`, `bb pr show` only).
- Host CLI version metadata: `version_check = "bb --version"`,
  `min_version = "1.4.1"`.
- Positive PHP `bb-cli` identity guard on agent-facing skill/command preflight.
- Agent-facing `bb auth show` capture that emits only `Username`, rejects
  missing or multiple `Username` lines, and never prints `AppPassword`.
- Explicit feature remote-branch deletion after merge, with protected-head
  exclusions preserved.
- Authorized warning wording (no `brew install bb` substring, no
  `@pilatos` / paulvanderlei even as negatives, no `bb auth login` substring).
- Apply-progress wording without absolute host or worktree paths.
- Rename the misleading merge test that currently reads as if PHP merge
  closed the source branch.

### Out

- Dual-runtime / config-variant support for TypeScript and PHP in one recipe.
- npm / `@pilatos/bitbucket-cli` install or docs.
- Changing `_VCS_RECIPE_LABELS` or the binary name `bb`.
- Enabling `bitbucket-pr-flow` in this repository's dogfood manifest or lock.
- Executing `bb pr create` or `bb pr merge` during planning or apply as a
  discovery mechanism (`bb pr create --help` performs a real operation).
- A second sibling recipe for the TypeScript CLI (future work only if users
  demonstrate a supported need).

## Capabilities

Capability owners follow the existing spec layout: Bitbucket provider contract
lives in `vcs-pr-flow`; install-plan contract lives in `recipe-cli-deps`.

### New Requirements

- **Bitbucket provider targets the PHP `bb-cli` contract.**
  `bitbucket-pr-flow` MUST identify Homebrew formula `bb-cli`
  (https://bb-cli.github.io, pure PHP) as its sole upstream. Live recipe
  surfaces MUST NOT mention `@pilatos/bitbucket-cli`, paulvanderlei, or
  `bb auth login` / `bb pr view`.

- **Install offer resolves brew `bb-cli` on macOS.**
  For binary `bb`, `resolve_install_plan` MUST offer `brew install bb-cli`
  when Homebrew is available, MUST use an empty apt package (guidance +
  `install_url` on apt-only Linux), and MUST NEVER emit `brew install bb`.
  `npx` remains guidance-only. Doctor stays check-only; `install_url` still
  flows through warning text.

- **Host CLI version floor is 1.4.1.**
  `bitbucket-pr-flow` `[[deps.cli]]` MUST set `version_check = "bb --version"`
  and `min_version = "1.4.1"`. Recipe version MUST stay `1.3.0`. The two
  numbers MUST NOT be conflated in docs or tests.

- **Positive PHP identity guard.**
  Agent-facing install/auth preflight MUST positively verify that `bb` on
  `PATH` is PHP `bb-cli` (not merely fail later on TypeScript verbs). A
  different `bb` MUST block with `bb-cli` / `brew install bb-cli` /
  https://bb-cli.github.io guidance.

- **Redacted `bb auth show` output.**
  Agent-facing auth checks MUST capture `bb auth show` and emit only the
  `Username` line. Missing `Username` or more than one `Username` line MUST
  reject. `AppPassword` MUST never be printed.

- **Feature remote-branch deletion after merge.**
  Post-merge cleanup MUST explicitly delete the feature remote branch while
  preserving protected-head exclusions (`main` / `master` / `development` /
  `staging` / configured base or integration branch).

- **Authorized warning wording and apply-progress hygiene.**
  Live surfaces MUST keep the user-authorized warning locks: never the
  three-token string `brew install bb`, never `@pilatos` / paulvanderlei
  even as a negative example, never the substring `bb auth login`.
  `apply-progress.md` MUST NOT embed absolute host or worktree paths.

### Modified Requirements

- **`vcs-pr-flow` — Bitbucket auth preflight and runtime docs.**
  Keep `bb auth show` as the auth inspection command (already specified).
  Replace "manual login" / `bb auth login` remediation with the PHP
  `bb auth save` flow. Account-match mismatch still blocks (no `auth switch`).
  Create/show/merge guidance MUST use PHP methods (`create`, `show`, `merge`)
  and only verified flags; unverified flags MUST be called out as open
  verification gaps rather than copied from the TypeScript CLI.
  Auth capture MUST emit only `Username` (reject missing or multiple lines;
  never print `AppPassword`). Post-merge feature cleanup MUST include
  explicit remote-branch deletion with protected-head exclusions.

- **`recipe-cli-deps` — TTY opt-in install for known packages.**
  Current spec text (`npx` and `bb` SHALL be guidance-only) MUST change so
  that `bb` is a mapped binary (`bb-cli` / empty apt) and only `npx` stays
  guidance-only. Constrained static map, TTY confirm, and non-TTY/doctor
  non-install behavior are unchanged. The Bitbucket `[[deps.cli]]` row MUST
  also carry `version_check = "bb --version"` and `min_version = "1.4.1"`.

## Impact

| Area | Change |
|------|--------|
| `catalog/recipes/bitbucket-pr-flow/recipe.toml` | PHP `install_url`; drop paulvanderlei from `auto_switch_account` help; keep binary `bb`, recipe version `1.3.0`; add `version_check = "bb --version"` and `min_version = "1.4.1"` |
| `catalog/recipes/bitbucket-pr-flow/README.md` | PHP identity, `bb auth show`/`save`, version example `1.3.0` |
| `catalog/recipes/bitbucket-pr-flow/commands/bb-pr-create.md` | PHP auth remediation, redacted Username-only auth capture, PHP identity guard, create syntax or explicit open-verification notes |
| `catalog/recipes/bitbucket-pr-flow/skills/bitbucket-merge-workflow/SKILL.md` | `pr show` instead of `view`; merge/create flags only if verified; PHP identity guard; redacted auth; explicit feature remote-branch deletion; preserve protected-head / no-auto-merge / cleanup policy |
| `docs/recipes-catalog.md` | PHP CLI contract; version example `1.3.0`; install-offer wording |
| `docs/recipe-schema.md` | `bb` is no longer listed as guidance-only |
| `lib/_internal/dep_install.py` | Remove `bb` from `_GUIDANCE_ONLY`; map `"bb": ("bb-cli", "")` |
| `lib/_internal/doctor.py` | Unchanged behavior; corrected `install_url` flows through |
| `lib/_internal/agents-render.py` | **No change** (`bitbucket-pr-flow` → `("Bitbucket", "bb")`) |
| `tests/test_bitbucket_pr_flow_recipe.py` | Goldens for URL, auth save/show, redacted Username, identity guard, `pr show`, create/merge contract, remote-branch delete, apply-progress path hygiene; rename misleading merge test |
| `tests/test_env_scaffold.py` | Replace `test_bb_guidance_only` with brew `bb-cli` / empty-apt / never `brew install bb` |
| `tests/test_recipes_catalog.py` | Version/identity drift assertions; host `1.4.1` distinct from recipe `1.3.0` |
| `tests/test_sync_pipeline.py` | Bitbucket fixture version `1.3.0` (today hardcoded `1.1.0`) |
| `tests/test_dep_check.py` | Unchanged (generic `version_check` / `min_version` parser already exists) |
| `openspec/changes/bitbucket-bb-cli-alignment/apply-progress.md` | Strip absolute host/worktree paths from wording (hardening apply; not this RED phase) |
| `tests/evals/eval_vcs_pr_flow_live.py` | Revise expected Bitbucket verbs after the PHP contract is known; still no real remote merge |

This repository's own `ai-specs.toml` / lock MUST NOT enable Bitbucket as part
of verification. Sync/eval coverage uses temporary isolated fixtures, as today.

## Open verification gaps

Resolved during apply from upstream docs (bb-cli.github.io) plus read-only
probes only (`bb pr list`, `bb pr show`). Do not infer from
`bb pr create --help`.

Known PHP surface (verified locally on `bb-cli` 1.4.1):

- `bb pr` methods: `list` (`l`), `diff` (`d`), `files`, `commits` (`c`),
  `approve` (`a`), `no-approve` (`na`), `request-changes` (`rc`),
  `no-request-changes` (`nrc`), `decline`, `merge` (`m`), `create`, `show`
- `bb auth` methods: `save`, `show`
- Global options: `--project`, `-i`/`--interactive`, `--title`, `--description`

Unverified — document conservatively until confirmed; do not publish as
executable guidance:

| Current recipe invocation | Gap |
|---------------------------|-----|
| `bb pr create --source … --destination … --body …` | `create` exists; those flags are not in the verified surface. How source/destination branches are supplied under `--project` / `--title` / `--description` is unknown. |
| `bb pr view <id> --json --jq '…'` | Method is `show`; positional id, output shape, and JSON/jq support are unverified. Approval-SHA guard cannot keep the TypeScript pipeline until this is known. |
| `bb pr merge <id> --strategy squash --close-source-branch` | `merge` exists; squash and source-branch closure flags/semantics are unverified. Protected-head policy stays; the PHP argv does not. |
| `bb auth save` exact prompts | Replace `login` in blocker text; confirm non-secret invocation before encoding flags. |
| `bb auth show` owner parse | Keep the command; fixture the output shape before treating username extraction as stable. |

## Risks

- **`brew install bb` name collision.** Homebrew `bb` is getbb.app (IDE cask),
  not Bitbucket. Any install offer, test, or doc that emits `brew install bb`
  installs the wrong product. The map MUST use formula `bb-cli`.
- **Binary collision.** `command -v bb` cannot tell PHP `bb-cli` from a
  TypeScript Bitbucket CLI. Users with the wrong `bb` on PATH will pass
  presence checks and fail PHP commands. Hardening adds a positive PHP
  identity guard that blocks with `bb-cli` / brew guidance.
- **Publishing unverified flags.** Copying TypeScript argv onto PHP `create` /
  `merge` / `show` will recreate today's blocker. Open gaps stay explicit until
  docs or read-only probes confirm them.
- **TypeScript consumers.** Anyone who installed from the current paulvanderlei
  URL sees a breaking guidance change. That is intended. There is no in-recipe
  variant.
- **Version vs identity.** `min_version = "1.4.1"` is a numeric floor, not
  proof of PHP identity. A foreign `bb` that prints a parseable version can
  still pass dep check; the identity guard is the product control.
- **Auth secret leak.** Raw `bb auth show` prints `AppPassword`. Agent-facing
  checks MUST capture and redact to `Username` only.

## Rollback

Revert the change branch / PR. No data migration. Users who accepted
`brew install bb-cli` keep that formula; recipe docs and the install map
return to guidance-only `bb` with the previous URL.

## Tracker

- card_id: `6a9a626a0f40c736263c4a34`
- shortLink: `sgY6K4P2`
- url: https://trello.com/c/sgY6K4P2
- list: In Progress

## Success Criteria

- `bitbucket-pr-flow` `[[deps.cli]]` declares binary `bb` with PHP `bb-cli`
  upstream identity (https://bb-cli.github.io or that project's install docs),
  and no paulvanderlei / `@pilatos` URL remains on live recipe surfaces
  (manifest, README, command, skill, catalog).
- `resolve_install_plan("bb")` offers opt-in `brew install bb-cli` on macOS
  (Homebrew present), falls back to guidance with `install_url` on apt-only
  Linux and other unsupported systems, and never proposes `brew install bb`.
- Every `bb` invocation in recipe docs matches the PHP `bb-cli` 1.4.1 verified
  surface (`auth save`/`show`, `pr show`/`create`/`merge`, global
  `--project`/`--title`/`--description`) or carries an explicit
  open-verification note. No `bb auth login` substring, no `bb pr view`.
- Version references in recipe docs and tests are consistent with catalog
  recipe version `1.3.0` (no `1.1.0` drift). Host CLI metadata is
  `version_check = "bb --version"` and `min_version = "1.4.1"`. Renderer
  label stays `Bitbucket (`bb` CLI)`.
- Agent-facing auth checks emit only `Username`, reject missing or multiple
  `Username` lines, and never print `AppPassword`.
- A non-PHP `bb` on PATH is blocked with PHP `bb-cli` / `brew install bb-cli`
  guidance.
- Post-merge cleanup explicitly deletes the feature remote branch and never
  deletes a protected head.
- `apply-progress.md` wording has no absolute host or worktree paths.
- The misleading merge test is renamed so it no longer reads as if PHP merge
  closed the source branch.

## Planning depth

**Standard** (proposal + spec + tasks). Requested depth: Standard. Signal
depth: Standard (catalog-recipe contract + install-plan delta, no new
runtime abstraction). Decided depth: Standard. Depth line to be annotated in
`tasks.md` before authorization.
