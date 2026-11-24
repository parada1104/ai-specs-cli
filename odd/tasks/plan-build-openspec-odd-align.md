# plan-build-openspec-odd-align

Acercar el modo `openspec` de Plan Build a ODD: rescatar `specs/` y `config.yaml`, colapsar el resto del change en **un solo archivo de plan** con forma ODD, y hacer el archivado obligatorio pre-merge en ambos modos.
Revisión 2 (L5): sustituye el alcance de la revisión 1, que mantenía `proposal.md`, `design.md` y `verify-report.md` como artefactos obligatorios. Solo diseño: no se toca código de producción.
Rama `plan-build-openspec-odd-align` desde `development` `3a4e548`. El archivado obligatorio en `odd` revierte a propósito una decisión de #589 (S10).
Card: `6b04e0e37aa95686f307241f` — [65Ijc3h5](https://trello.com/c/65Ijc3h5)

## Specs

- **S1.** Alcance: solo el modo `openspec`, más la reversión explícita de S10 en `odd`. *«solo el modo openspec, es decir cuando ODD no existe. El modo odd entregado en #589 no se toca. El comando de migración odd↔openspec es otro change.»* La reversión de #589 entra por S10 (L5); lo demás de `odd` no se toca.
- **S2.** Objetivo: *«mejorar un poco el flujo actual de plan-build-flow centrado en openspec, pero más acercando lo que ya tenemos a odd»*.
- **S3.** Sin código en esta vuelta: *«Esta vuelta es SOLO DISEÑO: no toques código.»*
- **S4.** *(reescrita por L5)* De los artefactos actuales del change sobreviven dos: el delta `specs/` y **un** archivo de plan con forma ODD. `proposal.md`, `design.md` y `verify-report.md` dejan de ser obligatorios; lo que aportaban (criterios y verificación por requisito) vive dentro del plan. Sigue prohibido sustituir el árbol de OpenSpec por el de `odd/`.
- **S5.** Forma del plan: *«tasks.md con Specs S# que citan verbatim, Tasks enlazadas a S# y Log»*.
- **S6.** Prácticas candidatas, evaluadas una por una: *«commit o evidencia por tarea; verificación por S#; commits por unidad de trabajo; el gate y premerge usando el mismo resolvedor»*.
- **S7.** Cada propuesta declara *«qué piezas del CLI o del SKILL cambian y cómo se mantiene determinista desde el CLI»*, con valor, costo y recomendación.
- **S8.** La evidencia de #589 se commitea como primer commit, docs y conventional commit. *(Hecho: `d007708`.)*
- **S9.** Un solo archivo: *«lo que debemos rescatar es specs + un solo archivo que haga lo que hace odd hoy en día pero solo specs + tasks pero bajo openspec»*. El change en `openspec/changes/<c>/` queda con `specs/` y el plan; nada más obligatorio.
- **S10.** Archivado obligatorio: *«el archive en ambos casos debe ser obligatorio pre merge»*. Aplica a `openspec` y a `odd`; en `odd` revierte la decisión de #589 de no exigir archivado.
- **S11.** Configuración del proyecto rescatada: *«specs + un solo archivo… y creo que el config.yaml [y] el archive en ambos casos debe ser obligatorio pre merge»*. `openspec/config.yaml` (schema, context, strict_tdd, testing) sobrevive como configuración del proyecto en ambos modos, y hay que mantenerlo vigente: hoy describe Bash/Python y `./tests/run.sh`, y el CLI ya es Go.
- **S12.** Ceremonia proporcional: *«también debe achicarse la ceremonia cuando no sea necesaria»*. Un change Light lleva plan corto, sin delta si no cambia reglas y sin pasos de más.
- **S13.** El merge lo decide el humano: *«el agente pregunta si es momento de ir»*. El agente no mergea por iniciativa propia; el aseguramiento va en capas para que tampoco se pueda mergear a mano sin archivado.
- **S14.** El delta y el plan son el contrato de entrega: nada de lo que se retire de obligatorio puede dejar una regla sin comprobación determinista en el CLI.

## Divergencias actuales entre ODD y el modo `openspec`

| Práctica ODD | Cómo estaba antes de esta revisión | Dónde |
| --- | --- | --- |
| Identidad del change derivada de la rama | El gate abre con **cualquier** `openspec/changes/*/tasks.md`; ignora rama y slug | `internal/cli/planbuildgate.go:282` |
| Un predicado de plan por modo | `PlanIsValid` solo se usa en `odd` | `internal/planbuild/pair.go:65` |
| Un solo documento de plan | Tres artefactos obligatorios repartidos (`proposal.md`, `design.md`, `verify-report.md`) por tier | `internal/gates/gates.go:78-105`, `evidence.go:127` |
| `## Specs` verbatim / `## Tasks` / `## Log` | No existen; el código solo lee `Depth:` y `## Tracker` | `internal/cli/gates.go:25`, `internal/tracker/ledger.go:252` |
| Verificación por requisito | Solo `full`, y contra `## Success Criteria` de `proposal.md` | `internal/gates/criteria.go:29` |
| Commits por tarea / unidad | Un solo "Commit and push implementation **and** planning files" | `skills/plan-build-flow/SKILL.md:332` |
| Archivado obligatorio | Obligatorio en `openspec`, **inexistente** en `odd` | `gates.go:134` vs `gates.go:167` |
| Configuración del proyecto vigente | `openspec/config.yaml` existe y **solo el tracker lo lee** (`tracking:`); ningún validador lo mira | `internal/doctor/checks_tracker.go:52` |
| Aseguramiento fuera del agente | Ningún workflow corre en `pull_request`; el único workflow es por tag | `.github/workflows/release-binaries.yml:14` |

## Propuestas

Numeración nueva. Las rechazadas de la revisión 1 (check de presencia de `Log`, `gates workunits`, importar la ceremonia ODD completa) pasan a **R1–R3**; ver L5.

### P0 — Un resolvedor para change, plan, config y archivo *(recomendada; A aprobada)*

`internal/planbuild` centraliza las cuatro rutas y la identidad, igual que ya hace con el modo:

- `Change(mode, repoRoot, branch)` → nombre del change, de `Branch` + `ChangeFromBranch`.
- `PlanPath` consciente del modo: `odd/tasks/<change>.md` u `openspec/changes/<change>/tasks.md`.
- `ConfigPath` consciente del modo: `odd/config.yaml` u `openspec/config.yaml`.
- `ArchivePath` consciente del modo: `odd/changes/archive/YYYY-MM-DD-<c>/` u `openspec/changes/archive/YYYY-MM-DD-<c>/`.
- `PlanIsValid` sigue siendo el **único** predicado de contención y regularidad, y ahora lo usan gate, premerge, promoción y archivado.
- Escalera de desambiguación: la rama empareja → ese change; si no empareja y hay exactamente un change activo → ese; si hay dos o más activos y la rama no empareja → **bloqueo** nombrando la regla y los candidatos.
- **`--change` como override explícito**, por encima de la escalera. No es un adorno: sin él la capa de CI de P7 no es viable (ver más abajo).

**Valor.** Cierra el agujero de determinismo (cualquier `tasks.md` satisface el gate de cualquier rama) y da una sola identidad a gate, premerge, promoción y archivado. Cuatro rutas en una función es menos código que cuatro derivaciones repartidas.

**Costo.** Medio. Cambia la semántica de `plan-build-gate`, toca tres llamadores, y la escalera nuevas necesita tests de emparejamiento y de ambigüedad. **Risk item 3**: gates ya consumidos por repos en `openspec`.

### P1 — Un solo archivo de plan por change *(recomendada; reemplaza al contrato de la revisión 1)*

`openspec/changes/<c>/` queda con dos cosas:

```text
openspec/changes/<c>/
├── specs/<dominio>/spec.md      # delta, sin cambios (S9)
└── tasks.md                     # el plan completo, forma ODD
```

El plan absorbe lo que hacían los tres artefactos, en este orden:

```text
# <change>                            (2-3 líneas de cabecera: qué, delta, card)
## Specs
- **S1.** <requisito, una línea> — *"<cita verbatim del humano>"*
## Tasks
| ID | Spec | Tarea | Ruta | Commit |
| T1 | S1, S2 | <qué se hace + sus tests> | worker | pendiente |
## Design                              (solo tier full, y solo si hay decisión que explicar)
## Verification                        (solo standard/full: una fila por S#)
- **S1: PASS** — `<comando>`, exit 0, commit `<sha>`, `<fecha>`
## Log
- **L1** — <petición original del humano, verbatim>
```

- `S#` cita al **humano**, no al `proposal.md`: el valor de ODD es no parafrasear, y el proposal ya era texto del agente.
- Cuando hay delta, la celda `Spec` cita también el requirement del delta; el `S#` es la petición, el delta la capacidad normalizada. No se duplica texto.
- `Log` va al final; `L1` es obligatorio y cada corrección posterior entra verbatim.
- `proposal.md`, `design.md` y `verify-report.md` **se siguen aceptando si están** (compatibilidad con quien los tiene y con los archives históricos), pero no se exigen. «No obligatorio» no es «prohibido».

**Valor.** Un change es un archivo que se lee de arriba abajo, igual que en ODD: es lo que Robert pidió literalmente (S9) y lo que hace que cambiar de modo no cambie lo que el agente tiene que saber. Además **simplifica** el grader: una fuente de requisitos, una de evidencia, un archivo que parsear.

**Costo.** Medio. Un parser estricto con golden (unas 150 líneas), reescritura de §4, §6 y §7.2 del SKILL, y aceptar que `## Verification` es una sección nueva que ODD no tiene (por eso va después de `## Tasks`, y solo cuando el tier la pide). **Risk item 3**: cambia el contrato de entrega para todo change `openspec`.

### P2 — Verificación por `S#`, con un solo grader *(recomendada; B aprobada y simplificada)*

`internal/gates/criteria.go` ya resuelve este problema para `full`: una fuente autoritativa de requisitos, una fila de veredicto por requisito, rechazo de duplicados. Se **generaliza**, no se duplica:

- la fuente de requisitos pasa a ser `## Specs` del plan; `## Success Criteria` de `proposal.md`/`design.md` solo se usa si `## Specs` no existe (archives viejos);
- la evidencia se lee de `## Verification` del plan; `verify-report.md` solo si `## Verification` no existe;
- el mapeo acepta `S1: PASS` y `Criterion 1: PASS`;
- si `## Specs` existe y no parsea, es **bloqueo**, nunca fallback silencioso.

**Valor.** La verificación queda anclada a lo que pidió el humano y deja de ser boilerplate de tier. Y sale más barato que en la revisión 1: sin `verify-report.md` obligatorio, el grader tiene una sola fuente en vez de dos con cadena de fallback.

**Costo.** Bajo-medio: una función con dos fuentes ordenadas y tests. **Risk item 3**: reglas de entrega ya consumidas.

### P3 — Commits por tarea y por unidad de trabajo *(recomendada; C aprobada)*

- Un commit por fila de `## Tasks`, o por unidad de ~400 líneas cambiadas cuando una tarea lo exceda; Conventional Commit; tests y docs junto al comportamiento.
- El sha se escribe en la celda `Commit` de esa fila.
- El premerge comprueba que toda celda distinta de `pendiente` es un sha que existe y es ancestro de `HEAD`.

**Valor.** Rebanadas revisables, `git bisect` útil, y evidencia por tarea sin inventar artefactos: es el mismo sha que ya pide la verificación.

**Costo.** Bajo: prosa en §7.3 y unas 30 líneas de Go (`git merge-base --is-ancestor`). Dos límites que se declaran y no se esconden: (a) el check de ancestro vale solo en la rama de review, porque un squash invalida los shas en `development`; (b) el CLI no puede juzgar si una unidad es «razonable», así que la granularidad se queda en prosa y el CLI solo comprueba el registro.

### P4 — Archivado obligatorio pre-merge en ambos modos *(recomendada; S10)*

**Qué es archivar en `openspec`** (ya existe, no cambia): promover los deltas a `openspec/specs/`, mover `openspec/changes/<c>/` (con su plan dentro) a `openspec/changes/archive/YYYY-MM-DD-<c>/`, y commitear en la rama de review (`SKILL.md:369-371`).

**Qué es archivar en `odd`** (nuevo, revierte #589): promover los deltas a `odd/specs/` y mover `odd/changes/<c>/` a `odd/changes/archive/YYYY-MM-DD-<c>/`. **`odd/tasks/<c>.md` se queda donde está**: es el plan de la feature, vive con los otros 154 planes, y es lo que citan los handoffs (`Spec: odd/tasks/<c>.md`). La asimetría con `openspec` (donde el plan sí se mueve) es real y se documenta: en `openspec` el plan pertenece al change, en `odd` pertenece a la feature.

**Premerge, ambos modos:** no debe quedar ninguna carpeta de change activa, el archivo con fecha debe existir y contener el plan y su delta, y los deltas deben estar promovidos. En `odd` eso significa que `CheckPremergeOdd` gana exactamente una condición nueva (archivo presente, change activo ausente) y conserva `PlanIsValid` + paridad. El `PlanIsValid` sobre `odd/tasks/<c>.md` sigue siendo lo que vigila el hook durante la implementación, para lo que fue construido.

**Verbo nuevo: `ai-specs changes archive [<change>]`.** Sin él, «archivar» es una instrucción en prosa que el agente puede olvidar, y la capa 1 del aseguramiento no tiene dientes. Hace: rechazar si la promoción tiene pendientes, mover la carpeta a la fecha indicada o a hoy, rechazar si ya existe un archive para ese change (misma ambigüedad que ya detecta `resolveArchive`), y usar la identidad de P0. Determinista y atómico: un solo `os.Rename` dentro del mismo árbol.

**Valor.** Los dos modos terminan igual: un change archivado, con fecha, con plan y delta promovido. Y el merge deja de depender de que alguien recuerde mover una carpeta.

**Costo.** Medio-alto, y hay que decirlo claro: **cambia código entregado ayer** (`CheckPremergeOdd`, #589), añade un verbo con tests, y toca la prosa de §7.4 y del README. **Risk item 3** por el premerge y item 1 en la medida en que mover carpetas toca evidencia versionada — mitigado porque el verbo se niega a mover si hay pendientes y nunca borra.

### P5 — `config.yaml` rescatado, y con bucle de frescura *(recomendada; S11)*

**Papel.** `config.yaml` es la configuración del **proyecto**, no del change: contexto para el agente, rigor de TDD y el comando de tests que el proyecto considera su verificación. Lo leen dos consumidores distintos y ninguno debe invadir al otro:

- el **CLI** lee solo las claves que él mismo usa: `tracking:` (hoy, `internal/doctor/checks_tracker.go:52`) y `testing.runner:` (nuevo);
- el **SKILL** lee la prosa: `context`, `strict_tdd`, `rules.*`, `ai_specs_guidance`.

**Dónde vive.** Misma regla que todo lo demás, vía `ConfigPath` de P0: `openspec/config.yaml` en `openspec`, `odd/config.yaml` en `odd`. Aviso de contradicción a resolver: `catalog/recipes/plan-build-flow/README.md:84` **excluye hoy** `odd/config.yml`, y el proposal de #589 también; esa línea hay que enmendarla o elegir un camino único para los dos modos (decisión A' de abajo).

**Qué valida el CLI, y qué no.** Solo lo que consume: que sea un fichero regular contenido en el repo (el mismo predicado de P0), que parsee, que `schema:` y `testing.runner:` no estén vacíos y que `strict_tdd`, si está, sea booleano. `context` y `rules.*` son prosa opaca: **nadie las valida**, porque un esquema estricto repetiría la trampa del oráculo congelado de `internal/schema` que ya bloqueó declarar `[config.mode]` en #589.

**Cómo se mantiene vigente** (el problema real que señala Robert: el archivo describe Bash/Python y `./tests/run.sh` mientras el CLI es Go). La causa es que nada lo lee al calificar, así que nada lo obliga a ser cierto. Se convierte su afirmación central en carga estructural:

1. cada comando de una fila de `## Verification` debe resolverse en disco (una ruta relativa tiene que existir; un comando suelto, estar en `PATH`);
2. el comando declarado en `testing.runner` debe **aparecer** entre los comandos de verificación de un change standard/full.

Con eso, un change que migre el gate de `./tests/run.sh` a `go test` no puede pasar el premerge sin actualizar `config.yaml` en el mismo change. La obsolescencia deja de ser silenciosa: **falla el gate**. Es la única forma barata y determinista que encontré.

**Opcional, no en esta entrega:** que `ai-specs init` siembre el archivo si falta y que `doctor` avise de rutas que ya no existen.

**Costo.** Bajo-medio: dos comprobaciones y una ruta más en P0. **Risk item 3** por el premerge; **item 6** si se implementa sin test del bucle de frescura.

### P6 — Ceremonia proporcional al tier *(recomendada; S12)*

Matriz que sustituye a los mínimos actuales (`gates.go:78-105`) y a la evidencia por tier (`evidence.go:127`):

| | light | standard | full |
| --- | --- | --- | --- |
| plan ODD (`## Specs` + `## Tasks`) | sí, corto: 1-3 tareas, `Log` opcional | sí, con `## Log` | sí, con `## Log` |
| delta `specs/` | solo si cambia una regla | ≥1 dominio | ≥1 dominio |
| `## Design` en el plan | no | no | sí, si hay decisión que explicar |
| `## Verification` con `S#: PASS` | no | una fila por `S#` | una fila por `S#`, veredicto estricto `PASS` |
| `config.yaml` | no | sí | sí |
| archivado obligatorio | sí | sí | sí |
| `proposal.md` / `design.md` / `verify-report.md` | no | no | no |

Lo que se **borra** de obligatorio es tanto como lo que se añade: `proposal.md` para light y standard, `design.md` para full, `verify-report.md` para standard y full. `checkMinimums` se encoge a tres reglas y la evidencia pasa a leerse de `## Verification`.

**Valor.** Es la mitad del pedido de Robert y la mitad del ahorro: menos archivos obligatorios, menos prosa ceremonial, y el tier sigue decidiendo con la misma granularidad de hoy.

**Costo.** Bajo (borrar condiciones y ajustar tests), pero cambia expectativas que ya tienen los repos en `openspec`: **Risk item 3**.

### P7 — Aseguramiento en capas *(recomendada; S13)*

1. **CLI.** `ai-specs gates premerge` bloquea si no está archivado, en ambos modos (P4). Es determinista y es lo único que no depende de que el agente obedezca.
2. **El verbo de archivado no tiene atajos.** `changes archive` se niega si la promoción tiene pendientes, así que la capa 1 no se puede satisfacer saltándose la promoción.
3. **SKILL.** Obliga a promover y archivar antes de mergear, y a **preguntar**: «¿es momento de ir?», con la aprobación explícita del humano registrada antes de abrir el PR o mergear. No es invención: `SKILL.md:377` ya dice «merge only after explicit user approval»; lo que falta es que la pregunta sea el paso, no una nota.
4. **CI.** Un workflow nuevo en `pull_request` que construya el binario y corra `ai-specs gates premerge --change "$GITHUB_HEAD_REF"`, marcado como *required check* en la protección de `development`.

**Viabilidad del CI en este repo, con la evidencia:** el único workflow es `release-binaries.yml`, disparado por tag `v*`, y ya sabe construir y testear Go (`go vet`/`go test ./cmd/... ./internal/...`); **no hay ningún workflow en `pull_request` ni ninguna invocación de un verbo `ai-specs`**. Así que la capa 4 es viable pero es trabajo nuevo: un `pr-gate.yml` con `actions/checkout`, setup de Go, `go build ./cmd/ai-specs` y la invocación del gate. Dos obstáculos reales:

- **HEAD detached.** `actions/checkout` en `pull_request` deja HEAD detached, y el premerge de `odd` resuelve el change con `symbolic-ref`, que devuelve vacío y produce el bloqueo «detached HEAD». Por eso P0 incluye `--change`: el workflow debe pasarlo explícitamente desde `GITHUB_HEAD_REF`. Sin ese override, la capa 4 no funciona en `odd`.
- **«Required» no vive en el repo.** La protección de rama y los required checks son configuración del repositorio (`gh api`), no un archivo versionado: hoy no hay ni ruleset ni CODEOWNERS en `.github/`. Es un paso manual de una vez, del dueño del repo, y hay que documentarlo en el README o se pierde.
- Límite honesto: el CI solo ve lo commiteado, así que el archive tiene que estar en la rama del PR — coherente con `SKILL.md:369-371` («then commit and push to the review branch») y con el squash, que ya invalida los shas de P3.

**Costo.** Medio para la capa 4 (workflow nuevo + un paso de configuración del repo) y bajo para las capas 2 y 3. **Risk item 5** (delivery/CI).

### R1 — Check de presencia de `Log` *(rechazada)*

Presencia de un encabezado no es evidencia de nada, y un check débil se satisface con una línea. `Log` entra en el contrato (P1) y se queda en prosa.

### R2 — `gates workunits`: contar commits y líneas por tarea *(rechazada)*

Gradúa la forma del historial, es frágil y se equivoca con casos legítimos (un refactor de 500 líneas con razón). La parte determinista y honesta ya está en P3.

### R3 — Importar la ceremonia ODD completa en `openspec` *(fuera de alcance)*

Candidato de revisión = un commit, `Risk: item N` obligatoria, espejo en memoria, verificador independiente: todo eso existe para el switch de RDD y para quien corre ODD. `openspec` existe para quien no lo hace, y S4 prohíbe sustituir sus artefactos.

### Lo que no se toca

Fechado del archive, paridad de deltas contra `openspec/specs/`, guardián de premerge, promoción atómica, y todo `odd` salvo la condición nueva de archivado de P4 (S1).

## Qué pieza cambia y cómo sigue siendo determinista

| Pieza | Archivo | Cambio | Determinismo |
| --- | --- | --- | --- |
| Resolvedor | `internal/planbuild/{pair,mode}.go` | `Change`, `PlanPath`, `ConfigPath`, `ArchivePath`, `--change` | Identidad = función de rama + repo, con override explícito para CI. |
| Gate | `internal/cli/planbuildgate.go` | plan del change emparejado + `PlanIsValid`; bloqueo por ambigüedad | Deja de depender de que exista «algún» plan. |
| Parser del plan | `internal/gates/tasks.go` (nuevo) | gramática `## Specs`/`## Tasks`/`## Design`/`## Verification`/`## Log`, con golden | Base de P1, P2, P3 y P6. Un solo parser. |
| Mínimos | `internal/gates/gates.go:78-105` | se encoge a la matriz de P6 | Menos reglas, mismas garantías por tier. |
| Evidencia | `internal/gates/{criteria,evidence}.go` | fuente `## Specs`/`## Verification`, mapeo `S#` o `Criterion N` | Cuenta filas contra requisitos declarados. |
| Config | `internal/planbuild` + nuevo en `internal/gates` | validación mínima + bucle de frescura del runner | El comando declarado debe existir y aparecer en la evidencia. |
| Archivado | `internal/cli/changes.go` (nuevo), `internal/gates/gates.go` | verbo `changes archive`; archive obligatorio en `odd` | Un solo `os.Rename`, se niega con pendientes. |
| Promoción | `internal/specs/promote.go` | identidad de P0 | Reuse de `PlanIsValid`/`SourceContained`. |
| CI | `.github/workflows/pr-gate.yml` (nuevo) | `pull_request` + `gates premerge --change` | El gate es el mismo binario, sin lógica paralela. |
| SKILL / docs | `skills/plan-build-flow/SKILL.md`, `catalog/recipes/plan-build-flow/README.md`, `openspec/config.yaml` | §4, §6, §7.1–§7.4, §8; enmendar `odd/config.yml` | La prosa describe lo que el CLI comprueba, sin reglas paralelas. |

Regla de reparto que se propone dejar escrita: **lo que el CLI puede comprobar sin juzgar historial se comprueba en Go; lo que requiere juicio se queda en prosa y no se simula con un check débil.** Deterministas: identidad, validez y gramática del plan, `S#`↔`PASS`, ancestro del sha, existencia del comando de tests, paridad, archivado. En prosa: redacción verbatim, granularidad de la unidad, ruta (`worker`/`inline`), y el juicio de cuándo es momento de ir.

## Tasks

| ID | Spec | Tarea | Ruta | Commit |
| --- | --- | --- | --- | --- |
| T1 | S8 | Commit de la evidencia final de #589 en `odd/tasks/plan-build-odd-companion.md` | inline | `d007708` |
| T2 | S1–S7 | Diseño revisión 1: divergencias, P0–P6, mapa CLI/SKILL | inline | `c52120d` |
| T3 | S9–S14 | Diseño revisión 2: un solo archivo de plan, archivado en ambos modos, `config.yaml`, ceremonia por tier, capas de aseguramiento | inline | `4ab2799` |
| T4 | S3 | Commit de la revisión 2 | inline | `4ab2799` |
| T5 | S6, S7 | I1/P0: `Change`, `PlanPath`, `ConfigPath`, `ArchivePath`, `--change`, gate y premerge con el resolvedor único, tests de emparejamiento y ambigüedad | inline | `3c9df4e` |
| T6 | S4, S5, S12 | I2/P1+P6: parser `tasks.md` con golden, matriz de mínimos por tier, SKILL §4/§6/§8 | worker | `14ec2b3` |
| T7 | S14 | I3/P2: `criteria`/`evidence` con fuente `## Specs`/`## Verification` y compatibilidad `Criterion N` | inline | `79d3df9` |
| T8 | S5, S6 | I4/P3: prosa de commits por tarea y check de ancestro del sha en premerge | inline | `01d159d` |
| T9 | S10 | I5/P4: verbo `changes archive`, archivado obligatorio en `odd`, premerge de ambos modos | inline | `ca374e7` |
| T10 | S11, S13 | I6/P5+P7: validación de `config.yaml` con bucle de frescura, `pr-gate.yml`, SKILL §7.3/§7.4, README | inline | `85acb2c` |

## Implementación propuesta (change siguiente, no esta vuelta)

Seis unidades de trabajo en este orden; cada una deja el suite en verde y admite revisión sola.

| ID | Propuesta | Contenido | Risk |
| --- | --- | --- | --- |
| I1 | P0 | `Change`, `PlanPath`, `ConfigPath`, `ArchivePath`, `--change`, gate y premerge con el resolvedor único, tests de emparejamiento y ambigüedad | item 3 |
| I2 | P1+P6 | `internal/gates/tasks.go` con golden, matriz de mínimos por tier, SKILL §4/§6/§8 | item 3 |
| I3 | P2 | `criteria.go`/`evidence.go` generalizados con doble fuente, compatibilidad `Criterion N` | item 3 |
| I4 | P3 | Prosa de commits por tarea, check de ancestro en premerge, límites documentados | item 3 |
| I5 | P4 | Verbo `changes archive`, archivado obligatorio en `odd`, premerge de ambos modos | item 3, item 1 |
| I6 | P5+P7 | Validación de `config.yaml` y bucle de frescura; workflow `pr-gate.yml`; SKILL §7.3/§7.4 | item 3, item 5 |

## Puntos abiertos (decisiones de Robert)

- **A'.** `config.yaml` en `odd`: ¿`odd/config.yaml` (simétrico, pero contradice `README.md:84` y el proposal de #589, que hay que enmendar), o un camino único para los dos modos? Recomiendo `odd/config.yaml` y enmendar la línea.
- **B'.** `odd/tasks/<c>.md` al archivar: ¿se queda como registro permanente de la feature (mi recomendación, diff mínimo sobre #589) o se mueve al archive como en `openspec` (más simétrico, mueve el documento que citan los handoffs)?
- **C'.** Obligatoriedad de `## Verification` en standard: ¿una fila por `S#` aunque el `S#` sea prosa sin test posible, o permitir `S#: N/A — <razón>`? Recomiendo permitir `N/A` con razón, porque sin esa válvula la gente escribe `PASS` sin probar nada.
- **D'.** `--change` en el gate: ¿override solo para CI (y el premerge del día a día sigue derivando de la rama), o override general aplicable a mano? Recomiendo general, con el valor derivado impreso en el veredicto para que se vea qué se calificó.
- Las antiguas A–D (rigidez del pairing, `S#` frente a delta, alcance del check de sha, un change o dos) quedan **resueltas por L6**: escalera de P0, `S#` verbatim con cita del delta, check de sha solo en la rama de review, un solo change.

## Log

- **L1** — Petición original, verbatim: *«Cody: sesión nueva para un change nuevo. - Worktree: esta. Rama: plan-build-openspec-odd-align, desde development 3a4e548. - Primero: el cambio sin commitear en odd/tasks/plan-build-odd-companion.md es la evidencia final del change anterior (#589, ya mergeado). Commitéalo como primer commit: docs, conventional commit. Objetivo, en palabras de Robert: «mejorar un poco el flujo actual de plan-build-flow centrado en openspec, pero más acercando lo que ya tenemos a odd». Alcance: solo el modo openspec, es decir cuando ODD no existe. El modo odd entregado en #589 no se toca. El comando de migración odd↔openspec es otro change. Recupera contexto en Engram (obs 502 y 503, el cierre de #589) y en el contrato ODD de Gentle Shell (contracts/review-provider-contract-mirror/.../routing/pi.md). Esta vuelta es SOLO DISEÑO: no toques código. Propón qué prácticas de ODD llevar al modo openspec sin dejar de usar sus artefactos (proposal, tasks.md, specs y archivado). Por ejemplo: - tasks.md con Specs S# que citan verbatim, Tasks enlazadas a S# y Log; - commit o evidencia por tarea; - verificación por S#; - commits por unidad de trabajo; - el gate y premerge usando el mismo resolvedor. Para cada propuesta da su valor y su costo, y cuáles recomiendas. Indica qué piezas del CLI o del SKILL cambian y cómo se mantiene determinista desde el CLI. Escribe el plan en odd/tasks/plan-build-openspec-odd-align.md, porque este repo ya está en modo odd. Commit del plan permitido, sin push. Reporte corto con las propuestas y tu recomendación. Cierra con 'LISTO DISEÑO'.»*
- **L2** — Contexto recuperado de Engram: obs 502 y obs 503 (entrega de #589, decisiones, pendientes). El contrato ODD está en `assets/orchestrator-memory.md` y `assets/orchestrator.md:27-29` del paquete gentle-pi; el `routing/pi.md` que citaba la petición no existe en esa ruta.
- **L3** — Evidencia de código, spot-check propio: `planbuildgate.go:282` (el gate abría con cualquier `tasks.md`), `criteria.go:29` (fuente única de requisitos), `gates.go:100-160` (premerge recibía el slug por argumento). El resto lo devolvió exploración delegada de solo lectura.
- **L4** — Decisión de la revisión 1: solo diseño; commits `d007708` (evidencia de #589) y `c52120d` (diseño v1). Sin push, sin implementación, sin crear `odd/changes/`.
- **L5** — Redirección de Robert, verbatim en sus fragmentos clave: *«lo que debemos rescatar es specs + un solo archivo que haga lo que hace odd hoy en día pero solo specs + tasks pero bajo openspec»*; *«el archive en ambos casos debe ser obligatorio pre merge»*; *«también debe achicarse la ceremonia cuando no sea necesaria»*; *«el agente pregunta si es momento de ir»*. Aprueba A–D y pide rehacer el diseño con P0, P2 y P3 ajustados; que el plan sea UN archivo con Specs/Tasks/Log y que `proposal.md`, `design.md` y `verify-report.md` dejen de ser obligatorios («lo que hoy aportan vive dentro de ese archivo»). El resto del mensaje llegó como resumen de Cody, no como texto literal de Robert. Consecuencias: S4 se reescribe; la numeración de propuestas cambia y las rechazadas pasan a R1–R3; el archivado obligatorio en `odd` revierte a propósito #589, así que S1 se matiza.
- **L6** — Robert aprueba A–D de la revisión 1: escalera de emparejamiento de P0, `S#` verbatim con cita del delta, check de sha solo en la rama de review, y un solo change (no dos).
- **L7** — Aclaración de Robert sobre «el config.yml»: es `openspec/config.yaml` (schema, context, strict_tdd, testing), y su frase completa es *«specs + un solo archivo… y creo que el config.yaml [y] el archive en ambos casos debe ser obligatorio pre merge»*. Pide integrarlo: se rescata como configuración del proyecto, hay que proponer su papel en ambos modos y si el premerge exige que exista y sea válido, y hay que resolver que el actual está obsoleto (describe Bash/Python y `./tests/run.sh` mientras el CLI es Go). Resuelto en P5 y en S11.
- **L8** — Evidencia de la revisión 2, verificada antes de escribir: no existe ningún workflow en `pull_request` (`.github/workflows/release-binaries.yml:14` es solo por tag, y ya construye y testea Go), `openspec/config.yaml` solo lo lee el tracker (`checks_tracker.go:52`) y ningún validador lo mira, `.github/` no tiene ruleset ni CODEOWNERS, y `actions/checkout` en `pull_request` deja HEAD detached, lo que invalida un premerge de `odd` derivado solo de la rama. De ahí el `--change` en P0 y el obstáculo declarado en P7.
- **L9** — A'–D' resueltas por Cody como decisiones técnicas, sobre el diseño v2 ya aprobado por Robert: **A'** `odd/config.yaml`, enmendando `README.md:84` y el proposal de #589, porque Robert pidió `config.yaml` obligatorio en ambos modos; **B'** `odd/tasks/<c>.md` se queda como registro permanente; **C'** se permite `S#: N/A — <razón>`; **D'** `--change` es override general y el veredicto imprime el valor derivado.
- **L10** — Autorización de implementación: I1–I6 en orden, un commit por unidad, tests por regla y la revisión nativa que corresponda. Permisos vigentes: commits, push de la rama, PR contra `development` y card de Trello con su sección Tracker. El merge no. En I6 el workflow `pr-gate.yml` sí va; marcarlo como required check en la protección de `development` **no**, porque es configuración del repositorio y la decide Robert: queda documentado en el README y en el PR. Smoke con el binario en repos temporales en ambos modos (archive, premerge, frescura de config).
- **L11** — El MCP de Trello no está conectado a esta sesión (verificado: ninguna herramienta `mcp__trello__*` en el inventario). La card `6b04e0e37aa95686f307241f` se creó con la API REST usando `TRELLO_API_KEY`/`TRELLO_TOKEN` del entorno, en la lista *In Progress* del tablero `69ec097f13e2d38ecd89a557`. No es un fallo de disponibilidad del tracker: el tracker estaba disponible por API.
- **L12** — Implementadas I1–I6, un commit por unidad: I1 `3c9df4e` (resolvedor y `--change`), I2 `14ec2b3` (parser del plan y forma obligatoria por tier), I3 `79d3df9` (verificación por `S#` y retirada de los mínimos de fichero), I4 `01d159d` (celda `Commit` contra la rama), I5 `ca374e7` (verbo `changes archive` y archivado obligatorio en `odd`), I6 `85acb2c` (`config.yaml` validado con bucle de frescura y workflow `pr-gate`). Desviación de secuencia, mía: I2 fue aditivo y la retirada de `proposal.md`/`design.md`/`verify-report.md` ocurrió en I3 junto con la generalización del grader, porque el mapeo de criterios de `full` leía `proposal.md`; así ningún commit intermedio debilitó un gate.
- **L13** — Dos guardarraíles que la verificación encontró y que no venían del spec: (a) el verbo de archivado movía directorios con el nombre de change venido de la CLI, así que `..` o `a/b` eran una traversía de rutas; se cerró con `planbuild.ValidChangeName` como predicado único antes de cualquier movimiento, con test. (b) el bucle de frescura comparaba solo la primera palabra del comando, así que `go vet` satisfacía `runner: go test`; se endureció a programa + subcomando (`commandHead`), que atrapa el cambio de `./tests/run.sh` a `go test` y tolera una corrida enfocada del mismo runner. El endurecimiento completo a comando exacto rompía fixtures legítimas y se descartó.
- **L14** — Smoke del binario en repos temporales, 19/19: en `odd`, premerge bloqueado antes de promover y archivar, promovido, `changes archive` mueve el delta y deja el plan en `odd/tasks/`, premerge OK emparejando por rama a través del plan supervivente, y las tres reglas de frescura (comando inexistente bloquea, runner que no corrió bloquea, runner coincidente pasa); en `openspec`, pre-archive, promoción, archive con el plan dentro, premerge, re-archive rechazado, y la ceremonia reducida (light sin config pasa, standard sin config bloquea).
- **L15** — El modo `odd` sí queda con deber de config, pero solo cuando el plan declara `Depth:`: un plan sin esa línea conserva el contrato estrecho de #589 (plan, archivado y paridad) para no bloquear retroactivamente los 154 planes existentes. Se decidió así tras descubrir en el smoke que `CheckPremergeOdd` no pasa por `inspectFolder` y por tanto la config no se le aplicaba en absoluto.
- **L16** — Segunda pasada del verificador independiente: los seis defectos de L13/L14 quedaron cerrados, y encontró siete más, dos introducidos por mis propios arreglos. Decisiones:
  - se **revierte** la exención del archive sin fecha (estaba llaveada por el nombre del directorio, así que renombrar un change nuevo a `archive/<slug>` saltaba la gramática del plan); ahora todo archive se califica, y un archive anterior al contrato debe reescribirse si alguien necesita volver a correr el guardián sobre él;
  - el archive debe ser **probatorio por contenido** (plan o delta dentro), no una carpeta vacía;
  - el guardián de `odd` comprueba la paridad del delta **archivado**, no solo del activo;
  - `--root .` califica igual que `--root "$PWD"`, arreglado en los predicados de contención y no escondido en el workflow;
  - el nombre del change calificado se valida con `ValidChangeName` en ambos modos;
  - las filas de `## Verification` deben abrir con un único comando en backticks, no pueden registrar un fallo, admiten `N/A` con razón y no se duplican;
  - `## Log` es obligatorio desde standard (matriz P6).
- **L17** — PR [#590](https://github.com/parada1104/ai-specs-cli/pull/590) abierto contra `development`. `assess` marca el candidato como `reviewDue` por riesgo alto. La revisión nativa **no llegó a crear autoridad**: `review.start` paró en `lens_context_budget_exceeded` porque el candidato completo (4.611 líneas, 38 ficheros) excede el presupuesto de evidencia del revisor; el proveedor pide revisar candidatos más pequeños. No hay linaje que abandonar ni autoridad que reparar (`mutation_outcome: not_started`). Reducir el candidato significa en la práctica entregar por rebanadas encadenadas, y esa es una decisión de forma de entrega, no técnica: queda para Robert junto con el consentimiento.
- **Estado al cierre de esta sesión**: I1–I6 implementadas y commiteadas, rama pusheada, PR abierto, `go test ./...` en verde, paridad en su baseline conocido, 13 defectos de dos pasadas independientes cerrados con test. Merge **no** hecho. Requisito pendiente antes de mergear: resolver la revisión nativa (rebanada más pequeña + consentimiento), y la marca de required check de `pr-gate`, que es del dueño.

