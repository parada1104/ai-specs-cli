## Why

EPIC 2 — Recipes foundation ya tiene el schema `recipe.toml` diseñado (#19) y el contrato de manifest para `[recipes.<id>]` definido. Sin embargo, los usuarios no tienen una interfaz CLI para descubrir qué recipes están disponibles en `catalog/recipes/` ni para declararlas en su manifest. Hoy todo es manual: hay que conocer el ID, editar `ai-specs.toml` a mano, y ejecutar `sync` sin validación previa. Necesitamos dos comandos mínimos que cierren el ciclo de UX de recipes antes de avanzar a EPIC 3 (memory recipes).

## What Changes

- Agregar comando `ai-specs recipe list` que escanee `catalog/recipes/`, lea cada `recipe.toml`, y muestre: ID, nombre, descripción, versión, y estado (instalada / disponible / deshabilitada).
- Agregar comando `ai-specs recipe add <id>` que: valide que la recipe existe en el catálogo, lea el manifest local, agregue `[recipes.<id>]` con `enabled = true` y `version` exacta del catálogo, y muestre preview de primitives que se materializarán en el próximo `sync`.
- Ambos comandos deben reutilizar `lib/_internal/recipe-read.py` y `lib/_internal/recipe_schema.py` existentes.
- Ambos comandos deben respetar el contrato `recipe-manifest-contract` y no materializar nada por sí mismos.
- Agregar tests unitarios e integración para ambos comandos.

## Capabilities

### New Capabilities
- `recipe-list`: Comando CLI para listar recipes disponibles e instaladas con metadata y estado.
- `recipe-add`: Comando CLI para declarar una recipe en el manifest local con validación y preview de primitives.

### Modified Capabilities
- (Ninguna — este cambio es puramente aditivo; no modifica requisitos de specs existentes)

## Impact

- `bin/ai-specs`: Nuevos casos en el dispatcher para `recipe` subcomandos.
- `lib/`: Nuevos scripts `recipe-list.sh` / `recipe-list.py` y `recipe-add.sh` / `recipe-add.py`.
- `tests/`: Nuevos archivos de test.
- `catalog/recipes/`: Solo lectura; no modifica recipes existentes.
- `ai-specs.toml`: Solo escritura controlada por `recipe add`.
