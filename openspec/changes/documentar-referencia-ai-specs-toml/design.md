# Design: Documentar referencia completa de ai-specs.toml

## Technical Approach

Agregar una página de documentación dedicada, `docs/ai-specs-toml.md`, que convierta la superficie madura actual del contrato V1 en referencia práctica para usuarios. No se cambia el parser ni el render; el trabajo usa el comportamiento existente como fuente y lo fija con tests de documentación.

## Architecture Decisions

| Decision | Choice | Alternatives considered | Rationale |
|---|---|---|---|
| Ubicación | `docs/ai-specs-toml.md` | Expandir más el README | Evita que README crezca y da una URL estable a usuarios. |
| Alcance | Documentación conservadora de `[project]`, `[[deps]]` y `[mcp.<name>]` | Documentar todo lo que el runtime tolera | Evita presentar roadmap o secciones inmaduras como recomendación de usuario. |
| Verificación | Test de strings críticos y ejemplos | Validación manual | Reduce drift de docs en futuros cambios de manifest. |

## Documentation Shape

La referencia tendrá:
- Visión general y fuente de verdad.
- Tabla de superficie madura soportada.
- Sección por bloque TOML con campos, tipos, defaults y ejemplos.
- Contrato MCP con render por Claude Code, Cursor, OpenCode y Codex.
- Ejemplo completo con project, agents, deps y MCP.
- Límites explícitos para no documentar recipes, SDD ni roadmap como referencia estable.

## Testing Strategy

| Layer | What to Test | Approach |
|-------|-------------|----------|
| Docs contract | README enlaza la referencia | Extender `tests/test_manifest_contract_docs.py`. |
| Docs reference | Secciones/campos/agentes/env críticos y ausencia de superficie inmadura | Leer `docs/ai-specs-toml.md` y verificar strings canónicos + guardrails negativos. |
| Validation | Repo completo | Ejecutar `./tests/validate.sh`. |

## Migration / Rollout

No hay migración. La referencia documenta el contrato actual y debe mantenerse alineada con README, plantilla y tests existentes.
