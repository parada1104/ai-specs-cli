## ADDED Requirements

### Requirement: Opt-in estricto de sub_agents

El campo `[sdd].sub_agents` MUST ser opt-in puro. Su ausencia o su declaración explícita `false` MUST preservar el comportamiento V1 del CLI sin diferencias observables.

#### Scenario: Manifiesto sin sub_agents preserva V1
- **GIVEN** un `ai-specs.toml` sin la sección `[sdd]` o con `[sdd]` que omite `sub_agents`
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST producir exactamente los mismos artefactos que producía antes de introducir esta feature
- **AND** MUST NOT crear `.claude/agents/sdd-*.md`

#### Scenario: Manifiesto con sub_agents false preserva V1
- **GIVEN** un `ai-specs.toml` con `[sdd].sub_agents = false` explícitamente
- **WHEN** `ai-specs sync` corre
- **THEN** sync MUST tratar el caso igual que la ausencia del campo
- **AND** MUST NOT crear `.claude/agents/sdd-*.md`
- **AND** el runtime brief MUST permanecer idéntico al generado para un manifiesto sin el campo

### Requirement: Compatibilidad con flujo manual existente

El introducción de `sub_agents` MUST NOT romper el flujo de orquestación primaria existente. El orquestador primario MUST seguir siendo capaz de ejecutar el ciclo SDD inline sin subagentes nativos, tanto cuando el flag está apagado como cuando está prendido en harnesses sin soporte nativo.

#### Scenario: Orquestador inline sin sub_agents
- **GIVEN** `[sdd].sub_agents` es `false` o ausente
- **WHEN** se ejecuta un ciclo SDD vía la skill `openspec-phase-orchestrator`
- **THEN** el orquestador primario MUST ejecutar las fases inline como hoy
- **AND** la skill MUST NOT exigir la existencia de subagent files

#### Scenario: Orquestador inline en harness sin soporte
- **GIVEN** `[sdd].sub_agents = true` y solo un harness sin soporte nativo de subagentes está habilitado
- **WHEN** se ejecuta un ciclo SDD vía la skill `openspec-phase-orchestrator`
- **THEN** el orquestador primario MUST ejecutar las fases inline
- **AND** la skill MUST documentar el modo fallback en su salida o handoff
