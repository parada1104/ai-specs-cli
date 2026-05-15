## Context

El runtime SDD actual delega cada fase a un subagente `general` con prompt detallado dentro del orquestador primario. No existe diferenciación nativa por fase: explore, proposal, apply, verify y archive comparten el mismo runner. Esto bloquea tres mejoras que el equipo identificó como necesarias:

1. **Aislamiento de tools por fase**: explore debería ser read-only; archive debería tener `gh` y permisos Trello; apply debería poder correr tests y commits pero no push/merge.
2. **Presupuestos de turnos por fase**: artifacts y apply tienen volúmenes muy distintos; un solo budget global no escala.
3. **Distribución como feature de producto**: cualquier proyecto que ejecute `ai-specs init` + `ai-specs sync` debería recibir el catálogo de subagentes si lo declara explícitamente, sin requerir hand-copying.

El CLI ya tiene una pipeline madura para distribuir contenido bundled hacia el workspace del proyecto (`lib/_internal/refresh-bundled.py` con `.ai-specs.lock` y sidecars `.new`), una capa de validación de `ai-specs.toml` (`lib/_internal/toml-read.py`), y un renderer para `AGENTS.md` (`lib/_internal/agents-md-render.py`). Reutilizamos esa pipeline en vez de inventar una nueva.

Claude Code soporta subagentes nativos via `.claude/agents/*.md` con frontmatter (`name`, `description`, `tools`, `model`). OpenCode y Cursor no exponen un equivalente equivalente public en V1; la solución debe degradar limpiamente a fallback inline en esos harnesses.

## Goals / Non-Goals

**Goals:**

- Definir `[sdd].sub_agents` como flag canónico, **opt-in**, con default `false` y validación booleana estricta.
- Empaquetar seis subagent files en el CLI bajo `bundled-agents/claude/sdd-*.md` (raíz del repo CLI, junto a `bundled-skills/` y `bundled-commands/`) con frontmatter y cuerpo que codifiquen rol, tools permitidos/bloqueados y budget de turnos.
- Crear `lib/_internal/agents-render.py` como módulo dedicado al despliegue de subagent files por harness, reutilizando los patrones de `refresh-bundled.py` (lock + sidecars + idempotencia).
- Integrar el nuevo renderer en `lib/sync.sh` después de `refresh-bundled` y antes de la generación de `AGENTS.md`.
- Extender `agents-md-render.py` para listar subagentes activos en el brief solo cuando el flag es `true` y al menos un harness soportado está habilitado.
- Publicar `ai-specs/contracts/subagent-frontmatter.md` separado del contrato de skills.
- Garantizar **backward compatibility estricta**: rama OFF idéntica a la actual, byte-identical en `AGENTS.md` y sin escrituras en `.claude/`.
- Cobertura de tests para rama OFF (regresión), rama ON con `claude` único harness, rama ON con harnesses mixtos, transición ON→OFF (huérfanos), idempotencia, y archivo modificado a mano (sidecar `.new`).

**Non-Goals:**

- Implementar subagentes nativos para OpenCode o Cursor. Esta iteración solo entrega Claude Code y fallback inline declarado en el brief.
- Definir un orquestador nuevo. La skill `openspec-phase-orchestrator` se limita a referenciar los IDs de subagentes; la implementación interna del dispatcher se difiere a la card #68.
- Eliminar archivos huérfanos automáticamente al cambiar `sub_agents` de `true` a `false`. Mantenemos el principio de no destruir trabajo del usuario sin acción explícita.
- Reescribir la pipeline `refresh-bundled.py`. El nuevo renderer reutiliza sus utilidades de hashing y lock; no consolidamos código en esta iteración.
- Modificar el contrato de `skill-frontmatter-contract`. Los subagent files no son skills y reciben su propio contrato.

## Decisions

### Decisión 1: `[sdd].sub_agents` como booleano, no enum

**Elegido**: campo booleano simple (`true`/`false`).

**Alternativas consideradas**: enum (`disabled` / `claude_native` / `inline`), tabla anidada `[sdd.sub_agents]` con sub-flags por harness.

**Por qué**: el contrato de despliegue ya está condicionado por `[agents].enabled`. Agregar un enum o tabla anidada duplicaría esa información y abriría caminos para inconsistencias (`sub_agents = "claude_native"` con `claude` ausente de `[agents].enabled`). Un booleano deja la decisión de "qué harness usa nativo vs inline" en manos del contrato `sdd-subagent-deployment` y de `[agents].enabled`. Es la opción más pequeña que cubre el caso y se puede evolucionar a tabla si aparece una necesidad real.

### Decisión 2: Capability nueva `sdd-subagent-deployment` en vez de extender `sdd-cli-integration`

**Elegido**: nueva capability dedicada.

**Alternativas consideradas**: agregar todos los requirements como ADDED dentro de `sdd-cli-integration`.

**Por qué**: el despliegue de subagentes es un concepto autónomo con catálogo, frontmatter contract, reglas por harness y trazabilidad operativa. Mezclarlo dentro de `sdd-cli-integration` haría esa capability lo suficientemente grande para perder cohesión. Una capability nueva facilita futuras evoluciones (ej. añadir un séptimo subagente, soportar nuevos harnesses) sin tocar el contrato CLI base. `sdd-cli-integration` se limita a documentar cómo el subcomando reacciona al flag.

### Decisión 3: Renderer dedicado `lib/_internal/agents-render.py`

**Elegido**: módulo Python nuevo, invocado desde `lib/sync.sh`.

**Alternativas consideradas**: extender `refresh-bundled.py` con un modo "agents", o agregar lógica inline en `sync.sh` con `cp` + hashing manual.

**Por qué**: `refresh-bundled.py` ya tiene una lógica densa para skills y commands con su propio set de heurísticas (opted_out, lock, sidecars). Mezclar la lógica de subagent files heredando esas heurísticas obligaría a generalizar el módulo de manera prematura. Un módulo dedicado puede reusar las utilidades de `refresh-bundled` (hashing, normalización CRLF, gestión de `.ai-specs.lock`) vía import, mantener una superficie pequeña y testeable, y dejar espacio para reglas específicas (limpieza condicional de huérfanos, listado del runtime brief). El costo es duplicación mínima de la lógica de selección, que se compensa con claridad.

### Decisión 4: Reutilizar `.ai-specs.lock` para subagent files

**Elegido**: el mismo lock file rastrea hashes de subagent files con un prefijo `agents/<harness>/<name>.md`.

**Alternativas consideradas**: un lock dedicado `.ai-specs-agents.lock`.

**Por qué**: el lock existente ya cubre artefactos bundled distribuidos. Un segundo lock fragmenta el estado del proyecto y complica el flujo de migración. El prefijo en la clave evita colisiones con skills y commands.

### Decisión 5: Frontmatter de subagent files distinto del de skills

**Elegido**: contrato propio en `ai-specs/contracts/subagent-frontmatter.md`.

**Alternativas consideradas**: extender `skill-frontmatter-contract` para cubrir subagent files.

**Por qué**: los subagent files de Claude Code consumen campos específicos del harness (`tools`, `model`) que no aplican a `SKILL.md`. Mezclar ambos contratos en una sola capability obligaría a marcar campos como "solo aplica si el archivo es subagente" y dispararía falsos positivos en validadores. Un contrato separado mantiene cada artefacto con su superficie limpia. El contrato de subagent files es documental para V1; la validación estricta puede llegar después.

### Decisión 6: Fallback inline para harnesses sin soporte nativo

**Elegido**: cuando `sub_agents = true` pero el harness no es `claude`, el sistema NO materializa archivos y el runtime brief documenta el fallback inline.

**Alternativas consideradas**: emitir error si `sub_agents = true` y ningún harness soportado está habilitado; bloquear el sync hasta corregir.

**Por qué**: bloquear sync rompería proyectos que activan `sub_agents` con la expectativa de un futuro soporte para su harness. Documentar fallback inline en el brief mantiene el comportamiento útil (el orquestador sabe que debe correr fases inline) y honra la naturaleza opt-in del flag. La única condición de error real es: `sub_agents = true` y ningún harness habilitado en absoluto, que ya es un caso degenerado cubierto por la validación general de `[agents].enabled`.

### Decisión 7: Cambio `true → false` advierte, no destruye

**Elegido**: emitir aviso enumerando archivos huérfanos y sugerir comando explícito de limpieza.

**Alternativas consideradas**: limpiar automáticamente; crear un nuevo subcomando `ai-specs sdd cleanup`.

**Por qué**: el principio del CLI es no destruir archivos del workspace silenciosamente. El aviso da control al usuario. Un subcomando dedicado puede llegar después; en V1 el sugerencia textual es suficiente y mantiene el alcance del cambio acotado.

## Sequence Flow

### Flujo de sync con sub_agents activo (Claude Code)

```
Usuario                  bin/ai-specs       lib/sync.sh        refresh-bundled.py    agents-render.py    agents-md-render.py
   │                          │                   │                      │                    │                    │
   │── ai-specs sync ────────▶│                   │                      │                    │                    │
   │                          │── lib/sync.sh ───▶│                      │                    │                    │
   │                          │                   │── refresh-bundled ──▶│                    │                    │
   │                          │                   │                      │── lock + skills ─▶ │                    │
   │                          │                   │◀── ok ─────────────  │                    │                    │
   │                          │                   │── agents-render ────────────────────────▶│                    │
   │                          │                   │                      │                    │── leer manifiesto │
   │                          │                   │                      │                    │── chequear sub_agents
   │                          │                   │                      │                    │── chequear [agents].enabled
   │                          │                   │                      │                    │── materializar    │
   │                          │                   │                      │                    │   .claude/agents/sdd-*.md
   │                          │                   │                      │                    │── actualizar lock │
   │                          │                   │◀── ok ───────────────────────────────────│                    │
   │                          │                   │── agents-md-render ──────────────────────────────────────────▶│
   │                          │                   │                      │                    │                    │── leer manifiesto
   │                          │                   │                      │                    │                    │── si sub_agents=true
   │                          │                   │                      │                    │                    │  añadir sección
   │                          │                   │                      │                    │                    │── escribir AGENTS.md
   │                          │                   │◀── ok ───────────────────────────────────────────────────────│
   │◀── sync complete ────────│                   │                      │                    │                    │
```

### Decisión por harness en `agents-render.py`

```
manifest = read("ai-specs.toml")
flag = manifest["sdd"].get("sub_agents", False)
enabled = manifest["agents"]["enabled"]

if not flag:
    detect orphans in .claude/agents/sdd-*.md
    if orphans: warn(orphans)
    return

for harness in enabled:
    if harness == "claude":
        materialize bundled-agents/claude/sdd-*.md -> .claude/agents/sdd-*.md
        respect lock + .new sidecar
    else:
        # opencode, cursor, future harnesses
        record "inline fallback" for runtime brief
```

## Risks / Trade-offs

- **Riesgo**: usuarios consideran que `sub_agents = true` activa automáticamente subagentes en todos los harnesses y se confunden al ver que OpenCode/Cursor no reciben archivos. → **Mitigación**: documentación explícita en el runtime brief y el contrato `sdd-subagent-deployment` que declaran fallback inline; aviso en el log de sync cuando un harness queda sin materializar.
- **Riesgo**: el nuevo renderer agrega una step al pipeline de sync y eleva la duración de cada corrida aunque el flag esté OFF. → **Mitigación**: el módulo retorna inmediatamente si `sub_agents` es falso/ausente; el costo extra para la rama OFF debe ser indistinguible de no llamar al módulo (test de performance opcional, no requerido en V1).
- **Riesgo**: archivos huérfanos acumulándose al cambiar `true → false` repetidamente. → **Mitigación**: el aviso de huérfanos es persistente hasta que el usuario limpie manualmente; documentación describe el proceso de limpieza.
- **Riesgo**: editar a mano un subagent file y perder los cambios en el siguiente sync. → **Mitigación**: el patrón `.new` sidecar (heredado de `refresh-bundled`) garantiza que ediciones manuales se preservan y los cambios upstream quedan disponibles como `.new` para revisión humana.
- **Trade-off**: duplicación de lógica de hashing/lock entre `refresh-bundled.py` y `agents-render.py`. → **Compensación**: el módulo nuevo importa utilidades del existente; consolidación se difiere para evitar regresiones en V1.
- **Trade-off**: contrato de frontmatter separado del de skills puede confundir a contribuyentes nuevos. → **Compensación**: el README del CLI y la skill `skill-creator` documentarán cuándo aplica cada contrato.

## Migration Plan

1. **Pre-flight**: aplicar este cambio en una rama dedicada (worktree). Tests deben pasar con `[sdd].sub_agents` ausente (rama OFF idéntica a hoy).
2. **Rollout**: mergeable a `development` por PR vía `gh`. El flag default `false` hace que el cambio sea silencioso para todos los proyectos consumidores hasta que opten.
3. **Adopción**: un proyecto adopta el feature ejecutando:
   - editar `ai-specs/ai-specs.toml` para añadir `[sdd].sub_agents = true`,
   - correr `ai-specs sync`,
   - verificar que `.claude/agents/sdd-*.md` existen (si Claude Code está habilitado),
   - opcionalmente correr `ai-specs doctor` para confirmar.
4. **Rollback (proyecto consumidor)**: cambiar `sub_agents` a `false` (o removerlo), correr sync, eliminar los archivos huérfanos siguiendo la guía del aviso.
5. **Rollback (CLI mantenedor)**: revertir el commit que introduce este cambio. Los proyectos que ya activaron el flag perderán la materialización en el próximo `ai-specs upgrade`, pero los archivos materializados permanecen en su workspace (no se eliminan por revert).
6. **Coordinación con card #68**: la profesionalización del orquestador con subagent dispatch verdadero depende de este cambio; documentar en notas de PR.

## Open Questions

1. ¿La skill `openspec-phase-orchestrator` debe **requerir** los subagent files cuando `sub_agents = true`, o solo **referenciarlos** como mejora opcional? → Propuesta de tasks: referencia opcional en V1, requerimiento puede quedar para una iteración posterior atada a card #68.
2. ¿Necesitamos un `ai-specs sdd cleanup` subcomando para huérfanos en V1, o el aviso es suficiente? → Propuesta: aviso suficiente; subcomando opcional para iteración futura.
3. Para `model` en frontmatter de subagent: ¿lo hardcodeamos en el bundled source o lo dejamos vacío para que Claude Code use el default del usuario? → Propuesta: dejar vacío en V1 para no forzar tier; documentar la opción en el contrato.
4. Tests E2E reales contra Claude Code están fuera de scope (no hay automation que arranque Claude Code desde tests). ¿Suficiente con tests de filesystem que verifiquen presencia, contenido y idempotencia? → Propuesta: sí, suficiente para V1; verificación manual del flujo end-to-end queda en la columna de verify.
