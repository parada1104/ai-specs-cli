## ADDED Requirements

### Requirement: Agentes HTTP emiten url-type para MCPs shared

Para los agentes Claude, Cursor y OpenCode, los MCPs con `mode = "shared"` SHALL renderizarse como entradas de tipo `url` apuntando al daemon local. Los MCPs con `mode = "stdio"` (o sin `mode`) SHALL renderizarse con `command`/`args`/`env` como hasta ahora.

La URL SHALL seguir el formato `http://localhost:{port}/servers/{mcp_id}/mcp`, donde `{port}` se lee de `.ai-specs/run/proxy.port` en el momento del renderizado.

#### Scenario: MCP shared renderizado como url para Claude

- **WHEN** el MCP `trello` tiene `mode = "shared"`
- **AND** `.ai-specs/run/proxy.port` contiene el valor `54321`
- **AND** el agente es `claude`
- **THEN** la entrada en `.mcp.json` SHALL contener `"url": "http://localhost:54321/servers/trello/mcp"`
- **AND** SHALL NOT contener campos `command`, `args`, ni `env`

#### Scenario: MCP shared renderizado como url para Cursor

- **WHEN** el MCP `trello` tiene `mode = "shared"`
- **AND** `.ai-specs/run/proxy.port` contiene el valor `54321`
- **AND** el agente es `cursor`
- **THEN** la entrada en `.cursor/mcp.json` SHALL contener `"url": "http://localhost:54321/servers/trello/mcp"`

#### Scenario: MCP shared renderizado como url para OpenCode

- **WHEN** el MCP `trello` tiene `mode = "shared"`
- **AND** `.ai-specs/run/proxy.port` contiene el valor `54321`
- **AND** el agente es `opencode`
- **THEN** la entrada en `opencode.json` SHALL contener la URL en el formato correspondiente al schema de OpenCode

#### Scenario: MCP stdio renderizado como command/args/env para agentes HTTP

- **WHEN** el MCP `github` tiene `mode = "stdio"` (o carece de campo `mode`)
- **AND** el agente es `claude`, `cursor` o `opencode`
- **THEN** la entrada SHALL contener `command`, `args`, y `env` tal como en el comportamiento actual
- **AND** SHALL NOT contener campo `url`

---

### Requirement: Codex y Gemini usan stdio para todos los MCPs, incluidos los shared

Para los agentes Codex y Gemini, todos los MCPs SHALL renderizarse con `command`/`args`/`env` independientemente del valor de `mode`. Este es un fallback explícito para agentes sin capacidad de conexión HTTP a MCPs.

#### Scenario: MCP shared renderizado como stdio para Codex

- **WHEN** el MCP `trello` tiene `mode = "shared"`
- **AND** el agente es `codex`
- **THEN** la entrada SHALL contener `command`, `args`, y `env` (igual que un MCP stdio)
- **AND** SHALL NOT contener campo `url`

#### Scenario: MCP shared renderizado como stdio para Gemini

- **WHEN** el MCP `trello` tiene `mode = "shared"`
- **AND** el agente es `gemini`
- **THEN** la entrada SHALL contener `command`, `args`, y `env` (igual que un MCP stdio)
- **AND** SHALL NOT contener campo `url`

---

### Requirement: Puerto leído de proxy.port en tiempo de render

El renderizador SHALL leer el puerto del daemon desde `.ai-specs/run/proxy.port` en el momento de ejecutar `mcp-render`, no en tiempo de materialización. Si el archivo no existe durante el renderizado de un MCP shared, el sistema SHALL fallar con un error explícito.

#### Scenario: Puerto leído desde proxy.port al renderizar

- **WHEN** `mcp-render` procesa un MCP con `mode = "shared"`
- **THEN** SHALL leer el valor de puerto desde `.ai-specs/run/proxy.port`
- **AND** SHALL usar ese valor para construir la URL

#### Scenario: Archivo proxy.port ausente causa error

- **WHEN** `mcp-render` procesa un MCP con `mode = "shared"`
- **AND** `.ai-specs/run/proxy.port` no existe
- **THEN** el sistema SHALL fallar con un error explícito indicando que el daemon no ha sido iniciado
