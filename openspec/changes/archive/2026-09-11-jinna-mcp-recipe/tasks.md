# Tasks: `jinna-mcp-recipe`

Depth: full

## Planning classification and execution rules

- **Requested depth:** Full OpenSpec planning package.
- **Signal depth:** Full: this change extends the recipe/dependency schema, adds a security-sensitive cross-platform release installer, changes MCP materialization, adds a new catalog recipe, and requires deterministic and provider-backed evidence.
- **Decided depth:** Full.
- **Current phase:** Planning complete. Apply and Verify are not authorized by this artifact alone; they require the user's explicit phase approval.
- **Provider assumption:** Planning artifacts assume `parada1104/jinna-provider` is release-ready. Before Apply/Verify, the parent MUST run the provider readiness gate in `design.md` and `spec.md`. A missing or security-blocked provider release blocks end-to-end installer verification.
- **Repository rule:** All changes below belong to `ai-specs-cli` and its dedicated recipe worktree. Do not modify `jinna-provider` or the Python reference repository from this change.
- **TDD rule:** For each behavior task, write a focused failing test first (RED), implement the smallest change (GREEN), run the focused test, then run the relevant package/full suite before refactoring. Record RED/GREEN evidence in the task notes or verify report.
- **Safety rule:** No network download, provider installation, executable replacement, or secret collection may happen without the exact consent/non-TTY behavior specified here.
- **Completion rule:** A task is complete only when its acceptance checks and tests pass. Do not mark a task done because a file exists.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~5,100 authored (2 branch commits ~4,684 + this turn ~410) |
| Chained PRs recommended | Yes |
| Suggested split | The 7 reviewable work units below |
| Delivery strategy | exception-ok — `size:exception` accepted by the user |
| Chain strategy | not applicable (single PR) |

Decision needed before apply: No (apply is complete)
Chained PRs recommended: Yes (7 work units) — overridden by an accepted `size:exception`
Chain strategy: not applicable (single PR)
Delivery decision: single PR of ~5,200 changed lines with `size:exception` explicitly accepted

## Dependency graph

```text
0 Provider readiness contract (read-only gate before apply/verify)
  └─ 1 Schema + dependency result model
       ├─ 2 Platform/release metadata resolver
       │    └─ 3 Secure GitHub Release installer
       │         └─ 4 Consent/configuration lifecycle
       ├─ 5 Managed provider command resolution
       │    └─ 6 MCP materialization integration
       └─ 7 Catalog recipe assets
            └─ 8 Documentation and cross-runtime evidence
                 └─ 9 Full validation and review handoff
```

## Phase 0 — Freeze provider release contract and readiness gate

### [x] 0.1 Confirm provider release identity before implementation

- **Read:** `design.md`, `spec.md`, provider repository release docs, actual GitHub repository `https://github.com/parada1104/jinna-provider`.
- **Verify:** canonical repository, stable tag policy, binary version output, and minimum compatible provider version.
- **Acceptance:** the exact provider repository, tag format, and `min_version` used by the recipe are recorded in a fixture or task evidence; no guessed path remains.
- **Note:** This is a read-only prerequisite. It does not modify the provider or consume a release during artifact planning.

### [x] 0.2 Validate the real first release before Apply/Verify

- **Depends on:** provider repository release publication outside this change.
- **Verify:** five expected archives, `SHA256SUMS`, `RELEASE.json`, archive root layout, executable names, checksums, and host-target installation in an isolated directory.
- **Verify:** provider Go/SDK security gate is green and provider quality workflows have passed.
- **Acceptance:** apply/verify may claim end-to-end provider acquisition only after every gate passes; otherwise record a blocking prerequisite and keep the recipe installer unverified.

## Phase 1 — Dependency schema and resolution model

### [x] 1.1 Add RED tests for GitHub Release dependency declarations

- **Files:** `tests/test_recipe_schema.py` or a focused new recipe schema test module.
- **Test:** parse valid `installer = "github-release"`, `repository`, and `release_policy`; round-trip through `recipe-read`.
- **Test:** reject missing repository/policy, malformed repository, unknown installer/policy, and non-allowlisted provider repository.
- **Test:** prove existing legacy `[[deps.cli]]` recipes parse and serialize unchanged.
- **Evidence:** focused tests fail before implementation.

### [x] 1.2 Extend `CliDep` and schema validation

- **Files:** `lib/_internal/recipe_schema.py`, `lib/_internal/recipe-read.py`.
- **Implement:** optional installer metadata with strict enum/allowlist validation; preserve existing fields and defaults.
- **Implement:** `jinna` declaration shape from `spec.md`; keep `install_url` guidance-only for legacy paths.
- **Acceptance:** 1.1 passes; existing recipe schema suite remains green; unsafe values cannot reach an install plan.

### [x] 1.3 Add RED tests for structured provider resolution

- **Files:** dependency test module.
- **Test:** model resolution sources (`path`, `managed`, `unresolved`), concrete command path, version, target, release tag, and verification status.
- **Test:** compatible PATH candidate wins; managed candidate is considered only when PATH is absent/incompatible; no secret-bearing fields are serialized.
- **Evidence:** focused tests fail before implementation.

### [x] 1.4 Implement shared provider resolution result

- **Files:** `lib/_internal/dep_check.py`, `lib/_internal/dep_install.py` or a narrowly scoped sibling module.
- **Implement:** `ProviderResolution`/equivalent immutable result and a pure resolver seam; keep generic dependency checks backward compatible.
- **Acceptance:** source precedence and required-provider semantics are covered; doctor remains non-mutating.

## Phase 2 — Platform and GitHub release selection

### [x] 2.1 Add RED tests for host target mapping

- **Test:** map Darwin/Linux/Windows and `arm64`/`aarch64`/`amd64`/`x86_64` to the five supported targets.
- **Test:** reject unsupported OS/architecture without network or filesystem mutation.
- **Evidence:** focused tests fail before implementation.

### [x] 2.2 Implement deterministic target and asset naming

- **Files:** provider installer module.
- **Implement:** exact archive names from `spec.md`; select `.tar.gz` for Unix and `.zip` for Windows.
- **Acceptance:** no user-provided arbitrary filename can affect target selection; unsupported targets produce actionable guidance.

### [x] 2.3 Add RED tests for stable GitHub release metadata

- **Test:** injected HTTP fixtures for `/repos/parada1104/jinna-provider/releases/latest` and optional tag lookup.
- **Test:** skip drafts/prereleases; reject wrong repository, malformed JSON, duplicate assets, missing expected target, wrong host URL, oversized response, and timeout.
- **Test:** select exact `SHA256SUMS` and `RELEASE.json` assets.
- **Evidence:** focused tests fail before implementation.

### [x] 2.4 Implement bounded GitHub metadata/artifact fetching

- **Files:** provider installer module and dependency integration.
- **Implement:** stdlib HTTPS requests, bounded timeout/response size, stable User-Agent, GitHub host/repository allowlist, latest-stable policy.
- **Implement:** no GitHub token, `gh`, Go, package manager, or arbitrary shell command requirement for users.
- **Acceptance:** 2.3 passes; failures return structured actionable errors and never execute downloaded content.

## Phase 3 — Checksum, archive safety, and atomic installation

### [x] 3.1 Add RED tests for checksum verification

- **Test:** valid checksum entry, missing entry, malformed digest, duplicate entry, mismatched digest, and archive download failure.
- **Test:** no extraction or execution occurs before archive verification.
- **Evidence:** focused tests fail before implementation.

### [x] 3.2 Add RED tests for archive inspection

- **Test:** valid Unix tarball and Windows zip with expected executable/docs.
- **Test:** reject absolute paths, `..` traversal, symlinks, hardlinks, duplicate members, unexpected executables, missing executable, wrong root, corrupt archive, and excessive member/path size.
- **Evidence:** focused tests fail before implementation.

### [x] 3.3 Implement checksum and archive verification

- **Files:** provider installer module.
- **Implement:** exact `SHA256SUMS` parser, archive member validation, safe extraction to a temporary destination, Unix mode `0755`, Windows `.exe` handling.
- **Acceptance:** 3.1 and 3.2 pass; no rejected artifact can be executed.

### [x] 3.4 Add RED tests for atomic publication and receipts

- **Test:** injected failures at download, checksum, extraction, permission, version self-check, receipt write, and rename stages.
- **Test:** previous managed version and user-owned PATH executable remain unchanged.
- **Test:** receipt contains only repository/tag/target/archive/binary digests/status and no secrets.
- **Evidence:** focused tests fail before implementation.

### [x] 3.5 Implement managed cache and atomic publication

- **Files:** provider installer module; reuse existing binary-cache conventions where possible.
- **Implement:** user-writable version/target cache, staged install, `install.json` verification receipt, atomic directory/file publication, revalidation on reuse.
- **Acceptance:** 3.4 passes; failed candidates are cleaned/quarantined without deleting unrelated files; compatible candidates are reused without silent upgrades.

### [x] 3.6 Add RED tests for version self-check and managed revalidation

- **Test:** selected release version matches `<managed-path> version`; mismatched/unknown/failing output is rejected.
- **Test:** changed managed bytes or malformed receipt causes rejection and no execution.
- **Evidence:** focused tests fail before implementation.

### [x] 3.7 Implement provider version verification

- **Files:** provider installer/resolution module.
- **Implement:** strict successful version check for provider dependencies while preserving legacy unknown-version behavior for unrelated recipes.
- **Acceptance:** 3.6 passes; `doctor` reports the reason without downloading.

## Phase 4 — Consent and configuration lifecycle

### [x] 4.1 Add RED tests for TTY and non-TTY installation behavior

- **Test:** `doctor`, `sync`, CI, and non-TTY configuration never download or mutate.
- **Test:** interactive decline never contacts GitHub and leaves no temporary or destination artifact.
- **Test:** interactive acceptance executes exactly one resolved plan and rechecks the provider.
- **Evidence:** focused tests fail before implementation.

### [x] 4.2 Integrate provider install offer into the interactive dependency gate

- **Files:** `lib/_internal/dep_install.py`, `lib/_internal/config_wizard.py`, `lib/_internal/recipe-add.py` or the actual configuration seam selected during implementation.
- **Implement:** display repository, release policy/tag, target, archive, destination, checksum source, and replacement behavior before asking consent.
- **Implement:** post-install recheck; refuse to claim readiness when provider remains unresolved.
- **Acceptance:** 4.1 passes; existing known package-manager offers remain unchanged; no silent installation is possible.

### [x] 4.3 Add RED tests for decline/failure guidance and cleanup

- **Test:** canonical release URL and manual instructions are shown after decline/network/error/unsupported platform.
- **Test:** recipe state/configuration does not contain a broken unresolved marker after failure.
- **Evidence:** focused tests fail before implementation.

### [x] 4.4 Implement actionable failure and rollback reporting

- **Files:** dependency/configuration modules and diagnostics tests.
- **Acceptance:** 4.3 passes; errors are bounded, secret-safe, and name the next manual action.

## Phase 5 — Managed command resolution and MCP materialization

### [x] 5.1 Add RED tests for `{dep:jinna}` resolution

- **Files:** `tests/test_recipe_materialize.py` or focused materialization tests.
- **Test:** PATH candidate resolves to `jinna`.
- **Test:** verified managed candidate resolves to its absolute executable path.
- **Test:** missing provider in non-interactive mode omits the provider MCP entry and emits one actionable warning; marker never reaches output.
- **Evidence:** focused tests fail before implementation.

### [x] 5.2 Implement structured dependency marker resolution

- **Files:** `lib/_internal/recipe-materialize.py`, shared dependency resolver.
- **Implement:** resolve only `{dep:jinna}` to structured provider output before writing recipe MCP JSON; reject unresolved/unknown markers.
- **Acceptance:** 5.1 passes; unrelated MCPs and manifest precedence remain unchanged.

### [x] 5.3 Add RED tests for runtime-specific MCP output

- **Test:** Claude, Cursor, Pi, and OMP generic local MCP shape; OpenCode local command array `[resolved_command, "mcp"]`.
- **Test:** env references remain safe in each renderer; no absolute developer path from the catalog appears.
- **Test:** unrelated runtime config keys survive.
- **Evidence:** focused tests fail before implementation.

### [x] 5.4 Implement materialization integration

- **Files:** `lib/_internal/recipe-materialize.py`, `lib/_internal/mcp-render.py` only if required by the resolved-command seam.
- **Implement:** provider MCP preset with resolved command and `mcp` arg; preserve current translators and manifest precedence.
- **Acceptance:** 5.3 passes; missing provider cannot produce a broken marker or guessed shell command.

## Phase 6 — Catalog recipe assets

### [x] 6.1 Add RED catalog/recipe contract tests

- **Files:** new focused test module or existing catalog recipe tests.
- **Test:** metadata/id/version, exact dependency declaration, `jinna` MCP marker/args/timeout, env refs, no hooks, no literal secrets, and all referenced assets.
- **Test:** docs/catalog table consistency and runtime conflict behavior.
- **Evidence:** focused tests fail before assets are added.

### [x] 6.2 Add `catalog/recipes/jinna-mcp-recipe/recipe.toml`

- **Implement:** provider dependency installer fields, local MCP preset, safe OpenProject env references, init prompt, bundled skill/docs references.
- **Acceptance:** 6.1 schema/contract tests pass; no binary/source/absolute path is committed.

### [x] 6.3 Add read-only `init.md`

- **Implement:** explain provider binary `jinna`, remote OpenProject, required env values, provider detection/install offer, and next commands. Init MUST remain read-only and MUST NOT download.
- **Acceptance:** init renders correctly through `ai-specs recipe init` and never mutates files.

### [x] 6.4 Add bundled README and skill

- **Implement:** end-user quick start from GitHub Releases, supported platforms, checksum/managed cache behavior, PATH versus managed path, configuration, troubleshooting, rollback, and local versus official remote MCP boundary.
- **Implement:** explicitly state that Go/Mise/Python/Git are not user prerequisites and that `jinna-client` is reference-only.
- **Acceptance:** documentation review finds no ambiguous “Jinna in PATH” wording, secret literals, automatic fallback, or unsupported install promise.

## Phase 7 — Documentation and project integration

### [x] 7.1 Add RED docs/schema tests

- **Test:** recipe schema documentation includes GitHub Release installer fields and safety semantics.
- **Test:** catalog documentation lists `jinna-mcp-recipe`, provider dependency, and local MCP surface.
- **Evidence:** tests fail before docs are updated.

### [x] 7.2 Update `docs/recipe-schema.md` and catalog docs

- **Files:** `docs/recipe-schema.md`, `docs/recipes-catalog.md`, relevant README sections.
- **Implement:** document new installer declaration, allowlist, non-TTY behavior, consent, managed path, checksum verification, and compatibility guarantees.
- **Acceptance:** 7.1 passes; existing docs remain accurate.

### [x] 7.3 Add optional provider smoke harness

- **Files:** focused test/helper fixture only; no credentials committed.
- **Implement:** opt-in `jinna version` and minimal MCP initialize/tools-list smoke using a supplied binary/release fixture; OpenProject API health call remains optional.
- **Acceptance:** default tests do not require GitHub/OpenProject; smoke failure is distinct from deterministic recipe failure.

## Phase 8 — Full validation and handoff

### [x] 8.1 Run focused RED/GREEN evidence

- **Run:** schema, dependency, installer, archive, materialization, and recipe-specific tests after each work unit.
- **Record:** exact commands and RED/GREEN outcomes in `verify-report.md` or the change handoff.
- **Acceptance:** no task is marked complete without passing focused evidence.

### [x] 8.2 Run repository validation

- **Run:** `./tests/validate.sh` and `./tests/run.sh` from the recipe worktree.
- **Check:** Python compile, Bash syntax, full unittest suite, recipe catalog/schema round trips, docs/catalog consistency, and secret scans.
- **Acceptance:** all configured checks pass; unavailable quality signals are explicitly reported.

### [x] 8.3 Run provider-backed installer verification

- **Depends on:** 0.2.
- **Run:** isolated install against the real provider release for the host target; inspect checksum/receipt/path and run `jinna version`; validate optional MCP smoke if configured.
- **Acceptance:** no Go/Mise/Python/Git required by the installation path; all five asset names/checksum entries are contract-compatible.

### [x] 8.4 Compare implementation against every spec scenario

- **Review:** `specs/jinna-mcp-recipe/spec.md`, `design.md`, all task acceptance criteria, and existing recipe behavior.
- **Check:** no automatic official `/mcp` fallback, no secret literals, no arbitrary downloads, no unrelated recipe regressions.
- **Acceptance:** scenario matrix is complete and unresolved provider readiness/security issues are blocking, not hidden.

### [x] 8.5 Prepare review handoff

- **Record:** provider release/tag consumed, target/checksum evidence, install receipt shape, runtime materialization outputs, rollback behavior, known limitations, and test commands.
- **Create/complete:** `verify-report.md` for Standard/Full pre-merge evidence.
- **Acceptance:** reviewers can evaluate the change without reconstructing provider behavior from chat.

### [x] 8.6 Archive before merge

- **Depends on:** 8.5 and explicit delivery approval.
- **Implement:** archive `openspec/changes/jinna-mcp-recipe/` at `openspec/changes/archive/YYYY-MM-DD-jinna-mcp-recipe/` using the actual ISO date only after verify evidence and before merge.
- **Acceptance:** pre-merge guardian passes after archive; no direct push to `development`; delivery follows the GitHub PR workflow.

## Phase 9 — Declared environment value validation (review-found defect)

The live verification run after task 8.4 proved that `configure-recipes` persisted
`OPENPROJECT_AUTH=basicc` without validation, and the failure only surfaced later inside the provider
(`configuration error (OPENPROJECT_AUTH): must be 'basic' or 'bearer'`). Phase 9 closes that gap and is
controlled by Requirement 11.

### [x] 9.1 Add RED tests for declared allowed values

- **Files:** `tests/test_env_scaffold.py`.
- **Test:** collect declared values from a preset `env_allowed`; ignore undeclared, disabled, and malformed declarations.
- **Evidence:** 4 tests fail before implementation (`collect_env_allowed` absent; a string declaration became `['b','a','s','i','c']`; a non-iterable raised `TypeError`).

### [x] 9.2 Implement recipe-declared allowed values and constrained prompting

- **Files:** `lib/_internal/env_scaffold.py`.
- **Implement:** shared `_mcp_env_declarations` traversal, `collect_env_allowed`, and a `questionary.select` prompt for constrained variables whose default is reconciled case-insensitively against the declared values.
- **Acceptance:** existing `collect_env_vars` purpose text and composition stay unchanged; no `recipe_schema.py` change is required because `_parse_mcp` already stores arbitrary preset keys.

### [x] 9.3 Add RED tests and an early warning for an out-of-set configured value

- **Files:** `tests/test_doctor.py`, `lib/_internal/doctor.py`.
- **Implement:** a `harness-env-value` WARN naming the variable and its allowed values, case-insensitive, that never echoes the configured value.
- **Evidence:** RED before implementation, then green; a case-sensitive mutation makes the negative test fail, proving it is not vacuous.

### [x] 9.4 Document the declaration and harden malformed input

- **Files:** `catalog/recipes/jinna-mcp-recipe/recipe.toml`, `docs/recipe-schema.md`, `lib/_internal/env_scaffold.py`.
- **Implement:** declare `env_allowed = { OPENPROJECT_AUTH = ["basic", "bearer"] }`, document the key beside the MCP preset docs, and fail open on malformed declarations.
- **Acceptance:** parent review found both malformed-input defects and the corrective tests pin them; 9 focused tests pass.

## Suggested reviewable work units

When Apply is authorized, prefer these commits/PR slices:

1. Dependency schema and structured provider resolution tests/implementation.
2. GitHub Release metadata, target selection, checksum/archive verification, and atomic installer.
3. Consent/configuration lifecycle and managed command resolution.
4. MCP materialization changes and cross-runtime tests.
5. Catalog recipe assets and user documentation.
6. Full verification report and archive-tail preparation.
7. Declared environment value validation (`env_allowed`): collection, constrained prompt, early doctor warning, and documentation.

Do not commit or push from worker tasks. The parent owns review, commits, PR creation, and delivery authorization.

## Tracker

Create or link the Trello card before Apply/production implementation and record `card_id + url` in `proposal.md` before any production write. Planning artifact writes are not gated by this requirement.

## Forbidden work in this planning phase

- Do not create catalog recipe files or production Python code until Apply is explicitly approved.
- Do not modify `jinna-provider` from this change.
- Do not download or install a provider while writing or reviewing these artifacts.
- Do not weaken checksum, archive, TTY-consent, or provider-readiness gates to accommodate a missing release.
- Do not add an automatic OpenProject official `/mcp` fallback or write-replay path.
