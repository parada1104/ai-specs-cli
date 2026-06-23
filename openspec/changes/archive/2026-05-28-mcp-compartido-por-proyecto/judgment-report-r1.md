# Judgment Day Report — `mcp-compartido-por-proyecto`

**Fecha**: 2026-05-28
**Worktree**: `/Users/robert/proyectos/nnodes/ai-specs-cli/.worktrees/mcp-compartido-por-proyecto`
**Branch**: `feat/mcp-compartido-por-proyecto`
**Base**: `development`
**Estado**: Round 1 completo — pendiente decisión de scope para Fix Agent + re-judgment.

## Contexto

Cambio SDD que introduce un MCP daemon compartido entre worktrees del mismo proyecto (1 daemon por repo canónico, no por worktree). Diff: ~7039 líneas en 61 archivos, 26 commits sobre `development`.

Archivos clave:
- `lib/_internal/mcp-daemon.py` (NEW, 411 líneas) — lifecycle ensure/healthcheck/stop/status/restart
- `lib/_internal/recipe-materialize.py` — split shared/stdio + named-server-config
- `lib/_internal/mcp-render.py` — URL para Claude/Cursor/OpenCode/Pi; stdio para codex/gemini
- `lib/_internal/doctor.py` — checks daemon-uvx + daemon-running
- `lib/sync.sh`, `lib/daemon.sh`, `bin/ai-specs` — orquestación
- `templates/gitignore-root.tmpl` — agrega `.ai-specs/run/`
- `catalog/recipes/trello-mcp-workflow/recipe.toml` — `mode = "shared"`
- `docs/mcp-shared-daemon.md` (NEW)
- ~30 archivos de test bajo `tests/`
- Specs en `openspec/changes/mcp-compartido-por-proyecto/` + sync a `openspec/specs/`

Daemon identity: parent de `git --git-common-dir`. Lock+PID bajo `.ai-specs/run/`.

## Protocolo ejecutado

- Pattern 0: skill resolution → registry artifact removido (observación engram #544), inyectado bloque `## Project Standards (auto-resolved)` con runtime brief (strict TDD, fixtures, convenciones Bash/Python) en ambos prompts.
- Pattern 1: Judge A + Judge B lanzados en paralelo (Opus, modo ciego), mismo target, criterios complementarios.
- Pattern 2: synthesis hecha por orquestador (yo), sin re-review.

Ambos jueces reportaron `**Skill Resolution**: injected`.

## Veredicto Round 1

| # | Finding | Judge A | Judge B | Severity | Status |
|---|---------|---------|---------|----------|--------|
| 1 | Env-vars `$VAR`/`${VAR}` no se expanden en `proxy.named-config.json` → trello shared MCP autentica con literal `$TRELLO_TOKEN` (rompe la recipe flagship) | ✅ | ❌ | **CRITICAL** | **Confirmed** (validado 05-28) |
| 2 | `ensure_daemon` retorna port aún cuando `_wait_until_healthy` da False; `sync.sh` continúa con URL rota | ✅ | ❌ | **CRITICAL** | **Confirmed** (validado 05-28) |
| 3 | `init.sh` solo actualiza `.gitignore` con `--force` → proyectos existentes que activan shared no tienen `.ai-specs/run/` ignorado → secretos pueden ser `git add`-eados | ✅ | ✅ | WARNING (real) | **Confirmed** |
| 4 | Path uvx-missing borra `proxy.named-config.json` pero no detiene el daemon → orphan + estado inconsistente | ✅ | ✅ | WARNING (real) | **Confirmed** |
| 5 | `_sigterm_and_wait`: no escala a SIGKILL si SIGTERM se ignora, y solo señala el PID del proxy (no el process group) → hijos MCP huérfanos / daemon fantasma | ✅ | ✅ | WARNING (real) | **Confirmed** (combinado) |
| 6 | `doctor._check_daemon_running` solo hace healthcheck, no verifica `_is_pid_alive(pid)` → no distingue "no daemon" / "PID muerto" / "puerto squat por otro proceso" | ✅ | ✅ | WARNING (real) | **Confirmed** |
| 7 | Daemon identity vs git submodule/bare repo + `sync.sh` permite spawn no-git que `daemon.sh` rechaza (stop/status fallan) → fallback inconsistente | ✅ | ✅ | WARNING (real) | **Confirmed** |
| 8 | `read_mcp` lanza `ValueError` por mode inválido — no atrapado en `mcp-render.main()` ni `doctor` → traceback feo | ✅ | ❌ | WARNING (real) | Suspect A |
| 9 | `restart_daemon` libera el lock entre stop y start → race window con sync concurrente | ✅ | ❌ | WARNING (real) | Suspect A |
| 10 | `docs/mcp-shared-daemon.md` dice "secrets resolved" pero no lo hace (depende de #1) | ✅ | ❌ | WARNING (real) | Suspect A (atado a #1) |
| 11 | Error msg en `mcp-render.py:97` recomienda `ai-specs daemon ensure` — subcomando inexistente | ❌ | ✅ | WARNING (real) | Suspect B |
| 12 | Stale `proxy.named-config.json` no se borra cuando user remueve shared MCPs del manifest | ❌ | ✅ | WARNING (real) | Suspect B |
| 13 | `RECIPE_MCP_TEMP` sin `trap EXIT` → leak si `recipe-materialize` falla | ❌ | ✅ | WARNING (real) | Suspect B |
| 14 | `write_named_server_config` solo strip `mode`, deja `timeout`/`enabled`/`type`/etc. en JSON → mcp-proxy puede rechazar extras | ❌ | ✅ | WARNING (real) | Suspect B |
| 15 | `ai-specs daemon restart` no chequea `uvx` antes → traceback Python crudo si falta | ❌ | ✅ | WARNING (real) | Suspect B |
| 16 | Task 10.1 bullet 2 — test port-squat sin implementar (gap de cobertura) | ✅ | ❌ | WARNING (real, low) | Suspect A |
| 17 | `_spawn` no pasa `--host 127.0.0.1` → si upstream cambia default a `0.0.0.0` se expone LAN | ✅ | ❌ | WARNING (theoretical) | INFO |
| 18 | 3 writes atómicos separados (pid/port/hash) → SIGKILL entre ellos deja estado inconsistente | ✅ | ❌ | WARNING (theoretical) | INFO |
| 19 | Spec narrativa `mcp-shared-daemon/spec.md:106` dice `--show-toplevel`, código usa `--git-common-dir` | ❌ | ✅ | WARNING (theoretical) | INFO |
| 20 | `_acquire_lock` sin timeout → proceso colgado bloquea indef | ❌ | ✅ | WARNING (theoretical) | INFO |
| 21 | MCP merge logic duplicada (`materialize_recipes` inline vs `build_recipe_mcp`) → maintenance trap | ❌ | ✅ | WARNING (theoretical) | INFO |
| 22 | `_install_test_fakes` env-var monkey-patch limitado a `__main__` (importlib crea instancias separadas) | ✅ | ✅ | SUGGESTION | INFO |
| 23 | Test `test_daemon_concurrent_syncs.py` no cubre cross-worktree concurrente + docstring stale | ✅ | ✅ | SUGGESTION | INFO |
| 24 | `HEALTHCHECK_TIMEOUT`/`START_HEALTH_WAIT` no configurables | ✅ | ❌ | SUGGESTION | INFO |
| 25 | `proxy.log` sin rotación | ✅ | ❌ | SUGGESTION | INFO |
| 26 | uvx-missing heredoc duplica lógica de `recipe-materialize` | ✅ | ❌ | SUGGESTION | INFO |
| 27 | `os.fdopen` + manual close pattern sloppy en `write_named_server_config` | ❌ | ✅ | SUGGESTION | INFO |

### Resumen numérico
- **Confirmados** (ambos jueces): **5** distintos, todos WARNING (real). Items #3–#7.
- **Suspect A**: 7 (2 CRITICAL #1 #2, 4 WARNING real #8 #9 #10 #16, sin contar #10 que depende de #1).
- **Suspect B**: 5 WARNING (real) #11–#15.
- **INFO** (theoretical/suggestion, no se arreglan, solo se reportan): 11 — items #17–#27.
- **Contradicciones**: 0. Ángulos complementarios.

### Validación de CRITICAL (2026-05-28)

Ambos CRITICAL de Judge A fueron verificados leyendo el código directamente (no review, verificación de claim):

- **#1 CONFIRMADO**: `recipe-materialize.py:419` solo hace `{k: v for k, v in cfg.items() if k != "mode"}` — no llama `os.path.expandvars`. `mcp-daemon.py:_spawn` (192-211) ejecuta `uvx mcp-proxy --named-server-config` sin pasar `env=`. No hay expansión en ningún punto del path. El docstring de `write_named_server_config` ("expansion happens at daemon-spawn time") es **falso**.
- **#2 CONFIRMADO**: `mcp-daemon.py:246-247` llama `_wait_until_healthy(new_port)` sin capturar el retorno y hace `return new_port` incondicional.

Conteo actualizado: **7 confirmados** (2 CRITICAL + 5 WARNING real #3–#7).

## Detalle de findings críticos (de Judge A — requieren verificación)

### #1 — CRITICAL — Env-vars no se expanden en `proxy.named-config.json`

**Archivos**: `lib/_internal/recipe-materialize.py:404-441`, `tests/test_recipe_materialize_named_config.py:59-77`, `tests/test_trello_recipe_shared.py:76-77`.

Judge A verificó leyendo el upstream de `mcp-proxy` en `~/.cache/uv/.../mcp_proxy/config_loader.py:87-93`: el `env` dict del JSON se mergea tal cual en `StdioServerParameters`, sin shell expansion. Resultado: el child stdio MCP (e.g., `@delorenj/mcp-server-trello`) recibe `TRELLO_TOKEN=$TRELLO_TOKEN` literal y la auth falla silenciosamente. El design dice "Variables de entorno (secrets) se resuelven al spawn del daemon desde el shell del usuario" pero esa resolución no está implementada. Los tests usan `INNER_MCP_TIME` (sin env), tapando la regresión. Peor: `test_env_var_references_preserved_literally` cementa el bug.

**Fix sugerido**: o expandir env values con `os.path.expandvars` en `write_named_server_config` antes de serializar (y documentar que la named-config contiene secretos resueltos — ya es `0o600`), o documentar la preservación literal y exigir que los users exporten los secretos para que mcp-proxy los herede vía `--pass-environment` (en cuyo caso el JSON no debería listar el `env` map para esas keys).

### #2 — CRITICAL — `ensure_daemon` ignora el fail de healthcheck

**Archivo**: `lib/_internal/mcp-daemon.py:223-247`.

Judge A: `ensure_daemon` retorna el port nuevo unconditionalmente después de spawnear, incluso cuando `_wait_until_healthy` devuelve False. El caller (`sync.sh`) solo chequea el exit code (0) y procede al fan-out. `mcp-render.py` escribe configs por agente apuntando a un port que el daemon puede no estar sirviendo. El user ve sync exitoso, todos los agentes reciben URL rota. La check `daemon-running` de doctor solo lo detecta en la *siguiente* invocación. Comentario en línea 244 dice "do NOT block forever" pero no distingue "test stub returns False" de "real production daemon failed".

**Fix sugerido**: cuando `_wait_until_healthy` devuelve False, raise (o return -1) para que `sync.sh` aborte antes del fan-out; gate del silent-pass behind un env var test-only.

## Fix Agent + verificación de tests (2026-05-28)

Fix Agent (sonnet, opción A) aplicó los 16 findings #1–#16 + 16 tests nuevos. Reportó "380 tests, 6 failures, pre-existentes, 0 regressions".

**Verificación del claim de los 6 fallos — diagnóstico final: los 6 son 100% staleness de rama, NO bugs.** `development` está completamente verde.

| Fallos | Causa raíz | Estado en development |
|--------|-----------|----------------------|
| 4× `test_sync_pipeline` (AGENTS.md sale stub vacío; subrepos sin AGENTS.md) | Falta `1619f8a fix(sync): render AGENTS.md with project + MCP config in sync pipeline (#53)`. Sin #53 el fan-out nunca renderiza contenido de AGENTS.md (solo el symlink CLAUDE.md→AGENTS.md). Sync sale EXIT=0 igual. | ✅ verde |
| 2× `test_readme_references_testing_and_validate` (needles 'Testing foundation exists', 'skill-sync') | El `README.md` de la rama del change está 7 líneas atrás de development; el test lee `ROOT/README.md`. | ✅ verde |

- El Fix Agent NO introdujo regresiones (stash de sus cambios → mismos 6 fallan sobre la rama commiteada). ✅
- GOTCHA de verificación: `git checkout development -- lib bin tests …` en un worktree NO trae `README.md` si no se incluye → da falso positivo de fallo (me pasó: atribuí 2 fallos a "pre-existentes no relacionados" cuando eran staleness del README). La verificación correcta es correr la suite en `development` mismo.

**La rama del change está 3 commits detrás de `development`**: `1619f8a` (#53), `60e27b0` (trello board), `9ba6d68` (#51 Pi target).

**Consecuencia para Round 2**: #53 toca el render de AGENTS.md *con MCP config en el pipeline de sync*, que se solapa con el rework MCP shared/stdio de este change. Antes del Round 2 hay que **mergear/rebasear `development` en la rama del change** para (a) resolver los 6 fallos y (b) validar la interacción real #53 ↔ daemon/named-config. Round 2 sobre la base stale no probaría esa integración.

## Próximos pasos (para retomar en otra sesión)

1. **HECHO** — Validar los 2 CRITICAL (#1, #2): confirmados por lectura directa (ver sección "Validación de CRITICAL").
2. **HECHO** — Fix Agent opción A: 16 findings #1–#16 aplicados + 16 tests nuevos.
3. **PENDIENTE** — Mergear/rebasear `development` en `feat/mcp-compartido-por-proyecto`, resolver conflictos (foco: #53 render AGENTS.md ↔ MCP rendering del change). Correr `./tests/run.sh` — esperado: 0 fallos.
4. **PENDIENTE** — Round 2: re-lanzar Judge A + Judge B en paralelo (modo ciego) sobre la rama ya actualizada. APROBADO si 0 CRITICAL + 0 WARNING real confirmados.
5. **No mergear ni pushear** hasta `JUDGMENT: APPROVED ✅`.

3. **Re-judgment Round 2** después del Fix Agent. Re-lanzar Judge A + Judge B en paralelo (modo ciego, prompt similar pero apuntando solo al diff posterior al fix). APROBADO si 0 CRITICAL confirmados + 0 WARNING real confirmados (theoretical y suggestions pueden quedar).

4. **No mergear ni pushear** hasta `JUDGMENT: APPROVED ✅`. Per blocking rules del skill: no `git push` ni `git commit` después de fixes hasta que el re-judgment complete.

## Archivos a leer para retomar contexto

- Este reporte: `judgment-report.md` (worktree root)
- Engram: `mem_search(query: "mcp-compartido-por-proyecto judgment", project: "ai-specs-cli")` — los jueces guardaron findings significativos
- Specs del cambio: `openspec/changes/mcp-compartido-por-proyecto/{proposal,design,tasks}.md`
- Implementación: `lib/_internal/mcp-daemon.py`, `lib/_internal/recipe-materialize.py`, `lib/_internal/mcp-render.py`, `lib/sync.sh`, `lib/daemon.sh`, `bin/ai-specs`

## Para reanudar la sesión

Comando sugerido al volver:

```
leé judgment-report.md en el worktree de mcp-compartido-por-proyecto y retomemos donde quedamos
```

El próximo asistente debería: (1) leer este reporte, (2) verificar los 2 CRITICAL, (3) preguntar al user qué scope de fix prefiere, (4) delegar Fix Agent, (5) re-lanzar jueces en paralelo para Round 2.
