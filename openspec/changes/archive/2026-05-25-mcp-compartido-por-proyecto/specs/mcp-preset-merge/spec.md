## ADDED Requirements

### Requirement: El campo mode se mergea con precedencia manifest

El campo `mode` SHALL participar en el shallow merge de MCP presets con la misma semántica de precedencia que el resto de campos: el valor del manifest prevalece sobre el valor del preset de la recipe; si el manifest no declara `mode`, se hereda el valor del preset.

#### Scenario: mode del manifest prevalece sobre mode del preset

- **WHEN** la recipe proporciona un preset MCP con `mode = "shared"`
- **AND** el manifest declara el mismo MCP con `mode = "stdio"`
- **THEN** el MCP merged SHALL tener `mode = "stdio"`
- **AND** se SHALL emitir un warning por la clave conflictiva (consistente con el comportamiento de merge existente)

#### Scenario: mode del preset heredado cuando el manifest no lo declara

- **WHEN** la recipe proporciona un preset MCP con `mode = "shared"`
- **AND** el manifest declara el mismo MCP sin campo `mode`
- **THEN** el MCP merged SHALL tener `mode = "shared"` (heredado del preset)

#### Scenario: Merge de MCP nuevo desde recipe preserva mode

- **WHEN** la recipe proporciona un preset MCP con `mode = "shared"` cuyo ID no existe en el manifest
- **THEN** el MCP merged SHALL incluir `mode = "shared"` sin modificación
