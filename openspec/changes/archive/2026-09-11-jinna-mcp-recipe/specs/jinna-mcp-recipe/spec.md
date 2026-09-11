# Specification: Provider installation and MCP recipe integration

## Conventions

The requirements below use RFC 2119 terminology. `provider` means the Go project `parada1104/jinna-provider`; `provider binary` means its local executable `jinna`; `OpenProject` means the remote Self-Hosted service; and `MCP host` means an AI runtime that launches the local stdio server.

The scenarios are normative acceptance examples. They assume the provider release contract is available when a scenario requires a download. Provider readiness is an external prerequisite for Apply/Verify, not a reason to weaken these requirements.

## Requirement 1: Declare the provider dependency

The recipe MUST declare the provider binary as a required CLI dependency and MUST identify its constrained GitHub Release installer.

The declaration MUST include:

- binary name `jinna`;
- a human purpose explaining that it runs the local OpenProject provider MCP server;
- a bounded version check using `jinna version`;
- a minimum supported provider version;
- the release guidance URL;
- installer kind `github-release`;
- repository `parada1104/jinna-provider`;
- stable-release policy `latest-stable`.

The recipe MUST declare the local MCP preset with command marker `{dep:jinna}`, argument `mcp`, and environment references for `OPENPROJECT_BASE_URL`, `OPENPROJECT_API_TOKEN`, and optional `OPENPROJECT_AUTH`.

### Scenario: Valid provider dependency is parsed

- **Given** the catalog contains `jinna-mcp-recipe/recipe.toml`
- **When** the recipe schema is loaded
- **Then** it MUST expose the required `jinna` dependency and the `github-release` declaration
- **And** it MUST expose a local MCP preset whose args contain only `mcp`
- **And** it MUST expose only environment references, not token literals or a provider source path.

### Scenario: Legacy recipes remain compatible

- **Given** an existing recipe has legacy `[[deps.cli]]` fields only
- **When** the schema is parsed and serialized
- **Then** the recipe MUST retain its current behavior and serialized fields
- **And** no existing dependency installer kind or recipe MUST require new GitHub fields.

### Scenario: Invalid installer declarations are rejected

- **Given** a recipe declares an unknown installer kind, malformed repository, missing repository, unsupported release policy, or a repository other than the allowlisted provider
- **When** the recipe is validated
- **Then** validation MUST fail with an actionable field-specific error
- **And** no installer plan MUST be created.

## Requirement 2: Detect and reuse the provider

The dependency resolver MUST check the provider binary before offering installation.

A PATH candidate is acceptable only when the executable can be invoked successfully with `jinna version` and reports a parseable version at least equal to the declared minimum. The resolver MUST prefer a compatible PATH candidate and MUST NOT overwrite it.

The resolver MUST also inspect a verified managed candidate recorded by ai-specs. Managed candidates MUST be revalidated for executable status, receipt integrity, binary digest, and version before use.

### Scenario: Compatible PATH provider is reused

- **Given** `command -v jinna` resolves to an executable
- **And** `jinna version` exits successfully with a compatible version
- **When** the recipe dependency gate runs
- **Then** the resolver MUST report source `path`
- **And** it MUST not download, install, or replace any provider bytes
- **And** MCP materialization MUST use the command `jinna`.

### Scenario: Missing provider is identified

- **Given** no compatible PATH or managed provider exists
- **When** the dependency gate runs
- **Then** it MUST report the provider as required and unresolved
- **And** it MUST include the GitHub Release guidance and the selected host target in the interactive offer
- **And** it MUST not claim the MCP integration is ready.

### Scenario: Incompatible PATH provider is preserved

- **Given** `jinna` exists on PATH but its version is below the minimum or its version command fails
- **When** the dependency gate runs
- **Then** it MUST report the incompatibility
- **And** it MUST not modify or delete that executable
- **And** it MAY offer a separate managed installation after explicit consent.

## Requirement 3: Offer an explicit GitHub Release installation

The interactive recipe dependency flow MUST offer installation only on a real interactive TTY and only after displaying a complete plan. The plan MUST state the allowlisted repository, stable release policy/tag, selected OS/architecture, expected archive, destination, checksum source, and whether an existing candidate will remain untouched.

The installer MUST NOT require Go, Mise, Python, GitHub CLI, or a package manager on the user's machine.

Declining the offer MUST perform no download or filesystem mutation and MUST leave actionable manual instructions. Installation MUST never be silently triggered by `doctor`, `sync`, non-TTY execution, or passive dependency inspection.

### Scenario: User accepts a supported installation

- **Given** no compatible provider is available
- **And** the user is interacting through a TTY
- **When** the user explicitly accepts the displayed GitHub Release installation plan
- **Then** the installer MUST acquire only the selected provider release artifact
- **And** it MUST verify the artifact before publication
- **And** it MUST recheck the installed provider version before the recipe proceeds.

### Scenario: User declines installation

- **Given** the interactive plan is displayed
- **When** the user declines
- **Then** the installer MUST not contact GitHub or write a provider file
- **And** the recipe MUST report the provider as unresolved
- **And** it MUST show the canonical release URL and manual installation guidance.

### Scenario: Non-interactive execution cannot install

- **Given** `doctor`, `sync`, CI, or another command is running without an interactive TTY
- **When** the provider is absent
- **Then** no network download or installation MUST occur
- **And** the command MUST emit a warning or actionable unresolved-dependency result according to the existing command contract.

## Requirement 4: Select and verify the provider release

The GitHub installer MUST use HTTPS, bounded requests, and the allowlisted repository only. It MUST select a non-draft, non-prerelease stable release under the declared policy.

For the host target it MUST derive the expected artifact name exactly as follows:

- `jinna_<version>_linux_amd64.tar.gz`;
- `jinna_<version>_linux_arm64.tar.gz`;
- `jinna_<version>_darwin_amd64.tar.gz`;
- `jinna_<version>_darwin_arm64.tar.gz`;
- `jinna_<version>_windows_amd64.zip`.

It MUST require the matching `SHA256SUMS` entry, compare the downloaded archive digest using SHA-256, and reject malformed, missing, duplicated, or mismatched entries. It MUST not accept a user-supplied arbitrary URL or filename.

### Scenario: Correct host artifact is selected

- **Given** the latest stable release lists all supported provider artifacts
- **And** the host maps to `darwin/arm64`
- **When** the installer resolves the release
- **Then** it MUST select `jinna_<version>_darwin_arm64.tar.gz`
- **And** it MUST not select a different architecture, prerelease, or draft asset.

### Scenario: Unsupported platform is rejected

- **Given** the host is not one of the five supported target pairs
- **When** installation is requested
- **Then** the installer MUST not download or execute an artifact
- **And** it MUST report the supported targets and a manual guidance URL.

### Scenario: Checksum mismatch is rejected

- **Given** the selected archive's digest differs from its `SHA256SUMS` entry
- **When** verification runs
- **Then** the archive MUST be deleted or quarantined before extraction
- **And** no binary MUST be executed or installed
- **And** the prior provider installation MUST remain unchanged.

## Requirement 5: Validate archive contents and install atomically

The installer MUST stage downloads and extraction in a temporary directory on the destination filesystem. It MUST reject absolute paths, parent traversal, duplicate members, symlinks, hardlinks, unexpected executables, and files outside the expected archive root.

The expected archive MUST contain the provider executable (`jinna` or `jinna.exe`) and the documented release files. The installer MUST set Unix executable mode `0755` and MUST run the extracted executable's version command before publication.

Publication MUST be atomic: a failed download, verification, extraction, permission change, self-check, or receipt write MUST leave the previous managed version and any user-owned PATH executable untouched.

### Scenario: Valid archive is installed

- **Given** a verified archive with the expected root and provider files
- **When** extraction and self-check complete successfully
- **Then** the provider MUST be published under the managed user cache
- **And** a non-secret verification receipt MUST be written atomically
- **And** the resolver MUST return the concrete managed executable path.

### Scenario: Archive traversal is rejected

- **Given** an archive contains `../outside`, an absolute member, a link, or a duplicate member
- **When** archive inspection runs
- **Then** installation MUST fail before executing any member
- **And** the temporary extraction directory MUST be removed
- **And** no destination file outside the staging directory may change.

### Scenario: Existing managed version survives a failed replacement

- **Given** a verified managed provider version already exists
- **When** a later acquisition fails at any stage
- **Then** the existing version MUST remain executable and recorded
- **And** the failed candidate MUST not replace it
- **And** the error MUST identify the failed stage without exposing secrets.

## Requirement 6: Resolve the provider command for MCP materialization

The recipe MUST use a reserved `{dep:jinna}` command marker rather than embedding a developer-specific path in the catalog.

Before writing the recipe MCP temporary JSON, materialization MUST resolve the marker to:

1. `jinna` when a compatible PATH candidate is available;
2. the absolute verified managed path when the provider was acquired by ai-specs;
3. no provider MCP entry plus one actionable warning when no candidate is available in a non-interactive flow.

The marker MUST never reach generated runtime configuration. The resolver MUST return structured provider resolution data, not arbitrary shell text.

### Scenario: Managed provider is materialized

- **Given** a verified managed provider exists outside the user's PATH
- **When** recipe materialization runs
- **Then** the generated local MCP configuration MUST contain its concrete executable path and `mcp` argument
- **And** it MUST not contain `{dep:jinna}` or an unresolved placeholder.

### Scenario: Missing provider does not create a broken MCP command

- **Given** no provider candidate is available during non-interactive sync
- **When** materialization runs
- **Then** it MUST omit the unresolved provider MCP entry rather than emit a marker or guessed command
- **And** `doctor` MUST report the required provider and installation guidance.

### Scenario: Runtime-specific translation remains correct

- **Given** a resolved provider command and env references
- **When** MCP configuration is rendered for Claude, Cursor, OpenCode, Pi, and OMP
- **Then** each runtime MUST receive its existing local-MCP shape
- **And** OpenCode MUST receive a local command array `[resolved_command, "mcp"]`
- **And** no runtime file MUST contain literal API-token values.

## Requirement 7: Configure OpenProject safely

The recipe MUST configure the provider with environment references only:

```text
OPENPROJECT_BASE_URL=$OPENPROJECT_BASE_URL
OPENPROJECT_API_TOKEN=$OPENPROJECT_API_TOKEN
OPENPROJECT_AUTH=$OPENPROJECT_AUTH
```

The recipe MAY scaffold names and help text through the existing env flow, but it MUST NOT persist token values in `ai-specs.toml`, generated runtime configuration, command arguments, docs, receipts, logs, or committed `.env` files.

Provider installation MUST not call OpenProject. A credential-dependent provider health or MCP smoke check MUST be optional and separate from deterministic recipe installation tests.

### Scenario: Environment example is generated

- **Given** the recipe is enabled
- **When** ai-specs generates its environment example
- **Then** the required OpenProject variable names MUST appear with safe placeholders/help
- **And** no secret value or server credential MUST be copied from the process into a committed artifact.

### Scenario: Installation works without OpenProject credentials

- **Given** a public provider release is available but no OpenProject env values are configured
- **When** the provider install flow runs
- **Then** installation and `jinna version` verification MUST succeed or fail independently of OpenProject connectivity
- **And** the recipe MUST report credential setup as a separate next step.

## Requirement 8: Preserve protocol and operational boundaries

The recipe MUST configure only the local direct-API MCP `jinna mcp`. Documentation MUST distinguish:

- local provider binary `jinna`;
- remote OpenProject Self-Hosted service;
- Python `jinna-client` reference implementation;
- official OpenProject remote `/mcp` endpoint.

The recipe MUST NOT automatically proxy, select, or fall back to the official remote endpoint. It MUST NOT replay failed OpenProject writes through another MCP path. The official endpoint MAY be documented as an operator-selected alternative with its independent requirements.

### Scenario: Local provider is selected explicitly

- **Given** the recipe is enabled and the provider is installed
- **When** MCP configuration is materialized
- **Then** it MUST use `jinna mcp` and direct APIv3 semantics
- **And** it MUST not add an official remote `/mcp` server automatically.

### Scenario: Local write failure remains local

- **Given** a local provider write returns an error
- **When** the runtime reports that error
- **Then** no recipe hook or installer MUST issue a second write through the official remote MCP
- **And** the error MUST remain visible to the user for explicit follow-up.

## Requirement 9: Preserve compatibility and explain recovery

The change MUST preserve existing recipes, manifest keys, runtime configuration outside the owned MCP namespace, and non-provider dependency behavior.

The recipe README, init prompt, and bundled skill MUST document installation, supported platforms, checksum verification, managed path behavior, provider version checks, environment setup, troubleshooting, and rollback. Installation failures MUST expose an actionable release URL and MUST not claim readiness.

### Scenario: Existing runtime settings survive materialization

- **Given** a runtime config contains unrelated user-owned keys
- **When** the recipe MCP namespace is rendered
- **Then** unrelated keys MUST remain unchanged
- **And** only the owned MCP namespace may be replaced according to existing renderer rules.

### Scenario: Recipe removal does not delete provider data

- **Given** the provider was installed into the managed cache
- **When** the recipe is disabled or removed
- **Then** the provider binary MUST remain unless the user explicitly requests managed-artifact cleanup
- **And** no OpenProject data or user-owned PATH executable may be deleted.

## Requirement 10: Evidence and provider readiness gate

The implementation MUST provide deterministic unit, fixture, schema, installer, materialization, and documentation evidence. Network tests MUST use controlled fixtures or injected transports; the default suite MUST not require GitHub availability or OpenProject credentials.

Before Apply/Verify, the parent MUST confirm the provider readiness gate:

1. the public GitHub repository is accessible;
2. a stable release tag exists;
3. all five expected archives, `SHA256SUMS`, and `RELEASE.json` exist;
4. archive layout, version output, and checksums match this specification;
5. provider quality/security gates are green;
6. the host-target release artifact can be installed in an isolated directory without Go, Mise, Python, or Git.

### Scenario: Deterministic suite runs without live services

- **Given** a clean ai-specs checkout with test fixtures
- **When** `./tests/validate.sh` and `./tests/run.sh` execute without network credentials
- **Then** all recipe/schema/installer/materialization tests MUST pass
- **And** tests MUST prove no forbidden download or filesystem mutation on non-interactive/declined paths.

### Scenario: Provider readiness is incomplete

- **Given** the provider repository lacks a stable release, a target asset, a valid checksum, or a green security gate
- **When** Apply or Verify is requested
- **Then** the parent MUST report the provider prerequisite as blocked
- **And** no recipe result may claim end-to-end release installation is verified.

## Requirement 11: Validate declared environment values

A recipe MAY declare an accepted value set for an environment reference it materializes, keyed by the
same declaration key as `env` and expressed as `env_allowed` inside the `[[provides.mcp]]` preset.
Declared values are compared case-insensitively because providers accept their enumerated values in any
case.

The interactive configuration flow MUST NOT accept a value outside a declared set: a constrained
variable is prompted as a closed choice. `doctor` MUST report an out-of-set configured value as an early
warning instead of leaving the failure to the provider at runtime. Neither path MAY echo the configured
value, which may be a credential.

A malformed declaration (a value that is not a list, or a list whose entries are not non-empty strings)
MUST fail open: the variable stays unconstrained and no error is raised.

### Scenario: Constrained variable cannot take an out-of-set value

- **Given** an enabled recipe that declares `env_allowed` for one environment reference
- **When** the interactive configuration flow prompts for that variable
- **Then** the flow MUST offer only the declared values
- **And** the persisted value MUST be one of them.

### Scenario: Out-of-set configured value is reported early

- **Given** `ai-specs.env` holds a value outside the declared set for a constrained variable
- **When** `ai-specs doctor` runs
- **Then** the report MUST include a warning naming the variable and its allowed values
- **And** the warning MUST NOT contain the configured value.

### Scenario: Malformed declaration is ignored

- **Given** a recipe whose `env_allowed` entry is not a list of non-empty strings
- **When** the harness collects declared values
- **Then** the variable MUST remain unconstrained
- **And** no error MUST be raised in the configuration or doctor path.
