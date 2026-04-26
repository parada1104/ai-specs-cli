# manifest-contract Specification

## Purpose

Definir el contrato V1 canónico y validable de `ai-specs/ai-specs.toml` sin exigir más de lo que el runtime actual soporta.

## Requirements

### Requirement: Superficie canónica mínima del manifiesto

El sistema MUST tratar `ai-specs/ai-specs.toml` del root como fuente de verdad V1 y reconocer `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]` y opcionalmente `[recipes.<id>]` como superficie canónica base, y `[sdd]` como superficie canónica **opcional** cuando el proyecto declara integración SDD según el contrato `sdd-cli-integration`.

#### Scenario: Manifest V1 mínimo válido
- GIVEN un manifiesto con `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]` y opcionalmente `[sdd]` presentes u omitidos
- WHEN el contrato V1 se documenta y valida
- THEN cada sección y campo MUST quedar clasificado como requerido u opcional según el contrato manifest-contract vigente
- AND la omisión de `[sdd]` MUST NOT invalidar el manifiesto por sí sola

#### Scenario: Campo mínimo por sección
- GIVEN `project.name`, `agents.enabled`, `deps.id`, `deps.source`, `mcp.<name>.command`, `mcp.<name>.args`, `mcp.<name>.env`, `mcp.<name>.timeout`, y `recipes.<id>.enabled` / `recipes.<id>.version`
- WHEN se publica el contrato V1
- THEN `deps.id` y `deps.source` MUST quedar como mínimos requeridos para `[[deps]]`
- AND los campos canónicos MCP MUST seguir alineados con el contrato de alias (`env` canónico, `environment` tolerado)
- AND `recipes.<id>.enabled` y `recipes.<id>.version` MUST quedar como mínimos requeridos para `[recipes.<id>]`

#### Scenario: Sección recipes declarada
- GIVEN `[recipes.my-recipe]` está presente en el manifiesto
- WHEN se valida el manifiesto
- THEN `enabled` y `version` MUST estar presentes
- AND la omisión de `[recipes.*]` MUST NOT invalidar el manifiesto

#### Scenario: Campo faltante en recipe
- GIVEN `[recipes.my-recipe]` omite `version`
- WHEN se valida el manifiesto
- THEN la validación MUST fallar con mensaje de error explícito

#### Scenario: Sección SDD declarada
- GIVEN `[sdd]` está presente en el manifiesto
- WHEN se valida el manifiesto
- THEN los campos declarados MUST cumplir el contrato `sdd-cli-integration` para valores, enums y dependencias de proveedor
- AND los campos no reconocidos dentro de `[sdd]` MUST ser rechazados por el validador o MUST quedar explícitamente listados como tolerados en documentación y tests sin ambigüedad silenciosa

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

El sistema MUST validar solo lo que hoy ya tiene semántica operativa explícita y MUST dejar fuera de este cambio precedence y reglas no consumidas por el runtime. La validación estructural de `[sdd]` MUST aplicarse solo cuando la tabla está presente y el comando declara soporte SDD (`sdd`, `doctor`, o `sync` si se extiende validación).

#### Scenario: Validación de subrepos
- GIVEN `project.subrepos` declarado
- WHEN se aplica validación V1
- THEN el contrato MUST exigir coherencia con la resolución actual de targets del root
- AND la semántica de paths inválidos MUST permanecer alineada con la validación existente

#### Scenario: Sin SDD no se validan enums de artifact_store
- GIVEN el manifiesto no contiene `[sdd]`
- WHEN se ejecutan comandos que no mutan el slice SDD
- THEN el parser MUST NOT exigir `artifact_store` ni `provider` por ausencia de `[sdd]`
- AND `sync` MUST completar con éxito si ya era válido antes de introducir SDD

#### Scenario: Campo fuera de alcance
- GIVEN una regla propuesta sobre precedence no cubierta por el runtime V1
- WHEN se redacta el contrato V1
- THEN esa regla MUST marcarse como diferida
- AND MUST NOT convertirse en requisito validable de este cambio

### Requirement: Campos mínimos de la tabla recipes

El sistema SHALL reconocer en `[recipes.<id>]` al menos: `enabled` (booleano, requerido) y `version` (cadena exacta, requerida). La ausencia de toda sección `[recipes.*]` MUST NOT invalidar el manifiesto.

#### Scenario: Recipes mínimo válido
- GIVEN `[recipes.my-recipe]` declara `enabled = true` y `version = "1.0.0"`
- WHEN se valida el manifiesto
- THEN la validación MUST pasar

#### Scenario: Campo faltante en recipe
- GIVEN `[recipes.my-recipe]` omite `version`
- WHEN se valida el manifiesto
- THEN la validación MUST fallar con mensaje explícito

### Requirement: Campos mínimos de la tabla SDD

El sistema SHALL reconocer en `[sdd]` al menos: `enabled` (booleano), `provider` (cadena; en v1 el valor soportado MUST incluir `openspec`), y `artifact_store` (cadena con valores `filesystem`, `hybrid`, `memory`). El sistema MAY aceptar claves anidadas `openspec.*` o tabla `[sdd.openspec]` para opciones del proveedor siempre que estén documentadas en `sdd-cli-integration`.

#### Scenario: Defaults cuando la tabla existe parcialmente
- GIVEN `[sdd]` existe con solo `enabled = true`
- WHEN un comando completa el manifiesto
- THEN el comando MUST exigir o inferir `provider` y `artifact_store` según reglas documentadas sin escribir valores ilegales
- AND si faltan campos obligatorios sin regla de inferencia, el comando MUST fallar antes de mutar otros datos

#### Scenario: Con SDD se validan enums
- GIVEN `[sdd].artifact_store` tiene un valor no permitido
- WHEN el validador evalúa el manifiesto
- THEN el validador MUST fallar con mensaje explícito
- AND el mensaje MUST listar valores permitidos del contrato `sdd-artifact-store`

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
