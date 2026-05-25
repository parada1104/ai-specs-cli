# Archive Report — mcp-compartido-por-proyecto

**Date archived**: 2026-05-25
**Branch**: `feat/mcp-compartido-por-proyecto`
**Final commit**: `1ad0b89`
**Trello card**: https://trello.com/c/fwGUCRQk
**Verify verdict**: PASS_WITH_WARNINGS (one CRITICAL round closed by worktree-identity remediation; one informational warning explicitly accepted)

## Summary

Multiplexa los MCPs marcados como `mode = "shared"` en un único proceso `mcp-proxy` por raíz git, exponiendo cada server como Streamable HTTP en `http://localhost:{PORT}/servers/{name}/mcp`. Los agentes HTTP (Claude, Cursor, OpenCode) reciben entradas `url`; Codex y Gemini siguen renderizando `command`/`args`/`env` (stdio). Reduce la inflación N×M (N agentes × M worktrees) a 1 daemon por repositorio, comparte rate limits entre worktrees del mismo repo y elimina cold-starts repetidos. El cambio es aditivo: MCPs sin `mode` (o `mode = "stdio"`) se comportan idénticos al pre-change.

## Capabilities

**New (3)**
- `mcp-shared-daemon` — ciclo de vida del daemon (startup eager, idempotencia, healthcheck `/status`, recuperación de dead PID, asignación dinámica de puerto, identidad por raíz git compartida entre worktrees, subcomandos `daemon stop|status|restart`, detachment via setsid, state files en `.ai-specs/run/`, degradación híbrida cuando `uvx` falta — fail-fast en doctor, WARN+stdio-fallback en sync).
- `mcp-mode-shared` — semántica del campo `mode`, validación de enum (`"shared"` | `"stdio"`), ausencia equivale a stdio (sin breaking change), precedencia manifest sobre recipe.
- `mcp-named-config-materialization` — writer de `.ai-specs/run/proxy.named-config.json` (shape compatible con mcp-proxy, preservación literal de referencias `$VAR`/`${VAR}`, restart controlado del daemon cuando el SHA-256 del JSON cambia).

**Modified (5)**
- `mcp-env-rendering` — agentes HTTP emiten `url`-type para shared, Codex/Gemini fallback a stdio, puerto leído de `proxy.port` en tiempo de render (no de materialize).
- `recipe-schema` — campo opcional `mode` en `[[provides.mcp]]`.
- `recipe-manifest-contract` — campo opcional `mode` en `[mcp.<name>]`.
- `mcp-preset-merge` — `mode` participa en el shallow merge con precedencia manifest.
- `sync-hooks` — paso "ensure mcp-proxy daemon" insertado antes del fan-out cuando hay al menos un MCP shared.

## Implementation

- **NEW** `lib/_internal/mcp-daemon.py` (~360 LOC, stdlib only) — `ensure_daemon`, `healthcheck`, `stop_daemon`, `status_daemon`, `restart_daemon`. Identidad resuelta via `git rev-parse --path-format=absolute --git-common-dir` para que worktrees compartan estado.
- **NEW** `lib/daemon.sh` — wrapper bash que despacha a los subcomandos de Python.
- **NEW** `docs/mcp-shared-daemon.md` — guía usuario final.
- **Modified** `lib/_internal/recipe-materialize.py` — split shared vs stdio + escribe `proxy.named-config.json` con SHA-256 diff para decidir restart.
- **Modified** `lib/_internal/mcp-render.py` — branch `url` para shared en agentes HTTP, lee `.ai-specs/run/proxy.port` al renderizar.
- **Modified** `lib/_internal/recipe_schema.py` + `lib/_internal/toml-read.py` — validación de enum `mode` (constante duplicada con cross-ref comment).
- **Modified** `lib/_internal/doctor.py` — checks `daemon-uvx` (ERROR cuando falta `uvx` y hay shared MCPs) y `daemon-running` (WARN).
- **Modified** `lib/sync.sh` — paso `ensure mcp-proxy daemon` antes del fan-out; degradación WARN+stdio cuando `uvx` ausente; `GIT_ROOT` computado una vez via `--git-common-dir`.
- **Modified** `bin/ai-specs` — dispatch `ai-specs daemon {stop|status|restart}`.
- **Modified** `catalog/recipes/trello-mcp-workflow/recipe.toml` — declara `mode = "shared"` para el preset de Trello.
- **Modified** `templates/gitignore-root.tmpl` — añade `.ai-specs/run/` para evitar trackear el state del daemon.

## Tests

- **368 tests** en `tests/run.sh` (era 265 pre-change; +103 nuevos repartidos en 13 archivos cubriendo schemas, merge, lifecycle del daemon, render, orquestación de sync, doctor, degradación, recipe Trello, integración end-to-end con `mcp-proxy` real, edge cases y la prueba de identidad cross-worktree).
- **6 baseline failures** pre-existentes inalteradas (4 × `test_sync_pipeline` por AGENTS.md fan-out + secret redaction; 2 × `test_testing_foundation_skill` por README needles). **0 regresiones, 0 fixes accidentales del baseline.**

## Bugs found during apply

1. `restart_daemon` eliminaba el `proxy.named-config.json` que necesitaba para spawnear el reemplazo → `FileNotFoundError`. Fix en commit `829d738`.
2. `test_dispatch_to_daemon_restart` declaraba assertions fuera del scope de `tempfile.TemporaryDirectory()` → false negative oculto. Corregido durante recovery de Wave 2.
3. **worktree-identity-gap** (detectado en verify CRITICAL round): la implementación inicial usaba `git rev-parse --show-toplevel`, lo que producía un daemon distinto por worktree. Remediado convirtiendo 5 callsites en `lib/_internal/recipe-materialize.py`, `lib/_internal/doctor.py`, `lib/_internal/mcp-render.py`, `lib/daemon.sh` y `lib/sync.sh` a `git rev-parse --path-format=absolute --git-common-dir` con `Path.parent`, más el nuevo test `tests/test_daemon_worktree_pair.py` (commits `7809c7d` red + `1ad0b89` green).

## Deferred (3 G10 bullets, justificados inline en tasks.md)

- **10.1 port stale (someone else owns the port)** — race de port-grabbing fuera del scope de tests de unidad; documentado como deferred a QA manual.
- **10.2 alternative branches del documenting auto-restart path** — N/A tras resolverse Q1 (mcp-proxy no auto-restartea hijos crasheados; recovery via `ai-specs daemon restart`).
- **10.3 alternative "only alive" branch para status** — N/A: `status_daemon` retorna metadata enriquecida (`api_last_activity`, `server_instances`) además de `{pid, port, uptime_s}`, por lo que la rama "alive sin metadata" no existe en la implementación final.

## Commits

```
1ad0b89 fix(mcp-shared): resolve daemon identity via --git-common-dir so worktrees share one daemon
7809c7d test(mcp-shared): add worktree-pair integration test for shared daemon identity
91a2512 feat(mcp-shared): finalize Group 9 integration suite (tasks.md flips)
7633be5 test(mcp-shared): add Group 9 red integration tests for daemon end-to-end + idempotency + concurrent + precedence + stop
7e07280 feat(mcp-shared): close Q1/Q2 + document shared daemon in docs/
70914d9 test(mcp-shared): add Group 10 edge case tests (dead pid recovery + zero shared MCPs + /status metadata)
2572ea6 feat(mcp-shared): sync.sh degrades to stdio with WARN when uvx absent
7df45e8 test(mcp-shared): add Group 7 red tests for sync degradation when uvx missing
baeacbc feat(mcp-shared): trello recipe mode = "shared" + .ai-specs/run/ in gitignore template
e6e430d test(mcp-shared): add Group 8 red tests for trello recipe shared + gitignore .ai-specs/run/
4d1f4bc feat(mcp-shared): sync.sh ensure-daemon step + ai-specs daemon stop/status/restart
e2e5ab2 feat(mcp-shared): render shared MCPs as url for HTTP agents, stdio for codex/gemini
829d738 fix(mcp-shared): preserve named-config in restart_daemon
15ad208 feat(mcp-shared): doctor daemon-uvx ERROR + daemon-running WARN checks
d1eea00 test(mcp-shared): add Group 6 red tests for doctor daemon-uvx + daemon-running checks
504ffa9 test(mcp-shared): add Group 5 red tests for sync ensure-daemon + daemon dispatch
d6955fd test(mcp-shared): add Group 4 red tests for url rendering of shared MCPs
a1781b8 feat(mcp-shared): mcp-daemon.py module — ensure/healthcheck/stop/status/restart
1eda2da feat(mcp-shared): split shared/stdio MCPs + write named-server-config in materialize
ff79185 test(mcp-shared): add Group 2 red tests for split + named-config + materialize integration
5cbb48a test(mcp-shared): add Group 3 red tests for daemon lifecycle
21e2f34 feat(mcp-shared): validate mode enum in recipe & manifest schemas
32a6b79 test(mcp-shared): add Group 1 red tests for mode enum validation
89ef386 plan(sdd): close TBD-uvx, sharpen restart semantics, annotate Group 3->4 dep
4ab9c3e plan(sdd): rescue mcp-compartido-por-proyecto from Air worktree
```

## Verify outcome

**PASS_WITH_WARNINGS** after one CRITICAL round (worktree-identity-gap) cerrado por la remediación TDD (`7809c7d` + `1ad0b89`). El warning residual es informacional: `VALID_MCP_MODES` está duplicada en `recipe_schema.py` y `toml-read.py` con cross-ref comment — trade-off aceptado (la indirección importlib para 2 strings cuesta más complejidad de la que ahorra). Opcional follow-up: 1-line equality test en CI para detectar divergencia futura.

## Engram cross-references

- Proposal: topic `sdd/mcp-compartido-por-proyecto/proposal` (#639)
- Design: topic `sdd/mcp-compartido-por-proyecto/design` (#641)
- Apply-progress: topic `sdd/mcp-compartido-por-proyecto/apply-progress` (#645)
- Verify-report: topic `sdd/mcp-compartido-por-proyecto/verify-report` (#655)
- Trello card observation: topic `sdd/mcp-compartido-por-proyecto/trello-card` (#644)
