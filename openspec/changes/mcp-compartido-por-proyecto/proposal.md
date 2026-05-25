## Why

Hoy cada agente (Claude, Cursor, OpenCode, Codex, Gemini) levanta su propio proceso stdio por cada MCP declarado, y cada worktree del repo duplica todo eso de nuevo: con N agentes y M worktrees terminamos con N×M procesos del mismo servidor MCP corriendo en paralelo, cada uno con su propia caché de tokens, su propia conexión saliente y su propia ventana de timeout. Para un MCP "caro" como `@delorenj/mcp-server-trello` esto se nota: arranque lento, rate limits compartidos contra Trello, memoria desperdiciada. La salida natural es multiplexar: **un único daemon `mcp-proxy` por repo git** que aloja todos los MCPs marcados como `shared` y expone cada uno como Streamable HTTP en `http://localhost:PORT/servers/<name>/mcp`. Los agentes capaces de hablar HTTP (Claude Code, Cursor, OpenCode) reciben una `url`; el resto sigue por stdio sin cambios.

## What Changes

- Añadir campo opcional `mode = "shared"` en `[[provides.mcp]]` (recipe schema) y en `[mcp.<name>]` (manifest schema); ausencia del campo = stdio actual (sin breaking change).
- `recipe-materialize.py` separa MCPs `shared` de los `stdio`: para los `shared` escribe entrada en el `named-server-config.json` del daemon; los `stdio` siguen el flujo actual.
- Nuevo módulo `lib/_internal/mcp-daemon.py` con la gestión de ciclo de vida: `ensure_daemon()`, `healthcheck()`, `reload_config()`, `stop()` — invoca `uvx mcp-proxy --named-server-config ...` en proceso detached.
- `lib/sync.sh` agrega paso "ensure mcp-proxy daemon" antes del fan-out de configs por agente, sólo si la materialización detectó al menos un MCP `shared`.
- `lib/_internal/mcp-render.py`: traductores por agente emiten entradas `url`-type (`http://localhost:PORT/servers/<name>/mcp`) para MCPs `shared` en Claude/Cursor/OpenCode; Codex y Gemini reciben la entrada stdio tradicional como fallback explícito.
- Asignación dinámica de puerto: primera `sync` busca puerto libre alto, lo escribe a `.ai-specs/run/proxy.port`; syncs siguientes leen el archivo, hacen healthcheck, y rearrancan con nuevo puerto libre si el daemon murió.
- Identidad del daemon por raíz git: `git rev-parse --show-toplevel` define el directorio canónico; worktrees del mismo repo comparten un solo daemon.
- Estado runtime en `.ai-specs/run/proxy.{pid,port,named-config.json}` (no commiteado).
- Variables de entorno (secrets) se resuelven al spawn del daemon desde el shell del usuario y viven sólo dentro del `named-server-config.json`; los agentes nunca ven secrets, sólo una URL de localhost.
- Actualizar `catalog/recipes/trello-mcp-workflow/recipe.toml` para declarar `mode = "shared"` en el preset de `trello`.
- Nuevo subcomando `ai-specs daemon stop` para apagar el daemon manualmente; cleanup automático queda fuera de scope (orphan cleanup en logout es un problema separado).
- Tests de integración: levantar un `mcp-proxy` real contra un servidor stdio falso, validar que los traductores emiten `url` para `shared` y stdio para el resto, y que una segunda `sync` reusa el daemon sin recrear.

## Capabilities

### New Capabilities

- `mcp-shared-daemon`: orquestación del daemon `mcp-proxy` por raíz git — `ensure_daemon`, healthcheck, asignación dinámica de puerto, `stop`, identidad por `git rev-parse --show-toplevel`, estado en `.ai-specs/run/proxy.{pid,port}`.
- `mcp-mode-shared`: semántica del opt-in `mode = "shared"` en `[[provides.mcp]]` (recipe) y `[mcp.<name>]` (manifest); default = stdio; campo aditivo y compatible hacia atrás.
- `mcp-named-config-materialization`: materialización del `named-server-config.json` consumido por `mcp-proxy` a partir de los MCPs marcados como `shared` tras el merge recipe ↔ manifest.

### Modified Capabilities

- `recipe-schema`: el spec de `[[provides.mcp]]` debe documentar el nuevo campo opcional `mode`.
- `recipe-manifest-contract`: el spec de `[mcp.<name>]` en `ai-specs.toml` debe documentar el nuevo campo opcional `mode`.
- `mcp-preset-merge`: el merge debe preservar `mode` como cualquier otra clave, con la misma precedencia manifest > recipe.
- `mcp-env-rendering`: los traductores por agente deben distinguir MCPs `shared` (emitir `url`) de MCPs `stdio` (mantener `command`/`args`/`env` actual); Codex y Gemini permanecen stdio incluso si la entrada es `shared` (fallback explícito).
- `sync-hooks`: el paso "ensure mcp-proxy daemon" se inserta en el pipeline `on-sync` antes del fan-out por agente.

## Impact

- **Modificados**: `lib/_internal/recipe-materialize.py` (split shared/stdio + escritura de named-config), `lib/_internal/mcp-render.py` (traductores emiten `url` para shared), `lib/sync.sh` (ensure-daemon step + subcomando `daemon stop`), `catalog/recipes/trello-mcp-workflow/recipe.toml` (`mode = "shared"` en `[[provides.mcp]]`), schema validators de recipe.toml y ai-specs.toml, `.gitignore` (añadir `.ai-specs/run/`).
- **Nuevos**: `lib/_internal/mcp-daemon.py` (lifecycle del daemon vía `uvx mcp-proxy`), specs de las tres capabilities nuevas, tests de integración del daemon + fan-out.
- **Runtime dep nueva**: `mcp-proxy` invocado vía `uvx` (no requiere install global; uvx cachea la primera ejecución).
- **Sin breaking change**: MCPs sin `mode = "shared"` se comportan idénticos a hoy. Agentes sin capability HTTP (Codex, Gemini) siguen stdio aunque el MCP sea `shared`.
- **Resource win**: 1 daemon por raíz git con N subprocess internos cacheados, en vez de N×M procesos stdio por agente×worktree.
- **Risks**: dependencia de `uvx` en `PATH` del usuario (mitigación: doctor check); si el daemon muere todos los MCPs `shared` quedan inaccesibles hasta el próximo `sync`; orphan daemon si el usuario cierra terminales sin invocar `ai-specs daemon stop` (mitigación: cleanup en próxima sync detecta pid muerto y rearranca).
