# mcp-shared-daemon

## Purpose

Gestionar el ciclo de vida de un proceso `mcp-proxy` único por raíz git que aloja todos los MCPs marcados como `shared`, exponiendo cada uno como Streamable HTTP en `http://localhost:{PORT}/servers/{name}/mcp`.

## Requirements

### Requirement: Startup eagerly during sync when shared MCPs exist

El sistema SHALL iniciar el daemon `mcp-proxy` durante `ai-specs sync` si la materialización detecta al menos un MCP con `mode = "shared"`. El daemon SHALL arrancarse antes del fan-out de configs por agente.

#### Scenario: Sync con MCPs shared inicia el daemon

- **WHEN** `ai-specs sync` materializa al menos un MCP con `mode = "shared"`
- **THEN** el sistema SHALL invocar `ensure_daemon()` antes de ejecutar cualquier `mcp-render` step
- **AND** el daemon SHALL estar accesible vía HTTP antes de que el fan-out genere configs con `url`

#### Scenario: Sync sin MCPs shared omite el daemon

- **WHEN** `ai-specs sync` materializa cero MCPs con `mode = "shared"`
- **THEN** el sistema SHALL NOT iniciar ni verificar el daemon
- **AND** el pipeline SHALL continuar directamente al fan-out

---

### Requirement: Idempotencia — sync repetido no reinicia un daemon sano

El sistema SHALL verificar si un daemon existente responde correctamente antes de iniciar uno nuevo. Si el daemon está sano, SHALL reusar el proceso existente.

#### Scenario: Daemon sano es reutilizado

- **WHEN** existe un archivo `.ai-specs/run/proxy.pid` y `.ai-specs/run/proxy.port` válidos
- **AND** el daemon responde a `GET http://localhost:{port}/status` con HTTP 200
- **THEN** el sistema SHALL NOT reiniciar el daemon
- **AND** SHALL NOT crear un nuevo proceso `mcp-proxy`

#### Scenario: Segunda sync reutiliza daemon sin recrear

- **WHEN** `ai-specs sync` se ejecuta por segunda vez en el mismo proyecto
- **AND** el daemon del sync anterior sigue corriendo y sano
- **THEN** el número de procesos `mcp-proxy` activos SHALL permanecer en 1

---

### Requirement: Healthcheck via HTTP GET /status

El sistema SHALL comprobar la salud del daemon via `GET http://localhost:{port}/status`. Una respuesta HTTP 200 indica daemon sano. Cualquier otro resultado (error de conexión, timeout, código no-200) indica daemon no disponible.

#### Scenario: Respuesta 200 confirma daemon sano

- **WHEN** el sistema envía `GET http://localhost:{port}/status`
- **AND** el daemon responde con HTTP 200
- **THEN** `healthcheck()` SHALL retornar `True`

#### Scenario: Fallo de conexión indica daemon muerto

- **WHEN** el sistema envía `GET http://localhost:{port}/status`
- **AND** la conexión es rechazada o el timeout expira
- **THEN** `healthcheck()` SHALL retornar `False`

---

### Requirement: Detección y recuperación de daemon muerto

El sistema SHALL detectar un daemon no disponible (PID desaparecido O puerto no responde) y arrancarlo de nuevo con un puerto libre nuevo.

#### Scenario: PID inexistente → arranque nuevo

- **WHEN** `.ai-specs/run/proxy.pid` existe
- **AND** el proceso con ese PID ya no existe en el sistema operativo
- **THEN** el sistema SHALL marcar el daemon como muerto
- **AND** SHALL asignar un puerto libre nuevo
- **AND** SHALL iniciar un proceso `mcp-proxy` nuevo

#### Scenario: PID vivo pero puerto no responde → restart

- **WHEN** el proceso con el PID registrado existe
- **AND** `healthcheck()` retorna `False` (puerto no responde o timeout)
- **THEN** el sistema SHALL enviar SIGTERM al proceso existente
- **AND** SHALL iniciar un proceso `mcp-proxy` nuevo con puerto libre

---

### Requirement: Asignación dinámica de puerto

El sistema SHALL asignar un puerto libre en el rango alto (>= 49152) cuando no exista `.ai-specs/run/proxy.port` o cuando el daemon deba reiniciarse. El puerto asignado SHALL persistirse en `.ai-specs/run/proxy.port`.

#### Scenario: Primera sync asigna puerto libre

- **WHEN** `.ai-specs/run/proxy.port` no existe
- **THEN** el sistema SHALL encontrar un puerto libre disponible
- **AND** SHALL escribir el puerto en `.ai-specs/run/proxy.port`
- **AND** SHALL iniciar el daemon en ese puerto

#### Scenario: Syncs posteriores leen el puerto existente

- **WHEN** `.ai-specs/run/proxy.port` existe y el daemon está sano
- **THEN** el sistema SHALL leer el puerto desde ese archivo
- **AND** SHALL NO asignar un puerto nuevo

---

### Requirement: Identidad del daemon por raíz git

El daemon SHALL ser identificado por la raíz del repositorio git (`git rev-parse --show-toplevel`). Múltiples worktrees del mismo repositorio SHALL compartir el mismo daemon.

#### Scenario: Worktrees comparten daemon

- **WHEN** dos worktrees del mismo repositorio ejecutan `ai-specs sync` con MCPs shared
- **THEN** ambos SHALL usar el mismo proceso `mcp-proxy`
- **AND** el número de daemons activos para ese repositorio SHALL ser 1

#### Scenario: Repos diferentes tienen daemons independientes

- **WHEN** dos proyectos en distintas raíces git ejecutan `ai-specs sync` con MCPs shared
- **THEN** cada uno SHALL tener su propio proceso `mcp-proxy` independiente
- **AND** sus archivos de estado en `.ai-specs/run/` SHALL estar en paths distintos

---

### Requirement: Subcomando ai-specs daemon stop

El sistema SHALL proveer el subcomando `ai-specs daemon stop` que detiene el daemon de forma limpia mediante SIGTERM.

#### Scenario: Stop limpio mediante SIGTERM

- **WHEN** el usuario ejecuta `ai-specs daemon stop`
- **AND** el daemon está corriendo
- **THEN** el sistema SHALL enviar SIGTERM al proceso
- **AND** SHALL esperar a que el proceso termine
- **AND** SHALL eliminar los archivos `.ai-specs/run/proxy.{pid,port,named-config.json}`

#### Scenario: Stop cuando no hay daemon activo

- **WHEN** el usuario ejecuta `ai-specs daemon stop`
- **AND** no existe un daemon activo (PID no existe o archivos de estado ausentes)
- **THEN** el sistema SHALL emitir un mensaje informativo
- **AND** SHALL NOT fallar con error

---

### Requirement: Subcomando ai-specs daemon status

El sistema SHALL proveer el subcomando `ai-specs daemon status` que reporta el estado actual del daemon.

#### Scenario: Status de un daemon vivo

- **WHEN** el usuario ejecuta `ai-specs daemon status`
- **AND** el daemon está corriendo y sano
- **THEN** el sistema SHALL imprimir un bloque con `pid`, `port`, `uptime_s` y la lista de MCPs alojados
- **AND** SHALL terminar con exit code 0

#### Scenario: Status cuando no hay daemon activo

- **WHEN** el usuario ejecuta `ai-specs daemon status`
- **AND** no existe daemon vivo (PID no existe, port no responde, o state files ausentes)
- **THEN** el sistema SHALL imprimir un mensaje informativo
- **AND** SHALL terminar con exit code 1

---

### Requirement: Subcomando ai-specs daemon restart

El sistema SHALL proveer el subcomando `ai-specs daemon restart` que detiene el daemon actual (si existe) e inicia uno nuevo con la config vigente.

#### Scenario: Restart con daemon corriendo

- **WHEN** el usuario ejecuta `ai-specs daemon restart`
- **AND** el daemon está corriendo
- **THEN** el sistema SHALL enviar SIGTERM al proceso existente
- **AND** SHALL iniciar un nuevo proceso `mcp-proxy` con la config actual
- **AND** SHALL terminar con exit code 0 cuando el nuevo daemon esté sano

#### Scenario: Restart sin daemon previo

- **WHEN** el usuario ejecuta `ai-specs daemon restart`
- **AND** no existe daemon activo
- **THEN** el sistema SHALL iniciar un nuevo daemon (equivalente a `ensure_daemon`)
- **AND** SHALL terminar con exit code 0

---

### Requirement: Desvinculación del terminal (process detachment)

El daemon SHALL sobrevivir al cierre del terminal que lo inició. El proceso SHALL lanzarse desvinculado del grupo de procesos del shell (patrón nohup/setsid).

#### Scenario: Daemon sobrevive cierre de terminal

- **WHEN** el daemon es iniciado por `ensure_daemon()`
- **THEN** el proceso SHALL ejecutarse en un grupo de procesos independiente (setsid o equivalente)
- **AND** el cierre del terminal iniciador SHALL NOT terminar el daemon

---

### Requirement: Archivos de estado en .ai-specs/run/

El estado runtime del daemon SHALL persistirse en los archivos `.ai-specs/run/proxy.pid`, `.ai-specs/run/proxy.port`, y `.ai-specs/run/proxy.named-config.json`. Estos archivos SHALL estar incluidos en `.gitignore`.

#### Scenario: Archivos de estado escritos al iniciar

- **WHEN** el daemon arranca exitosamente
- **THEN** SHALL existir `.ai-specs/run/proxy.pid` con el PID del proceso
- **AND** SHALL existir `.ai-specs/run/proxy.port` con el número de puerto
- **AND** SHALL existir `.ai-specs/run/proxy.named-config.json` con la config del daemon

#### Scenario: Directorio .ai-specs/run/ ignorado por git

- **WHEN** se inspecciona el `.gitignore` del proyecto
- **THEN** el patrón `.ai-specs/run/` SHALL estar presente
- **AND** ningún archivo bajo `.ai-specs/run/` SHALL ser trackeado por git

---

### Requirement: TBD — comportamiento cuando uvx no está en PATH

> **Estado**: ABIERTO — pendiente de resolución en fase de diseño.
>
> Cuando se detecta al menos un MCP `shared` pero `uvx` no está disponible en `PATH`, existen tres comportamientos candidatos:
> 1. **fail-fast**: sync falla con error explícito.
> 2. **warn+degrade-to-stdio**: se emite advertencia y los MCPs `shared` se tratan como `stdio`.
> 3. **require doctor pass**: se exige que `ai-specs doctor` pase antes de permitir sync con MCPs shared.
>
> El comportamiento definitivo se resolverá en fase de diseño.
