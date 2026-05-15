## Why

Hoy el flujo SDD vive como prompt prosa dentro del orquestador primario: una sola superficie de ejecución asume "fase = prompt detallado", y el harness no recibe agentes nativos por fase. Eso impide acotar tools, presupuestos de turnos y permisos por fase (explore read-only, proposal con git, apply con tests, archive con `gh`), obliga al orquestador a recordar el contrato de cada fase en cada ciclo y bloquea cualquier evolución hacia recipes que definan políticas finas por fase. El ticket #82 cerró el debate de diseño: queremos subagentes especializados como **feature de producto** del CLI, opt-in vía manifiesto, distribuibles a cualquier proyecto que ejecute `ai-specs init` + `ai-specs sync`.

## What Changes

- Extender el contrato canónico de `ai-specs.toml` con `[sdd].sub_agents` (booleano, **default `false`**, opcional, **backward compatible**).
- Empaquetar en el CLI seis archivos bundled-agents para Claude Code bajo `bundled-agents/claude/sdd-*.md` en la raíz del repo CLI (junto a `bundled-skills/` y `bundled-commands/`) — `sdd-explore`, `sdd-proposal`, `sdd-artifacts`, `sdd-apply`, `sdd-verify`, `sdd-archive`, cada uno con frontmatter canónico de Claude Code (`name`, `description`, `tools`, `model`) y cuerpo que codifica rol, tools permitidos/bloqueados y budget de turnos.
- Introducir un nuevo renderer `lib/_internal/agents-render.py` que, cuando `sub_agents = true`, materializa los archivos en `.claude/agents/` (Claude Code) y queda preparado para fallback inline en `opencode`/`cursor` (orquestador ejecuta fases sin subagentes nativos).
- Integrar el renderer en el sync-agent existente respetando la pipeline `recipe-sync-materialization` (orden, idempotencia, sidecars `.new`, `.ai-specs.lock`).
- Extender `lib/_internal/agents-md-render.py` para que el runtime brief liste subagentes activos cuando `[sdd].sub_agents = true`.
- Actualizar la skill `openspec-phase-orchestrator` para referenciar los `subagent_type` (`sdd-explore`, `sdd-proposal`, …) y describir el contrato de handoff por fase.
- Actualizar `templates/ai-specs.toml.tmpl` para incluir `[sdd] sub_agents = false` comentado con guía.
- Publicar un contrato dedicado para frontmatter de subagent files (separado de `skill-frontmatter-contract`, que cubre skills, no agentes).
- Garantizar **backward compatibility estricta**: sin `sub_agents = true`, el SDD se comporta exactamente como hoy (sin escrituras nuevas, sin agentes nativos materializados, sin ruido en doctor ni en runtime brief).
- Tests de regresión en `test_sync_pipeline.py` para la rama OFF y tests nuevos para la rama ON (despliegue, idempotencia, fallback por harness).

No hay BREAKING changes: el comportamiento default preserva el contrato V1.

## Capabilities

### New Capabilities
- `sdd-subagent-deployment`: contrato del despliegue de subagentes SDD por harness. Define los seis roles, el frontmatter canónico de subagent files, el modo de distribución según harness habilitado (nativo en Claude Code, fallback inline para OpenCode/Cursor), y las garantías de idempotencia/limpieza cuando el flag se desactiva.

### Modified Capabilities
- `manifest-contract`: añade `[sdd].sub_agents` (booleano, opcional, default `false`) al conjunto canónico V1; valida tipo y rechaza valores no booleanos; la ausencia del campo MUST seguir siendo válida.
- `sdd-cli-integration`: documenta `sub_agents` como flag declarativo del proveedor SDD; cuando es `true`, el comando de habilitación/sync MUST coordinar el despliegue, y cuando es `false`/ausente, MUST NOT escribir agent files ni alterar harnesses.
- `recipe-sync-materialization`: agrega el orden y la regla de materialización de subagent files por harness habilitado, preservando la regla de `ai-specs/skills/` intocable y la idempotencia de re-sync.
- `agents-md-runtime-brief`: añade una sub-sección opt-in en el runtime brief que enumera los subagentes activos solo cuando `[sdd].sub_agents = true`; ausencia del flag MUST producir runtime brief idéntico al actual (byte-identical en idempotencia).
- `backward-compatibility`: cubre `[sdd]` y `[sdd].sub_agents` como adiciones opt-in cuya ausencia preserva V1 y cuya presencia con valor `false` también preserva V1.

## Impact

- **Manifiesto y validación**: `ai-specs/ai-specs.toml` (schema), `lib/_internal/toml-read.py` (parser), validador V1, mensajes de error.
- **Renderers**: `lib/_internal/agents-render.py` (nuevo, registra subagent files por harness), `lib/_internal/agents-md-render.py` (extensión condicional del runtime brief).
- **Sync-agent**: integración en `lib/sync.sh` o helpers Python invocados; respeto a `.ai-specs.lock` y sidecars `.new`.
- **Bundled assets**: seis archivos nuevos en `bundled-agents/claude/` a la raíz del CLI (junto a `bundled-skills/`, `bundled-commands/`), plus `ai-specs/contracts/subagent-frontmatter.md` como documento humano.
- **Templates**: `templates/ai-specs.toml.tmpl` incluye `[sdd]` comentado.
- **Skills**: `ai-specs/skills/openspec-phase-orchestrator/SKILL.md` referencia los IDs de subagentes y documenta el contrato de handoff por fase.
- **Doctor**: chequeo no destructivo de coherencia entre `[sdd].sub_agents = true` y presencia de `.claude/agents/sdd-*.md` cuando Claude Code está en `[agents].enabled`.
- **Tests**: regresión OFF (`test_sync_pipeline.py`), nuevos tests para parser, renderer y fallback por harness.
- **Dependencias previas**: este cambio depende lógicamente del trabajo ya cerrado en cards #65 (runtime brief), #66 (recipe init prompts) y #67 (SDD adaptativo) — todas en `REVIEW`/cerradas en el board.
- **Rollback**: revertir el commit que introduce el renderer + schema; `ai-specs/ai-specs.toml` puede simplemente omitir `[sdd].sub_agents` y los archivos en `.claude/agents/sdd-*.md` pueden eliminarse manualmente. Como el default es `false`, ningún proyecto consumidor recibe el comportamiento nuevo hasta optar; el rollback no rompe manifests existentes.
