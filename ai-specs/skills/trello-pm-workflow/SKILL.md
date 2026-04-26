---
name: trello-pm-workflow
description: >
  Establece el contrato de cards de Trello para el proyecto ai-specs-cli.
  Trigger: When creating, updating, or reviewing a Trello card for project work.
license: MIT
metadata:
  author: parada1104
  version: "1.0"
  scope: [root]
  auto_invoke:
    - "Creating a Trello card"
    - "Updating a Trello card"
    - "Planning work with Trello"
---

# trello-pm-workflow — Contrato de Cards Trello

Este skill define cómo estructurar cards de Trello para el proyecto `ai-specs-cli`. 
Es dogfooding: lo usamos nosotros primero, luego lo promovemos a recipe/capability.

**Separación de responsabilidades:**
- **Trello** = estado, tracking, visibilidad PM/CTO
- **Vault** (Obsidian) = memoria canónica, decisiones, contexto estructurado
- **OpenMemory** = memoria operativa de sesión
- **OpenSpec** = specs, changes, artifacts

---

## Tipos de Card

Cada card DEBE tener un tipo. El tipo determina template, checklist y flujo.

| Tipo | Cuándo usar | Checklist base |
|------|-------------|----------------|
| `epic` | Grupo de cards relacionadas, milestone | Definir cards hijas, orden, criterio de cierre |
| `feature` | Nueva capability, comando, skill | Tests, docs, validación, sync specs |
| `bug` | Regresión, fix necesario | Reproducción, fix, test regresión, verify |
| `spike` | Investigación, experimento | Hipótesis, experimento, conclusión, decisión go/no-go |
| `decision` | ADR, decisión arquitectónica | Análisis, tradeoffs, doc en vault, consecuencias |
| `handoff` | Cierre de sesión, contexto para siguiente | Resumen, archivos, próximos pasos, riesgos |

---

## Template Base (todos los tipos)

```markdown
## Contexto
<!-- Por qué existe esta card. Problema u oportunidad. -->

## Objetivo
<!-- Resultado esperado en una oración. -->

## Alcance (In Scope)
<!-- Lista de entregables concretos -->
- [ ] Entregable 1
- [ ] Entregable 2

## Fuera de alcance (Out of Scope)
<!-- Qué NO incluye esta card -->

## Criterios de aceptación (Definition of Done)
- [ ] Criterio 1
- [ ] Criterio 2

## Dependencias
<!-- Cards o cambios bloqueantes -->

## Notas
<!-- Decisiones, links, contexto adicional -->
```

---

## Templates por Tipo

### epic

```markdown
## Contexto
Visión de alto nivel. Qué problema resuelve este EPIC.

## Objetivo
Resultado medible al cerrar el EPIC.

## Alcance
- [ ] Card hija 1
- [ ] Card hija 2

## Criterios de cierre
- [ ] Todas las cards hijas en Done
- [ ] Documentación actualizada
- [ ] Demo/validación realizada

## Dependencias
- EPIC previo / card bloqueante

## Notas
Roadmap, prioridad, stakeholders.
```

### feature

```markdown
## Contexto
Qué falta en el producto. Por qué ahora.

## Objetivo
Qué capability se entrega.

## Alcance
- [ ] Implementación
- [ ] Tests
- [ ] Documentación

## Fuera de alcance
Features futuras, optimizaciones.

## DoD
- [ ] Tests pasan (`./tests/validate.sh`)
- [ ] Docs actualizados (README, spec, design)
- [ ] AGENTS.md sync si hay skills nuevos
- [ ] Verificación completa

## OpenSpec / SDD (si aplica)
- **Change**: `openspec/changes/<nombre>/`
- **Artifacts**: [ ] proposal [ ] design [ ] specs [ ] tasks

## Dependencias
- Cards bloqueantes

## Notas
Decisiones técnicas, tradeoffs.
```

### bug

```markdown
## Contexto
Cómo se reproduce. Impacto.

## Objetivo
Fix + prevención de regresión.

## Alcance
- [ ] Reproducir bug
- [ ] Identificar causa raíz
- [ ] Implementar fix
- [ ] Test de regresión

## DoD
- [ ] Bug no se reproduce
- [ ] Test de regresión pasa
- [ ] No introduce nuevos bugs

## Notas
Logs, contexto, links relacionados.
```

### spike

```markdown
## Contexto
Incertidumbre a resolver. Hipótesis.

## Objetivo
Conclusión go/no-go con evidencia.

## Alcance
- [ ] Definir hipótesis
- [ ] Ejecutar experimento
- [ ] Documentar hallazgos
- [ ] Decisión: ¿proseguir?

## DoD
- [ ] Conclusión documentada (vault o card)
- [ ] Próximos pasos claros (si aplica)

## Notas
Experimentos fallidos también son válidos.
```

### decision

```markdown
## Contexto
Qué decisión se necesita. Por qué ahora.

## Opciones consideradas
1. Opción A — pros/cons
2. Opción B — pros/cons

## Decisión
Qué se eligió.

## Consecuencias
Qué cambia. Riesgos.

## DoD
- [ ] ADR en vault (`decisiones/YYYY-MM-DD-{slug}.md`)
- [ ] Stakeholders informados
- [ ] Cards actualizadas si aplica

## Notas
Links a discusiones, docs.
```

### handoff

```markdown
## Contexto
Sesión que termina. Estado actual.

## Qué se hizo
- [ ] Tarea completada 1
- [ ] Tarea completada 2

## Qué falta
- [ ] Pendiente 1
- [ ] Pendiente 2

## Archivos relevantes
- `path/to/file` — qué hace

## Próximos pasos
1. Próximo paso prioritario
2. Segundo paso

## Riesgos o bloqueos
- Riesgo 1

## Memoria canónica candidata
- ¿Decisiones que van al vault?
- ¿Observaciones para OpenMemory?
```

---

## Checklists Generativas

### feature con SDD/OpenSpec

Si la feature usa OpenSpec, AGREGAR a la checklist:

```markdown
## OpenSpec / SDD
- [ ] Change creado: `openspec new change "<nombre>"`
- [ ] Proposal redactado
- [ ] Design redactado
- [ ] Specs redactados
- [ ] Tasks redactados
- [ ] Apply ejecutado
- [ ] Verify report generado
- [ ] Specs sync a main
- [ ] Change archivado
```

### feature sin SDD

```markdown
## DoD
- [ ] Implementación completa
- [ ] Tests pasan
- [ ] Docs actualizados
- [ ] Verificación manual
```

---

## Flujo de Trabajo PM/CTO

### 1. Planificación
1. Leer board (Backlog → Ready)
2. Elegir card o crear nueva con template
3. Definir dependencias

### 2. Ejecución
1. Mover card a In Progress
2. Trabajar (código, tests, docs)
3. Actualizar checklist en card

### 3. Cierre
1. Mover card a Review
2. Verificar DoD
3. Mover a Done
4. Si hay memoria canónica: escribir en vault
5. Si hay decisiones: ADR en vault

### 4. Handoff (si aplica)
1. Crear card tipo `handoff` o archivo en vault
2. Documentar estado, próximos pasos, riesgos

---

## Integración con Vault

| Qué | Dónde va |
|-----|----------|
| Estado de card | Trello |
| Resumen de sesión | Vault `sessions/` o card `handoff` |
| Decisiones arquitectónicas | Vault `decisiones/` |
| Specs técnicos | Vault `specs/` (o OpenSpec) |
| Contexto de negocio | Vault `_context/README.md` |

**Regla:** Trello es SOURCE OF TRUTH para estado. Vault es SOURCE OF TRUTH para contenido estructurado.

---

## Comando Sugerido

Cuando el usuario quiera crear una card, usar este flujo:

```
1. Preguntar tipo de card
2. Preguntar título
3. Generar descripción con template base + específico del tipo
4. Crear card en Trello (lista "Backlog" o "To Do")
5. Si es feature con SDD, ofrecer crear change de OpenSpec
```

---

## Critical Rules

1. **SIEMPRE usar template** según tipo de card
2. **SIEMPRE incluir DoD** en feature/bug/epic
3. **Opcional SDD checklist** solo si aplica OpenSpec
4. **NUNCA mezclar estado y memoria** — Trello para tracking, vault para contenido
5. **ATOMICIDAD** — una card = un cambio atómico (feature, bug, spike)
6. **EPICs agrupan cards** — un EPIC no tiene código directo, solo cards hijas

---

## Dogfooding Notes

Este skill es dogfooding para el proyecto `ai-specs-cli`. 
Se prueba aquí primero. Si funciona, se empaqueta como recipe `trello-pm`.

Board de dogfooding: https://trello.com/b/rqCk2b5M/ai-specs-dogfooding