# mcp-env-rendering

## Purpose

Define how the MCP renderer in `lib/_internal/mcp-render.py` recognises and normalises environment-variable references in the `[mcp.<name>]` entries of `ai-specs/ai-specs.toml` when fanning out to per-agent config files.

## Requirements

### Requirement: MCP env values MUST accept both `$VAR` and `${VAR}` reference forms

The MCP renderer SHALL recognise both `$VARIABLE_NAME` and `${VARIABLE_NAME}` as references to environment variables when rendering the `env` / `environment` field of an MCP server entry. The canonical form documented in `ai-specs.toml` SHALL remain `$VARIABLE_NAME`; the braced form is accepted as a defensive fallback for projects that already use it.

Both forms MUST produce identical canonical output per target agent:

- For agent `opencode`, the rendered value MUST be `{env:VARIABLE_NAME}` placed under the `environment` field of the server entry.
- For generic agents (Claude, Cursor, and any other agent without a registered translator), the rendered value MUST be `${VARIABLE_NAME}` placed under the `env` field of the server entry.

Variable names MUST follow shell conventions: an initial letter or underscore, followed by uppercase letters, digits, or underscores (`[A-Z_][A-Z0-9_]*`). Values that do not match either form (for example literal strings, mixed-case identifiers, or values containing surrounding text) MUST pass through unchanged.

#### Scenario: Braced env reference renders as OpenCode native syntax

- **GIVEN** an `ai-specs.toml` with `[mcp.demo]` whose `environment` is `{ API_KEY = '${DEMO_API_KEY}' }` and `enabled = ['opencode']`
- **WHEN** the user runs `ai-specs sync`
- **THEN** the rendered `opencode.json` MUST contain `"environment": { "API_KEY": "{env:DEMO_API_KEY}" }` for the `demo` server entry

#### Scenario: Braced env reference renders as Cursor canonical syntax

- **GIVEN** an `ai-specs.toml` with `[mcp.demo]` whose `env` is `{ API_KEY = '${DEMO_API_KEY}' }` and `enabled = ['cursor']`
- **WHEN** the user runs `ai-specs sync`
- **THEN** the rendered `.cursor/mcp.json` MUST contain `"env": { "API_KEY": "${DEMO_API_KEY}" }` for the `demo` server entry

#### Scenario: Braced env reference renders as Claude canonical syntax

- **GIVEN** an `ai-specs.toml` with `[mcp.demo]` whose `env` is `{ API_KEY = '${DEMO_API_KEY}' }` and `enabled = ['claude']`
- **WHEN** the user runs `ai-specs sync`
- **THEN** the rendered `.mcp.json` MUST contain `"env": { "API_KEY": "${DEMO_API_KEY}" }` for the `demo` server entry

#### Scenario: Plain `$VAR` and braced `${VAR}` produce identical canonical output

- **GIVEN** two equivalent `ai-specs.toml` files that differ only in env reference form (one uses `'$DEMO_API_KEY'`, the other uses `'${DEMO_API_KEY}'`)
- **WHEN** the user runs `ai-specs sync` against both
- **THEN** the rendered config files for each enabled agent MUST be byte-identical

#### Scenario: Literal values and malformed identifiers pass through unchanged

- **GIVEN** an `ai-specs.toml` with `[mcp.demo]` whose `env` contains `{ MODE = 'fixture', NOTE = '$lowercase', ADJ = '$VAR-suffix' }`
- **WHEN** the user runs `ai-specs sync`
- **THEN** every rendered config MUST keep the values exactly as `"fixture"`, `"$lowercase"`, and `"$VAR-suffix"` (no substitution applied)
