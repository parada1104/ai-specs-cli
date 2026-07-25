# recipe-cli delta — internal test fixtures

## ADDED Requirements

### Requirement: Internal test recipe ids are not installable via CLI

Recipe ids matching the internal fixture convention (`test-` prefix, as defined
by `is_internal_test_recipe`) SHALL NOT be installable or briefable through
consumer-facing CLI commands, even if a matching directory somehow exists under
the CLI catalog.

#### Scenario: recipe add rejects internal test id
- **WHEN** se ejecuta `recipe add test-fixture` (or any id where
  `is_internal_test_recipe(id)` is true)
- **THEN** SHALL fallar con un mensaje que indique que la recipe es un fixture
  interno de tests y no forma parte del catálogo público
- **AND** NO SHALL mutar el manifest
- **AND** exit code SHALL ser 1

#### Scenario: recipe init rejects internal test id
- **WHEN** se ejecuta `recipe init test-fixture` (or any internal test id)
- **THEN** SHALL fallar con el mismo criterio de rechazo
- **AND** exit code SHALL ser 1

### Requirement: Internal test recipes are not shipped in the production catalog

The distributed CLI catalog at `$AI_SPECS_HOME/catalog/recipes/` MUST NOT contain
directories whose names match the internal fixture convention. Those fixtures
live under the repo's `tests/fixtures/recipes/` for the test suite only.

#### Scenario: Shipped catalog has no test-* recipes
- **GIVEN** a normal CLI install (or repo `catalog/recipes/` as shipped)
- **WHEN** the catalog directory is listed
- **THEN** no entry SHALL have an id / directory name matching
  `is_internal_test_recipe`
- **AND** `recipe list` continues to omit any such ids (defense in depth)

#### Scenario: Materialize refuses enabled internal test id
- **GIVEN** a manifest that somehow declares `[recipes.test-fixture] enabled = true`
- **AND** `test-fixture` is not present in the production catalog
- **WHEN** sync / recipe-materialize runs
- **THEN** the operation SHALL NOT materialize skills, commands, templates, docs,
  or MCP from an internal fixture
- **AND** SHALL surface an explicit error or skip for that recipe id (no silent
  success that leaves agent-facing `test-skill` / `test-command` artifacts)

## MODIFIED Requirements

### Requirement: Comando recipe add

Happy-path scenarios that previously used `test-fixture` as a stand-in catalog
recipe MUST use a non-internal synthetic or public recipe id instead. Internal
ids are covered by the reject scenarios above.

#### Scenario: Agregar recipe disponible
- **WHEN** se ejecuta `recipe add <public-or-synthetic-id>` y la recipe existe
  en el catálogo del CLI
- **THEN** SHALL agregar `[recipes.<id>]` con `enabled = true` y SIN `version`
- **AND** SHALL mostrar preview de skills, commands, mcp, templates y docs
- **AND** exit code SHALL ser 0
- **AND** no materialize/sync is triggered
