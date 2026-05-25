# mcp-mode-shared

## Purpose

Definir la semántica del campo opcional `mode = "shared"` en las declaraciones MCP, tanto en `[[provides.mcp]]` (recipe schema) como en `[mcp.<name>]` (manifest), y garantizar compatibilidad hacia atrás con el comportamiento stdio existente.

## Requirements

### Requirement: Campo mode aceptado en [[provides.mcp]] y [mcp.<name>]

El campo `mode` SHALL ser válido tanto en `[[provides.mcp]]` de `recipe.toml` como en `[mcp.<name>]` de `ai-specs.toml`. El campo SHALL ser opcional en ambos contextos.

#### Scenario: mode = "shared" en recipe preset

- **WHEN** un `recipe.toml` declara `[[provides.mcp]]` con `mode = "shared"`
- **THEN** la validación SHALL pasar
- **AND** el MCP SHALL ser marcado como shared durante la materialización

#### Scenario: mode = "shared" en manifest

- **WHEN** `ai-specs.toml` declara `[mcp.trello]` con `mode = "shared"`
- **THEN** la validación SHALL pasar
- **AND** el MCP SHALL ser marcado como shared durante la materialización

---

### Requirement: Ausencia de mode equivale a stdio (sin breaking change)

La ausencia del campo `mode` SHALL tratarse como `mode = "stdio"`. El comportamiento existente de todos los MCPs declarados sin `mode` SHALL permanecer idéntico al actual.

#### Scenario: MCP sin campo mode usa stdio

- **WHEN** `[mcp.example]` no declara el campo `mode`
- **THEN** el sistema SHALL tratar el MCP como `mode = "stdio"`
- **AND** el comportamiento de renderizado y sync SHALL ser idéntico al comportamiento previo a este cambio

#### Scenario: mode = "stdio" explícito equivale a ausencia

- **WHEN** `[mcp.example]` declara `mode = "stdio"` explícitamente
- **THEN** el comportamiento SHALL ser idéntico al caso de ausencia de `mode`

---

### Requirement: Validación — solo "shared" o "stdio" son valores válidos

El sistema SHALL rechazar cualquier valor de `mode` que no sea `"shared"` o `"stdio"`.

#### Scenario: Valor desconocido rechazado

- **WHEN** una declaración MCP incluye `mode = "proxy"` o cualquier valor fuera del enum
- **THEN** la validación SHALL fallar con un error explícito que indique los valores válidos

#### Scenario: Valores válidos aceptados

- **WHEN** una declaración MCP incluye `mode = "shared"` o `mode = "stdio"` (o ausencia del campo)
- **THEN** la validación SHALL pasar

---

### Requirement: Precedencia del manifest sobre la recipe (integración con mcp-preset-merge)

Si la recipe declara `mode = "shared"` para un MCP pero el manifest declara `mode = "stdio"` (o no declara `mode`) para ese mismo MCP, el valor del manifest SHALL prevalecer. Esta regla es consistente con la semántica de shallow merge definida en `mcp-preset-merge`.

#### Scenario: Manifest stdio anula recipe shared

- **WHEN** `recipe.toml` declara `mode = "shared"` para el MCP `trello`
- **AND** `ai-specs.toml` declara `[mcp.trello]` con `mode = "stdio"`
- **THEN** el MCP merged SHALL tener `mode = "stdio"`

#### Scenario: Manifest sin mode hereda mode de la recipe

- **WHEN** `recipe.toml` declara `mode = "shared"` para el MCP `trello`
- **AND** `ai-specs.toml` declara `[mcp.trello]` sin campo `mode`
- **THEN** el MCP merged SHALL tener `mode = "shared"` (heredado del preset de la recipe)
