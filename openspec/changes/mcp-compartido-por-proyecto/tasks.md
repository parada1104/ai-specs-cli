## Group 1: Schema extension (campo mode)

### 1.1 recipe_schema.py — validación de mode en [[provides.mcp]]

- [x] [red] Escribir test: recipe con `mode = "shared"` en `[[provides.mcp]]` pasa validación (`tests/unit/test_recipe_schema_mode.py`)
- [x] [red] Escribir test: recipe con `mode = "stdio"` explícito en `[[provides.mcp]]` pasa validación
- [x] [red] Escribir test: recipe sin campo `mode` en `[[provides.mcp]]` pasa validación (sin breaking change)
- [x] [red] Escribir test: recipe con `mode = "proxy"` (valor fuera del enum) falla con `RecipeValidationError` mencionando los valores válidos
- [x] [green] Implementar: aceptar campo opcional `mode` con enum `["shared", "stdio"]` en `lib/_internal/recipe_schema.py`
- [x] [refactor] Revisar mensajes de error del validator para coherencia con el resto del schema

### 1.2 toml-read.py — validación de mode en [mcp.<name>]

- [x] [red] Escribir test: manifest con `mode = "shared"` en `[mcp.trello]` pasa validación (`tests/unit/test_toml_read_mode.py`)
- [x] [red] Escribir test: manifest con `mode = "stdio"` explícito en `[mcp.trello]` pasa validación
- [x] [red] Escribir test: manifest sin campo `mode` en `[mcp.example]` pasa validación (sin breaking change)
- [x] [red] Escribir test: manifest con `mode = "foo"` falla con error indicando valores válidos
- [x] [green] Implementar: preservar y validar campo `mode` en `read_mcp()` en `lib/_internal/toml-read.py`

### 1.3 mcp-preset-merge — preservación de mode en el merge

- [x] [red] Escribir test: recipe `mode = "shared"` + manifest `mode = "stdio"` → merged tiene `mode = "stdio"` (manifest gana) (`tests/unit/test_mcp_preset_merge_mode.py`)
- [x] [red] Escribir test: recipe `mode = "shared"` + manifest sin `mode` → merged tiene `mode = "shared"` (heredado del preset)
- [x] [red] Escribir test: recipe `mode = "shared"` + MCP ID no existe en manifest → merged incluye `mode = "shared"` sin modificación
- [x] [red] Escribir test: conflicto de `mode` emite warning consistente con el comportamiento de merge existente
- [x] [green] Verificar que el shallow merge existente en `lib/_internal/recipe-materialize.py` ya preserva `mode` como cualquier otra clave (puede no requerir cambios si el merge es genérico)
- [x] [refactor] Extraer constante `VALID_MCP_MODES = ("shared", "stdio")` si queda duplicada entre schema y toml-read

---

## Group 2: Materialización — split shared/stdio y named-config

### 2.1 split_mcps_by_mode

- [x] [red] Escribir test: input mixto `{"trello": {..., "mode": "shared"}, "github": {...}}` → `split_mcps_by_mode` retorna `({"trello": ...}, {"github": ...})` (`tests/unit/test_recipe_materialize_split.py`)
- [x] [red] Escribir test: input vacío → `({}, {})`
- [x] [red] Escribir test: solo stdio → `({}, {...})`
- [x] [red] Escribir test: solo shared → `({...}, {})`
- [x] [red] Escribir test: MCP sin campo `mode` cae en el grupo stdio
- [x] [green] Implementar `split_mcps_by_mode(merged_mcp: dict) -> tuple[dict, dict]` en `lib/_internal/recipe-materialize.py`

### 2.2 write_named_server_config

- [x] [red] Escribir test: output JSON tiene shape `{"mcpServers": {...}}` (`tests/unit/test_recipe_materialize_named_config.py`)
- [x] [red] Escribir test: la key `mode` NO aparece en ningún server del output (mcp-proxy no la reconoce)
- [x] [red] Escribir test: referencias `$VAR` y `${VAR}` en `env` se preservan literalmente (sin expansión)
- [x] [red] Escribir test: múltiples MCPs shared aparecen como entradas separadas bajo `mcpServers`
- [x] [red] Escribir test: el archivo se escribe con permisos `0o600`
- [x] [green] Implementar `write_named_server_config(shared: dict, output_path: Path) -> None` en `lib/_internal/recipe-materialize.py`

### 2.3 Integración en materialize_recipes()

- [x] [red] Escribir test: `materialize_recipes()` con MCP shared crea `.ai-specs/run/proxy.named-config.json` (`tests/integration/test_materialize_shared_mcp.sh`)
- [x] [red] Escribir test: `materialize_recipes()` sin MCPs shared NO crea `.ai-specs/run/proxy.named-config.json`
- [x] [red] Escribir test: hash SHA-256 del named-config cambia entre syncs si el MCP cambia (base para detección de cambio en Group 3)
- [x] [green] Modificar `materialize_recipes()` en `lib/_internal/recipe-materialize.py`: llamar `split_mcps_by_mode`, si `shared` no vacío llamar `write_named_server_config`, obtener `git_root` vía `subprocess(['git', 'rev-parse', '--show-toplevel'])`
- [x] [refactor] Asegurar que `recipe_mcp_out` temp sigue incluyendo `mode` para el downstream (`mcp-render.py`)

---

## Group 3: Daemon module (lib/_internal/mcp-daemon.py)

### 3.1 Utilidades internas

- [x] [red] Escribir test: `_pick_free_port()` retorna un int en `(1024, 65536)` (`tests/unit/test_mcp_daemon_pick_free_port.py`)
- [x] [red] Escribir test: dos invocaciones consecutivas de `_pick_free_port()` retornan valores distintos con alta probabilidad
- [x] [green] Implementar `_pick_free_port()` usando `socket.socket().bind(('', 0))`
- [x] [red] Escribir test: `_state_dir(git_root)` retorna `<git_root>/.ai-specs/run/` y crea el directorio si no existe (`tests/unit/test_mcp_daemon_state_dir.py`)
- [x] [green] Implementar `_state_dir(git_root: Path) -> Path`
- [x] [red] Escribir test: `_is_pid_alive(pid)` retorna `False` para PID inexistente, `True` para PID propio (`tests/unit/test_mcp_daemon_pid_alive.py`)
- [x] [green] Implementar `_is_pid_alive(pid: int) -> bool` con `os.kill(pid, 0)` envuelto en try/except
- [x] [red] Escribir test: `_hash_config(path)` retorna strings distintos para contenidos distintos, idénticos para contenidos iguales (`tests/unit/test_mcp_daemon_hash_config.py`)
- [x] [green] Implementar `_hash_config(path: Path) -> str` con SHA-256 del JSON canónico

### 3.2 healthcheck

- [x] [red] Escribir test: servidor HTTP fake en hilo que responde 200 → `healthcheck(port) == True` (`tests/unit/test_mcp_daemon_healthcheck.py`)
- [x] [red] Escribir test: puerto sin nada escuchando → `healthcheck(port) == False` (timeout o connection refused)
- [x] [red] Escribir test: servidor que responde 500 → `healthcheck(port) == False`
- [x] [green] Implementar `healthcheck(port: int, timeout: float = 2.0) -> bool` con `urllib.request.urlopen`

### 3.3 ensure_daemon — path de arranque nuevo

- [x] [red] Escribir test: sin state files → `ensure_daemon` spawna proceso y escribe `proxy.pid` y `proxy.port` (`tests/unit/test_mcp_daemon_ensure.py`)
- [x] [green] Implementar path de spawn en `ensure_daemon(git_root, named_config_path)`: `subprocess.Popen` con `start_new_session=True`, stdout/stderr a `proxy.log`, escribir state files al final

### 3.4 ensure_daemon — idempotencia

- [x] [red] Escribir test: daemon sano (mock healthcheck=True + PID vivo) → `ensure_daemon` retorna el puerto existente sin spawnear nuevo proceso
- [x] [green] Implementar rama idempotente: leer `proxy.pid` + `proxy.port`, invocar `_is_pid_alive` + `healthcheck`; si ambos OK, retornar puerto

### 3.5 ensure_daemon — restart por PID muerto

- [x] [red] Escribir test: `proxy.pid` existe pero PID muerto → `ensure_daemon` limpia state files, asigna nuevo puerto, spawna nuevo proceso
- [x] [green] Implementar rama de PID muerto: remover state files stale, continuar con spawn

### 3.6 ensure_daemon — restart por puerto no responde

- [x] [red] Escribir test: PID vivo pero `healthcheck == False` → `ensure_daemon` envía SIGTERM, asigna nuevo puerto, spawna proceso nuevo
- [x] [green] Implementar rama de puerto stale: `os.kill(pid, signal.SIGTERM)`, esperar exit, continuar con spawn

### 3.7 ensure_daemon — detección de cambio de config

- [x] [red] Escribir test: hash del named-config difiere entre syncs y daemon sano → `ensure_daemon` hace restart (no retorna idempotente)
- [x] [green] Implementar detección de hash: comparar `_hash_config` del config actual vs contenido previo en state; si difiere, forzar restart

### 3.8 Concurrencia — file lock

- [x] [red] Escribir test: dos llamadas concurrentes a `ensure_daemon` resultan en exactamente 1 proceso spawneado (simular con threads o dos procesos) (`tests/unit/test_mcp_daemon_lock.py`)
- [x] [green] Implementar `_acquire_lock(state_dir)` como context manager con `fcntl.flock(LOCK_EX)` y envolver la sección crítica de `ensure_daemon`

### 3.9 stop_daemon

- [x] [red] Escribir test: daemon activo → `stop_daemon` envía SIGTERM, espera exit, elimina `proxy.pid`, `proxy.port`, `proxy.named-config.json` (`tests/unit/test_mcp_daemon_stop.py`)
- [x] [red] Escribir test: no hay daemon activo (sin state files) → `stop_daemon` retorna `False` sin error
- [x] [red] Escribir test: state files presentes pero PID muerto → `stop_daemon` limpia los archivos y retorna `False`
- [x] [green] Implementar `stop_daemon(git_root: Path) -> bool`

### 3.10 status_daemon

- [x] [red] Escribir test: daemon vivo → `status_daemon` retorna dict con al menos `{pid, port}` (`tests/unit/test_mcp_daemon_status.py`)
- [x] [red] Escribir test: daemon muerto o sin state files → `status_daemon` retorna `None`
- [x] [green] Implementar `status_daemon(git_root: Path) -> dict | None`; incluir `uptime_s` si se puede calcular desde mtime de `proxy.pid`

### 3.11 restart_daemon

- [x] [red] Escribir test: `restart_daemon` = `stop_daemon` seguido de `ensure_daemon`; daemon queda vivo con nuevo puerto (`tests/unit/test_mcp_daemon_restart.py`)
- [x] [green] Implementar `restart_daemon(git_root: Path, named_config_path: Path) -> int` como composición de stop + ensure

### 3.12 CLI entrypoint del módulo

- [x] [red] Escribir test: invocar `python3 -m lib._internal.mcp-daemon ensure <git_root> --named-config <path>` retorna exit 0 y escribe state files (`tests/unit/test_mcp_daemon_cli.py`)
- [x] [red] Escribir test: invocar con subcomando `stop` llama `stop_daemon`
- [x] [red] Escribir test: invocar con subcomando `status` imprime info y retorna exit 0 si vivo, exit 1 si muerto
- [x] [red] Escribir test: invocar con subcomando `restart` llama `restart_daemon`
- [x] [green] Implementar bloque `if __name__ == "__main__"` con argparse para `{ensure, stop, status, restart}`

---

> **Depende de Group 3**: los tests de `_resolve_proxy_port` requieren un `.ai-specs/run/proxy.port` válido. Para los unit tests use fixtures que escriban el archivo directamente; para los integration tests use la API real de `mcp-daemon.ensure_daemon`. NO empiece Group 4 sin haber implementado al menos `_state_dir`, `_pick_free_port` y la escritura atómica de `proxy.port` (tasks 3.1).

## Group 4: Renderizado por agente (url para shared)

### 4.1 _resolve_proxy_port y _render_url_entry

- [x] [red] Escribir test: `_resolve_proxy_port` lee el int desde `proxy.port` cuando el archivo existe (`tests/unit/test_mcp_render_url.py`)
- [x] [red] Escribir test: `_resolve_proxy_port` falla con error explícito cuando `proxy.port` no existe y hay MCPs shared
- [x] [red] Escribir test: `_render_url_entry("trello", 54321)` retorna dict con `"url": "http://localhost:54321/servers/trello/mcp"`
- [x] [green] Implementar `_resolve_proxy_port(project_root: Path) -> int` y `_render_url_entry(mcp_id: str, port: int) -> dict` en `lib/_internal/mcp-render.py`

### 4.2 Render Claude — shared emite url

- [x] [red] Escribir test: MCP `trello` con `mode = "shared"`, agente `claude`, port 54321 → entrada en `.mcp.json` contiene `"url": "http://localhost:54321/servers/trello/mcp"` y NO contiene `command`/`args`/`env`
- [x] [green] Modificar `_translate_generic` en `lib/_internal/mcp-render.py`: rama `mode == "shared"` AND `agent == "claude"` emite `_render_url_entry`

### 4.3 Render Cursor — shared emite url

- [x] [red] Escribir test: MCP `trello` con `mode = "shared"`, agente `cursor`, port 54321 → entrada en `.cursor/mcp.json` contiene la URL correcta
- [x] [green] Modificar `_translate_generic`: rama `mode == "shared"` AND `agent == "cursor"` emite `_render_url_entry`

### 4.4 Render OpenCode — shared emite url con type remote

- [x] [red] Escribir test: MCP `trello` con `mode = "shared"`, agente `opencode`, port 54321 → entrada en `opencode.json` contiene `"type": "remote"` y la URL correcta
- [x] [green] Modificar `_translate_opencode` en `lib/_internal/mcp-render.py`: rama shared emite shape nativo de OpenCode

### 4.5 Render Codex — shared SIEMPRE emite stdio (fallback explícito)

- [x] [red] Escribir test: MCP `trello` con `mode = "shared"`, agente `codex` → entrada contiene `command`/`args`/`env` y NO contiene `url`
- [x] [green] Confirmar/implementar fallback explícito en render Codex: ignorar `mode`, emitir stdio siempre

### 4.6 Render Gemini — shared SIEMPRE emite stdio (fallback explícito)

- [x] [red] Escribir test: MCP `trello` con `mode = "shared"`, agente `gemini` → entrada contiene `command`/`args`/`env` y NO contiene `url`
- [x] [green] Confirmar/implementar fallback explícito en render Gemini: ignorar `mode`, emitir stdio siempre

### 4.7 Render — MCPs stdio sin cambios para todos los agentes

- [x] [red] Escribir test: MCP `github` sin `mode` (o con `mode = "stdio"`), agente `claude` → comportamiento idéntico al actual (sin breaking change)
- [x] [green] Verificar que el render path para stdio no fue alterado por los cambios de la rama shared

### 4.8 Integración del port resolver en main()

- [x] [red] Escribir test: `translate_servers` resuelve el puerto desde `proxy.port` solo si algún server tiene `mode = "shared"` (no lo lee si todo es stdio)
- [x] [green] Modificar `main()` o `translate_servers()` en `mcp-render.py` para resolver el puerto una vez antes del loop de servers y pasarlo como contexto

### 4.9 Limpieza del campo mode antes de serializar

- [x] [red] Escribir test: ningún config file de agente contiene la key `mode` en el output final (el campo no es nativo de los schemas de los agentes)
- [x] [green] Asegurar que `mode` se elimina del dict antes de serializar en todos los paths de render

---

## Group 5: Orquestación en sync.sh y subcomando daemon

### 5.1 Paso "ensure mcp-proxy daemon" en sync.sh

- [x] [red] Escribir test de integración: sync con al menos un MCP shared → `sync.sh` invoca `mcp-daemon.py ensure` antes del fan-out (`tests/integration/test_sync_daemon_step.sh`)
- [x] [red] Escribir test de integración: sync sin MCPs shared → `sync.sh` NO invoca `mcp-daemon.py ensure` (el paso se omite)
- [x] [red] Escribir test de integración: fallo de `mcp-daemon.py ensure` → sync termina con error, fan-out NO se ejecuta
- [x] [green] Modificar `lib/sync.sh`: añadir bloque condicional post-materialización que detecta presencia de `proxy.named-config.json` e invoca `python3 -m lib._internal.mcp-daemon ensure`

### 5.2 lib/daemon.sh — wrapper bash

- [x] [red] Escribir test: `lib/daemon.sh stop` delega a `python3 -m lib._internal.mcp-daemon stop <git_root>` (`tests/unit/test_daemon_sh.sh`)
- [x] [red] Escribir test: `lib/daemon.sh status` delega a `python3 -m lib._internal.mcp-daemon status <git_root>`
- [x] [red] Escribir test: `lib/daemon.sh restart` delega a `python3 -m lib._internal.mcp-daemon restart <git_root> --named-config <path>`
- [x] [red] Escribir test: subcomando desconocido imprime usage y sale con error
- [x] [green] Crear `lib/daemon.sh` con parseo de `{stop|status|restart}`, resolución de `git_root` vía `git rev-parse --show-toplevel`, delegación al entrypoint Python

### 5.3 bin/ai-specs — dispatch daemon

- [x] [red] Escribir test: `ai-specs daemon stop` invoca `lib/daemon.sh stop` (`tests/integration/test_ai_specs_daemon_dispatch.sh`)
- [x] [red] Escribir test: `ai-specs daemon status` invoca `lib/daemon.sh status`
- [x] [red] Escribir test: `ai-specs daemon restart` invoca `lib/daemon.sh restart`
- [x] [red] Escribir test: `ai-specs help` incluye `daemon` en el listado de subcomandos
- [x] [green] Modificar `bin/ai-specs`: añadir `daemon) bash "$LIB_DIR/daemon.sh" "$@" ;;` al case, actualizar help text
- [x] [green] Añadir `lib/daemon.sh` al path cubierto por `tests/validate.sh` (`bash -n lib/*.sh`)

---

## Group 6: Doctor — check de uvx

### 6.1 Check daemon-uvx cuando hay MCPs shared

- [x] [red] Escribir test: doctor con manifest que tiene MCP `mode = "shared"` + `uvx` ausente en PATH → exit code != 0, output contiene `ERROR  daemon-uvx` (`tests/integration/test_doctor_uvx_missing.sh`)
- [x] [red] Escribir test: doctor con MCP shared + `uvx` presente en PATH → check `daemon-uvx` aparece como `OK`
- [x] [red] Escribir test: doctor sin MCPs shared → check `daemon-uvx` NO aparece en el output (no aplica)
- [x] [green] Implementar check `daemon-uvx` en `lib/_internal/doctor.py`: resolver MCPs shared del manifest + presets; si hay al menos uno y `shutil.which("uvx") is None` → `ERROR` con guidance de install; si presente → `OK`

### 6.2 Check daemon-running (opcional pero en scope)

- [x] [red] Escribir test: doctor con state files presentes y daemon vivo → `daemon-running` aparece como `OK`
- [x] [red] Escribir test: doctor con state files presentes pero daemon caído → `daemon-running` aparece como `WARN` con guidance para ejecutar `ai-specs sync`
- [x] [red] Escribir test: doctor sin state files → check `daemon-running` no se evalúa
- [x] [green] Implementar check `daemon-running` en `lib/_internal/doctor.py`: si existen state files, hacer healthcheck; emitir OK/WARN según resultado

---

## Group 7: Sync degradation — uvx ausente

- [x] [red] Escribir test de integración: sync con MCP shared y `uvx` ausente → exit 0, output contiene `WARN`, configs de agente quedan con `command`/`args`/`env` (stdio) (`tests/integration/test_sync_uvx_missing_degrades.sh`)
- [x] [red] Escribir test: el daemon NO es invocado cuando `uvx` está ausente
- [x] [red] Escribir test: la degradación es local y no modifica el manifest (no persiste `mode = "stdio"` en `ai-specs.toml`)
- [x] [green] Implementar en `lib/sync.sh` (o en `mcp-daemon.py ensure` que delega al shell): si `uvx` no en PATH y hay MCPs shared → emitir `WARN`, sobrescribir mode a stdio para la render de esta sync, continuar sin invocar daemon
- [x] [refactor] Extraer la detección de `uvx` en función reutilizable entre doctor y sync

---

## Group 8: Recipe trello y .gitignore

### 8.1 catalog/recipes/trello-mcp-workflow/recipe.toml

- [x] [red] Escribir test: fixture que materialize la recipe trello → produce `proxy.named-config.json` con server `trello` (valida que el campo `mode = "shared"` está activo) (`tests/test_trello_recipe_shared.py`)
- [x] [green] Modificar `catalog/recipes/trello-mcp-workflow/recipe.toml`: añadir `mode = "shared"` al bloque `[[provides.mcp]]` del preset trello

### 8.2 .gitignore — patrón .ai-specs/run/

- [x] [red] Escribir test: el template `templates/gitignore-root.tmpl` (y `ai-specs init` end-to-end) incluye el patrón `.ai-specs/run/` dentro del bloque gestionado (`tests/test_gitignore_run_dir.py`)
- [x] [green] Añadir patrón `.ai-specs/run/` al template `templates/gitignore-root.tmpl` (fuente que `ai-specs init` appendea a `.gitignore` del root)
- [x] [green] Proyectos ya inicializados pueden actualizar su `.gitignore` corriendo `ai-specs init --force` (regenera el bloque gestionado desde el template)

---

## Group 9: Integration test — happy path completo

- [ ] [red] Escribir test de integración end-to-end: declarar MCP shared en fixture, ejecutar `ai-specs sync`, verificar que `proxy.pid` existe y `kill -0 $(cat proxy.pid)` succeed (`tests/integration/test_daemon_end_to_end.sh`)
- [ ] [red] Escribir test: `GET http://localhost:$(cat proxy.port)/status` retorna 200
- [ ] [red] Escribir test: `.mcp.json` (claude) contiene `"url": "http://localhost:.../servers/trello/mcp"`
- [ ] [red] Escribir test de idempotencia: segunda sync sin cambios → PID idéntico, un solo proceso `mcp-proxy` activo (`tests/integration/test_daemon_idempotency.sh`)
- [ ] [red] Escribir test de worktrees concurrentes: dos `ai-specs sync` en paralelo desde worktrees distintos → exactamente 1 proceso `mcp-proxy` al final (`tests/integration/test_daemon_concurrent_syncs.sh`)
- [ ] [red] Escribir test: manifest con `mode = "stdio"` anula recipe `mode = "shared"` → daemon no arranca, render emite stdio (`tests/integration/test_manifest_precedence_over_recipe.sh`)
- [ ] [red] Escribir test: `ai-specs daemon stop` termina el proceso y elimina state files
- [ ] [green] Asegurar que todos los fixtures necesarios para los tests de integración existen y son autocontenidos
- [ ] [refactor] Crear helper compartido de fixtures de integración si los tests repiten patrones de setup similares

---

## Group 10: Edge cases y resolución de open questions (Q1/Q2)

### 10.1 Edge cases verificables durante apply

- [ ] [red] Escribir test: `proxy.pid` existe pero proceso muerto → siguiente `ensure_daemon` detecta PID muerto y respawnea (`tests/integration/test_daemon_dead_pid_recovery.sh`)
- [ ] [red] Escribir test: `proxy.port` existe pero puerto no responde (ocupado por otro proceso) → healthcheck retorna False → `ensure_daemon` asigna puerto nuevo
- [ ] [red] Escribir test: zero MCPs shared → directorio `.ai-specs/run/` no se crea y `ensure_daemon` no es invocado
- [ ] [green] Verificar comportamiento del port race (socket cerrado entre `_pick_free_port` y spawn de mcp-proxy) — documentar como known minor race en `CLAUDE.md` si no es prevenible

### 10.2 Resolución de Q1 — comportamiento de mcp-proxy ante crash de MCP interno

> **Nota**: Esta tarea depende de verificar el comportamiento real de `mcp-proxy` durante apply.

- [ ] [green] Durante apply, verificar contra README de mcp-proxy si un MCP stdio interno que crashea es reiniciado automáticamente por el proxy o si el error se propaga al cliente HTTP
- [ ] [green] Si mcp-proxy NO reinicia automáticamente: documentar en `CLAUDE.md` como comportamiento conocido ("si un MCP shared crashea, `ai-specs daemon restart` lo recupera")
- [ ] [green] Si mcp-proxy SÍ reinicia automáticamente: documentar como bonus, ninguna acción adicional requerida

### 10.3 Resolución de Q2 — shape del endpoint /status de mcp-proxy

> **Nota**: Esta tarea depende de verificar el contrato del endpoint durante apply.

- [ ] [green] Durante apply, verificar el payload JSON de `GET /status` en mcp-proxy: ¿retorna metadata estructurada sobre los servers alojados, o sólo "alive"?
- [ ] [green] Si retorna metadata: actualizar `status_daemon` para incluir `servers: [...]` en el dict de retorno
- [ ] [green] Si retorna sólo "alive": ajustar `status_daemon` para retornar sólo `{pid, port, uptime_s}` y documentar "para listar MCPs alojados, consultar `proxy.named-config.json`"

### 10.4 Validación de suite completa

- [ ] [green] Ejecutar `./tests/run.sh` y confirmar que la suite completa pasa
- [ ] [green] Ejecutar `./tests/validate.sh` y confirmar que py_compile, shell syntax y unit tests pasan
- [ ] [refactor] Revisar cobertura de tests añadidos; identificar paths sin tests y añadir casos faltantes si quedan brechas críticas
