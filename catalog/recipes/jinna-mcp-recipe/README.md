# OpenProject Provider MCP

`jinna-mcp-recipe` installs and configures the local Go provider for OpenProject. It is the user-facing installation path for the provider binary, not a replacement for the provider implementation.

## What gets installed

The provider project is [`parada1104/jinna-provider`](https://github.com/parada1104/jinna-provider). Its user-facing executable is named `jinna`.

The recipe checks for a compatible `jinna` executable first. If it is missing or too old, an interactive configuration run offers the matching archive from [GitHub Releases](https://github.com/parada1104/jinna-provider/releases). The installer:

- selects Linux amd64/arm64, macOS amd64/arm64, or Windows amd64;
- downloads only the allowlisted provider release;
- verifies the archive against `SHA256SUMS`;
- rejects unsafe archive members and unexpected content;
- verifies `jinna version` before publication;
- installs into a user-writable managed cache without replacing an unrelated PATH executable;
- records non-secret installation provenance for later revalidation.

Users do not need Go, Mise, Python, Git, GitHub CLI, or a package manager to use a release binary. Go and Mise are maintainer/build requirements of the provider repository.

## Install and configure

From the target project:

```bash
ai-specs recipe add jinna-mcp-recipe
ai-specs configure-recipes
ai-specs sync
```

`configure-recipes` is interactive when it offers provider installation. It displays the release source, target, destination, and checksum step before asking for consent. If the provider is declined or cannot be downloaded, no partial binary is published and the command prints the manual release URL.

Passive commands are safe:

```bash
ai-specs doctor
ai-specs recipe init jinna-mcp-recipe
```

These commands detect/report the provider but never download or install it. Non-interactive `sync` also never installs a dependency; it leaves the provider MCP unresolved and reports the next action.

## OpenProject configuration

OpenProject is a remote Self-Hosted service. Configure it through the environment flow or a user-owned ignored environment file:

```bash
export OPENPROJECT_BASE_URL="https://openproject.example.com"
export OPENPROJECT_API_TOKEN="<token>"
# Optional: basic is the provider default.
export OPENPROJECT_AUTH="basic"
```

Do not put token values in `ai-specs.toml`, generated runtime files, MCP arguments, committed documentation, or logs. The recipe passes environment references to the local provider; it does not contact OpenProject while installing the binary.

## MCP behavior

After a verified provider is available, the recipe materializes the local stdio server:

```text
jinna mcp
```

The provider calls OpenProject APIv3 directly. The official OpenProject remote `/mcp` endpoint is a separate operator-selected integration. This recipe never proxies, automatically switches, or replays failed writes between the local provider and the official endpoint.

## Troubleshooting and rollback

Managed installations live under a tag-keyed cache:

```text
<ai-specs-home>/cache/bin/jinna/<release-tag>/<goos>-<goarch>/
```

For example, the first release installs to `.../cache/bin/jinna/v0.1.0/darwin-arm64/jinna` with an
`install.json` receipt beside it.

- `jinna` missing: rerun `ai-specs configure-recipes` interactively and accept the GitHub Release plan, or install the matching archive manually.
- Unsupported platform: use a supported target or run the provider from a compatible environment; the recipe never builds source or selects an alternate artifact.
- Checksum/archive/version failure: the candidate is rejected and the previous managed installation remains unchanged.
- Provider installed but runtime cannot find it: rerun `ai-specs sync` so the verified managed executable path is resolved into the local MCP configuration.
- To roll back, there is no automatic "select an older version" command. Either reacquire a different release explicitly, or remove the unwanted tag directory from the cache yourself (for example `rm -rf <ai-specs-home>/cache/bin/jinna/<release-tag>`). Removing this recipe does not delete a provider binary or an unrelated user-owned executable.
