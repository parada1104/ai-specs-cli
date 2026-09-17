# OpenProject provider setup

This recipe configures the local Go provider for OpenProject.

## Components

- **Provider binary:** `jinna`, the executable from `parada1104/jinna-provider`.
- **OpenProject:** the remote Self-Hosted service addressed by `OPENPROJECT_BASE_URL`.
- **Python client:** historical reference material only; it is not required.
- **Local MCP:** `jinna mcp`, a stdio server that calls OpenProject APIv3.

## Setup flow

1. Run `ai-specs recipe add jinna-mcp-recipe <project>` if the recipe is not enabled.
2. Run `ai-specs configure-recipes <project>` in an interactive terminal.
3. The dependency gate checks `jinna` and `jinna version`.
4. If no compatible provider is available, the gate shows an explicit GitHub Release installation plan. Accepting it downloads the supported platform archive, verifies `SHA256SUMS`, and installs it into the managed user cache. Declining leaves the recipe incomplete and prints the manual release URL.
5. Configure the OpenProject environment values through the existing environment flow:
   - `OPENPROJECT_BASE_URL`
   - `OPENPROJECT_API_TOKEN`
   - optional `OPENPROJECT_AUTH` (`basic` or `bearer`)
6. Run `ai-specs sync <project>` to materialize the runtime MCP configuration.

`ai-specs recipe init` is read-only. It prints this guidance and never downloads or installs the provider. `ai-specs doctor` also remains non-mutating.

## MCP boundary

The recipe configures local `jinna mcp`. It does not enable or automatically select OpenProject's official remote `/mcp` endpoint, and it never retries a failed write through another MCP transport.
