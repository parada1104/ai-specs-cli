## ADDED Requirements

### Requirement: Sección opcional de subagentes SDD en el runtime brief

Cuando `[sdd].sub_agents = true` y al menos un harness soportado está habilitado, el runtime brief generado SHALL incluir una sección listando los subagentes SDD activos por `name` y `description`. Cuando el flag es `false` o ausente, el brief MUST NO contener esa sección y MUST preservar idempotencia byte-identical respecto al brief generado antes de la introducción de la feature.

#### Scenario: Sub_agents activo añade la sección
- **GIVEN** `[sdd].sub_agents = true` y `claude` habilitado
- **WHEN** `ai-specs sync` genera `AGENTS.md`
- **THEN** el brief MUST contener una subsección titulada explícitamente para subagentes SDD
- **AND** la subsección MUST listar los seis subagentes activos con `name` y `description` resumida
- **AND** la subsección MUST indicar la ubicación canónica `.claude/agents/sdd-*.md`

#### Scenario: Sub_agents desactivado mantiene brief idéntico
- **GIVEN** un proyecto sin cambios en `ai-specs.toml` salvo eventual edición de campos no relacionados con SDD
- **AND** `[sdd].sub_agents` es `false` o ausente
- **WHEN** `ai-specs sync` genera `AGENTS.md` en dos corridas con la misma configuración
- **THEN** ambas corridas MUST producir un brief byte-idéntico
- **AND** el brief MUST NO contener la sección de subagentes SDD

#### Scenario: Harness sin soporte nativo se documenta
- **GIVEN** `[sdd].sub_agents = true` y solo `opencode` habilitado
- **WHEN** `ai-specs sync` genera `AGENTS.md`
- **THEN** la sección de subagentes MUST indicar que `opencode` ejecuta las fases SDD inline en el orquestador
- **AND** MUST NO declarar archivos materializados en `.opencode/` ni en `.claude/`

### Requirement: Marker manual sigue ganando

Si `AGENTS.md` contiene el marker `<!-- ai-specs:runtime-brief -->`, el sync tool SHALL preservar el archivo manual sin escribir contenido auto-generado, incluso cuando `[sdd].sub_agents = true`. Esta regla MUST sobreescribir cualquier comportamiento de inserción automático.

#### Scenario: Marker manual evita reescritura aún con sub_agents activo
- **GIVEN** un `AGENTS.md` con marker manual
- **AND** `[sdd].sub_agents = true`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST NOT modificar `AGENTS.md`
- **AND** sync MUST continuar el resto de pasos sin error
- **AND** sync MAY registrar la omisión en log o salida
