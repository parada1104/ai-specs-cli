# Proposal: Definir contrato del ai-specs.toml

## Intent
Establecer un contrato V1 canónico y validable para `ai-specs/ai-specs.toml`, porque hoy la manifest funciona como fuente operativa de verdad pero sus reglas viven dispersas entre plantilla, README y lectores tolerantes.

## Scope
### In Scope
- Definir la superficie V1 actual: `[project]`, `[agents]`, `[[deps]]`, `[mcp.<name>]`.
- Clasificar cada campo como requerido, opcional, defaulted o alias tolerado.
- Alinear documentación, plantilla y puntos mínimos de validación con el comportamiento soportado hoy.

### Out of Scope
- `ai-specs doctor` y su UX.
- Reglas de precedence/merge, `[memory]` y cambios de runtime no necesarios para expresar el contrato.

## Capabilities
### New Capabilities
- `manifest-contract`: Contrato canónico del manifiesto `ai-specs.toml`, incluyendo compatibilidad y límites de validación.

### Modified Capabilities
- None.

## Approach
Tomar el comportamiento existente como baseline: documentar el contrato mínimo realmente soportado, explicitar compatibilidad hacia atrás y agregar validación conservadora sin exigir campos que hoy el runtime no necesita.

## Affected Areas
| Area | Impact | Description |
|------|--------|-------------|
| `templates/ai-specs.toml.tmpl` | Modified | Convertir comentarios implícitos en contrato V1 consistente. |
| `README.md` | Modified | Publicar ownership, secciones soportadas y límites de compatibilidad. |
| `lib/_internal/toml-read.py` | Modified | Normalización/documentación alineada al contrato. |
| `lib/_internal/target-resolve.py` | Modified | Mantener validación de `subrepos` coherente con V1. |
| `lib/_internal/vendor-skills.py` | Modified | Reflejar mínimos requeridos para `[[deps]]`. |
| `lib/_internal/mcp-render.py` | Modified | Declarar `env` canónico y aliases tolerados. |

## Risks
| Risk | Likelihood | Mitigation |
|------|------------|------------|
| Endurecer de más y romper manifests válidos hoy | Med | Baseline conservador + aliases/defaults explícitos. |
| Congelar contradicciones actuales | Med | Reconciliar template, README y runtime antes de validar. |
| Mezclar este cambio con precedence/doctor | Low | Mantener non-goals estrictos en specs y diseño. |

## Rollback Plan
Revertir cambios de docs/plantilla/validación de este change y volver al comportamiento tolerante actual.

## Dependencies
- Exploration `openspec/changes/definir-contrato-ai-specs-toml/exploration.md`
- Memoria `sdd-init/ai-specs-cli`

## Success Criteria
- [ ] Existe una definición V1 única del manifiesto actual.
- [ ] La propuesta separa contrato, compatibilidad y trabajo explícitamente diferido.
- [ ] El siguiente paso puede escribir specs sin ambigüedad sobre capabilities y límites.
