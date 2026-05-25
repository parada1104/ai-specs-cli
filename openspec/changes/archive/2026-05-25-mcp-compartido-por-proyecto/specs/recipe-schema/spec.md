## ADDED Requirements

### Requirement: Campo mode opcional en [[provides.mcp]]

El bloque `[[provides.mcp]]` SHALL soportar un campo opcional `mode` de tipo string con valores permitidos `"shared"` y `"stdio"`. La ausencia del campo SHALL ser equivalente a `mode = "stdio"`.

#### Scenario: Preset MCP con mode = "shared" válido

- **WHEN** `recipe.toml` declara `[[provides.mcp]]` con `id = "trello"` y `mode = "shared"`
- **THEN** la validación del recipe SHALL pasar
- **AND** el preset SHALL ser marcado como shared durante la materialización

#### Scenario: Preset MCP sin campo mode mantiene comportamiento stdio

- **WHEN** `recipe.toml` declara `[[provides.mcp]]` sin campo `mode`
- **THEN** la validación SHALL pasar
- **AND** el comportamiento de sync SHALL ser idéntico al comportamiento previo a este cambio

#### Scenario: Valor de mode desconocido rechazado

- **WHEN** `recipe.toml` declara `[[provides.mcp]]` con `mode = "proxy"` u otro valor fuera del enum
- **THEN** la validación SHALL fallar con un error explícito que liste los valores válidos (`"shared"`, `"stdio"`)
