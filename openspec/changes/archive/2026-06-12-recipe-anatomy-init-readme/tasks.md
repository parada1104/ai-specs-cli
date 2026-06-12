## 1. Migrar trello-mcp-workflow al layout canónico

- [x] 1.1 Crear `catalog/recipes/trello-mcp-workflow/init.md` con el contrato ejecutable (secciones `Preguntas`, `MCP Discovery`, `TOML Target`, `Post-write`) cubriendo `board_id`, `default_list`, `epic_list` y la MCP `trello`.
- [x] 1.2 Crear `catalog/recipes/trello-mcp-workflow/README.md` (raíz) con la doc humana actual de `docs/README.md` revisada para servir como descripción del catálogo.
- [x] 1.3 Eliminar `catalog/recipes/trello-mcp-workflow/docs/init.md` y `catalog/recipes/trello-mcp-workflow/docs/README.md`.
- [x] 1.4 Actualizar `catalog/recipes/trello-mcp-workflow/recipe.toml`: `[init].prompt = "init.md"` (era `"docs/init.md"`) y `provides.docs[].source = "README.md"` (era `"docs/README.md"`).
- [x] 1.5 Si el directorio `docs/` queda vacío en la recipe, eliminarlo del catálogo.

## 2. Extender recipe-add.py para placeholders de config

- [x] 2.1 Modificar `lib/_internal/recipe-add.py` para que, al appendear `[recipes.<id>]`, también appendee `[recipes.<id>.config]` con:
  - Campos requeridos: `key = ""  # REQUIRED`
  - Campos opcionales con default: `key = "default_value"`
  - Campos opcionales sin default: `# key = ""  # optional`
- [x] 2.2 Agregar test `test_add_writes_config_placeholders` en `tests/test_recipe_add.py`.

## 3. Slash command /recipe-init y skill

- [x] 3.1 Crear el archivo del slash command (`.claude/commands/recipe-init.md`) con el contrato: ejecutar `ai-specs recipe init <id>`, capturar stdout, instruir al agente a seguir el contrato secuencialmente, preguntar al usuario, proponer diff, y escribir al manifest tras aprobación humana.
- [x] 3.2 Crear skill `.claude/skills/recipe-init/SKILL.md` documentando el patrón para agentes.
- [x] 3.3 Documentar en el slash command/skill que no se invoca `ai-specs sync`; el agente solo propone diff a `ai-specs.toml` para review humano.

## 4. Especificaciones canónicas

- [x] 4.1 Confirmar que el cambio en `recipe-schema/spec.md` documenta el layout canónico (root `README.md` + `init.md`, skills anidados, `docs/` reservado para `provides.docs[]`).
- [x] 4.2 Confirmar que el nuevo spec `recipe-init-contract/spec.md` cubre estructura del documento, formato de preguntas, TOML target, MCP discovery, post-write, slash command y read-only runtime.
- [x] 4.3 Actualizar `design.md` con decisión D6: `recipe add` escribe placeholders de config.

## 5. Tests

- [x] 5.1 Revisar `tests/test_recipe_init.py` y actualizar cualquier referencia a `docs/init.md` para apuntar al nuevo path en raíz.
- [x] 5.2 Revisar `tests/test_recipe_read.py` por referencias a la ruta vieja.
- [x] 5.3 Agregar test que valide el parseo de `[init].prompt = "init.md"` (root) además del existente para `docs/init.md`.
- [x] 5.4 Actualizar fixtures de test para layout root (`init.md` en raíz en lugar de `docs/init.md`).
- [x] 5.5 Ejecutar `./tests/run.sh` y `./tests/validate.sh` y dejarlos verdes.

## 6. Documentación operativa

- [x] 6.1 Si existe doc del catálogo que muestra el layout actual de una recipe, actualizar el ejemplo al layout canónico nuevo.
- [x] 6.2 Actualizar el AGENTS.md / runtime brief solo si menciona la convención vieja.

## 7. Verificación pre-archive

- [ ] 7.1 Correr `openspec validate recipe-anatomy-init-readme` sin errores.
- [x] 7.2 Correr `./tests/validate.sh` con éxito.
- [x] 7.3 Smoke manual: `ai-specs recipe init trello-mcp-workflow` debe imprimir el nuevo `init.md` como `Prompt Content` del brief.
- [x] 7.4 Confirmar que `recipe-init.py` sigue read-only (no toca `ai-specs.toml`).
- [x] 7.5 Confirmar que `recipe-add.py` escribe placeholders (sí toca `ai-specs.toml`, pero solo para `[recipes.<id>]` y `[recipes.<id>.config]`).
