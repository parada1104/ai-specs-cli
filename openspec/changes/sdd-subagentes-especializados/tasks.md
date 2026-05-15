## 1. Infrastructure — Schema y bundled assets

- [x] 1.1 Crear `bundled-agents/claude/` en la raíz del CLI (junto a `bundled-skills/` y `bundled-commands/`) y los seis archivos `sdd-explore.md`, `sdd-proposal.md`, `sdd-artifacts.md`, `sdd-apply.md`, `sdd-verify.md`, `sdd-archive.md` con frontmatter canónico (`name`, `description`, `tools`, `model` opcional) y cuerpo que describa rol, tools permitidos/bloqueados y budget de turnos
- [x] 1.2 Crear `ai-specs/contracts/subagent-frontmatter.md` documentando campos requeridos, opcionales, regla de "derivado, no editado a mano" y separación explícita del contrato `skill-frontmatter-contract`
- [x] 1.3 Extender `templates/ai-specs.toml.tmpl` con un bloque `[sdd]` comentado que incluya `sub_agents = false` y un breve comentario que explique el efecto
- [x] 1.4 Documentar `[sdd].sub_agents` en el README del manifiesto (sección de campos opcionales) con default, semántica y advertencia de feature de producto

## 2. Implementation — Manifest parser y renderer

- [x] 2.1 Extender `lib/_internal/toml-read.py` para reconocer `[sdd].sub_agents` como booleano opcional con default `false`; validar tipo y emitir error explícito si el valor no es booleano
- [x] 2.2 Crear `lib/_internal/agents-render.py` con: lectura de manifiesto, evaluación de `sub_agents` + `[agents].enabled`, materialización a `.claude/agents/sdd-*.md` para Claude Code, registro de fallback inline para harnesses no soportados, e integración con `.ai-specs.lock` y sidecars `.new`
- [x] 2.3 Implementar en `agents-render.py` la detección de archivos huérfanos cuando `sub_agents` está apagado pero existen `.claude/agents/sdd-*.md` previos, y emitir aviso sin eliminar
- [x] 2.4 Reutilizar utilidades de hashing/normalización de `refresh-bundled.py` desde `agents-render.py` vía import; documentar la dependencia en el módulo nuevo
- [x] 2.5 Modificar `lib/sync.sh` para invocar `agents-render.py` después de `refresh-bundled` y antes de la generación de `AGENTS.md`; capturar exit codes y errores
- [x] 2.6 Extender `lib/_internal/agents-md-render.py` para añadir, cuando `sub_agents = true` y al menos un harness soportado está habilitado, una sección que liste subagentes activos por `name` y `description` con referencia a la ubicación canónica; preservar marker manual y idempotencia byte-identical

## 3. Implementation — Doctor y skill orchestrator

- [x] 3.1 Extender `lib/_internal/doctor.py` para comprobar coherencia entre `[sdd].sub_agents = true` y presencia de `.claude/agents/sdd-*.md` cuando `claude` está en `[agents].enabled`; emitir severidades `OK`/`WARN`/`ERROR` según contrato existente
- [x] 3.2 Actualizar la skill `ai-specs/skills/openspec-phase-orchestrator/SKILL.md` para referenciar los IDs `sdd-explore`, `sdd-proposal`, `sdd-artifacts`, `sdd-apply`, `sdd-verify`, `sdd-archive` como subagent_type cuando el feature está activo; documentar contrato de handoff por fase y fallback inline

## 4. Testing — Regresión y nueva cobertura

- [x] 4.1 Añadir tests en `tests/test_toml_read.py` (o módulo equivalente) para parser de `[sdd].sub_agents`: ausente, `true`, `false`, valor no booleano
- [x] 4.2 Crear `tests/test_agents_render.py` con cobertura: rama OFF idempotente, rama ON con `claude` único harness, rama ON con harnesses mixtos (`claude` + `opencode`), rama ON con solo `opencode`, transición ON→OFF (huérfanos detectados, aviso emitido, archivos no eliminados), idempotencia (dos corridas producen mismo lock y mismos archivos), archivo editado a mano genera sidecar `.new`
- [x] 4.3 Añadir test de regresión en `tests/test_sync_pipeline.py` que verifique `AGENTS.md` byte-idéntico antes y después de este cambio cuando `sub_agents` está ausente
- [x] 4.4 Añadir test en `tests/test_sync_pipeline.py` para la rama ON: `AGENTS.md` incluye la sección de subagentes con los seis nombres y declaración de fallback cuando aplica
- [x] 4.5 Añadir test para doctor que cubra: `sub_agents` ausente no emite chequeo; `sub_agents = true` con archivos presentes reporta `OK`; archivos faltantes reportan `WARN` o `ERROR`

## 5. Validation — Cierre del ciclo

- [ ] 5.1 Ejecutar `./tests/run.sh` y dejar evidencia (logs o resumen) de paso de todos los tests focalizados
- [ ] 5.2 Ejecutar `./tests/validate.sh` y confirmar `py_compile` + `bash -n` + tests completos
- [ ] 5.3 Correr `openspec validate sdd-subagentes-especializados` y dejar evidencia del resultado
- [ ] 5.4 Verificar manualmente el flujo end-to-end: en un proyecto consumidor (puede ser este mismo repo en un worktree temporal), activar `sub_agents = true`, correr sync, confirmar `.claude/agents/sdd-*.md` presentes con frontmatter válido, confirmar runtime brief actualizado, después desactivar, confirmar aviso de huérfanos
- [ ] 5.5 Producir `verify-report.md` resumiendo: evidencia de cada escenario de specs, lista de archivos modificados, comandos ejecutados, decisiones finales sobre open questions del design.md
