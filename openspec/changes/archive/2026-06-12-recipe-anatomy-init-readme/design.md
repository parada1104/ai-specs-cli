## Context

El catálogo de recipes hoy tiene un solo caso real (`trello-mcp-workflow`) y varios fixtures. La estructura actual ubica `init.md` y `README.md` bajo `docs/`, lo que mezcla audiencias en un mismo subdirectorio: `init.md` es para el agente, `README.md` es para humanos. Además, `init.md` contiene una guía descriptiva ("ask or confirm these values: ...") en vez de un contrato estructurado que el agente pueda ejecutar paso a paso.

El wrapper de invocación actual es solo el shell `ai-specs recipe init <id>`, que imprime un brief read-only (recipe metadata + estado del manifest + contenido de `init.md`). Quien escribe el TOML es el agente, leyendo el output. Este flujo funciona pero requiere que el usuario copie/pegue manualmente: ejecuta `ai-specs recipe init`, captura el output, lo pega al agente. Un slash command `/recipe-init <id>` cierra ese loop sin cambiar la responsabilidad de escritura.

## Goals / Non-Goals

**Goals:**
- Establecer layout canónico claro: `README.md` (humano, raíz), `init.md` (agente, raíz, ejecutable), `SKILL.md` (anidado en `skills/<id>/`), `commands/`, `templates/`.
- Convertir `init.md` en contrato ejecutable: secciones predecibles (preguntas, defaults, validaciones, target TOML) que el agente sigue mecánicamente.
- Cerrar el loop usuario↔agente con `/recipe-init <id>` que invoca el shell por debajo y entrega el output al contexto del agente.
- Migrar `trello-mcp-workflow` al nuevo layout como caso de referencia.
- Mantener backward-compatibility de comportamiento: `recipe-init.py` sigue read-only; el shell sigue retornando el mismo brief estructural.

**Non-Goals:**
- No tocar el flag `usas_sdd` ni desacoplar capacidades Trello del lifecycle SDD (futura card).
- No definir triggers alternativos para recipes que no usen SDD.
- No introducir vendor namespace (`vendor:ai-specs:<recipe>`).
- No migrar otras recipes (no existen otras reales; las fixtures de test son irrelevantes para este layout).
- No convertir `recipe-init.py` en interactivo (sigue read-only; el agente escribe el TOML).
- No eliminar `ai-specs recipe add` como comando standalone; sigue siendo el primitivo para humanos.

## Decisions

### D1. `init.md` y `README.md` van a la raíz de la recipe, no a `docs/`

**Razón**: separa audiencias por ubicación. La raíz es el "manifiesto" de la recipe; `docs/` queda libre para docs instalables (`provides.docs[]`) que se materializan en el proyecto consumidor.

**Alternativas consideradas**:
- *Mantener `docs/init.md`*: status quo. Mezcla audiencias, no soluciona el problema base.
- *`agent/init.md` y `humans/README.md`*: separación más explícita pero introduce dos subdirectorios nuevos sin beneficio real para una raíz que ya es semántica clara.

### D2. `SKILL.md` permanece en `skills/<id>/` (no se mueve a raíz)

**Razón**: `provides.skills` es array — una recipe puede declarar varias skills. Forzar `SKILL.md` a la raíz crearía un caso especial para single-skill que no escala. Mantener `skills/<id>/SKILL.md` siempre es predecible.

**Alternativas consideradas**:
- *`SKILL.md` en raíz si N=1, anidado si N>1*: introduce branching de layout que el lector tiene que aprender. Costo de cognición > ahorro de un nivel de directorio.
- *`SKILL.md` siempre en raíz, forzar single-skill*: limita expresividad sin justificación.

### D3. `init.md` ejecutable usa formato declarativo, no imperativo

**Forma propuesta** (sketch):

```markdown
# Recipe Init Contract

> Sigue este script para configurar la recipe. Pregunta cada bloque, valida la
> respuesta, y al final propón el diff a `ai-specs/ai-specs.toml` para review humano.

## Preguntas

### board_id
- **Required**: yes
- **Type**: string
- **Pregunta**: "¿Cuál es el ID del board de Trello para este proyecto?"
- **Validación**: 24 caracteres hex (formato Trello).
- **Hint**: el board ID está en la URL: `https://trello.com/b/<board_id>/...`

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

\`\`\`toml
[recipes.trello-mcp-workflow.config]
board_id = "<answer:board_id>"
default_list = "<answer:default_list>"  # omitir si es default
epic_list = "<answer:epic_list>"        # omitir si es default
\`\`\`

## Post-write

- Recordar al usuario: ejecutar `ai-specs sync` para materializar.
- No invocar `sync` automáticamente; requiere review humano.
```

**Razón**: el formato declarativo (preguntas + types + defaults + target) es interpretable por cualquier modelo sin ambigüedad. El imperativo ("ask the user X, then ask Y") deja decisiones a interpretación.

**Alternativas consideradas**:
- *YAML/TOML como formato estructurado*: más estricto pero pierde la flexibilidad de Markdown para hints y ejemplos. Markdown con secciones convencionales es lo bastante estructurado para que el agente lo siga sin necesitar parseo formal.
- *JSON Schema embebido*: overkill; las preguntas son <10 por recipe.

### D4. Slash command `/recipe-init` es un wrapper, no agrega lógica

**Flujo**:

```
Usuario:  /recipe-init trello-mcp-workflow
   │
   ▼
Slash command (.claude/commands/recipe-init.md):
   │  1. Bash: ai-specs recipe init trello-mcp-workflow
   │  2. Captura stdout (brief + init.md)
   │  3. Instruye al agente a seguir el contrato
   ▼
Agente:
   │  Sigue init.md sección por sección
   │  Pregunta al usuario, valida respuestas
   │  Propone diff a ai-specs.toml
   ▼
Usuario:  acepta diff
   │
   ▼
Edit:    ai-specs/ai-specs.toml
   │
   ▼
Usuario:  ai-specs sync (cuando esté listo)
```

**Razón**: el slash command no introduce estado nuevo. Es UX wrapper sobre el shell existente. Toda la lógica vive en `init.md` y la interpretación del agente.

### D6. `recipe add` escribe placeholders de config en el manifest

**Razón**: si un usuario humano ejecuta `ai-specs recipe add <id>` y luego `sync`, el sync fallaría con "missing required config field" sin que el usuario sepa qué campos necesita. Al appendear `[recipes.<id>.config]` con placeholders (`""` para requeridos, defaults para opcionales) al momento de `add`, el manifest es self-documenting: el usuario ve inmediatamente qué debe completar.

**Implementación**: `recipe-add.py` lee `recipe.config_schema.fields` después de validar la recipe y, antes de escribir el manifest, genera líneas de placeholder ordenadas alfabéticamente:
- `required = true` → `key = ""  # REQUIRED`
- `default` presente → `key = "default_value"`
- opcional sin default → `# key = ""  # optional`

**Impacto en `recipe init`**: como `recipe add` ya dejó placeholders, el brief de `recipe init` muestra "Existing config keys" con los placeholders en lugar de "(none)", y las instrucciones pasan de "Add required X" a "Update existing key X". Esto es consistente y menos confuso.

### D5. `README.md` de la recipe no se instala en el proyecto consumidor

**Razón**: el README de raíz documenta la recipe **para quien la consume desde el catálogo** (humanos viendo `catalog/recipes/<id>/README.md`). La doc instalable en el proyecto consumidor es responsabilidad de `provides.docs[]`, que apunta a archivos separados (típicamente bajo `docs/<project-doc>.md`).

**Implicación**: `trello-mcp-workflow` hoy declara `provides.docs[]` con `source = "docs/README.md", target = "docs/trello-mcp-workflow.md"`. El nuevo `README.md` raíz reemplaza al `docs/README.md` actual; `provides.docs[]` debe apuntar al nuevo `README.md` raíz **o** a un archivo separado dedicado a la doc instalable.

**Decisión**: dejar que `provides.docs[]` apunte al nuevo `README.md` raíz. Simplifica: un solo archivo describe la recipe, y el sync lo materializa en el proyecto consumidor con renaming. Si en el futuro se quiere divergir (catálogo vs proyecto), se introduce un archivo separado.

## Risks / Trade-offs

- **[Risk]** El layout cambia para `trello-mcp-workflow`, que es la única recipe real. Tests que asumen `docs/init.md` fallarán.  
  **Mitigation**: actualizar tests en el mismo PR; correr `./tests/validate.sh` antes de archive.

- **[Risk]** `init.md` ejecutable depende de que el agente interprete consistente el formato declarativo. Distintos modelos podrían divergir en cómo formulan preguntas.  
  **Mitigation**: el contrato fija el TOML target literal, no la formulación de la pregunta. Variación cosmética en preguntas es aceptable; lo crítico es el output.

- **[Trade-off]** El slash command `/recipe-init` añade un punto de entrada nuevo sin reducir el shell `ai-specs recipe init`. Hay redundancia.  
  **Aceptado**: el slash command es Claude-specific; el shell sigue siendo el primitivo invocable desde otros agentes/runtimes.

- **[Risk]** Si `provides.docs[]` apunta al nuevo `README.md` raíz, el README **se vuelve un asset doble propósito** (catálogo + instalado en proyecto). Lectores podrían esperar contenidos distintos.  
  **Mitigation**: el README es lo bastante genérico (qué hace, instalación, config) para servir ambas audiencias. Si diverge en el futuro, separar en dos archivos.

## Migration Plan

1. Crear nuevos archivos en raíz: `init.md` (contenido nuevo), `README.md` (contenido actual de `docs/README.md` o reescrito).
2. Eliminar `docs/init.md` y `docs/README.md` (después de mover/copiar contenido).
3. Actualizar `recipe.toml`:
   - `[init].prompt = "init.md"` (era `"docs/init.md"`)
   - `provides.docs[].source = "README.md"` (era `"docs/README.md"`)
4. Actualizar tests que referencien las rutas viejas.
5. Crear `.claude/commands/recipe-init.md` con el flujo del slash command.
6. Actualizar spec `recipe-schema` con la convención canónica.
7. Crear spec `recipe-init-contract` con el formato declarativo.
8. `./tests/validate.sh` debe pasar antes de archive.

**Rollback**: `git revert` del PR. No hay migración de datos ni efecto sobre proyectos ya sincronizados (la materialización en `ai-specs/recipes/<id>/` no cambia su layout — solo cambia dónde el catálogo guarda los assets fuente).

## Open Questions

- Ninguna pendiente que bloquee el avance. Decisiones cerradas con el usuario en explore.
