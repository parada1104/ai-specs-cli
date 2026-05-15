## ADDED Requirements

### Requirement: Habilitación declarativa de sub_agents

Cuando `[sdd].sub_agents = true` en `ai-specs.toml`, el sistema SHALL coordinar el despliegue de subagent files según el contrato `sdd-subagent-deployment` durante `ai-specs sync`. Cuando `sub_agents` es `false` o ausente, el sistema MUST NOT desplegar subagent files ni alterar harnesses por motivo de SDD.

#### Scenario: Activación sin destruir manifiesto previo
- **GIVEN** un manifiesto V1 con `[sdd]` ya presente
- **WHEN** el usuario añade `sub_agents = true` y ejecuta sync
- **THEN** el sistema MUST preservar el resto del manifiesto sin pérdidas
- **AND** el manifiesto resultante MUST seguir siendo parseable por `tomllib`

#### Scenario: Sub_agents activado sin proveedor compatible
- **GIVEN** `[sdd].provider` declarado con un valor distinto de `openspec` y `sub_agents = true`
- **WHEN** el comando que coordina el despliegue se ejecuta
- **THEN** el sistema MUST emitir un error explícito o aviso documentado que indique que solo `openspec` soporta el catálogo canónico de subagentes en v1
- **AND** el comando MUST NOT desplegar archivos parciales

### Requirement: Integración con doctor

Cuando `[sdd].enabled = true` y `[sdd].sub_agents = true`, `ai-specs doctor` SHALL comprobar de manera no destructiva que los subagent files esperados existen en cada harness soportado habilitado y emitir severidades `OK`/`WARN`/`ERROR` alineadas al contrato existente de doctor.

#### Scenario: Doctor reporta archivos presentes
- **GIVEN** `sub_agents = true`, `claude` habilitado y los seis archivos en `.claude/agents/sdd-*.md`
- **WHEN** `ai-specs doctor` corre
- **THEN** doctor MUST reportar `OK` para la comprobación de subagentes
- **AND** MUST NOT marcar fallos por motivo de SDD

#### Scenario: Doctor detecta archivos faltantes
- **GIVEN** `sub_agents = true`, `claude` habilitado y falta uno o más subagent files
- **WHEN** `ai-specs doctor` corre
- **THEN** doctor MUST reportar `WARN` o `ERROR` según severidad documentada
- **AND** MUST nombrar el archivo faltante y sugerir `ai-specs sync` como remediación

#### Scenario: Doctor silencioso con sub_agents desactivado
- **GIVEN** `sub_agents = false` o ausente
- **WHEN** `ai-specs doctor` corre
- **THEN** doctor MUST NOT emitir comprobaciones de subagentes
- **AND** el código de salida MUST NO depender de esa comprobación
