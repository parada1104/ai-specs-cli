# Design: Provider acquisition and MCP recipe integration

## Design status

This design assumes that the provider repository is made release-ready before implementation verification. The implementation/apply and verify phases MUST re-check the provider readiness gate listed below; a valid recipe design does not authorize consuming an unpublished or security-blocked provider release.

## Architecture overview

```text
┌──────────────────────────────┐
│ catalog/recipes/jinna-mcp... │
│ recipe.toml + docs + skill   │
└──────────────┬───────────────┘
               │ validated CliDep: github-release
               ▼
┌──────────────────────────────┐
│ ai-specs dependency layer    │
│ detect → plan → consent      │
│ download → verify → install  │
└──────────────┬───────────────┘
               │ concrete executable path
               ▼
┌──────────────────────────────┐
│ MCP materializer              │
│ resolve {dep:jinna} marker   │
│ preserve env references       │
└──────────────┬───────────────┘
               │ generated runtime config
               ▼
┌──────────────────────────────┐
│ local `jinna mcp` process     │
│ direct OpenProject APIv3      │
└──────────────┬───────────────┘
               │ HTTPS + API token
               ▼
       Remote OpenProject Self-Hosted
```

The recipe does not contain provider source or a binary. It consumes a versioned release contract owned by `parada1104/jinna-provider`.

## Architecture decisions

### D1. Use an explicit GitHub Release installer kind

Extend `CliDep` with a constrained installer declaration while preserving all legacy fields:

```toml
[[deps.cli]]
binary = "jinna"
purpose = "Run the local OpenProject provider MCP server"
required = true
install_url = "https://github.com/parada1104/jinna-provider/releases"
version_check = "jinna version"
min_version = "0.1.0"
installer = "github-release"
repository = "parada1104/jinna-provider"
release_policy = "latest-stable"
```

The parser MUST allow only the documented installer kinds (`""`/legacy and `"github-release"`), require `repository` and `release_policy` for the latter, and reject malformed or unknown values. The GitHub installer MUST allowlist the exact repository `parada1104/jinna-provider`; a recipe or user value cannot turn this feature into an arbitrary URL/repository executor.

Existing recipes with no new fields parse and behave unchanged. `install_url` remains the human-facing fallback link; it is not itself an executable command.

### D2. Use the provider's tagged release contract

The provider release contract is frozen against the first real release before apply/verify:

- Repository: `parada1104/jinna-provider`.
- Stable tag: `v<version>`; drafts and prereleases are excluded by `latest-stable`.
- Linux/macOS assets:
  - `jinna_<version>_linux_amd64.tar.gz`
  - `jinna_<version>_linux_arm64.tar.gz`
  - `jinna_<version>_darwin_amd64.tar.gz`
  - `jinna_<version>_darwin_arm64.tar.gz`
- Windows asset: `jinna_<version>_windows_amd64.zip`.
- Release assets also include `SHA256SUMS` and `RELEASE.json`.
- Each archive contains a root directory named after the archive stem, the executable `jinna` or `jinna.exe`, `LICENSE`, `README.md`, and the provider `docs/` files.
- `RELEASE.json` records the version, source commit, build date, source epoch, and artifact list. The installer MUST require this asset, bound its size, and reject it when it is not an object, when `version` disagrees with the stable tag, when `artifacts` is not an array of unique names, or when the selected archive is not listed.

The installer derives the expected target asset name from detected `(goos, goarch)` and the release version. It MUST NOT accept an arbitrary asset filename from user input. It obtains a stable release either from the GitHub API (`/repos/parada1104/jinna-provider/releases/latest`) or a future explicitly pinned tag using the same allowlisted repository. API and asset URLs MUST remain on `api.github.com`/`github.com` for this repository and use bounded HTTPS requests.

### D3. Prefer an existing compatible provider, otherwise use a managed cache

Resolution order:

1. `shutil.which("jinna")` finds a local executable.
2. Run its declared version command with a bounded timeout; for this provider the command MUST exit successfully and expose a parseable semantic version.
3. If the version is at least `min_version`, use the command name `jinna` and do not replace it.
4. If no compatible PATH executable exists, inspect a verified managed installation receipt under the ai-specs user cache and reuse that exact binary.
5. If neither candidate is usable, the interactive dependency flow creates a GitHub Release install offer.

A compatible PATH binary is never overwritten. An incompatible PATH binary is never modified; the user is told why it is not accepted and is offered a separate managed installation.

The managed layout is keyed by the exact release tag consumed from the allowlisted repository, matching the existing version-keyed binary acquisition pattern (`cache_root` is resolved as `<ai-specs-home>/cache/bin/jinna`):

```text
<ai-specs-home>/cache/bin/jinna/
└── <release-tag>/          # e.g. v0.1.0, exactly as published
    └── <goos>-<goarch>/    # e.g. darwin-arm64
        ├── jinna[.exe]
        └── install.json
```

There is no automatic "select an older version" mechanism: rollback is an explicit user action that removes or replaces a tag directory, or reinstalls a retained directory.

The user cache root SHOULD reuse the existing ai-specs binary-cache abstraction where possible. It MUST be user-writable and MUST NOT require administrator privileges. If the existing CLI home is not writable, the implementation MUST resolve the platform user-data/cache directory rather than writing into the installed package or project repository.

`install.json` is a local receipt containing only non-secret provenance: repository, tag/version, target, archive name, archive digest, binary digest, and verification status. It is evidence for revalidation, not a trust substitute; the binary is rechecked before use.

### D4. Latest stable on first acquisition; no surprise upgrades

`latest-stable` is resolved only when no compatible provider is available or when an explicit incompatible version needs replacement. A compatible PATH or managed version is reused; normal sync does not silently upgrade it. The installer records the selected release so rollback and support diagnostics can identify it.

An explicit version pin/upgrade command MAY be designed later, but is not required for this MVP. The recipe MUST expose the minimum compatible provider version and the release URL in diagnostics.

### D5. Separate plan/consent/execution and fail atomically

The installer has four testable seams:

1. `resolve_install_plan(dep, platform, optional_version)` validates the declaration and returns a non-executing plan.
2. `preview_install(plan)` reports repository, tag policy, target, destination, archive, checksum source, and replacement behavior.
3. `offer_and_install(plans, tty=True)` asks for explicit confirmation per required missing provider and executes only approved plans.
4. `install_github_release(plan)` performs network, verification, extraction, self-check, and atomic publication.

`doctor`, non-TTY paths, `sync`, and passive dependency checks MUST NOT download or install. The interactive `recipe add`/`configure-recipes` dependency gate MAY offer installation after displaying the complete plan. Declining returns a visible incomplete state and manual GitHub Release guidance.

Installation sequence:

```text
resolve platform
    │
fetch bounded release metadata
    │
select stable tag + exact expected asset
    │
fetch SHA256SUMS + selected archive over HTTPS
    │
verify manifest entry and archive digest
    │
inspect/extract only expected regular files
    │
run absolute binary `version` self-check
    │
write receipt to staging area
    │
atomic rename staged version directory
    │
recheck executable and receipt
```

Downloads are staged in the destination's parent filesystem. Any network, metadata, checksum, archive, permissions, self-check, or receipt failure removes only temporary files and leaves the prior managed version and any PATH executable unchanged.

### D6. Resolve a provider command marker before MCP rendering

A static `command = "jinna"` is insufficient when the managed installation is intentionally outside the user's PATH. Add a reserved recipe MCP marker:

```toml
[[provides.mcp]]
id = "jinna"
command = "{dep:jinna}"
args = ["mcp"]
timeout = 30000
env = {
  OPENPROJECT_BASE_URL = "$OPENPROJECT_BASE_URL",
  OPENPROJECT_API_TOKEN = "$OPENPROJECT_API_TOKEN",
  OPENPROJECT_AUTH = "$OPENPROJECT_AUTH"
}
```

`recipe-materialize.py` resolves `{dep:jinna}` before writing its recipe MCP temporary JSON:

- compatible PATH candidate → concrete command `jinna`;
- verified managed candidate → absolute executable path;
- no candidate in non-interactive sync → omit the unresolved provider MCP entry and emit one actionable warning, while `doctor` reports the required dependency; never emit the marker or a guessed command to runtime files;
- explicit project manifest overrides remain subject to existing manifest-precedence rules and are not rewritten by recipe defaults.

The resolver returns a structured dependency acquisition result, not an arbitrary shell string. `mcp-render.py` receives only a concrete command and existing argument/env values; all per-agent translators continue to handle the command format. OpenCode receives a local command array, and generic runtimes receive a concrete command plus `mcp` argument.

This keeps generated runtime files usable when a managed binary is installed and keeps unresolved installations from being represented as ready MCP integrations.

### D7. Keep OpenProject configuration secret-safe and explicit

The recipe's MCP environment values are references only. `env_scaffold` may generate `ai-specs.env.example` entries and the existing user-owned `ai-specs.env` flow may collect values, but the recipe MUST NOT:

- put an API token in TOML, JSON, docs, command arguments, or logs;
- call the OpenProject API during provider installation;
- infer a server URL from GitHub metadata;
- require OpenProject's official `/mcp` endpoint.

The local provider uses direct APIv3 and the runtime user supplies `OPENPROJECT_BASE_URL` and `OPENPROJECT_API_TOKEN`; `OPENPROJECT_AUTH` remains optional and follows the provider's documented default. Provider installation and OpenProject health verification are separate operations. A real OpenProject smoke is opt-in and credential-dependent.

### D8. Keep the official remote MCP boundary explicit

The recipe configures only local `jinna mcp`. Documentation MAY explain the official OpenProject remote `/mcp` endpoint as a separate operator-selected integration, but no recipe hook, resolver, retry path, or MCP preset may automatically switch to it. A failed local write is never replayed against the official endpoint, and a failed official write is never replayed through `jinna`.

### D9. Let the recipe declare an env value set and validate it early

Configuration values that the provider enumerates (today `OPENPROJECT_AUTH`: `basic` or `bearer`) are
validated at the boundary the recipe owns — not as hardcoded CLI knowledge, and not only inside the
provider at runtime.

- A `[[provides.mcp]]` preset MAY declare `env_allowed = { KEY = ["a", "b"] }` beside `env`, keyed by the
  declaration key. Accepted values are recipe data, so provider knowledge stays in the catalog and no
  per-provider branch is added to the CLI.
- The interactive flow prompts a constrained variable as a closed choice, making an out-of-set value
  unrepresentable; a value already configured out of set is reported by `doctor` as an early
  `harness-env-value` warning instead of failing later at provider start-up.
- Comparison is case-insensitive because the provider accepts enumerated values in any case, and the
  warning never echoes the configured value, which may be a credential.
- A malformed declaration fails open: the variable is left unconstrained and no error is raised, so a
  catalog authoring mistake cannot break `configure-recipes`.
- This is deliberately limited to enumerated values. Free-form values (URLs, tokens, absolute paths) keep
  the existing free-text prompt; deeper semantic validation remains the provider's responsibility.

## Installer module design

### Schema/data model

Extend the existing `CliDep` dataclass with optional fields:

```python
@dataclass(frozen=True)
class CliDep:
    binary: str
    purpose: str
    required: bool = True
    install_url: str = ""
    version_check: str = ""
    min_version: str = ""
    installer: str = ""
    repository: str = ""
    release_policy: str = ""
```

The exact field names may be adjusted only if the resulting schema retains the same invariants. `recipe-read.py` MUST round-trip the fields. Legacy package-manager plans continue to be selected by the existing binary map; `github-release` is selected only for a validated matching declaration.

A resolved acquisition result should carry:

```python
@dataclass(frozen=True)
class ProviderResolution:
    binary: str
    command: str
    version: str
    source: str              # "path" | "managed" | "unresolved"
    path: Path | None
    repository: str
    release_tag: str
    target: tuple[str, str]
    verified: bool
```

No secret-bearing field is allowed in these structures or their serialized receipts.

### Platform mapping

Use a pure helper analogous to the existing binary gate mapping:

| Host | Go target |
|---|---|
| Darwin + arm64/aarch64 | `darwin/arm64` |
| Darwin + x86_64/amd64 | `darwin/amd64` |
| Linux + arm64/aarch64 | `linux/arm64` |
| Linux + x86_64/amd64 | `linux/amd64` |
| Windows + amd64/x86_64 | `windows/amd64` |

Unknown platforms fail with an actionable supported-target message. The recipe does not attempt an emulation, source build, or alternate package.

### Release fetch and verification

- Use Python standard-library HTTPS requests with bounded response size and timeout.
- Send a stable `User-Agent` identifying ai-specs, but no GitHub credential is required for public releases.
- Reject API responses that are not objects, stable releases, the allowlisted repository, or contain duplicate/missing assets.
- Require exact `SHA256SUMS` entry for the selected archive; parse only `<64 lowercase/uppercase hex>  <expected filename>` lines.
- Verify the downloaded archive's SHA-256 before extraction.
- Enforce a maximum archive/API response size appropriate for the small provider artifacts, and a maximum individual archive member size of 32 MiB (the released `jinna` executable is already ~7.9 MiB, so the original 8 MiB member cap left almost no headroom).
- Require the release `RELEASE.json` asset and validate its version and artifact list against the selected stable tag and archive before extraction.
- Reject absolute paths, `..` traversal, symlinks, hardlinks, duplicate members, unexpected executable names, and files outside the expected archive root.
- Extract only the provider executable and documented release files; do not run or preserve arbitrary archive content.
- On Unix, set executable mode `0755`; on Windows, retain `.exe` and use the platform command path.
- Run `<managed-path> version` with a timeout and require the reported version to equal the selected release version (allowing the provider's documented `v` display convention).
- Write `install.json` atomically only after all checks pass, then atomically publish the version/target directory.

### Dependency and command lifecycle

- `dep_check` reports PATH and managed candidates without mutation.
- `config_wizard` displays dependency status, invokes the opt-in plan only on an interactive TTY, rechecks after execution, and refuses to claim configuration readiness if the required provider remains unresolved.
- `recipe-add` may leave the recipe declaration present when the user declines, matching existing configuration semantics, but prints the exact manual next step and does not write a broken concrete MCP command.
- `recipe-materialize` resolves the marker from the shared acquisition/resolution seam and records no provider binary bytes in the project.
- `doctor` reports missing/unverified provider state as a warning or error according to the existing required-dependency policy, without downloading.

## Recipe assets

The catalog recipe should contain:

```text
catalog/recipes/jinna-mcp-recipe/
├── recipe.toml
├── init.md
├── README.md
└── skills/jinna-mcp-recipe/SKILL.md
```

`recipe.toml` declares the provider dependency, the `jinna` MCP preset, the OpenProject env references, and no runtime hook. The skill and README explain:

- provider binary versus remote OpenProject service;
- PATH detection and managed installation behavior;
- GitHub Release source, supported targets, checksums, and rollback;
- configuration of `OPENPROJECT_BASE_URL`, `OPENPROJECT_API_TOKEN`, and optional auth;
- local `jinna mcp` versus official remote `/mcp` selection;
- no automatic write retry or protocol fallback;
- no need for Go/Mise/Python/Git for end users;
- developer-only source build guidance, if retained, clearly separated from the normal path.

`init.md` remains a read-only, agent-readable setup brief. Actual installation belongs to the interactive dependency/configuration gate, not an opaque init hook.

## Failure and rollback model

| Failure | Result |
|---|---|
| Unsupported host | No download; actionable supported-target guidance. |
| GitHub API/network/HTTP failure | No mutation; show release URL and retry/manual guidance. |
| Stable release missing or malformed | No mutation; fail closed and report repository/release contract problem. |
| Missing checksum or digest mismatch | Delete staged artifact; preserve prior installation; report verification failure. |
| Unsafe/corrupt archive | Delete staged extraction; never execute; preserve prior installation. |
| Version self-check mismatch | Reject staged binary; preserve prior installation. |
| Destination permission failure | No partial publication; report a user-writable path requirement. |
| User declines consent | No download/mutation; leave recipe visibly unready with manual instructions. |
| Managed binary later fails revalidation | Do not execute it; report stale/corrupt receipt and offer explicit reacquisition. |
| MCP materialization without provider | Omit unresolved provider MCP entry and report dependency warning; preserve unrelated runtime config. |

A recipe removal does not silently delete provider installations. Managed artifact cleanup, if added later, must be an explicit user-approved operation.

## Provider readiness gate before apply/verify

Before applying or verifying this change, the parent MUST confirm all of the following against the real provider repository and first release:

1. `https://github.com/parada1104/jinna-provider` is accessible and the canonical module/repository identity is final.
2. A non-draft, non-prerelease tag exists with all five target archives, `SHA256SUMS`, and `RELEASE.json`.
3. Archive names, root layout, executable names, version output, and checksum format match D2.
4. Provider quality gates are green, including the resolved Go/SDK vulnerability gate; a known blocking advisory is not silently waived.
5. Downloading each target metadata/checksum and installing the host target succeeds in an isolated test directory without Go, Mise, Python, or live OpenProject credentials.
6. The provider's documented release version and minimum version policy are recorded in the recipe fixture before end-to-end verification.

If any gate fails, apply may continue only for schema/docs work that does not claim a working release installer; full verify and delivery remain blocked.

## Verification strategy

### Unit tests

- Schema accepts/rejects installer fields and round-trips legacy recipes.
- Platform mapping covers all five targets and unsupported hosts.
- Version parsing distinguishes compatible, incompatible, unknown, and failing provider commands.
- Release metadata selects only the allowlisted stable release and exact expected asset.
- Checksums accept valid lines and reject missing, duplicate, malformed, or mismatched entries.
- Archive inspection rejects traversal, links, duplicates, unexpected files, and corrupt archives.
- Atomic installation leaves the old candidate untouched on every injected failure.
- Consent tests prove no TTY/no/declined paths do not download or mutate.
- Managed receipts revalidate binary/version/digest and reject tampering.

### Recipe and materialization tests

- `recipe.toml` validates and declares exactly the provider dependency/MCP/env contract.
- Existing compatible PATH provider renders `command = "jinna"`.
- Managed provider renders its verified absolute executable path and `args = ["mcp"]`.
- Missing provider in non-interactive sync does not emit `{dep:jinna}` or a guessed command.
- All enabled runtimes (Claude, Cursor, OpenCode, Pi, OMP) receive the correct local MCP shape.
- Env references render in each runtime without literal secret values.
- Existing recipes and unrelated manifest keys remain unchanged.
- Docs/catalog version and dependency tables remain consistent.

### Optional smoke

When explicitly configured with a built/released provider and test credentials, run `jinna version` and a minimal `jinna mcp` initialize/tools-list exchange. A real OpenProject API health call is optional and never part of the default test suite.
