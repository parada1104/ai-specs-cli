# Design: MCP compartido por proyecto

## Contexto técnico

Hoy el pipeline de `ai-specs sync` materializa MCPs declarados en `[mcp.*]` y `[[provides.mcp]]` como configs stdio independientes por agente: `mcp-render.py` recorre los servidores merged y emite, para cada agente habilitado, una entrada `command`/`args`/`env` en el formato nativo (`.mcp.json`, `opencode.json`, `.codex/config.toml`, etc.). El runtime de cada agente lanza su propio subprocess stdio cada vez que invoca un tool del MCP. Esto se multiplica por N agentes y por M worktrees del mismo repositorio.

Este cambio introduce una capa de daemon HTTP multiplexante (`mcp-proxy`) que vive una sola vez por raíz git y aloja todos los MCPs marcados como `shared`. Los agentes con capacidad HTTP (Claude, Cursor, OpenCode) reciben una entrada `url` apuntando a `http://localhost:{port}/servers/{name}/mcp`; los agentes sin HTTP (Codex, Gemini) siguen recibiendo stdio. La identidad del daemon es la raíz git canónica (`dirname` del `git rev-parse --path-format=absolute --git-common-dir`), por lo que distintos worktrees del mismo repo comparten un único proceso.

Los módulos que crecen son `recipe-materialize.py` (split shared/stdio + escritura del `named-server-config.json`), `mcp-render.py` (rama `url` para shared en los 3 agentes HTTP) y `sync.sh` (paso "ensure mcp-proxy daemon" antes del fan-out + subcomando `ai-specs daemon ...`). El módulo nuevo `lib/_internal/mcp-daemon.py` concentra todo el ciclo de vida del proceso `mcp-proxy` (start, healthcheck, stop, status, restart, asignación de puerto, file locking). `doctor.py` recibe un check preventivo nuevo (verificar `uvx` en PATH cuando hay MCPs `shared`). Los validators de schema (recipe y manifest) aceptan el campo opcional `mode`.

## Arquitectura del daemon

```text
worktree-A/$ ai-specs sync
        │
        ▼
   sync.sh
        │
        ├─► recipe-materialize.py
        │      │
        │      ├─► split_mcps_by_mode(merged_mcp)
        │      │    │
        │      │    ├─► shared = {"trello": {...}}
        │      │    └─► stdio  = {"github": {...}}
        │      │
        │      ├─► write_named_server_config(shared, <git-root>/.ai-specs/run/proxy.named-config.json)
        │      │
        │      └─► write_recipe_mcp_temp(merged_mcp_with_mode)   # mode preservado para downstream
        │
        ├─► if shared not empty:
        │      mcp-daemon.py ensure  <git-root>  --named-config <path>
        │           │
        │           ├─► git_root = dirname(rev-parse --git-common-dir)   # canonical repo root, shared across worktrees
        │           ├─► state_dir = <git-root>/.ai-specs/run/
        │           ├─► acquire lock state_dir/proxy.lock   (fcntl LOCK_EX)
        │           ├─► if proxy.pid && proxy.port exist:
        │           │      pid_alive = os.kill(pid, 0)
        │           │      port_ok   = HTTP GET http://localhost:port/status (timeout=2s)
        │           │      if pid_alive AND port_ok:
        │           │           return port                              ◄── idempotent
        │           │      else:
        │           │           SIGTERM pid; remove stale files
        │           │
        │           ├─► port = _pick_free_port()                    # socket().bind(('',0))
        │           ├─► spawn: uvx mcp-proxy --port {port} \
        │           │              --named-server-config <path>
        │           │      with start_new_session=True,
        │           │           stdout/stderr → state_dir/proxy.log
        │           │
        │           ├─► poll healthcheck loop (max ~5s)
        │           ├─► write proxy.pid, proxy.port (atomic)
        │           └─► release lock; return port
        │
        └─► mcp-render.py  (per enabled agent)
               │
               ├─► load recipe_mcp_temp + manifest mcp (merged with mode preserved)
               ├─► for each server:
               │      if mode == "shared" AND agent in {claude,cursor,opencode}:
               │           emit  { "url": "http://localhost:{port}/servers/{id}/mcp" }
               │      else:
               │           emit  { "command", "args", "env" }    # current behaviour
               │
               └─► write per-agent file (.mcp.json, opencode.json, .codex/config.toml, ...)
```

```text
Multi-worktree topology for one repo:

   <git-root>/.ai-specs/run/proxy.{pid,port,named-config.json,lock,log}
                 ▲                   ▲
                 │                   │
   worktree-A/   │   worktree-B/     │
   ai-specs sync ┘   ai-specs sync ──┘
                        │
                        ▼
                  same proxy.pid/port file → same daemon → port reused
```

## Decisiones técnicas

### Decisión 1: Lenguaje del daemon manager — Python

`lib/_internal/mcp-daemon.py` se implementa en Python.

- **Rationale**: alinea con `mcp-render.py` y `recipe-materialize.py`; el daemon requiere HTTP GET, JSON manipulation, socket binding, signal handling y file locking, todo trivial en stdlib (`urllib`, `json`, `socket`, `os`, `fcntl`). El resto de los módulos `_internal/*.py` ya siguen el patrón script-importable.
- **Alternativas**:
  - Bash script puro: rechazado — HTTP healthcheck y manipulación de JSON con `jq` es feo y poco testeable.
  - Go binary embebido: rechazado — el CLI distribuye sólo scripts, no binarios.

### Decisión 2: Mecanismo de detachment

`subprocess.Popen(cmd, start_new_session=True, stdout=log_fd, stderr=log_fd, stdin=DEVNULL, close_fds=True)`, con `log_fd = open(state_dir/'proxy.log', 'ab')`.

- **Rationale**: `start_new_session=True` invoca `setsid(2)` en el child, desvinculándolo del process group del shell que invocó `ai-specs sync`. Redirigir `stdout`/`stderr` evita que el daemon herede los pipes del padre. Es nativo y portable a macOS/Linux.
- **Alternativas**:
  - `nohup`/`setsid` shell wrappers: rechazado — requiere shell intermedio y oculta el PID real.
  - `os.fork()` + doble-fork: rechazado — más complejo, no aporta sobre `start_new_session=True`.
- **Trade-off**: `proxy.log` crece sin rotación. Aceptable porque mcp-proxy es relativamente silencioso; documentar como deuda menor.

### Decisión 3: Healthcheck protocol

`urllib.request.urlopen(f"http://localhost:{port}/status", timeout=2.0)` retorna 200 → daemon vivo. Cualquier `URLError`, `socket.timeout` o código no-200 → daemon muerto. Combinado con `os.kill(pid, 0)` para PID liveness.

- **Rationale**: `mcp-proxy` expone `GET /status` por contrato (documentado en su README). El doble check (PID + puerto) cubre los dos modos de falla independientes: proceso muerto pero puerto ocupado por otro, o proceso vivo pero hung.
- **Alternativas**:
  - Solo PID check: rechazado — puerto colgado sin proceso (port en TIME_WAIT, otro proceso usando ese port) es posible.
  - Solo healthcheck HTTP: rechazado — quedaría sin detectar el caso en que mcp-proxy crashed pero su PID file persiste.
- **Trade-off**: 2 syscalls por idempotency check (~5 ms). Insignificante.

### Decisión 4: Asignación de puerto

```python
def _pick_free_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s:
        s.bind(('', 0))
        return s.getsockname()[1]
```

El kernel asigna un puerto efímero libre en `(49152, 65535)` (en macOS y Linux). El socket se cierra inmediatamente; mcp-proxy reabre el mismo puerto microsegundos después (race teóricamente posible pero prácticamente nunca observado en este patrón).

- **Rationale**: simple, dependiente del kernel, no asume rangos. Persistimos el puerto elegido en `proxy.port` para que syncs siguientes lo reusen sin re-elegir.
- **Alternativas**:
  - Hash determinista del git root: rechazado — dos proyectos podrían colisionar; además requeriría un rango reservado que el usuario podría tener ocupado.
  - Range scan manual (`for p in range(49152, 65536)`): rechazado — más código, mismo resultado.
- **Trade-off**: si el sistema operativo decide reusar el mismo puerto para otra cosa entre el `close()` y el spawn de `mcp-proxy`, la siguiente sync detecta `port_ok == False` y reasigna. Auto-correctivo.

### Decisión 5: Reload vs restart al cambiar el named-config

Cuando `proxy.named-config.json` cambia entre syncs y el daemon está corriendo, **restart completo**: SIGTERM al PID actual, esperar exit, spawn nuevo en el mismo puerto si está libre o uno nuevo si no.

- **Rationale**: `mcp-proxy` actualmente no documenta un mecanismo de reload por señal (no responde a SIGHUP recargando la config). Implementar reload requeriría un cambio en upstream. El restart cuesta <500 ms (mcp-proxy inicializa muy rápido) y es la solución correcta hasta que upstream documente algo mejor.
- **Detección de cambio**: hash SHA-256 del JSON canónico antes y después de escribir. Si difiere y el daemon está corriendo, restart.
- **Alternativas**:
  - SIGHUP reload: rechazado por desconocido en upstream (verificable si emergiera, no es prioridad).
  - Reload via HTTP POST a un endpoint admin: rechazado — mcp-proxy no expone tal endpoint.
- **Trade-off**: durante el restart (~500 ms) los agentes que ya tenían conexión abierta verán un drop de conexión; los SDKs de MCP suelen reconectar.

### Decisión 6: Estado compartido entre worktrees

Archivos en `<git-root>/.ai-specs/run/`:
- `proxy.pid` — PID del proceso
- `proxy.port` — puerto asignado
- `proxy.named-config.json` — config consumida por mcp-proxy (incluye secrets resueltos)
- `proxy.lock` — flock para serializar `ensure_daemon` entre procesos
- `proxy.log` — stdout/stderr del daemon

- **Rationale**: la raíz git canónica es única por repositorio, sin importar cuántos worktrees existan. `git rev-parse --path-format=absolute --git-common-dir` apunta al `.git` real (no al gitdir-link de un worktree); su `dirname` es el working tree del repo principal y todos los worktrees lo resuelven al mismo valor. Todos los worktrees inspeccionan/escriben en la misma ruta absoluta.
- **Trade-off**: en un worktree sin la carpeta `.ai-specs/run/` la primera sync debe crearla. Aceptable; mkdir parents=True.

### Decisión 7: Concurrencia entre syncs simultáneos

`fcntl.flock(lock_fd, fcntl.LOCK_EX)` sobre `<state_dir>/proxy.lock` durante toda la sección crítica de `ensure_daemon` (read state → healthcheck → spawn → write state).

- **Rationale**: dos worktrees ejecutando `ai-specs sync` simultáneamente podrían cada uno detectar "no daemon", spawnear dos procesos, y dejar `proxy.pid` apuntando a sólo uno. El lock serializa la decisión; la segunda invocación, al entrar, ve el estado escrito por la primera y reutiliza.
- **Alternativas**:
  - Sin lock: rechazado — race condition observable, dos daemons orfandando el puerto el uno al otro.
  - PID-file con atomic create (`O_EXCL`): rechazado — más sutil, mismo resultado que flock.
- **Trade-off**: el lock se mantiene durante el spawn + healthcheck (~3-5 s en peor caso). Aceptable; syncs simultáneos esperan por turnos.

### Decisión 8: Resolver `uvx`

`shutil.which("uvx")` al arrancar `ensure_daemon`. Si no existe → ver Resolución de TBDs (TBD-2) abajo.

- **Rationale**: `uvx` es la forma idiomática de invocar `mcp-proxy` (cachea la instalación, no requiere `pip install` previo del usuario). Es razonable depender de él porque `ai-specs` ya documenta `uv`/`uvx` en el README de instalación.

## Resolución de TBDs

### TBD-1: subcomandos `daemon status` y `daemon restart`

**Decisión**: **incluir ambos en scope** de este cambio, junto a `daemon stop` (ya comprometido en la proposal).

```text
ai-specs daemon stop      # SIGTERM al daemon, cleanup de state files
ai-specs daemon status    # imprime pid, port, uptime, MCPs alojados (lee /status del proxy)
ai-specs daemon restart   # stop seguido de ensure
```

- **Rationale**: el costo marginal de implementar `status` y `restart` sobre `stop` + `ensure_daemon` es bajo (cada uno es <30 líneas) y el valor en DX durante development es alto. Un usuario que sospecha que el daemon murió necesita `status` para confirmarlo y `restart` para arreglarlo sin tener que invocar `sync` completo (que toca muchos archivos).
- **Comportamiento de `status`**: retorna exit 0 con info si vive, exit 1 sin info si está caído o ausente. `restart` siempre intenta el ciclo completo y termina con 0 si el daemon queda sano.
- **Alternativa rechazada**: scope minimal (sólo `stop`) — DX inferior; agregar después requeriría otro change.

### TBD-2: comportamiento sin `uvx` en PATH

**Decisión**: **estrategia híbrida — doctor fail-fast preventivo + sync degradado graceful**.

- En `ai-specs doctor`: si la materialización detecta al menos un MCP con `mode = "shared"` (resuelto desde `[mcp.*]` + recipe presets) y `shutil.which("uvx") is None` → emitir check `ERROR` con guidance "`install uv: curl -LsSf https://astral.sh/uv/install.sh | sh`". Exit code de doctor cambia a no-zero.
- En `ai-specs sync`: si hay MCPs `shared` y `uvx` no está en PATH → emitir `WARN`, marcar esos MCPs como `mode = "stdio"` para el render de esa sync (degradación local, no persistente al manifest), continuar el pipeline. Sync termina con exit 0.

- **Rationale**:
  - Doctor está pensado para CI/CD y onboarding: que rompa preventivamente cuando algo es inconsistente da feedback temprano.
  - Sync está pensado para iteración del desarrollador: romper toda la sync porque un binario falta sería hostil; degradar al comportamiento previo (stdio) preserva el flujo y el usuario aún ve sus agentes funcionando, sólo sin la optimización del daemon. El `WARN` deja claro que algo está sub-óptimo.
- **Cobertura por tests**:
  - doctor: `tests/integration/test_doctor_uvx_missing.sh` — fixture con MCP shared y PATH sin uvx → exit 1 + check ERROR.
  - sync: `tests/integration/test_sync_uvx_missing_degrades.sh` — fixture similar → exit 0 + WARN + configs renderizadas con stdio.
- **Alternativas rechazadas**:
  - `fail-fast` en sync: UX hostil; rompe en medio de un flujo de iteración.
  - Auto-install via `pip`/`brew`: invasivo, fuera del scope de `ai-specs`.
  - Sólo doctor sin degradación en sync: el usuario que ignora doctor verá un sync que falla en producción.

## Cambios concretos por archivo

### Modificados

#### `lib/_internal/recipe-materialize.py`

- **Nueva función** `split_mcps_by_mode(merged_mcp: dict) -> tuple[dict, dict]`:
  - Input: dict `{server_id: config}` ya merged (recipe + manifest).
  - Output: `(shared, stdio)` donde `shared` contiene los entries con `mode == "shared"` y `stdio` el resto (incluye los que no declaran `mode`).
  - Side effect: cada entry conserva su key `mode` original para el downstream.
- **Nueva función** `write_named_server_config(shared: dict, output_path: Path) -> None`:
  - Escribe `{"mcpServers": shared_minus_mode}` al `output_path`.
  - `shared_minus_mode` elimina la key `mode` de cada server porque mcp-proxy no la reconoce; conserva `command`, `args`, `env` literales.
  - Setea permisos `0o600` (touchea secrets resueltos del shell env).
- **Modificar** `materialize_recipes()`:
  - Tras el merge final, llama `split_mcps_by_mode(recipe_mcp)`.
  - Si `shared` no vacío: `write_named_server_config(shared, <git_root>/.ai-specs/run/proxy.named-config.json)`. El `git_root` se obtiene resolviendo `subprocess.run(['git', 'rev-parse', '--path-format=absolute', '--git-common-dir'], cwd=project_root)` y tomando su `Path(...).parent`.
  - Continúa escribiendo el `recipe_mcp_out` temp con los servers completos (incluyendo `mode`) para que `mcp-render.py` decida.

#### `lib/_internal/mcp-render.py`

- **Nueva función** `_render_url_entry(mcp_id: str, port: int) -> dict`:
  - Retorna `{"type": "remote", "url": f"http://localhost:{port}/servers/{mcp_id}/mcp"}` para agentes que aceptan ese shape (Claude, Cursor, OpenCode).
- **Nueva función** `_resolve_proxy_port(project_root: Path) -> int`:
  - Lee `<git_root>/.ai-specs/run/proxy.port`. Falla con error explícito si no existe Y hay MCPs `shared` en el set actual.
- **Modificar** `_translate_generic` y `_translate_opencode`:
  - Antes del switch por agent: si `cfg.get("mode") == "shared"` Y `agent in ("claude", "cursor", "opencode")` → emitir entry tipo `url` (con shape específico por agente).
  - Para Codex y Gemini: ignorar `mode` y emitir stdio como hasta ahora (fallback explícito).
  - El `mode` se elimina del dict antes de serializar (no es un campo nativo de los schemas de los agentes).
- **Modificar** `translate_servers(agent, servers)`:
  - Recibe ahora un dict opcional `runtime: {"port": int}` con el puerto pre-resuelto. El caller (en `main()`) lo resuelve si detecta `mode == "shared"` en cualquier server.

#### `lib/sync.sh`

- **Después** del paso `recipe-materialize`, **antes** del fan-out:
  ```bash
  if [[ -f "$ROOT_PATH/.ai-specs/run/proxy.named-config.json" ]]; then
      echo "▸ ensure mcp-proxy daemon"
      python3 lib/_internal/mcp-daemon.py ensure "$GIT_ROOT" \
        --named-config "$GIT_ROOT/.ai-specs/run/proxy.named-config.json" \
        || { echo "ERROR: daemon ensure failed"; exit 1; }
  fi
  ```
  - El `named-config.json` se escribe sólo si hay MCPs shared → su presencia es el switch.
- **Nuevo subcomando** `ai-specs daemon {stop|status|restart}` vía `bin/ai-specs` + `lib/daemon.sh`:
  - `lib/daemon.sh` parsea el subcomando, resuelve `git_root` y delega a `python3 lib/_internal/mcp-daemon.py {stop|status|restart} "$GIT_ROOT" [--named-config ...]` (invocación por ruta directa porque el nombre del archivo contiene un guion).

#### `lib/doctor.sh` y `lib/_internal/doctor.py`

- **Nuevo check** `daemon-uvx`:
  - Sólo se ejecuta cuando el manifest + recipe presets producen al menos un MCP con `mode = "shared"` resuelto.
  - Si `shutil.which("uvx") is None` → `ERROR` con guidance "Install uv from https://docs.astral.sh/uv/".
  - Si presente → `OK`.
- **Nuevo check opcional** `daemon-running`:
  - Si hay state files en `<git_root>/.ai-specs/run/` → intentar healthcheck.
  - Si vivo → `OK`. Si state files presentes pero daemon caído → `WARN` con guidance "Run `ai-specs sync` to restart the daemon".
  - Si no hay state files → no se evalúa (no es un problema en sí mismo).

#### `lib/_internal/recipe_schema.py` (validator de recipe.toml)

- En la definición de `[[provides.mcp]]`: aceptar key opcional `mode` con enum `["shared", "stdio"]`. Rechazar otros valores con `RecipeValidationError`.

#### `lib/_internal/toml-read.py` (reader de manifest)

- En `read_mcp(data)`: preservar key `mode` si presente. Validar enum `["shared", "stdio"]`; rechazar otros.

#### `catalog/recipes/trello-mcp-workflow/recipe.toml`

- En el bloque `[[provides.mcp]]` del trello server: añadir `mode = "shared"`.

#### `.gitignore` (template `bundled-skills/.../template-gitignore` o equivalente que `init` materializa)

- Añadir patrón `.ai-specs/run/`.
- `gitignore-render.py` ya rinde patrones de `[[deps]]`; añadir aquí el patrón estático del daemon.

### Nuevos

#### `lib/_internal/mcp-daemon.py`

API pública del módulo:

```python
def ensure_daemon(git_root: Path, named_config_path: Path) -> int:
    """Idempotent: returns the port of a healthy daemon. Spawns if needed."""

def healthcheck(port: int, timeout: float = 2.0) -> bool:
    """HTTP GET /status with timeout. True iff response is 200."""

def stop_daemon(git_root: Path) -> bool:
    """SIGTERM + wait. Cleans state files. Returns True if a daemon was stopped."""

def status_daemon(git_root: Path) -> dict | None:
    """Returns {pid, port, uptime_s, servers: [...]} if alive; None otherwise."""

def restart_daemon(git_root: Path, named_config_path: Path) -> int:
    """stop + ensure. Returns the new port."""

def _pick_free_port() -> int:
    """socket().bind(('', 0)) idiom."""

def _state_dir(git_root: Path) -> Path:
    """Returns <git_root>/.ai-specs/run/. Creates if needed."""

def _acquire_lock(state_dir: Path):
    """Context manager: fcntl flock LOCK_EX on state_dir/proxy.lock."""

def _is_pid_alive(pid: int) -> bool:
    """os.kill(pid, 0) wrapped in try/except."""

def _hash_config(path: Path) -> str:
    """SHA-256 of canonical JSON content; for change detection."""
```

CLI entrypoint (cuando se invoca como módulo):

```text
python3 lib/_internal/mcp-daemon.py ensure  <git_root> --named-config <path>
python3 lib/_internal/mcp-daemon.py stop    <git_root>
python3 lib/_internal/mcp-daemon.py status  <git_root>
python3 lib/_internal/mcp-daemon.py restart <git_root> --named-config <path>
```

#### `lib/daemon.sh`

Bash wrapper que parsea `ai-specs daemon {stop|status|restart}`, resuelve `git_root` vía `git rev-parse --path-format=absolute --git-common-dir` (parent) y delega al CLI entrypoint de `mcp-daemon.py` por ruta directa (el nombre del archivo contiene un guion, así que `python3 -m` no es viable). Reporta errores con el formato estándar del resto de comandos.

#### `bin/ai-specs`

Añadir entry `daemon) bash "$LIB_DIR/daemon.sh" "$@" ;;` al `case`. Actualizar el `help` text.

## Lifecycle del daemon — diagrama de estados

```text
                         ┌───────────────────┐
                         │   not-running     │
                         │ (no state files;  │
                         │  no proc)         │
                         └──────────┬────────┘
                                    │ ensure_daemon() called
                                    │ AND shared MCPs detected
                                    ▼
                         ┌───────────────────┐
                         │     starting      │
                         │ (spawn pending;   │
                         │  state half-      │
                         │  written)         │
                         └──────────┬────────┘
                                    │ healthcheck OK
                                    │ state files committed
                                    ▼
        ┌──────────────────┐ ┌───────────────────┐ ┌────────────────────┐
        │  daemon stop     │ │     running       │ │  config change     │
        │  (SIGTERM)       │◄┤  (pid + port      ├─►   detected         │
        └────────┬─────────┘ │   responding)     │ │  (hash differs)    │
                 │           └──────────┬────────┘ └─────────┬──────────┘
                 │                      │                    │
                 │     ┌────────────────┴───────┐            │
                 │     │ process died (no PID)  │            │
                 │     │ OR port not responding │            │
                 │     ▼                        │            ▼
                 │  ┌───────────────┐           │  ┌──────────────────┐
                 │  │     dead      │           │  │    restarting    │
                 │  │ (state stale, │           │  │ (SIGTERM + spawn │
                 │  │  proc gone)   │           │  │  new on same or  │
                 │  └──────┬────────┘           │  │  new port)       │
                 │         │ next sync detects  │  └─────────┬────────┘
                 │         │ → respawn          │            │
                 ▼         ▼                    ▼            │
              ┌──────────────────────────────────────────────┘
              ▼
         not-running
```

Triggers:
- `not-running → starting`: `sync.sh` ejecuta `ensure_daemon` con MCPs shared presentes
- `starting → running`: primer healthcheck OK + state files persistidos
- `running → restarting`: `_hash_config` detecta cambio entre syncs
- `running → dead`: external crash; detectado por próximo `ensure_daemon`
- `dead → starting`: próximo `sync` o `ai-specs daemon restart`
- `running → not-running`: `ai-specs daemon stop`

## Tests strategy

### Unit tests (Python, `tests/unit/`)

- `test_mcp_daemon_pick_free_port.py`:
  - `_pick_free_port()` retorna un int en (1024, 65536).
  - Dos invocaciones consecutivas retornan ints diferentes con alta probabilidad.
- `test_mcp_daemon_healthcheck.py`:
  - Con un servidor HTTP fake (`http.server` en hilo) que responde 200 → `healthcheck(port) == True`.
  - Con puerto sin nada → `healthcheck(port) == False` (timeout).
  - Con servidor que responde 500 → `healthcheck(port) == False`.
- `test_recipe_materialize_split.py`:
  - Input dict mixto: `{"trello": {..., "mode": "shared"}, "github": {...}}` → `split_mcps_by_mode` retorna `({"trello": ...}, {"github": ...})`.
  - Empty input → `({}, {})`.
  - Sólo stdio → `({}, {...})`. Sólo shared → `({...}, {})`.
- `test_recipe_materialize_named_config.py`:
  - `write_named_server_config` escribe JSON con shape `{"mcpServers": {...}}`.
  - Excluye la key `mode` del output.
  - Setea permisos `0o600`.
- `test_mcp_render_url.py`:
  - Para cada agent en `{claude, cursor, opencode}` y un server con `mode = "shared"` y `port = 12345`:
    - Output incluye URL `http://localhost:12345/servers/{id}/mcp`.
    - Output NO incluye `command`/`args`/`env`.
  - Para `codex` y `gemini` con `mode = "shared"`:
    - Output incluye `command`/`args`/`env`.
    - Output NO incluye `url`.
- `test_mcp_mode_validation.py`:
  - Recipe con `mode = "shared"` → pasa.
  - Recipe con `mode = "proxy"` → falla con error mencionando enum válido.

### Integration tests (`tests/integration/`)

- `test_daemon_end_to_end.sh`:
  - Fixture con recipe que declara `mode = "shared"` y stdio MCP fake (echo server simple).
  - Levanta sync; verifica `proxy.pid` existe y `kill -0 $(cat proxy.pid)` succeed.
  - GET `http://localhost:$(cat proxy.port)/status` retorna 200.
  - `.mcp.json` (claude) tiene `url: http://localhost:.../servers/trello/mcp`.
- `test_daemon_idempotency.sh`:
  - Sync 1: verifica daemon arranca; record PID.
  - Sync 2 (sin cambios): verifica PID idéntico, daemon no reiniciado.
- `test_daemon_concurrent_syncs.sh`:
  - Lanza `ai-specs sync` desde dos worktrees en paralelo con `&`.
  - Espera ambos; verifica exactamente 1 process `mcp-proxy` activo.
- `test_manifest_precedence_over_recipe.sh`:
  - Recipe: `mode = "shared"`. Manifest: `mode = "stdio"`.
  - Verifica que el daemon no arranca (no MCPs shared post-merge) y el render emite stdio.
- `test_sync_uvx_missing_degrades.sh`:
  - PATH sin `uvx`; recipe shared.
  - Sync sale con 0, emite WARN, configs por agente quedan con stdio.
- `test_doctor_uvx_missing_errors.sh`:
  - PATH sin `uvx`; manifest declara `mode = "shared"`.
  - `ai-specs doctor` exit code != 0, output contiene `ERROR  daemon-uvx`.

### Edge cases

- **PID file exists but process dead**: matar manualmente el daemon entre syncs; siguiente sync debe detectar (`_is_pid_alive == False`) y respawnear.
- **Port file exists but port unreachable**: bloquear el puerto con otro proceso entre syncs; healthcheck retorna False; sync debe SIGTERM (best effort), reasignar puerto.
- **Config change between syncs**: cambiar `mode` de un MCP en manifest entre syncs; verifica restart.
- **Zero shared MCPs**: manifest sin `mode = "shared"` en nada; verifica que `.ai-specs/run/` no se crea y `ensure_daemon` no se invoca.
- **Daemon survives shell close**: spawn daemon, cerrar el shell que lo lanzó (en test usar `subprocess` que termina), verificar que el daemon sigue vivo (test marca un skip en CI si no se puede simular shell close real).

## Riesgos & mitigaciones

- **Daemon crash silencia todos los MCPs `shared`** → mitigation: `doctor` reporta WARN si state files existen pero daemon no responde; siguiente `sync` lo rearranca automáticamente; usuario también puede invocar `ai-specs daemon restart` directamente.
- **Orphan daemon tras logout sin stop** → mitigation: la próxima `sync` detecta PID muerto (o detecta que `proxy.pid` referencia un PID reciclado por el OS, lo cual cubre `_is_pid_alive`+`healthcheck` en conjunto) y rearranca. Aceptado como deuda menor: el usuario que quiere limpieza explícita usa `ai-specs daemon stop`.
- **Race condition entre worktrees simultáneos** → mitigation: file lock `proxy.lock` (fcntl LOCK_EX) durante toda la sección crítica de `ensure_daemon`.
- **Port file stale** → mitigation: `ensure_daemon` siempre hace healthcheck antes de confiar en `proxy.port`; rechaza puertos no-respondedores y reasigna.
- **Secret leak en `proxy.named-config.json`** → mitigation: el archivo se escribe con `chmod 0600` (sólo el usuario lo lee); `.ai-specs/run/` está en `.gitignore`; documentar en CLAUDE.md / README que los secrets viven en ese archivo durante la sesión.
- **`uvx` ausente** → mitigation: TBD-2 resuelto arriba (doctor falla, sync degrada).
- **mcp-proxy upstream cambia su CLI flags o el formato de `/status`** → mitigation: pin de versión via `uvx mcp-proxy@<pinned>` cuando el contrato sea estable (deferred al rollout; por ahora `uvx mcp-proxy` latest).

## Roll-out

### Phase 1 (este change)

- Implementar el daemon manager, schema additions, render branch, sync wiring, doctor check.
- Migrar `trello-mcp-workflow` a `mode = "shared"` en su preset.
- Resto de MCPs (en otras recipes o en manifests de proyectos) siguen stdio: no se tocan.

### Phase 2 (futuro change, no incluido)

- Migrar otros MCPs comunes (postgres, filesystem, github oficial) a `mode = "shared"` en sus recipes cuando exista beneficio claro.
- Considerar pin de versión de `mcp-proxy` (e.g. `uvx mcp-proxy@1.2.3`) cuando upstream se estabilice.
- Considerar log rotation para `proxy.log` si se observa crecimiento problemático.

### Roll-back

- Cambio puramente aditivo en schemas (`mode` opcional). MCPs sin `mode` → stdio idéntico al actual.
- Para revertir: remover el campo `mode = "shared"` de `catalog/recipes/trello-mcp-workflow/recipe.toml` → próxima sync vuelve a stdio sin tocar el daemon.
- Para limpiar el daemon: `ai-specs daemon stop` → SIGTERM + remove state files.
- No hay migración requerida: ningún proyecto existente cambia su comportamiento hasta que adopte explícitamente `mode = "shared"`.

## Open Questions (post-design)

- **Q1**: ¿Cómo se comporta `mcp-proxy` ante un MCP stdio interno que crashea durante runtime? ¿Se reinicia automáticamente o el proxy reporta el error al cliente HTTP? Asumimos lo segundo; verificar contra el README de mcp-proxy durante apply. Si reinicia: bonus, sin acción. Si no: documentar como conocido en CLAUDE.md.
- **Q2**: El endpoint `/status` de `mcp-proxy` ¿retorna metadata estructurada sobre los servers alojados, o sólo "alive"? Esto determina cuánta info puede mostrar `ai-specs daemon status`. Si retorna sólo alive, `status` se queda con `{pid, port, uptime}` y deja la lista de MCPs como "ver `proxy.named-config.json`". A confirmar en apply.

## Resolución de Q1 / Q2 (verificación empírica durante apply)

Ambas preguntas se cerraron contra `uvx mcp-proxy` (instalado vía `uv`) con un MCP real
(`uvx mcp-server-time`) como child stdio. Las decisiones del diseño se preservan; este
bloque documenta el comportamiento observado y reemplaza a "Open Questions (post-design)"
de arriba.

### Q1 — comportamiento ante crash de un MCP stdio interno

**Observado**: `mcp-proxy` inicializa cada named server **una sola vez** al arrancar
(envía el handshake MCP `initialize` y, sólo cuando todos los children responden, expone
el servidor Uvicorn en `/servers/<name>/{sse,mcp}`). No hay restart loop visible en sus
logs ni en su modelo (`mcp_proxy.mcp_server.Setting up named server …` aparece una sola
vez por server; tras eso, el child se mantiene como subprocess estable). Si un child
crashea en runtime, las llamadas posteriores del cliente reciben el error upstream;
upstream **no** revive al child silenciosamente.

**Resolución**: documentado como comportamiento conocido en `docs/mcp-shared-daemon.md`.
Recuperación manual con `ai-specs daemon restart`. La próxima `ai-specs sync` también
re-arranca si el hash de `proxy.named-config.json` cambió (Decisión 5). El check
`daemon-running` del doctor convierte un proxy hung en un `WARN` visible en el siguiente
diagnóstico. La probe activa de crashes mid-runtime se deja diferida a QA manual: el
modelo de inicialización single-shot de upstream es evidencia suficiente del comportamiento.

### Q2 — shape de `GET /status`

**Observado**: el endpoint devuelve JSON con la siguiente forma (ejemplo real):

```json
{
  "api_last_activity": "2026-05-25T07:07:06.006025+00:00",
  "server_instances": {"trello": "configured", "github": "configured"}
}
```

`api_last_activity` es un timestamp ISO-8601 con tz; `server_instances` es un mapping
`{server_id → state_string}`. Hoy el único state observado es `"configured"`; valores
futuros (`"initialising"`, `"failed"`, …) deben tratarse como opacos.

**Resolución**: `status_daemon` enriquecido. Si `/status` responde 200 con JSON parseable,
el dict retornado incluye `api_last_activity` y `servers` (alias semántico de
`server_instances`) junto a los `pid`, `port`, `uptime_s` originales. Si el endpoint no
responde (daemon arrancando, hung, port robado, etc.) el dict mantiene la shape base sin
romper `ai-specs daemon status`. Implementado en `_fetch_status_metadata` (timeout 2 s,
silently swallow `URLError`/`OSError`/`JSONDecodeError`) — ver
`tests/test_daemon_dead_pid_recovery.StatusDaemonExposesProxyMetadataTests` para los dos
extremos.

### Port race (Decisión 4 — refresco)

`_pick_free_port()` cierra el socket microsegundos antes de spawn de `mcp-proxy`. La
ventana de race es real pero auto-correctiva: la próxima `ensure_daemon` healthcheckea
`GET /status` contra el `proxy.port` registrado, falla por `Connection refused`, SIGTERMa
al PID huérfano, reasigna puerto y respawnea. La misma rama cubre el caso de daemon muerto
externamente. Tests: `tests/test_mcp_daemon_ensure.StaleHealthcheckRestartTests` +
`tests/test_daemon_dead_pid_recovery.DeadPidRecoveryTests`.
