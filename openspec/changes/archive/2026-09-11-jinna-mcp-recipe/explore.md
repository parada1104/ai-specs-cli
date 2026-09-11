# Exploration: `jinna-mcp-recipe`

## Scope

This change adds a user-facing ai-specs recipe for the OpenProject provider maintained in the separate `parada1104/jinna-provider` repository. The recipe is an installer and configurator: it detects the provider executable, offers a consented installation from a verified GitHub Release when the executable is unavailable, and configures the local MCP server for enabled ai-specs runtimes.

This exploration is intentionally based on the provider's current repository contract and the existing ai-specs recipe/dependency architecture. It does not implement the recipe or change the provider.

## User outcome

A user should be able to enable the recipe without installing Go, Mise, Python, Git, or CI tooling for the provider. The recipe should:

1. Detect the local provider executable `jinna` through the operating system `PATH`.
2. Validate the discovered executable with `jinna version` and the recipe's supported version policy.
3. Reuse a compatible installation.
4. If `jinna` is absent or incompatible, offer an explicit installation from `https://github.com/parada1104/jinna-provider` GitHub Releases.
5. Select the archive for the current OS and CPU architecture, verify its SHA-256 checksum, and install it into a managed location without overwriting an unrelated executable.
6. Make the installed provider discoverable to the generated MCP configuration, or use an explicit managed executable path when the host does not inherit the updated `PATH`.
7. Configure the local stdio MCP command `jinna mcp` and pass OpenProject settings by environment-variable references.
8. Explain the required remote OpenProject URL and API token without writing secret values into generated artifacts.

If the user declines installation, the recipe must stop or remain visibly incomplete with actionable manual instructions; it must never silently continue with a broken MCP configuration.

## Repository and provider findings

### Provider repository

- Canonical repository: `https://github.com/parada1104/jinna-provider`.
- Product boundary: the provider is a Go project; its user-facing executable is named `jinna`.
- Local MCP entrypoint: `jinna mcp` over newline-delimited JSON-RPC stdio.
- OpenProject Self-Hosted remains a remote service configured through `OPENPROJECT_BASE_URL`; it is never an executable dependency.
- `jinna-client` is a read-only Python reference and is not an end-user prerequisite.
- The provider's planned distribution is versioned GitHub Release archives for Linux amd64/arm64, macOS amd64/arm64, and Windows amd64, with a SHA-256 manifest and release metadata.
- The provider does not proxy or automatically switch to the official OpenProject `/mcp` endpoint. The recipe must preserve that boundary.

The provider repository and first release are being finalized in parallel. The recipe must depend on a stable release contract (repository, tag/version, target naming, archive layout, and checksum manifest), not on an implementation worktree or an unpublished absolute path.

### ai-specs-cli dependency infrastructure

- `catalog/recipes/*/recipe.toml` declares recipes, MCP presets, docs, skills, configuration fields, and `[[deps.cli]]` dependencies.
- `[[deps.cli]]` currently describes a binary, purpose, optional install guidance URL, version check, and minimum version.
- `ai-specs doctor` checks declared binaries through `shutil.which` and reports missing required dependencies as warnings.
- The interactive recipe configuration flow offers installation only for a small hard-coded set of Homebrew/apt binaries. Unknown binaries and guidance-only binaries receive instructions but are not downloaded.
- Non-TTY detection is intentionally safe: dependency checks do not install anything. Interactive installation requires explicit confirmation.
- The existing schema does not yet express a verified cross-platform GitHub Release installer, a repository allowlist, an artifact version policy, or an install destination.
- Existing dependency checks parse a version from `version_check`; unparseable versions are currently treated as non-blocking/unknown. The recipe must decide whether that behavior is sufficient for a required provider binary.

This means a catalog-only recipe cannot yet satisfy the requested user outcome. The change needs a bounded extension to the ai-specs dependency-install seam, plus the new recipe assets and integration tests.

## Existing recipe shape to preserve

Recipes commonly contain:

```text
catalog/recipes/<id>/
├── recipe.toml
├── init.md                 # optional init brief
├── README.md               # optional copied operator documentation
├── skills/<id>/SKILL.md   # optional bundled skill
└── commands/*.md          # optional commands
```

The provider recipe should primarily use:

- one `[[deps.cli]]` entry for the provider binary;
- one `[init]` prompt for installation/configuration guidance;
- one `[[provides.mcp]]` entry for `jinna mcp`;
- environment references for `OPENPROJECT_BASE_URL`, `OPENPROJECT_API_TOKEN`, and optional `OPENPROJECT_AUTH`;
- a bundled skill and README with the install, configuration, security, and troubleshooting flow;
- no runtime hook unless an existing lifecycle seam is proven necessary.

The MCP preset must not embed a token, a server URL, a local absolute path, a shell pipeline, or an automatic remote `/mcp` fallback.

## Proposed lifecycle boundary

```text
ai-specs recipe configure/init
          │
          ├─ dependency check: command -v jinna + jinna version
          │       │
          │       ├─ compatible → reuse
          │       └─ missing/incompatible → explicit GitHub Release install offer
          │                                  │
          │                                  ├─ detect OS/arch
          │                                  ├─ fetch release metadata/artifact
          │                                  ├─ verify checksum and archive safety
          │                                  └─ install into managed directory
          │
          ├─ validate provider discovery again
          ├─ configure OpenProject env references
          └─ materialize `jinna mcp` for enabled runtimes
```

The installer should be a narrow package-aware extension, not a general-purpose remote shell or arbitrary URL executor. It must not run provider installation during passive `doctor`, non-interactive sync, or an unrelated recipe operation.

## Candidate technical seams

1. **Manifest schema**
   - Extend `CliDep` only with fields necessary to select a constrained installer, such as an explicit installer kind and a repository/artifact source.
   - Preserve compatibility for all existing recipes and reject unknown installer values.
   - Keep the GitHub repository allowlisted and represented as data, not assembled from arbitrary user input.

2. **Install-plan resolver**
   - Keep existing Homebrew/apt behavior unchanged.
   - Add a GitHub Release plan for the provider that is resolved only from the validated recipe declaration.
   - Separate plan preview from execution so TTY confirmation, network access, checksum verification, and filesystem mutation are independently testable.

3. **Release installer**
   - Use HTTPS and a bounded GitHub Releases API/artifact flow without requiring `gh`, Go, or a package manager.
   - Select only the current supported target.
   - Verify the signed-by-content SHA-256 manifest before extracting.
   - Reject missing, duplicate, unexpected, or path-traversal archive entries.
   - Stage into a temporary directory and atomically rename only after all checks pass.
   - Preserve an existing user-owned binary; report the managed path if a PATH update is needed.

4. **Recipe materialization**
   - Keep the MCP command as the provider-owned `jinna mcp` interface.
   - Prefer a command name that resolves after installation; if the runtime cannot see the updated PATH, use the validated managed path through the existing manifest mechanism rather than guessing.
   - Pass only env references; never render secret values into `ai-specs.toml`, generated runtime config, docs, or logs.

5. **Documentation and diagnostics**
   - `doctor` remains non-mutating and reports whether the provider is found, its version, and the official GitHub installation guidance.
   - Interactive configure/init explains the proposed install, target, destination, version policy, and checksum verification before asking for consent.
   - Declining or failing installation produces a clear next action and does not claim the MCP is ready.

## Initial assumptions

- The recipe id remains `jinna-mcp-recipe` and the provider executable remains `jinna`.
- GitHub Releases are the authoritative end-user distribution channel; source builds are maintainer/developer fallback documentation only.
- The first supported target set is Linux amd64/arm64, macOS amd64/arm64, and Windows amd64.
- The recipe configures the local direct-API MCP. The official OpenProject remote `/mcp` endpoint is documented as a separate operator choice, never automatically proxied, selected, or used as a write fallback.
- OpenProject configuration uses environment references; the recipe may scaffold names/help but must not persist token values.
- Installation is opt-in and interactive. Passive checks and non-TTY paths remain guidance-only.
- A release must expose a stable, machine-checkable artifact naming and checksum contract before the recipe's installer can be accepted.

## Open decisions for proposal/design

1. Whether an existing compatible binary outside the current `PATH` can be adopted, or whether the first version strictly requires `command -v jinna`.
2. Whether the default release policy is an explicit stable version pin, a configured version, or the latest stable release with recorded metadata.
3. Whether the managed install directory is project-local, user-local, or selected by the existing ai-specs installation model; the choice must work across runtime hosts and Windows.
4. How generated MCP configurations refer to a managed executable when a host process does not inherit a modified `PATH`.
5. Exact provider archive names and `SHA256SUMS`/`RELEASE.json` schema to freeze against the real first GitHub Release.
6. Whether provider version validation should block configuration on an unknown/unparseable version or remain consistent with the existing guidance-only semantics.
7. Whether GitHub API rate-limit/authentication failures should fall back only to explicit manual instructions (never to an arbitrary mirror or automatic source build).

## Non-goals discovered

- Reimplementing the provider or its OpenProject HTTP/MCP behavior in ai-specs-cli.
- Installing Go, Mise, Python, GitHub CLI, or a package manager for end users.
- Embedding the provider source tree or an absolute developer path in the catalog.
- Installing through an arbitrary URL, shell command, unverified archive, or silent background action.
- Automatically enabling or selecting OpenProject's official `/mcp` endpoint.
- Retrying or replaying failed OpenProject writes through a second MCP path.
- Adding Homebrew, Scoop, apt, container, or package-manager publishing for the provider in this change.
- Requiring live OpenProject credentials or a live OpenProject instance to validate recipe materialization.

## Exploration conclusion

The requested recipe is feasible, but it is not a manifest-only addition. The existing recipe dependency model can detect `jinna` and show guidance, while the requested experience requires a secure, opt-in GitHub Release installer plus a catalog recipe and runtime materialization tests. The next phase should freeze the minimal installer contract, destination/version policy, provider release assumptions, and rollback behavior before implementation.
