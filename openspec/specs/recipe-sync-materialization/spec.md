# recipe-sync-materialization Specification

## Purpose

Define how `ai-specs sync` resolves recipes from the CLI catalog, validates them, and materializes them into the project workspace, including the external directory layout for recipe-bundled and dependency skills.
## Requirements
### Requirement: Sync reads recipe declarations
During sync, the system SHALL parse all `[recipes.*]` tables from `ai-specs.toml` and filter to those with `enabled = true`.

#### Scenario: Multiple recipes enabled
- **WHEN** three recipes are declared with `enabled = true`
- **THEN** sync SHALL process all three in declaration order

### Requirement: Recipe validation
Before materialization, the system SHALL validate that: the recipe directory exists in the CLI recipe catalog at `catalog/recipes/<id>/`, `recipe.toml` is parseable, all required fields are present, and all referenced local paths (`skills/`, `commands/`, `templates/`, `docs/`) exist.

#### Scenario: Missing recipe.toml
- **WHEN** the CLI catalog directory `catalog/recipes/<id>/` exists but lacks `recipe.toml`
- **THEN** sync SHALL fail with "recipe.toml not found"

#### Scenario: Missing referenced skill directory
- **WHEN** `recipe.toml` declares a bundled skill but `skills/<id>/` does not exist
- **THEN** sync SHALL fail with "bundled skill not found"

### Requirement: Materialization order
The system SHALL materialize primitives in this order: skills (bundled then deps), commands, MCP presets, templates, docs. Bundled recipe skills SHALL be materialized into `.recipe/{recipe-id}/skills/{skill-id}/`. Dependency skills SHALL be materialized into `.deps/{dep-id}/skills/{skill-id}/`. Commands, MCP presets, templates, and docs SHALL continue to be materialized into `ai-specs/` as before.

#### Scenario: Full materialization with external directories
- **WHEN** a valid recipe with bundled skills and deps is processed
- **THEN** bundled skills SHALL be created under `.recipe/{recipe-id}/skills/`
- **AND** dep skills SHALL be created under `.deps/{dep-id}/skills/`
- **AND** commands, templates, docs, and MCP presets SHALL be created in `ai-specs/`
- **AND** derived artifacts (agent configs) SHALL reflect new skills and commands

#### Scenario: Re-sync idempotency with external directories
- **WHEN** sync runs twice with no changes
- **THEN** the second run SHALL not fail
- **AND** no unintended modifications SHALL occur in `.recipe/`, `.deps/`, or `ai-specs/`

### Requirement: MCP preset merge strategy
When a recipe declares an MCP preset with the same `id` as an existing `[mcp.<id>]` in the project manifest, the system SHALL merge the recipe fields into the derived config with project manifest values taking precedence over recipe defaults. The system SHALL emit a warning naming the recipe and the MCP id.

#### Scenario: Project manifest MCP overrides recipe preset
- **WHEN** the project manifest declares `[mcp.openmemory]` and a recipe also declares `mcp.id = "openmemory"`
- **THEN** sync SHALL merge the recipe fields into the derived MCP config
- **AND** project manifest fields SHALL take precedence on key overlap
- **AND** sync SHALL emit a warning describing the overlap for `mcp.id='openmemory'`

### Requirement: Sync resolves recipes from the CLI catalog
Sync SHALL resolve enabled recipes from the CLI recipe catalog. A consumer project SHALL NOT be required to host `catalog/recipes/` in its own workspace for recipe validation or materialization.

#### Scenario: Consumer project without local catalog
- **GIVEN** a project with `ai-specs/ai-specs.toml` and enabled recipe `tracker`
- **AND** the CLI catalog contains `tracker`
- **WHEN** `ai-specs sync` runs
- **THEN** sync SHALL validate and materialize `tracker` from the CLI catalog
- **AND** it SHALL NOT require `project_root/catalog/recipes/tracker/`

### Requirement: Idempotent sync
Running sync multiple times with the same manifest SHALL produce the same result.

#### Scenario: Re-sync unchanged recipe
- **WHEN** sync runs twice with no changes
- **THEN** the second run SHALL not fail
- **AND** no unintended modifications SHALL occur

### Requirement: Recipe skill materialization path
Bundled skills from a recipe SHALL be materialized to `.recipe/{recipe-id}/skills/{skill-id}/`, preserving the directory structure from `catalog/recipes/<id>/skills/`.

#### Scenario: Single bundled skill
- **WHEN** a recipe declares a bundled skill `id = "my-skill"`
- **THEN** sync SHALL create `.recipe/{recipe-id}/skills/my-skill/SKILL.md`
- **AND** any assets under `catalog/recipes/<id>/skills/my-skill/assets/` SHALL be copied to `.recipe/{recipe-id}/skills/my-skill/assets/`

### Requirement: Dependency skill materialization path
Dependency skills from a recipe's `[[deps]]` table SHALL be materialized to `.deps/{dep-id}/skills/{skill-id}/`.

#### Scenario: Single dependency skill
- **WHEN** a recipe declares a dependency skill `id = "vendor-skill"` from `dep-id = "vendor-lib"`
- **THEN** sync SHALL create `.deps/vendor-lib/skills/vendor-skill/SKILL.md`
- **AND** the skill contents SHALL match the vendored source

### Requirement: Local skills directory untouched
Sync SHALL NOT write bundled or dependency skills into `ai-specs/skills/`. `ai-specs/skills/` SHALL remain exclusively for local, project-owned skills.

#### Scenario: Sync with existing local skills
- **GIVEN** `ai-specs/skills/local-skill/` exists
- **WHEN** sync materializes recipe and dep skills
- **THEN** `ai-specs/skills/local-skill/` SHALL remain unchanged
- **AND** no new directories SHALL be created under `ai-specs/skills/`

### Requirement: Init remains separate from sync materialization

Recipe initialization MAY preview or propose templates, overrides, and manifest config needed before sync, but init SHALL NOT run `ai-specs sync` and SHALL NOT silently materialize recipe primitives. Sync SHALL remain responsible for materializing enabled recipe primitives.

#### Scenario: Init does not run sync

- **GIVEN** a recipe declares bundled skills, commands, MCP presets, templates, and docs
- **WHEN** `ai-specs recipe init <id>` runs
- **THEN** init SHALL NOT materialize those primitives
- **AND** init SHALL NOT update derived agent configs or registries through sync

#### Scenario: Init previews template target

- **GIVEN** a recipe declares a template with `target = "ai-specs/example.md"`
- **WHEN** `ai-specs recipe init <id>` runs
- **THEN** init MAY report that the template target is relevant to setup
- **AND** init MAY propose a reviewable create or update action
- **AND** sync SHALL remain the command that materializes the declared template primitive

#### Scenario: Human-reviewed init mutation remains idempotent

- **GIVEN** an init workflow has already created or updated a project override file after review
- **WHEN** `ai-specs recipe init <id>` runs again
- **THEN** init SHALL detect the existing file
- **AND** init SHALL propose skip, update, or diff guidance instead of creating a duplicate file

#### Scenario: Sync still materializes after init

- **GIVEN** init has proposed or applied reviewed manifest config for a recipe
- **AND** the recipe is enabled in `ai-specs/ai-specs.toml`
- **WHEN** `ai-specs sync` runs
- **THEN** sync SHALL validate and materialize the enabled recipe according to the sync materialization contract
- **AND** sync SHALL NOT assume init previously ran

### Requirement: Materialización de subagent files por harness

Cuando `[sdd].sub_agents = true`, el sistema SHALL materializar subagent files solo en directorios específicos del harness habilitado, sin tocar `ai-specs/skills/`, `.recipe/`, ni `.deps/`. Para `claude`, el destino MUST ser `.claude/agents/sdd-*.md`. Para harnesses que no soportan subagentes nativos, la materialización MUST omitirse sin error.

#### Scenario: Materialización a Claude Code
- **GIVEN** `[sdd].sub_agents = true` y `claude` en `[agents].enabled`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST crear `.claude/agents/sdd-explore.md`, `.claude/agents/sdd-proposal.md`, `.claude/agents/sdd-artifacts.md`, `.claude/agents/sdd-apply.md`, `.claude/agents/sdd-verify.md`, `.claude/agents/sdd-archive.md`
- **AND** los archivos MUST ser byte-idénticos al bundled source en el CLI

#### Scenario: Harness no soportado se omite limpiamente
- **GIVEN** `[sdd].sub_agents = true` y solo `opencode` en `[agents].enabled`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST completar sin error
- **AND** sync MUST NOT crear archivos de subagentes para ese harness
- **AND** sync MAY anotar la omisión en el log o runtime brief

#### Scenario: Subagentes no contaminan ai-specs/skills/
- **GIVEN** `[sdd].sub_agents = true`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST NOT crear ni modificar archivos bajo `ai-specs/skills/`, `.recipe/`, ni `.deps/`
- **AND** los subagent files MUST quedar exclusivamente en el directorio del harness destino

### Requirement: Orden y respeto a la pipeline de sync

La materialización de subagent files SHALL integrarse con la pipeline existente respetando idempotencia, sidecars `.new` para archivos modificados localmente, y `.ai-specs.lock` para hashes.

#### Scenario: Re-sync idempotente
- **GIVEN** un sync previo creó los subagent files
- **WHEN** `ai-specs sync` corre nuevamente sin cambios
- **THEN** los archivos resultantes MUST ser byte-idénticos al estado previo
- **AND** `.ai-specs.lock` MUST reflejar hashes consistentes

#### Scenario: Archivo modificado localmente
- **GIVEN** un subagent file en `.claude/agents/sdd-explore.md` fue editado a mano
- **WHEN** `ai-specs sync` corre con bundled source distinto
- **THEN** sync MUST escribir el nuevo contenido en `.claude/agents/sdd-explore.md.new`
- **AND** sync MUST NOT sobrescribir el archivo editado silenciosamente
- **AND** sync MUST emitir un aviso que nombre el sidecar generado

### Requirement: Transición de sub_agents true → false

Cuando el flag cambia de `true` a `false` entre dos corridas de sync, el sistema MUST advertir sobre archivos huérfanos y MUST NOT eliminarlos silenciosamente.

#### Scenario: Cambio de true a false reporta huérfanos
- **GIVEN** un sync previo materializó `.claude/agents/sdd-*.md` con `sub_agents = true`
- **AND** el manifiesto ahora declara `sub_agents = false`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST emitir un aviso enumerando los archivos huérfanos detectados
- **AND** sync MUST sugerir un comando o acción explícita para removerlos
- **AND** sync MUST NOT eliminar los archivos por sí solo

