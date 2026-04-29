# Proposal: Documentar referencia completa de ai-specs.toml

## Intent
Crear una referencia de usuario completa y navegable para `ai-specs/ai-specs.toml`, porque el contrato V1 ya existe pero hoy la explicación está repartida entre README, plantilla y tests.

## Scope
### In Scope
- Documentar secciones soportadas: `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]`, `[recipes.<id>]` y `[sdd]`.
- Explicar campos, tipos, defaults, compatibilidad y ejemplos de configuración.
- Documentar el render MCP por runtime/agente, incluyendo el contrato `env` con referencias a entorno y literales estáticos.
- Enlazar la referencia desde README.
- Cubrir la referencia con tests o fixture verificable.

### Out of Scope
- Cambiar el schema funcional del manifiesto salvo gaps estrictamente necesarios detectados por la documentación.
- Rediseñar recipes, bindings v2, capabilities o hooks.
- Agregar soporte funcional nuevo para agentes o MCP.

## Capabilities
### Modified Capabilities
- `manifest-contract`: agrega una referencia de usuario completa del manifiesto V1 y su render por agente.

## Approach
Mantener el contrato runtime intacto y publicar una página dedicada en docs que sea la fuente de explicación de usuario. README debe apuntar a esa referencia y conservar solo el resumen de contrato V1.

## Affected Areas
| Area | Impact | Description |
|------|--------|-------------|
| `docs/` | Added | Nueva referencia dedicada de `ai-specs.toml`. |
| `README.md` | Modified | Enlace explícito a la referencia. |
| `tests/` | Modified | Prueba/fixture que verifica que los ejemplos y campos clave sigan documentados. |

## Risks
| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Documentar comportamiento que no existe | Med | Derivar la referencia de tests y código actual; mantener non-goals explícitos. |
| Duplicar demasiado el README | Med | README enlaza la referencia y conserva solo resumen. |
| Ejemplos se desactualizan | Med | Agregar test de documentación con strings críticos y ejemplos por agente. |

## Success Criteria
- [ ] Existe una referencia completa de `ai-specs.toml` bajo `docs/`.
- [ ] README enlaza la referencia.
- [ ] Los ejemplos de manifest y MCP/env están verificados por tests o fixture.
- [ ] `./tests/validate.sh` pasa.
