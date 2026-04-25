# manifest-contract Specification

## Purpose

Definir el contrato V1 canónico y validable de `ai-specs/ai-specs.toml` sin exigir más de lo que el runtime actual soporta.

## Requirements

### Requirement: Superficie canónica mínima del manifiesto

El sistema MUST tratar `ai-specs/ai-specs.toml` del root como fuente de verdad V1 y reconocer solo `[project]`, `[agents]`, `[[deps]]` y `[mcp.<name>]` como superficie canónica de este cambio.

#### Scenario: Manifest V1 mínimo válido
- GIVEN un manifiesto con `[project]`, `[agents]`, `[[deps]]` o `[mcp.<name>]` presentes u omitidos
- WHEN el contrato V1 se documenta y valida
- THEN cada sección y campo MUST quedar clasificado como requerido, opcional, defaulted o alias tolerado
- AND no se MUST introducir secciones nuevas dentro de este cambio

#### Scenario: Campo mínimo por sección
- GIVEN `project.name`, `agents.enabled`, `deps.id`, `deps.source`, `mcp.<name>.command`, `mcp.<name>.args`, `mcp.<name>.env` y `mcp.<name>.timeout`
- WHEN se publica el contrato V1
- THEN `deps.id` y `deps.source` MUST quedar como mínimos requeridos para `[[deps]]`
- AND el resto MUST quedar explícitamente como opcional, defaulted o canónico por sección según el soporte real actual

### Requirement: Compatibilidad conservadora hacia atrás

El sistema MUST preservar manifests que hoy funcionan por lectores tolerantes. La validación SHALL endurecer solo lo necesario para expresar el contrato V1 actual.

#### Scenario: Secciones faltantes
- GIVEN un manifiesto sin `[agents]`, `[[deps]]` o `[mcp]`
- WHEN se evalúa contra el contrato V1
- THEN el manifiesto MUST seguir siendo válido
- AND la lectura MUST resolver defaults equivalentes al comportamiento actual

#### Scenario: Alias MCP tolerado
- GIVEN un servidor MCP que usa `environment` en lugar de `env`
- WHEN el contrato V1 define compatibilidad
- THEN `env` MUST quedar como campo canónico
- AND `environment` MUST quedar documentado como alias tolerado sin cambiar el significado soportado hoy

### Requirement: Límite explícito de validación V1

El sistema MUST validar solo lo que hoy ya tiene semántica operativa explícita y MUST dejar fuera de este cambio precedence, `doctor`, `[memory]` y reglas no consumidas por el runtime.

#### Scenario: Validación de subrepos
- GIVEN `project.subrepos` declarado
- WHEN se aplica validación V1
- THEN el contrato MUST exigir coherencia con la resolución actual de targets del root
- AND la semántica de paths inválidos MUST permanecer alineada con la validación existente

#### Scenario: Campo fuera de alcance
- GIVEN una regla propuesta sobre precedence o UX de `doctor`
- WHEN se redacta el contrato V1
- THEN esa regla MUST marcarse como diferida
- AND MUST NOT convertirse en requisito validable de este cambio

### Requirement: Alineación entre contrato, plantilla y README

El sistema MUST dejar una sola definición V1 consistente entre plantilla, README y límites de compatibilidad del runtime.

#### Scenario: Plantilla consistente
- GIVEN `templates/ai-specs.toml.tmpl`
- WHEN se actualiza para este cambio
- THEN sus comentarios MUST reflejar la clasificación canónica de campos y aliases tolerados
- AND MUST NOT prometer comportamiento no soportado hoy

#### Scenario: README consistente
- GIVEN la documentación pública del manifiesto
- WHEN se publica el contrato V1
- THEN el README MUST describir ownership, secciones soportadas y límites de compatibilidad V1
- AND MUST dejar claro que el manifiesto del root es la única fuente de verdad en V1
