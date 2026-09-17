---
name: jinna-mcp-recipe
description: Install and use the local OpenProject provider MCP through ai-specs; detect the provider binary, configure OpenProject environment references, and keep local/official MCP boundaries explicit.
license: MIT
metadata:
  author: ai-specs
  version: "1.0"
  scope: runtime
  auto_invoke:
    - "Using the OpenProject provider MCP"
    - "Configuring jinna-mcp-recipe"
    - "Installing or troubleshooting the OpenProject provider"
---

# OpenProject provider MCP

## Terminology

- **Provider:** the separate Go project `parada1104/jinna-provider`.
- **Provider binary:** the local executable `jinna`.
- **OpenProject:** the remote Self-Hosted service configured by `OPENPROJECT_BASE_URL`.
- **Local MCP:** `jinna mcp`, a stdio server that uses OpenProject APIv3.
- **Python client:** reference-only material; do not require or invoke it.

## Installation flow

1. Check the provider dependency through `ai-specs doctor` or the recipe configuration flow.
2. Reuse a compatible `jinna` found on `PATH`.
3. If it is missing or incompatible, run `ai-specs configure-recipes` interactively and review the GitHub Release plan before accepting installation.
4. The installer selects the host target, verifies the provider archive with `SHA256SUMS`, validates archive contents, checks `jinna version`, and publishes atomically.
5. Run `ai-specs sync` after installation so the generated MCP configuration uses either `jinna` or the verified managed executable path.

Never assume that OpenProject itself is installed locally. Never require Go, Mise, Python, Git, GitHub CLI, or a package manager for a provider release binary.

## Configuration

Keep credentials outside generated files:

```text
OPENPROJECT_BASE_URL
OPENPROJECT_API_TOKEN
OPENPROJECT_AUTH (optional: basic or bearer)
```

Use the existing ai-specs environment flow and user-owned ignored environment file. Do not put token values in MCP arguments, manifests, documentation, logs, or tool payloads.

## MCP boundary

Use the recipe-provided local `jinna mcp` server. The official OpenProject remote `/mcp` endpoint is a separate operator-selected option with its own server configuration and authentication. Do not automatically proxy, switch, or replay writes between these protocols.

## Safe troubleshooting

- Missing provider: configure interactively and accept the allowlisted GitHub Release plan, or follow the printed manual URL.
- Unknown version: treat the provider as unresolved until `jinna version` succeeds.
- Checksum or archive failure: do not execute the candidate; retain the previous managed version.
- Missing OpenProject settings: complete environment configuration; provider installation itself does not contact OpenProject.
- Non-interactive or CI run: expect detection and guidance only, never a download.
