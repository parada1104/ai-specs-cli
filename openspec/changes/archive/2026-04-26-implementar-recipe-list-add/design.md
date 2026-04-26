## Overview

Este cambio agrega dos comandos CLI (`recipe list`, `recipe add`) que cierran el ciclo de descubrimiento e instalación declarativa de recipes. El diseño sigue el patrón existente de `bin/ai-specs` + `lib/*.sh` + `lib/_internal/*.py`.

## Architecture

### Entrypoints

```
bin/ai-specs
├── recipe) bash "$LIB_DIR/recipe.sh" "$@" ;;
```

`lib/recipe.sh` actúa como sub-dispatcher:
```
recipe list [path]   → bash "$LIB_DIR/recipe-list.sh" "$path"
recipe add <id> [path] → bash "$LIB_DIR/recipe-add.sh" "$id" "$path"
```

### Recipe List (`lib/recipe-list.sh` + `lib/_internal/recipe-list.py`)

**Flujo:**
1. Resolver `catalog/recipes/` desde el proyecto (default: cwd).
2. Listar directorios (excluyendo `.gitkeep`).
3. Por cada directorio, ejecutar `recipe-read.py` para obtener metadata JSON.
4. Leer `ai-specs.toml` para detectar `[recipes.*]` instaladas.
5. Renderizar tabla TSV/ASCII con columnas: ID, Nombre, Versión, Estado.

**Estados:**
- `installed` → `[recipes.<id>]` existe y `enabled = true`
- `disabled` → `[recipes.<id>]` existe y `enabled = false`
- `available` → no existe en el manifest

**Salida:**
```
ID                     NAME                           VERSION   STATUS
runtime-memory-openmemory  Runtime Memory (OpenMemory)    1.0.0     installed
test-fixture           Test Fixture Recipe            1.0.0     available
```

### Recipe Add (`lib/recipe-add.sh` + `lib/_internal/recipe-add.py`)

**Flujo:**
1. Validar que `<id>` existe en `catalog/recipes/<id>/recipe.toml`.
2. Leer manifest local (`ai-specs.toml`).
3. Si `[recipes.<id>]` ya existe → advertir y abortar (idempotencia: no sobrescribir).
4. Agregar `[recipes.<id>]` con `enabled = true` y `version = <exact_version_from_catalog>`.
5. Mostrar preview de primitives:
   ```
   Recipe 'test-fixture' agregada al manifest.
   Próximo sync materializará:
   - skills: test-skill
   - commands: test-command
   - mcp: test-mcp
   - templates: test-template → docs/test-template-output.md
   - docs: test-doc → docs/test-doc-output.md
   ```

**Validaciones:**
- Recipe ID no existe en catálogo → error explícito.
- `ai-specs.toml` no existe → error (requiere `ai-specs init` previo).
- `[recipes.<id>]` ya existe → warning + abort (no mutación accidental).

## Data Flow

```
Usuario → bin/ai-specs recipe list
        → lib/recipe-list.sh
        → lib/_internal/recipe-list.py
        → catalog/recipes/*/recipe.toml (lectura)
        → ai-specs.toml (lectura)
        → stdout (tabla)

Usuario → bin/ai-specs recipe add <id>
        → lib/recipe-add.sh
        → lib/_internal/recipe-add.py
        → catalog/recipes/<id>/recipe.toml (lectura)
        → ai-specs.toml (lectura + escritura)
        → stdout (confirmación + preview)
```

## Error Handling

- Todos los errores van a `stderr` con código de salida ≠ 0.
- Mensajes en español (consistente con CLI existente).
- Errores de parsing de TOML usan los mensajes ya definidos en `recipe-read.py`.

## Testing Strategy

- **Unit tests** (`test_recipe_list.py`, `test_recipe_add.py`): Módulos Python puros con `tempfile` + fixtures.
- **Integration tests**: Invocar `bin/ai-specs recipe list` y `recipe add` en `tests/fixtures/` con proyecto de prueba.
- **Cobertura mínima**: al menos 3 escenarios por comando (happy path + 2 errores).

## Open Questions / Decisions

1. **¿Formato de salida?** Tabla ASCII simple (consistente con `doctor`). No JSON/TSV por ahora.
2. **¿`recipe add` con `--force`?** No en V1. Si el usuario quiere cambiar version, edita manualmente.
3. **¿`recipe remove`?** Fuera de scope. Se puede deshabilitar con `enabled = false` manual.
