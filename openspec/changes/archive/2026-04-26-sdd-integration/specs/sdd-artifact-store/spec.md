# sdd-artifact-store

## ADDED Requirements

### Requirement: Enumeración canónica de modos de almacenamiento

El sistema SHALL aceptar el campo `artifact_store` en `[sdd]` con exactamente uno de los valores: `filesystem`, `hybrid`, `memory`, y MUST rechazar valores desconocidos con error de validación en comandos que mutan o validan el manifiesto.

#### Scenario: Valor inválido rechazado
- **WHEN** `artifact_store` es una cadena no listada en el contrato
- **THEN** el validador MUST fallar con mensaje que enumere los valores permitidos
- **AND** el manifiesto MUST NOT quedar en estado parcialmente escrito por un comando atómico de habilitación

### Requirement: Modo filesystem

Cuando `artifact_store` es `filesystem`, el sistema SHALL tratar el árbol `openspec/` en el root del proyecto como la única fuente de verdad para artefactos OpenSpec gestionados por el proveedor, y MUST NOT declarar dependencia de APIs de memoria de agente para el funcionamiento del CLI.

#### Scenario: Doctor exige openspec en disco
- **WHEN** `artifact_store` es `filesystem` y `[sdd].enabled` es verdadero y `provider` es `openspec`
- **THEN** `ai-specs doctor` MUST emitir `ERROR` si `openspec/` falta o está corrupto según chequeos mínimos definidos en `sdd-cli-integration`
- **AND** el mensaje MUST indicar ejecutar el flujo de inicialización SDD u `openspec init`

### Requirement: Modo hybrid

Cuando `artifact_store` es `hybrid`, el sistema SHALL exigir la misma coherencia en disco que en `filesystem` para `openspec/`, y MAY emitir `WARN` cuando no se detecte integración de memoria operativa (por heurística documentada, p. ej. ausencia de hooks opcionales) sin tratar dicha ausencia como `ERROR` mientras el árbol `openspec/` sea válido.

#### Scenario: Híbrido sin memoria sigue siendo válido en disco
- **WHEN** `artifact_store` es `hybrid` y `openspec/` está completo y válido y no hay integración de memoria
- **THEN** `doctor` MUST NOT fallar con `ERROR` únicamente por la ausencia de memoria
- **AND** `doctor` MAY emitir `WARN` documentado

### Requirement: Modo memory (contrato v1)

Cuando `artifact_store` es `memory`, el sistema SHALL documentar en salida de ayuda y en validación que la persistencia exclusiva en memoria operativa no está soportada de extremo a extremo con OpenSpec en v1, y MUST tratar el modo como **experimental**: el CLI MAY exigir `openspec/` mientras exista desalineación con OpenSpec upstream, emitiendo `WARN` en `doctor` en lugar de `ERROR` salvo que `openspec/` esté totalmente ausente y el usuario haya optado por política estricta documentada.

#### Scenario: Memoria experimental no bloquea CI por defecto
- **WHEN** `artifact_store` es `memory` y `openspec/` no existe pero el usuario ejecuta solo validación no destructiva
- **THEN** el sistema MUST NOT eliminar datos remotos ni memoria ajena
- **AND** el comando de habilitación SHOULD advertir que OpenSpec permanece orientado a archivos

### Requirement: Coherencia con proveedor OpenSpec

Para `provider=openspec`, los tres modos SHALL documentarse respecto a la limitación: OpenSpec CLI opera sobre archivos; por tanto `memory` no puede desactivar `openspec/` sin romper `openspec validate` hasta que exista un adaptador externo. El sistema MUST reflejar esta limitación en `design.md` y en mensajes de usuario.

#### Scenario: Transparencia hacia el usuario
- **WHEN** el usuario selecciona `artifact_store=memory` con `provider=openspec`
- **THEN** el CLI MUST mostrar advertencia de limitación v1 al habilitar o al consultar ayuda del subcomando `sdd`
- **AND** la documentación del cambio MUST enlazar al plan de adaptador de memoria futuro
