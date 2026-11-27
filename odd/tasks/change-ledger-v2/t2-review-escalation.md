# T2 (write gate) — cuatro rondas de revisión nativa, todas escaladas

Estado al cierre de este informe. Nada mergeado en T2.

## Estado verificado

- **T1 mergeado**: `366049b3` (#610). `ai-specs sync` posterior: **0 archivos de diff**, no hubo PR de sync.
- **T2**: 13 commits en `feat/change-ledger-t2` (tip `725059e6`), **sin push y sin merge**. `go build`, `go vet` y `go test ./...` verdes; guardian pre-merge verde.
- R3–R5 movidas a T8 por decisión técnica de Cody (L4 del plan de T2, L11 del plan maestro).
- T3 sin empezar. Sin subagentes ni jobs activos.
- Las cuatro lineages cerraron en `escalated`: no existe recibo `approved` para el candidato de T2.

## Rondas y hallazgos

| Ronda | Candidato | Hallazgos severos |
|---|---|---|
| 1 `review-89159…` | completo (R1–R5) | removed-flag-token como contrato; R2 no puntúa el path pendiente; umbral R3 bloquea un worktree documental; `--explain` ignorado en canonical spec |
| 2 `review-1d95e…` | completo | canonical fast-path se salta R2; `.toml` borrado entero invisible a R4; flag comentado dispara R4; **las salidas de R3/R4 no desbloquean** (+ REG: guarda de comentarios tapaba un registro real) |
| 3 `review-05ec1…` | recortado (R1/R2) | R2 sólo corre en ruta de producción; R1 mira el diff sin saber si el worktree es un change (+ REG: la exención workstream se saltaba R2) |
| 4 `review-e0e8b…` | recortado + fixes | R1-explain-ignored, R1-high-risk-scope, R1-workstream-diff, R3-002 corroborados por el refuter; R3-003 con causalidad desconocida → **stop** |

El proveedor sólo expone ids y refs de proof de la ronda 4 (no las claims), así que el
agrupamiento se hace por la parte del código que cada hallazgo señala.

## Agrupado por causa raíz

### 1. La aplicabilidad de una refutación se deduce de la posición en el flujo, no de sus precondiciones

El gate es una cadena de returns tempranos: agent config → artefacto → canonical spec →
árbol de planificación odd → ¿producción? → ledger → ¿workstream? → refutaciones → explain.
Cada ronda encontró una variante de lo mismo:

- r1: R2 dejó de puntuar el path pendiente al mover una guarda.
- r2: el fast-path de canonical spec lo saltaba.
- r3: fuera del alcance de producción no corría.
- r3 REG-001: la exención workstream lo saltaba.
- r4: las escrituras a artefacto y al árbol de planificación no reportan ni gradúan
  (`R1-explain-ignored`, `R1-high-risk-scope`).

Es el mismo defecto cinco veces.

### 2. El hecho S16 ("no hay nada que graduar") vive en el CLI, no en la capa de refutaciones

Cada regla lo reimplementa o lo olvida:

- r1: el umbral R3 se aplicaba a un worktree documental.
- r3: una edición heredada de canonical spec bloqueaba la siguiente escritura normal.
- r4: el orden workstream ↔ R1 del diff (`R1-workstream-diff`).

### 3. "Contrato público" decidido por la forma de una línea

- r1: token de flag removido como prueba de contrato.
- r2: flag comentado disparando R4.
- r2 REG: registro real tras un comentario de bloque siendo ignorado.
- r2: `.toml` borrado entero invisible a R4.

Esta clase se fue con R3–R5 a T8, y es la evidencia de que esas reglas necesitaban un
store de declaraciones, no un escaneo de diff.

### 4. Salidas anunciadas que no resuelven nada

- r2 `R3-001`: los mensajes ofrecían "escribe el plan o el delta" sin que ninguna regla
  los leyera. Hacerlas efectivas exigió estado de decisiones y umbrales que recién existe
  en T8.

### 5. La explicación se emite al final de un camino concreto, no por la función que decide

- r1 `R1-004`: `--explain` ignorado en la rama de canonical spec.
- r4 `R1-explain-ignored`: lo mismo en las rutas de artefacto y del árbol de planificación.

## Opciones

1. **Rediseño del seam (recomendada).** Recolectar hechos una vez —modo, change,
   ¿workstream?, tier, path pendiente, ¿producción?, high-risk declarado, entradas del
   diff, presencia de plan/delta— y expresar R1/R2 como reglas puras sobre esos hechos,
   con el `--explain` y el rechazo derivados del mismo veredicto. Es el contrato
   `Facts()`/`Verdict()` que el plan maestro ya especifica para T7. Elimina de raíz las
   causas 1, 2 y 5. R3–R5 siguen en T8 junto a los umbrales y las decisiones.
2. **Aceptar T2 como está.** Los cuatro defectos conocidos están corregidos y fijados con
   tests; se mergea documentando las cuatro escaladas. La garantía pasa a ser la suite,
   no la revisión nativa.
3. **Más parches.** Descartada por el propio patrón: cada parche reveló otra instancia de
   las causas 1–3 en la ronda siguiente.

## Recomendación

Opción 1. El patrón no es "faltan casos": es que el veredicto del gate no tiene una sola
fuente de hechos. Mientras la aplicabilidad de cada regla dependa de dónde está el
`return` en la cadena, cada guarda nueva reabre una variante del mismo defecto.

## Riesgo declarado

Los cuatro fixes de T2 están verificados por tests (RED→GREEN en ambos niveles) pero
**sin recibo de aprobación nativa**. Mergear así cambia la naturaleza de la garantía.

## Pendiente fuera de T2

- T3 (discriminador de workstream + `changes workstream`) depende de la misma decisión de
  diseño, porque toca el mismo camino de código.
- Release, bump validado, tag, instalación global y verificación sin `AI_SPECS_HOME`
  quedan sin empezar por la instrucción de parar.
- Worktree y rama quedan en su lugar para `ai-specs worktree-cleanup`.
