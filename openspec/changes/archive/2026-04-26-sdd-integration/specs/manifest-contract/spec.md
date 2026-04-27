# manifest-contract — delta (sdd-integration)

## MODIFIED Requirements

### Requirement: Superficie canónica mínima del manifiesto

El sistema MUST tratar `ai-specs/ai-specs.toml` del root como fuente de verdad V1 y reconocer `[project]`, `[agents]`, `[[deps]]` y `[mcp.<name>]` como superficie canónica base, y `[sdd]` como superficie canónica **opcional** cuando el proyecto declara integración SDD según el contrato `sdd-cli-integration`.

#### Scenario: Manifest V1 mínimo válido
- **WHEN** un manifiesto contiene cualquier combinación de `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]` y opcionalmente `[sdd]`
- **THEN** cada sección declarada MUST quedar clasificada como requerida u opcional según el contrato manifest-contract vigente
- **AND** la omisión de `[sdd]` MUST NOT invalidar el manifiesto por sí sola

#### Scenario: Campo mínimo por sección
- **WHEN** se evalúan entradas `[[deps]]` y bloques `[mcp.<name>]`
- **THEN** `deps.id` y `deps.source` MUST ser los mínimos requeridos para cada entrada `[[deps]]`
- **AND** los campos canónicos MCP MUST seguir alineados con el contrato de alias (`env` canónico, `environment` tolerado)

#### Scenario: Sección SDD declarada
- **WHEN** `[sdd]` está presente en el manifiesto
- **THEN** los campos declarados MUST cumplir el contrato `sdd-cli-integration` para valores, enums y dependencias de proveedor
- **AND** los campos no reconocidos dentro de `[sdd]` MUST ser rechazados por el validador o MUST quedar explícitamente listados como tolerados en documentación y tests sin ambigüedad silenciosa

## ADDED Requirements

### Requirement: Campos mínimos de la tabla SDD

El sistema SHALL reconocer en `[sdd]` al menos: `enabled` (booleano), `provider` (cadena; en v1 el valor soportado MUST incluir `openspec`), y `artifact_store` (cadena con valores `filesystem`, `hybrid`, `memory`). El sistema MAY aceptar claves anidadas `openspec.*` o tabla `[sdd.openspec]` para opciones del proveedor siempre que estén documentadas en `sdd-cli-integration`.

#### Scenario: Defaults cuando la tabla existe parcialmente
- **WHEN** `[sdd]` existe con solo `enabled = true`
- **THEN** los comandos que completan el manifiesto MUST exigir o inferir `provider` y `artifact_store` según reglas documentadas sin escribir valores ilegales
- **AND** si faltan campos obligatorios sin regla de inferencia, el comando MUST fallar antes de mutar otros datos

### Requirement: Validación del slice SDD en el runtime

El sistema MUST aplicar validación estructural de `[sdd]` únicamente cuando la tabla está presente y cuando el comando en ejecución declara soporte SDD (`sdd`, `doctor`, o `sync` si se extiende validación). Los proyectos sin `[sdd]` MUST permanecer alineados con el comportamiento previo a este cambio.

#### Scenario: Sin SDD no se validan enums de artifact_store
- **WHEN** el manifiesto no contiene `[sdd]`
- **THEN** el parser MUST NOT exigir `artifact_store` ni `provider`
- **AND** `sync` MUST completar con éxito si ya era válido antes de este cambio

#### Scenario: Con SDD se validan enums
- **WHEN** `[sdd].artifact_store` tiene un valor no permitido
- **THEN** el validador MUST fallar con mensaje explícito
- **AND** el mensaje MUST listar valores permitidos del contrato `sdd-artifact-store`
