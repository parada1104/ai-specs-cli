## Context

El proyecto carece de un esquema unificado para metadatos en documentos de memoria. Cada agente y sesión genera handoffs, decisiones y specs con formatos ad-hoc, lo que impide la indexación automática y la consistencia. Este design establece las decisiones técnicas para un frontmatter YAML estándar.

## Goals / Non-Goals

**Goals:**
- Definir un esquema YAML mínimo, obligatorio y extensible.
- Especificar valores cerrados para campos críticos (`type`, `scope`, `status`).
- Documentar ejemplos aplicables a docs canónicas, handoffs y proposed memory.
- Publicar el estándar en `ai-specs/docs/canonical-memory-frontmatter.md`.

**Non-Goals:**
- No se construye un parser/validador automático en este change.
- No se migra documentación histórica existente.
- No se ataca al Context Router ni a integraciones con Obsidian.

## Decisions

1. **YAML sobre TOML**
   - *Rationale*: YAML frontmatter es el estándar de facto en Markdown (Jekyll, Hugo, Obsidian). TOML requiere aprendizaje adicional para contributors no técnicos.
   - *Alternativa considerada*: TOML — rechazado por fricción de adopción.

2. **Campos mínimos obligatorios: `type`, `scope`, `status`, `tags`, `source`, `updated_at`**
   - *Rationale*: Cubren las dimensiones esenciales para indexación (qué es, de qué dominio, en qué estado, categorización, procedencia, vigencia).
   - `title` se incluye como recomendado pero no obligatorio para no romper documentos existentes que usan H1 como título.

3. **Valores cerrados con extensibilidad controlada**
   - `type`: `decision`, `handoff`, `spec`, `context`, `proposed`
   - `scope`: `project`, `team`, `system`, `session`
   - `status`: `proposed`, `accepted`, `deprecated`
   - *Rationale*: Valores cerrados garantizan consistencia en indexación. La extensibilidad se permite mediante PR al estándar, no ad-hoc.

4. **`confidence` como campo opcional**
   - *Rationale*: Relevante para `proposed` memory y decisiones arquitectónicas, pero innecesario para handoffs históricos. Float 0.0–1.0.

5. **Ubicación en `ai-specs/docs/`**
   - *Rationale*: El maintainer solicitó explícitamente que el estándar viva bajo `ai-specs/` para que su procedencia (generado por/mantenido por ai-specs) sea evidente.
   - *Alternativa considerada*: `docs/ai-memory/` — se prefiere `ai-specs/docs/` por ser más explícito sobre ownership.

## Risks / Trade-offs

- **[Risk]** Adopción manual sin validador automático puede generar desviaciones del estándar.
  - *Mitigación*: Incluir ejemplos explícitos en el doc y recomendar revisiones en PR.
- **[Risk]` Valores cerrados pueden ser restrictivos para casos de borde.
  - *Mitigación*: Documentar proceso de extensión (PR al estándar) y mantener `tags` como válvula de escape semántica.
- **[Trade-off]** `title` no obligatorio puede dificultar la generación de índices automáticos.
  - *Mitigación*: Recomendar fuertemente su uso y usar H1 como fallback.

## Migration Plan

No aplica — este change es documentación pura. La adopción es gradual: nuevos documentos usan el estándar; documentos existentes se migran bajo demanda.

## Open Questions

- ¿Se requiere un campo `author` o `agent` para trazabilidad? (Pendiente de validar con uso real.)
- ¿Se integrará con `vault-context` skill para validación automática en sesiones futuras?
