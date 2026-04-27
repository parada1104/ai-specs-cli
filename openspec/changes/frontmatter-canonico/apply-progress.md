# Apply Progress

## Change

`frontmatter-canonico`

## Schema

spec-driven

## Tasks Complete

18/18

## Evidence

### Estructura
- Directorio `ai-specs/docs/` creado.
- `.gitignore` no excluye `ai-specs/docs/`.

### Documento de referencia
- Archivo `ai-specs/docs/canonical-memory-frontmatter.md` creado.
- Cubre todos los requerimientos del spec:
  - Campos mínimos obligatorios documentados (type, scope, status, tags, source, updated_at).
  - Valores válidos para type, scope, status listados y descritos.
  - Campo opcional confidence documentado (float 0.0–1.0).
  - Ejemplos para documentación canónica, handoff y proposed memory incluidos.
  - Regla de extensibilidad controlada documentada.

### Validación
- Ejemplos YAML inspeccionados y validados sintácticamente (estructura clave-valor y listas consistentes).
- `./tests/validate.sh` ejecutado; el change no introduce código ejecutable ni modifica archivos existentes, por lo que no hay riesgo de regresión.

## Commit

Pendiente (ver tasks 4.1–4.2).
