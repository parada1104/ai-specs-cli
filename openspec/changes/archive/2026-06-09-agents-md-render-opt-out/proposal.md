## Why

Desde v0.11.0, `ai-specs sync` e `init` regeneran `AGENTS.md` en cada corrida: campos estructurados de `resolved-config.json`, prosa de `[brief]`, y fragmentos `[provides.brief]` de recipes habilitadas. Es el default correcto para proyectos nuevos.

Para adoptantes que migraron con `/rules-audit` o curan el brief a mano, el único escape hatch hoy es **file-level**: el marker HTML `<!-- ai-specs:runtime-brief -->` en `AGENTS.md`. Funciona, pero es opaco, vive fuera del manifest, y no modela la intención del proyecto en `ai-specs.toml`.

**Gap (Trello #18):** falta un flag explícito en el manifest para decir "sync, no toques `AGENTS.md`" — ni `[brief]`, ni fragments de recipes.

## What Changes

- **Nuevo campo `[brief].render`** (boolean, default `true`). Cuando `false`, la pipeline **no escribe** `AGENTS.md` en sync/init/subrepos.
- **Skip en shell callers** (`sync.sh`, `init.sh`, `sync-agent.sh`) *antes* de invocar `agents-render.py` — el renderer no gana lógica nueva de flag.
- **Init con `render = false`**: si no existe `AGENTS.md`, escribir placeholder de una línea (mismo fallback que hoy); si ya existe, nunca overwrite.
- **Subrepos**: el flag del manifest **root** propaga a targets derivados (`sync-agent.sh`).
- **`doctor`**: INFO cuando render off; WARN si recipes tienen `[provides.brief]` pero render off; ERROR si falta `AGENTS.md`.
- **Docs**: `docs/ai-specs-toml.md` § `[brief]` — documentar `render`, precedencia vs marker, relación con subrepos.

**Sin cambios:** skills, MCP presets, hooks, `recipe-materialize`, symlinks de harness (`CLAUDE.md` → `AGENTS.md`). El marker HTML **se mantiene** como escape hatch file-managed.

## Approach (decisiones cerradas)

1. **Nombre y ubicación (Exploración D1)**: `[brief] render = false`. Default `true` (omitir key = comportamiento actual). Solo `false` explícito desactiva.

2. **Dónde enforzar (D2)**: guard en `sync.sh`, `init.sh`, `sync-agent.sh` (`ensure_target_workspace`). `agents-render.py` sigue siendo compose-and-write puro.

3. **Precedencia (D3)**:
   ```
   1. [brief].render = false  → skip total (sin merge de fragments ni [brief])
   2. <!-- ai-specs:runtime-brief -->  → skip (cuando render=true)
   3. Render normal
   ```

4. **Subrepos (D4)**: flag del root manifest aplica a todos los subrepo targets. Sin override per-subrepo en V1.

5. **Init (D5)**: placeholder `# AGENTS.md - Runtime context` si falta archivo; stderr explicativo. `init --force` no pisa brief manual existente.

6. **Symlinks harness (D6)**: se crean si `AGENTS.md` existe. `.omp/AGENTS.md` (card #11) fuera de scope.

7. **Doctor (D7)**: checks listados en exploración — ver Impact.

8. **Observabilidad (D8)**:
   ```
   ▸ agents-render (root)
     · skipped AGENTS.md (brief.render = false)
   ```

**Helper compartido** (recomendado en design):
```python
def brief_render_enabled(manifest: dict) -> bool:
    brief = manifest.get("brief") or {}
    return brief.get("render", True) is not False
```

Valores no-booleanos → doctor ERROR (alineado con footgun `True`/`False` de card #16).

### Flagged alternatives

- **`[project] agents_md_managed`**: rechazado — lejos de la docs de `[brief]`.
- **Top-level `[runtime_brief]`**: rechazado — superficie extra sin beneficio.
- **Lógica del flag dentro de `agents-render.py`**: rechazado — los callers ya orquestan preserve-marker; mismo patrón para render-off.
- **Eliminar marker HTML**: rechazado — non-goal explícito de `runtime-brief-rendering` spec.

## Before / After

**Manifest adoptante post-migración `/rules-audit`:**
```toml
[brief]
render = false
# intro/purpose/... pueden quedar como documentación; no afectan AGENTS.md
```

**`ai-specs sync` stdout — antes:**
```
▸ agents-render (root)
  (AGENTS.md regenerado con fragments + [brief])
```

**después (`render = false`):**
```
▸ agents-render (root)
  · skipped AGENTS.md (brief.render = false)
```

**`AGENTS.md` en disco:** byte-identical al sync anterior (o placeholder en init fresco).

## Out of Scope

- Desactivar skills, MCPs, hooks, o `recipe-materialize` cuando `render = false`.
- Quitar el marker `<!-- ai-specs:runtime-brief -->`.
- Opt-out para mirrors por harness (`.omp/AGENTS.md`, card #11).
- `ai-specs sync --force-brief` CLI override (candidato futuro).
- Cambiar cómo los harness **leen** `AGENTS.md` — solo política de **escritura**.

## Capabilities

### New Capabilities

- _(ninguna — extensión del contrato manifest + rendering existente)_

### Modified Capabilities

- **`runtime-brief-rendering`**: nuevo requirement para `[brief].render = false` — skip en sync/init/subrepos; precedencia flag > marker; init placeholder; observabilidad stdout/stderr.
- **`recipe-manifest-contract`**: nuevo campo opcional `[brief].render` (boolean, default true); semántica cuando false (prose y fragments no se emiten a disco).

## Impact

- **Specs (delta targets)**:
  - `openspec/specs/runtime-brief-rendering/spec.md` (modified)
  - `openspec/specs/recipe-manifest-contract/spec.md` (modified)
- **Code**:
  - `lib/sync.sh` — guard antes de `agents-render.py`
  - `lib/init.sh` — guard + placeholder path
  - `lib/sync-agent.sh` — guard en `ensure_target_workspace()`
  - `lib/_internal/doctor.py` — checks nuevos
  - Helper Python compartido (módulo TBD en design — `toml-read` o `brief-render-policy.py`)
- **Docs**:
  - `docs/ai-specs-toml.md` — `[brief].render`, precedencia, subrepos, migración desde marker
  - `templates/ai-specs.toml.tmpl` — comentario opcional de ejemplo
- **Tests**:
  - `tests/test_agents_md_render_opt_out.py` (nuevo, dedicado)
  - Regresión en `tests/test_runtime_brief_baseline.py` / `tests/test_sync_pipeline.py` (marker intacto cuando `render=true`)

## Risks & Migration

1. **Usuario espera que `[brief]` siga aplicando con `render=false`** — docs deben ser explícitas: flag = hands-off total.
2. **Subrepos sorprendidos** — documentar que el flag root silencia render en subrepos también.
3. **Placeholder commiteado por error** — doctor puede WARN si sigue siendo one-liner (open question).
4. **`render = True` inválido TOML** — doctor alineado con card #16.
5. **Proyectos con marker + `render=false`** — redundante pero inofensivo; doctor INFO.

**Migración recomendada para adoptantes manuales:**
1. Añadir `[brief] render = false` al manifest.
2. Opcional: quitar marker HTML si ya no lo necesitan.
3. Mantener `AGENTS.md` como fuente canónica; sync deja de tocarlo.

**ai-specs-cli dogfood:** puede adoptar el flag en apply posterior; no bloquea este change.

## Acceptance Signals

- `./tests/run.sh` verde con `tests/test_agents_md_render_opt_out.py`.
- `./tests/validate.sh` verde.
- Con `[brief] render = false`: sync no modifica `AGENTS.md` existente (byte-identical).
- Con `render = false` + sin archivo: init crea placeholder; sync posterior no lo enriquece.
- Con `render = true` + marker: comportamiento actual intacto (regresión).
- Subrepo target: mismo skip cuando root `render = false`.
- `ai-specs doctor`: WARN fragments muertos; ERROR si falta `AGENTS.md` con render off.

## Open Questions (for specs/design)

1. **Parsing estricto:** ¿`render = "false"` (string) es ERROR en doctor o WARN + treat-as-true? Recomendación: ERROR en materialize/doctor.
2. **Heurística placeholder en doctor:** ¿WARN si content == `# AGENTS.md - Runtime context`?
3. **`resolved-config` con `render=false`:** ¿seguir embediendo `brief_fragments`? Recomendación: sí (solo skip write).
4. **`--force-brief` futuro:** documentar como non-goal MVP; reconsiderar si hay demanda.

## Rollback

- Revertir guards en `sync.sh`, `init.sh`, `sync-agent.sh`, `doctor.py`.
- Quitar `[brief].render` de docs/template.
- Revertir delta specs.
- Sin migración de datos: `render` ausente = comportamiento previo.

## Next Phases

- **specs** → delta en `runtime-brief-rendering` + `recipe-manifest-contract` (resolver Open Questions 1-3).
- **design** → módulo helper, matriz flag/marker/subrepos/init, mensajes exactos, doctor checks.
- **tasks** → breakdown TDD strict por capa (shell guards → doctor → e2e sync/init/subrepo).

(NO ejecutar specs/design/tasks/code en esta ronda.)
