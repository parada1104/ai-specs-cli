# plan-build-openspec-odd-align

Acercar el modo `openspec` de Plan Build a las prácticas de ODD sin abandonar sus artefactos (proposal, delta de `specs/`, `tasks.md`, `verify-report.md`, archivado y paridad). Solo diseño: esta revisión no toca código de producción.
Rama `plan-build-openspec-odd-align` desde `development` `3a4e548`. El modo `odd` entregado en #589 (`3a4e548`) no se toca. Card: pendiente de crear al aprobar el diseño.

## Specs

- **S1.** Alcance: solo el modo `openspec`. *«solo el modo openspec, es decir cuando ODD no existe. El modo odd entregado en #589 no se toca. El comando de migración odd↔openspec es otro change.»*
- **S2.** Objetivo: *«mejorar un poco el flujo actual de plan-build-flow centrado en openspec, pero más acercando lo que ya tenemos a odd»*.
- **S3.** Sin código en esta vuelta: *«Esta vuelta es SOLO DISEÑO: no toques código.»*
- **S4.** Se conservan los artefactos de openspec: proposal, `tasks.md`, `specs/` y archivado. Ninguna propuesta puede sustituirlos por el árbol `odd/`.
- **S5.** Forma del plan, si se adopta: *«tasks.md con Specs S# que citan verbatim, Tasks enlazadas a S# y Log»*.
- **S6.** Prácticas candidatas a evaluar, una por una con valor y costo: *«commit o evidencia por tarea; verificación por S#; commits por unidad de trabajo; el gate y premerge usando el mismo resolvedor»*.
- **S7.** Cada propuesta declara *«qué piezas del CLI o del SKILL cambian y cómo se mantiene determinista desde el CLI»*, y cuáles se recomiendan.
- **S8.** La evidencia del change anterior se commitea como primer commit: *«el cambio sin commitear en odd/tasks/plan-build-odd-companion.md es la evidencia final del change anterior (#589, ya mergeado). Commitéalo como primer commit: docs, conventional commit.»*

## Divergencias actuales entre ODD y el modo `openspec`

Evidencia leída en esta rama; el spot-check se hizo sobre los tres puntos marcados **(v)**.

| Práctica ODD | Cómo está hoy en `openspec` | Dónde |
| --- | --- | --- |
| Identidad del change derivada de la rama | El gate abre si existe **cualquier** `openspec/changes/*/tasks.md`; ignora rama y slug **(v)** | `internal/cli/planbuildgate.go:282` |
| Un único predicado de plan por modo | `PlanIsValid` solo se usa en `odd`; `openspec` no pasa por él | `internal/planbuild/pair.go:65`, `internal/gates/gates.go:167` |
| `## Specs` con citas verbatim | No existe. El `tasks.md` es libre; lo único que el código lee es `Depth:` y `## Tracker` | `internal/cli/gates.go:30`, `internal/tracker/ledger.go:252` |
| `## Tasks` con `S#`, ruta y commit | No existe formato; hay ejemplos P0–P5 y un "Review Workload Forecast" en archives | `openspec/changes/archive/2026-08-02-plan-build-delivery-contracts/tasks.md` |
| `## Log` con `L1` verbatim | No existe: las correcciones del humano se pierden al archivar | — |
| Verificación por requisito | Solo tier `full`, y contra `## Success Criteria` de `proposal.md`/`design.md` **(v)** | `internal/gates/criteria.go:26`, `internal/gates/evidence.go:127` |
| Commits por tarea / por unidad de trabajo | Un solo mandato: "Commit and push implementation **and** planning files" | `skills/plan-build-flow/SKILL.md:332` |
| Una tarea a la vez | No se menciona en ninguna sección | `SKILL.md` (sin coincidencias de "one task"/"per task") |
| Gate y premerge con el mismo resolvedor | Comparten el resolvedor de **modo** (`planbuild.ResolveRepo`); la identidad del change es un `<slug>` a mano, y en premerge el archivado es obligatorio | `internal/cli/gates.go:148-155` |

Lo que **sí** está alineado y no hay que inventar: tier mínimo por profundidad, `verify-report.md` con veredicto/comando/commit/fecha, paridad de promoción de deltas, y el guardián de premerge. En `odd` eso se sustituye por el plan emparejado; en `openspec` es su contrato y se queda.

## Propuestas

Formato: valor, costo y recomendación. Los números de línea son de esta rama.

### P0 — Un resolvedor de change para ambos modos *(recomendada)*

`internal/planbuild` gana la identidad del change en los dos modos, con la misma forma que ya usa `odd`:

- `Change(mode, repoRoot, branch)` → nombre del change, derivado de `Branch` + `ChangeFromBranch`.
- `PlanPath` pasa a ser consciente del modo: `odd/tasks/<change>.md` u `openspec/changes/<change>/tasks.md`.
- `PlanIsValid` sigue siendo el **único** predicado (fichero regular y contenido en el repo) y ahora lo usan el gate, el premerge y `specs promote` en ambos modos.
- Regla de desambiguación, para no romper el uso actual:
  1. si la rama empareja con un change existente, ese change;
  2. si la rama no empareja y hay **exactamente un** change activo, ese (comportamiento de hoy);
  3. si hay dos o más activos y la rama no empareja, **bloqueo** nombrando la regla de emparejamiento y los candidatos. La ambigüedad es el error, no la ausencia.
- `--slug`/`<slug>` sigue aceptándose y gana como override explícito sobre (2) y (3).

**Valor.** Cierra el agujero de determinismo más grande que tiene el modo: hoy un `tasks.md` de otro change satisface el gate de la rama actual, y el premerge depende de un slug escrito a mano en la línea de comandos. Con P0 la identidad del change es una función del repo, no del historial de la shell: la misma función decide en el hook, en el premerge y en la promoción. Es además el gesto más "ODD" que existe — ODD identifica el change por la rama — y no toca ni un artefacto.

**Costo.** Medio. Cambia la semántica de `plan-build-gate` (puede bloquear donde antes pasaba), toca tres llamadores y pide tests de emparejamiento en ambos modos. Riesgo real: ramas con nombres que no son el change (hay repos así) y el caso (3) bloqueando trabajo legítimo. Mitigación: la escalera de tres pasos de arriba, y que el mensaje de bloqueo liste los candidatos y la regla, nunca solo "no encuentro el plan".

### P1 — Contrato formal de `tasks.md`: Specs `S#` / Tasks / Log *(recomendada)*

Gramática única, versionada en el SKILL y fijada con un golden test:

```text
## Specs
- **S1.** <requisito en una línea> — *"<cita verbatim del humano>"*

## Tasks
| ID | Spec | Tarea | Ruta | Commit |
| T1 | S1, S2 | <qué se hace + sus tests> | worker | pendiente |

## Log
- **L1** — <petición original del humano, verbatim>
```

- `S#` cita la petición **del humano**, no el `proposal.md`: el valor de ODD es no parafrasear al humano, y el proposal ya es texto del agente.
- Cuando el tier produce delta de `specs/`, la celda `Spec` cita además el requirement del delta; el delta sigue siendo la capacidad normalizada y el `S#` la petición. No se duplica el texto.
- `L1` es obligatorio; cada corrección posterior del humano se añade verbatim, como en ODD.
- `Log` va último, y el handoff a un worker dice `Spec: openspec/changes/<c>/tasks.md, T2, S3-S4`.

**Valor.** Es el mismo artefacto cognitivo que ODD, así que cambiar de modo no cambia lo que el agente tiene que saber hacer. Devuelve la trazabilidad de correcciones que hoy se pierde al archivar, y hace el plan gradeable (habilita P2 y P3). Sin este contrato, las demás propuestas no tienen dónde leer.

**Costo.** Bajo en Go (un parser de ~120 líneas y un golden), medio en prosa: reescribe §4, §6 y §7.2 del SKILL y la sección de readiness. Costo de adopción para quien ya tiene changes viejos: los archives existentes no se reescriben — el parser solo se aplica al change activo.

### P2 — Verificación por `S#`, generalizando el grader que ya existe *(recomendada)*

`internal/gates/criteria.go` ya resuelve exactamente este problema para `full`: una fuente autoritativa de requisitos (`## Success Criteria`), una fila `Criterion N: PASS` por requisito y rechazo de duplicados. La propuesta no añade un grader nuevo, **generaliza el que existe**:

- la fuente de requisitos es `## Specs` de `tasks.md` cuando existe (modo `openspec`), y `## Success Criteria` cuando no;
- el mapeo acepta `S1: PASS` o `Criterion 1: PASS`, así los `verify-report.md` históricos siguen válidos y `full` no se degrada;
- `verify-report.md` gana `S<n>: PASS|FAIL — <evidencia>` para todos los tiers, no solo `full`.

**Valor.** La verificación deja de ser un boilerplate de tier y pasa a estar anclada a lo que el humano pidió, que es justo la práctica de ODD que más se nota. Y se hace sin un segundo grader: la lección de #589 (L9) fue que un predicado copiado se corrige en un sitio y no en el otro.

**Costo.** Medio. Hay que cambiar una función que hoy tiene un fallback delicado (`proposal.md` autoritativo, `design.md` solo si falta el proposal) y ampliar sus tests. Riesgo: un `tasks.md` con `## Specs` mal formado pasaría a ser la fuente autoritativa y podría relajar la verificación; se mitiga con el parser estricto de P1 — si `## Specs` existe y no parsea, es bloqueo, nunca fallback.

### P3 — Commits por tarea y por unidad de trabajo *(recomendada)*

Hoy `§7.3` dice una sola cosa: commitear implementación y planificación juntas. Se sustituye por:

- un commit por fila de `## Tasks`, o por unidad de ~400 líneas cambiadas cuando una tarea lo exceda, con Conventional Commit y tests+docs junto al comportamiento;
- el sha se escribe en la celda `Commit` de esa fila;
- el premerge comprueba que toda celda `Commit` distinta de `pendiente` es un sha que existe y es ancestro de `HEAD`.

**Valor.** Rebanadas revisables, `git bisect` útil y evidencia de "esto se entregó como se planeó". El commit por tarea es la forma más barata de tener evidencia por tarea: no hace falta ningún artefacto nuevo, es el mismo sha que ya pide el `verify-report.md`.

**Costo.** Bajo: prosa en §7.3 más unas 30 líneas de Go (`git merge-base --is-ancestor`). Dos límites que hay que declarar y no esconder: (a) la comprobación de ancestro vale solo en la rama de review, porque un squash-merge invalida los shas en `development`; (b) el CLI no puede juzgar si una unidad de trabajo es "razonable", así que la granularidad se queda en prosa y el CLI solo comprueba el registro, no el juicio.

### P4 — `Log` con gate de presencia *(no recomendada como check)*

Se descarta añadir un check de "existe `## Log` y `L1` no está vacío". Presencia de un encabezado no es evidencia de nada, y un check débil que se puede satisfacer con una línea invita a satisfacerlo con una línea. El `Log` entra en el contrato (P1) y se queda en prosa: lo revisa el humano y la revisión nativa, no el CLI.

### P5 — `gates workunits`: contar commits y líneas por tarea *(no recomendada)*

Un verbo que gradúe la forma del historial (¿cuántos commits por tarea?, ¿cuántas líneas por commit?) es caro, frágil y se equivoca con los casos legítimos: un refactor de una tarea puede tocar dos ficheros, y una tarea de infraestructura puede ser un commit de 500 líneas con razón. Se queda fuera. La parte determinista y honesta de esta idea ya está en P3.

### P6 — Importar la ceremonia ODD completa en `openspec` *(fuera de alcance)*

Candidato de revisión = un commit, línea `Risk: item N` obligatoria, espejo del documento en memoria, `gentle-ai-verify` obligatorio en alto riesgo. Todo eso existe para el switch de RDD y para usuarios que corren ODD. El modo `openspec` existe precisamente para quien no lo hace, y `S4` prohíbe sustituir sus artefactos. No se trae.

### Lo que no se toca

Archivado con fecha, paridad de deltas contra `openspec/specs/`, mínimos por tier, `verify-report.md` como prueba de entrega y el guardián de premerge. `odd` queda byte a byte como está en `3a4e548`; el comando de migración odd↔openspec es otro change (`S1`).

## Qué pieza cambia y cómo sigue siendo determinista

| Pieza | Archivo | Cambio | Determinismo |
| --- | --- | --- | --- |
| Resolvedor | `internal/planbuild/pair.go`, `mode.go` | `Change`; `PlanPath` por modo; `PlanIsValid` compartido | Identidad del change = función de rama + repo. Misma función en gate, premerge y promote. |
| Gate | `internal/cli/planbuildgate.go` | `planBuildGateHasPlan` pasa a change emparejado + `PlanIsValid`; bloqueo por ambigüedad con candidatos | Sí: deja de depender de que exista "algún" plan. |
| Premerge | `internal/cli/gates.go`, `internal/gates/gates.go` | El `<slug>` es override; por defecto sale del resolvedor | Sí: un solo camino de resolución, el del hook. |
| Parser del plan | `internal/gates/tasks.go` (nuevo) | Gramática `## Specs` / `## Tasks` / `## Log`, estricta, con golden | Sí, y es la base de P2 y P3. Un solo parser, sin copia en el hook. |
| Evidencia | `internal/gates/criteria.go` | La fuente de requisitos pasa a ser `Specs` cuando existe; mapeo `S#` o `Criterion N` | Sí: cuenta filas contra requisitos declarados. |
| Promoción | `internal/specs/promote.go` | Slug por resolvedor, arg como override | Sí: reuse de `PlanIsValid`/`SourceContained`. |
| SKILL | `skills/plan-build-flow/SKILL.md` | §4 (contrato de `tasks.md`), §6 (readiness), §7.2 (PR gate), §7.3 (commits), §7.4 (bloqueos), §8 (slug ← rama) | La prosa describe lo que el CLI comprueba; no introduce reglas paralelas. |
| README / docs | `catalog/recipes/plan-build-flow/README.md`, `docs/go-migration-parity-contract.md` | Superficies nuevas y límites declarados de P3 | — |

Regla de reparto que se propone dejar escrita: **lo que el CLI puede comprobar sin juzgar historial se comprueba en Go; lo que requiere juicio se queda en prosa y no se simula con un check débil.** Aplicado, quedan deterministas la identidad, la validez del plan, la gramática, el `S#`↔`PASS`, el sha ancestro, la paridad y el archivado; quedan en prosa la redacción verbatim, la granularidad de la unidad de trabajo, la ruta (`worker`/`inline`) y el momento de escribir las tareas.

## Tasks

Diseño solamente (`S3`); la implementación es la lista de abajo, pendiente de aprobación.

| ID | Spec | Tarea | Ruta | Commit |
| --- | --- | --- | --- | --- |
| T1 | S8 | Commit de la evidencia final de #589 en `odd/tasks/plan-build-odd-companion.md` | inline | `d007708` |
| T2 | S1–S7 | Este documento: divergencias, P0–P6 con valor/costo, recomendación y mapa CLI/SKILL | inline | pendiente |
| T3 | S3, S7 | Commit del diseño | inline | pendiente |

## Implementación propuesta (change siguiente, no esta vuelta)

Cuatro unidades de trabajo, en este orden; cada una deja el suite en verde y admite revisión sola.

| ID | Propuesta | Contenido | Riesgo a declarar |
| --- | --- | --- | --- |
| I1 | P0 | `Change`, `PlanPath` por modo, gate y premerge con el resolvedor único, tests de emparejamiento y de ambigüedad | item 3: cambia la semántica de un gate que ya consumen los repos en `openspec` |
| I2 | P1 | `internal/gates/tasks.go`, golden de la gramática, SKILL §4/§6/§8 | item 3: formato nuevo para el mismo artefacto |
| I3 | P2 | `criteria.go` generalizado, `verify-report.md` con `S#`, compatibilidad `Criterion N` | item 3: reglas de entrega ya consumidas |
| I4 | P3 | Prosa de commits por tarea, check de ancestro en premerge, límites documentados | item 3: el premerge gana un bloqueo nuevo |

## Puntos abiertos (decisiones de Robert)

- **A. Pairing en `openspec`.** ¿La escalera de P0 (rama → único activo → bloqueo por ambigüedad), o emparejamiento estricto que bloquee cualquier rama sin change homónimo? Recomiendo la escalera: es determinista y no rompe a quien nombra ramas a su manera.
- **B. `S#` frente al delta.** Verificado: `S#` cita la petición del humano y el `Spec` cita además el requirement del delta cuando existe. ¿Basta, o quiere que el `S#` sea la única fuente y el delta se derive de él?
- **C. Alcance del check de `Commit`.** Solo en la rama de review, porque un squash invalida los shas. ¿Aceptable, o prefiere que el check también corra post-merge ignorando filas cuyo sha no sea ancestro?
- **D. Forma de entrega.** Un change con las cuatro unidades (I1–I4) o dos changes separados (resolvedor+contrato, después verificación+commits). Recomiendo uno con cuatro commits: comparten el parser.

## Log

- **L1** — Petición original, verbatim: *«Cody: sesión nueva para un change nuevo. - Worktree: esta. Rama: plan-build-openspec-odd-align, desde development 3a4e548. - Primero: el cambio sin commitear en odd/tasks/plan-build-odd-companion.md es la evidencia final del change anterior (#589, ya mergeado). Commitéalo como primer commit: docs, conventional commit. Objetivo, en palabras de Robert: «mejorar un poco el flujo actual de plan-build-flow centrado en openspec, pero más acercando lo que ya tenemos a odd». Alcance: solo el modo openspec, es decir cuando ODD no existe. El modo odd entregado en #589 no se toca. El comando de migración odd↔openspec es otro change. Recupera contexto en Engram (obs 502 y 503, el cierre de #589) y en el contrato ODD de Gentle Shell (contracts/review-provider-contract-mirror/.../routing/pi.md). Esta vuelta es SOLO DISEÑO: no toques código. Propón qué prácticas de ODD llevar al modo openspec sin dejar de usar sus artefactos (proposal, tasks.md, specs y archivado). Por ejemplo: - tasks.md con Specs S# que citan verbatim, Tasks enlazadas a S# y Log; - commit o evidencia por tarea; - verificación por S#; - commits por unidad de trabajo; - el gate y premerge usando el mismo resolvedor. Para cada propuesta da su valor y su costo, y cuáles recomiendas. Indica qué piezas del CLI o del SKILL cambian y cómo se mantiene determinista desde el CLI. Escribe el plan en odd/tasks/plan-build-openspec-odd-align.md, porque este repo ya está en modo odd. Commit del plan permitido, sin push. Reporte corto con las propuestas y tu recomendación. Cierra con 'LISTO DISEÑO'.»*
- **L2** — Contexto recuperado de Engram: obs 502 (entrega de #589, decisiones, pendientes reales) y obs 503 (resumen de sesión, próximos pasos). El contrato ODD está en `/Users/robert/.gentle-shell/…/gentle-pi/assets/orchestrator-memory.md` (orden del documento, `S#` verbatim, `L1`, columnas del plan) y `assets/orchestrator.md:27-29` (línea `Risk:`); el `routing/pi.md` citado en la petición no existe en esa ruta del paquete instalado.
- **L3** — Evidencia de código, spot-check propio sobre las tres afirmaciones decisivas: `planbuildgate.go:282` (el gate abre con cualquier `tasks.md`), `criteria.go:26` (una sola fuente de requisitos, `proposal.md` autoritativo), `gates.go:100-160` (premerge exige archivado y recibe el slug por argumento). El resto del mapa lo devolvió una exploración delegada de solo lectura.
- **L4** — Decisión de esta vuelta: solo diseño; se commitea `d007708` (evidencia de #589) y este documento. Sin push, sin implementación, sin crear el árbol `odd/changes/` porque en `odd` un change sin delta pasa con su plan.
