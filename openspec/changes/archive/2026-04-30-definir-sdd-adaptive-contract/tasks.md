# Tasks: Definir contrato SDD adaptativo (thresholds de ceremonia)

## Phase 1: Configuración y Schema

- [x] 1.1 Actualizar `openspec/config.yaml` con sección raíz `sdd` que incluya `mode: adaptive` y `decision_matrix` completa para los 4 niveles (`trivial`, `local_fix`, `behavior_change`, `domain_change`) con sus campos `artifacts`, `worktree_required`, `proposal_required` y `design_required`.
- [x] 1.2 Extender `lib/_internal/recipe_schema.py` con el dataclass `SddConfig` que tenga el campo opcional `threshold: str = ""`.
- [x] 1.3 Extender el dataclass `Recipe` en `lib/_internal/recipe_schema.py` con el campo `sdd: SddConfig = field(default_factory=SddConfig)`.
- [x] 1.4 Actualizar `validate_recipe_toml` en `lib/_internal/recipe_schema.py` para parsear la tabla opcional `[sdd]`, extraer `threshold` y validar que, si está presente y no vacío, sea uno de los 4 niveles conocidos; de lo contrario, elevar `RecipeValidationError`.

## Phase 2: Validación en CLI

- [x] 2.1 Implementar la constante `CEREMONY_LEVELS = frozenset({"trivial", "local_fix", "behavior_change", "domain_change"})` en `lib/_internal/sdd.py`.
- [x] 2.2 Implementar `load_decision_matrix(config_path: Path) -> dict | None` en `lib/_internal/sdd.py` para leer `openspec/config.yaml` y retornar el diccionario de la matriz cuando `sdd.mode` sea `"adaptive"`; retornar `None` si la sección falta o el modo es `"formal"`.
- [x] 2.3 Implementar `validate_decision_matrix(matrix: dict) -> list[str]` en `lib/_internal/sdd.py` para verificar: (a) presencia exacta de los 4 niveles, (b) tipos correctos (`artifacts` es lista, flags son bool), (c) retornar lista de mensajes de error vacía si es válida.
- [x] 2.4 Actualizar `lib/_internal/recipe-read.py` para que, tras cargar una `Recipe`, valide defensivamente que `recipe.sdd.threshold` —si no está vacío— pertenezca a `CEREMONY_LEVELS`, y eleve `RecipeValidationError` con el valor inválido y la lista de niveles permitidos.

## Phase 3: Skills

- [x] 3.1 Crear `ai-specs/skills/openspec-sdd-decision/SKILL.md` con frontmatter obligatorio (`name`, `description`, `version`), trigger de clasificación de cambios, pasos obligatorios (leer config, inspeccionar specs, analizar diff, seleccionar nivel, reportar estructurado) y la restricción de no proceder sin confirmación humana.
- [x] 3.2 Actualizar `ai-specs/skills/openspec-sdd-workflow/SKILL.md` en la sección "Standard Start" para añadir un paso previo que invoque `openspec-sdd-decision` antes de `openspec-new-change`.
- [x] 3.3 Actualizar `ai-specs/skills/openspec-sdd-workflow/SKILL.md` en la sección "Phase Map" para indicar que se omitan fases de artefactos innecesarias cuando el nivel sea `trivial` o `local_fix`.
- [x] 3.4 Actualizar `ai-specs/skills/openspec-sdd-workflow/SKILL.md` en "Guardrails" para añadir la regla de respetar `sdd.threshold` de la recipe activa, emitir warning si la clasificación queda por debajo y consultar al usuario.
- [x] 3.5 Actualizar `ai-specs/skills/openspec-phase-orchestrator/SKILL.md` en "Phase Map" para hacer condicional la generación de `proposal.md` y `design.md` según `proposal_required` y `design_required` del nivel clasificado.
- [x] 3.6 Actualizar `ai-specs/skills/openspec-phase-orchestrator/SKILL.md` en "Advancement Rules" para permitir omitir el ciclo SDD completo cuando el nivel sea `trivial`, aplicando el cambio directamente y documentando la clasificación.

## Phase 4: Tests

- [x] 4.1 Crear `tests/test_sdd.py` con `test_load_decision_matrix_returns_none_when_missing` que verifique el fallback a `None` cuando `openspec/config.yaml` no tiene sección `sdd`.
- [x] 4.2 Crear en `tests/test_sdd.py` el test `test_load_decision_matrix_returns_dict_when_present` que verifique carga correcta de la matriz cuando `sdd.decision_matrix` existe y `mode` es `adaptive`.
- [x] 4.3 Crear en `tests/test_sdd.py` el test `test_validate_decision_matrix_missing_level` que verifique error cuando falta uno de los 4 niveles.
- [x] 4.4 Crear en `tests/test_sdd.py` el test `test_validate_decision_matrix_extra_level` que verifique error (o warning) cuando existe un nivel desconocido.
- [x] 4.5 Crear en `tests/test_sdd.py` el test `test_validate_decision_matrix_wrong_type_artifacts` que verifique error cuando `artifacts` no es una lista.
- [x] 4.6 Crear en `tests/test_sdd.py` el test `test_validate_decision_matrix_wrong_type_flags` que verifique error cuando algún flag no es booleano.
- [x] 4.7 Crear en `tests/test_sdd.py` el test `test_validate_decision_matrix_valid` que verifique lista vacía de errores para una matriz completa y correcta.
- [x] 4.8 Extender `tests/test_recipe_schema.py` con `test_recipe_without_sdd_section_parses` que verifique que una recipe sin `[sdd]` produce `recipe.sdd.threshold == ""`.
- [x] 4.9 Extender `tests/test_recipe_schema.py` con `test_recipe_with_valid_threshold_parses` que verifique que `sdd.threshold = "behavior_change"` se parsea correctamente.
- [x] 4.10 Extender `tests/test_recipe_schema.py` con `test_recipe_with_invalid_threshold_fails` que verifique que `sdd.threshold = "major_change"` eleva `RecipeValidationError`.
- [x] 4.11 Extender `tests/test_recipe_schema.py` con `test_recipe_to_dict_includes_sdd` que verifique que la serialización a diccionario incluye la clave `sdd`.

## Phase 5: Verificación

- [x] 5.1 Ejecutar `./tests/run.sh` y corregir cualquier falla relacionada con este change hasta que todos los tests pasen.
- [x] 5.2 Ejecutar `./tests/validate.sh` y corregir cualquier error de `py_compile` o `bash -n` hasta que pase limpio.
- [x] 5.3 Revisar que no existen regressions en los tests existentes de `tests/test_recipe_schema.py`, `tests/test_sdd.py` (nuevos) y el resto del suite.
