## Why

`init.md` hoy es una guía descriptiva ("ask or confirm these values: ...") cuando el agente la necesita como **contrato ejecutable**: secuencia estructurada de preguntas y reglas que el agente sigue para escribir `[recipes.<id>.config]` al manifest sin ambigüedad. Además, la convención actual ubica `init.md` y `README.md` bajo `docs/`, mezclando audiencias (humano vs agente) en el mismo subdirectorio. La estructura canónica debe distinguir esa frontera para que cualquier recipe nueva tenga un patrón claro a seguir.

## What Changes

- Definir layout canónico de recipe con `README.md` (humano) e `init.md` (agente) en la raíz. `SKILL.md` permanece en `skills/<id>/SKILL.md` (multi-skill ready, sin cambios).
- Reescribir `init.md` como contrato ejecutable: bloques estructurados con preguntas, defaults, validaciones y el TOML resultante esperado. El agente lo sigue paso a paso para escribir `[recipes.<id>.config]`. **BREAKING** — cambia el contenido y la convención de ubicación de `init.md` para `trello-mcp-workflow`.
- Agregar `README.md` humano explícito en raíz de la recipe (qué hace, instalación, tabla de config). Este README **no se instala** en el proyecto consumidor; describe la recipe para humanos del catálogo. La doc instalable sigue siendo `provides.docs[]` apuntando a archivos separados.
- Crear slash command `/recipe-init <id>` que (a) ejecuta `ai-specs recipe init <id>` por debajo, (b) recibe el brief + contenido de `init.md` en el contexto del agente, (c) deja al agente seguir el contrato y proponer el diff a `ai-specs.toml` para review humano.
- Migrar `trello-mcp-workflow` al nuevo layout como caso de prueba: mover `docs/init.md` → `init.md`, `docs/README.md` → `README.md`, actualizar `recipe.toml` para apuntar a `init.md` en raíz.
- Actualizar la referencia en `recipe.toml` `[init].prompt` para `trello-mcp-workflow` de `docs/init.md` a `init.md`.

## Capabilities

### New Capabilities

- `recipe-init-contract`: contrato ejecutable de `init.md` — formato estructurado (preguntas, defaults, validaciones, TOML target) que el agente sigue para configurar una recipe. Incluye la UX del slash command `/recipe-init` como wrapper de invocación.

### Modified Capabilities

- `recipe-schema`: layout canónico de recipe — `README.md` e `init.md` en raíz pasan a ser convención canónica documentada. La spec actual permite `docs/` como subdirectorio opcional pero no fija dónde viven `init.md` y `README.md`; este cambio establece la convención.

## Impact

- **Specs**: `openspec/specs/recipe-schema/spec.md` (modified — añadir layout canónico de archivos raíz); nuevo `openspec/specs/recipe-init-contract/spec.md`.
- **Catalog**: `catalog/recipes/trello-mcp-workflow/` — mover `docs/init.md` → `init.md`, `docs/README.md` → `README.md`, reescribir contenido de `init.md`, escribir nuevo `README.md` humano, actualizar `recipe.toml`.
- **Slash commands**: nuevo archivo `.claude/commands/recipe-init.md` (o equivalente según el harness) que documenta la invocación.
- **Code**: `lib/_internal/recipe-init.py` no requiere cambios — sigue read-only; emite el brief con el contenido del nuevo `init.md` ejecutable. Solo se ajusta el `[init].prompt` en `recipe.toml`.
- **Tests**: actualizar `tests/test_recipe_init.py` y `tests/test_recipe_read.py` si referencian la ruta `docs/init.md`.
- **Docs**: si hay docs del catálogo que muestran el layout actual, actualizarlas para reflejar la nueva convención.

## Rollback

Cambio reversible por archivos:
- Restaurar `catalog/recipes/trello-mcp-workflow/docs/init.md` y `docs/README.md` desde git.
- Revertir `recipe.toml` `[init].prompt` a `docs/init.md`.
- Revertir spec `recipe-schema/spec.md`.
- Eliminar nuevo spec `recipe-init-contract/spec.md` y el slash command.
- No hay migración de datos ni efectos sobre proyectos consumidores ya sincronizados (el layout afecta al catálogo, no al output sincronizado en `ai-specs/recipes/<id>/`).
