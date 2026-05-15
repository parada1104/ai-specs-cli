# agents-md-runtime-brief Specification

## Purpose

Define the content contract for `AGENTS.md` as a runtime operational brief derived from `ai-specs.toml`. The brief includes project identity, enabled runtimes, MCP servers, active recipes/bindings/capabilities, safety rules, context sources, and workflow rules. It does NOT include skill catalogs or Auto-invoke tables.
## Requirements
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
The generated `AGENTS.md` SHALL NOT contain an exhaustive skills table, skill directory listing, or Auto-invoke mappings. Skills SHALL remain discoverable through the filesystem and their `SKILL.md` frontmatter.

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
- **AND** it SHALL proceed with remaining sync steps normally

### Requirement: Secrets redaction in MCP listings
When rendering MCP server configuration into the runtime brief, the system SHALL redact secret values and show only env variable references or placeholder text.

#### Scenario: MCP with env-backed secret
- **GIVEN** an `[mcp.openmemory]` server has `url = "http://localhost:8080/mcp"` and an env-backed secret token
- **WHEN** `ai-specs sync` generates the runtime brief
- **THEN** the MCP listing SHALL show the URL and description
- **AND** it SHALL show a placeholder or env variable name for the secret
- **AND** it SHALL NOT emit the literal secret value

### Requirement: Recipe config fields rendered in runtime brief
When `ai-specs.toml` declares enabled recipes with non-empty config schemas, the generated `AGENTS.md` SHALL include a per-recipe subsection listing each config field with its `required`, `type`, `default`, and `validation` attributes.

#### Scenario: Enabled recipe with config fields
- **GIVEN** an enabled recipe with `[config.board_id]` where `required = true`, `type = "string"`, and `validation.regex = "^[0-9a-fA-F]{24}$"`
- **AND** `[config.default_list]` where `required = false`, `type = "string"`, `default = "In Progress"`
- **WHEN** `ai-specs sync` generates `AGENTS.md`
- **THEN** the runtime brief SHALL contain a config fields table for that recipe
- **AND** the table SHALL list `board_id` with `required`, `type`, and `validation`
- **AND** the table SHALL list `default_list` with `required`, `type`, and `default`

#### Scenario: Recipe without config schema omits subsection
- **GIVEN** an enabled recipe with no `[config]` table
- **WHEN** `ai-specs sync` generates `AGENTS.md`
- **THEN** the runtime brief SHALL NOT contain a config fields subsection for that recipe

#### Scenario: Config field with regex validation shows pattern
- **GIVEN** an enabled recipe with `[config.board_id]` where `validation.regex = "^[0-9a-fA-F]{24}$"`
- **WHEN** `ai-specs sync` generates `AGENTS.md`
- **THEN** the runtime brief SHALL display the regex pattern next to the field name


### Requirement: Sección opcional de subagentes SDD en el runtime brief

Cuando `[sdd].sub_agents = true` y al menos un harness soportado está habilitado, el runtime brief generado SHALL incluir una sección listando los subagentes SDD activos por `name` y `description`. Cuando el flag es `false` o ausente, el brief MUST NO contener esa sección y MUST preservar idempotencia byte-identical respecto al brief generado antes de la introducción de la feature.

#### Scenario: Sub_agents activo añade la sección
- **GIVEN** `[sdd].sub_agents = true` y `claude` habilitado
- **WHEN** `ai-specs sync` genera `AGENTS.md`
- **THEN** el brief MUST contener una subsección titulada explícitamente para subagentes SDD
- **AND** la subsección MUST listar los seis subagentes activos con `name` y `description` resumida
- **AND** la subsección MUST indicar la ubicación canónica `.claude/agents/sdd-*.md`

#### Scenario: Sub_agents desactivado mantiene brief idéntico
- **GIVEN** un proyecto sin cambios en `ai-specs.toml` salvo eventual edición de campos no relacionados con SDD
- **AND** `[sdd].sub_agents` es `false` o ausente
- **WHEN** `ai-specs sync` genera `AGENTS.md` en dos corridas con la misma configuración
- **THEN** ambas corridas MUST producir un brief byte-idéntico
- **AND** el brief MUST NO contener la sección de subagentes SDD

#### Scenario: Harness sin soporte nativo se documenta
- **GIVEN** `[sdd].sub_agents = true` y solo `opencode` habilitado
- **WHEN** `ai-specs sync` genera `AGENTS.md`
- **THEN** la sección de subagentes MUST indicar que `opencode` ejecuta las fases SDD inline en el orquestador
- **AND** MUST NO declarar archivos materializados en `.opencode/` ni en `.claude/`

### Requirement: Marker manual sigue ganando

Si `AGENTS.md` contiene el marker `<!-- ai-specs:runtime-brief -->`, el sync tool SHALL preservar el archivo manual sin escribir contenido auto-generado, incluso cuando `[sdd].sub_agents = true`. Esta regla MUST sobreescribir cualquier comportamiento de inserción automático.

#### Scenario: Marker manual evita reescritura aún con sub_agents activo
- **GIVEN** un `AGENTS.md` con marker manual
- **AND** `[sdd].sub_agents = true`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST NOT modificar `AGENTS.md`
- **AND** sync MUST continuar el resto de pasos sin error
- **AND** sync MAY registrar la omisión en log o salida
