# sdd-cli-integration Specification

## Purpose

Definir la integración del subcomando `ai-specs sdd` con el manifiesto y el proveedor OpenSpec como primer backend soportado.

## Requirements

### Requirement: Subcomando SDD en el CLI

El sistema SHALL exponer un subcomando `sdd` desde `bin/ai-specs` con ayuda (`--help`) coherente con los demás subcomandos y documentación mínima en el mensaje de ayuda que referencie `ai-specs.toml` y el proveedor OpenSpec como primer backend soportado.

#### Scenario: Ayuda accesible
- **WHEN** el usuario ejecuta `ai-specs sdd --help`
- **THEN** el sistema MUST imprimir uso, flags soportados en v1 y enlaces o referencias de lectura al README del repositorio del CLI
- **AND** el comando MUST terminar con código de salida cero

### Requirement: Habilitación declarativa en el manifiesto

El sistema SHALL persistir el estado SDD en `ai-specs/ai-specs.toml` bajo la tabla opcional `[sdd]` con al menos los campos: `enabled` (booleano), `provider` (cadena, primer valor soportado `openspec`), y `artifact_store` con valores permitidos `memory`, `hybrid`, `filesystem`.

#### Scenario: Habilitar sin destruir manifiesto previo
- **WHEN** el usuario ejecuta el flujo de habilitación de SDD contra un proyecto con manifiesto V1 válido existente
- **THEN** el sistema MUST fusionar o escribir `[sdd]` sin eliminar secciones preexistentes no relacionadas
- **AND** el manifiesto resultante MUST permanecer parseable por `tomllib`

### Requirement: Verificación del binario OpenSpec

Cuando `provider` es `openspec`, el sistema SHALL comprobar que el ejecutable `openspec` está disponible en `PATH` antes de invocar subcomandos que lo requieran, y MUST emitir un error claro si falta, incluyendo el nombre del paquete npm canónico `@fission-ai/openspec` y el requisito de Node.js documentado por upstream.

#### Scenario: OpenSpec ausente sin instalación automática
- **WHEN** `openspec` no está en `PATH` y el usuario no solicitó instalación explícita del proveedor
- **THEN** el comando MUST fallar con mensaje accionable y código de salida distinto de cero
- **AND** el sistema MUST NOT ejecutar `npm install` sin consentimiento explícito vía flag documentado

### Requirement: Instalación opcional del CLI del proveedor

El sistema SHALL soportar un flag explícito (nombre exacto implementable, p. ej. `--install-provider-cli`) que, cuando `provider` es `openspec`, invoque `npm install -g @fission-ai/openspec@latest` solo si `npm` está disponible; si `npm` falta, MUST fallar con error explícito.

#### Scenario: Instalación explícita solicitada
- **WHEN** el usuario pasa el flag de instalación explícita y `npm` está en `PATH`
- **THEN** el sistema MUST ejecutar la instalación global documentada y revalidar que `openspec` responde a `--version` o comando equivalente
- **AND** si la instalación falla, el comando MUST propagar fallo con salida distinta de cero

### Requirement: Inicialización del layout OpenSpec

Cuando `provider` es `openspec` y el directorio `openspec/` no existe o está incompleto según reglas documentadas en el diseño del cambio, el sistema SHALL ejecutar `openspec init` de forma no interactiva derivando herramientas desde `[agents].enabled` cuando sea posible, y MUST NOT sobrescribir un `openspec/` existente sin flag de fuerza explícito documentado.

#### Scenario: init idempotente con árbol existente
- **WHEN** `openspec/` ya existe y el usuario no pasó flag de fuerza
- **THEN** el sistema MUST omitir `openspec init` destructivo
- **AND** el sistema MAY aplicar solo fusiones seguras de configuración documentadas en tareas de implementación

### Requirement: Sincronización de skills y comandos del catálogo

El sistema SHALL reutilizar el pipeline existente de `refresh-bundled` (o su helper Python) para copiar al workspace del proyecto las skills y comandos OpenSpec empaquetados en el CLI bajo un preset identificable (p. ej. `openspec`), respetando `.ai-specs.lock` y el comportamiento de sidecars `.new` existente.

#### Scenario: Preset OpenSpec materializa artefactos versionables
- **WHEN** el usuario habilita SDD con proveedor OpenSpec en un proyecto ya inicializado con `ai-specs init`
- **THEN** el sistema MUST dejar skills y comandos bajo `ai-specs/skills/` y `ai-specs/commands/` según el preset
- **AND** los archivos MUST ser elegibles para commit por el equipo sin pasos manuales adicionales de copia desde el repositorio del CLI

### Requirement: Integración con doctor

Cuando `[sdd].enabled` es verdadero, `ai-specs doctor` SHALL ejecutar comprobaciones adicionales de solo lectura: presencia de `openspec/`, parseabilidad mínima de `openspec/config.yaml` si existe, y `openspec` en `PATH` para `provider=openspec`, con severidades `OK`/`WARN`/`ERROR` alineadas al contrato de `doctor` existente.

#### Scenario: SDD deshabilitado no añade ruido
- **WHEN** `[sdd]` está ausente o `enabled` es falso
- **THEN** `doctor` MUST NOT emitir comprobaciones SDD como ERROR
- **AND** el código de salida de `doctor` MUST depender solo de las reglas previas a este cambio más las nuevas condiciones cuando SDD está habilitado
