## Why

Hoy `recipe-materialize.py` hace `recipe_mcp[mcp.id] = mcp.config`, lo que significa que una recipe puede pisar completamente un MCP preset que el usuario ya configuró en su `ai-specs.toml`. Esto es destructivo, sorpresivo, y contradice la expectativa de que el proyecto es dueño de su propia configuración.

## What Changes

- Cambiar el merge de MCP presets de "overwrite" a "shallow merge con manifest precedence".
- En `build_recipe_mcp()`: el manifest del proyecto prevalece sobre los defaults de la recipe.
- En `materialize_recipes()`: misma semántica de merge para el recipe_mcp acumulado.
- Si hay conflicto de keys a nivel 1, emitir `warn` pero no pisar.
- Si el proyecto no tiene el MCP, la recipe lo crea completo.
- Tests de regresión: fixture con MCP existente + recipe que provee mismo MCP.

## Capabilities

### New Capabilities
- `mcp-preset-merge`: protocolo de merge seguro donde el proyecto gana siempre sobre los defaults de recipe.

### Modified Capabilities
- (ninguna: no cambian specs existentes, solo se corrige implementación)

## Impact

- `lib/_internal/recipe-materialize.py`: dos funciones modificadas (`build_recipe_mcp`, `materialize_recipes`).
- `tests/test_recipe_materialize.py` (o similar): nuevos tests de regresión.
- Sin breaking change para usuarios que no tenían MCPs en conflicto.
- Para usuarios que sí tenían conflictos previos, el comportamiento ahora preserva su config.
