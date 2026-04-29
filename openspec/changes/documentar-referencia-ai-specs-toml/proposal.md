# Proposal: Documentar referencia completa de ai-specs.toml

## Intent
Crear una referencia de usuario navegable para el estado maduro actual de `ai-specs/ai-specs.toml`, porque el contrato V1 existe pero su explicación está repartida entre README, plantilla y tests.

## Scope
### In Scope
- Documentar la superficie madura actual: `[project]`, `[[deps]]` y `[mcp.<name>]`.
- Incluir `[agents]` solo como contexto necesario para explicar fan-out y render MCP.
- Explicar campos, tipos, defaults, compatibilidad y ejemplos de configuración.
- Documentar el render MCP por runtime/agente, incluyendo el contrato `env` con referencias a entorno y literales estáticos.
- Enlazar la referencia desde README.
- Reducir README para evitar roadmap o superficies aún inmaduras.
- Cubrir la referencia con tests o fixture verificable.

### Out of Scope
- Cambiar el schema funcional del manifiesto salvo gaps estrictamente necesarios detectados por la documentación.
- Documentar o recomendar recipes, bindings v2, capabilities o hooks.
- Documentar `[sdd]` como parte de la referencia estable del TOML.
- Agregar soporte funcional nuevo para agentes o MCP.

## Capabilities
### Modified Capabilities
- `manifest-contract`: agrega una referencia de usuario conservadora para la superficie madura del manifiesto V1 y su render MCP por agente.

## Approach
Mantener el contrato runtime intacto y publicar una página dedicada en docs que explique solo el estado actual confiable. README debe apuntar a esa referencia y conservar un resumen corto, sin roadmap especulativo ni recetas prematuras.

## Affected Areas
| Area | Impact | Description |
|------|--------|-------------|
| `docs/` | Added | Nueva referencia dedicada de `ai-specs.toml`. |
| `README.md` | Modified | Enlace explícito a la referencia y reducción de contenido roadmap. |
| `tests/` | Modified | Prueba/fixture que verifica que los ejemplos y campos clave sigan documentados. |

## Risks
| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Documentar comportamiento que no existe | Med | Derivar la referencia de tests y código actual; mantener non-goals explícitos. |
| Duplicar demasiado el README | Med | README enlaza la referencia y conserva solo resumen. |
| Ejemplos se desactualizan | Med | Agregar test de documentación con strings críticos y ejemplos por agente. |

## Success Criteria
- [ ] Existe una referencia madura de `ai-specs.toml` bajo `docs/` centrada en `[project]`, `[[deps]]` y `[mcp.<name>]`.
- [ ] README enlaza la referencia.
- [ ] README evita roadmap y recetas como contrato estable.
- [ ] Los ejemplos de manifest y MCP/env están verificados por tests o fixture.
- [ ] `./tests/validate.sh` pasa.
