# vault-canonical-store (delta)

## Purpose

Reinforce the catalog recipe `vault-canonical-store` so enabling it ships both
the canonical-store discipline skill and the Obsidian-native skill stack used
for LLM wiki / second-brain workflows (kepano/obsidian-skills), while keeping
the vault MCP path env-owned and portable.

## Requirements

### Requirement: Recipe ships kepano Obsidian skills as dep skills

The recipe `vault-canonical-store` SHALL declare, in addition to bundled
`vault-context`, the following skills with `source = "dep"`,
`url = "https://github.com/kepano/obsidian-skills.git"`, and the matching
monorepo `path`:

| id | path |
|----|------|
| `obsidian-markdown` | `skills/obsidian-markdown` |
| `obsidian-bases` | `skills/obsidian-bases` |
| `json-canvas` | `skills/json-canvas` |
| `obsidian-cli` | `skills/obsidian-cli` |
| `defuddle` | `skills/defuddle` |

Enabling the recipe and running sync SHALL vendor these skills through the
standard recipe-dep materialization path (project recipe cache `.deps/`),
without requiring the consumer to add project-level `[[deps]]` entries for them.

#### Scenario: Enable recipe vendors kepano skills

- **GIVEN** a project with `vault-canonical-store` enabled
- **WHEN** recipe materialization / sync runs successfully
- **THEN** each of the five kepano skill ids resolves into the project's
  resolved-skills set with source tier `dep`
- **AND** the project manifest need not list those ids under `[[deps]]`

### Requirement: vault-context remains the discipline skill

`vault-context` SHALL remain `source = "bundled"` and SHALL document when to
load Obsidian-native skills (markdown/bases/canvas/cli/defuddle) versus when to
use the vault MCP + Engram split. It SHALL instruct agents not to hardcode a
filesystem path when the runtime brief / MCP config already provides the scoped
vault.

#### Scenario: Decision write uses Obsidian markdown conventions

- **GIVEN** the agent is recording a durable decision in the vault
- **WHEN** `vault-context` is loaded
- **THEN** the skill guidance points at `obsidian-markdown` for wikilinks,
  properties, callouts, and embeds when writing `.md` notes

### Requirement: MCP preset stays env-scoped and version-pinned

The recipe SHALL continue to provide MCP id `vault-canonical` using
`@modelcontextprotocol/server-filesystem@2025.7.1` with a single directory arg
`${CANONICAL_VAULT_PATH}` and env passthrough for that variable. The recipe
SHALL NOT embed machine-specific absolute paths in `recipe.toml`.

#### Scenario: Preset uses env var not literal path

- **GIVEN** the catalog `vault-canonical-store` recipe.toml
- **WHEN** the MCP preset is inspected
- **THEN** args include `${CANONICAL_VAULT_PATH}` as one element
- **AND** the package pin is `@modelcontextprotocol/server-filesystem@2025.7.1`

### Requirement: Docs describe iCloud / spaced path setup

Recipe README and catalog docs SHALL state that `CANONICAL_VAULT_PATH` is set in
`.envrc` (typically `$OBSIDIAN_VAULT_PATH/<vault_scope>`), that values with
spaces must be quoted in shell exports, and that the recipe declares the MCP
preset (correcting any stale “recipe does not declare MCP” wording).

#### Scenario: README matches shipped MCP

- **GIVEN** a reader opens the vault-canonical-store README
- **WHEN** they look for MCP setup
- **THEN** they learn the recipe provides `vault-canonical` via env-backed path
- **AND** they see an example export that survives spaces in Obsidian/iCloud paths

## Acceptance Criteria (test map)

| AC | Coverage | Req |
|----|----------|-----|
| AC1 | `test_vault_canonical_store_recipe` dep skill declarations | kepano deps |
| AC2 | materialize / resolved-skills dry test | vendor on enable |
| AC3 | recipe.toml pin + arg assertions | MCP preset |
| AC4 | README / catalog doc checks (spot or content assert) | docs |
| AC5 | `vault-context` content mentions Obsidian skills | discipline cross-link |
