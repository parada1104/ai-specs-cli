# sdd-subagent-deployment Specification

## Purpose
TBD - created by archiving change sdd-subagentes-especializados. Update Purpose after archive.
## Requirements
### Requirement: Catálogo canónico de subagentes SDD

El sistema MUST definir un catálogo cerrado de seis subagentes SDD identificados por nombre estable: `sdd-explore`, `sdd-proposal`, `sdd-artifacts`, `sdd-apply`, `sdd-verify`, `sdd-archive`. Cada subagente MUST tener un rol único, una lista explícita de tools permitidos, una lista explícita de tools bloqueados, y un presupuesto de turnos.

#### Scenario: Inventario completo en el catálogo bundled
- **GIVEN** el CLI distribuye bundled subagent files
- **WHEN** un mantenedor inspecciona `bundled-agents/claude/` en la raíz del CLI
- **THEN** el directorio MUST contener exactamente los seis archivos `sdd-explore.md`, `sdd-proposal.md`, `sdd-artifacts.md`, `sdd-apply.md`, `sdd-verify.md`, `sdd-archive.md`
- **AND** cada archivo MUST corresponder a un rol único del catálogo

#### Scenario: Roles y límites definidos
- **GIVEN** uno cualquiera de los seis subagent files bundled
- **WHEN** un agente o mantenedor lee su contenido
- **THEN** el archivo MUST declarar el rol del subagente
- **AND** MUST enumerar tools permitidos y tools bloqueados
- **AND** MUST declarar un budget de turnos numérico

### Requirement: Frontmatter canónico de subagent files

El sistema MUST publicar un contrato de frontmatter para `*.md` de subagentes que incluya como mínimo `name` (slug del subagente), `description` (resumen accionable), `tools` (lista o "all" según el harness destino) y, cuando aplique, `model`. El contrato MUST documentar que los archivos generados son artefactos derivados y SHALL NOT ser editados a mano fuera del CLI bundled source.

#### Scenario: Frontmatter de subagent cumple contrato
- **GIVEN** un subagent file bundled `sdd-explore.md`
- **WHEN** el contrato es validado
- **THEN** el frontmatter MUST contener `name = sdd-explore`
- **AND** MUST contener una `description` no vacía
- **AND** MUST contener una lista `tools` consistente con el rol read-only del subagente

#### Scenario: Contrato documentado y separado del de skills
- **GIVEN** `ai-specs/contracts/subagent-frontmatter.md`
- **WHEN** un mantenedor lo lee
- **THEN** el contrato MUST describir los campos requeridos, los opcionales y la regla de "derivado, no editado a mano"
- **AND** MUST aclarar explícitamente que es independiente del contrato `skill-frontmatter-contract`

### Requirement: Despliegue gobernado por el flag del manifiesto

El sistema MUST desplegar los subagent files en el harness destino solo cuando `[sdd].sub_agents = true` y el harness está habilitado en `[agents].enabled`. Cuando el flag es `false` o ausente, el sistema MUST NOT escribir, modificar ni dejar artefactos `.new` para subagent files.

#### Scenario: Sub_agents activado con Claude Code habilitado
- **GIVEN** `[sdd].sub_agents = true`
- **AND** `claude` está en `[agents].enabled`
- **WHEN** `ai-specs sync` se ejecuta
- **THEN** el sistema MUST materializar los seis archivos en `.claude/agents/sdd-*.md`
- **AND** los archivos materializados MUST ser byte-idénticos al bundled source

#### Scenario: Sub_agents desactivado preserva el harness
- **GIVEN** `[sdd].sub_agents = false` o el campo ausente
- **WHEN** `ai-specs sync` se ejecuta
- **THEN** el sistema MUST NOT crear `.claude/agents/sdd-*.md`
- **AND** si existían de un sync previo, el sistema MUST advertir y dejar la decisión de removerlos al usuario (no eliminar silenciosamente)

### Requirement: Fallback para harnesses sin soporte de subagentes nativos

El sistema MUST manejar harnesses que no soportan subagent files nativos (`opencode`, `cursor`) mediante un fallback documentado: NO materializar archivos en esos harnesses, y dejar registrado en el runtime brief que el orquestador primario ejecuta las fases SDD en línea cuando ese harness está habilitado.

#### Scenario: Sub_agents activado con harness sin soporte nativo
- **GIVEN** `[sdd].sub_agents = true`
- **AND** solo `opencode` está en `[agents].enabled`
- **WHEN** `ai-specs sync` se ejecuta
- **THEN** el sistema MUST NOT crear archivos de subagentes para ese harness
- **AND** el runtime brief MUST indicar que las fases SDD corren inline en el orquestador

#### Scenario: Sub_agents activado con harnesses mixtos
- **GIVEN** `[sdd].sub_agents = true`
- **AND** tanto `claude` como `opencode` están en `[agents].enabled`
- **WHEN** `ai-specs sync` se ejecuta
- **THEN** el sistema MUST materializar subagent files solo para `claude`
- **AND** el runtime brief MUST listar los subagentes activos para `claude` y declarar fallback inline para `opencode`

### Requirement: Idempotencia y limpieza del despliegue

El sistema MUST garantizar que re-ejecutar `ai-specs sync` con la misma configuración produce resultados byte-idénticos, y que cambiar el flag de `true` a `false` no destruye archivos en silencio.

#### Scenario: Re-sync con misma configuración produce resultado idéntico
- **GIVEN** `[sdd].sub_agents = true` y `claude` habilitado
- **WHEN** `ai-specs sync` corre dos veces sin cambios
- **THEN** la segunda corrida MUST producir los mismos archivos byte-idénticos
- **AND** MUST NOT emitir errores

#### Scenario: Desactivar sub_agents después de un sync previo
- **GIVEN** un sync previo materializó `.claude/agents/sdd-*.md`
- **WHEN** el usuario cambia `[sdd].sub_agents` a `false` y vuelve a correr `ai-specs sync`
- **THEN** el sistema MUST NOT eliminar archivos silenciosamente
- **AND** el sistema MUST emitir un aviso indicando los archivos huérfanos y sugerir su remoción manual o mediante un comando explícito documentado

### Requirement: Trazabilidad operativa en runtime brief

Cuando `[sdd].sub_agents = true` y al menos un harness soportado está habilitado, el sistema MUST listar los subagentes activos en el runtime brief generado, identificándolos por `name` y `description`, sin duplicar el cuerpo del subagent file.

#### Scenario: Listado de subagentes en runtime brief con sub_agents activo
- **GIVEN** `[sdd].sub_agents = true` y `claude` habilitado
- **WHEN** se genera `AGENTS.md`
- **THEN** el brief MUST incluir una sección que enumera los seis subagentes activos por `name` y `description`
- **AND** la sección MUST referenciar la ubicación canónica `.claude/agents/sdd-*.md`

#### Scenario: Sin sub_agents no se altera el runtime brief
- **GIVEN** `[sdd].sub_agents = false` o ausente
- **WHEN** se genera `AGENTS.md`
- **THEN** el brief MUST permanecer byte-idéntico al generado antes de introducir esta feature en proyectos cuyo manifiesto no cambió

