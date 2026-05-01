# agents-md-runtime-brief Specification

## Purpose

Define the content contract for `AGENTS.md` as a runtime operational brief derived from `ai-specs.toml`. The brief includes project identity, enabled runtimes, MCP servers, active recipes/bindings/capabilities, safety rules, context sources, and workflow rules. It does NOT include skill catalogs or Auto-invoke tables.

## ADDED Requirements

### Requirement: AGENTS.md is a runtime brief
`AGENTS.md` SHALL be a concise, human-meaningful runtime context document generated from `ai-specs.toml`. It SHALL communicate project identity, enabled agents, MCPs, active recipes/bindings, safety rules, context sources, and workflow rules.

#### Scenario: Generated brief contains project identity
- **WHEN** `ai-specs sync` runs with a valid `ai-specs.toml`
- **THEN** the generated `AGENTS.md` SHALL include the project name, manifest path, purpose, and enabled runtimes

#### Scenario: Generated brief contains MCP configuration
- **WHEN** `ai-specs.toml` declares one or more `[mcp]` servers
- **THEN** the generated `AGENTS.md` SHALL list configured MCPs with their names and descriptions
- **AND** it SHALL show env variable references for secrets
- **AND** it SHALL NOT expose literal secret values

#### Scenario: Generated brief contains active recipes and bindings
- **WHEN** `ai-specs.toml` declares enabled recipes, bindings, or capabilities
- **THEN** the generated `AGENTS.md` SHALL include a section listing active operational bundles
- **AND** it SHALL indicate which bundles are currently enabled

#### Scenario: Generated brief contains safety and workflow rules
- **WHEN** `ai-specs.toml` defines safety rules or workflow rules
- **THEN** the generated `AGENTS.md` SHALL include these rules in dedicated sections

#### Scenario: Generated brief contains context source precedence
- **WHEN** `ai-specs.toml` defines context sources and their precedence
- **THEN** the generated `AGENTS.md` SHALL document the precedence order and conflict policy

### Requirement: AGENTS.md does not contain skill catalogs
The generated `AGENTS.md` SHALL NOT contain an exhaustive skills table, skill directory listing, or Auto-invoke mappings. Skills SHALL remain discoverable through the filesystem and the separate registry artifact.

#### Scenario: Sync does not emit skills table into AGENTS.md
- **WHEN** `ai-specs sync` runs against a project with multiple skills
- **THEN** the generated `AGENTS.md` SHALL NOT contain a skills catalog or Auto-invoke table
- **AND** skills SHALL remain discoverable in `ai-specs/skills/` and `.recipe/*/skills/` and `.deps/*/skills/`

#### Scenario: AGENTS.md size is reduced compared to legacy registry mode
- **WHEN** `ai-specs sync` runs on a project with many skills
- **THEN** the generated `AGENTS.md` SHALL be smaller than the legacy auto-generated registry version
- **AND** its content SHALL focus on runtime operational context

### Requirement: Idempotent generation
Running `ai-specs sync` multiple times with the same `ai-specs.toml` SHALL produce byte-identical `AGENTS.md` output.

#### Scenario: Re-sync produces identical AGENTS.md
- **WHEN** `ai-specs sync` runs twice with the same manifest and no changes
- **THEN** the second run SHALL produce an `AGENTS.md` that is byte-identical to the first

### Requirement: Manual runtime-brief marker support
If `AGENTS.md` contains a runtime-brief marker (e.g., `<!-- ai-specs:runtime-brief -->`), the sync tool SHALL preserve the marker and skip overwriting the file with auto-generated content, treating the file as manually maintained.

#### Scenario: Manual runtime brief is preserved
- **GIVEN** an existing `AGENTS.md` contains a runtime-brief marker
- **WHEN** `ai-specs sync` runs
- **THEN** the sync tool SHALL NOT overwrite `AGENTS.md`
- **AND** it SHALL generate the registry artifact normally

### Requirement: Secrets redaction in MCP listings
When rendering MCP server configuration into the runtime brief, the system SHALL redact secret values and show only env variable references or placeholder text.

#### Scenario: MCP with env-backed secret
- **GIVEN** an `[mcp.openmemory]` server has `url = "http://localhost:8080/mcp"` and an env-backed secret token
- **WHEN** `ai-specs sync` generates the runtime brief
- **THEN** the MCP listing SHALL show the URL and description
- **AND** it SHALL show a placeholder or env variable name for the secret
- **AND** it SHALL NOT emit the literal secret value
