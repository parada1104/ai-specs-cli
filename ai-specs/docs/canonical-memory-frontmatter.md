# Canonical Memory Frontmatter

Estándar de metadatos YAML para documentos de memoria canónica del proyecto `ai-specs-cli`.

## Propósito

Garantizar que todo documento de memoria (handoffs, decisiones, specs, contexto) sea indexable, consistente y descubrible por agentes y humanos.

## Campos Mínimos Obligatorios

Todo documento de memoria canónica **debe** incluir los siguientes campos en su frontmatter YAML:

| Campo | Tipo | Descripción |
|---|---|---|
| `type` | string | Categoría semántica del documento |
| `scope` | string | Alcance de aplicabilidad |
| `status` | string | Estado de vigencia del contenido |
| `tags` | list[string] | Etiquetas para indexación y descubrimiento |
| `source` | string | Origen o procedencia del documento |
| `updated_at` | string (ISO 8601) | Fecha de última actualización |

## Valores Válidos

### `type`

- `decision` — Decisiones arquitectónicas o de diseño (ADR)
- `handoff` — Resumen de sesión o transferencia de contexto
- `spec` — Especificación técnica o contrato
- `context` — Contexto de negocio o proyecto
- `proposed` — Memoria propuesta, pendiente de validación

### `scope`

- `project` — Aplica al proyecto completo
- `team` — Aplica al equipo de trabajo
- `system` — Aplica a un sistema o subsistema específico
- `session` — Aplica a una sesión o iteración particular

### `status`

- `proposed` — Propuesto, aún no ratificado
- `accepted` — Aceptado y vigente
- `deprecated` — Obsoleto, mantenido solo para referencia histórica

## Campo Opcional

### `confidence`

- **Tipo**: float
- **Rango**: `0.0` a `1.0` inclusive
- **Uso**: Indica el nivel de confianza en el contenido, especialmente útil para documentos `type: proposed` o decisiones arquitectónicas.

## Ejemplos

### Documentación Canónica

```yaml
---
type: context
scope: project
status: accepted
tags: [architecture, conventions]
source: ai-specs-core-team
updated_at: "2026-04-27T00:00:00Z"
---

# Convenciones de nomenclatura

...
```

### Handoff

```yaml
---
type: handoff
scope: session
status: accepted
tags: [sprint-12, auth-module]
source: claude-session-2026-04-27
confidence: 0.95
updated_at: "2026-04-27T18:30:00Z"
---

# Handoff: Implementación de autenticación

## Qué se hizo
- ...

## Qué falta
- ...
```

### Proposed Memory

```yaml
---
type: proposed
scope: system
status: proposed
tags: [obsidian, vault, frontmatter]
source: robert-proposal-001
confidence: 0.70
updated_at: "2026-04-27T12:00:00Z"
---

# Propuesta: Integrar validador de frontmatter en vault-context

## Hipótesis
...
```

## Extensibilidad

Nuevos valores para `type`, `scope` o `status` **solo** se agregan mediante actualización de este documento de referencia. No se permite el uso ad-hoc de valores no listados en documentos individuales.

Para proponer una extensión:

1. Abrir un change de OpenSpec o una card en el board de roadmap.
2. Actualizar este archivo (`ai-specs/docs/canonical-memory-frontmatter.md`).
3. Validar que los nuevos valores no entren en conflicto con los existentes.

## Referencias

- Card Trello: [Definir frontmatter estándar para canonical memory](https://trello.com/c/zonvpc21)
- Change OpenSpec: `frontmatter-canonico`
