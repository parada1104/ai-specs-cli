# recipe-cli Specification

## Purpose

Definir los comandos CLI `ai-specs recipe list` y `ai-specs recipe add <id>` para descubrir e instalar recipes del catálogo.

## Requirements

### Requirement: Comando recipe list
El sistema SHALL proveer `ai-specs recipe list [path]` que escanee `catalog/recipes/`, lea cada `recipe.toml`, determine el estado de instalación desde el manifest local, y muestre una tabla legible.

#### Scenario: Lista con recipes disponibles e instaladas
- **WHEN** el catálogo contiene recipes y el manifest declara `[recipes.test-fixture]` con `enabled = true`
- **THEN** `recipe list` SHALL mostrar: ID, nombre, versión, y estado (`installed` / `available` / `disabled`)
- **AND** cada recipe del catálogo SHALL aparecer exactamente una vez

#### Scenario: Catálogo vacío
- **WHEN** `catalog/recipes/` solo contiene `.gitkeep`
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
