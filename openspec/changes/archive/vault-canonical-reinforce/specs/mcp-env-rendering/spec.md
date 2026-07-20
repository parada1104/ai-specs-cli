# mcp-env-rendering (delta)

## Purpose

Extend MCP env/arg rendering guarantees so vault filesystem presets survive
paths that contain spaces (common with Obsidian iCloud locations such as
`Mobile Documents/...`) when the path is supplied via `CANONICAL_VAULT_PATH`.

## Requirements

### Requirement: Vault MCP path is env-owned via wrapper (not `${VAR}` argv)

The `vault-canonical-store` MCP preset SHALL NOT place `${CANONICAL_VAULT_PATH}`
(or `$CANONICAL_VAULT_PATH`) in command args. It SHALL launch a materialized
wrapper script (`vault-fs-mcp.sh`) with `command = "bash"` and a single relative
script path arg, while passing `CANONICAL_VAULT_PATH` through the MCP `env`
table. The wrapper SHALL resolve the absolute directory (including paths with
spaces and a leading `~/`) and exec the pinned filesystem MCP with that path
as one argv.

#### Scenario: Sync renders wrapper across agents

- **GIVEN** `vault-canonical-store` enabled and agents include
  `claude`, `cursor`, `opencode`, and `omp`
- **WHEN** the user runs `ai-specs sync`
- **THEN** each rendered `vault-canonical` entry uses `bash` +
  `ai-specs/recipes/vault-canonical-store/bin/vault-fs-mcp.sh`
- **AND** rendered args do not contain `CANONICAL_VAULT_PATH` or a literal
  spaced vault path
- **AND** the MCP env/environment table still references `CANONICAL_VAULT_PATH`

#### Scenario: Wrapper rejects unexpanded nested vars

- **GIVEN** `CANONICAL_VAULT_PATH` still contains a `$` (e.g. nested
  `$OBSIDIAN_VAULT_PATH/...` left literal)
- **WHEN** `vault-fs-mcp.sh` starts
- **THEN** it exits non-zero with an explicit error (does not pass the literal
  string to the filesystem server)

### Requirement: Env passthrough remains required for vault path

The vault MCP preset SHALL continue to declare
`env.CANONICAL_VAULT_PATH = "$CANONICAL_VAULT_PATH"` (or equivalent) so
envrc-scaffold discovers the variable even if args-only scanning is absent.

#### Scenario: envrc discovers CANONICAL_VAULT_PATH from vault recipe

- **GIVEN** `vault-canonical-store` enabled
- **WHEN** envrc scaffolding collects MCP env references
- **THEN** `CANONICAL_VAULT_PATH` appears among discovered vars

## Acceptance Criteria (test map)

| AC | Coverage | Req |
|----|----------|-----|
| AC1 | sync pipeline unit test with spaced path | single argv element |
| AC2 | OpenCode bare-dollar assert for vault preset | OpenCode form |
| AC3 | envrc collect includes `CANONICAL_VAULT_PATH` | discovery |
