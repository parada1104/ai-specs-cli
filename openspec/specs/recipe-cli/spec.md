# recipe-cli Specification

## Purpose

Definir los comandos CLI `ai-specs recipe list` y `ai-specs recipe add <id>` para descubrir e instalar recipes del catálogo del CLI en proyectos consumidores.

## Requirements

### Requirement: Comando recipe list
El sistema SHALL proveer `ai-specs recipe list [path]` que escanee el catálogo de recipes del CLI, lea cada `recipe.toml`, determine el estado de instalación desde el manifest local, y muestre una tabla legible.

#### Scenario: Lista con recipes disponibles e instaladas
- **WHEN** el catálogo contiene recipes y el manifest declara `[recipes.test-fixture]` con `enabled = true`
- **THEN** `recipe list` SHALL mostrar: ID, nombre, versión, y estado (`installed` / `available` / `disabled`)
- **AND** cada recipe del catálogo SHALL aparecer exactamente una vez

#### Scenario: Catálogo vacío
- **WHEN** el catálogo de recipes del CLI no contiene recipes válidas
- **THEN** `recipe list` SHALL mostrar mensaje "No hay recipes disponibles"
- **AND** exit code SHALL ser 0

#### Scenario: Proyecto no inicializado
- **WHEN** se ejecuta `recipe list` en un directorio sin `ai-specs/ai-specs.toml`
- **THEN** SHALL fallar con "Proyecto no inicializado. Ejecuta: ai-specs init"
- **AND** exit code SHALL ser 1

### Requirement: Comando recipe add
El sistema SHALL proveer `ai-specs recipe add <id> [path]` que valide la recipe en el catálogo, la agregue al manifest local, y muestre preview de primitives.

#### Scenario: Agregar recipe disponible
- **WHEN** se ejecuta `recipe add test-fixture` y la recipe existe en el catálogo
- **THEN** SHALL agregar `[recipes.test-fixture]` con `enabled = true` y `version` exacta del catálogo
- **AND** SHALL mostrar preview de skills, commands, mcp, templates y docs
- **AND** exit code SHALL ser 0

#### Scenario: Recipe ya instalada
- **WHEN** `[recipes.test-fixture]` ya existe en el manifest
- **THEN** SHALL abortar con "Recipe 'test-fixture' ya está en el manifest. Usa ai-specs sync para materializar."
- **AND** NO SHALL mutar el manifest
- **AND** exit code SHALL ser 1

#### Scenario: Recipe ID inexistente
- **WHEN** se ejecuta `recipe add nonexistent`
- **THEN** SHALL fallar con "Recipe 'nonexistent' no encontrada en catalog/recipes/"
- **AND** exit code SHALL ser 1

### Requirement: Catálogo resuelto desde el CLI
Los comandos `recipe list`, `recipe add`, y `recipe init` SHALL resolver recipes desde el catálogo distribuido por el CLI. Un proyecto consumidor SHALL NOT necesitar ni poseer `catalog/recipes/` para usar esos comandos.

#### Scenario: Proyecto consumidor sin catálogo local
- **GIVEN** un proyecto inicializado sin `catalog/recipes/`
- **AND** el catálogo del CLI contiene la recipe `tracker`
- **WHEN** se ejecuta `ai-specs recipe list`, `ai-specs recipe add tracker`, o `ai-specs recipe init tracker`
- **THEN** el comando correspondiente SHALL resolver `tracker` desde el catálogo del CLI
- **AND** SHALL NOT requerir un `catalog/recipes/` dentro del proyecto consumidor

#### Scenario: Catálogo local del proyecto no define el contrato
- **GIVEN** un proyecto inicializado que contiene un directorio `catalog/recipes/` por razones de desarrollo o dogfooding
- **AND** el catálogo del CLI contiene la recipe `tracker`
- **WHEN** se ejecuta `ai-specs recipe list`, `ai-specs recipe add tracker`, o `ai-specs recipe init tracker`
- **THEN** el contrato SHALL seguir resolviendo recipes desde el catálogo del CLI
- **AND** la presencia de `project_root/catalog/recipes/` SHALL NOT convertirse en requisito para proyectos consumidores normales

#### Scenario: Proyecto no inicializado
- **WHEN** se ejecuta `recipe add` en directorio sin `ai-specs.toml`
- **THEN** SHALL fallar con mensaje explícito de proyecto no inicializado
- **AND** exit code SHALL ser 1

### Requirement: Integración con recipe-read.py
Ambos comandos SHALL reutilizar `lib/_internal/recipe-read.py` y `lib/_internal/recipe_schema.py` para parsear `recipe.toml`. NO SHALL duplicar lógica de parsing o validación.

#### Scenario: Recipe.toml inválido en catálogo
- **WHEN** un directorio en `catalog/recipes/` contiene `recipe.toml` inválido
- **THEN** `recipe list` SHALL mostrar la recipe con estado `error` y mensaje breve
- **AND** `recipe add` de ese ID SHALL fallar con el error de validación original

### Requirement: No materialización
`recipe add` SHALL NOT ejecutar sync, copiar archivos, ni materializar primitives. Su única responsabilidad es declarar la recipe en el manifest.

#### Scenario: Add sin sync
- **WHEN** `recipe add test-fixture` completa exitosamente
- **THEN** los archivos de la recipe NO SHALL existir en `ai-specs/` ni en el proyecto root
- **AND** solo SHALL mutarse `ai-specs.toml`

### Requirement: Idempotencia declarativa
Ejecutar `recipe add` dos veces para el mismo ID SHALL producir el mismo manifest (sin duplicados) o fallar limpiamente.

#### Scenario: Doble add
- **WHEN** `recipe add test-fixture` se ejecuta dos veces
- **THEN** la segunda ejecución SHALL fallar con "ya está en el manifest"
- **AND** el manifest SHALL tener exactamente una entrada `[recipes.test-fixture]`

### Requirement: Command recipe init

The system SHALL provide `ai-specs recipe init <id> [path]` to produce an agent-readable initialization brief for a recipe from the CLI catalog. The optional `path` argument SHALL select the project root using the same path semantics as existing recipe commands.

#### Scenario: Init command for installed recipe with init workflow

- **GIVEN** an initialized project containing `ai-specs/ai-specs.toml`
- **AND** the catalog contains recipe `tracker`
- **AND** the recipe declares a valid `[init]` workflow
- **AND** the manifest declares `[recipes.tracker]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the command SHALL exit 0
- **AND** it SHALL print an agent-readable initialization brief
- **AND** the brief SHALL include recipe identity, install state, existing recipe config state, init prompt content or path, relevant manifest context, and reviewable next actions

#### Scenario: Init command can inspect available recipe before add

- **GIVEN** an initialized project containing `ai-specs/ai-specs.toml`
- **AND** the catalog contains recipe `tracker`
- **AND** the recipe declares a valid `[init]` workflow
- **AND** the manifest does not declare `[recipes.tracker]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the command SHALL exit 0
- **AND** the initialization brief SHALL state that the recipe is not installed
- **AND** the brief MAY propose adding `[recipes.tracker]` as a reviewable manifest change

#### Scenario: Init command on uninitialized project

- **GIVEN** a directory without `ai-specs/ai-specs.toml`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the command SHALL fail with an explicit uninitialized-project error
- **AND** exit code SHALL be 1
- **AND** no files SHALL be mutated

#### Scenario: Init command for missing recipe

- **GIVEN** an initialized project containing `ai-specs/ai-specs.toml`
- **AND** `catalog/recipes/missing/` does not exist
- **WHEN** `ai-specs recipe init missing` runs
- **THEN** the command SHALL fail with `Recipe 'missing' no encontrada en catalog/recipes/` or an equivalent explicit recipe-not-found error
- **AND** exit code SHALL be 1
- **AND** no files SHALL be mutated

#### Scenario: Init command for recipe without init workflow

- **GIVEN** an initialized project containing `ai-specs/ai-specs.toml`
- **AND** the catalog contains recipe `basic`
- **AND** the recipe has no `[init]` declaration
- **WHEN** `ai-specs recipe init basic` runs
- **THEN** the command SHALL report that recipe `basic` has no init workflow
- **AND** exit code SHALL be 1
- **AND** no files SHALL be mutated

### Requirement: Init brief is reviewable and idempotent

`ai-specs recipe init` SHALL NOT silently apply behavioral changes. Any manifest edits, config updates, template overrides, or generated files proposed by init SHALL be shown as reviewable actions or patches before mutation. Re-running init SHALL detect existing recipe declarations, existing `[recipes.<id>.config]` keys, and existing target files so it can propose updates instead of duplicates.

#### Scenario: Existing recipe config detected

- **GIVEN** an initialized project whose manifest contains `[recipes.tracker.config]`
- **AND** the table already declares `board_id = "abc123"`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief SHALL report the existing `board_id` config key
- **AND** the brief SHALL NOT propose appending a duplicate `board_id` key
- **AND** any replacement SHALL be presented as an update to the existing key

#### Scenario: Existing recipe declaration detected

- **GIVEN** an initialized project whose manifest contains `[recipes.tracker]`
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief SHALL identify the existing recipe declaration
- **AND** the brief SHALL NOT propose appending a second `[recipes.tracker]` table

#### Scenario: Existing template target detected

- **GIVEN** a recipe init workflow that may propose a template override
- **AND** the target file already exists in the project
- **WHEN** `ai-specs recipe init tracker` runs
- **THEN** the initialization brief SHALL identify the existing target
- **AND** the brief SHALL propose reviewable update, skip, or diff guidance instead of silently overwriting the file
