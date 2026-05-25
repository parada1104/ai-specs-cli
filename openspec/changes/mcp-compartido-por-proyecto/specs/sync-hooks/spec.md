## ADDED Requirements

### Requirement: Paso "ensure mcp-proxy daemon" insertado antes del fan-out

El pipeline `on-sync` SHALL incluir un nuevo paso "ensure mcp-proxy daemon" que se ejecuta después de la materialización de recipes y antes de cualquier paso `mcp-render` (fan-out por agente). El paso SHALL invocarse únicamente cuando la materialización haya detectado al menos un MCP con `mode = "shared"`.

#### Scenario: Paso daemon insertado cuando existen MCPs shared

- **WHEN** la materialización de recipes produce al menos un MCP con `mode = "shared"`
- **THEN** `sync.sh` SHALL ejecutar el paso "ensure mcp-proxy daemon" antes de iniciar el fan-out de configs por agente
- **AND** el daemon SHALL estar disponible antes de que cualquier agente reciba una config con `url`

#### Scenario: Paso daemon omitido cuando no hay MCPs shared

- **WHEN** la materialización de recipes produce cero MCPs con `mode = "shared"`
- **THEN** `sync.sh` SHALL omitir el paso "ensure mcp-proxy daemon"
- **AND** el pipeline SHALL continuar directamente al fan-out sin intentar iniciar ni verificar el daemon

#### Scenario: Fallo del paso daemon detiene el sync

- **WHEN** el paso "ensure mcp-proxy daemon" falla (por ejemplo, `uvx` ausente o puerto no asignable)
- **THEN** `sync.sh` SHALL detenerse con un error explícito
- **AND** el fan-out de configs por agente SHALL NOT ejecutarse
