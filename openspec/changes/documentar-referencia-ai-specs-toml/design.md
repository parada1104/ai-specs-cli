# Design: Documentar referencia completa de ai-specs.toml

## Technical Approach

Agregar una página de documentación dedicada, `docs/ai-specs-toml.md`, que convierta el contrato V1 actual en referencia práctica para usuarios. No se cambia el parser ni el render; el trabajo usa el comportamiento existente como fuente y lo fija con tests de documentación.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Ubicación | `docs/ai-specs-toml.md` | Expandir más el README | Evita que README crezca y da una URL estable a usuarios. |
| Alcance | Documentación de contrato actual | Introducir schema nuevo | La card pide entender qué soporta y cómo configurarlo, no cambiar runtime. |
| Verificación | Test de strings críticos y ejemplos | Validación manual | Reduce drift de docs en futuros cambios de manifest. |

## Documentation Shape

La referencia tendrá:
- Visión general y fuente de verdad.
- Tabla de secciones soportadas.
- Sección por bloque TOML con campos, tipos, defaults y ejemplos.
- Contrato MCP con render por Claude Code, Cursor, OpenCode y Codex.
- Ejemplo completo mínimo y ejemplo con SDD/recipes/MCP.
- Límites explícitos y campos diferidos.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Docs contract | README enlaza la referencia | Extender `tests/test_manifest_contract_docs.py`. |
| Docs reference | Secciones/campos/agentes/env críticos | Leer `docs/ai-specs-toml.md` y verificar strings canónicos. |
| Validation | Repo completo | Ejecutar `./tests/validate.sh`. |

## Migration / Rollout

No hay migración. La referencia documenta el contrato actual y debe mantenerse alineada con README, plantilla y tests existentes.
