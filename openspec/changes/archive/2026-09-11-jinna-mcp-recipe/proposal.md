# Proposal: Install and configure the OpenProject provider recipe

## Status

Proposed after exploration. This proposal defines the product boundary and the implementation scope; the exact installer schema and filesystem contract belong to the design phase.

## Planning classification

- **Requested depth:** Full OpenSpec planning package.
- **Signal depth:** Full: this change extends the dependency installer, adds a cross-platform remote binary installation path, adds a new MCP recipe, changes user-facing setup behavior, and requires security/materialization evidence.
- **Decided depth:** Full.

## Problem

The Go OpenProject provider is distributed as the executable `jinna` from the separate public repository `parada1104/jinna-provider`. A recipe that only declares `jinna` as a CLI dependency can detect the executable and show a link, but the current ai-specs installer cannot securely install an unknown cross-platform binary from a GitHub Release. Users would have to discover the provider, choose an archive, verify it, install it, and configure every AI runtime manually.

The recipe must own that user-facing flow while keeping provider behavior and OpenProject data access in the provider repository.

## Goals

1. Add a catalog recipe named `jinna-mcp-recipe` for the local OpenProject provider.
2. Detect the provider binary by executable name and verify it with `jinna version`.
3. Reuse a compatible provider installation without overwriting it.
4. When the provider is missing or incompatible, present an explicit, understandable opt-in offer to install it from the allowlisted GitHub repository `parada1104/jinna-provider`.
5. Select only a supported OS/architecture artifact, verify the provider's SHA-256 manifest, reject unsafe archives, and publish the managed installation atomically.
6. Make the installed provider available to generated runtime configurations, including hosts that do not inherit a modified shell `PATH`.
7. Configure the local stdio MCP entrypoint `jinna mcp` for the enabled ai-specs runtimes.
8. Pass `OPENPROJECT_BASE_URL`, `OPENPROJECT_API_TOKEN`, and optional `OPENPROJECT_AUTH` as environment references, never as literal secret values.
9. Keep passive checks and non-interactive flows non-mutating; installation must require explicit user consent.
10. Provide deterministic recipe/schema/materialization tests and an optional smoke path that can run when a provider binary and OpenProject credentials are available.
11. Explain the boundary between the local provider and OpenProject's official remote `/mcp` endpoint; never proxy, auto-select, or replay writes between them.

## User experience

### Existing compatible provider

```text
User enables/configures recipe
  → ai-specs detects provider binary `jinna`
  → ai-specs runs `jinna version`
  → compatible binary is reused
  → user supplies/retains OpenProject env references
  → ai-specs materializes `jinna mcp`
```

### Missing or incompatible provider

```text
User enables/configures recipe
  → ai-specs reports provider missing/incompatible
  → show repository, selected target, version policy, destination,
    checksum verification, and consent question
  → user declines: stop or leave clearly incomplete with manual guidance
  → user accepts: fetch allowlisted GitHub Release metadata/artifact
  → verify HTTPS response, target, archive contents, SHA-256, and version
  → stage and atomically install provider binary
  → re-run `jinna version`
  → only after success materialize `jinna mcp`
```

The provider installer must not require Go, Mise, Python, GitHub CLI, a package manager, or source checkout on the user's machine. Go/Mise remain maintainer/build tools in the provider repository.

## Success Criteria

- The `jinna-mcp-recipe` catalog assets validate and materialize a local provider MCP entry without embedding provider source, developer paths, or secret literals.
- A missing or incompatible provider produces an explicit, allowlisted GitHub Release installation offer that selects the supported target, verifies checksums and archive safety, and publishes atomically only after consent.
- A compatible provider on `PATH` or in the verified managed cache is reused safely, while unresolved/non-interactive cases remain visibly incomplete and non-mutating.
- Enabled ai-specs runtimes receive the provider-owned `jinna mcp` command with correct runtime-specific translation and OpenProject environment references.
- Existing recipe behavior remains compatible and the change has deterministic schema, installer, materialization, documentation, and full-validation evidence.
- Provider-backed installation verification is claimed only after the real `parada1104/jinna-provider` release, five target assets, checksums, and security gates pass the readiness gate.

## In scope

### ai-specs dependency installation

- Extend the validated `[[deps.cli]]` model with a constrained, explicit installer description for a GitHub Release binary (exact field names to be frozen in design).
- Keep current Homebrew/apt plans for existing known CLIs unchanged.
- Add a provider-specific or generic allowlisted GitHub Release install plan that separates preview, consent, download, verification, extraction, and atomic installation.
- Keep doctor guidance-only and non-mutating.
- Keep non-TTY configure/sync flows non-mutating; no download or install without an interactive affirmative response.
- Preserve existing recipe compatibility and reject unknown installer kinds or unsafe repository/source values.

### `jinna-mcp-recipe` catalog assets

- `recipe.toml` with metadata, provider dependency declaration, installation guidance, local MCP preset, and environment references.
- `init.md` with the read-only setup brief and precise terminology.
- Bundled README and skill covering installation, provider version checks, OpenProject environment configuration, MCP host setup, troubleshooting, security, and the official remote-MCP alternative.
- No provider source, generated binary, credentials, absolute developer path, or arbitrary shell installer in the catalog.
- No runtime hook unless design proves an existing lifecycle hook is required; the normal dependency/configuration flow is preferred.

### Verification

- Schema and dependency-install unit tests for valid/invalid release declarations, repository allowlisting, target selection, version checks, consent/no-consent behavior, and failure cleanup.
- Archive fixture tests for the five provider target names, checksum mismatch, missing manifest entry, duplicate/unexpected files, path traversal, wrong target, corrupt archive, and atomic rollback.
- Recipe tests for catalog validation, MCP command/args/env shape, secret absence, docs/manifest consistency, and materialization for Claude, Cursor, OpenCode, Pi, and OMP.
- Optional provider smoke test that runs `jinna version` and, when explicitly configured, `jinna mcp` against a test OpenProject endpoint; no live credentials are required for the default suite.
- Documentation and validation evidence through `./tests/validate.sh` and the configured unit test command.

## Out of scope

- Reimplementing or modifying the Go provider, its API client, its MCP tool registry, or its release workflow.
- Installing Go, Mise, Python, Git, `gh`, or a package manager for end users.
- Building the provider from source as the normal recipe path.
- Arbitrary URL, arbitrary shell, arbitrary archive, or arbitrary repository installation.
- Silent downloads, background upgrades, or replacing a user-owned `jinna` without consent.
- Installing Homebrew/Scoop/apt packages or publishing provider packages outside the provider's GitHub Release artifacts.
- Automatically enabling, selecting, proxying, or falling back to OpenProject's official `/mcp` endpoint.
- Retrying a failed OpenProject write through a different MCP path.
- Storing API tokens in recipe files, generated manifests, runtime arguments, logs, or committed `.env` files.
- Requiring a live OpenProject instance for normal recipe validation.

## Acceptance criteria

- A fresh installation can discover the recipe and materialize it only from valid catalog assets.
- A compatible `jinna` already on `PATH` is detected, version-checked, and reused.
- A missing provider produces an explicit install offer that identifies the official GitHub repository and selected target; declining leaves no partial installation.
- An accepted installation verifies the expected release artifact and checksum before making the binary executable; any failed verification leaves the destination unchanged.
- The post-install command check succeeds before MCP configuration is considered ready.
- Generated runtime configurations invoke the provider-owned `jinna mcp` command, carry only env references, and contain no secret literals or absolute developer paths.
- `doctor` and non-interactive flows never mutate the filesystem or download artifacts.
- Existing recipes and dependency installers retain their current behavior.
- All supported runtime materializations are validated; local smoke is optional and clearly separated from deterministic tests.
- The recipe documents that OpenProject Self-Hosted is remote, the provider binary is local, and the Python client is reference-only.
- Rollback and failure handling are documented and tested; no OpenProject mutation is part of provider installation.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Release artifact or checksum is tampered with | Fixed GitHub repository allowlist, HTTPS, SHA-256 manifest verification, strict archive validation, atomic staging. |
| Release naming drifts from provider workflow | Freeze a machine-checkable provider release contract in design and add fixture tests against the real first-release layout. |
| Installer overwrites a user-managed binary | Detect/reuse compatible binaries; use a separate managed destination; require explicit replacement consent and preserve metadata. |
| Runtime cannot see a newly modified `PATH` | Validate the managed executable path and use the existing runtime command/path mechanism rather than assuming shell inheritance. |
| User accidentally exposes an API token | Use env references only, redact diagnostics, and never place token values in manifests or arguments. |
| Non-interactive automation becomes surprising or unsafe | Keep passive/CI/non-TTY paths guidance-only and fail closed when consent is unavailable. |
| A provider update breaks MCP behavior | Support an explicit version policy and retain prior managed artifacts/metadata for rollback. |
| Official and local MCP behaviors are conflated | Document and test explicit protocol selection; no automatic proxy, fallback, or write replay. |
| GitHub API limits or network failure | Report a bounded actionable error and manual release URL; do not fall back to arbitrary mirrors or source execution. |

## Rollback

- If the installer fails before atomic publication, remove only its temporary staging directory and leave the existing binary/configuration untouched.
- If a newly installed managed version is incompatible, select the previously recorded managed version or remove the managed provider through the normal recipe lifecycle; do not delete an unrelated user-owned binary.
- If materialization fails, preserve the prior manifest and report the provider/configuration error; do not attempt a second MCP transport automatically.
- Reverting this recipe change removes the installer/recipe code from ai-specs-cli but does not remove a successful provider binary or undo any OpenProject mutation. Provider binary removal is an explicit user action.

## Affected modules and files

- `lib/_internal/recipe_schema.py`: validated installer metadata while preserving legacy recipes.
- `lib/_internal/dep_check.py`: provider detection/version result semantics.
- `lib/_internal/dep_install.py` or a focused sibling installer module: constrained GitHub Release planning and execution.
- `lib/_internal/config_wizard.py` and the relevant recipe/configuration entrypoint: explicit consent and post-install recheck.
- `catalog/recipes/jinna-mcp-recipe/**`: manifest, init brief, docs, skill, and any minimal test fixture.
- `tests/test_recipe_schema.py`, dependency installer tests, recipe/materialization tests, and new focused archive/security fixtures.
- `docs/recipe-schema.md`, `docs/recipes-catalog.md`, and user-facing README sections where the dependency installer contract changes.

## Tracker

- card_id: `6aa21ffc2e72c0ce4b22a75b`
- url: https://trello.com/c/KV5tMVug/122-recipe-install-and-configure-openproject-provider
- list: In Progress

## Delivery boundary

This change is ready for implementation only after the design freezes the release artifact contract, install destination/path behavior, version policy, installer schema, and failure/rollback details. The provider's separate GitHub release must expose the agreed archive/checksum contract before an end-to-end recipe installation can be accepted.
