# manifest-contract Delta Specification

## Modified Requirements

### Requirement: Alineación entre contrato, plantilla y README

El sistema MUST dejar una sola definición V1 consistente entre plantilla, README y límites de compatibilidad del runtime, y SHOULD publicar una referencia dedicada para usuarios que explique la configuración completa soportada por `ai-specs/ai-specs.toml`.

#### Scenario: README enlaza referencia dedicada
- GIVEN la documentación pública del manifiesto
- WHEN un usuario lee el README
- THEN el README MUST enlazar una referencia dedicada de `ai-specs.toml`
- AND el enlace MUST estar cerca del resumen del contrato V1

#### Scenario: Referencia documenta superficie completa
- GIVEN la referencia dedicada de `ai-specs.toml`
- WHEN se publica la documentación
- THEN MUST documentar `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]`, `[recipes.<id>]` y `[sdd]`
- AND MUST describir campos, tipos válidos, defaults y compatibilidad tolerada para cada sección

#### Scenario: Referencia documenta MCP por agente
- GIVEN la referencia dedicada de `ai-specs.toml`
- WHEN se documenta `[mcp.<name>]`
- THEN MUST explicar cómo se materializa MCP para Claude Code, Cursor, OpenCode y Codex cuando aplique
- AND MUST documentar `env = ["VAR"]` como referencia a variable del entorno
- AND MUST documentar `env = { VAR = "literal" }` como valor estático literal

#### Scenario: Ejemplos verificables
- GIVEN la referencia dedicada de `ai-specs.toml`
- WHEN se agregan ejemplos de configuración
- THEN los ejemplos críticos MUST estar cubiertos por una prueba o fixture verificable
- AND la verificación MUST fallar si se omiten secciones, agentes o formas `env` requeridas por este cambio
