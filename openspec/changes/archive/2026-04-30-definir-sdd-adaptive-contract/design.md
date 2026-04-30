# Design: Definir contrato SDD adaptativo (thresholds de ceremonia)

## Context

El estado actual del SDD en `ai-specs-cli` es binario: un change usa el ciclo completo OpenSpec (proposal → specs → design → tasks → apply → verify → archive) o no usa SDD en absoluto. Las skills `openspec-sdd-workflow` y `openspec-phase-orchestrator` asumen que todo change duradero requiere todos los artefactos. No existe mecanismo para omitir `design.md` o `proposal.md` según el impacto real del cambio.

Esta rigidez genera fricción para cambios triviales (typos, fixes localizados) porque fuerza artefactos que no aportan valor. A la vez, eliminar la ceremonia por completo para esos cambios pierde trazabilidad. Se necesita un gradiente.

## Goals / Non-Goals

### Goals
1. **Gradiente de ceremonia**: introducir 4 niveles de ceremonia (`trivial`, `local_fix`, `behavior_change`, `domain_change`) que mapeen a conjuntos diferentes de artefactos requeridos.
2. **Configuración declarativa**: la matriz de decisión vive en `openspec/config.yaml` bajo `sdd.decision_matrix`, editable por humanos y legible por herramientas.
3. **Validación en CLI**: `lib/_internal/sdd.py` y `lib/_internal/recipe-read.py` validan la configuración y emiten errores o advertencias cuando la clasificación de un change choca contra el umbral declarado.
4. **Skill de decisión para agentes**: una skill markdown (`openspec-sdd-decision`) guía al agente para clasificar cambios antes de crear artefactos, inspeccionando specs existentes y reportando rationale.

### Non-Goals
- **Dispatcher automático de fases** (fase C, card #68): este change no implementa un orquestador que, dado un nivel, salte fases automáticamente. Solo declara el contrato; el orquestador existente (`openspec-phase-orchestrator`) se adaptará por skill update.
- **Cambios en CLI entrypoints visibles al usuario final**: no se añaden nuevos comandos `ai-specs` visibles; la validación es interna (`lib/_internal/*.py`).
- **Aplicación retroactiva de SDD a codebase existente**: no se exige que changes ya archivados se reclasifiquen. El contrato aplica solo a changes nuevos.

## Decisions

### 1. ¿Por qué 4 niveles y no 3 o 5?

**Decision**: 4 niveles.

**Rationale**:
- Con 3 niveles (ej: trivial / fix / major), el hueco entre "fix sin specs" y "cambio de dominio completo" es muy grande. El 80 % de los cambios de producto (ajustar validaciones, permisos, notificaciones) quedaría forzado al nivel más alto o al más bajo, sin un nivel intermedio apropiado.
- Con 5 niveles, la granularidad añade fricción cognitiva: el agente (y el humano) pierde tiempo decidiendo entre niveles adyacentes que differen en matices irrelevantes.
- 4 niveles ofrecen una progresión clara: nada → tests → specs+tests → todo, con criterios de clasificación que un agente puede aplicar leyendo el diff y las specs existentes.

**Alternativas consideradas**:
- 3 niveles (trivial / fix / major): rechazado por el hueco intermedio.
- 5 niveles (añadiendo `refactoring` o `infra_change`): rechazado porque aumenta la complejidad de decisión sin mejorar significativamente la precisión.

### 2. ¿Por qué config en `openspec/config.yaml` y no en `ai-specs.toml`?

**Decision**: la matriz de decisión vive en `openspec/config.yaml` bajo `sdd.decision_matrix`.

**Rationale**:
- `ai-specs.toml` es el manifiesto raíz del proyecto: agents, deps, recipes, MCPs. Meter detalles de configuración de proveedor SDD (OpenSpec) en el manifiesto rompe la separación de responsabilidades y acopla el proyecto a un proveedor.
- `openspec/config.yaml` ya es leído y escrito por `lib/_internal/sdd.py` durante `ai-specs sdd enable`. Es el lugar natural para configuración específica del proveedor SDD.
- El schema de `config.yaml` es YAML, lo cual permite listas y tablas anidadas de forma legible para la matriz de niveles.

**Alternativas consideradas**:
- Sección `[sdd]` en `ai-specs.toml`: rechazado porque ensucia el manifiesto raíz con detalles de proveedor. Si en el futuro se cambia de proveedor SDD, la matriz quedaría huérfana o mal nombrada.
- Archivo separado `openspec/sdd-matrix.yaml`: rechazado porque añade un archivo más al árbol sin beneficio; `config.yaml` ya centraliza la configuración OpenSpec.

### 3. ¿Por qué `sdd.threshold` en `recipe.toml` es opcional y no obligatorio?

**Decision**: el campo `sdd.threshold` en `recipe.toml` es opcional.

**Rationale**:
- Retrocompatibilidad: el catálogo de recipes existentes no declara este campo. Forzarlo rompería `ai-specs sync` para cualquier recipe que no se actualice.
- Semántica de ausencia: si una recipe no declara `threshold`, significa que acepta cualquier nivel de ceremonia (default implícito `trivial`). Esto es el comportamiento más permisivo y menos sorprendente.
- Las recipes que representan dominios críticos (ej: auth, billing) pueden optar por declarar `threshold = "domain_change"` o `"behavior_change"`.

**Alternativas consideradas**:
- Campo obligatorio: rechazado por el costo de migración del catálogo existente.
- Default global en `ai-specs.toml`: rechazado porque el umbral es una propiedad de la recipe (dominio), no del proyecto global.

### 4. ¿Por qué skills markdown en vez de código Python para la decisión del agente?

**Decision**: la clasificación de cambios se implementa como una skill markdown (`openspec-sdd-decision`), no como heurísticas en Python.

**Rationale**:
- La clasificación requiere juicio semántico: leer specs existentes, entender la intención del change, evaluar si afecta contratos de usuario. Un algoritmo determinístico basado en tamaño de diff o cantidad de archivos es frágil (un cambio de 1 línea en una validación de pago es `behavior_change`, no `trivial`).
- La skill markdown guía al agente LLM para que aplique los criterios definidos en el spec, inspeccionando el codebase de forma contextual.
- El output estructurado (nivel, rationale, archivos tocados) es generado por el LLM siguiendo el prompt de la skill, no por un parser rígido.

**Alternativas consideradas**:
- Heurísticas Python (contar archivos, líneas modificadas, presencia de `test_` files): rechazado porque no captura intención ni semántica de dominio.
- Modelo de clasificación entrenado: rechazado por complejidad y dependencia de datos de entrenamiento.

### 5. ¿Cómo se maneja el fallback si no hay `decision_matrix` configurado?

**Decision**: si `openspec/config.yaml` no contiene `sdd.decision_matrix`, el sistema cae en **modo formal** (fallback seguro).

**Rationale**:
- Preserva el comportamiento actual: todo change se trata como `domain_change`, exigiendo el ciclo completo.
- Evita que proyectos existentes se vean afectados negativamente al actualizar `ai-specs-cli`: si no optan por el contrato adaptativo, siguen con el flujo formal.
- La skill `openspec-sdd-decision` debe detectar la ausencia de `decision_matrix` y advertir al agente: "No se encontró decision_matrix; se usa modo formal (domain_change) como fallback."

**Flujo de fallback**:

```
lib/_internal/sdd.py::load_decision_matrix(config_path)
  ├─ config.yaml existe y tiene sdd.decision_matrix → return matrix
  └─ config.yaml no tiene sdd.decision_matrix → return None
        └─ caller (skill / CLI validation) interpreta None como modo formal
```

## Risks / Trade-offs

### Riesgo de inconsistencia entre agentes
- **Descripción**: si un agente sigue la skill `openspec-sdd-decision` y clasifica un change como `local_fix`, pero otro agente (o el humano) no consulta la skill y asume `domain_change`, los artefactos generados serán inconsistentes.
- **Mitigación**: la skill se incluye en el catálogo de recipes por defecto para proyectos que usen OpenSpec. El campo `sdd.threshold` en recipe actúa como guardia: si una recipe exige `behavior_change`, el validador CLI advertirá si alguien intenta clasificar por debajo.

### Riesgo de complejidad en `config.yaml`
- **Descripción**: la sección `sdd.decision_matrix` añade una tabla anidada de 4 niveles × 4 campos. Errores de edición manual (typo en `worktree_required`, lista de artefactos malformada) pueden invalidar la configuración.
- **Mitigación**: `validate_decision_matrix(matrix)` en `lib/_internal/sdd.py` verifica:
  - Presencia de los 4 niveles exactos.
  - Tipos correctos (`artifacts` es lista, flags son booleanos).
  - Artefactos conocidos (whitelist de strings: `proposal.md`, `design.md`, `tasks.md`, etc.).

### Trade-off: flexibilidad vs. enforcement
- **Descripción**: un contrato adaptativo es más flexible, pero también más fácil de eludir si el agente decide clasificar un cambio de dominio como `trivial`.
- **Mitigación**: la skill obliga a inspeccionar specs existentes antes de clasificar. Además, la validación CLI puede comparar la clasificación contra `sdd.threshold` y emitir warnings (no errores fatales, para no bloquear al usuario). El enforcement completo queda para futuras herramientas (ej. `ai-specs doctor`).

## Migration Plan

### Adopción en un proyecto nuevo
1. Ejecutar `ai-specs sdd enable` (ya genera `openspec/config.yaml`).
2. Editar `openspec/config.yaml` añadiendo:
   ```yaml
   sdd:
     mode: adaptive
     decision_matrix:
       trivial:
         artifacts: []
         worktree_required: false
         proposal_required: false
         design_required: false
       local_fix:
         artifacts: []
         worktree_required: false
         proposal_required: false
         design_required: false
       behavior_change:
         artifacts: ["tasks.md"]
         worktree_required: true
         proposal_required: false
         design_required: false
       domain_change:
         artifacts: ["proposal.md", "design.md", "tasks.md"]
         worktree_required: true
         proposal_required: true
         design_required: true
   ```
3. Opcional: en recipes críticas del catálogo, añadir `sdd.threshold = "behavior_change"` o `"domain_change"`.
4. Ejecutar `./tests/validate.sh` para verificar que la matriz es válida.

### Migración desde "siempre formal" a "adaptativo"
1. Confirmar que el proyecto tiene `openspec/config.yaml` con `schema: spec-driven`.
2. Añadir la sección `sdd` al `config.yaml` existente (ver arriba).
3. No se requiere reclasificar changes archivados; el contrato aplica prospectivamente.
4. Actualizar `AGENTS.md` o la recipe que vendorea las skills para incluir `openspec-sdd-decision`.
5. En el primer change tras la migración, el agente usará la skill para clasificar. Si la skill no detecta `decision_matrix`, fallback a `domain_change` (comportamiento anterior), por lo que la migración es segura incluso si se olvida el paso 2.

## Open Questions

1. **¿Debe `ai-specs doctor` validar el `decision_matrix`?**
   - Hoy `ai-specs doctor` (implementado en card previa) valida la estructura del manifiesto y directorios. Validar la matriz SDD sería coherente, pero queda fuera del alcance de este change para no expandir el scope. Se propone como mejora futura.

2. **¿Cómo se versiona el schema de `decision_matrix`?**
   - Si en el futuro se añaden campos nuevos a cada nivel (ej. `review_required`), los proyectos con la versión antigua de la matriz seguirán funcionando porque el validador ignora campos desconocidos con advertencia. Sin embargo, no hay un campo de versión explícito en `sdd.decision_matrix`. Se podría añadir `sdd.version` en una iteración futura.

## Component Changes

### 1. `openspec/config.yaml`

Nueva sección raíz opcional `sdd`:

```yaml
sdd:
  mode: adaptive   # "adaptive" | "formal"
  decision_matrix:
    trivial:
      artifacts: []
      worktree_required: false
      proposal_required: false
      design_required: false
    local_fix:
      artifacts: []
      worktree_required: false
      proposal_required: false
      design_required: false
    behavior_change:
      artifacts: ["tasks.md"]
      worktree_required: true
      proposal_required: false
      design_required: false
    domain_change:
      artifacts: ["proposal.md", "design.md", "tasks.md"]
      worktree_required: true
      proposal_required: true
      design_required: true
```

- `mode`: cuando es `"formal"`, el sistema ignora `decision_matrix` y trata todo change como `domain_change`.
- `mode: adaptive` activa la lectura de `decision_matrix`.

### 2. `lib/_internal/recipe_schema.py`

- Nuevo dataclass `SddConfig` con campo opcional `threshold: str = ""`.
- Extender `Recipe` con campo `sdd: SddConfig = field(default_factory=SddConfig)`.
- En `validate_recipe_toml`, parsear la tabla opcional `[sdd]` (si existe) y extraer `threshold`.
- Validar que `threshold`, si no está vacío, sea uno de los 4 niveles conocidos.

### 3. `lib/_internal/sdd.py`

- Nueva función `load_decision_matrix(config_path: Path) -> dict | None`:
  - Lee `openspec/config.yaml`.
  - Si `sdd.mode` es `"formal"` o no existe `sdd.decision_matrix`, retorna `None`.
  - Si existe, retorna el dict de la matriz.
- Nueva función `validate_decision_matrix(matrix: dict) -> list[str]`:
  - Verifica que las 4 claves de nivel existan exactamente.
  - Verifica que cada nivel tenga los campos `artifacts` (lista), `worktree_required` (bool), `proposal_required` (bool), `design_required` (bool).
  - Retorna lista de strings de error (vacía si es válido).
- Añadir las 4 constantes de niveles a un frozenset `CEREMONY_LEVELS` para reutilización.

### 4. `lib/_internal/recipe-read.py`

- Tras cargar la `Recipe`, si `recipe.sdd.threshold` no es vacío:
  - Verificar que el valor esté en `CEREMONY_LEVELS`.
  - Si no, elevar `RecipeValidationError` con mensaje indicando el valor inválido y los permitidos.
- Este check es redundante con el de `recipe_schema.py` pero defensivo, ya que `recipe-read.py` es el punto de entrada CLI para lectura de recipes.

### 5. `ai-specs/skills/openspec-sdd-decision/SKILL.md` (nueva skill)

Contenido del prompt para agentes:

- **Trigger**: "Clasificar un change antes de crear artefactos SDD".
- **Pasos obligatorios**:
  1. Leer `openspec/config.yaml` y verificar `sdd.decision_matrix`. Si no existe, emitir warning y fallback a `domain_change`.
  2. Inspeccionar specs existentes bajo `openspec/specs/` que puedan estar relacionadas con el change.
  3. Analizar el diff o descripción del change.
  4. Seleccionar uno de los 4 niveles aplicando los criterios del spec `sdd-adaptive-contract`.
  5. Reportar:
     - `classification`: nivel elegido.
     - `reasoning`: párrafo conciso justificando la elección.
     - `specs_touched`: lista de specs existentes modificadas o consultadas.
     - `code_touched`: lista de archivos de código afectados.
     - `tests_touched`: lista de archivos de test afectados.
- **Restricción**: no proceder a crear artefactos hasta que el usuario confirme o corrija la clasificación.

### 6. `ai-specs/skills/openspec-sdd-workflow/SKILL.md`

- En la sección "Standard Start", añadir un paso previo a `openspec-new-change`:
  - "Antes de crear el change, invocar `openspec-sdd-decision` para clasificar el change y determinar el nivel de ceremonia."
- En la sección "Phase Map", añadir nota:
  - "Si el nivel de ceremonia es `trivial` o `local_fix`, omitir fases de artefactos que la matriz declare innecesarios."
- En "Guardrails", añadir:
  - "Respetar el `sdd.threshold` de la recipe activa. Si la clasificación del change está por debajo del threshold, emitir warning y consultar al usuario."

### 7. `ai-specs/skills/openspec-phase-orchestrator/SKILL.md`

- En "Phase Map", ajustar la descripción de fases para que sea condicional:
  - `proposal.md`: solo si `proposal_required: true` para el nivel clasificado.
  - `design.md`: solo si `design_required: true` para el nivel clasificado.
  - `tasks.md`: siempre se genera para `behavior_change` y `domain_change`; para `local_fix` puede ser una lista mínima de tareas.
- En "Advancement Rules", añadir:
  - "Si el nivel es `trivial`, el ciclo SDD puede omitirse por completo. El agente aplica el cambio directamente y documenta la clasificación en el mensaje de commit o nota operacional."

### 8. Tests

#### `tests/test_sdd.py`
- `test_load_decision_matrix_returns_none_when_missing`: verifica fallback cuando no hay sección `sdd`.
- `test_load_decision_matrix_returns_dict_when_present`: verifica carga correcta.
- `test_validate_decision_matrix_missing_level`: falta un nivel → error.
- `test_validate_decision_matrix_extra_level`: nivel desconocido → error (o warning según spec).
- `test_validate_decision_matrix_wrong_type_artifacts`: `artifacts` no es lista → error.
- `test_validate_decision_matrix_wrong_type_flags`: flag no es bool → error.
- `test_validate_decision_matrix_valid`: matriz completa y correcta → lista vacía.

#### `tests/test_recipe_schema.py`
- `test_recipe_without_sdd_section_parses`: recipe sin `[sdd]` → `recipe.sdd.threshold == ""`.
- `test_recipe_with_valid_threshold_parses`: `sdd.threshold = "behavior_change"` → parse OK.
- `test_recipe_with_invalid_threshold_fails`: `sdd.threshold = "major_change"` → `RecipeValidationError`.
- `test_recipe_to_dict_includes_sdd`: serialización JSON incluye `sdd`.

## Data Flow (ASCII)

```
+----------+     +------------------------+     +------------------+
|  Agent   | --> | openspec-sdd-decision  | --> | inspect specs    |
| request  |     | skill                  |     | read config.yaml |
+----------+     +------------------------+     +------------------+
                        |
                        v
               +-------------------+
               | classification    |
               | + rationale       |
               | + touched files   |
               +-------------------+
                        |
                        v
               +-------------------+
               | Compare against   |
               | recipe threshold  |
               +-------------------+
                        |
          +-------------+-------------+
          |                           |
          v                           v
   +--------------+           +--------------+
   | Below threshold|         | At/Above threshold|
   | => Warning     |         | => Proceed        |
   +--------------+           +--------------+
          |
          v
   +---------------------------+
   | openspec-phase-orchestrator|
   | skips phases per matrix    |
   +---------------------------+
```

## Appendix: Constants and Types

```python
# lib/_internal/sdd.py
CEREMONY_LEVELS = frozenset({"trivial", "local_fix", "behavior_change", "domain_change"})

# Expected shape of decision_matrix (validated, not enforced by static types)
DecisionMatrix = dict[str, dict[str, Any]]
# Example:
# {
#   "trivial": {
#     "artifacts": [],
#     "worktree_required": False,
#     "proposal_required": False,
#     "design_required": False,
#   },
#   ...
# }
```
