## Why

El proyecto acumula documentos de memoria (handoffs, decisiones, specs) dispersos sin un esquema de metadatos unificado. Esto dificulta la indexación automática, el descubrimiento por agentes y la consistencia entre sesiones. Se necesita un frontmatter YAML mínimo y obligatorio para todo documento canónico.

## What Changes

- Definir el conjunto mínimo de campos de frontmatter YAML (`type`, `scope`, `status`, `tags`, `source`, `updated_at`).
- Definir valores válidos y cerrados para cada campo (enumeraciones documentadas).
- Definir cómo tratar `confidence` como campo opcional.
- Crear documento de referencia canónico con ejemplos para docs, handoffs y proposed memory.
- Ubicar el estándar bajo `ai-specs/docs/canonical-memory-frontmatter.md` para que su procedencia sea explícita.

## Capabilities

### New Capabilities
- `canonical-memory-frontmatter`: Especifica el esquema de metadatos YAML mínimo para documentos de memoria canónica, incluyendo campos obligatorios, valores válidos, ejemplos por tipo de documento y reglas de extensibilidad.

### Modified Capabilities
- *(Ninguno — este change es puro estándar/documentación)*

## Impact

- Nuevos documentos de memoria deberán incluir el frontmatter definido.
- El estándar se almacena en `ai-specs/docs/` (convención de ubicación propuesta por el maintainer).
- No afecta código ejecutable ni APIs del CLI.
