# Pre-Apply Analysis: implementar-recipe-list-add

> **Change**: `implementar-recipe-list-add`
> **Worktree**: `.worktrees/implementar-recipe-list-add`
> **Branch**: `feat/implementar-recipe-list-add`
> **Base**: `development` (3c1a891)
> **Date**: 2026-04-26
> **Artifacts**: 4/4 complete (proposal, design, specs/recipe-cli, tasks)

---

## 1. Executive Summary

Este cambio compacta las cards Trello #21 (`recipe list`) y #22 (`recipe add`) en un único ciclo SDD. Agrega dos comandos CLI que cierran el ciclo de UX de recipes: descubrimiento (`list`) e instalación declarativa (`add`). El scope es puramente aditivo — no modifica specs existentes ni rompe backward compatibility.

**Complejidad estimada**: Media. Reutiliza módulos Python existentes (`recipe-read.py`, `recipe_schema.py`) y sigue el patrón arquitectónico establecido (`bin/ai-specs` → `lib/*.sh` → `lib/_internal/*.py`).

**Effort estimado**: 6-8 horas de implementación + 2-3 horas de testing y validación.

---

## 2. Artifact Inventory & Quality Assessment

| Artifact | Status | Quality | Notes |
|----------|--------|---------|-------|
| `proposal.md` | ✅ | Alto | Motivación clara, scope bien delimitado, impacto documentado. |
| `design.md` | ✅ | Alto | Arquitectura coherente con el codebase. Flujo de datos explícito. Decisiones documentadas. |
| `specs/recipe-cli/spec.md` | ✅ | Alto | 5 requerimientos, 13 escenarios. Cobertura de happy path + errores + edge cases. |
| `tasks.md` | ✅ | Alto | 6 fases, 28 tareas checkbox. Ordenación por dependencias. Tests separados por comando. |

**Hallazgo positivo**: Los artifacts no duplican lógica de specs existentes (`recipe-schema`, `recipe-manifest-contract`, `recipe-sync-materialization`). En cambio, se apoyan sobre ellos — este cambio es consumidor de contratos ya establecidos.

---

## 3. Technical Architecture Analysis

### 3.1 Componentes nuevos

```
bin/ai-specs              +1 caso: recipe → lib/recipe.sh
lib/recipe.sh             NUEVO — sub-dispatcher
lib/recipe-list.sh        NUEVO — wrapper bash
lib/recipe-add.sh         NUEVO — wrapper bash
lib/_internal/recipe-list.py   NUEVO — lógica de listado
lib/_internal/recipe-add.py    NUEVO — lógica de escritura en manifest
```

### 3.2 Dependencias del codebase existente

| Módulo existente | Uso planeado | Riesgo |
|------------------|--------------|--------|
| `recipe-read.py` | Parsear `recipe.toml` del catálogo | Bajo — API estable, tests verdes |
| `recipe_schema.py` | Validar estructura de recipes | Bajo — dataclasses probadas |
| `toml-read.py` | Leer `ai-specs.toml` para detectar `[recipes.*]` | **MEDIO** — necesita verificar que expone lectura de secciones dinámicas |

**⚠️ Riesgo identificado**: `lib/_internal/toml-read.py` hoy se usa principalmente para leer secciones conocidas (`[project]`, `[agents]`, `[[deps]]`, `[mcp.*]`). La lectura de `[recipes.*]` (tablas dinámicas con ID variable) requiere verificar si `toml-read.py` soporta enumeración de tablas con wildcard o si `recipe-list.py` debe usar `tomllib` directamente.

### 3.3 Patrón arquitectónico

El diseño propuesto (Bash dispatcher → Bash wrapper → Python helper) es **consistente** con:
- `doctor` → `lib/doctor.sh` → `lib/_internal/doctor.py`
- `sync` → `lib/sync.sh` → múltiples `.py`
- `sdd` → `lib/sdd.sh` → `lib/_internal/sdd.py`

**Sin desviaciones arquitectónicas detectadas**.

### 3.4 Mutaciones de estado

| Comando | Lectura | Escritura | Idempotencia |
|---------|---------|-----------|--------------|
| `recipe list` | `catalog/recipes/*`, `ai-specs.toml` | Ninguna | Sí (read-only) |
| `recipe add` | `catalog/recipes/<id>`, `ai-specs.toml` | `ai-specs.toml` | Sí (falla limpio si duplicado) |

---

## 4. Spec Compliance Audit

### 4.1 Specs existentes — impacto

| Spec existente | Relación | ¿Este cambio la respeta? |
|----------------|----------|--------------------------|
| `recipe-schema` | Consume su formato de `recipe.toml` | ✅ Sí — usa `recipe-read.py` |
| `recipe-manifest-contract` | Consume su contrato `[recipes.<id>]` | ✅ Sí — escribe `enabled` + `version` exacta |
| `recipe-sync-materialization` | No materializa (diseño explícito) | ✅ Sí — `add` solo declara, no copia |
| `manifest-contract` | Escribe en `ai-specs.toml` | ✅ Sí — respeta campos mínimos `enabled` + `version` |
| `sdd-cli-integration` | Ninguna | N/A |

### 4.2 Nueva spec — `recipe-cli`

**Cobertura de escenarios**: 13 escenarios distribuidos en 5 requerimientos.

| Requerimiento | Escenarios | Tipo | Observación |
|---------------|------------|------|-------------|
| `recipe list` | 3 | Happy + Edge + Error | ✅ Completo |
| `recipe add` | 4 | Happy + Error x3 | ✅ Completo |
| Integración `recipe-read.py` | 1 | Edge (TOML inválido) | ✅ Bien pensado |
| No materialización | 1 | Happy (negativo) | ✅ Crítico para no romper contrato sync |
| Idempotencia | 1 | Edge | ✅ Previene duplicados |

**Gap menor detectado**: No hay escenario explícito de `recipe add` con `[recipes.<id>]` existente pero `enabled = false` (recipe deshabilitada). El diseño dice "abortar si ya existe", lo cual cubre ambos casos (`enabled = true` o `false`), pero la spec podría explicitarlo. **No bloqueante**.

---

## 5. Test Strategy Assessment

### 5.1 Tests planificados en tasks.md

| Suite | Tests | Tipo | Cobertura estimada |
|-------|-------|------|-------------------|
| `test_recipe_list.py` | 6 | Unit + Integration | Alto — catálogo vacío, estados, inválido, CLI |
| `test_recipe_add.py` | 7 | Unit + Integration | Alto — happy path, duplicado, inexistente, no-sync, preview, idempotencia |

**Total**: 13 tests nuevos. Con los ~19 tests existentes, el proyecto quedaría con ~32 tests.

### 5.2 Riesgo de testing

- **Fixtures**: El catálogo de test `catalog/recipes/test-fixture/` ya existe y es rico (skills, commands, mcp, templates, docs). Se puede reutilizar.
- **Proyecto de prueba**: Se necesitará un `tests/fixtures/minimal-project/` con `ai-specs.toml` para pruebas de integración. No existe hoy, pero `tests/fixtures/` ya es un patrón usado por `test_sync_pipeline.py`.
- **Mocking de TOML**: `recipe-add.py` debe escribir en `ai-specs.toml` preservando el resto del archivo. Esto requiere un parser/serializer TOML que conserve comentarios y formato. **Python `tomllib` no escribe**. Hay que evaluar si usar `tomli_w` o hacer append manual controlado.

---

## 6. Open Questions & Decision Points

### 🔴 Pregunta abierta #1: Escritura TOML en `recipe add`

**Problema**: `recipe-add.py` necesita modificar `ai-specs.toml` agregando `[recipes.<id>]` sin destruir comentarios ni formato existente.

**Opciones**:
1. **Append manual controlado** — Leer el archivo como texto, detectar el final, appendear la nueva sección con formato fijo. Riesgo: frágil si el archivo no termina en newline.
2. **`tomli_w`/`tomlkit`** — Dependencia nueva. `tomli_w` es lightweight (~2KB) y complementa `tomllib`. `tomlkit` preserva comentarios pero es más pesado. El proyecto hoy tiene cero dependencias externas de Python.
3. **Template-based** — No aplica; no queremos regenerar todo el manifest.

**Recomendación**: Opción 1 (append manual) para MVP, con validación de que la sección no existe previamente. Documentar que `recipe add` solo appendea al final del archivo. Esto es consistente con la filosofía "zero deps" del proyecto.

**Impacto en tasks**: Agregar tarea 3.5a: "Validar que ai-specs.toml termina en newline antes de append; si no, agregar newline."

---

### 🟡 Pregunta abierta #2: Lectura de `[recipes.*]` desde TOML

**Problema**: `recipe-list.py` necesita enumerar todas las tablas `[recipes.*]` del manifest para determinar estado.

**Opciones**:
1. **Usar `tomllib` directamente** — `data = tomllib.load(f)` → iterar `data.keys()` filtrando strings que empiecen con `recipes.`. Simple y funciona con tablas TOML estándar.
2. **Extender `toml-read.py`** — Agregar función `get_recipe_tables()` al módulo existente. Más reusable pero requiere modificar código existente.

**Recomendación**: Opción 2 — Extender `toml-read.py` con `list_recipe_tables(path) -> list[str]` y `get_recipe_table(path, recipe_id) -> dict`. Esto centraliza el parsing TOML y beneficia a futuros comandos. Es un cambio pequeño y bien encapsulado.

**Impacto en tasks**: Agregar tarea 2.3a: "Extender toml-read.py con funciones para leer tablas [recipes.*]". Esto reduce deuda técnica.

---

### 🟡 Pregunta abierta #3: Formato de salida de `recipe list`

**Problema**: El diseño propone tabla ASCII simple. No hay estándar previo en el codebase.

**Análisis**:
- `doctor` probablemente usa texto plano con `printf`/`echo` (a confirmar).
- Tabla ASCII manual es propenso a desalineación con IDs largos o nombres con caracteres especiales.
- TSV (tab-separated) es más fácil de generar y parsear, pero menos legible para humanos.

**Recomendación**: Usar formato simple de "lista" en lugar de tabla alineada:
```
✓ test-fixture      1.0.0   installed
  test-cmd-conflict-a  1.0.0   available
✗ bad-recipe        —       error (missing version)
```
Esto es más fácil de testear y menos frágil. La columna de estado con iconos/emoji es opcional; texto plano (`[installed]`, `[available]`) es más portable.

**Impacto en tasks**: Modificar tarea 2.5: "Implementar renderizado de lista simple (no tabla alineada) a stdout".

---

### 🟢 Pregunta abierta #4: `recipe add` y version pinning

**Problema**: El diseño dice que `recipe add` escribe la versión exacta del catálogo. ¿Qué pasa si la recipe en el catálogo cambia de versión después?

**Análisis**:
- El contrato `recipe-manifest-contract` ya define que `sync` valida version pinning y falla si hay mismatch.
- `recipe add` no tiene que resolver esto; solo declara la versión que ve en el momento del add.
- El usuario puede editar manualmente después.

**Recomendación**: Sin acción. El comportamiento propuesto es correcto y consistente con `recipe-manifest-contract`.

---

## 7. Risk Matrix

| Riesgo | Probabilidad | Impacto | Mitigación |
|--------|-------------|---------|------------|
| Escritura TOML corrompe `ai-specs.toml` | Media | Alto | Append manual controlado + tests con fixtures + backup del archivo original en tests |
| `toml-read.py` no soporta tablas dinámicas | Baja | Medio | Usar `tomllib` directamente como fallback; extender `toml-read.py` es trivial |
| Tabla ASCII se desalinea | Media | Bajo | Cambiar a lista simple (recomendación #3) |
| Tests de integración requieren proyecto de prueba | Alta | Bajo | Crear `tests/fixtures/minimal-project/` con `ai-specs.toml` mínimo (reutilizar patrón existente) |
| Scope creep hacia `recipe remove` o `recipe sync` | Baja | Medio | Documentar explícitamente en design.md que están fuera de scope; rechazar en review |

---

## 8. Recommendations

### 8.1 Before Apply

1. **✅ Resolver Pregunta #1 (escritura TOML)**: Decidir append manual vs. librería. Documentar decisión en `design.md`.
2. **✅ Resolver Pregunta #2 (lectura TOML)**: Extender `toml-read.py` con helpers para `[recipes.*]`. Es 5-10 líneas de código y reduce deuda.
3. **✅ Resolver Pregunta #3 (formato de salida)**: Confirmar formato simple vs. tabla. Recomiendo lista simple.
4. **✅ Verificar `doctor.py`**: Confirmar cómo formatea salida para mantener consistencia visual.

### 8.2 During Apply

1. **TDD estricto**: Implementar tests antes de código para tasks 4.x y 5.x. Esto valida el contrato spec desde el principio.
2. **Commits pequeños**: Un commit por fase (Infrastructure → List → Add → Tests → Polish).
3. **No modificar catalog/recipes/**: Los tests deben usar fixtures temporales, no mutar el catálogo real.
4. **Validar con `./tests/run.sh` después de cada fase**: No esperar al final.

### 8.3 After Apply (Before Merge)

1. Ejecutar `./tests/run.sh` y `./tests/validate.sh` en el worktree.
2. Ejecutar `ai-specs sync` en el proyecto root para regenerar `AGENTS.md` si es necesario.
3. Revisar que no quedan archivos `.pyc` ni `__pycache__` fuera de `.gitignore`.
4. Preparar PR description con referencia a la card Trello #21 (compactada).

---

## 9. Dependency Graph

```
proposal.md
    ├──→ design.md
    │       ├──→ lib/recipe.sh
    │       ├──→ lib/recipe-list.sh + recipe-list.py
    │       └──→ lib/recipe-add.sh + recipe-add.py
    │
    └──→ specs/recipe-cli/spec.md
            └──→ tasks.md
                    ├──→ Fase 1: Infrastructure
                    ├──→ Fase 2: List (depends on Fase 1)
                    ├──→ Fase 3: Add (depends on Fase 1)
                    ├──→ Fase 4: Tests List (depends on Fase 2)
                    ├──→ Fase 5: Tests Add (depends on Fase 3)
                    └──→ Fase 6: Validation (depends on Fase 4+5)
```

---

## 10. Final Verdict

| Criterio | Estado |
|----------|--------|
| Artifacts completos | ✅ 4/4 |
| Specs coherentes con existentes | ✅ |
| Diseño arquitectónicamente consistente | ✅ |
| Tests planificados con cobertura adecuada | ✅ |
| Riesgos identificados y mitigables | ✅ |
| Preguntas abiertas resolvibles pre-apply | ✅ 3/4 (una es no-acción) |

**Recomendación**: ✅ **APROBADO PARA APPLY** con la condición de resolver las 3 preguntas abiertas (#1, #2, #3) antes de empezar la fase de implementación (tasks 2.x y 3.x). La pregunta #4 es no-acción.

El cambio es bien delimitado, aditivo, y cierra un gap real de UX entre el diseño de recipes (#19) y su uso práctico. No hay breaking changes detectados.
