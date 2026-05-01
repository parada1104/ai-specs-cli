## Context

`lib/_internal/recipe-materialize.py` tiene dos lugares donde mergea MCP presets: `build_recipe_mcp()` y `materialize_recipes()`. Ambos usan asignación directa (`recipe_mcp[mcp.id] = mcp.config`), lo que pisa completamente cualquier configuración previa del proyecto. Esto viola el principio de que el proyecto es dueño de su configuración.

## Goals / Non-Goals

**Goals:**
- Hacer shallow merge de MCP presets con precedencia del manifest del proyecto.
- Emitir warnings cuando haya conflictos de keys.
- Mantener backward compatibility para proyectos sin MCPs configurados.
- Agregar tests de regresión.

**Non-Goals:**
- Deep merge de config anidada (solo nivel 1).
- Validación de MCP schema.
- Cambiar la semántica de otros merges (skills, commands, templates).

## Decisions

1. **Shallow merge nivel 1**: Solo keys de primer nivel del dict de config. No se mergean dicts anidados. Rationale: simple, suficiente para el caso de uso actual, evita sorpresas con estructuras complejas.
2. **Precedencia del proyecto**: El manifest del proyecto gana siempre. Rationale: el usuario debe tener control total sobre su config.
3. **Warn en conflicto**: No se lanza excepción, solo warning. Rationale: la recipe sigue funcionando, solo se ignora el default conflictivo.
4. **Dos lugares, mismo patrón**: Tanto `build_recipe_mcp()` como `materialize_recipes()` usan la misma lógica de merge. Rationale: consistencia y predictibilidad.

## Risks / Trade-offs

- [Risk] Algunas recipes podrían depender del comportamiento anterior de overwrite. → Mitigation: este es un bug, no una feature. El comportamiento anterior era destructivo.
- [Risk] Los warnings pueden generar ruido en proyectos con muchas recipes. → Mitigation: solo se emite cuando hay conflicto real de keys.
