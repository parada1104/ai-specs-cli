---
name: trello-pm-workflow
description: >
  Contrato ligero de cards Trello para proyectos ai-specs. Trigger: crear,
  actualizar o revisar cards de trabajo. Dogfooding local antes de convertirlo
  en recipe configurable.
license: MIT
metadata:
  author: parada1104
  version: "2.0"
  scope: [root]
  auto_invoke:
    - "Creating a Trello card"
    - "Updating a Trello card"
    - "Planning work with Trello"
---

# trello-pm-workflow

Trello es el tracker operativo. El runtime brief define el board, listas y MCP disponibles para el proyecto actual.

## Responsabilidades

- Trello: estado, prioridad, dependencias, visibilidad PM/CTO.
- OpenSpec: artifacts SDD cuando la card requiere cambio durable.
- Vault: decisiones y handoffs canónicos.
- OpenMemory: continuidad operativa buscable.

## Regla De Flujo

- Una sesión trabaja una solicitud explícita, una card o un change.
- Una card relevante debe mapear a un ciclo SDD cuando implica implementación, diseño durable o decisión técnica compleja.
- Una card puede bloquear otra; las dependencias deben estar explícitas en Trello.
- La card debe enlazar o nombrar su OpenSpec change cuando exista.

## Template Base

```markdown
## Contexto
<por qué existe esta card>

## Objetivo
<resultado esperado en una oración>

## Alcance
- [ ] <entregable>

## Fuera de alcance
<lo que no incluye>

## Criterios de aceptación
- [ ] <criterio verificable>

## OpenSpec / SDD
- Change: `<nombre-o-pendiente>`
- Artifacts: proposal / specs / design / tasks / apply / verify / archive

## Dependencias
- <cards o cambios bloqueantes>

## Notas
<links, decisiones, contexto>
```

## Tipos Sugeridos

- `epic`: agrupa cards; no implementa código directo.
- `feature`: capability, comando, recipe, skill o comportamiento nuevo.
- `bug`: regresión o fix con reproducción y prueba de regresión.
- `spike`: investigación con conclusión go/no-go.
- `decision`: tradeoff que debe registrarse en Vault.
- `handoff`: continuidad entre sesiones cuando no basta con la card activa.

## Checklist SDD Para Features

Agregar cuando aplique:

```markdown
- [ ] Change creado en worktree dedicado
- [ ] Proposal completo
- [ ] Specs completos
- [ ] Design completo
- [ ] Tasks completos
- [ ] Apply ejecutado
- [ ] Verify report generado
- [ ] PR/merge realizado si corresponde
- [ ] Change archivado si corresponde
```

## Cierre De Card

Antes de mover a Review/Done:

- Verificar criterios de aceptación.
- Confirmar estado de OpenSpec artifacts si hubo SDD.
- Registrar decisión/handoff en Vault si cambia el canon del proyecto.
- Guardar memoria operativa en OpenMemory solo si ayuda a próximas sesiones.
- Dejar links a PR, change, verify report o handoff.

## Reglas

- No mezclar estado y memoria: Trello rastrea estado; Vault conserva decisiones; OpenSpec conserva specs/artifacts.
- No mover una card bloqueada a apply si sus dependencias siguen abiertas.
- No hardcodear board/listas en esta skill; leerlos del runtime brief o MCP.
- Esta skill es dogfooding local. La parametrización reusable pertenece a la futura recipe configurable.
