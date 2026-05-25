# mcp-named-config-materialization

## Purpose

Definir cómo se construye y escribe el archivo `proxy.named-config.json` que consume `mcp-proxy`, a partir de los MCPs con `mode = "shared"` resultantes del merge recipe ↔ manifest.

## Requirements

### Requirement: MCPs shared se recopilan en named-server-config.json

Tras el merge recipe ↔ manifest, el sistema SHALL recopilar todos los MCPs cuyo `mode` resuelto sea `"shared"` y escribir su configuración en `.ai-specs/run/proxy.named-config.json`.

#### Scenario: MCPs shared escritos al archivo de config

- **WHEN** el merge produce al menos un MCP con `mode = "shared"`
- **THEN** el sistema SHALL escribir `.ai-specs/run/proxy.named-config.json`
- **AND** el archivo SHALL contener todos los MCPs shared y únicamente ellos

#### Scenario: Sin MCPs shared no se escribe config

- **WHEN** el merge produce cero MCPs con `mode = "shared"`
- **THEN** el sistema SHALL NOT escribir `.ai-specs/run/proxy.named-config.json`
- **AND** el sistema SHALL NOT iniciar el daemon

---

### Requirement: Formato compatible con el schema de mcp-proxy

El archivo `proxy.named-config.json` SHALL seguir el schema que espera `mcp-proxy`:

```json
{
  "mcpServers": {
    "<name>": {
      "command": "<string>",
      "args": ["<string>", ...],
      "env": { "<KEY>": "<value>", ... }
    }
  }
}
```

Cada clave de `mcpServers` SHALL ser el ID del MCP (`<name>`). Los campos `command`, `args`, y `env` SHALL provenir de la configuración merged del MCP.

#### Scenario: Formato correcto para MCP shared

- **WHEN** el MCP `trello` tiene `mode = "shared"`, `command = "uvx"`, `args = ["mcp-server-trello"]`, y `env = {"TRELLO_TOKEN": "$TRELLO_TOKEN"}`
- **THEN** el archivo SHALL contener `{"mcpServers": {"trello": {"command": "uvx", "args": ["mcp-server-trello"], "env": {"TRELLO_TOKEN": "$TRELLO_TOKEN"}}}}`

#### Scenario: Múltiples MCPs shared en el mismo archivo

- **WHEN** el merge produce los MCPs `trello` y `github` ambos con `mode = "shared"`
- **THEN** ambos SHALL aparecer como entradas bajo `mcpServers` en el mismo archivo

---

### Requirement: Referencias de env vars preservadas en la config del daemon

Las referencias a variables de entorno en la forma `$VAR` o `${VAR}` SHALL preservarse literalmente en `proxy.named-config.json`. La resolución de variables SHALL ocurrir al momento de spawn del daemon, no durante la materialización.

#### Scenario: Referencia $VAR preservada sin expansión

- **WHEN** un MCP shared declara `env = { TRELLO_TOKEN = "$TRELLO_TOKEN" }`
- **THEN** `proxy.named-config.json` SHALL contener `"TRELLO_TOKEN": "$TRELLO_TOKEN"` literalmente
- **AND** el valor SHALL NOT ser expandido durante la escritura del archivo

#### Scenario: Referencia ${VAR} preservada sin expansión

- **WHEN** un MCP shared declara `env = { API_KEY = "${SOME_API_KEY}" }`
- **THEN** `proxy.named-config.json` SHALL contener `"API_KEY": "${SOME_API_KEY}"` literalmente

---

### Requirement: Cambios en la config disparan restart del daemon

Cuando el archivo `proxy.named-config.json` cambia y el daemon está corriendo, el sistema SHALL ejecutar un restart controlado (SIGTERM al proceso actual + spawn de un proceso nuevo con la config actualizada). El sistema SHALL NOT depender de señales de reload (mcp-proxy no documenta SIGHUP reload). La detección del cambio SHALL basarse en el hash SHA-256 del JSON canónico antes y después de escribir.

#### Scenario: Config cambiada en sync subsiguiente dispara restart controlado

- **WHEN** un segundo `ai-specs sync` produce un `proxy.named-config.json` cuyo SHA-256 difiere del anterior
- **AND** el daemon está corriendo y sano
- **THEN** el sistema SHALL enviar SIGTERM al proceso actual
- **AND** SHALL esperar a que el proceso termine
- **AND** SHALL spawnear un proceso nuevo con la config actualizada
- **AND** el daemon SHALL servir la configuración actualizada al completar el restart

#### Scenario: Config sin cambios no dispara restart

- **WHEN** un segundo `ai-specs sync` produce un `proxy.named-config.json` byte-idéntico al anterior (mismo SHA-256)
- **THEN** el sistema SHALL NOT enviar SIGTERM al daemon
- **AND** el PID del daemon SHALL permanecer invariante