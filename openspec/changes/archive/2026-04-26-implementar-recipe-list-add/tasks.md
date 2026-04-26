## 1. Infrastructure & Dispatch

- [x] 1.1 Agregar caso `recipe` al dispatcher de `bin/ai-specs` con subcomandos `list` y `add`
- [x] 1.2 Crear `lib/recipe.sh` como sub-dispatcher de `recipe list` y `recipe add`
- [x] 1.3 Crear stubs vacíos `lib/recipe-list.sh` y `lib/recipe-add.sh` con `--help`
- [x] 1.4 Validar que `bin/ai-specs recipe --help` muestra ayuda y no falla

## 2. Recipe List — Core

- [x] 2.1 Crear `lib/_internal/recipe-list.py` con función `list_recipes(catalog_dir, manifest_path)`
- [x] 2.2 Implementar escaneo de `catalog/recipes/` (excluir `.gitkeep`)
- [x] 2.3 Implementar lectura de `ai-specs.toml` para detectar `[recipes.*]`
- [x] 2.4 Implementar resolución de estado: `installed` / `disabled` / `available`
- [x] 2.5 Implementar renderizado de tabla ASCII/TSV a stdout
- [x] 2.6 Conectar `lib/recipe-list.sh` → `lib/_internal/recipe-list.py`
- [x] 2.7 Manejar catálogo vacío con mensaje amigable
- [x] 2.8 Manejar proyecto no inicializado con error explícito

## 3. Recipe Add — Core

- [x] 3.1 Crear `lib/_internal/recipe-add.py` con función `add_recipe(catalog_dir, manifest_path, recipe_id)`
- [x] 3.2 Implementar validación de existencia de recipe en catálogo
- [x] 3.3 Implementar lectura de versión exacta desde `recipe.toml`
- [x] 3.4 Implementar lectura de manifest local y detección de duplicados
- [x] 3.5 Implementar escritura de `[recipes.<id>]` en `ai-specs.toml`
- [x] 3.6 Implementar preview de primitives (skills, commands, mcp, templates, docs)
- [x] 3.7 Conectar `lib/recipe-add.sh` → `lib/_internal/recipe-add.py`
- [x] 3.8 Manejar recipe ya instalada: warning + abort sin mutación
- [x] 3.9 Manejar proyecto no inicializado con error explícito

## 4. Tests — Recipe List

- [x] 4.1 Crear `tests/test_recipe_list.py` con clase `RecipeListTests`
- [x] 4.2 Test: lista muestra recipe disponible cuando no está en manifest
- [x] 4.3 Test: lista muestra `installed` cuando `[recipes.<id>]` tiene `enabled = true`
- [x] 4.4 Test: lista muestra `disabled` cuando `[recipes.<id>]` tiene `enabled = false`
- [x] 4.5 Test: catálogo vacío devuelve lista vacía
- [x] 4.6 Test: recipe.toml inválido se marca con estado `error`
- [x] 4.7 Test: CLI `bin/ai-specs recipe list` en fixture de tests produce output

## 5. Tests — Recipe Add

- [x] 5.1 Crear `tests/test_recipe_add.py` con clase `RecipeAddTests`
- [x] 5.2 Test: add agrega recipe al manifest con versión exacta
- [x] 5.3 Test: add aborta cuando recipe ya existe en manifest
- [x] 5.4 Test: add falla cuando recipe ID no existe en catálogo
- [x] 5.5 Test: add no muta archivos del proyecto (solo ai-specs.toml)
- [x] 5.6 Test: add muestra preview correcto de primitives
- [x] 5.7 Test: doble add es idempotente (falla limpio, no duplica)

## 6. Validación & Polish

- [x] 6.1 Ejecutar `./tests/run.sh` y asegurar que todos los tests pasan
- [x] 6.2 Ejecutar `./tests/validate.sh` (py_compile + bash -n)
- [x] 6.3 Verificar que `bin/ai-specs recipe list` y `recipe add` respetan `--help`
- [x] 6.4 Actualizar `AGENTS.md` si es necesario (autogenerado vía sync)
- [x] 6.5 Verificar cobertura mínima: 3+ escenarios por comando
