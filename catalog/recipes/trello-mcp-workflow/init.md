# Recipe Init Contract

> Sigue este script para configurar la recipe. Pregunta cada bloque, valida la
> respuesta, y al final propón el diff a `ai-specs/ai-specs.toml` para review humano.

## Preguntas

### board_id
- **Required**: yes
- **Type**: string
- **Pregunta**: "¿Cuál es el ID del board de Trello para este proyecto?"
- **Validación**: 24 caracteres hex (formato Trello).
- **Hint**: El board ID real son 24 caracteres hex (ej: `69ec097f13e2d38ecd89a557`). **No** es el shortLink de 8 caracteres que aparece en la URL del board (`https://trello.com/b/<shortLink>/...`).
  - **Opción 1 (MCP Tool)**: Ejecuta `trello_get_active_board_info` — el campo `id` devuelve el board ID real.
  - **Opción 2 (UI de Trello)**: Board menu → More → Close board... — la URL de la página de cierre contiene el ID real.
  - **Opción 3 (API directa)**: `curl -s "https://api.trello.com/1/boards/<shortLink>?key=<key>&token=<token>"` — el campo `id` del JSON es el board ID real.

### default_list
- **Required**: no
- **Type**: string
- **Default**: `In Progress`
- **Pregunta**: "¿Qué lista usar como destino por defecto? (deja vacío para `In Progress`)"

### epic_list
- **Required**: no
- **Type**: string
- **Default**: `Epic`
- **Pregunta**: "¿Qué lista usar para epics? (deja vacío para `Epic`)"

## MCP Discovery

- Si `[mcp.trello]` no existe en `ai-specs.toml`, propón un bloque reviewable
  (no escribir credenciales; usar `${env:TRELLO_API_KEY}` y `${env:TRELLO_TOKEN}`).
- Si existe, no proponer cambios de credenciales.

## TOML Target

Escribe bajo `[recipes.trello-mcp-workflow.config]`:

```toml
[recipes.trello-mcp-workflow.config]
board_id = "<answer:board_id>"
default_list = "<answer:default_list>"  # omitir si es default
epic_list = "<answer:epic_list>"        # omitir si es default
```

## Post-write

- Recordar al usuario: ejecutar `ai-specs sync` para materializar.
- No invocar `sync` automáticamente; requiere review humano.
