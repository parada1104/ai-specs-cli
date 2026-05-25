## ADDED Requirements

### Requirement: Campo mode opcional en [mcp.<name>]

La sección `[mcp.<name>]` de `ai-specs.toml` SHALL soportar un campo opcional `mode` de tipo string con valores permitidos `"shared"` y `"stdio"`. La ausencia del campo SHALL ser equivalente a `mode = "stdio"`.

#### Scenario: Declaración MCP con mode = "shared" en manifest

- **WHEN** `ai-specs.toml` declara `[mcp.trello]` con `mode = "shared"`
- **THEN** la validación del manifest SHALL pasar
- **AND** el MCP SHALL ser tratado como shared durante la materialización y el renderizado

#### Scenario: Declaración MCP sin campo mode no sufre breaking change

- **WHEN** `ai-specs.toml` declara `[mcp.example]` sin campo `mode`
- **THEN** la validación SHALL pasar
- **AND** el comportamiento de sync SHALL ser idéntico al comportamiento previo a este cambio

#### Scenario: mode en manifest prevalece sobre mode en recipe preset

- **WHEN** `ai-specs.toml` declara `[mcp.trello]` con `mode = "stdio"`
- **AND** la recipe declara `[[provides.mcp]]` con `id = "trello"` y `mode = "shared"`
- **THEN** el valor del manifest (`"stdio"`) SHALL prevalecer tras el merge
