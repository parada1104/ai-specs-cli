# manifest-contract Delta Specification

## Modified Requirements

### Requirement: Alineación entre contrato, plantilla y README

El sistema MUST dejar una definición V1 consistente entre plantilla, README y límites de compatibilidad del runtime, y SHOULD publicar una referencia dedicada para usuarios que explique la superficie madura actual de `ai-specs/ai-specs.toml` sin promover roadmap o secciones inmaduras.

#### Scenario: README enlaza referencia dedicada
- GIVEN la documentación pública del manifiesto
- WHEN un usuario lee el README
- THEN el README MUST enlazar una referencia dedicada de `ai-specs.toml`
- AND el enlace MUST estar cerca del resumen del contrato V1

#### Scenario: Referencia documenta superficie madura
- GIVEN la referencia dedicada de `ai-specs.toml`
- WHEN se publica la documentación
- THEN MUST documentar `[project]`, `[[deps]]` y `[mcp.<name>]`
- AND MAY documentar `[agents]` solo como contexto de fan-out
- AND MUST describir campos, tipos válidos, defaults y compatibilidad tolerada para esa superficie madura
- AND MUST NOT documentar `[recipes.<id>]` ni `[sdd]` como parte de la referencia estable

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
- AND la verificación MUST fallar si se omiten `[project]`, `[[deps]]`, `[mcp.<name>]`, agentes de render o formas `env` requeridas por este cambio

#### Scenario: README evita roadmap como contrato
- GIVEN la documentación pública del manifiesto
- WHEN un usuario lee el README
- THEN el README MUST mantener un resumen breve y enlazar la referencia dedicada
- AND MUST NOT promover recetas, SDD o roadmap como superficie estable del TOML
