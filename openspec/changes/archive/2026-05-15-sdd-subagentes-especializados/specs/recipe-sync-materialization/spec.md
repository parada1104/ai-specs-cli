## ADDED Requirements

### Requirement: Materialización de subagent files por harness

Cuando `[sdd].sub_agents = true`, el sistema SHALL materializar subagent files solo en directorios específicos del harness habilitado, sin tocar `ai-specs/skills/`, `.recipe/`, ni `.deps/`. Para `claude`, el destino MUST ser `.claude/agents/sdd-*.md`. Para harnesses que no soportan subagentes nativos, la materialización MUST omitirse sin error.

#### Scenario: Materialización a Claude Code
- **GIVEN** `[sdd].sub_agents = true` y `claude` en `[agents].enabled`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST crear `.claude/agents/sdd-explore.md`, `.claude/agents/sdd-proposal.md`, `.claude/agents/sdd-artifacts.md`, `.claude/agents/sdd-apply.md`, `.claude/agents/sdd-verify.md`, `.claude/agents/sdd-archive.md`
- **AND** los archivos MUST ser byte-idénticos al bundled source en el CLI

#### Scenario: Harness no soportado se omite limpiamente
- **GIVEN** `[sdd].sub_agents = true` y solo `opencode` en `[agents].enabled`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST completar sin error
- **AND** sync MUST NOT crear archivos de subagentes para ese harness
- **AND** sync MAY anotar la omisión en el log o runtime brief

#### Scenario: Subagentes no contaminan ai-specs/skills/
- **GIVEN** `[sdd].sub_agents = true`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST NOT crear ni modificar archivos bajo `ai-specs/skills/`, `.recipe/`, ni `.deps/`
- **AND** los subagent files MUST quedar exclusivamente en el directorio del harness destino

### Requirement: Orden y respeto a la pipeline de sync

La materialización de subagent files SHALL integrarse con la pipeline existente respetando idempotencia, sidecars `.new` para archivos modificados localmente, y `.ai-specs.lock` para hashes.

#### Scenario: Re-sync idempotente
- **GIVEN** un sync previo creó los subagent files
- **WHEN** `ai-specs sync` corre nuevamente sin cambios
- **THEN** los archivos resultantes MUST ser byte-idénticos al estado previo
- **AND** `.ai-specs.lock` MUST reflejar hashes consistentes

#### Scenario: Archivo modificado localmente
- **GIVEN** un subagent file en `.claude/agents/sdd-explore.md` fue editado a mano
- **WHEN** `ai-specs sync` corre con bundled source distinto
- **THEN** sync MUST escribir el nuevo contenido en `.claude/agents/sdd-explore.md.new`
- **AND** sync MUST NOT sobrescribir el archivo editado silenciosamente
- **AND** sync MUST emitir un aviso que nombre el sidecar generado

### Requirement: Transición de sub_agents true → false

Cuando el flag cambia de `true` a `false` entre dos corridas de sync, el sistema MUST advertir sobre archivos huérfanos y MUST NOT eliminarlos silenciosamente.

#### Scenario: Cambio de true a false reporta huérfanos
- **GIVEN** un sync previo materializó `.claude/agents/sdd-*.md` con `sub_agents = true`
- **AND** el manifiesto ahora declara `sub_agents = false`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST emitir un aviso enumerando los archivos huérfanos detectados
- **AND** sync MUST sugerir un comando o acción explícita para removerlos
- **AND** sync MUST NOT eliminar los archivos por sí solo
