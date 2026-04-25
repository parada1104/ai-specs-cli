# Design: Definir contrato del ai-specs.toml

## Technical Approach

Definir el contrato V1 desde el runtime actual, no desde una schema futura. La implementación centraliza normalización y clasificación canónica en los lectores internos ya existentes, mantiene la validación fuerte solo donde hoy ya hay semántica operativa (`[[deps]]` y `project.subrepos`), y alinea `README.md` + `templates/ai-specs.toml.tmpl` con ese mismo baseline.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Contract authority | Usar `lib/_internal/toml-read.py` como punto de normalización V1 para `project`, `agents`, `deps` y `mcp` | Crear un schema/validator nuevo separado | Evita duplicar reglas y mantiene el cambio conservador. |
| Validation scope | Validar solo invariantes ya operativas: `deps.id/source`, alias MCP (`environment`→`env`), y `project.subrepos` en `target-resolve.py` | Agregar validación global/doctor | El spec exige NO meter precedence, doctor ni reglas sin consumo real. |
| Compatibility mode | Canonicalizar `env` como campo documentado y tolerar `environment` como alias de entrada; secciones faltantes siguen resolviendo defaults | Romper aliases legacy o exigir secciones completas | Preserva manifests hoy funcionales por lectores tolerantes. |

## Data Flow

```text
ai-specs.toml
   │
   ├─ toml-read.py ── normaliza defaults + aliases V1
   │      ├─ target-resolve.py ── valida project.subrepos
   │      ├─ vendor-skills.py  ── exige deps.id/source
   │      └─ mcp-render.py     ── renderiza MCP ya canonicalizado
   │
   └─ README/template ── describen exactamente esa misma superficie
```

## File Changes

| File | Action | Description |
|------|--------|-------------|
| `templates/ai-specs.toml.tmpl` | Modify | Reescribir comentarios para marcar required/optional/defaulted/alias tolerado sin prometer features fuera de V1. |
| `README.md` | Modify | Publicar ownership del root manifest, secciones soportadas, límites V1 y compatibilidad conservadora. |
| `lib/_internal/toml-read.py` | Modify | Agregar lectores/normalizadores explícitos para `agents`, `deps` y `mcp`; canonicalizar `environment`→`env`; defaults estables para secciones faltantes. |
| `lib/_internal/target-resolve.py` | Modify | Consumir lectura normalizada y mantener intacta la semántica actual de errores de `subrepos`. |
| `lib/_internal/vendor-skills.py` | Modify | Consumir deps normalizadas y dejar explícito que solo `id` y `source` son obligatorios. |
| `lib/_internal/mcp-render.py` | Modify | Consumir MCP normalizado para que `env` sea canónico y `environment` siga funcionando como alias tolerado. |
| `tests/test_target_resolve.py` | Modify | Cubrir que la validación de `subrepos` sigue igual bajo el contrato V1. |
| `tests/test_sync_pipeline.py` | Modify | Verificar que manifests con secciones faltantes/compatibles no rompen `sync`. |
| `tests/test_toml_read.py` | Create | Probar defaults, clasificación mínima y canonicalización MCP. |

## Interfaces / Contracts

```python
# Canonical shapes after toml-read normalization
project = {"name": str, "subrepos": list[str]}
agents = {"enabled": list[str]}           # missing => []
deps = [{"id": str, "source": str, ...}] # validation stays conservative
mcp = {
  name: {
    "command": str | list[str] | None,
    "args": list,
    "env": dict,           # canonical output
    "timeout": int | None,
    "enabled": bool | None # tolerated passthrough, not new contract surface
  }
}
```

Clasificación V1 a documentar:
- `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]` = única superficie canónica.
- `deps.id` y `deps.source` = mínimos requeridos.
- `project.name`, `agents.enabled`, `mcp.*.command/args/env/timeout` = opcionales/defaulted según soporte real.
- `environment` = alias tolerado de entrada, no nombre canónico.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Unit | Normalización de `toml-read.py` | Nuevo `tests/test_toml_read.py` para secciones faltantes, `environment`→`env`, y listas vacías/defaults. |
| Unit | Validación de `project.subrepos` | Extender `tests/test_target_resolve.py` sin cambiar textos/razones de error existentes. |
| Integration | Consumo real del manifiesto por `sync`/`sync-agent` | Ajustar `tests/test_sync_pipeline.py` para manifests válidos mínimos y compatibilidad hacia atrás. |
| Validation | Sanidad del repo | Ejecutar `./tests/run.sh`; `./tests/validate.sh` queda informativo por fallas conocidas de fixtures. |

## Migration / Rollout

No migration required. El cambio formaliza y normaliza el contrato V1 del manifiesto root existente; manifests actuales deben seguir funcionando. Solo se documentan campos canónicos y aliases tolerados.

## Open Questions

- [ ] Ninguna bloqueante para `sdd-tasks`; precedence, `doctor` y `[memory]` siguen explícitamente diferidos.
