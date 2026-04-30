# Proposal: Definir contrato SDD adaptativo (thresholds de ceremonia)

## Why

El ciclo SDD/OpenSpec completo (proposal → specs → design → tasks → apply → verify → archive) es necesario para cambios arquitectónicos o de dominio, pero resulta excesivo para cambios triviales (typos, copy, CSS menor) o fixes localizados sin cambio de intención. Forzar el flujo completo para todo cambio genera fricción, burocracia innecesaria, y desincentiva el mantenimiento continuo de la fuente de verdad.

Este change introduce un **gradiente de ceremonia** que permite al agente y al humano decidir cuánto SDD aplicar según el impacto real del cambio, manteniendo la fuente de verdad viva sin convertir cada commit en un ciclo formal.

## What Changes

- **Nuevo spec formal** `sdd-adaptive-contract` que define 4 niveles de ceremonia y sus criterios de clasificación.
- **Extensión de `openspec/config.yaml`** con sección `decision_matrix` que mapea niveles a artefactos requeridos.
- **Extensión del schema de `recipe.toml`** con campo opcional `sdd.threshold` para que una recipe pueda forzar formalidad mínima en dominios críticos.
- **Validación en CLI**: `lib/_internal/sdd.py` lee y valida `decision_matrix`; `lib/_internal/recipe_schema.py` valida `sdd.threshold`.
- **Nueva skill `openspec-sdd-decision`** que guía al agente para clasificar cambios y elegir el nivel de ceremonia correcto.
- **Actualización de skills existentes** (`openspec-sdd-workflow`, `openspec-phase-orchestrator`) para respetar el contrato adaptativo (ej: no exigir `design.md` para cambios de comportamiento pequeños).
- **Tests** para validación de config y recipe schema.

## Capabilities

### New Capabilities
- `sdd-adaptive-contract`: Define el gradiente de ceremonia SDD con 4 niveles (trivial, local-fix, behavior-change, domain-change), criterios de clasificación, artefactos requeridos por nivel, y configuración declarativa en `openspec/config.yaml`.

### Modified Capabilities
- *(Ninguno. Este change no altera los requisitos de capabilities SDD existentes como `sdd-artifact-store` o `sdd-cli-integration`; solo introduce un nuevo capability que las consume.)*

## Impact

### Affected Modules
- `lib/_internal/sdd.py` — lectura/validación de `decision_matrix`
- `lib/_internal/recipe_schema.py` — extensión del dataclass `Recipe` con campo `sdd`
- `lib/_internal/recipe-read.py` — validación de `sdd.threshold` al leer recipe.toml
- `ai-specs/skills/openspec-sdd-workflow/SKILL.md` — referenciar contrato adaptativo
- `ai-specs/skills/openspec-phase-orchestrator/SKILL.md` — respetar artefactos por nivel
- `ai-specs/skills/openspec-sdd-decision/SKILL.md` — **nueva skill**
- `openspec/config.yaml` — **nueva sección `sdd.decision_matrix`**
- `tests/test_sdd.py` — tests de validación
- `tests/test_recipe_schema.py` — tests de schema extendido

### APIs / Interfaces
- No cambios en CLI entrypoints visibles al usuario final.
- `ai-specs doctor` o `ai-specs validate` podrán advertir si un change no cumple el umbral declarado (futuro, no en este change).

### Dependencies
- Ninguna nueva. Depende del schema `spec-driven` ya existente.

### Rollback Plan
- Este change es aditivo: solo agrega configuración opcional y validación. Si genera problemas, se puede revertir eliminando la sección `sdd` de `openspec/config.yaml` y el campo `sdd` de `recipe.toml`. Las skills pueden ignorar el contrato si no encuentran la configuración (fallback a modo formal actual).
